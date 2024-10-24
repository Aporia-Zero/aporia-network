package p2p

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"go.uber.org/zap"
)

// Peer represents a connected peer
type Peer struct {
	// Peer identity
	ID       peer.ID
	AddrInfo peer.AddrInfo

	// Connection
	host       host.Host
	streams    map[protocol.ID]network.Stream
	streamLock sync.RWMutex

	// State
	connected bool
	lastSeen  time.Time

	// Message handler
	msgHandler chan<- types.Message

	// Logger
	logger *zap.Logger
}

// NewPeer creates a new peer
func NewPeer(peerInfo peer.AddrInfo, h host.Host) *Peer {
	logger, _ := zap.NewProduction()

	return &Peer{
		ID:        peerInfo.ID,
		AddrInfo:  peerInfo,
		host:      h,
		streams:   make(map[protocol.ID]network.Stream),
		connected: false,
		lastSeen:  time.Now(),
		logger:    logger,
	}
}

// Connect establishes connection with the peer
func (p *Peer) Connect(ctx context.Context) error {
	if p.connected {
		return nil
	}

	if err := p.host.Connect(ctx, p.AddrInfo); err != nil {
		return types.NetworkError{
			Code:    types.ErrCodePeerConnection,
			Message: err.Error(),
		}
	}

	p.connected = true
	p.lastSeen = time.Now()

	// Start stream handlers
	go p.handleStreams()

	p.logger.Info("Connected to peer",
		zap.String("peer", p.ID.String()))

	return nil
}

// Disconnect closes connection with the peer
func (p *Peer) Disconnect() {
	p.streamLock.Lock()
	defer p.streamLock.Unlock()

	// Close all streams
	for _, stream := range p.streams {
		stream.Close()
	}
	p.streams = make(map[protocol.ID]network.Stream)

	p.connected = false

	p.logger.Info("Disconnected from peer",
		zap.String("peer", p.ID.String()))
}

// SendMessage sends a message to the peer
func (p *Peer) SendMessage(msg types.Message) error {
	if !p.connected {
		return types.NetworkError{
			Code:    types.ErrCodePeerConnection,
			Message: "peer not connected",
		}
	}

	// Get or create stream for message type
	stream, err := p.getStream(protocol.ID(msg.Type))
	if err != nil {
		return err
	}

	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return types.NetworkError{
			Code:    types.ErrCodeMessageFormat,
			Message: err.Error(),
		}
	}

	// Write message length and data
	if err := writeMessage(stream, data); err != nil {
		p.closeStream(protocol.ID(msg.Type))
		return err
	}

	p.lastSeen = time.Now()
	return nil
}

// IsConnected returns connection status
func (p *Peer) IsConnected() bool {
	return p.connected
}

// LastSeen returns the last seen time
func (p *Peer) LastSeen() time.Time {
	return p.lastSeen
}

// SetMessageHandler sets the message handler
func (p *Peer) SetMessageHandler(handler chan<- types.Message) {
	p.msgHandler = handler
}

// Internal methods

func (p *Peer) getStream(protocolID protocol.ID) (network.Stream, error) {
	p.streamLock.Lock()
	defer p.streamLock.Unlock()

	// Check existing stream
	if stream, exists := p.streams[protocolID]; exists {
		return stream, nil
	}

	// Create new stream
	stream, err := p.host.NewStream(context.Background(), p.ID, protocolID)
	if err != nil {
		return nil, types.NetworkError{
			Code:    types.ErrCodePeerConnection,
			Message: err.Error(),
		}
	}

	p.streams[protocolID] = stream
	return stream, nil
}

func (p *Peer) closeStream(protocolID protocol.ID) {
	p.streamLock.Lock()
	defer p.streamLock.Unlock()

	if stream, exists := p.streams[protocolID]; exists {
		stream.Close()
		delete(p.streams, protocolID)
	}
}

func (p *Peer) handleStreams() {
	for {
		if !p.connected {
			return
		}

		// Accept new stream
		stream, err := p.host.NewStream(context.Background(), p.ID, "")
		if err != nil {
			p.logger.Error("Failed to accept stream",
				zap.String("peer", p.ID.String()),
				zap.Error(err))
			continue
		}

		// Handle stream in goroutine
		go p.handleStream(stream)
	}
}

func (p *Peer) handleStream(stream network.Stream) {
	defer stream.Close()

	for {
		// Read message
		data, err := readMessage(stream)
		if err != nil {
			p.logger.Error("Failed to read message",
				zap.String("peer", p.ID.String()),
				zap.Error(err))
			return
		}

		// Deserialize message
		var msg types.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			p.logger.Error("Failed to deserialize message",
				zap.String("peer", p.ID.String()),
				zap.Error(err))
			continue
		}

		// Send to message handler
		if p.msgHandler != nil {
			p.msgHandler <- msg
		}

		p.lastSeen = time.Now()
	}
}

// Helper functions for reading/writing messages

func writeMessage(stream network.Stream, data []byte) error {
	// Write message length
	length := uint32(len(data))
	if err := binary.Write(stream, binary.BigEndian, length); err != nil {
		return types.NetworkError{
			Code:    types.ErrCodeMessageFormat,
			Message: err.Error(),
		}
	}

	// Write message data
	if _, err := stream.Write(data); err != nil {
		return types.NetworkError{
			Code:    types.ErrCodeMessageFormat,
			Message: err.Error(),
		}
	}

	return nil
}

func readMessage(stream network.Stream) ([]byte, error) {
	// Read message length
	var length uint32
	if err := binary.Read(stream, binary.BigEndian, &length); err != nil {
		return nil, types.NetworkError{
			Code:    types.ErrCodeMessageFormat,
			Message: err.Error(),
		}
	}

	// Read message data
	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, types.NetworkError{
			Code:    types.ErrCodeMessageFormat,
			Message: err.Error(),
		}
	}

	return data, nil
}
