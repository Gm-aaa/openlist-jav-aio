package ffmpeg

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

// tmpName returns a temporary filename that preserves the original extension
// so ffmpeg can infer the output format. e.g. "foo.aac" → "foo.tmp.aac".
func tmpName(dest string) string {
	ext := filepath.Ext(dest)
	return strings.TrimSuffix(dest, ext) + ".tmp" + ext
}

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

	tmpFile := tmpName(destSRT)
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
	return util.AtomicRename(tmpFile, destSRT)
}

// ExtractAudio extracts only the audio stream from videoURL to destAudio.
// Streams directly from remote URL — does NOT download the video.
// Writes to a temp file first and renames on success for crash safety.
func (r *Runner) ExtractAudio(ctx context.Context, videoURL, destAudio string) error {
	r.log.Info("ffmpeg extracting audio from remote (this may take a while for large files)",
		"dest", destAudio)
	start := time.Now()

	tmpFile := tmpName(destAudio)

	path := BinPath(r.binDir, "ffmpeg")
	cmd := exec.CommandContext(ctx, path,
		"-user_agent", httpUserAgent,
		"-i", videoURL,
		"-vn",
		"-acodec", "copy",
		"-y", tmpFile,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Log progress periodically so the user knows ffmpeg is still working.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				os.Remove(tmpFile)
				return fmt.Errorf("extract audio: %w\nstderr: %s", err, stderr.String())
			}
			if err := util.AtomicRename(tmpFile, destAudio); err != nil {
				return err
			}
			r.log.Info("audio extracted", "dest", destAudio,
				"size_mb", fileSizeMB(destAudio),
				"duration_s", int(time.Since(start).Seconds()))
			return nil
		case <-ticker.C:
			r.log.Info("ffmpeg still extracting audio...",
				"elapsed_s", int(time.Since(start).Seconds()),
				"size_mb", fileSizeMB(tmpFile))
		}
	}
}

// fileSizeMB returns the file size in MB, or 0 if the file cannot be read.
func fileSizeMB(path string) float64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return float64(fi.Size()) / (1024 * 1024)
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
