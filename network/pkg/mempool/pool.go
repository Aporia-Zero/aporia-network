package mempool

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"go.uber.org/zap"
)

// TransactionPool represents the mempool for pending transactions
type TransactionPool struct {
	// Configuration
	config *PoolConfig

	// Transactions
	txs    map[string]*PoolTransaction
	txLock sync.RWMutex

	// Indexes
	byNonce     map[string][]string // address -> ordered tx hashes
	byTimestamp []string            // ordered tx hashes

	// Status tracking
	status *PoolStatus

	// Channels
	newTxChan chan *types.Transaction

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Logger
	logger *zap.Logger
}

// PoolConfig represents mempool configuration
type PoolConfig struct {
	MaxSize              int
	MaxTransactionSize   int64
	ExpirationDuration   time.Duration
	CleanupInterval      time.Duration
	MaxPendingPerAccount int
}

// PoolTransaction represents a transaction in the pool
type PoolTransaction struct {
	Transaction *types.Transaction
	AddedAt     time.Time
	Status      types.TxStatus
}

// PoolStatus represents mempool status
type PoolStatus struct {
	CurrentSize   int
	PendingCount  int
	RejectedCount int
	ExpiredCount  int
	mu            sync.RWMutex
}

// NewTransactionPool creates a new transaction pool
func NewTransactionPool(config *PoolConfig) (*TransactionPool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	logger, _ := zap.NewProduction()

	pool := &TransactionPool{
		config:    config,
		txs:       make(map[string]*PoolTransaction),
		byNonce:   make(map[string][]string),
		status:    &PoolStatus{},
		newTxChan: make(chan *types.Transaction, 1000),
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
	}

	// Start background processes
	go pool.cleanupRoutine()
	go pool.processingRoutine()

	return pool, nil
}

// AddTransaction adds a transaction to the pool
func (p *TransactionPool) AddTransaction(tx *types.Transaction) error {
	p.txLock.Lock()
	defer p.txLock.Unlock()

	// Check pool size
	if len(p.txs) >= p.config.MaxSize {
		return ErrPoolFull
	}

	// Check transaction size
	if tx.Size() > p.config.MaxTransactionSize {
		return ErrTxTooLarge
	}

	// Check if transaction already exists
	hash := tx.Hash.String()
	if _, exists := p.txs[hash]; exists {
		return ErrTxAlreadyExists
	}

	// Create pool transaction
	poolTx := &PoolTransaction{
		Transaction: tx,
		AddedAt:     time.Now(),
		Status:      types.TxStatusPending,
	}

	// Add to main storage
	p.txs[hash] = poolTx

	// Update indexes
	p.updateIndexes(tx, hash)

	// Update status
	p.updateStatus(1, 0, 0, 0)

	// Notify new transaction
	select {
	case p.newTxChan <- tx:
	default:
		// Channel full, skip notification
	}

	p.logger.Info("Transaction added to pool",
		zap.String("hash", hash),
		zap.Int("pool_size", len(p.txs)))

	return nil
}

// GetTransaction retrieves a transaction from the pool
func (p *TransactionPool) GetTransaction(hash string) (*types.Transaction, error) {
	p.txLock.RLock()
	defer p.txLock.RUnlock()

	if poolTx, exists := p.txs[hash]; exists {
		return poolTx.Transaction, nil
	}

	return nil, ErrTxNotFound
}

// GetPendingTransactions returns all pending transactions
func (p *TransactionPool) GetPendingTransactions() []*types.Transaction {
	p.txLock.RLock()
	defer p.txLock.RUnlock()

	result := make([]*types.Transaction, 0, len(p.txs))
	for _, poolTx := range p.txs {
		if poolTx.Status == types.TxStatusPending {
			result = append(result, poolTx.Transaction)
		}
	}

	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})

	return result
}

// RemoveTransaction removes a transaction from the pool
func (p *TransactionPool) RemoveTransaction(hash string) {
	p.txLock.Lock()
	defer p.txLock.Unlock()

	if poolTx, exists := p.txs[hash]; exists {
		// Remove from indexes
		p.removeFromIndexes(poolTx.Transaction, hash)

		// Remove from main storage
		delete(p.txs, hash)

		// Update status
		p.updateStatus(-1, 0, 0, 0)

		p.logger.Info("Transaction removed from pool",
			zap.String("hash", hash),
			zap.Int("pool_size", len(p.txs)))
	}
}

// Clear removes all transactions from the pool
func (p *TransactionPool) Clear() {
	p.txLock.Lock()
	defer p.txLock.Unlock()

	p.txs = make(map[string]*PoolTransaction)
	p.byNonce = make(map[string][]string)
	p.byTimestamp = nil

	p.status = &PoolStatus{}

	p.logger.Info("Transaction pool cleared")
}

// Stop stops the transaction pool
func (p *TransactionPool) Stop() {
	p.cancel()
}

// Internal methods

func (p *TransactionPool) updateIndexes(tx *types.Transaction, hash string) {
	// Update nonce index
	from := string(tx.From[:])
	if txs, exists := p.byNonce[from]; exists {
		p.byNonce[from] = append(txs, hash)
		// Sort by nonce
		sort.Slice(p.byNonce[from], func(i, j int) bool {
			return p.txs[p.byNonce[from][i]].Transaction.Nonce <
				p.txs[p.byNonce[from][j]].Transaction.Nonce
		})
	} else {
		p.byNonce[from] = []string{hash}
	}

	// Update timestamp index
	p.byTimestamp = append(p.byTimestamp, hash)
	// Sort by timestamp
	sort.Slice(p.byTimestamp, func(i, j int) bool {
		return p.txs[p.byTimestamp[i]].AddedAt.Before(
			p.txs[p.byTimestamp[j]].AddedAt)
	})
}

func (p *TransactionPool) removeFromIndexes(tx *types.Transaction, hash string) {
	// Remove from nonce index
	from := string(tx.From[:])
	if txs, exists := p.byNonce[from]; exists {
		for i, h := range txs {
			if h == hash {
				p.byNonce[from] = append(txs[:i], txs[i+1:]...)
				break
			}
		}
		if len(p.byNonce[from]) == 0 {
			delete(p.byNonce, from)
		}
	}

	// Remove from timestamp index
	for i, h := range p.byTimestamp {
		if h == hash {
			p.byTimestamp = append(p.byTimestamp[:i], p.byTimestamp[i+1:]...)
			break
		}
	}
}

func (p *TransactionPool) updateStatus(
	pending, confirmed, rejected, expired int) {
	p.status.mu.Lock()
	defer p.status.mu.Unlock()

	p.status.CurrentSize += pending
	p.status.PendingCount += pending
	p.status.RejectedCount += rejected
	p.status.ExpiredCount += expired
}

// Background routines

func (p *TransactionPool) cleanupRoutine() {
	ticker := time.NewTicker(p.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.cleanup()
		}
	}
}

func (p *TransactionPool) cleanup() {
	p.txLock.Lock()
	defer p.txLock.Unlock()

	now := time.Now()
	expired := 0

	// Check all transactions
	for hash, poolTx := range p.txs {
		// Remove expired transactions
		if now.Sub(poolTx.AddedAt) > p.config.ExpirationDuration {
			p.removeFromIndexes(poolTx.Transaction, hash)
			delete(p.txs, hash)
			expired++
		}
	}

	if expired > 0 {
		p.updateStatus(0, 0, 0, expired)
		p.logger.Info("Cleaned up expired transactions",
			zap.Int("expired", expired),
			zap.Int("remaining", len(p.txs)))
	}
}

func (p *TransactionPool) processingRoutine() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case tx := <-p.newTxChan:
			p.processPendingTransaction(tx)
		}
	}
}

func (p *TransactionPool) processPendingTransaction(tx *types.Transaction) {
	// This is where you would implement transaction pre-processing
	// For example:
	// - Verify computational proof
	// - Check nonce sequence
	// - Validate signatures
	// - Check account balances
	// etc.
}

// GetStatus returns the current pool status
func (p *TransactionPool) GetStatus() PoolStatus {
	p.status.mu.RLock()
	defer p.status.mu.RUnlock()
	return *p.status
}

// GetTransactionsByAddress returns all transactions for an address
func (p *TransactionPool) GetTransactionsByAddress(address string) []*types.Transaction {
	p.txLock.RLock()
	defer p.txLock.RUnlock()

	var result []*types.Transaction
	if hashes, exists := p.byNonce[address]; exists {
		result = make([]*types.Transaction, 0, len(hashes))
		for _, hash := range hashes {
			if poolTx, exists := p.txs[hash]; exists {
				result = append(result, poolTx.Transaction)
			}
		}
	}

	return result
}

// HasTransaction checks if a transaction exists in the pool
func (p *TransactionPool) HasTransaction(hash string) bool {
	p.txLock.RLock()
	defer p.txLock.RUnlock()
	_, exists := p.txs[hash]
	return exists
}

// GetTransactionStatus returns the status of a transaction
func (p *TransactionPool) GetTransactionStatus(hash string) (types.TxStatus, error) {
	p.txLock.RLock()
	defer p.txLock.RUnlock()

	if poolTx, exists := p.txs[hash]; exists {
		return poolTx.Status, nil
	}
	return 0, ErrTxNotFound
}

// Size returns the current number of transactions in the pool
func (p *TransactionPool) Size() int {
	p.txLock.RLock()
	defer p.txLock.RUnlock()
	return len(p.txs)
}

// Default configuration
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxSize:              10000,
		MaxTransactionSize:   1 << 20, // 1MB
		ExpirationDuration:   24 * time.Hour,
		CleanupInterval:      15 * time.Minute,
		MaxPendingPerAccount: 100,
	}
}
