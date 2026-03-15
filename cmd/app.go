package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"

	"github.com/openlist-jav-aio/jav-aio/internal/config"
	"github.com/openlist-jav-aio/jav-aio/internal/ffmpeg"
	"github.com/openlist-jav-aio/jav-aio/internal/llm"
	"github.com/openlist-jav-aio/jav-aio/internal/logger"
	"github.com/openlist-jav-aio/jav-aio/internal/openlist"
	"github.com/openlist-jav-aio/jav-aio/internal/pipeline"
	"github.com/openlist-jav-aio/jav-aio/internal/scraper"
	"github.com/openlist-jav-aio/jav-aio/internal/state"
	"github.com/openlist-jav-aio/jav-aio/internal/strm"
	"github.com/openlist-jav-aio/jav-aio/internal/subtitle"
	"github.com/openlist-jav-aio/jav-aio/internal/whisper"
)

// App holds all initialized components for the CLI commands.
type App struct {
	Cfg               *config.Config
	Log               *slog.Logger
	DB                *state.DB
	OL                *openlist.Client
	Pipeline          *pipeline.Pipeline
	Scraper           *scraper.Scraper
	STRMFunc          func(ctx context.Context, javID, outDir, url string) error
	SubtitleProcessor *subtitle.Processor
}

// buildApp reads viper config, initializes all components, and returns a ready App.
func buildApp() (*App, error) {
	cfg := config.Default()
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Apply sub-defaults manually (mirrors config.applySubDefaults which is unexported).
	d := config.Default()
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

	log := logger.New(cfg.Log.Level, cfg.Log.Format, cfg.Log.File)

	// State DB
	db, err := state.Open(cfg.State.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}

	// Request delay
	delayMin, err := time.ParseDuration(cfg.OpenList.RequestDelay.Min)
	if err != nil {
		delayMin = 500 * time.Millisecond
	}
	delayMax, err := time.ParseDuration(cfg.OpenList.RequestDelay.Max)
	if err != nil {
		delayMax = 2 * time.Second
	}

	// OpenList client
	ol := openlist.NewClient(cfg.OpenList.BaseURL, cfg.OpenList.Token, openlist.RequestDelay{
		Min: delayMin,
		Max: delayMax,
	}).WithLogger(log)

	// Scraper
	sc, err := scraper.New(cfg.Scraper.PreferredSources, cfg.Scraper.Language, log)
	if err != nil {
		return nil, fmt.Errorf("init scraper: %w", err)
	}

	// ffmpeg runner (optional — warn if unavailable)
	var ffmpegRunner *ffmpeg.Runner
	ffmpegRunner, err = ffmpeg.NewRunner(cfg.Subtitle.FFmpegCacheDir, log)
	if err != nil {
		log.Warn("ffmpeg runner unavailable; subtitle step will fail at runtime", "error", err)
		ffmpegRunner = nil
	}

	// whisper runner
	whisperRunner := whisper.NewRunner(cfg.Subtitle.WhisperBin, cfg.Subtitle.Model, cfg.Subtitle.Language, log)

	// subtitle processor
	subtitleProc := subtitle.NewProcessor(
		ffmpegRunner,
		whisperRunner,
		cfg.Subtitle.KeepAudio,
		cfg.Subtitle.KeepAudioMax,
		cfg.Subtitle.AudioDir,
		log,
	)

	// LLM provider
	var llmProvider llm.Provider
	switch cfg.Translate.Provider {
	case "ollama":
		llmProvider = llm.NewOllamaProvider(cfg.Translate.Ollama.BaseURL, cfg.Translate.Ollama.Model, log)
	default:
		llmProvider = llm.NewOpenAIProvider(cfg.Translate.OpenAI.BaseURL, cfg.Translate.OpenAI.APIKey, cfg.Translate.OpenAI.Model, log)
	}

	coverEnabled := cfg.Scraper.Cover
	scrapeFunc := func(ctx context.Context, javID, outDir string) error {
		_, err := sc.Scrape(ctx, javID, outDir, coverEnabled)
		return err
	}

	strmFunc := func(ctx context.Context, javID, outDir, url string) error {
		return strm.Generate(outDir, javID, url)
	}

	subtitleFunc := func(ctx context.Context, videoURL, outDir, javID string) error {
		if ffmpegRunner == nil {
			return errors.New("ffmpeg runner not available")
		}
		_, err := subtitleProc.Process(ctx, videoURL, outDir, javID)
		return err
	}

	translateFunc := func(ctx context.Context, srtPath, outDir, javID, lang string) error {
		srtData, err := os.ReadFile(srtPath)
		if err != nil {
			return fmt.Errorf("read srt %s: %w", srtPath, err)
		}
		translated, err := llmProvider.Translate(ctx, string(srtData), lang)
		if err != nil {
			return err
		}
		destPath := filepath.Join(outDir, javID+"."+lang+".srt")
		return os.WriteFile(destPath, []byte(translated), 0644)
	}

	pl := pipeline.New(pipeline.Deps{
		DB:    db,
		Steps: cfg.Pipeline.Steps,
		Log:   log,
		ScrapeFunc:    scrapeFunc,
		STRMFunc:      strmFunc,
		SubtitleFunc:  subtitleFunc,
		TranslateFunc: translateFunc,
		TargetLang:    cfg.Translate.TargetLanguage,
	})

	return &App{
		Cfg:               cfg,
		Log:               log,
		DB:                db,
		OL:                ol,
		Pipeline:          pl,
		Scraper:           sc,
		STRMFunc:          strmFunc,
		SubtitleProcessor: subtitleProc,
	}, nil
}
