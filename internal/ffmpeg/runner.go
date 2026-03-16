package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// httpUserAgent is sent to remote HTTP servers when reading video/audio streams.
// 115 CDN rejects ffmpeg's default "Lavf/x.x.x" user-agent with 403; using
// a curl-style UA passes the CDN's user-agent check.
const httpUserAgent = "curl/8.18.0"

type Runner struct {
	binDir string
	log    *slog.Logger
}

func NewRunner(cacheDir string, log *slog.Logger) (*Runner, error) {
	dir, err := Setup(cacheDir)
	if err != nil {
		return nil, err
	}
	return &Runner{binDir: dir, log: log}, nil
}

// HasEmbeddedSubtitles returns true if the remote video URL contains a subtitle stream.
func (r *Runner) HasEmbeddedSubtitles(ctx context.Context, videoURL string) (bool, error) {
	start := time.Now()
	args := []string{
		"-user_agent", httpUserAgent,
		"-v", "error",
		"-show_streams", "-select_streams", "s",
		"-print_format", "csv",
		videoURL,
	}
	r.log.Debug("ffprobe check subtitles", "url", videoURL)
	out, err := r.run(ctx, "ffprobe", args...)
	if err != nil {
		return false, fmt.Errorf("ffprobe subtitles: %w", err)
	}
	has := strings.TrimSpace(out) != ""
	r.log.Debug("ffprobe subtitle result", "has", has, "duration_ms", time.Since(start).Milliseconds())
	return has, nil
}

// ExtractSubtitle extracts the first subtitle stream from videoURL to destSRT.
// Writes to a temp file first and renames on success, so a killed process never
// leaves a truncated SRT at the final destination.
func (r *Runner) ExtractSubtitle(ctx context.Context, videoURL, destSRT string) error {
	r.log.Debug("ffmpeg extract subtitle", "url", videoURL, "dest", destSRT)

	tmpFile := destSRT + ".tmp"
	_, err := r.run(ctx, "ffmpeg",
		"-user_agent", httpUserAgent,
		"-i", videoURL,
		"-map", "0:s:0",
		"-c:s", "srt",
		"-y", tmpFile,
	)
	if err != nil {
		os.Remove(tmpFile)
		return err
	}
	return atomicRename(tmpFile, destSRT)
}

// ExtractAudio extracts only the audio stream from videoURL to destAudio.
// Streams directly from remote URL — does NOT download the video.
// Writes to a temp file first and renames on success for crash safety.
func (r *Runner) ExtractAudio(ctx context.Context, videoURL, destAudio string) error {
	r.log.Debug("ffmpeg extract audio from remote", "url", videoURL, "dest", destAudio)
	start := time.Now()

	tmpFile := destAudio + ".tmp"
	_, err := r.run(ctx, "ffmpeg",
		"-user_agent", httpUserAgent,
		"-i", videoURL,
		"-vn",
		"-acodec", "copy",
		"-y", tmpFile,
	)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("extract audio: %w", err)
	}
	if err := atomicRename(tmpFile, destAudio); err != nil {
		return err
	}
	r.log.Debug("audio extracted", "dest", destAudio, "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// atomicRename moves src to dst. Falls back to read+write+remove if rename
// fails (e.g. cross-filesystem).
func atomicRename(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		data, readErr := os.ReadFile(src)
		if readErr != nil {
			return fmt.Errorf("rename %s → %s: %w; read fallback: %v", src, dst, err, readErr)
		}
		if writeErr := os.WriteFile(dst, data, 0644); writeErr != nil {
			return fmt.Errorf("rename %s → %s: %w; write fallback: %v", src, dst, err, writeErr)
		}
		os.Remove(src)
	}
	return nil
}

func (r *Runner) run(ctx context.Context, bin string, args ...string) (string, error) {
	path := BinPath(r.binDir, bin)
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w\nstderr: %s", bin, err, stderr.String())
	}
	return stdout.String(), nil
}
