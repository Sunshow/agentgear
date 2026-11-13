package config

import (
	"strings"

	"github.com/spf13/viper"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
)

type Config struct {
	Server        ServerConfig               `mapstructure:"server" yaml:"server"`
	Gateways      []GatewayConfig            `mapstructure:"gateways" yaml:"gateways"`
	Logging       LoggingConfig              `mapstructure:"logging" yaml:"logging"`
	Memory        memory.StoreConfig         `mapstructure:"memory" yaml:"memory"`
	ThinkingStore memory.ThinkingStoreConfig `mapstructure:"thinking_store" yaml:"thinking_store"`
	Tagging       tagging.Config             `mapstructure:"tagging" yaml:"tagging"`
	Transformers  transformer.Config         `mapstructure:"transformers" yaml:"transformers"`
	Video         VideoConfig                `mapstructure:"video" yaml:"video"`
}

type ServerConfig struct {
	Port    int    `mapstructure:"port"`
	Host    string `mapstructure:"host"`
	APIPort int    `mapstructure:"api_port"`
	APIHost string `mapstructure:"api_host"`
}

type GatewayConfig struct {
	Name     string `mapstructure:"name" json:"name" yaml:"name"`
	Path     string `mapstructure:"path" json:"path" yaml:"path"`
	Upstream string `mapstructure:"upstream" json:"upstream" yaml:"upstream"`
	Type     string `mapstructure:"type" json:"type" yaml:"type"`
	// Protocol turns this gateway into a protocol adapter instead of a
	// transparent proxy. Empty keeps the existing transparent behavior.
	Protocol string              `mapstructure:"protocol" json:"protocol" yaml:"protocol"`
	Video    *VideoGatewayConfig `mapstructure:"video" json:"video,omitempty" yaml:"video,omitempty"`
	Timeout  int                 `mapstructure:"timeout" json:"timeout" yaml:"timeout"`
	Enabled  bool                `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
}

// VideoGatewayConfig holds the per-gateway overrides for a video protocol
// adapter (doubao, ali, ...). Empty values fall back to the shared VideoConfig.
type VideoGatewayConfig struct {
	APIKey        string            `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	PublicBaseURL string            `mapstructure:"public_base_url" json:"public_base_url" yaml:"public_base_url"`
	Models        map[string]string `mapstructure:"models" json:"models" yaml:"models"`
}

// VideoConfig holds the settings shared by every video protocol gateway.
type VideoConfig struct {
	StorePath            string `mapstructure:"store_path" json:"store_path" yaml:"store_path"`
	TaskTTLHours         int    `mapstructure:"task_ttl_hours" json:"task_ttl_hours" yaml:"task_ttl_hours"`
	CreateTimeoutSeconds int    `mapstructure:"create_timeout_seconds" json:"create_timeout_seconds" yaml:"create_timeout_seconds"`
	QueryTimeoutSeconds  int    `mapstructure:"query_timeout_seconds" json:"query_timeout_seconds" yaml:"query_timeout_seconds"`
	CacheTTLMs           int    `mapstructure:"cache_ttl_ms" json:"cache_ttl_ms" yaml:"cache_ttl_ms"`
	// APIKey is the default upstream key for every video gateway. Empty means
	// "forward the inbound Authorization bearer token".
	APIKey string `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	// PublicBaseURL is the default externally reachable base URL used to build
	// the local content proxy URL. Empty returns the upstream asset URL.
	PublicBaseURL string `mapstructure:"public_base_url" json:"public_base_url" yaml:"public_base_url"`
}

type LoggingConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Dir        string `mapstructure:"dir"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

func Load(configPath string) (*Config, error) {
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./configs")
	}

	viper.SetEnvPrefix("AGENTGEAR")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Server defaults
	viper.SetDefault("server.port", 9000)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.api_port", 9001)
	viper.SetDefault("server.api_host", "0.0.0.0")

	// Logging defaults
	viper.SetDefault("logging.enabled", true)
	viper.SetDefault("logging.dir", "./logs")
	viper.SetDefault("logging.max_size", 100)
	viper.SetDefault("logging.max_backups", 10)
	viper.SetDefault("logging.max_age", 30)

	// Memory defaults
	viper.SetDefault("memory.max_connections", 1000)
	viper.SetDefault("memory.max_request_body_kb", 512)
	viper.SetDefault("memory.max_response_body_kb", 512)
	viper.SetDefault("memory.retention_minutes", 60)

	// Thinking preserve defaults
	viper.SetDefault("thinking_store.max_entries", 50000)
	viper.SetDefault("thinking_store.entry_ttl_minutes", 24*60)
	viper.SetDefault("thinking_store.persist_path", "./data/thinking_store.db")

	// Video protocol adapters share these settings; each gateway carries its
	// own path, upstream and protocol.
	viper.SetDefault("video.store_path", "./data/video_tasks.db")
	viper.SetDefault("video.task_ttl_hours", 72)
	viper.SetDefault("video.create_timeout_seconds", 300)
	viper.SetDefault("video.query_timeout_seconds", 30)
	viper.SetDefault("video.cache_ttl_ms", 1500)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) HasTransformers() bool {
	return len(c.Transformers.Definitions) > 0 || len(c.Transformers.Mappings) > 0
}

func (c *Config) HasTagging() bool {
	return len(c.Tagging.Rules) > 0
}
