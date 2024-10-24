package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"
	"go.uber.org/zap"
)

// Protocol versions
const (
	ProtocolVersion = "1.0.0"
	ProtocolBase    = "/aporia-zero"
)

// Protocol IDs
const (
	ProtocolTransaction = protocol.ID(ProtocolBase + "/tx/1.0.0")
	ProtocolBlock       = protocol.ID(ProtocolBase + "/block/1.0.0")
	ProtocolConsensus   = protocol.ID(ProtocolBase + "/consensus/1.0.0")
	ProtocolSync        = protocol.ID(ProtocolBase + "/sync/1.0.0")
	ProtocolDiscovery   = protocol.ID(ProtocolBase + "/discovery/1.0.0")
)

// Protocol manager handles protocol-specific logic
type ProtocolManager struct {
	host      host.Host
	protocols map[protocol.ID]*Protocol
	handlers  map[protocol.ID]MessageHandler
	logger    *zap.Logger
	mu        sync.RWMutex
}

// Protocol represents a specific protocol implementation
type Protocol struct {
	ID      protocol.ID
	Version string
	Handler MessageHandler
	Metrics *ProtocolMetrics
}

// ProtocolMetrics tracks protocol-specific metrics
type ProtocolMetrics struct {
	MessagesReceived uint64
	MessagesSent     uint64
	BytesReceived    uint64
	BytesSent        uint64
	Errors           uint64
	LastActivity     time.Time
	mu               sync.RWMutex
}

// NewProtocolManager creates a new protocol manager
func NewProtocolManager(h host.Host) *ProtocolManager {
	logger, _ := zap.NewProduction()

	return &ProtocolManager{
		host:      h,
		protocols: make(map[protocol.ID]*Protocol),
		handlers:  make(map[protocol.ID]MessageHandler),
		logger:    logger,
	}
}

// RegisterProtocol registers a new protocol
func (pm *ProtocolManager) RegisterProtocol(id protocol.ID, handler MessageHandler) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.protocols[id]; exists {
		return fmt.Errorf("protocol already registered: %s", id)
	}

	protocol := &Protocol{
		ID:      id,
		Version: ProtocolVersion,
		Handler: handler,
		Metrics: &ProtocolMetrics{
			LastActivity: time.Now(),
		},
	}

	pm.protocols[id] = protocol
	pm.handlers[id] = handler

	// Set stream handler for protocol
	pm.host.SetStreamHandler(id, pm.handleStream(protocol))

	pm.logger.Info("Registered protocol",
		zap.String("protocol", string(id)))

	return nil
}

// RemoveProtocol removes a protocol
func (pm *ProtocolManager) RemoveProtocol(id protocol.ID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if protocol, exists := pm.protocols[id]; exists {
		pm.host.RemoveStreamHandler(protocol.ID)
		delete(pm.protocols, id)
		delete(pm.handlers, id)

		pm.logger.Info("Removed protocol",
			zap.String("protocol", string(id)))
	}
}

// GetProtocol returns a protocol by ID
func (pm *ProtocolManager) GetProtocol(id protocol.ID) (*Protocol, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	protocol, exists := pm.protocols[id]
	return protocol, exists
}

// HandleMessage handles an incoming message
func (pm *ProtocolManager) HandleMessage(ctx context.Context, msg types.Message) error {
	pm.mu.RLock()
	handler, exists := pm.handlers[protocol.ID(msg.Type)]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler for protocol: %s", msg.Type)
	}

	if err := handler(msg); err != nil {
		return fmt.Errorf("handler error: %w", err)
	}

	// Update metrics
	if protocol, exists := pm.GetProtocol(protocol.ID(msg.Type)); exists {
		protocol.Metrics.updateMetrics(true, uint64(len(msg.Payload)), nil)
	}

	return nil
}

// Protocol metrics methods
func (m *ProtocolMetrics) updateMetrics(received bool, bytes uint64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if received {
		m.MessagesReceived++
		m.BytesReceived += bytes
	} else {
		m.MessagesSent++
		m.BytesSent += bytes
	}

	if err != nil {
		m.Errors++
	}

	m.LastActivity = time.Now()
}

func (m *ProtocolMetrics) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"messages_received": m.MessagesReceived,
		"messages_sent":     m.MessagesSent,
		"bytes_received":    m.BytesReceived,
		"bytes_sent":        m.BytesSent,
		"errors":            m.Errors,
		"last_activity":     m.LastActivity,
	}
}

// Stream handling
func (pm *ProtocolManager) handleStream(p *Protocol) func(network.Stream) {
	return func(stream network.Stream) {
		defer stream.Close()

		for {
			// Read message
			data, err := readMessage(stream)
			if err != nil {
				pm.logger.Error("Failed to read message",
					zap.String("protocol", string(p.ID)),
					zap.Error(err))
				return
			}

			// Parse message
			var msg types.Message
			if err := json.Unmarshal(data, &msg); err != nil {
				pm.logger.Error("Failed to parse message",
					zap.String("protocol", string(p.ID)),
					zap.Error(err))
				continue
			}

			// Handle message
			ctx := context.Background()
			if err := pm.HandleMessage(ctx, msg); err != nil {
				pm.logger.Error("Failed to handle message",
					zap.String("protocol", string(p.ID)),
					zap.Error(err))
				p.Metrics.updateMetrics(true, uint64(len(data)), err)
				continue
			}

			p.Metrics.updateMetrics(true, uint64(len(data)), nil)
		}
	}
}
