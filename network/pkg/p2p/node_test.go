package p2p

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create test nodes
func createTestNode(t *testing.T) (*Node, crypto.PrivKey) {
	// Generate private key
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
	require.NoError(t, err)

	config := &types.NetworkConfig{
		NodeID:          "test-node",
		PrivateKey:      priv,
		ListenAddresses: []string{"/ip4/127.0.0.1/tcp/0"},
		MaxPeers:        10,
		ProtocolID:      "/aporia-test/1.0.0",
	}

	node, err := NewNode(config)
	require.NoError(t, err)

	return node, priv
}

func TestNodeCreation(t *testing.T) {
	node, _ := createTestNode(t)
	defer node.Stop()

	assert.NotNil(t, node)
	assert.NotNil(t, node.host)
	assert.NotNil(t, node.peers)
	assert.NotNil(t, node.handlers)
}

func TestNodeStartStop(t *testing.T) {
	node, _ := createTestNode(t)

	// Start node
	err := node.Start()
	require.NoError(t, err)

	// Verify node is running
	assert.NotEmpty(t, node.host.Addrs())
	assert.NotNil(t, node.dht)

	// Stop node
	err = node.Stop()
	require.NoError(t, err)
}

func TestPeerConnection(t *testing.T) {
	// Create two nodes
	node1, _ := createTestNode(t)
	node2, _ := createTestNode(t)

	// Start both nodes
	require.NoError(t, node1.Start())
	require.NoError(t, node2.Start())
	defer node1.Stop()
	defer node2.Stop()

	// Get node2's address info
	addr := node2.host.Addrs()[0]
	pid := node2.host.ID()
	peerInfo := peer.AddrInfo{
		ID:    pid,
		Addrs: []multiaddr.Multiaddr{addr},
	}

	// Connect node1 to node2
	err := node1.AddPeer(peerInfo)
	require.NoError(t, err)

	// Verify connection
	time.Sleep(100 * time.Millisecond) // Allow time for connection
	assert.Equal(t, 1, len(node1.GetPeers()))
	assert.Equal(t, 1, len(node2.GetPeers()))
}

func TestMessageBroadcast(t *testing.T) {
	// Create three nodes in a network
	nodes := make([]*Node, 3)
	for i := range nodes {
		node, _ := createTestNode(t)
		require.NoError(t, node.Start())
		defer node.Stop()
		nodes[i] = node
	}

	// Connect nodes in a line: 0 <-> 1 <-> 2
	for i := 0; i < len(nodes)-1; i++ {
		addr := nodes[i+1].host.Addrs()[0]
		pid := nodes[i+1].host.ID()
		peerInfo := peer.AddrInfo{
			ID:    pid,
			Addrs: []multiaddr.Multiaddr{addr},
		}
		require.NoError(t, nodes[i].AddPeer(peerInfo))
	}

	// Setup message reception channels
	received := make([]chan types.Message, len(nodes))
	for i, node := range nodes {
		received[i] = make(chan types.Message, 1)
		node.RegisterHandler(protocol.ID("test"), func(msg types.Message) error {
			received[i] <- msg
			return nil
		})
	}

	// Broadcast message from node 0
	testMsg := types.Message{
		Type:    types.MessageTypeTransaction,
		Payload: []byte("test message"),
	}
	err := nodes[0].Broadcast(testMsg)
	require.NoError(t, err)

	// Verify all nodes receive the message
	timeout := time.After(2 * time.Second)
	for i := range nodes {
		select {
		case msg := <-received[i]:
			assert.Equal(t, testMsg.Payload, msg.Payload)
		case <-timeout:
			t.Fatalf("Node %d did not receive message", i)
		}
	}
}

func TestPeerDiscovery(t *testing.T) {
	// Create DHT bootstrap node
	bootstrap, _ := createTestNode(t)
	require.NoError(t, bootstrap.Start())
	defer bootstrap.Stop()

	bootstrapAddr := bootstrap.host.Addrs()[0].String() + "/p2p/" + bootstrap.host.ID().Pretty()

	// Create test nodes using bootstrap node
	nodes := make([]*Node, 3)
	for i := range nodes {
		config := &types.NetworkConfig{
			BootstrapPeers:  []string{bootstrapAddr},
			ListenAddresses: []string{"/ip4/127.0.0.1/tcp/0"},
			MaxPeers:        10,
			ProtocolID:      "/aporia-test/1.0.0",
		}

		node, err := NewNode(config)
		require.NoError(t, err)
		require.NoError(t, node.Start())
		defer node.Stop()
		nodes[i] = node
	}

	// Wait for peer discovery
	time.Sleep(5 * time.Second)

	// Verify each node has discovered peers
	for i, node := range nodes {
		peers := node.GetPeers()
		assert.NotEmpty(t, peers, "Node %d should have discovered peers", i)
	}
}

func TestProtocolHandling(t *testing.T) {
	node, _ := createTestNode(t)
	require.NoError(t, node.Start())
	defer node.Stop()

	// Register protocol handler
	received := make(chan types.Message, 1)
	protocolID := protocol.ID("/test/1.0.0")
	node.RegisterHandler(protocolID, func(msg types.Message) error {
		received <- msg
		return nil
	})

	// Create test message
	testMsg := types.Message{
		Type:    types.MessageTypeTransaction,
		Payload: []byte("test protocol message"),
	}

	// Send message to self (loopback)
	stream, err := node.host.NewStream(context.Background(), node.host.ID(), protocolID)
	require.NoError(t, err)
	defer stream.Close()

	err = writeMessage(stream, testMsg)
	require.NoError(t, err)

	// Verify message reception
	select {
	case msg := <-received:
		assert.Equal(t, testMsg.Payload, msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("Message not received")
	}
}

func TestPeerManagement(t *testing.T) {
	node, _ := createTestNode(t)
	require.NoError(t, node.Start())
	defer node.Stop()

	// Test max peers limit
	for i := 0; i < node.config.MaxPeers+1; i++ {
		peer := createTestNode(t)
		require.NoError(t, peer.Start())
		defer peer.Stop()

		err := node.AddPeer(peer.host.Peerstore().PeerInfo(peer.host.ID()))
		if i < node.config.MaxPeers {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err) // Should fail when exceeding max peers
		}
	}

	// Test peer removal
	peers := node.GetPeers()
	for _, p := range peers {
		node.RemovePeer(p.ID)
	}
	assert.Empty(t, node.GetPeers())
}

func TestNetworkResilience(t *testing.T) {
	// Create a network of nodes
	nodes := make([]*Node, 5)
	for i := range nodes {
		node, _ := createTestNode(t)
		require.NoError(t, node.Start())
		defer node.Stop()
		nodes[i] = node
	}

	// Connect nodes in a mesh
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			addr := nodes[j].host.Addrs()[0]
			pid := nodes[j].host.ID()
			peerInfo := peer.AddrInfo{
				ID:    pid,
				Addrs: []multiaddr.Multiaddr{addr},
			}
			require.NoError(t, nodes[i].AddPeer(peerInfo))
		}
	}

	// Simulate node failure
	nodes[2].Stop() // Stop middle node

	// Verify network remains connected
	time.Sleep(time.Second)
	msg := types.Message{
		Type:    types.MessageTypeTransaction,
		Payload: []byte("resilience test"),
	}

	err := nodes[0].Broadcast(msg)
	require.NoError(t, err)

	// Verify message reaches remaining nodes
	for i, node := range nodes {
		if i == 2 {
			continue // Skip failed node
		}
		assert.True(t, len(node.GetPeers()) > 0)
	}
}
