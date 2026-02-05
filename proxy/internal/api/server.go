package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunshow/agentgear/proxy/internal/config"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
	"go.uber.org/zap"
)

type Server struct {
	router       *gin.Engine
	store        *memory.ConnectionStore
	tagging      *tagging.Engine
	transformer  *transformer.Registry
	configWriter *config.ConfigWriter
	logger       *zap.Logger
	addr         string
}

type Config struct {
	Host         string
	Port         int
	Store        *memory.ConnectionStore
	Tagging      *tagging.Engine
	Transformer  *transformer.Registry
	ConfigWriter *config.ConfigWriter
	Logger       *zap.Logger
}

func NewServer(cfg Config) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	s := &Server{
		router:       router,
		store:        cfg.Store,
		tagging:      cfg.Tagging,
		transformer:  cfg.Transformer,
		configWriter: cfg.ConfigWriter,
		logger:       cfg.Logger,
		addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}

	s.setupRoutes()
	return s
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func (s *Server) setupRoutes() {
	api := s.router.Group("/api")
	{
		api.GET("/health", s.healthCheck)
		api.GET("/stats", s.getStats)

		api.GET("/connections", s.listConnections)
		api.GET("/connections/:id", s.getConnection)
		api.DELETE("/connections", s.clearConnections)

		api.GET("/tags", s.listTags)

		api.GET("/tagging/rules", s.listTaggingRules)
		api.PUT("/tagging/rules/:name", s.updateTaggingRule)
		api.DELETE("/tagging/rules/:name", s.deleteTaggingRule)
		api.POST("/tagging/test", s.testTagging)

		// Transformer definitions API
		api.GET("/transformers/defs", s.listDefinitions)
		api.POST("/transformers/defs", s.createDefinition)
		api.PUT("/transformers/defs/:name", s.updateDefinition)
		api.DELETE("/transformers/defs/:name", s.deleteDefinition)

		// Mapping rules API
		api.GET("/mappings", s.listMappings)
		api.POST("/mappings", s.createMapping)
		api.PUT("/mappings/:name", s.updateMappingRule)
		api.DELETE("/mappings/:name", s.deleteMappingRule)

		api.GET("/gateways", s.listGateways)
		api.POST("/gateways", s.createGateway)
		api.PUT("/gateways/:name", s.updateGateway)
		api.DELETE("/gateways/:name", s.deleteGateway)

		api.POST("/config/save", s.saveConfig)
		api.POST("/restart", s.restartProxy)
	}
}

func (s *Server) Run() error {
	s.logger.Info("API server starting", zap.String("addr", s.addr))
	return s.router.Run(s.addr)
}

func (s *Server) Router() *gin.Engine {
	return s.router
}
