package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
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
	if err != nil {
		rec = &state.Record{OpenListPath: task.OpenListPath, JavID: task.JavID}
	}

	d := p.deps

	// Step: Scrape
	if d.Steps.Scrape && !rec.ScrapeDone && d.ScrapeFunc != nil {
		log.Debug("step start", "step", "scrape")
		if err := d.ScrapeFunc(ctx, task.JavID, task.OutDir); err != nil {
			log.Error("scrape failed", "step", "scrape", "error", err)
			rec.ErrorMsg = fmt.Sprintf("scrape: %v", err)
			d.DB.Upsert(rec)
		} else {
			rec.ScrapeDone = true
			d.DB.Upsert(rec)
			log.Debug("step done", "step", "scrape")
		}
	}

	// Step: STRM
	if d.Steps.STRM && !rec.StrmDone && d.STRMFunc != nil {
		log.Debug("step start", "step", "strm")
		if err := d.STRMFunc(ctx, task.JavID, task.OutDir, task.FileURL); err != nil {
			log.Error("strm failed", "step", "strm", "error", err)
			rec.ErrorMsg = fmt.Sprintf("strm: %v", err)
			d.DB.Upsert(rec)
		} else {
			rec.StrmDone = true
			d.DB.Upsert(rec)
			log.Debug("step done", "step", "strm")
		}
	}

	// Step: Subtitle
	var srtPath string
	if d.Steps.Subtitle && !rec.SubtitleDone && d.SubtitleFunc != nil {
		log.Debug("step start", "step", "subtitle")
		if err := d.SubtitleFunc(ctx, task.FileURL, task.OutDir, task.JavID); err != nil {
			log.Error("subtitle failed", "step", "subtitle", "error", err)
			rec.ErrorMsg = fmt.Sprintf("subtitle: %v", err)
			d.DB.Upsert(rec)
		} else {
			rec.SubtitleDone = true
			srtPath = filepath.Join(task.OutDir, task.JavID+".srt")
			d.DB.Upsert(rec)
			log.Debug("step done", "step", "subtitle")
		}
	}

	// Step: Translate
	if d.Steps.Translate && !rec.TranslateDone && srtPath != "" && d.TranslateFunc != nil {
		log.Debug("step start", "step", "translate")
		if err := d.TranslateFunc(ctx, srtPath, task.OutDir, task.JavID, d.TargetLang); err != nil {
			log.Error("translate failed", "step", "translate", "error", err)
			rec.ErrorMsg = fmt.Sprintf("translate: %v", err)
			d.DB.Upsert(rec)
		} else {
			rec.TranslateDone = true
			d.DB.Upsert(rec)
			log.Debug("step done", "step", "translate")
		}
	}

	log.Info("pipeline done", "duration_ms", time.Since(start).Milliseconds())
	return nil
}
