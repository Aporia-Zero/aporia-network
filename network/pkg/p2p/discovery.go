package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

// Discovery service configuration
type DiscoveryConfig struct {
	// DHT configuration
	DHTEnabled     bool
	BootstrapPeers []string

	// MDNS configuration
	MDNSEnabled  bool
	MDNSInterval time.Duration

	// Discovery intervals
	DiscoveryInterval time.Duration

	// Limits
	MaxPeers int
}

// Discovery service manages peer discovery
type DiscoveryService struct {
	host   host.Host
	dht    *dht.IpfsDHT
	config DiscoveryConfig

	// Discovered peers
	peers    map[peer.ID]peer.AddrInfo
	peerLock sync.RWMutex

	// Channels
	discoveries chan peer.AddrInfo

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	logger *zap.Logger
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(h host.Host, cfg DiscoveryConfig) (*DiscoveryService, error) {
	ctx, cancel := context.WithCancel(context.Background())
	logger, _ := zap.NewProduction()

	// Create DHT if enabled
	var kadDHT *dht.IpfsDHT
	var err error
	if cfg.DHTEnabled {
		kadDHT, err = dht.New(ctx, h)
		if err != nil {
			cancel()
			return nil, err
		}
	}

	return &DiscoveryService{
		host:        h,
		dht:         kadDHT,
		config:      cfg,
		peers:       make(map[peer.ID]peer.AddrInfo),
		discoveries: make(chan peer.AddrInfo, 100),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}, nil
}

// Start starts the discovery service
func (d *DiscoveryService) Start() error {
	// Start DHT if enabled
	if d.config.DHTEnabled {
		if err := d.startDHT(); err != nil {
			return err
		}
	}

	// Start MDNS if enabled
	if d.config.MDNSEnabled {
		if err := d.startMDNS(); err != nil {
			return err
		}
	}

	// Start discovery loop
	go d.discoveryLoop()

	d.logger.Info("Discovery service started")
	return nil
}

// Stop stops the discovery service
func (d *DiscoveryService) Stop() {
	d.cancel()
	if d.dht != nil {
		d.dht.Close()
	}
	d.logger.Info("Discovery service stopped")
}

// GetDiscoveredPeers returns a channel of discovered peers
func (d *DiscoveryService) GetDiscoveredPeers() <-chan peer.AddrInfo {
	return d.discoveries
}

// Internal methods

func (d *DiscoveryService) startDHT() error {
	// Bootstrap DHT
	if err := d.dht.Bootstrap(d.ctx); err != nil {
		return err
	}

	// Connect to bootstrap peers
	for _, addr := range d.config.BootstrapPeers {
		if err := d.connectToBootstrapPeer(addr); err != nil {
			d.logger.Error("Failed to connect to bootstrap peer",
				zap.String("addr", addr),
				zap.Error(err))
		}
	}

	return nil
}

func (d *DiscoveryService) startMDNS() error {
	// Create MDNS service
	service := mdns.NewMdnsService(d.host, "aporia-zero", d)
	if err := service.Start(); err != nil {
		return err
	}

	return nil
}

// HandlePeerFound implements the MDNS PeerHandler interface
func (d *DiscoveryService) HandlePeerFound(pi peer.AddrInfo) {
	d.addDiscoveredPeer(pi)
}

func (d *DiscoveryService) connectToBootstrapPeer(addr string) error {
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}

	pi, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return err
	}

	return d.host.Connect(d.ctx, *pi)
}

func (d *DiscoveryService) discoveryLoop() {
	ticker := time.NewTicker(d.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if d.config.DHTEnabled {
				d.findPeersViaDHT()
			}
		}
	}
}

func (d *DiscoveryService) findPeersViaDHT() {
	peers, err := d.dht.FindPeers(d.ctx, "aporia-zero")
	if err != nil {
		d.logger.Error("DHT peer discovery failed", zap.Error(err))
		return
	}

	for p := range peers {
		d.addDiscoveredPeer(p)
	}
}

func (d *DiscoveryService) addDiscoveredPeer(pi peer.AddrInfo) {
	d.peerLock.Lock()
	defer d.peerLock.Unlock()

	// Check if we have space for new peers
	if len(d.peers) >= d.config.MaxPeers {
		return
	}

	// Check if peer is already known
	if _, exists := d.peers[pi.ID]; exists {
		return
	}

	// Add peer
	d.peers[pi.ID] = pi

	// Notify about new peer
	select {
	case d.discoveries <- pi:
	default:
		// Channel is full, skip notification
	}

	d.logger.Info("Discovered new peer",
		zap.String("peer", pi.ID.String()),
		zap.Int("total_peers", len(d.peers)))
}

// GetPeerCount returns the number of discovered peers
func (d *DiscoveryService) GetPeerCount() int {
	d.peerLock.RLock()
	defer d.peerLock.RUnlock()
	return len(d.peers)
}

// RemovePeer removes a peer from the discovered peers list
func (d *DiscoveryService) RemovePeer(id peer.ID) {
	d.peerLock.Lock()
	defer d.peerLock.Unlock()

	delete(d.peers, id)
	d.logger.Info("Removed peer",
		zap.String("peer", id.String()),
		zap.Int("total_peers", len(d.peers)))
}
