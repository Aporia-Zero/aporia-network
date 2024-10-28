package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aporia-zero/network/pkg/api"
	"github.com/aporia-zero/network/pkg/mempool"
	"github.com/aporia-zero/network/pkg/p2p"
	"github.com/aporia-zero/network/pkg/types"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

var (
	configPath = flag.String("config", "config.yaml", "Path to configuration file")
	logLevel   = flag.String("log-level", "info", "Logging level (debug, info, warn, error)")
)

type Config struct {
	// Network configuration
	Network struct {
		ListenAddresses []string `yaml:"listen_addresses"`
		BootstrapPeers  []string `yaml:"bootstrap_peers"`
		MaxPeers        int      `yaml:"max_peers"`
		ProtocolID      string   `yaml:"protocol_id"`
	} `yaml:"network"`

	// API configuration
	API struct {
		Host          string   `yaml:"host"`
		Port          int      `yaml:"port"`
		EnableSwagger bool     `yaml:"enable_swagger"`
		EnableMetrics bool     `yaml:"enable_metrics"`
		CorsAllowList []string `yaml:"cors_allow_list"`
	} `yaml:"api"`

	// Mempool configuration
	Mempool struct {
		MaxSize            int           `yaml:"max_size"`
		MaxTransactionSize int64         `yaml:"max_transaction_size"`
		ExpirationDuration time.Duration `yaml:"expiration_duration"`
		CleanupInterval    time.Duration `yaml:"cleanup_interval"`
	} `yaml:"mempool"`
}

func main() {
	flag.Parse()

	// Initialize logger
	logger, err := initLogger(*logLevel)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration",
			zap.Error(err),
			zap.String("path", *configPath))
	}

	// Create application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize P2P node
	node, err := initP2PNode(ctx, config, logger)
	if err != nil {
		logger.Fatal("Failed to initialize P2P node",
			zap.Error(err))
	}

	// Initialize mempool
	txPool, err := initMempool(config, logger)
	if err != nil {
		logger.Fatal("Failed to initialize mempool",
			zap.Error(err))
	}

	// Initialize API server
	apiServer, err := initAPIServer(config, node, txPool, logger)
	if err != nil {
		logger.Fatal("Failed to initialize API server",
			zap.Error(err))
	}

	// Start services
	if err := startServices(ctx, node, txPool, apiServer, logger); err != nil {
		logger.Fatal("Failed to start services",
			zap.Error(err))
	}

	// Wait for shutdown signal
	waitForShutdown(ctx, cancel, node, txPool, apiServer, logger)
}

func initLogger(level string) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(getLogLevel(level))
	return config.Build()
}

func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	default:
		return zap.InfoLevel
	}
}

func loadConfig(path string) (*Config, error) {
	configFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(configFile, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &config, nil
}

func initP2PNode(ctx context.Context, config *Config, logger *zap.Logger) (*p2p.Node, error) {
	nodeConfig := &types.NetworkConfig{
		ListenAddresses: config.Network.ListenAddresses,
		BootstrapPeers:  config.Network.BootstrapPeers,
		MaxPeers:        config.Network.MaxPeers,
		ProtocolID:      config.Network.ProtocolID,
	}

	node, err := p2p.NewNode(nodeConfig)
	if err != nil {
		return nil, fmt.Errorf("creating P2P node: %w", err)
	}

	return node, nil
}

func initMempool(config *Config, logger *zap.Logger) (*mempool.TransactionPool, error) {
	poolConfig := &mempool.PoolConfig{
		MaxSize:            config.Mempool.MaxSize,
		MaxTransactionSize: config.Mempool.MaxTransactionSize,
		ExpirationDuration: config.Mempool.ExpirationDuration,
		CleanupInterval:    config.Mempool.CleanupInterval,
	}

	pool, err := mempool.NewTransactionPool(poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating transaction pool: %w", err)
	}

	return pool, nil
}

func initAPIServer(config *Config, node *p2p.Node, pool *mempool.TransactionPool, logger *zap.Logger) (*api.APIServer, error) {
	apiConfig := &api.APIConfig{
		Host:           config.API.Host,
		Port:           config.API.Port,
		EnableSwagger:  config.API.EnableSwagger,
		EnableMetrics:  config.API.EnableMetrics,
		AllowedOrigins: config.API.CorsAllowList,
	}

	// Create API services
	services := &api.APIServices{
		NodeService:        node,
		TransactionService: pool,
	}

	server, err := api.NewAPIServer(apiConfig, services)
	if err != nil {
		return nil, fmt.Errorf("creating API server: %w", err)
	}

	return server, nil
}

func startServices(
	ctx context.Context,
	node *p2p.Node,
	pool *mempool.TransactionPool,
	server *api.APIServer,
	logger *zap.Logger,
) error {
	// Start P2P node
	if err := node.Start(); err != nil {
		return fmt.Errorf("starting P2P node: %w", err)
	}
	logger.Info("P2P node started")

	// Start mempool
	if err := pool.Start(); err != nil {
		return fmt.Errorf("starting mempool: %w", err)
	}
	logger.Info("Mempool started")

	// Start API server
	if err := server.Start(); err != nil {
		return fmt.Errorf("starting API server: %w", err)
	}
	logger.Info("API server started")

	return nil
}

func waitForShutdown(
	ctx context.Context,
	cancel context.CancelFunc,
	node *p2p.Node,
	pool *mempool.TransactionPool,
	server *api.APIServer,
	logger *zap.Logger,
) {
	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Info("Received shutdown signal",
		zap.String("signal", sig.String()))

	// Cancel context
	cancel()

	// Graceful shutdown timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown services
	logger.Info("Shutting down services...")

	if err := server.Stop(); err != nil {
		logger.Error("Error stopping API server",
			zap.Error(err))
	}

	if err := pool.Stop(); err != nil {
		logger.Error("Error stopping mempool",
			zap.Error(err))
	}

	if err := node.Stop(); err != nil {
		logger.Error("Error stopping P2P node",
			zap.Error(err))
	}

	logger.Info("Shutdown complete")
}
