package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openlist-jav-aio/jav-aio/internal/id"
)

var subtitleCmd = &cobra.Command{
	Use:   "subtitle <openlist-path>",
	Short: "Generate subtitle for the given OpenList path",
	Args:  cobra.ExactArgs(1),
	RunE:  runSubtitle,
}

func init() {
	rootCmd.AddCommand(subtitleCmd)
}

func runSubtitle(cmd *cobra.Command, args []string) error {
	app, err := buildApp()
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}
	defer app.DB.Close()

	ctx := context.Background()
	olPath := args[0]

	javID, ok := id.Extract(olPath)
	if !ok {
		return fmt.Errorf("could not extract JAV ID from path: %s", olPath)
	}

	fileURL, err := app.OL.GetFileURL(ctx, olPath, "")
	if err != nil {
		return fmt.Errorf("get file URL for %s: %w", olPath, err)
	}

	outDir := filepath.Join(app.Cfg.Output.BaseDir, javID)

	srtPath, err := app.SubtitleProcessor.Process(ctx, fileURL, outDir, javID)
	if err != nil {
		return fmt.Errorf("subtitle processing for %s: %w", javID, err)
	}

	app.Log.Info("subtitle generated", "id", javID, "srt", srtPath)
	return nil
}
