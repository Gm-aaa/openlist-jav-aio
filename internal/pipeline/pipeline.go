package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/openlist-jav-aio/jav-aio/internal/config"
	"github.com/openlist-jav-aio/jav-aio/internal/state"
)

type Task struct {
	OpenListPath string
	JavID        string
	FileURL      string
	OutDir       string
}

type Deps struct {
	DB            *state.DB
	Steps         config.StepsConfig
	Log           *slog.Logger
	ScrapeFunc    func(ctx context.Context, javID, outDir string) error
	STRMFunc      func(ctx context.Context, javID, outDir, url string) error
	SubtitleFunc  func(ctx context.Context, videoURL, outDir, javID string) error
	TranslateFunc func(ctx context.Context, srtPath, outDir, javID, lang string) error
	TargetLang    string
	// NotifyFunc is called after translate completes successfully (optional).
	NotifyFunc func(ctx context.Context, task Task, srtPath string)
}

type Pipeline struct {
	deps Deps
}

func New(deps Deps) *Pipeline {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Pipeline{deps: deps}
}

func (p *Pipeline) Run(ctx context.Context, task Task) error {
	log := p.deps.Log.With("id", task.JavID, "path", task.OpenListPath)
	log.Info("pipeline start")
	start := time.Now()

	rec, err := p.deps.DB.Get(task.OpenListPath)
	if errors.Is(err, state.ErrNotFound) {
		rec = &state.Record{OpenListPath: task.OpenListPath, JavID: task.JavID}
	} else if err != nil {
		return fmt.Errorf("db get %s: %w", task.OpenListPath, err)
	}

	d := p.deps

	// Helper: persist record and log errors (DB failure should not crash pipeline).
	upsert := func(step string) {
		if err := d.DB.Upsert(rec); err != nil {
			log.Error("db upsert failed", "step", step, "error", err)
		}
	}

	// clearError only clears ErrorMsg if it was set by the given step,
	// so a later step's success doesn't erase a prior step's error.
	clearError := func(step string) {
		if strings.HasPrefix(rec.ErrorMsg, step+":") {
			rec.ErrorMsg = ""
		}
	}

	// Step: Scrape
	if d.Steps.Scrape && !rec.ScrapeDone && d.ScrapeFunc != nil {
		log.Debug("step start", "step", "scrape")
		if err := d.ScrapeFunc(ctx, task.JavID, task.OutDir); err != nil {
			log.Error("scrape failed", "step", "scrape", "error", err)
			rec.ErrorMsg = fmt.Sprintf("scrape: %v", err)
			upsert("scrape")
		} else {
			rec.ScrapeDone = true
			clearError("scrape")
			upsert("scrape")
			log.Debug("step done", "step", "scrape")
		}
	}

	if ctx.Err() != nil {
		log.Info("pipeline cancelled", "duration_ms", time.Since(start).Milliseconds())
		return ctx.Err()
	}

	// Step: STRM
	if d.Steps.STRM && !rec.StrmDone && d.STRMFunc != nil {
		log.Debug("step start", "step", "strm")
		if err := d.STRMFunc(ctx, task.JavID, task.OutDir, task.FileURL); err != nil {
			log.Error("strm failed", "step", "strm", "error", err)
			rec.ErrorMsg = fmt.Sprintf("strm: %v", err)
			upsert("strm")
		} else {
			rec.StrmDone = true
			clearError("strm")
			upsert("strm")
			log.Debug("step done", "step", "strm")
		}
	}

	if ctx.Err() != nil {
		log.Info("pipeline cancelled", "duration_ms", time.Since(start).Milliseconds())
		return ctx.Err()
	}

	// Pre-populate srtPath if subtitle was already completed in a prior run,
	// so the translate step is not skipped due to an empty srtPath.
	var srtPath string
	if rec.SubtitleDone {
		srtPath = filepath.Join(task.OutDir, task.JavID+".srt")
	}

	// Step: Subtitle
	if d.Steps.Subtitle && !rec.SubtitleDone && d.SubtitleFunc != nil {
		log.Debug("step start", "step", "subtitle")
		if err := d.SubtitleFunc(ctx, task.FileURL, task.OutDir, task.JavID); err != nil {
			log.Error("subtitle failed", "step", "subtitle", "error", err)
			rec.ErrorMsg = fmt.Sprintf("subtitle: %v", err)
			upsert("subtitle")
		} else {
			rec.SubtitleDone = true
			clearError("subtitle")
			srtPath = filepath.Join(task.OutDir, task.JavID+".srt")
			upsert("subtitle")
			log.Debug("step done", "step", "subtitle")
		}
	}

	if ctx.Err() != nil {
		log.Info("pipeline cancelled", "duration_ms", time.Since(start).Milliseconds())
		return ctx.Err()
	}

	// Step: Translate
	if d.Steps.Translate && !rec.TranslateDone && srtPath != "" && d.TranslateFunc != nil {
		log.Debug("step start", "step", "translate")
		if err := d.TranslateFunc(ctx, srtPath, task.OutDir, task.JavID, d.TargetLang); err != nil {
			log.Error("translate failed", "step", "translate", "error", err)
			rec.ErrorMsg = fmt.Sprintf("translate: %v", err)
			upsert("translate")
		} else {
			rec.TranslateDone = true
			clearError("translate")
			upsert("translate")
			log.Debug("step done", "step", "translate")
			if d.NotifyFunc != nil {
				d.NotifyFunc(ctx, task, srtPath)
			}
		}
	}

	log.Info("pipeline done", "duration_ms", time.Since(start).Milliseconds())
	return nil
}
