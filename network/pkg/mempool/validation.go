package mempool

import (
	"errors"
	"sync"

	"github.com/aporia-zero/network/pkg/types"
)

var (
	ErrPoolFull        = errors.New("transaction pool is full")
	ErrTxTooLarge      = errors.New("transaction size exceeds maximum")
	ErrTxAlreadyExists = errors.New("transaction already exists in pool")
	ErrTxNotFound      = errors.New("transaction not found in pool")
	ErrInvalidNonce    = errors.New("invalid transaction nonce")
	ErrInvalidProof    = errors.New("invalid computation proof")
)

// TransactionValidator validates transactions before adding to pool
type TransactionValidator struct {
	config   *ValidationConfig
	nonceMgr *NonceManager
}

// ValidationConfig represents validation configuration
type ValidationConfig struct {
	MinComputationCost uint64
	MaxComputationCost uint64
	NonceWindow        uint64
}

// NonceManager tracks account nonces
type NonceManager struct {
	nonces map[string]uint64 // address -> highest nonce
	mu     sync.RWMutex
}

// NewTransactionValidator creates a new transaction validator
func NewTransactionValidator(config *ValidationConfig) *TransactionValidator {
	return &TransactionValidator{
		config: config,
		nonceMgr: &NonceManager{
			nonces: make(map[string]uint64),
		},
	}
}

// ValidateTransaction performs comprehensive transaction validation
func (v *TransactionValidator) ValidateTransaction(tx *types.Transaction) error {
	// Basic validation
	if err := v.validateBasics(tx); err != nil {
		return err
	}

	// Validate nonce
	if err := v.validateNonce(tx); err != nil {
		return err
	}

	// Validate computation proof
	if err := v.validateComputationProof(tx); err != nil {
		return err
	}

	// Validate signature
	if err := v.validateSignature(tx); err != nil {
		return err
	}

	return nil
}

// Basic transaction validation
func (v *TransactionValidator) validateBasics(tx *types.Transaction) error {
	// Check transaction size
	if tx.Size() == 0 {
		return errors.New("empty transaction")
	}

	// Check addresses
	if tx.From == [20]byte{} {
		return errors.New("missing sender address")
	}

	// Check value
	if tx.Value == 0 {
		return errors.New("zero transaction value")
	}

	return nil
}

// Nonce validation
func (v *TransactionValidator) validateNonce(tx *types.Transaction) error {
	v.nonceMgr.mu.RLock()
	currentNonce, exists := v.nonceMgr.nonces[string(tx.From[:])]
	v.nonceMgr.mu.RUnlock()

	if !exists {
		// First transaction from this account
		if tx.Nonce != 0 {
			return ErrInvalidNonce
		}
	} else {
		// Check nonce sequence
		if tx.Nonce != currentNonce+1 {
			return ErrInvalidNonce
		}
	}

	return nil
}

// Computation proof validation
func (v *TransactionValidator) validateComputationProof(tx *types.Transaction) error {
	// Verify proof exists
	if len(tx.ComputationProof) == 0 {
		return ErrInvalidProof
	}

	// Verify computation cost
	cost := calculateComputationCost(tx.ComputationProof)
	if cost < v.config.MinComputationCost {
		return errors.New("insufficient computation")
	}
	if cost > v.config.MaxComputationCost {
		return errors.New("excessive computation")
	}

	// Verify proof validity
	if !verifyComputationProof(tx.ComputationProof) {
		return ErrInvalidProof
	}

	return nil
}

// Signature validation
func (v *TransactionValidator) validateSignature(tx *types.Transaction) error {
	if len(tx.Signature) == 0 {
		return errors.New("missing signature")
	}

	if !tx.VerifySignature() {
		return errors.New("invalid signature")
	}

	return nil
}

// Update nonce after transaction is confirmed
func (v *TransactionValidator) UpdateNonce(address string, nonce uint64) {
	v.nonceMgr.mu.Lock()
	defer v.nonceMgr.mu.Unlock()

	v.nonceMgr.nonces[address] = nonce
}

// Helper functions

func calculateComputationCost(proof []byte) uint64 {
	// Implement computation cost calculation
	// This would validate and measure the proof of computation
	return 0
}

func verifyComputationProof(proof []byte) bool {
	// Implement proof verification
	// This would verify the validity of the computation proof
	return true
}

// Default configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MinComputationCost: 1000,
		MaxComputationCost: 1000000,
		NonceWindow:        100,
	}
}
