package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock services
type MockNodeService struct {
	mock.Mock
}

func (m *MockNodeService) GetNodeInfo() types.NodeInfo {
	args := m.Called()
	return args.Get(0).(types.NodeInfo)
}

func (m *MockNodeService) GetPeers() []types.Peer {
	args := m.Called()
	return args.Get(0).([]types.Peer)
}

func (m *MockNodeService) AddPeer(addr string) error {
	args := m.Called(addr)
	return args.Error(0)
}

func (m *MockNodeService) RemovePeer(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

type MockTransactionService struct {
	mock.Mock
}

func (m *MockTransactionService) SubmitTransaction(tx *types.Transaction) error {
	args := m.Called(tx)
	return args.Error(0)
}

func (m *MockTransactionService) GetTransaction(hash string) (*types.Transaction, error) {
	args := m.Called(hash)
	return args.Get(0).(*types.Transaction), args.Error(1)
}

func (m *MockTransactionService) GetPendingTransactions() ([]*types.Transaction, error) {
	args := m.Called()
	return args.Get(0).([]*types.Transaction), args.Error(1)
}

func (m *MockTransactionService) GetTransactionStatus(hash string) (types.TxStatus, error) {
	args := m.Called(hash)
	return args.Get(0).(types.TxStatus), args.Error(1)
}

// Setup test environment
func setupTestAPI() (*gin.Engine, *APIServer, *MockNodeService, *MockTransactionService) {
	gin.SetMode(gin.TestMode)

	mockNode := new(MockNodeService)
	mockTx := new(MockTransactionService)

	services := &APIServices{
		NodeService:        mockNode,
		TransactionService: mockTx,
	}

	config := DefaultAPIConfig()
	server, _ := NewAPIServer(config, services)

	return server.GetRouter(), server, mockNode, mockTx
}

// Test node endpoints
func TestHandleGetNodeInfo(t *testing.T) {
	router, _, mockNode, _ := setupTestAPI()

	expectedInfo := types.NodeInfo{
		ID:        "test-node",
		Version:   "1.0.0",
		PeerCount: 5,
	}

	mockNode.On("GetNodeInfo").Return(expectedInfo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/node/info", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	info, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, expectedInfo.ID, info["id"])
	assert.Equal(t, expectedInfo.Version, info["version"])
	assert.Equal(t, float64(expectedInfo.PeerCount), info["peer_count"])
}

func TestHandleGetPeers(t *testing.T) {
	router, _, mockNode, _ := setupTestAPI()

	expectedPeers := []types.Peer{
		{ID: "peer1", Address: "addr1"},
		{ID: "peer2", Address: "addr2"},
	}

	mockNode.On("GetPeers").Return(expectedPeers)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/node/peers", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	peers, ok := response.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, peers, len(expectedPeers))
}

func TestHandleAddPeer(t *testing.T) {
	router, _, mockNode, _ := setupTestAPI()

	mockNode.On("AddPeer", "test-addr").Return(nil)

	reqBody := map[string]string{"address": "test-addr"}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/node/peers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// Test transaction endpoints
func TestHandleSubmitTransaction(t *testing.T) {
	router, _, _, mockTx := setupTestAPI()

	tx := &types.Transaction{
		Hash:  [32]byte{1, 2, 3},
		Value: 1000,
	}

	mockTx.On("SubmitTransaction", mock.AnythingOfType("*types.Transaction")).Return(nil)

	body, _ := json.Marshal(tx)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tx", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleGetTransaction(t *testing.T) {
	router, _, _, mockTx := setupTestAPI()

	expectedTx := &types.Transaction{
		Hash:  [32]byte{1, 2, 3},
		Value: 1000,
	}

	mockTx.On("GetTransaction", "0x123").Return(expectedTx, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tx/0x123", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
}

func TestHandleGetPendingTransactions(t *testing.T) {
	router, _, _, mockTx := setupTestAPI()

	expectedTxs := []*types.Transaction{
		{Hash: [32]byte{1}, Value: 1000},
		{Hash: [32]byte{2}, Value: 2000},
	}

	mockTx.On("GetPendingTransactions").Return(expectedTxs, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tx/pending", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	txs, ok := response.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, txs, len(expectedTxs))
}

// Test error handling
func TestHandleErrors(t *testing.T) {
	router, _, mockNode, mockTx := setupTestAPI()

	tests := []struct {
		name         string
		setup        func()
		path         string
		method       string
		body         interface{}
		expectedCode int
	}{
		{
			name: "Invalid transaction hash",
			setup: func() {
				mockTx.On("GetTransaction", "invalid").Return((*types.Transaction)(nil), ErrTxNotFound)
			},
			path:         "/api/v1/tx/invalid",
			method:       "GET",
			expectedCode: http.StatusNotFound,
		},
		{
			name: "Invalid peer address",
			setup: func() {
				mockNode.On("AddPeer", "invalid").Return(fmt.Errorf("invalid address"))
			},
			path:         "/api/v1/node/peers",
			method:       "POST",
			body:         map[string]string{"address": "invalid"},
			expectedCode: http.StatusInternalServerError,
		},
		{
			name: "Invalid transaction submission",
			setup: func() {
				mockTx.On("SubmitTransaction", mock.Anything).Return(fmt.Errorf("invalid transaction"))
			},
			path:         "/api/v1/tx",
			method:       "POST",
			body:         &types.Transaction{},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			var body []byte
			if tt.body != nil {
				body, _ = json.Marshal(tt.body)
			}

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, bytes.NewBuffer(body))
			if tt.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)

			var response APIError
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.NotEmpty(t, response.Message)
		})
	}
}

// Test pagination
func TestPagination(t *testing.T) {
	router, _, _, mockTx := setupTestAPI()

	// Create test transactions
	var txs []*types.Transaction
	for i := 0; i < 50; i++ {
		txs = append(txs, &types.Transaction{
			Hash:  [32]byte{byte(i)},
			Value: uint64(i * 1000),
		})
	}

	mockTx.On("GetPendingTransactions").Return(txs, nil)

	tests := []struct {
		name          string
		query         string
		expectedSize  int
		expectedPage  int
		expectedTotal int64
	}{
		{
			name:          "Default pagination",
			query:         "",
			expectedSize:  10,
			expectedPage:  1,
			expectedTotal: 50,
		},
		{
			name:          "Custom page size",
			query:         "?page_size=20",
			expectedSize:  20,
			expectedPage:  1,
			expectedTotal: 50,
		},
		{
			name:          "Custom page",
			query:         "?page=2&page_size=15",
			expectedSize:  15,
			expectedPage:  2,
			expectedTotal: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/v1/tx/pending"+tt.query, nil)
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			data, ok := response["data"].([]interface{})
			require.True(t, ok)
			assert.Len(t, data, tt.expectedSize)
			assert.Equal(t, float64(tt.expectedPage), response["page"])
			assert.Equal(t, float64(tt.expectedTotal), response["total"])
		})
	}
}
