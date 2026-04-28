package config

import (
	"strings"

	"github.com/spf13/viper"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
)

type Config struct {
	Server        ServerConfig               `mapstructure:"server"`
	Gateways      []GatewayConfig            `mapstructure:"gateways"`
	Logging       LoggingConfig              `mapstructure:"logging"`
	Memory        memory.StoreConfig         `mapstructure:"memory"`
	ThinkingStore memory.ThinkingStoreConfig `mapstructure:"thinking_store"`
	Tagging       tagging.Config             `mapstructure:"tagging"`
	Transformers  transformer.Config         `mapstructure:"transformers"`
}

type ServerConfig struct {
	Port    int    `mapstructure:"port"`
	Host    string `mapstructure:"host"`
	APIPort int    `mapstructure:"api_port"`
	APIHost string `mapstructure:"api_host"`
}

type GatewayConfig struct {
	Name     string `mapstructure:"name" json:"name"`
	Path     string `mapstructure:"path" json:"path"`
	Upstream string `mapstructure:"upstream" json:"upstream"`
	Type     string `mapstructure:"type" json:"type"`
	Timeout  int    `mapstructure:"timeout" json:"timeout"`
	Enabled  bool   `mapstructure:"enabled" json:"enabled"`
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
	viper.SetDefault("thinking_store.max_entries", 5000)
	viper.SetDefault("thinking_store.entry_ttl_minutes", 24*60)
	viper.SetDefault("thinking_store.persist_path", "./data/thinking_store.json")

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
