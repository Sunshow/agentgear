package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadVideoDefaults(t *testing.T) {
	path := writeConfig(t, `
video:
  store_path: /tmp/video_tasks.db
gateways:
  - name: orlucky-doubao
    path: /doubao
    upstream: http://127.0.0.1:19080
    protocol: doubao
    enabled: true
    video:
      models:
        doubao-seedance-2-0-mini-260615: seedance-mini
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Video.StorePath != "/tmp/video_tasks.db" {
		t.Fatalf("store path = %q", cfg.Video.StorePath)
	}
	if cfg.Video.TaskTTLHours != 72 || cfg.Video.CreateTimeoutSeconds != 300 ||
		cfg.Video.QueryTimeoutSeconds != 30 || cfg.Video.CacheTTLMs != 1500 {
		t.Fatalf("unexpected video defaults: %#v", cfg.Video)
	}
	if len(cfg.Gateways) != 1 {
		t.Fatalf("gateways = %#v", cfg.Gateways)
	}
	gateway := cfg.Gateways[0]
	if gateway.Protocol != "doubao" {
		t.Fatalf("protocol = %q", gateway.Protocol)
	}
	if gateway.Video == nil || gateway.Video.Models["doubao-seedance-2-0-mini-260615"] != "seedance-mini" {
		t.Fatalf("video models = %#v", gateway.Video)
	}
}

func TestLoadVideoGateways(t *testing.T) {
	path := writeConfig(t, `
video:
  api_key: sk-global
  public_base_url: http://agentgear:9000
gateways:
  - name: orlucky-doubao
    path: /doubao
    upstream: https://router.orlucky.com/model/api
    protocol: doubao
    enabled: true
  - name: orlucky-ali
    path: /ali
    upstream: https://router.orlucky.com/model/api
    protocol: ali
    enabled: false
    video:
      api_key: sk-ali
      models:
        wan3.0-video: wan-3.0
  - name: anthropic
    path: /v1
    upstream: https://api.anthropic.com
    enabled: true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Video.APIKey != "sk-global" || cfg.Video.PublicBaseURL != "http://agentgear:9000" {
		t.Fatalf("unexpected shared settings: %#v", cfg.Video)
	}
	if len(cfg.Gateways) != 3 {
		t.Fatalf("gateways = %#v", cfg.Gateways)
	}
	ali := cfg.Gateways[1]
	if ali.Protocol != "ali" || ali.Enabled {
		t.Fatalf("ali gateway = %#v", ali)
	}
	if ali.Video == nil || ali.Video.APIKey != "sk-ali" || ali.Video.Models["wan3.0-video"] != "wan-3.0" {
		t.Fatalf("ali video settings = %#v", ali.Video)
	}
	if cfg.Gateways[2].Protocol != "" {
		t.Fatalf("transparent gateway protocol = %q", cfg.Gateways[2].Protocol)
	}
}
