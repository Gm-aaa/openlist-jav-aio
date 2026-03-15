package pipeline_test

import (
	"context"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/config"
	"github.com/openlist-jav-aio/jav-aio/internal/pipeline"
	"github.com/openlist-jav-aio/jav-aio/internal/state"
)

func TestPipeline_SkipsCompletedSteps(t *testing.T) {
	db, _ := state.Open(":memory:")
	defer db.Close()

	db.Upsert(&state.Record{
		OpenListPath:  "/jav/ABC-123.mp4",
		JavID:         "ABC-123",
		ScrapeDone:    true,
		StrmDone:      true,
		SubtitleDone:  true,
		TranslateDone: true,
	})

	scrapeCount := 0
	p := pipeline.New(pipeline.Deps{
		DB:    db,
		Steps: config.StepsConfig{Scrape: true, STRM: true, Subtitle: true, Translate: true},
		ScrapeFunc: func(ctx context.Context, javID, outDir string) error {
			scrapeCount++
			return nil
		},
	})

	err := p.Run(context.Background(), pipeline.Task{
		OpenListPath: "/jav/ABC-123.mp4",
		JavID:        "ABC-123",
		FileURL:      "http://openlist/d/jav/ABC-123.mp4",
		OutDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if scrapeCount != 0 {
		t.Errorf("expected scrape to be skipped, called %d times", scrapeCount)
	}
}
