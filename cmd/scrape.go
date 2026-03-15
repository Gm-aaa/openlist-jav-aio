package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var scrapeCmd = &cobra.Command{
	Use:   "scrape <jav-id>",
	Short: "Scrape metadata for a JAV ID directly",
	Args:  cobra.ExactArgs(1),
	RunE:  runScrape,
}

func init() {
	rootCmd.AddCommand(scrapeCmd)
}

func runScrape(cmd *cobra.Command, args []string) error {
	app, err := buildApp()
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}
	defer app.DB.Close()

	ctx := context.Background()
	javID := args[0]
	outDir := filepath.Join(app.Cfg.Output.BaseDir, javID)

	info, err := app.Scraper.Scrape(ctx, javID, outDir, app.Cfg.Scraper.Cover)
	if err != nil {
		return fmt.Errorf("scrape %s: %w", javID, err)
	}

	app.Log.Info("scrape complete", "id", javID, "title", info.Title, "out_dir", outDir)
	return nil
}
