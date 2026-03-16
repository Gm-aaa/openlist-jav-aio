package config

import (
	"fmt"
	"strconv"
	"strings"

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
	Notify    NotifyConfig    `mapstructure:"notify"`
	Log       LogConfig       `mapstructure:"log"`
	State     StateConfig     `mapstructure:"state"`
}

type OpenListConfig struct {
	BaseURL        string       `mapstructure:"base_url"`
	Token          string       `mapstructure:"token"`
	ScanPaths      []string     `mapstructure:"scan_paths"`
	ScanExtensions []string     `mapstructure:"scan_extensions"`
	RequestDelay   RequestDelay `mapstructure:"request_delay"`
	MinFileSize    string       `mapstructure:"min_file_size"` // e.g. "500MB", "1G"; "" or "0" = no filter
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
	PythonBin      string `mapstructure:"python_bin"`    // python executable used by "jav-aio model download"
	Model          string `mapstructure:"model"`
	Language       string `mapstructure:"language"`
	Sensitivity    string `mapstructure:"sensitivity"`   // "" = WhisperJAV default; "aggressive" / "conservative" / "balanced"
	ComputeType    string `mapstructure:"compute_type"`  // "" = WhisperJAV default; e.g. "int8_float32" for CPU
	CPUThreads     int    `mapstructure:"cpu_threads"`   // 0 = WhisperJAV default (1); set to vCPU count for full utilisation
	FFmpegCacheDir string `mapstructure:"ffmpeg_cache_dir"`
	KeepAudio      bool   `mapstructure:"keep_audio"`
	KeepAudioMax   int    `mapstructure:"keep_audio_max"`
	AudioDir       string `mapstructure:"audio_dir"`
}

type TranslateConfig struct {
	TargetLanguage string       `mapstructure:"target_language"`
	Provider       string       `mapstructure:"provider"`
	MaxTokens      int          `mapstructure:"max_tokens"` // LLM output token cap; 0 = use API default
	OpenAI         OpenAIConfig `mapstructure:"openai"`
	Ollama         OllamaConfig `mapstructure:"ollama"`
	DeepLX         DeepLXConfig `mapstructure:"deeplx"`
}

type DeepLXConfig struct {
	BaseURL    string `mapstructure:"base_url"`
	SourceLang string `mapstructure:"source_lang"` // e.g. "JA"; "" = auto-detect
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

// NotifyConfig controls outgoing webhook notifications fired after translate completes.
type NotifyConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	URL     string            `mapstructure:"url"`     // POST target
	Headers map[string]string `mapstructure:"headers"` // optional extra headers
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
	if cfg.Subtitle.PythonBin == "" {
		cfg.Subtitle.PythonBin = d.Subtitle.PythonBin
	}
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
	if cfg.Translate.DeepLX.BaseURL == "" {
		cfg.Translate.DeepLX.BaseURL = d.Translate.DeepLX.BaseURL
	}
	if cfg.Translate.DeepLX.SourceLang == "" {
		cfg.Translate.DeepLX.SourceLang = d.Translate.DeepLX.SourceLang
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
			PythonBin:    "python3",
			Model:        "medium",
			Language:     "ja",
			Sensitivity:  "",
			ComputeType:  "",
			CPUThreads:   0,
			KeepAudio:    false,
			KeepAudioMax: 5,
		},
		Translate: TranslateConfig{
			TargetLanguage: "zh",
			Provider:       "openai",
			MaxTokens:      0,
			OpenAI:         OpenAIConfig{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
			Ollama:         OllamaConfig{BaseURL: "http://localhost:11434", Model: "qwen2.5:7b"},
			DeepLX:         DeepLXConfig{BaseURL: "http://localhost:1188", SourceLang: "JA"},
		},
		Webhook: WebhookConfig{Port: 8080},
		Log:     LogConfig{Level: "debug", Format: "text"},
		State:   StateConfig{DBPath: "./jav-aio.db"},
	}
}

// ParseSize parses a human-readable size string into bytes.
// Supported units (case-insensitive): B, K/KB, M/MB, G/GB, T/TB.
// A bare number is treated as bytes. Empty string or "0" returns 0.
// Examples: "500MB" → 524288000, "1.5G" → 1610612736, "100" → 100.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}

	// Split numeric part from unit.
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	numStr := strings.TrimSpace(s[:i])
	unit := strings.ToUpper(strings.TrimSpace(s[i:]))

	multipliers := map[string]int64{
		"":   1,
		"B":  1,
		"K":  1024,
		"KB": 1024,
		"M":  1024 * 1024,
		"MB": 1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"T":  1024 * 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}
	mult, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unknown size unit %q in %q", unit, s)
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return int64(val * float64(mult)), nil
}
