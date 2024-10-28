package mempool

import (
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create test transaction
func createTestTransaction(nonce uint64) *types.Transaction {
	tx := &types.Transaction{
		Nonce: nonce,
		Value: 1000,
	}

	// Generate random hash
	rand.Read(tx.Hash[:])
	rand.Read(tx.From[:])
	rand.Read(tx.To[:])

	// Add mock computation proof
	tx.ComputationProof = []byte{1, 2, 3, 4}

	return tx
}

func TestNewTransactionPool(t *testing.T) {
	config := DefaultPoolConfig()
	pool, err := NewTransactionPool(config)

	require.NoError(t, err)
	assert.NotNil(t, pool)
	assert.Equal(t, 0, pool.Size())
}

func TestAddTransaction(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())
	tx := createTestTransaction(0)

	// Add transaction
	err := pool.AddTransaction(tx)
	require.NoError(t, err)
	assert.Equal(t, 1, pool.Size())

	// Try to add same transaction again
	err = pool.AddTransaction(tx)
	assert.Equal(t, ErrTxAlreadyExists, err)
}

func TestGetTransaction(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())
	tx := createTestTransaction(0)

	// Add transaction
	err := pool.AddTransaction(tx)
	require.NoError(t, err)

	// Get transaction
	retrievedTx, err := pool.GetTransaction(tx.Hash.String())
	require.NoError(t, err)
	assert.Equal(t, tx.Hash, retrievedTx.Hash)

	// Try to get non-existent transaction
	_, err = pool.GetTransaction("non-existent")
	assert.Equal(t, ErrTxNotFound, err)
}

func TestRemoveTransaction(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())
	tx := createTestTransaction(0)

	// Add and remove transaction
	pool.AddTransaction(tx)
	pool.RemoveTransaction(tx.Hash.String())
	assert.Equal(t, 0, pool.Size())

	// Verify transaction is removed
	assert.False(t, pool.HasTransaction(tx.Hash.String()))
}

func TestPoolSize(t *testing.T) {
	config := DefaultPoolConfig()
	config.MaxSize = 2
	pool, _ := NewTransactionPool(config)

	// Add transactions up to max size
	tx1 := createTestTransaction(0)
	tx2 := createTestTransaction(1)
	tx3 := createTestTransaction(2)

	require.NoError(t, pool.AddTransaction(tx1))
	require.NoError(t, pool.AddTransaction(tx2))
	assert.Equal(t, ErrPoolFull, pool.AddTransaction(tx3))
}

func TestTransactionExpiration(t *testing.T) {
	config := DefaultPoolConfig()
	config.ExpirationDuration = 100 * time.Millisecond
	config.CleanupInterval = 50 * time.Millisecond
	pool, _ := NewTransactionPool(config)

	// Add transaction
	tx := createTestTransaction(0)
	pool.AddTransaction(tx)

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, pool.Size())
}

func TestConcurrentAccess(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())
	var wg sync.WaitGroup
	txCount := 100

	// Add transactions concurrently
	wg.Add(txCount)
	for i := 0; i < txCount; i++ {
		go func(nonce uint64) {
			defer wg.Done()
			tx := createTestTransaction(nonce)
			pool.AddTransaction(tx)
		}(uint64(i))
	}
	wg.Wait()

	assert.Equal(t, txCount, pool.Size())
}

func TestTransactionValidation(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())
	validator := NewTransactionValidator(DefaultValidationConfig())

	tests := []struct {
		name    string
		tx      *types.Transaction
		wantErr error
	}{
		{
			name:    "Valid transaction",
			tx:      createTestTransaction(0),
			wantErr: nil,
		},
		{
			name: "Missing computation proof",
			tx: &types.Transaction{
				Nonce:            0,
				Value:            1000,
				ComputationProof: nil,
			},
			wantErr: ErrInvalidProof,
		},
		{
			name:    "Invalid nonce",
			tx:      createTestTransaction(10), // Non-sequential nonce
			wantErr: ErrInvalidNonce,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTransaction(tt.tx)
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetPendingTransactions(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())

	// Add multiple transactions
	txs := make([]*types.Transaction, 3)
	for i := range txs {
		txs[i] = createTestTransaction(uint64(i))
		pool.AddTransaction(txs[i])
	}

	// Get pending transactions
	pending := pool.GetPendingTransactions()
	assert.Equal(t, len(txs), len(pending))

	// Verify order (by timestamp)
	for i := 1; i < len(pending); i++ {
		assert.True(t, pending[i-1].Timestamp <= pending[i].Timestamp)
	}
}

func TestTransactionsByAddress(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())
	addr := [20]byte{1, 2, 3}

	// Add transactions for specific address
	txCount := 5
	for i := 0; i < txCount; i++ {
		tx := createTestTransaction(uint64(i))
		tx.From = addr
		pool.AddTransaction(tx)
	}

	// Get transactions by address
	txs := pool.GetTransactionsByAddress(string(addr[:]))
	assert.Equal(t, txCount, len(txs))

	// Verify nonce sequence
	for i, tx := range txs {
		assert.Equal(t, uint64(i), tx.Nonce)
	}
}

func TestPoolStatus(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())

	// Initial status
	status := pool.GetStatus()
	assert.Equal(t, 0, status.CurrentSize)
	assert.Equal(t, 0, status.PendingCount)

	// Add transaction
	tx := createTestTransaction(0)
	pool.AddTransaction(tx)

	// Updated status
	status = pool.GetStatus()
	assert.Equal(t, 1, status.CurrentSize)
	assert.Equal(t, 1, status.PendingCount)
}

func TestClear(t *testing.T) {
	pool, _ := NewTransactionPool(DefaultPoolConfig())

	// Add transactions
	for i := 0; i < 5; i++ {
		tx := createTestTransaction(uint64(i))
		pool.AddTransaction(tx)
	}

	// Clear pool
	pool.Clear()
	assert.Equal(t, 0, pool.Size())
	assert.Empty(t, pool.GetPendingTransactions())
}
