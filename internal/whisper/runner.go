// Package whisper wraps the WhisperJAV CLI (github.com/meizhong986/WhisperJAV)
// to generate SRT subtitle files from audio or video inputs.
//
// # WhisperJAV CLI investigation summary
//
// Repository: https://github.com/meizhong986/WhisperJAV (v1.8.x as of 2026-03)
// Note: the originally specified repo "YubikoSecurity/whisperJAV" does not exist;
// the canonical project is "meizhong986/WhisperJAV".
//
// Audio input support: YES. WhisperJAV accepts any format that FFmpeg can decode,
// including .aac, .mp3, .wav, .flac, .m4a, and video containers (.mp4, .mkv, etc.).
// Passing a pre-extracted .aac file is fully supported and avoids transmitting video
// data over the network when the audio is already local.
//
// CLI flags used by this runner:
//
//	whisperjav <input_file>          — positional audio/video path
//	    --model          <name>      — Whisper model (tiny/base/small/medium/large-v3;
//	                                   also accepts litagin/anime-whisper, kotoba-v2)
//	    --language       <lang>      — BCP-47 language code, e.g. "ja"
//	    --output-format  srt         — emit SRT (also supports vtt, both)
//	    --output-dir     <dir>       — directory to write the subtitle file
//
// Output file naming: WhisperJAV writes <stem_of_input>.<ext>; this runner
// renames the result to <javID>.srt via the SRTPath helper so callers always
// get a predictable path regardless of the original audio filename.
package whisper

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner wraps the WhisperJAV CLI binary.
type Runner struct {
	bin      string
	model    string
	language string
	log      *slog.Logger
}

// NewRunner creates a Runner.
//   - bin:      absolute path (or name on $PATH) of the whisperjav executable.
//   - model:    Whisper model name, e.g. "medium", "large-v3", "litagin/anime-whisper".
//   - language: BCP-47 code, e.g. "ja".
//   - log:      structured logger; uses slog.Default() when nil.
func NewRunner(bin, model, language string, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{bin: bin, model: model, language: language, log: log}
}

// SRTPath returns the expected output SRT path for a given javID inside outDir.
func SRTPath(outDir, javID string) string {
	return filepath.Join(outDir, javID+".srt")
}

// Transcribe runs WhisperJAV on audioPath and writes a subtitle file to outDir.
// The output file is renamed to <javID>.srt regardless of the audio filename.
//
// audioPath must be a local file path to a pre-extracted audio file (.aac, .mp3, etc.).
// WhisperJAV accepts any audio format that FFmpeg can decode.
//
// ctx may be nil; a background context is used in that case.
func (r *Runner) Transcribe(ctx context.Context, audioPath, outDir, javID string) (string, error) {
	// Verify the binary exists before attempting execution.
	if _, err := os.Stat(r.bin); err != nil {
		return "", fmt.Errorf("whisperJAV binary not found at %s: %w", r.bin, err)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("create outDir: %w", err)
	}

	srtPath := SRTPath(outDir, javID)
	start := time.Now()
	r.log.Debug("whisperJAV transcribe",
		"audio", audioPath,
		"model", r.model,
		"lang", r.language,
		"out", srtPath,
	)

	// WhisperJAV names the output file after the stem of the input file,
	// so we use a temporary directory and rename the result to <javID>.srt.
	tmpDir, err := os.MkdirTemp("", "whisperjav-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	args := []string{
		audioPath,
		"--model", r.model,
		"--language", languageName(r.language),
		"--output-format", "srt",
		"--output-dir", tmpDir,
	}

	// Use a non-nil context; exec.CommandContext requires one.
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, r.bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisperJAV failed: %w\nstderr: %s", err, stderr.String())
	}

	// Locate the generated .srt file and move it to the canonical path.
	generated, err := findSRT(tmpDir)
	if err != nil {
		return "", fmt.Errorf("find generated srt: %w", err)
	}
	if err := os.Rename(generated, srtPath); err != nil {
		// Rename may fail across filesystems; fall back to copy+delete.
		if copyErr := copyFile(generated, srtPath); copyErr != nil {
			return "", fmt.Errorf("move srt to output: %w (copy fallback: %v)", err, copyErr)
		}
	}

	r.log.Debug("transcription done",
		"srt", srtPath,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return srtPath, nil
}

// findSRT returns the first .srt file found in dir.
func findSRT(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".srt" {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .srt file found in %s", dir)
}

// languageName converts BCP-47 codes to whisperjav full language names.
func languageName(code string) string {
	switch strings.ToLower(code) {
	case "ja", "japanese":
		return "japanese"
	case "zh", "chinese":
		return "chinese"
	case "ko", "korean":
		return "korean"
	case "en", "english":
		return "english"
	default:
		return code // pass through unknown codes as-is
	}
}

// copyFile copies src to dst using os.ReadFile / os.WriteFile.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
