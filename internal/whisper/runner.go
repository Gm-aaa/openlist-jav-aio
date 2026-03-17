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
//	    --model          <name>      — Whisper model (e.g. medium, large-v2, large-v3, turbo)
//	    --language       <lang>      — full name: japanese, korean, chinese, english
//	    --output-format  srt         — emit SRT (also supports vtt, both)
//	    --output-dir     <dir>       — directory to write the subtitle file
//	    --no-signature               — disable attribution URL at end of SRT
//	    --no-progress                — suppress progress bars
//	    --accept-cpu-mode            — suppress GPU warning when no CUDA available
//	    --compute-type   <type>      — CTranslate2 quantisation (auto/float16/int8/...)
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

	"github.com/openlist-jav-aio/jav-aio/internal/util"
)

// resolveBin returns the absolute path of bin.
// If bin is already an absolute path, os.Stat is used to verify existence.
// Otherwise exec.LookPath is used to search $PATH, matching exec.Command behaviour.
func resolveBin(bin string) (string, error) {
	if filepath.IsAbs(bin) {
		if _, err := os.Stat(bin); err != nil {
			return "", fmt.Errorf("binary not found at %s: %w", bin, err)
		}
		return bin, nil
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in PATH: %w", bin, err)
	}
	return resolved, nil
}

// Runner wraps the WhisperJAV CLI binary.
type Runner struct {
	bin         string
	model       string
	language    string
	sensitivity string // "" = WhisperJAV default; "aggressive" / "conservative" / "balanced"
	computeType string // "" = WhisperJAV default; e.g. "int8_float32" for CPU
	cpuOnly     bool   // true = pass --accept-cpu-mode (Docker/no-GPU environments)
	log         *slog.Logger
}

// RunnerOptions holds optional tuning parameters for NewRunner.
type RunnerOptions struct {
	// Sensitivity controls WhisperJAV's hallucination-filter aggressiveness.
	// Valid values: "aggressive", "conservative", "balanced". "" = WhisperJAV default.
	Sensitivity string
	// ComputeType sets the CTranslate2 quantisation mode, e.g. "int8_float32".
	// "" = WhisperJAV default (int8 on CPU). "int8_float32" gives the best
	// CPU speed/accuracy balance.
	ComputeType string
	// CPUOnly passes --accept-cpu-mode to suppress interactive GPU warnings.
	// Use in Docker or environments without GPU.
	CPUOnly bool
}

// NewRunner creates a Runner.
//   - bin:      absolute path (or name on $PATH) of the whisperjav executable.
//   - model:    Whisper model name, e.g. "large-v3", "litagin/anime-whisper".
//   - language: BCP-47 code, e.g. "ja".
//   - opts:     optional tuning parameters; zero value uses WhisperJAV defaults.
//   - log:      structured logger; uses slog.Default() when nil.
func NewRunner(bin, model, language string, opts RunnerOptions, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{
		bin:         bin,
		model:       model,
		language:    language,
		sensitivity: opts.Sensitivity,
		computeType: opts.ComputeType,
		cpuOnly:     opts.CPUOnly,
		log:         log,
	}
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
	// Verify the binary is resolvable before attempting execution.
	// Handles both absolute paths and PATH-based names (e.g. "whisperjav" in Docker).
	if _, err := resolveBin(r.bin); err != nil {
		return "", fmt.Errorf("whisperJAV: %w", err)
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
		"--no-signature", // disable the WhisperJAV attribution URL appended at end of SRT
		"--no-progress",  // suppress progress bars in captured stderr
	}
	if r.cpuOnly {
		args = append(args, "--accept-cpu-mode") // suppress interactive GPU warning when no CUDA is available
	}
	if r.sensitivity != "" {
		args = append(args, "--sensitivity", r.sensitivity)
	}
	if r.computeType != "" {
		args = append(args, "--compute-type", r.computeType)
	}

	// Use a non-nil context; exec.CommandContext requires one.
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, r.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisperJAV failed: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	// Log full output at debug level so issues can be diagnosed.
	r.log.Debug("whisperJAV output", "stdout", stdout.String(), "stderr", stderr.String())

	// Locate the generated .srt file and move it to the canonical path.
	generated, err := findSRT(tmpDir)
	if err != nil {
		return "", fmt.Errorf("find generated srt: %w", err)
	}
	if err := util.AtomicRename(generated, srtPath); err != nil {
		return "", fmt.Errorf("move srt to output: %w", err)
	}

	// Reject SRT files with no subtitle blocks — WhisperJAV may write an empty
	// or whitespace-only file when its hallucination filter removes all content.
	if !util.IsValidSRT(srtPath) {
		os.Remove(srtPath)
		return "", fmt.Errorf("whisperJAV produced no subtitles for %s (0 blocks after post-processing)", javID)
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

// languageName converts BCP-47 codes to the full language names that
// WhisperJAV's --language flag expects (japanese, korean, chinese, english).
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

