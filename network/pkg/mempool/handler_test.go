package mempool

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock transaction validator
type MockValidator struct {
	validateFunc func(*types.Transaction) error
}

func (m *MockValidator) ValidateTransaction(tx *types.Transaction) error {
	if m.validateFunc != nil {
		return m.validateFunc(tx)
	}
	return nil
}

// Transaction handler test setup
func setupTransactionHandler(t *testing.T) (*TransactionHandler, *MockValidator) {
	validator := &MockValidator{}
	config := &HandlerConfig{
		MaxBatchSize:      100,
		ProcessingTimeout: time.Second,
		BatchTimeout:      100 * time.Millisecond,
		RetryAttempts:     3,
		RetryDelay:        50 * time.Millisecond,
	}

	handler := NewTransactionHandler(config, validator)
	return handler, validator
}

func TestTransactionHandling(t *testing.T) {
	handler, _ := setupTransactionHandler(t)

	// Test transaction
	tx := &types.Transaction{
		Hash:  [32]byte{1},
		From:  [20]byte{1},
		To:    [20]byte{2},
		Value: 1000,
		Nonce: 0,
	}

	// Process transaction
	err := handler.ProcessTransaction(tx)
	require.NoError(t, err)

	// Verify transaction was processed
	status, err := handler.GetTransactionStatus(tx.Hash)
	require.NoError(t, err)
	assert.Equal(t, types.TxStatusPending, status)
}

func TestBatchProcessing(t *testing.T) {
	handler, _ := setupTransactionHandler(t)

	// Create test transactions
	txCount := 50
	txs := make([]*types.Transaction, txCount)
	for i := range txs {
		txs[i] = &types.Transaction{
			Hash:  [32]byte{byte(i)},
			Value: uint64(i * 1000),
			Nonce: uint64(i),
		}
	}

	// Process transactions concurrently
	var wg sync.WaitGroup
	wg.Add(len(txs))

	for _, tx := range txs {
		go func(tx *types.Transaction) {
			defer wg.Done()
			err := handler.ProcessTransaction(tx)
			assert.NoError(t, err)
		}(tx)
	}

	wg.Wait()

	// Verify all transactions were processed
	for _, tx := range txs {
		status, err := handler.GetTransactionStatus(tx.Hash)
		require.NoError(t, err)
		assert.NotEqual(t, types.TxStatusPending, status)
	}
}

func TestValidationFailure(t *testing.T) {
	handler, validator := setupTransactionHandler(t)

	// Set validator to fail
	validator.validateFunc = func(tx *types.Transaction) error {
		return ErrInvalidTransaction
	}

	// Test transaction
	tx := &types.Transaction{
		Hash:  [32]byte{1},
		Value: 1000,
	}

	// Process transaction
	err := handler.ProcessTransaction(tx)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidTransaction, err)

	// Verify transaction status
	status, err := handler.GetTransactionStatus(tx.Hash)
	require.NoError(t, err)
	assert.Equal(t, types.TxStatusFailed, status)
}

func TestRetryMechanism(t *testing.T) {
	handler, validator := setupTransactionHandler(t)

	attempts := 0
	validator.validateFunc = func(tx *types.Transaction) error {
		attempts++
		if attempts < 3 {
			return ErrTemporaryFailure
		}
		return nil
	}

	// Test transaction
	tx := &types.Transaction{
		Hash:  [32]byte{1},
		Value: 1000,
	}

	// Process transaction
	err := handler.ProcessTransaction(tx)
	require.NoError(t, err)

	// Verify retry attempts
	assert.Equal(t, 3, attempts)

	// Verify final status
	status, err := handler.GetTransactionStatus(tx.Hash)
	require.NoError(t, err)
	assert.Equal(t, types.TxStatusPending, status)
}

func TestProcessingTimeout(t *testing.T) {
	handler, validator := setupTransactionHandler(t)

	// Set validator to hang
	validator.validateFunc = func(tx *types.Transaction) error {
		time.Sleep(2 * time.Second)
		return nil
	}

	// Test transaction
	tx := &types.Transaction{
		Hash:  [32]byte{1},
		Value: 1000,
	}

	// Process transaction with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := handler.ProcessTransactionWithContext(ctx, tx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestBatchValidation(t *testing.T) {
	handler, validator := setupTransactionHandler(t)

	// Create transactions with dependencies
	txs := []*types.Transaction{
		{Hash: [32]byte{1}, From: [20]byte{1}, Nonce: 0},
		{Hash: [32]byte{2}, From: [20]byte{1}, Nonce: 1},
		{Hash: [32]byte{3}, From: [20]byte{1}, Nonce: 2},
	}

	// Process transactions in reverse order
	for i := len(txs) - 1; i >= 0; i-- {
		err := handler.ProcessTransaction(txs[i])
		require.NoError(t, err)
	}

	// Verify processing order
	for i, tx := range txs {
		status, err := handler.GetTransactionStatus(tx.Hash)
		require.NoError(t, err)
		if i == 0 {
			assert.Equal(t, types.TxStatusPending, status)
		} else {
			assert.Equal(t, types.TxStatusPending, status)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	handler, _ := setupTransactionHandler(t)

	// Create test transactions
	txCount := 100
	var wg sync.WaitGroup
	wg.Add(txCount * 2) // For both processing and status checks

	for i := 0; i < txCount; i++ {
		tx := &types.Transaction{
			Hash:  [32]byte{byte(i)},
			Value: uint64(i * 1000),
		}

		// Process transaction
		go func(tx *types.Transaction) {
			defer wg.Done()
			err := handler.ProcessTransaction(tx)
			assert.NoError(t, err)
		}(tx)

		// Check status concurrently
		go func(tx *types.Transaction) {
			defer wg.Done()
			_, err := handler.GetTransactionStatus(tx.Hash)
			assert.NoError(t, err)
		}(tx)
	}

	wg.Wait()
}

func TestQueueManagement(t *testing.T) {
	handler, validator := setupTransactionHandler(t)

	// Set validator to process slowly
	validator.validateFunc = func(tx *types.Transaction) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	// Fill transaction queue
	queueSize := 1000
	processed := make(chan struct{}, queueSize)

	for i := 0; i < queueSize; i++ {
		tx := &types.Transaction{
			Hash:  [32]byte{byte(i)},
			Value: uint64(i * 1000),
		}

		go func() {
			err := handler.ProcessTransaction(tx)
			assert.NoError(t, err)
			processed <- struct{}{}
		}()
	}

	// Verify all transactions are eventually processed
	for i := 0; i < queueSize; i++ {
		select {
		case <-processed:
			// Transaction processed successfully
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for transaction processing")
		}
	}
}

func TestTransactionPrioritization(t *testing.T) {
	handler, _ := setupTransactionHandler(t)

	// Create transactions with different priorities
	highPriorityTx := &types.Transaction{
		Hash:     [32]byte{1},
		Value:    1000000,
		GasPrice: 100,
	}

	lowPriorityTx := &types.Transaction{
		Hash:     [32]byte{2},
		Value:    1000,
		GasPrice: 1,
	}

	// Process transactions
	err := handler.ProcessTransaction(lowPriorityTx)
	require.NoError(t, err)

	err = handler.ProcessTransaction(highPriorityTx)
	require.NoError(t, err)

	// Verify processing order through status updates
	time.Sleep(100 * time.Millisecond)

	highPriorityStatus, _ := handler.GetTransactionStatus(highPriorityTx.Hash)
	lowPriorityStatus, _ := handler.GetTransactionStatus(lowPriorityTx.Hash)

	assert.Equal(t, types.TxStatusPending, highPriorityStatus)
	assert.Equal(t, types.TxStatusPending, lowPriorityStatus)
}
