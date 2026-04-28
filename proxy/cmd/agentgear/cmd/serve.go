package cmd

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/sunshow/agentgear/proxy/internal/api"
	"github.com/sunshow/agentgear/proxy/internal/config"
	"github.com/sunshow/agentgear/proxy/internal/logger"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/proxy"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
)

var configPath string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the proxy server",
	Long:  "Start the AgentGear proxy server with gateway routing, transformers, and API management.",
	RunE:  runServeE,
}

func init() {
	serveCmd.Flags().StringVarP(&configPath, "config", "c", "", "path to config file")

	// Make serve the default command when no subcommand is specified
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runServeE(cmd, args)
	}
	// Copy the config flag to root as well for backward compatibility
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "", "path to config file")
}

func runServeE(cmd *cobra.Command, args []string) error {
	runServe(cmd, args)
	return nil
}

func runServe(cmd *cobra.Command, args []string) {
	// Determine actual config file path
	actualConfigPath := configPath
	if actualConfigPath == "" {
		actualConfigPath = "./configs/config.yaml"
	}

	cfg, err := config.Load(actualConfigPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Printf("=== Configuration Loaded ===")
	log.Printf("Server: host=%s, port=%d, api_port=%d", cfg.Server.Host, cfg.Server.Port, cfg.Server.APIPort)
	log.Printf("Gateways: %d configured", len(cfg.Gateways))
	for _, gw := range cfg.Gateways {
		log.Printf("  - %s: path=%s, upstream=%s, enabled=%v", gw.Name, gw.Path, gw.Upstream, gw.Enabled)
	}
	log.Printf("Logging: enabled=%v, dir=%s", cfg.Logging.Enabled, cfg.Logging.Dir)
	log.Printf("Memory: max_connections=%d, retention=%dm", cfg.Memory.MaxConnections, cfg.Memory.RetentionMinutes)
	log.Printf("ThinkingStore: max_entries=%d, ttl=%dm", cfg.ThinkingStore.MaxEntries, cfg.ThinkingStore.EntryTTLMinutes)
	log.Printf("Tagging: %d rules", len(cfg.Tagging.Rules))
	log.Printf("Transformers: %d definitions, %d mappings", len(cfg.Transformers.Definitions), len(cfg.Transformers.Mappings))
	log.Printf("===========================\n")

	// Shared session log toggle across all handlers and API server
	var sessionLogEnabled atomic.Bool
	sessionLogEnabled.Store(cfg.Logging.Enabled)

	// Business logger: always enabled for tagging/transformer/mapping logs
	businessLogger, err := logger.NewBusinessLogger(cfg.Logging.Dir)
	if err != nil {
		log.Fatalf("failed to create business logger: %v", err)
	}
	defer businessLogger.Sync()

	// Initialize config writer for CRUD operations
	configWriter := config.NewConfigWriter(actualConfigPath, cfg)
	defer configWriter.Close()

	// Initialize memory store
	memoryStore := memory.NewConnectionStore(cfg.Memory)
	defer memoryStore.Close()

	// Initialize thinking store
	thinkingStore := memory.NewThinkingStore(cfg.ThinkingStore)
	defer thinkingStore.Close()

	// Initialize tagging engine
	taggingEngine := tagging.NewEngine(cfg.Tagging)

	// Initialize transformer registry (always initialize for API management)
	transformerRegistry := transformer.NewRegistry(cfg.Transformers)

	// Initialize WebSocket hub
	wsHub := api.NewWSHub(businessLogger.Logger)
	go wsHub.Run()

	// Start API server in goroutine
	if cfg.Server.APIPort > 0 {
		apiServer := api.NewServer(api.Config{
			Host:              cfg.Server.APIHost,
			Port:              cfg.Server.APIPort,
			Store:             memoryStore,
			Tagging:           taggingEngine,
			Transformer:       transformerRegistry,
			ConfigWriter:      configWriter,
			Logger:            businessLogger.Logger,
			SessionLogEnabled: &sessionLogEnabled,
		})
		apiServer.SetWSHub(wsHub)

		go func() {
			if err := apiServer.Run(); err != nil {
				businessLogger.Sugar().Errorf("API server error: %v", err)
			}
		}()
		businessLogger.Sugar().Infof("API server starting on %s:%d", cfg.Server.APIHost, cfg.Server.APIPort)
	}

	// Start proxy server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Register gateway routes
	enabledGateways := 0
	for _, gw := range cfg.Gateways {
		if !gw.Enabled {
			continue
		}
		enabledGateways++

		proxyHandler := proxy.NewHandler(proxy.Config{
			GatewayName:         gw.Name,
			GatewayPath:         gw.Path,
			UpstreamURL:         gw.Upstream,
			UpstreamType:        gw.Type,
			Timeout:             time.Duration(gw.Timeout) * time.Second,
			BusinessLogger:      businessLogger.Logger,
			LogDir:              cfg.Logging.Dir,
			LogEnabled:          &sessionLogEnabled,
			MemoryStore:         memoryStore,
			ThinkingStore:       thinkingStore,
			TaggingEngine:       taggingEngine,
			TransformerRegistry: transformerRegistry,
			WSHub:               wsHub,
		})

		r.Any(gw.Path+"/*path", proxyHandler.ProxyRequest)
		businessLogger.Sugar().Infof("Gateway '%s' registered: %s/* -> %s", gw.Name, gw.Path, gw.Upstream)
	}

	if enabledGateways == 0 {
		log.Printf("warning: no enabled gateways configured")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	businessLogger.Sugar().Infof("AgentGear proxy starting on %s with %d gateway(s)", addr, enabledGateways)

	if err := r.Run(addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
