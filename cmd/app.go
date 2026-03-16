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
	"github.com/openlist-jav-aio/jav-aio/internal/util"
	"github.com/openlist-jav-aio/jav-aio/internal/ffmpeg"
	"github.com/openlist-jav-aio/jav-aio/internal/llm"
	"github.com/openlist-jav-aio/jav-aio/internal/logger"
	"github.com/openlist-jav-aio/jav-aio/internal/notify"
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
	MinFileBytes      int64 // parsed from Cfg.OpenList.MinFileSize
}

// filterBySize returns only files whose size is >= minBytes. minBytes=0 means no filter.
func filterBySize(files []openlist.FileInfo, minBytes int64) []openlist.FileInfo {
	if minBytes <= 0 {
		return files
	}
	out := files[:0:0]
	for _, f := range files {
		if f.Size >= minBytes {
			out = append(out, f)
		}
	}
	return out
}

// buildApp reads viper config, initializes all components, and returns a ready App.
func buildApp() (*App, error) {
	cfg := config.Default()
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	config.ApplySubDefaults(cfg)

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
	whisperRunner := whisper.NewRunner(
		cfg.Subtitle.WhisperBin,
		cfg.Subtitle.Model,
		cfg.Subtitle.Language,
		whisper.RunnerOptions{
			Sensitivity: cfg.Subtitle.Sensitivity,
			ComputeType: cfg.Subtitle.ComputeType,
			CPUThreads:  cfg.Subtitle.CPUThreads,
		},
		log,
	)

	// subtitle processor
	subtitleProc := subtitle.NewProcessor(
		ffmpegRunner,
		whisperRunner,
		cfg.Subtitle.KeepAudio,
		cfg.Subtitle.KeepAudioMax,
		cfg.Subtitle.AudioDir,
		log,
	)

	// LLM / translation provider
	var llmProvider llm.Provider
	switch cfg.Translate.Provider {
	case "deeplx":
		llmProvider = llm.NewDeepLXProvider(cfg.Translate.DeepLX.BaseURL, cfg.Translate.DeepLX.SourceLang, log)
	case "ollama":
		llmProvider = llm.NewOllamaProvider(cfg.Translate.Ollama.BaseURL, cfg.Translate.Ollama.Model, log)
	default: // "openai" and any OpenAI-compatible provider
		llmProvider = llm.NewOpenAIProvider(
			cfg.Translate.OpenAI.BaseURL,
			cfg.Translate.OpenAI.APIKey,
			cfg.Translate.OpenAI.Model,
			cfg.Translate.MaxTokens,
			log,
		)
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
		tmpFile := destPath + ".tmp"
		if err := os.WriteFile(tmpFile, []byte(translated), 0644); err != nil {
			return err
		}
		return util.AtomicRename(tmpFile, destPath)
	}

	var notifyFunc func(ctx context.Context, task pipeline.Task, srtPath string)
	if cfg.Notify.Enabled && cfg.Notify.URL != "" {
		notifier := notify.New(cfg.Notify.URL, cfg.Notify.Headers, log)
		notifyFunc = func(ctx context.Context, task pipeline.Task, srtPath string) {
			notifier.Send(ctx, task.JavID, task.OpenListPath, srtPath)
		}
	}

	pl := pipeline.New(pipeline.Deps{
		DB:          db,
		Steps:       cfg.Pipeline.Steps,
		RetryConfig: cfg.Retry,
		Log:         log,
		ScrapeFunc:    scrapeFunc,
		STRMFunc:      strmFunc,
		SubtitleFunc:  subtitleFunc,
		TranslateFunc: translateFunc,
		TargetLang:    cfg.Translate.TargetLanguage,
		NotifyFunc:    notifyFunc,
	})

	minFileBytes, err := config.ParseSize(cfg.OpenList.MinFileSize)
	if err != nil {
		log.Warn("invalid min_file_size, filter disabled", "value", cfg.OpenList.MinFileSize, "error", err)
		minFileBytes = 0
	}

	return &App{
		Cfg:               cfg,
		Log:               log,
		DB:                db,
		OL:                ol,
		Pipeline:          pl,
		Scraper:           sc,
		STRMFunc:          strmFunc,
		SubtitleProcessor: subtitleProc,
		MinFileBytes:      minFileBytes,
	}, nil
}
