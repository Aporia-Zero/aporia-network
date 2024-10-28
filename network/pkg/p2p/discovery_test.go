package p2p

import (
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDiscovery(t *testing.T) (*DiscoveryService, host.Host) {
	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)

	config := DiscoveryConfig{
		DHTEnabled:        true,
		MDNSEnabled:       true,
		MDNSInterval:      time.Second,
		DiscoveryInterval: time.Second,
		MaxPeers:          10,
	}

	discovery, err := NewDiscoveryService(h, config)
	require.NoError(t, err)

	return discovery, h
}

func TestDiscoveryServiceCreation(t *testing.T) {
	discovery, host := setupTestDiscovery(t)
	defer host.Close()

	assert.NotNil(t, discovery)
	assert.NotNil(t, discovery.host)
	assert.NotNil(t, discovery.peers)
	if discovery.config.DHTEnabled {
		assert.NotNil(t, discovery.dht)
	}
}

func TestDiscoveryServiceStartStop(t *testing.T) {
	discovery, host := setupTestDiscovery(t)
	defer host.Close()

	// Start service
	err := discovery.Start()
	require.NoError(t, err)

	// Verify service is running
	assert.NotNil(t, discovery.ctx)
	if discovery.config.DHTEnabled {
		assert.NotNil(t, discovery.dht)
	}

	// Stop service
	discovery.Stop()
	assert.Eventually(t, func() bool {
		select {
		case <-discovery.ctx.Done():
			return true
		default:
			return false
		}
	}, time.Second, 100*time.Millisecond)
}

func TestPeerDiscoveryViaDHT(t *testing.T) {
	// Create bootstrap node
	bootstrap, bootstrapHost := setupTestDiscovery(t)
	require.NoError(t, bootstrap.Start())
	defer bootstrapHost.Close()

	// Create test nodes
	nodes := make([]*DiscoveryService, 3)
	hosts := make([]host.Host, 3)

	bootstrapAddr := bootstrapHost.Addrs()[0].String() + "/p2p/" + bootstrapHost.ID().Pretty()

	for i := range nodes {
		config := DiscoveryConfig{
			DHTEnabled:        true,
			BootstrapPeers:    []string{bootstrapAddr},
			DiscoveryInterval: time.Second,
			MaxPeers:          10,
		}

		h, err := libp2p.New(
			libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
			libp2p.DisableRelay(),
		)
		require.NoError(t, err)

		discovery, err := NewDiscoveryService(h, config)
		require.NoError(t, err)
		require.NoError(t, discovery.Start())

		nodes[i] = discovery
		hosts[i] = h
		defer h.Close()
	}

	// Wait for peer discovery
	time.Sleep(5 * time.Second)

	// Verify each node has discovered peers
	for i, node := range nodes {
		assert.Greater(t, node.GetPeerCount(), 0, "Node %d should have discovered peers", i)
	}
}

func TestMDNSDiscovery(t *testing.T) {
	// Create nodes with MDNS enabled
	config := DiscoveryConfig{
		MDNSEnabled:  true,
		MDNSInterval: time.Second,
		MaxPeers:     10,
	}

	nodes := make([]*DiscoveryService, 3)
	hosts := make([]host.Host, 3)

	for i := range nodes {
		h, err := libp2p.New(
			libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
			libp2p.DisableRelay(),
		)
		require.NoError(t, err)

		discovery, err := NewDiscoveryService(h, config)
		require.NoError(t, err)
		require.NoError(t, discovery.Start())

		nodes[i] = discovery
		hosts[i] = h
		defer h.Close()
	}

	// Wait for MDNS discovery
	time.Sleep(3 * time.Second)

	// Verify peer discovery
	for i, node := range nodes {
		peers := node.GetPeerCount()
		assert.Greater(t, peers, 0, "Node %d should have discovered peers via MDNS", i)
	}
}

func TestMaxPeersLimit(t *testing.T) {
	// Create discovery service with low peer limit
	config := DiscoveryConfig{
		MaxPeers: 2,
	}

	discovery, host := setupTestDiscovery(t)
	discovery.config = config
	defer host.Close()

	// Try to add more peers than the limit
	for i := 0; i < config.MaxPeers+2; i++ {
		peerID := peer.ID(fmt.Sprintf("peer-%d", i))
		peerInfo := peer.AddrInfo{
			ID:    peerID,
			Addrs: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/1234")},
		}

		discovery.HandlePeerFound(peerInfo)
	}

	assert.LessOrEqual(t, discovery.GetPeerCount(), config.MaxPeers)
}

func TestPeerRemoval(t *testing.T) {
	discovery, host := setupTestDiscovery(t)
	defer host.Close()

	// Add test peer
	peerID := peer.ID("test-peer")
	peerInfo := peer.AddrInfo{
		ID:    peerID,
		Addrs: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/1234")},
	}

	discovery.HandlePeerFound(peerInfo)
	assert.Equal(t, 1, discovery.GetPeerCount())

	// Remove peer
	discovery.RemovePeer(peerID)
	assert.Equal(t, 0, discovery.GetPeerCount())
}

func TestDiscoveryChannelBuffer(t *testing.T) {
	discovery, host := setupTestDiscovery(t)
	defer host.Close()

	// Get discovery channel
	discoveryChan := discovery.GetDiscoveredPeers()

	// Add multiple peers quickly
	for i := 0; i < 20; i++ {
		peerID := peer.ID(fmt.Sprintf("peer-%d", i))
		peerInfo := peer.AddrInfo{
			ID:    peerID,
			Addrs: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/1234")},
		}

		discovery.HandlePeerFound(peerInfo)
	}

	// Verify channel doesn't block and some peers are received
	peerCount := 0
	timeout := time.After(time.Second)

	for {
		select {
		case <-discoveryChan:
			peerCount++
		case <-timeout:
			assert.Greater(t, peerCount, 0)
			return
		}
	}
}

func TestDHTBootstrapping(t *testing.T) {
	// Create bootstrap node
	bootstrap, bootstrapHost := setupTestDiscovery(t)
	require.NoError(t, bootstrap.Start())
	defer bootstrapHost.Close()

	bootstrapAddr := bootstrapHost.Addrs()[0].String() + "/p2p/" + bootstrapHost.ID().Pretty()

	// Create node that uses bootstrap peer
	config := DiscoveryConfig{
		DHTEnabled:     true,
		BootstrapPeers: []string{bootstrapAddr},
	}

	discovery, host := setupTestDiscovery(t)
	discovery.config = config
	defer host.Close()

	// Start discovery and wait for bootstrap
	err := discovery.Start()
	require.NoError(t, err)
	time.Sleep(2 * time.Second)

	// Verify DHT is bootstrapped
	assert.NotNil(t, discovery.dht)
	routing, err := discovery.dht.GetRoutingTable()
	require.NoError(t, err)
	assert.Greater(t, routing.Size(), 0)
}

func TestDiscoveryResilience(t *testing.T) {
	// Create network of discovery services
	services := make([]*DiscoveryService, 5)
	hosts := make([]host.Host, 5)

	for i := range services {
		discovery, host := setupTestDiscovery(t)
		require.NoError(t, discovery.Start())
		services[i] = discovery
		hosts[i] = host
		defer host.Close()
	}

	// Wait for initial discovery
	time.Sleep(2 * time.Second)

	// Stop middle node
	hosts[2].Close()
	services[2].Stop()

	// Wait for network to adjust
	time.Sleep(2 * time.Second)

	// Verify remaining nodes are still connected
	for i, service := range services {
		if i == 2 {
			continue // Skip stopped node
		}
		assert.Greater(t, service.GetPeerCount(), 0)
	}

	// Add new node
	newDiscovery, newHost := setupTestDiscovery(t)
	require.NoError(t, newDiscovery.Start())
	defer newHost.Close()

	// Verify new node discovers existing network
	time.Sleep(2 * time.Second)
	assert.Greater(t, newDiscovery.GetPeerCount(), 0)
}
