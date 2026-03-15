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
	if err := os.WriteFile(f, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

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

func TestLoadFromYAML_PartialSubBlock(t *testing.T) {
	// Only set whisper_bin in subtitle block; other subtitle defaults should be preserved.
	yaml := `
openlist:
  base_url: "http://test:5244"
  token: "tok"
output:
  base_dir: "/tmp/out"
subtitle:
  whisper_bin: "/opt/whisper"
`
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Subtitle.WhisperBin != "/opt/whisper" {
		t.Errorf("expected whisper_bin=/opt/whisper, got %s", cfg.Subtitle.WhisperBin)
	}
	if cfg.Subtitle.Model != "medium" {
		t.Errorf("expected default model=medium preserved, got %q", cfg.Subtitle.Model)
	}
	if cfg.Subtitle.Language != "ja" {
		t.Errorf("expected default language=ja preserved, got %q", cfg.Subtitle.Language)
	}
	if cfg.Subtitle.KeepAudioMax != 5 {
		t.Errorf("expected default keep_audio_max=5 preserved, got %d", cfg.Subtitle.KeepAudioMax)
	}
}

func TestValidate_MissingRequired(t *testing.T) {
	cfg := config.Default()
	// Missing base_url and token
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for missing openlist.base_url")
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"100", 100, false},
		{"1K", 1024, false},
		{"1KB", 1024, false},
		{"5M", 5 * 1024 * 1024, false},
		{"500MB", 500 * 1024 * 1024, false},
		{"1G", 1024 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1.5G", int64(1.5 * 1024 * 1024 * 1024), false},
		{"2T", 2 * 1024 * 1024 * 1024 * 1024, false},
		{"5mb", 5 * 1024 * 1024, false}, // case-insensitive
		{"badunit", 0, true},
		{"notanum MB", 0, true},
	}
	for _, c := range cases {
		got, err := config.ParseSize(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseSize(%q): expected error, got nil", c.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSize(%q): unexpected error: %v", c.input, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q): got %d, want %d", c.input, got, c.want)
		}
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
