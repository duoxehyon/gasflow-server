package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// Config stores application settings
type Config struct {
	// Ethereum connection
	EthEndpoint string

	// Server ports
	WSPort  string
	APIPort string

	// Feature flags
	EnableWS     bool
	EnableAPI    bool
	EnableSaving bool

	// Storage
	OutputDir string
}

// Initialize logger
var logger = logrus.New()

func initLogger() {
	logger.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: time.RFC3339, // ISO 8601 format
		FullTimestamp:   true,
	})
	logger.SetOutput(os.Stdout)       // Log to console
	logger.SetLevel(logrus.InfoLevel) // Default log level
}

func parseConfig() *Config {
	cfg := &Config{}

	// Ethereum settings
	flag.StringVar(&cfg.EthEndpoint, "eth", "ws://localhost:8546", "Ethereum node websocket endpoint")

	// Server ports
	flag.StringVar(&cfg.WSPort, "ws-port", ":8080", "WebSocket server port")
	flag.StringVar(&cfg.APIPort, "api-port", ":8081", "Internal API server port")

	// Feature flags
	flag.BoolVar(&cfg.EnableWS, "enable-ws", false, "Enable WebSocket server")
	flag.BoolVar(&cfg.EnableAPI, "enable-api", false, "Enable internal API server")
	flag.BoolVar(&cfg.EnableSaving, "enable-saving", false, "Enable block data saving")

	// Storage settings (only used if enable-saving is true)
	flag.StringVar(&cfg.OutputDir, "output", "data", "Directory for saved block data")

	// Parse flags
	flag.Parse()

	// Validate configuration
	if cfg.EthEndpoint == "" {
		logger.Fatal("Ethereum endpoint is required")
	}

	// Create output directory if saving is enabled
	if cfg.EnableSaving {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			logger.Fatalf("Failed to create output directory: %v", err)
		}
	}

	return cfg
}

func printBanner(cfg *Config) {
	logger.Info("🚀 Starting Blockchain Data Engine")
	logger.WithFields(logrus.Fields{
		"Ethereum Node": cfg.EthEndpoint,
		"WebSocket":     cfg.EnableWS,
		"API Server":    cfg.EnableAPI,
		"Block Saving":  cfg.EnableSaving,
	}).Info("Configuration Loaded")
}

func main() {
	initLogger()
	cfg := parseConfig()
	printBanner(cfg)

	outputDir := ""
	if cfg.EnableSaving {
		outputDir = cfg.OutputDir
	}

	manager, err := NewDataManager(cfg.EthEndpoint, outputDir)
	if err != nil {
		logger.Fatalf("Failed to create data manager: %v", err)
	}

	var wsServer *WSServer
	if cfg.EnableWS {
		wsServer = NewWSServer(cfg.WSPort)
		manager.SetWSCallback(wsServer.BroadcastUpdate)

		if err := wsServer.Start(); err != nil {
			logger.Fatalf("Failed to start WebSocket server: %v", err)
		}
		logger.Infof("WebSocket server running on %s", cfg.WSPort)
	}

	var apiServer *APIServer
	if cfg.EnableAPI {
		apiServer = NewAPIServer(cfg.APIPort, manager)
		if err := apiServer.Start(); err != nil {
			logger.Fatalf("Failed to start API server: %v", err)
		}
		logger.Infof("Internal API server running on %s", cfg.APIPort)
	}

	// Start the manager
	if err := manager.Start(); err != nil {
		logger.Fatalf("Failed to start data manager: %v", err)
	}
	logger.Info("Data manager started successfully")

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Warnf("Received signal %v, shutting down...", sig)

	// Graceful shutdown
	if cfg.EnableWS {
		wsServer.Stop()
		logger.Info("WebSocket server stopped")
	}

	manager.Stop()
	logger.Info("Data manager stopped")
	logger.Info("Shutdown complete")
}
