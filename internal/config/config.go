package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	OpenList  OpenListConfig  `mapstructure:"openlist"`
	Output    OutputConfig    `mapstructure:"output"`
	Pipeline  PipelineConfig  `mapstructure:"pipeline"`
	Retry     RetryConfig     `mapstructure:"retry"`
	Scraper   ScraperConfig   `mapstructure:"scraper"`
	Subtitle  SubtitleConfig  `mapstructure:"subtitle"`
	Translate TranslateConfig `mapstructure:"translate"`
	Webhook   WebhookConfig   `mapstructure:"webhook"`
	Log       LogConfig       `mapstructure:"log"`
	State     StateConfig     `mapstructure:"state"`
}

type OpenListConfig struct {
	BaseURL        string       `mapstructure:"base_url"`
	Token          string       `mapstructure:"token"`
	ScanPaths      []string     `mapstructure:"scan_paths"`
	ScanExtensions []string     `mapstructure:"scan_extensions"`
	RequestDelay   RequestDelay `mapstructure:"request_delay"`
}

type RequestDelay struct {
	Min string `mapstructure:"min"`
	Max string `mapstructure:"max"`
}

type OutputConfig struct {
	BaseDir string `mapstructure:"base_dir"`
}

type PipelineConfig struct {
	PollInterval string      `mapstructure:"poll_interval"`
	Steps        StepsConfig `mapstructure:"steps"`
}

type StepsConfig struct {
	IDExtract bool `mapstructure:"id_extract"`
	Scrape    bool `mapstructure:"scrape"`
	STRM      bool `mapstructure:"strm"`
	Subtitle  bool `mapstructure:"subtitle"`
	Translate bool `mapstructure:"translate"`
}

type RetryConfig struct {
	MaxAttempts int    `mapstructure:"max_attempts"`
	BaseDelay   string `mapstructure:"base_delay"`
	MaxDelay    string `mapstructure:"max_delay"`
	Jitter      bool   `mapstructure:"jitter"`
}

type ScraperConfig struct {
	PreferredSources []string `mapstructure:"preferred_sources"`
	Language         string   `mapstructure:"language"`
	Cover            bool     `mapstructure:"cover"`
}

type SubtitleConfig struct {
	WhisperBin     string `mapstructure:"whisper_bin"`
	Model          string `mapstructure:"model"`
	Language       string `mapstructure:"language"`
	FFmpegCacheDir string `mapstructure:"ffmpeg_cache_dir"`
	KeepAudio      bool   `mapstructure:"keep_audio"`
	KeepAudioMax   int    `mapstructure:"keep_audio_max"`
	AudioDir       string `mapstructure:"audio_dir"`
}

type TranslateConfig struct {
	TargetLanguage string       `mapstructure:"target_language"`
	Provider       string       `mapstructure:"provider"`
	OpenAI         OpenAIConfig `mapstructure:"openai"`
	Ollama         OllamaConfig `mapstructure:"ollama"`
}

type OpenAIConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

type OllamaConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

type WebhookConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Secret  string `mapstructure:"secret"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	File   string `mapstructure:"file"`
	Format string `mapstructure:"format"` // "text" (default) | "json"
}

type StateConfig struct {
	DBPath string `mapstructure:"db_path"`
}

// LoadFile reads a YAML config file and merges it with defaults.
func LoadFile(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	cfg := Default()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	ApplySubDefaults(cfg)
	return cfg, nil
}

// ApplySubDefaults fills in zero-value fields in sub-structs from known defaults.
// This handles the case where a partial YAML block (e.g., subtitle: {whisper_bin: ...})
// causes viper to overwrite the entire sub-struct, zeroing fields not present in YAML.
func ApplySubDefaults(cfg *Config) {
	d := Default()
	if cfg.Subtitle.Model == "" {
		cfg.Subtitle.Model = d.Subtitle.Model
	}
	if cfg.Subtitle.Language == "" {
		cfg.Subtitle.Language = d.Subtitle.Language
	}
	if cfg.Subtitle.KeepAudioMax == 0 {
		cfg.Subtitle.KeepAudioMax = d.Subtitle.KeepAudioMax
	}
	if cfg.Retry.MaxAttempts == 0 {
		cfg.Retry.MaxAttempts = d.Retry.MaxAttempts
	}
	if cfg.Retry.BaseDelay == "" {
		cfg.Retry.BaseDelay = d.Retry.BaseDelay
	}
	if cfg.Retry.MaxDelay == "" {
		cfg.Retry.MaxDelay = d.Retry.MaxDelay
	}
	if cfg.Pipeline.PollInterval == "" {
		cfg.Pipeline.PollInterval = d.Pipeline.PollInterval
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = d.Log.Level
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = d.Log.Format
	}
	if cfg.State.DBPath == "" {
		cfg.State.DBPath = d.State.DBPath
	}
	if cfg.Translate.TargetLanguage == "" {
		cfg.Translate.TargetLanguage = d.Translate.TargetLanguage
	}
	if cfg.Translate.Provider == "" {
		cfg.Translate.Provider = d.Translate.Provider
	}
	if cfg.Translate.OpenAI.BaseURL == "" {
		cfg.Translate.OpenAI.BaseURL = d.Translate.OpenAI.BaseURL
	}
	if cfg.Translate.OpenAI.Model == "" {
		cfg.Translate.OpenAI.Model = d.Translate.OpenAI.Model
	}
	if cfg.Translate.Ollama.BaseURL == "" {
		cfg.Translate.Ollama.BaseURL = d.Translate.Ollama.BaseURL
	}
	if cfg.Translate.Ollama.Model == "" {
		cfg.Translate.Ollama.Model = d.Translate.Ollama.Model
	}
	if cfg.Webhook.Port == 0 {
		cfg.Webhook.Port = d.Webhook.Port
	}
}

// Validate checks required fields are set.
func (c *Config) Validate() error {
	if c.OpenList.BaseURL == "" {
		return fmt.Errorf("openlist.base_url is required")
	}
	if c.OpenList.Token == "" {
		return fmt.Errorf("openlist.token is required")
	}
	if c.Output.BaseDir == "" {
		return fmt.Errorf("output.base_dir is required")
	}
	return nil
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		OpenList: OpenListConfig{
			ScanExtensions: []string{".mp4", ".mkv", ".avi"},
			RequestDelay:   RequestDelay{Min: "500ms", Max: "2s"},
		},
		Pipeline: PipelineConfig{
			PollInterval: "1h",
			Steps: StepsConfig{
				IDExtract: true, Scrape: true, STRM: true,
				Subtitle: true, Translate: true,
			},
		},
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   "2s",
			MaxDelay:    "30s",
			Jitter:      true,
		},
		Scraper: ScraperConfig{
			PreferredSources: []string{"javdb", "javbus"},
			Language:         "zh",
			Cover:            true,
		},
		Subtitle: SubtitleConfig{
			Model:        "medium",
			Language:     "ja",
			KeepAudio:    false,
			KeepAudioMax: 5,
		},
		Translate: TranslateConfig{
			TargetLanguage: "zh",
			Provider:       "openai",
			OpenAI:         OpenAIConfig{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
			Ollama:         OllamaConfig{BaseURL: "http://localhost:11434", Model: "qwen2.5:7b"},
		},
		Webhook: WebhookConfig{Port: 8080},
		Log:     LogConfig{Level: "debug", Format: "text"},
		State:   StateConfig{DBPath: "./jav-aio.db"},
	}
}
