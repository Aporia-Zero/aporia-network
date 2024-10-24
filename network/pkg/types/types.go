package types

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"time"
)

// NetworkConfig represents network configuration
type NetworkConfig struct {
	// Node identity
	NodeID     string
	PrivateKey *ecdsa.PrivateKey

	// Network parameters
	ListenAddresses []string
	BootstrapPeers  []string
	MaxPeers        int

	// Protocol parameters
	ProtocolID string
	Version    string

	// API configuration
	APIPort int
	APIHost string

	// Transaction pool configuration
	MaxPoolSize int
	MaxTxSize   int64
}

// Peer represents a network peer
type Peer struct {
	ID           string
	Address      string
	LastSeen     time.Time
	Version      string
	Capabilities []string
}

// Transaction represents a network transaction
type Transaction struct {
	Hash             [32]byte
	From             [20]byte
	To               [20]byte
	Value            uint64
	Nonce            uint64
	Data             []byte
	Signature        []byte
	ComputationProof []byte
}

// Block represents a network block
type Block struct {
	Hash         [32]byte
	PrevHash     [32]byte
	Height       uint64
	Timestamp    time.Time
	Transactions []Transaction
	StateRoot    [32]byte
	ProposerID   string
}

// Message represents a network message
type Message struct {
	Type      MessageType
	Payload   []byte
	From      string
	Timestamp time.Time
}

// MessageType represents different types of network messages
type MessageType uint8

const (
	MessageTypeTransaction MessageType = iota
	MessageTypeBlock
	MessageTypePeerDiscovery
	MessageTypeStateSync
	MessageTypeConsensus
)

// Error types
type NetworkError struct {
	Code    int
	Message string
}

func (e NetworkError) Error() string {
	return fmt.Sprintf("network error (code=%d): %s", e.Code, e.Message)
}

// Network error codes
const (
	ErrCodePeerConnection = iota + 1000
	ErrCodeMessageFormat
	ErrCodeProtocolVersion
	ErrCodeTxPool
	ErrCodeDiscovery
)

// Transaction status
type TxStatus int

const (
	TxStatusPending TxStatus = iota
	TxStatusConfirmed
	TxStatusFailed
)

// Transaction validation result
type TxValidationResult struct {
	Valid           bool
	Error           error
	ComputationCost uint64
}

// Block validation result
type BlockValidationResult struct {
	Valid     bool
	Error     error
	StateRoot [32]byte
}

// Helper methods for Transaction
func (tx *Transaction) Hash() [32]byte {
	// Implement proper hashing logic
	return [32]byte{}
}

func (tx *Transaction) Verify() bool {
	// Implement verification logic
	return true
}

func (tx *Transaction) String() string {
	return fmt.Sprintf("TX(hash=%s, from=%s, to=%s, value=%d)",
		hex.EncodeToString(tx.Hash[:]),
		hex.EncodeToString(tx.From[:]),
		hex.EncodeToString(tx.To[:]),
		tx.Value,
	)
}

// Helper methods for Block
func (b *Block) CalculateHash() [32]byte {
	// Implement proper hashing logic
	return [32]byte{}
}

func (b *Block) Verify() bool {
	// Implement verification logic
	return true
}

func (b *Block) String() string {
	return fmt.Sprintf("Block(hash=%s, height=%d, txs=%d)",
		hex.EncodeToString(b.Hash[:]),
		b.Height,
		len(b.Transactions),
	)
}

// Helper methods for Peer
func (p *Peer) IsActive() bool {
	return time.Since(p.LastSeen) < 5*time.Minute
}

func (p *Peer) HasCapability(cap string) bool {
	for _, c := range p.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

func (p *Peer) String() string {
	return fmt.Sprintf("Peer(id=%s, addr=%s)", p.ID, p.Address)
}
