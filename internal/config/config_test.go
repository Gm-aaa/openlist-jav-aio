package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg := config.Default()
	if cfg.Pipeline.PollInterval != "1h" {
		t.Errorf("expected default poll interval 1h, got %s", cfg.Pipeline.PollInterval)
	}
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("expected default retry attempts 3, got %d", cfg.Retry.MaxAttempts)
	}
	if cfg.Subtitle.KeepAudioMax != 5 {
		t.Errorf("expected default keep_audio_max 5, got %d", cfg.Subtitle.KeepAudioMax)
	}
	if cfg.Subtitle.Model != "medium" {
		t.Errorf("expected default subtitle model 'medium', got %s", cfg.Subtitle.Model)
	}
	if cfg.Subtitle.Language != "ja" {
		t.Errorf("expected default subtitle language 'ja', got %s", cfg.Subtitle.Language)
	}
}

func TestLoadFromYAML(t *testing.T) {
	yaml := `
openlist:
  base_url: "http://test:5244"
  token: "tok"
output:
  base_dir: "/tmp/out"
`
	f := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(f, []byte(yaml), 0644)

	cfg, err := config.LoadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenList.BaseURL != "http://test:5244" {
		t.Errorf("unexpected base_url: %s", cfg.OpenList.BaseURL)
	}
	// Defaults should still apply for unset fields
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("expected default retry, got %d", cfg.Retry.MaxAttempts)
	}
}

func TestValidate_MissingRequired(t *testing.T) {
	cfg := config.Default()
	// Missing base_url and token
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for missing openlist.base_url")
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := config.Default()
	cfg.OpenList.BaseURL = "http://openlist:5244"
	cfg.OpenList.Token = "tok"
	cfg.Output.BaseDir = "/tmp/out"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}
