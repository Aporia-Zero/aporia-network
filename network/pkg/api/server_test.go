package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerInitialization(t *testing.T) {
	config := DefaultAPIConfig()
	services := &APIServices{
		NodeService:        &MockNodeService{},
		TransactionService: &MockTransactionService{},
	}

	server, err := NewAPIServer(config, services)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.NotNil(t, server.router)
}

func TestServerStartStop(t *testing.T) {
	config := DefaultAPIConfig()
	config.Port = 8081 // Use different port for testing

	services := &APIServices{
		NodeService:        &MockNodeService{},
		TransactionService: &MockTransactionService{},
	}

	server, err := NewAPIServer(config, services)
	require.NoError(t, err)

	// Start server
	err = server.Start()
	require.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test server is running
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/health", config.Host, config.Port))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop server
	err = server.Stop()
	require.NoError(t, err)
}

func TestRouteConfiguration(t *testing.T) {
	router, _, _, _ := setupTestAPI()

	tests := []struct {
		name         string
		method       string
		path         string
		expectedCode int
	}{
		{
			name:         "Node info route",
			method:       "GET",
			path:         "/api/v1/node/info",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Peers route",
			method:       "GET",
			path:         "/api/v1/node/peers",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Invalid route",
			method:       "GET",
			path:         "/invalid",
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "Invalid method",
			method:       "POST",
			path:         "/api/v1/node/info",
			expectedCode: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestHealthCheck(t *testing.T) {
	router, _, _, _ := setupTestAPI()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response["status"])
	assert.NotNil(t, response["time"])
}

func TestMetricsEndpoint(t *testing.T) {
	config := DefaultAPIConfig()
	config.EnableMetrics = true

	services := &APIServices{
		NodeService:        &MockNodeService{},
		TransactionService: &MockTransactionService{},
	}

	server, err := NewAPIServer(config, services)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check for required metric categories
	assert.Contains(t, response, "uptime")
	assert.Contains(t, response, "memory")
	assert.Contains(t, response, "goroutines")
}

func TestSwaggerEndpoint(t *testing.T) {
	config := DefaultAPIConfig()
	config.EnableSwagger = true

	services := &APIServices{
		NodeService:        &MockNodeService{},
		TransactionService: &MockTransactionService{},
	}

	server, err := NewAPIServer(config, services)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/swagger/index.html", nil)
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestServerTimeout(t *testing.T) {
	config := DefaultAPIConfig()
	config.ReadTimeout = 100 * time.Millisecond

	services := &APIServices{
		NodeService:        &MockNodeService{},
		TransactionService: &MockTransactionService{},
	}

	server, err := NewAPIServer(config, services)
	require.NoError(t, err)

	slowHandler := func(c *gin.Context) {
		time.Sleep(200 * time.Millisecond)
		c.String(http.StatusOK, "response")
	}

	server.router.GET("/slow", slowHandler)

	// Start server
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	// Test timeout
	client := &http.Client{}
	resp, err := client.Get(fmt.Sprintf("http://%s:%d/slow", config.Host, config.Port))
	assert.Error(t, err) // Should timeout

	if resp != nil {
		resp.Body.Close()
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	config := DefaultAPIConfig()
	services := &APIServices{
		NodeService:        &MockNodeService{},
		TransactionService: &MockTransactionService{},
	}

	server, err := NewAPIServer(config, services)
	require.NoError(t, err)

	// Add handler that takes some time
	server.router.GET("/long", func(c *gin.Context) {
		time.Sleep(100 * time.Millisecond)
		c.String(http.StatusOK, "done")
	})

	// Start server
	err = server.Start()
	require.NoError(t, err)

	// Start a request that will take some time
	go func() {
		client := &http.Client{}
		resp, err := client.Get(fmt.Sprintf("http://%s:%d/long", config.Host, config.Port))
		if err == nil {
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()
		}
	}()

	// Give request time to start
	time.Sleep(50 * time.Millisecond)

	// Stop server gracefully
	done := make(chan bool)
	go func() {
		err = server.Stop()
		require.NoError(t, err)
		done <- true
	}()

	// Verify shutdown completes
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Server shutdown timed out")
	}
}

func TestServerErrorHandling(t *testing.T) {
	router, _, mockNode, mockTx := setupTestAPI()

	tests := []struct {
		name         string
		setup        func()
		method       string
		path         string
		expectedCode int
		expectedErr  string
	}{
		{
			name: "Service error",
			setup: func() {
				mockNode.On("GetNodeInfo").Return(types.NodeInfo{}).Once()
			},
			method:       "GET",
			path:         "/api/v1/node/info",
			expectedCode: http.StatusInternalServerError,
			expectedErr:  "Service error",
		},
		{
			name: "Validation error",
			setup: func() {
				mockTx.On("SubmitTransaction", nil).Return(fmt.Errorf("Invalid transaction")).Once()
			},
			method:       "POST",
			path:         "/api/v1/tx",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "Invalid transaction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)

			var response APIError
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.Contains(t, response.Message, tt.expectedErr)
		})
	}
}
