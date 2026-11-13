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
	"github.com/sunshow/agentgear/proxy/internal/video"
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

	// Video protocol gateways share one task store. bbolt allows a single
	// process-wide open of the file, so the store is created once and passed
	// to every adapter.
	var videoStore *video.Store
	var videoAdapters []video.Adapter
	defer func() {
		for _, adapter := range videoAdapters {
			if err := adapter.Close(); err != nil {
				businessLogger.Sugar().Errorf("video adapter close error: %v", err)
			}
		}
		if videoStore != nil {
			if err := videoStore.Close(); err != nil {
				businessLogger.Sugar().Errorf("video store close error: %v", err)
			}
		}
	}()

	// Register gateway routes. A gateway with protocol set is a protocol
	// adapter (doubao, ali, ...); otherwise it stays a transparent proxy.
	enabledGateways := 0
	for _, gw := range cfg.Gateways {
		if !gw.Enabled {
			continue
		}
		enabledGateways++

		if gw.Protocol != "" {
			if videoStore == nil {
				opened, err := video.OpenStore(cfg.Video.StorePath, time.Duration(cfg.Video.TaskTTLHours)*time.Hour)
				if err != nil {
					log.Fatalf("failed to open video task store: %v", err)
				}
				videoStore = opened
			}
			apiKey := cfg.Video.APIKey
			publicBaseURL := cfg.Video.PublicBaseURL
			var models map[string]string
			if gw.Video != nil {
				if gw.Video.APIKey != "" {
					apiKey = gw.Video.APIKey
				}
				if gw.Video.PublicBaseURL != "" {
					publicBaseURL = gw.Video.PublicBaseURL
				}
				models = gw.Video.Models
			}
			createTimeout := time.Duration(cfg.Video.CreateTimeoutSeconds) * time.Second
			if gw.Timeout > 0 {
				createTimeout = time.Duration(gw.Timeout) * time.Second
			}
			videoAdapter, err := video.NewAdapter(video.Config{
				Protocol:      gw.Protocol,
				Prefix:        gw.Path,
				Upstream:      gw.Upstream,
				APIKey:        apiKey,
				PublicBaseURL: publicBaseURL,
				Store:         videoStore,
				StorePath:     cfg.Video.StorePath,
				TaskTTL:       time.Duration(cfg.Video.TaskTTLHours) * time.Hour,
				CreateTimeout: createTimeout,
				QueryTimeout:  time.Duration(cfg.Video.QueryTimeoutSeconds) * time.Second,
				CacheTTL:      time.Duration(cfg.Video.CacheTTLMs) * time.Millisecond,
				Models:        models,
			}, businessLogger.Logger)
			if err != nil {
				log.Fatalf("failed to start video gateway %s: %v", gw.Name, err)
			}
			videoAdapter.Register(r)
			videoAdapters = append(videoAdapters, videoAdapter)
			businessLogger.Sugar().Infof("Video gateway '%s' registered: %s/* protocol=%s -> %s",
				gw.Name, gw.Path, gw.Protocol, gw.Upstream)
			continue
		}

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
