package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
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
func (r *Runner) ExtractSubtitle(ctx context.Context, videoURL, destSRT string) error {
	r.log.Debug("ffmpeg extract subtitle", "url", videoURL, "dest", destSRT)
	_, err := r.run(ctx, "ffmpeg",
		"-user_agent", httpUserAgent,
		"-i", videoURL,
		"-map", "0:s:0",
		"-c:s", "srt",
		"-y", destSRT,
	)
	return err
}

// ExtractAudio extracts only the audio stream from videoURL to destAudio.
// Streams directly from remote URL — does NOT download the video.
func (r *Runner) ExtractAudio(ctx context.Context, videoURL, destAudio string) error {
	r.log.Debug("ffmpeg extract audio from remote", "url", videoURL, "dest", destAudio)
	start := time.Now()
	_, err := r.run(ctx, "ffmpeg",
		"-user_agent", httpUserAgent,
		"-i", videoURL,
		"-vn",             // no video
		"-acodec", "copy", // copy audio stream without re-encoding
		"-y", destAudio,
	)
	if err != nil {
		return fmt.Errorf("extract audio: %w", err)
	}
	r.log.Debug("audio extracted", "dest", destAudio, "duration_ms", time.Since(start).Milliseconds())
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
