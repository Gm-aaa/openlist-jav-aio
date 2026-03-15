package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openlist-jav-aio/jav-aio/internal/id"
	"github.com/openlist-jav-aio/jav-aio/internal/strm"
)

var strmCmd = &cobra.Command{
	Use:   "strm <openlist-path>",
	Short: "Generate a .strm file for the given OpenList path",
	Args:  cobra.ExactArgs(1),
	RunE:  runStrm,
}

func init() {
	rootCmd.AddCommand(strmCmd)
}

func runStrm(cmd *cobra.Command, args []string) error {
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

	if err := strm.Generate(outDir, javID, fileURL); err != nil {
		return fmt.Errorf("generate strm for %s: %w", javID, err)
	}

	app.Log.Info("strm generated", "id", javID, "out_dir", outDir)
	return nil
}
