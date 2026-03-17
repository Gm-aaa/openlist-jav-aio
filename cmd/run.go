package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openlist-jav-aio/jav-aio/internal/id"
	"github.com/openlist-jav-aio/jav-aio/internal/pipeline"
	"github.com/openlist-jav-aio/jav-aio/internal/state"
)

var runCmd = &cobra.Command{
	Use:   "run [openlist-path]",
	Short: "Process all files under an OpenList directory path",
	Args:  cobra.ExactArgs(1),
	RunE:  runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	app, err := buildApp()
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}
	defer app.DB.Close()

	ctx := context.Background()
	dirPath := args[0]
	cfg := app.Cfg

	enabledSteps := state.EnabledSteps{
		Scrape:    cfg.Pipeline.Steps.Scrape,
		STRM:      cfg.Pipeline.Steps.STRM,
		Subtitle:  cfg.Pipeline.Steps.Subtitle,
		Translate: cfg.Pipeline.Steps.Translate,
	}

	files, err := app.OL.ListFiles(ctx, dirPath, cfg.OpenList.ScanExtensions)
	if err != nil {
		return fmt.Errorf("list files at %s: %w", dirPath, err)
	}
	files = filterBySize(files, app.MinFileBytes)

	app.Log.Info("processing files", "count", len(files), "path", dirPath)

	for _, f := range files {
		javID, ok := id.Extract(f.Name)
		if !ok {
			app.Log.Warn("could not extract JAV ID, skipping", "file", f.Name)
			continue
		}

		// Skip files that have already completed all enabled steps.
		if app.DB.IsComplete(f.Path, enabledSteps) {
			app.Log.Debug("run: skipping completed file", "path", f.Path)
			continue
		}

		fileURL, err := app.OL.GetFileURL(ctx, f.Path, f.Sign)
		if err != nil {
			app.Log.Warn("get file URL failed, skipping", "file", f.Name, "error", err)
			continue
		}

		outDir := filepath.Join(cfg.Output.BaseDir, javID)

		task := pipeline.Task{
			OpenListPath: f.Path,
			JavID:        javID,
			Sign:         f.Sign,
			FileURL:      fileURL,
			OutDir:       outDir,
		}
		if err := app.Pipeline.Run(ctx, task); err != nil {
			app.Log.Error("pipeline error", "id", javID, "error", err)
		}
	}

	return nil
}
