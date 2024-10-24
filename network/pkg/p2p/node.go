package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

// Node represents a P2P network node
type Node struct {
	// Configuration
	config *types.NetworkConfig

	// LibP2P host
	host host.Host

	// DHT for peer discovery
	dht *dht.IpfsDHT

	// Connected peers
	peers    map[peer.ID]*Peer
	peerLock sync.RWMutex

	// Protocol handlers
	handlers map[protocol.ID]MessageHandler

	// Channel for new messages
	msgChan chan types.Message

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Logger
	logger *zap.Logger
}

// MessageHandler handles incoming messages
type MessageHandler func(msg types.Message) error

// NewNode creates a new P2P node
func NewNode(config *types.NetworkConfig) (*Node, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize logger
	logger, _ := zap.NewProduction()

	// Create libp2p host
	h, err := createHost(config)
	if err != nil {
		cancel()
		return nil, err
	}

	// Create DHT
	kadDHT, err := dht.New(ctx, h)
	if err != nil {
		cancel()
		return nil, err
	}

	node := &Node{
		config:   config,
		host:     h,
		dht:      kadDHT,
		peers:    make(map[peer.ID]*Peer),
		handlers: make(map[protocol.ID]MessageHandler),
		msgChan:  make(chan types.Message, 1000),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
	}

	return node, nil
}

// Start starts the node
func (n *Node) Start() error {
	// Start DHT
	if err := n.dht.Bootstrap(n.ctx); err != nil {
		return err
	}

	// Connect to bootstrap peers
	for _, addr := range n.config.BootstrapPeers {
		if err := n.connectToPeer(addr); err != nil {
			n.logger.Error("Failed to connect to bootstrap peer",
				zap.String("addr", addr),
				zap.Error(err))
		}
	}

	// Start peer discovery
	go n.discoverPeers()

	// Start message handler
	go n.handleMessages()

	n.logger.Info("Node started",
		zap.String("id", n.host.ID().String()),
		zap.Strings("addresses", multiaddrsToStrings(n.host.Addrs())))

	return nil
}

// Stop stops the node
func (n *Node) Stop() error {
	n.cancel()

	if err := n.host.Close(); err != nil {
		return err
	}

	n.logger.Info("Node stopped", zap.String("id", n.host.ID().String()))
	return nil
}

// Broadcast broadcasts a message to all peers
func (n *Node) Broadcast(msg types.Message) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	for _, peer := range n.peers {
		if err := peer.SendMessage(msg); err != nil {
			n.logger.Error("Failed to send message to peer",
				zap.String("peer", peer.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// RegisterHandler registers a message handler for a protocol
func (n *Node) RegisterHandler(protocolID protocol.ID, handler MessageHandler) {
	n.handlers[protocolID] = handler
}

// AddPeer adds a new peer
func (n *Node) AddPeer(peerInfo peer.AddrInfo) error {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if len(n.peers) >= n.config.MaxPeers {
		return types.NetworkError{
			Code:    types.ErrCodePeerConnection,
			Message: "max peers reached",
		}
	}

	// Create new peer
	p := NewPeer(peerInfo, n.host)

	// Connect to peer
	if err := p.Connect(n.ctx); err != nil {
		return err
	}

	n.peers[peerInfo.ID] = p

	n.logger.Info("Added new peer",
		zap.String("peer", peerInfo.ID.String()),
		zap.Int("total_peers", len(n.peers)))

	return nil
}

// RemovePeer removes a peer
func (n *Node) RemovePeer(id peer.ID) {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if p, exists := n.peers[id]; exists {
		p.Disconnect()
		delete(n.peers, id)

		n.logger.Info("Removed peer",
			zap.String("peer", id.String()),
			zap.Int("total_peers", len(n.peers)))
	}
}

// GetPeers returns all connected peers
func (n *Node) GetPeers() []*Peer {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	peers := make([]*Peer, 0, len(n.peers))
	for _, p := range n.peers {
		peers = append(peers, p)
	}
	return peers
}

// Internal methods

func createHost(config *types.NetworkConfig) (host.Host, error) {
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(config.ListenAddresses...),
		libp2p.Identity(config.PrivateKey),
	}

	return libp2p.New(opts...)
}

func (n *Node) connectToPeer(addr string) error {
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return err
	}

	return n.AddPeer(*peerInfo)
}

func (n *Node) discoverPeers() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			peers, err := n.dht.FindPeers(n.ctx, n.config.ProtocolID)
			if err != nil {
				n.logger.Error("Peer discovery failed", zap.Error(err))
				continue
			}

			for p := range peers {
				if err := n.AddPeer(p); err != nil {
					n.logger.Error("Failed to add discovered peer",
						zap.String("peer", p.ID.String()),
						zap.Error(err))
				}
			}
		}
	}
}

func (n *Node) handleMessages() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case msg := <-n.msgChan:
			if handler, exists := n.handlers[protocol.ID(msg.Type)]; exists {
				if err := handler(msg); err != nil {
					n.logger.Error("Message handler failed",
						zap.String("type", string(msg.Type)),
						zap.Error(err))
				}
			}
		}
	}
}

func multiaddrsToStrings(addrs []multiaddr.Multiaddr) []string {
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}
	return result
}

// GetNodeInfo returns information about the node
func (n *Node) GetNodeInfo() types.NodeInfo {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	return types.NodeInfo{
		ID:         n.host.ID().String(),
		Addresses:  multiaddrsToStrings(n.host.Addrs()),
		PeerCount:  len(n.peers),
		ProtocolID: n.config.ProtocolID,
		Version:    n.config.Version,
	}
}

// SendMessage sends a message to a specific peer
func (n *Node) SendMessage(peerID peer.ID, msg types.Message) error {
	n.peerLock.RLock()
	p, exists := n.peers[peerID]
	n.peerLock.RUnlock()

	if !exists {
		return types.NetworkError{
			Code:    types.ErrCodePeerConnection,
			Message: "peer not found",
		}
	}

	return p.SendMessage(msg)
}

// GetMessageChan returns the message channel
func (n *Node) GetMessageChan() <-chan types.Message {
	return n.msgChan
}
