package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter() *gin.Engine {
	router := gin.New()
	logger, _ := zap.NewDevelopment()

	// Add test middleware
	router.Use(recoveryMiddleware(logger))
	router.Use(loggerMiddleware(logger))
	router.Use(corsMiddleware([]string{"*"}))

	return router
}

func TestCORSMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Test endpoint
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "test")
	})

	tests := []struct {
		name           string
		origin         string
		method         string
		expectedOrigin string
		expectedStatus int
	}{
		{
			name:           "Valid origin",
			origin:         "http://localhost:3000",
			method:         "GET",
			expectedOrigin: "http://localhost:3000",
			expectedStatus: 200,
		},
		{
			name:           "OPTIONS request",
			origin:         "http://localhost:3000",
			method:         "OPTIONS",
			expectedOrigin: "http://localhost:3000",
			expectedStatus: 200,
		},
		{
			name:           "No origin",
			origin:         "",
			method:         "GET",
			expectedOrigin: "",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedOrigin != "" {
				assert.Equal(t, tt.expectedOrigin, w.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestLoggerMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Test endpoint that triggers different response types
	router.GET("/success", func(c *gin.Context) {
		c.String(200, "success")
	})

	router.GET("/error", func(c *gin.Context) {
		c.Error(fmt.Errorf("test error"))
		c.String(500, "error")
	})

	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{
			name:         "Successful request",
			path:         "/success",
			expectedCode: 200,
		},
		{
			name:         "Error request",
			path:         "/error",
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", tt.path, nil)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Endpoint that triggers a panic
	router.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)

	router.ServeHTTP(w, req)

	assert.Equal(t, 500, w.Code)

	var response APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 500, response.Code)
	assert.Equal(t, "Internal server error", response.Message)
}

func TestRateLimiterMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Add rate limiter middleware
	router.Use(rateLimiterMiddleware(2, time.Second))

	router.GET("/test", func(c *gin.Context) {
		c.String(200, "test")
	})

	tests := []struct {
		name         string
		requests     int
		expectedCode int
	}{
		{
			name:         "Under limit",
			requests:     2,
			expectedCode: 200,
		},
		{
			name:         "At limit",
			requests:     3,
			expectedCode: 429,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < tt.requests; i++ {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "/test", nil)
				req.RemoteAddr = "192.168.1.1:12345" // Set client IP

				router.ServeHTTP(w, req)

				if i == tt.requests-1 {
					assert.Equal(t, tt.expectedCode, w.Code)
				}
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Add auth middleware
	router.Use(authMiddleware(AuthTypeBearer))

	router.GET("/protected", func(c *gin.Context) {
		c.String(200, "protected")
	})

	tests := []struct {
		name         string
		token        string
		expectedCode int
	}{
		{
			name:         "Valid token",
			token:        "Bearer valid-token",
			expectedCode: 200,
		},
		{
			name:         "Missing token",
			token:        "",
			expectedCode: 401,
		},
		{
			name:         "Invalid token",
			token:        "Bearer invalid-token",
			expectedCode: 401,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/protected", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", tt.token)
			}

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestValidationMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Add validation middleware
	router.Use(validationMiddleware())

	router.POST("/test", func(c *gin.Context) {
		c.String(200, "test")
	})

	tests := []struct {
		name         string
		contentType  string
		expectedCode int
	}{
		{
			name:         "Valid content type",
			contentType:  "application/json",
			expectedCode: 200,
		},
		{
			name:         "Invalid content type",
			contentType:  "text/plain",
			expectedCode: 415,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/test", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestMetricsMiddleware(t *testing.T) {
	router := setupTestRouter()

	// Add metrics middleware
	router.Use(metricsMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.String(200, "test")
	})

	// Make some requests
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
	}

	// Verify metrics were collected
	metrics := collectMetrics()
	assert.NotNil(t, metrics)
	assert.Greater(t, metrics["request_count"], float64(0))
}

func TestMiddlewareCombination(t *testing.T) {
	router := setupTestRouter()

	// Add multiple middleware
	router.Use(
		recoveryMiddleware(zap.NewExample()),
		loggerMiddleware(zap.NewExample()),
		corsMiddleware([]string{"*"}),
		rateLimiterMiddleware(5, time.Second),
		validationMiddleware(),
		metricsMiddleware(),
	)

	router.POST("/test", func(c *gin.Context) {
		c.String(200, "test")
	})

	// Test successful request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")

	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
}
