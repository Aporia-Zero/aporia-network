package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// APIServer represents the REST API server
type APIServer struct {
	// Configuration
	config *APIConfig

	// Router
	router *gin.Engine

	// Services
	services *APIServices

	// Server instance
	server *http.Server

	// Logger
	logger *zap.Logger
}

// APIConfig represents API configuration
type APIConfig struct {
	Host           string
	Port           int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
	AllowedOrigins []string
	EnableSwagger  bool
	EnableMetrics  bool
}

// APIServices represents the services used by the API
type APIServices struct {
	NodeService  NodeService
	TxService    TransactionService
	BlockService BlockService
	StateService StateService
}

// Service interfaces
type NodeService interface {
	GetNodeInfo() types.NodeInfo
	GetPeers() []types.Peer
	AddPeer(addr string) error
	RemovePeer(id string) error
}

type TransactionService interface {
	SubmitTransaction(tx *types.Transaction) error
	GetTransaction(hash string) (*types.Transaction, error)
	GetPendingTransactions() ([]*types.Transaction, error)
	GetTransactionStatus(hash string) (types.TxStatus, error)
}

type BlockService interface {
	GetBlock(hash string) (*types.Block, error)
	GetBlockByHeight(height uint64) (*types.Block, error)
	GetBlockRange(start, end uint64) ([]*types.Block, error)
	GetLatestBlock() (*types.Block, error)
}

type StateService interface {
	GetAccount(id string) (*types.Account, error)
	GetStateRoot() ([32]byte, error)
	GetStateProof(account string) ([]byte, error)
}

// NewAPIServer creates a new API server
func NewAPIServer(config *APIConfig, services *APIServices) (*APIServer, error) {
	logger, _ := zap.NewProduction()

	// Create Gin router
	router := gin.New()
	router.Use(gin.Recovery())

	// Create server
	server := &APIServer{
		config:   config,
		router:   router,
		services: services,
		logger:   logger,
	}

	// Initialize routes
	server.initializeRoutes()

	return server, nil
}

// Start starts the API server
func (s *APIServer) Start() error {
	// Configure HTTP server
	s.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:        s.router,
		ReadTimeout:    s.config.ReadTimeout,
		WriteTimeout:   s.config.WriteTimeout,
		MaxHeaderBytes: s.config.MaxHeaderBytes,
	}

	// Start server
	s.logger.Info("Starting API server",
		zap.String("host", s.config.Host),
		zap.Int("port", s.config.Port))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the API server
func (s *APIServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	s.logger.Info("API server stopped")
	return nil
}

// Initialize routes
func (s *APIServer) initializeRoutes() {
	// Add middleware
	s.router.Use(corsMiddleware(s.config.AllowedOrigins))
	s.router.Use(loggerMiddleware(s.logger))
	s.router.Use(recoveryMiddleware(s.logger))

	// API version group
	v1 := s.router.Group("/api/v1")
	{
		// Node endpoints
		node := v1.Group("/node")
		{
			node.GET("/info", s.handleGetNodeInfo)
			node.GET("/peers", s.handleGetPeers)
			node.POST("/peers", s.handleAddPeer)
			node.DELETE("/peers/:id", s.handleRemovePeer)
		}

		// Transaction endpoints
		tx := v1.Group("/tx")
		{
			tx.POST("", s.handleSubmitTransaction)
			tx.GET("/:hash", s.handleGetTransaction)
			tx.GET("/pending", s.handleGetPendingTransactions)
			tx.GET("/:hash/status", s.handleGetTransactionStatus)
		}

		// Block endpoints
		block := v1.Group("/block")
		{
			block.GET("/:hash", s.handleGetBlock)
			block.GET("/height/:height", s.handleGetBlockByHeight)
			block.GET("/range", s.handleGetBlockRange)
			block.GET("/latest", s.handleGetLatestBlock)
		}

		// State endpoints
		state := v1.Group("/state")
		{
			state.GET("/account/:id", s.handleGetAccount)
			state.GET("/root", s.handleGetStateRoot)
			state.GET("/proof/:account", s.handleGetStateProof)
		}
	}

	// Metrics endpoint
	if s.config.EnableMetrics {
		s.router.GET("/metrics", s.handleMetrics)
	}

	// Swagger documentation
	if s.config.EnableSwagger {
		s.router.GET("/swagger/*any", s.handleSwagger)
	}
}

// Health check endpoint
func (s *APIServer) handleHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

// GetRouter returns the Gin router instance
func (s *APIServer) GetRouter() *gin.Engine {
	return s.router
}

// Default configuration
func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		Host:           "0.0.0.0",
		Port:           8080,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
		AllowedOrigins: []string{"*"},
		EnableSwagger:  true,
		EnableMetrics:  true,
	}
}

// API error response
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// API success response
type APIResponse struct {
	Data    interface{} `json:"data"`
	Message string      `json:"message,omitempty"`
}

// Helper function for error responses
func errorResponse(c *gin.Context, status int, err error) {
	c.JSON(status, APIError{
		Code:    status,
		Message: err.Error(),
	})
}

// Helper function for success responses
func successResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Data: data,
	})
}

// Helper function for paginated responses
func paginatedResponse(c *gin.Context, data interface{}, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, gin.H{
		"data":        data,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}
package api

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/aporia-zero/network/pkg/types"
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

// APIServer represents the REST API server
type APIServer struct {
    // Configuration
    config     *APIConfig
    
    // Router
    router     *gin.Engine
    
    // Services
    services   *APIServices
    
    // Server instance
    server     *http.Server
    
    // Logger
    logger     *zap.Logger
}

// APIConfig represents API configuration
type APIConfig struct {
    Host            string
    Port            int
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    MaxHeaderBytes  int
    AllowedOrigins  []string
    EnableSwagger   bool
    EnableMetrics   bool
}

// APIServices represents the services used by the API
type APIServices struct {
    NodeService     NodeService
    TxService       TransactionService
    BlockService    BlockService
    StateService    StateService
}

// Service interfaces
type NodeService interface {
    GetNodeInfo() types.NodeInfo
    GetPeers() []types.Peer
    AddPeer(addr string) error
    RemovePeer(id string) error
}

type TransactionService interface {
    SubmitTransaction(tx *types.Transaction) error
    GetTransaction(hash string) (*types.Transaction, error)
    GetPendingTransactions() ([]*types.Transaction, error)
    GetTransactionStatus(hash string) (types.TxStatus, error)
}

type BlockService interface {
    GetBlock(hash string) (*types.Block, error)
    GetBlockByHeight(height uint64) (*types.Block, error)
    GetBlockRange(start, end uint64) ([]*types.Block, error)
    GetLatestBlock() (*types.Block, error)
}

type StateService interface {
    GetAccount(id string) (*types.Account, error)
    GetStateRoot() ([32]byte, error)
    GetStateProof(account string) ([]byte, error)
}

// NewAPIServer creates a new API server
func NewAPIServer(config *APIConfig, services *APIServices) (*APIServer, error) {
    logger, _ := zap.NewProduction()

    // Create Gin router
    router := gin.New()
    router.Use(gin.Recovery())

    // Create server
    server := &APIServer{
        config:     config,
        router:     router,
        services:   services,
        logger:     logger,
    }

    // Initialize routes
    server.initializeRoutes()

    return server, nil
}

// Start starts the API server
func (s *APIServer) Start() error {
    // Configure HTTP server
    s.server = &http.Server{
        Addr:           fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
        Handler:        s.router,
        ReadTimeout:    s.config.ReadTimeout,
        WriteTimeout:   s.config.WriteTimeout,
        MaxHeaderBytes: s.config.MaxHeaderBytes,
    }

    // Start server
    s.logger.Info("Starting API server",
        zap.String("host", s.config.Host),
        zap.Int("port", s.config.Port))

    go func() {
        if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            s.logger.Error("API server error", zap.Error(err))
        }
    }()

    return nil
}

// Stop stops the API server
func (s *APIServer) Stop() error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := s.server.Shutdown(ctx); err != nil {
        return fmt.Errorf("server shutdown failed: %w", err)
    }

    s.logger.Info("API server stopped")
    return nil
}

// Initialize routes
func (s *APIServer) initializeRoutes() {
    // Add middleware
    s.router.Use(corsMiddleware(s.config.AllowedOrigins))
    s.router.Use(loggerMiddleware(s.logger))
    s.router.Use(recoveryMiddleware(s.logger))

    // API version group
    v1 := s.router.Group("/api/v1")
    {
        // Node endpoints
        node := v1.Group("/node")
        {
            node.GET("/info", s.handleGetNodeInfo)
            node.GET("/peers", s.handleGetPeers)
            node.POST("/peers", s.handleAddPeer)
            node.DELETE("/peers/:id", s.handleRemovePeer)
        }

        // Transaction endpoints
        tx := v1.Group("/tx")
        {
            tx.POST("", s.handleSubmitTransaction)
            tx.GET("/:hash", s.handleGetTransaction)
            tx.GET("/pending", s.handleGetPendingTransactions)
            tx.GET("/:hash/status", s.handleGetTransactionStatus)
        }

        // Block endpoints
        block := v1.Group("/block")
        {
            block.GET("/:hash", s.handleGetBlock)
            block.GET("/height/:height", s.handleGetBlockByHeight)
            block.GET("/range", s.handleGetBlockRange)
            block.GET("/latest", s.handleGetLatestBlock)
        }

        // State endpoints
        state := v1.Group("/state")
        {
            state.GET("/account/:id", s.handleGetAccount)
            state.GET("/root", s.handleGetStateRoot)
            state.GET("/proof/:account", s.handleGetStateProof)
        }
    }

    // Metrics endpoint
    if s.config.EnableMetrics {
        s.router.GET("/metrics", s.handleMetrics)
    }

    // Swagger documentation
    if s.config.EnableSwagger {
        s.router.GET("/swagger/*any", s.handleSwagger)
    }
}

// Health check endpoint
func (s *APIServer) handleHealthCheck(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "status": "ok",
        "time":   time.Now().Unix(),
    })
}

// GetRouter returns the Gin router instance
func (s *APIServer) GetRouter() *gin.Engine {
    return s.router
}

// Default configuration
func DefaultAPIConfig() *APIConfig {
    return &APIConfig{
        Host:           "0.0.0.0",
        Port:           8080,
        ReadTimeout:    10 * time.Second,
        WriteTimeout:   10 * time.Second,
        MaxHeaderBytes: 1 << 20, // 1MB
        AllowedOrigins: []string{"*"},
        EnableSwagger:  true,
        EnableMetrics:  true,
    }
}

// API error response
type APIError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// API success response
type APIResponse struct {
    Data    interface{} `json:"data"`
    Message string      `json:"message,omitempty"`
}

// Helper function for error responses
func errorResponse(c *gin.Context, status int, err error) {
    c.JSON(status, APIError{
        Code:    status,
        Message: err.Error(),
    })
}

// Helper function for success responses
func successResponse(c *gin.Context, data interface{}) {
    c.JSON(http.StatusOK, APIResponse{
        Data: data,
    })
}

// Helper function for paginated responses
func paginatedResponse(c *gin.Context, data interface{}, total int64, page, pageSize int) {
    c.JSON(http.StatusOK, gin.H{
        "data":       data,
        "total":      total,
        "page":       page,
        "page_size":  pageSize,
        "total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
    })
}