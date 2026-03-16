package whisper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// modelRepoID maps short model names to their Hugging Face repository IDs.
// Models not in this map are assumed to already be a full HuggingFace repo ID
// (e.g. "litagin/anime-whisper", "kotoba-wisper-v2").
var modelRepoID = map[string]string{
	"tiny":      "Systran/faster-whisper-tiny",
	"base":      "Systran/faster-whisper-base",
	"small":     "Systran/faster-whisper-small",
	"medium":    "Systran/faster-whisper-medium",
	"large":     "Systran/faster-whisper-large-v2",
	"large-v2":  "Systran/faster-whisper-large-v2",
	"large-v3":  "Systran/faster-whisper-large-v3",
}

// RepoID returns the Hugging Face repository ID for a model name.
// Short names like "medium" are expanded; full repo IDs are returned as-is.
func RepoID(model string) string {
	if repo, ok := modelRepoID[strings.ToLower(model)]; ok {
		return repo
	}
	return model // already a HuggingFace repo ID (e.g. "litagin/anime-whisper")
}

// DownloadModel downloads the faster-whisper model via Python's huggingface_hub.
// pythonBin is the Python executable (e.g. "python3").
// model is the Whisper model name or HuggingFace repo ID.
// Progress is printed to stdout via the Python script; log receives structured events.
func DownloadModel(ctx context.Context, pythonBin, model string, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	repo := RepoID(model)
	log.Info("downloading whisper model", "model", model, "repo", repo)

	// Use huggingface_hub.snapshot_download which shows a progress bar and
	// caches the result in the same location that faster-whisper uses.
	script := fmt.Sprintf(
		"from huggingface_hub import snapshot_download; "+
			"snapshot_download(%q, repo_type='model')",
		repo,
	)
	cmd := exec.CommandContext(ctx, pythonBin, "-c", script)
	// Stream stdout/stderr to the terminal so the user sees the download
	// progress bar (huggingface_hub prints it to stderr).
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download model %q (repo %s): %w", model, repo, err)
	}
	log.Info("model download complete", "model", model, "repo", repo)
	return nil
}
