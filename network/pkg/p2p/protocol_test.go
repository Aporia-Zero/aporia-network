package p2p

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProtocolTest(t *testing.T) (*ProtocolManager, host.Host) {
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)

	pm := NewProtocolManager(h)
	return pm, h
}

func TestProtocolRegistration(t *testing.T) {
	pm, host := setupProtocolTest(t)
	defer host.Close()

	// Create test protocol
	protocolID := protocol.ID("/test/1.0.0")
	handler := func(msg types.Message) error {
		return nil
	}

	// Register protocol
	err := pm.RegisterProtocol(protocolID, handler)
	require.NoError(t, err)

	// Verify protocol registration
	protocol, exists := pm.GetProtocol(protocolID)
	assert.True(t, exists)
	assert.NotNil(t, protocol)
	assert.Equal(t, protocolID, protocol.ID)

	// Test duplicate registration
	err = pm.RegisterProtocol(protocolID, handler)
	assert.Error(t, err)
}

func TestProtocolCommunication(t *testing.T) {
	// Create two protocol managers
	pm1, host1 := setupProtocolTest(t)
	pm2, host2 := setupProtocolTest(t)
	defer host1.Close()
	defer host2.Close()

	// Connect hosts
	addr := host2.Addrs()[0]
	peer := host2.ID()
	host1.Connect(context.Background(), peer.AddrInfo{
		ID:    peer,
		Addrs: []multiaddr.Multiaddr{addr},
	})

	// Setup test protocol
	protocolID := protocol.ID("/test/1.0.0")
	received := make(chan types.Message, 1)

	handler := func(msg types.Message) error {
		received <- msg
		return nil
	}

	// Register protocol on both managers
	require.NoError(t, pm1.RegisterProtocol(protocolID, handler))
	require.NoError(t, pm2.RegisterProtocol(protocolID, handler))

	// Send test message
	testMsg := types.Message{
		Type:    types.MessageTypeTransaction,
		Payload: []byte("test message"),
		From:    host1.ID().String(),
	}

	ctx := context.Background()
	err := pm1.HandleMessage(ctx, testMsg)
	require.NoError(t, err)

	// Verify message reception
	select {
	case msg := <-received:
		assert.Equal(t, testMsg.Payload, msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("Message not received")
	}
}

func TestProtocolMetrics(t *testing.T) {
	pm, host := setupProtocolTest(t)
	defer host.Close()

	protocolID := protocol.ID("/test/1.0.0")
	handler := func(msg types.Message) error {
		return nil
	}

	require.NoError(t, pm.RegisterProtocol(protocolID, handler))
	protocol, _ := pm.GetProtocol(protocolID)

	// Send test messages
	testMsg := types.Message{
		Type:    types.MessageTypeTransaction,
		Payload: []byte("test message"),
	}

	for i := 0; i < 5; i++ {
		err := pm.HandleMessage(context.Background(), testMsg)
		require.NoError(t, err)
	}

	// Verify metrics
	metrics := protocol.Metrics.GetMetrics()
	assert.Equal(t, uint64(5), metrics["messages_received"])
	assert.Greater(t, metrics["bytes_received"], uint64(0))
}

func TestProtocolVersioning(t *testing.T) {
	pm, host := setupProtocolTest(t)
	defer host.Close()

	tests := []struct {
		name        string
		protocolID  protocol.ID
		shouldError bool
	}{
		{
			name:        "Valid version",
			protocolID:  protocol.ID("/test/1.0.0"),
			shouldError: false,
		},
		{
			name:        "Invalid version",
			protocolID:  protocol.ID("/test/invalid"),
			shouldError: true,
		},
	}

	handler := func(msg types.Message) error {
		return nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pm.RegisterProtocol(tt.protocolID, handler)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProtocolStreamHandling(t *testing.T) {
	pm1, host1 := setupProtocolTest(t)
	pm2, host2 := setupProtocolTest(t)
	defer host1.Close()
	defer host2.Close()

	// Connect hosts
	addr := host2.Addrs()[0]
	peer := host2.ID()
	host1.Connect(context.Background(), peer.AddrInfo{
		ID:    peer,
		Addrs: []multiaddr.Multiaddr{addr},
	})

	protocolID := protocol.ID("/test/1.0.0")
	messageCount := 100
	var wg sync.WaitGroup
	wg.Add(messageCount)

	// Register handler that counts messages
	handler := func(msg types.Message) error {
		wg.Done()
		return nil
	}

	require.NoError(t, pm1.RegisterProtocol(protocolID, handler))
	require.NoError(t, pm2.RegisterProtocol(protocolID, handler))

	// Send multiple messages concurrently
	for i := 0; i < messageCount; i++ {
		go func(i int) {
			msg := types.Message{
				Type:    types.MessageTypeTransaction,
				Payload: []byte(fmt.Sprintf("message-%d", i)),
			}
			err := pm1.HandleMessage(context.Background(), msg)
			require.NoError(t, err)
		}(i)
	}

	// Wait for all messages to be processed
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for messages")
	}
}

func TestProtocolErrorHandling(t *testing.T) {
	pm, host := setupProtocolTest(t)
	defer host.Close()

	protocolID := protocol.ID("/test/1.0.0")

	// Register handler that returns error
	handler := func(msg types.Message) error {
		return fmt.Errorf("test error")
	}

	require.NoError(t, pm.RegisterProtocol(protocolID, handler))

	// Send message and verify error handling
	msg := types.Message{
		Type:    types.MessageTypeTransaction,
		Payload: []byte("test message"),
	}

	err := pm.HandleMessage(context.Background(), msg)
	assert.Error(t, err)

	// Verify metrics were updated
	protocol, _ := pm.GetProtocol(protocolID)
	metrics := protocol.Metrics.GetMetrics()
	assert.Greater(t, metrics["errors"], uint64(0))
}

func TestProtocolBatchProcessing(t *testing.T) {
	pm, host := setupProtocolTest(t)
	defer host.Close()

	protocolID := protocol.ID("/test/1.0.0")
	processed := make(chan types.Message, 100)

	handler := func(msg types.Message) error {
		processed <- msg
		return nil
	}

	require.NoError(t, pm.RegisterProtocol(protocolID, handler))

	// Send batch of messages
	messages := make([]types.Message, 50)
	for i := range messages {
		messages[i] = types.Message{
			Type:    types.MessageTypeTransaction,
			Payload: []byte(fmt.Sprintf("message-%d", i)),
		}
	}

	// Process messages in batch
	ctx := context.Background()
	for _, msg := range messages {
		go func(m types.Message) {
			err := pm.HandleMessage(ctx, m)
			require.NoError(t, err)
		}(msg)
	}

	// Verify all messages were processed
	receivedCount := 0
	timeout := time.After(5 * time.Second)

	for receivedCount < len(messages) {
		select {
		case <-processed:
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout: only received %d/%d messages", receivedCount, len(messages))
		}
	}
}

func TestProtocolResourceManagement(t *testing.T) {
	pm, host := setupProtocolTest(t)
	defer host.Close()

	protocolID := protocol.ID("/test/1.0.0")

	// Add slow handler to test resource management
	handler := func(msg types.Message) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	require.NoError(t, pm.RegisterProtocol(protocolID, handler))

	// Send messages rapidly
	ctx := context.Background()
	start := time.Now()

	for i := 0; i < 10; i++ {
		msg := types.Message{
			Type:    types.MessageTypeTransaction,
			Payload: []byte(fmt.Sprintf("message-%d", i)),
		}
		err := pm.HandleMessage(ctx, msg)
		require.NoError(t, err)
	}

	duration := time.Since(start)

	// Verify messages were processed without overwhelming the system
	assert.True(t, duration < 2*time.Second, "Message processing took too long")
}
