package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openlist-jav-aio/jav-aio/internal/config"
	"github.com/openlist-jav-aio/jav-aio/internal/logger"
	"github.com/openlist-jav-aio/jav-aio/internal/whisper"
	"github.com/spf13/viper"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage Whisper model files",
}

var modelDownloadCmd = &cobra.Command{
	Use:   "download [model]",
	Short: "Download a faster-whisper model from Hugging Face",
	Long: `Download the specified faster-whisper model (or the one in config) so it is
cached locally before running the subtitle pipeline.

Short names are resolved to their Hugging Face repo automatically:
  tiny      → Systran/faster-whisper-tiny
  base      → Systran/faster-whisper-base
  small     → Systran/faster-whisper-small
  medium    → Systran/faster-whisper-medium
  large-v3  → Systran/faster-whisper-large-v3

Full HuggingFace repo IDs are also accepted:
  litagin/anime-whisper
  kotoba-whisper-v2

Requires Python with huggingface_hub installed (pip install huggingface_hub).
The model is cached in the default HuggingFace cache directory, which
faster-whisper (and WhisperJAV) reads automatically.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runModelDownload,
}

func init() {
	modelCmd.AddCommand(modelDownloadCmd)
	rootCmd.AddCommand(modelCmd)
}

func runModelDownload(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}
	config.ApplySubDefaults(cfg)

	log := logger.New(cfg.Log.Level, cfg.Log.Format, cfg.Log.File)

	model := cfg.Subtitle.Model
	if len(args) == 1 {
		model = args[0]
	}

	pythonBin := cfg.Subtitle.PythonBin

	fmt.Printf("Downloading model %q via %s ...\n", model, pythonBin)
	fmt.Printf("Hugging Face repo: %s\n\n", whisper.RepoID(model))

	if err := whisper.DownloadModel(context.Background(), pythonBin, model, log); err != nil {
		return err
	}

	fmt.Printf("\nModel %q downloaded successfully.\n", model)
	return nil
}
