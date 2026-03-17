// Package subtitle implements three-tier subtitle detection and generation:
//
//  1. External .srt file already present on disk  → use it directly.
//  2. Embedded subtitle track inside the video     → extract with ffmpeg.
//  3. No subtitle available                        → extract audio from the remote
//     URL (no full video download) and transcribe with WhisperJAV.
package subtitle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openlist-jav-aio/jav-aio/internal/ffmpeg"
	"github.com/openlist-jav-aio/jav-aio/internal/util"
	"github.com/openlist-jav-aio/jav-aio/internal/whisper"
)

// FindExternalSubtitle returns the path of a valid .srt file whose name starts
// with javID (case-insensitive) in outDir, or "" if none is found. Validity is
// checked by looking for at least one SRT timecode marker (" --> "), which
// guards against truncated files left behind by interrupted ffmpeg extractions.
func FindExternalSubtitle(outDir, javID string) string {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return ""
	}
	prefix := strings.ToLower(javID)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".srt") {
			path := filepath.Join(outDir, e.Name())
			if util.IsValidSRT(path) {
				return path
			}
		}
	}
	return ""
}

// Processor orchestrates the subtitle detection/generation pipeline for a
// single video identified by its JAV ID.
type Processor struct {
	ffmpeg      *ffmpeg.Runner
	whisper     *whisper.Runner
	keepAudio   bool
	audioKeeper *AudioKeeper
	audioDir    string
	log         *slog.Logger
}

// NewProcessor creates a Processor.
//
//   - ffmpegRunner:  pre-configured ffmpeg.Runner (used for probing and extraction).
//   - whisperRunner: pre-configured whisper.Runner (used for transcription).
//   - keepAudio:     when true, extracted audio files are retained on disk.
//   - keepAudioMax:  maximum number of audio files to keep (0 = unlimited, only
//     relevant when keepAudio is true).
//   - audioDir:      directory used for extracted audio; falls back to os.TempDir().
//   - log:           structured logger; uses slog.Default() when nil.
func NewProcessor(
	ffmpegRunner *ffmpeg.Runner,
	whisperRunner *whisper.Runner,
	keepAudio bool,
	keepAudioMax int,
	audioDir string,
	log *slog.Logger,
) *Processor {
	if log == nil {
		log = slog.Default()
	}
	var keeper *AudioKeeper
	if keepAudio && keepAudioMax > 0 {
		keeper = NewAudioKeeper(audioDir, keepAudioMax)
	}
	return &Processor{
		ffmpeg:      ffmpegRunner,
		whisper:     whisperRunner,
		keepAudio:   keepAudio,
		audioKeeper: keeper,
		audioDir:    audioDir,
		log:         log,
	}
}

// Process runs the three-tier subtitle pipeline for one video.
//
// videoURL is the remote OpenList streaming URL; the video is never fully
// downloaded — ffmpeg reads only the headers / audio stream as needed.
//
// Returns the absolute path of the resulting .srt file.
func (p *Processor) Process(ctx context.Context, videoURL, outDir, javID string) (string, error) {
	start := time.Now()
	p.log.Debug("subtitle process start", "id", javID)

	// Tier 1: external subtitle already on disk.
	if srtPath := FindExternalSubtitle(outDir, javID); srtPath != "" {
		p.log.Debug("external subtitle found, skipping transcription", "id", javID, "path", srtPath)
		return srtPath, nil
	}

	// Tier 2: embedded subtitle stream in the remote video.
	if p.ffmpeg == nil {
		return "", fmt.Errorf("ffmpeg runner not available; cannot check/extract subtitles")
	}
	hasSub, err := p.ffmpeg.HasEmbeddedSubtitles(ctx, videoURL)
	if err != nil {
		p.log.Warn("ffprobe subtitle check failed, proceeding to whisper",
			"id", javID, "error", err)
	} else if hasSub {
		p.log.Debug("extracting embedded subtitle", "id", javID)
		srtPath := filepath.Join(outDir, javID+".srt")
		if err := p.ffmpeg.ExtractSubtitle(ctx, videoURL, srtPath); err != nil {
			return "", fmt.Errorf("extract embedded subtitle: %w", err)
		}
		p.log.Debug("embedded subtitle extracted",
			"id", javID, "duration_ms", time.Since(start).Milliseconds())
		return srtPath, nil
	}

	// Tier 3: generate via WhisperJAV.
	// Extract only the audio stream from the remote URL — no full download.
	audioPath, err := p.extractAudio(ctx, videoURL, javID)
	if err != nil {
		return "", fmt.Errorf("extract audio for whisper: %w", err)
	}

	srtPath, err := p.whisper.Transcribe(ctx, audioPath, outDir, javID)
	if err != nil {
		p.handleAudioRetention(audioPath) // respect keepAudio even on failure
		return "", fmt.Errorf("whisper transcribe: %w", err)
	}

	p.handleAudioRetention(audioPath)
	p.log.Debug("subtitle generated via whisper",
		"id", javID, "srt", srtPath, "duration_ms", time.Since(start).Milliseconds())
	return srtPath, nil
}

// extractAudio extracts the audio stream of videoURL to audioDir/<javID>.aac.
// The extraction uses ffmpeg's stream copy mode so no re-encoding occurs and
// only the audio portion of the remote stream is read.
func (p *Processor) extractAudio(ctx context.Context, videoURL, javID string) (string, error) {
	dir := p.audioDir
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create audio dir: %w", err)
	}
	audioPath := filepath.Join(dir, javID+".aac")
	if err := p.ffmpeg.ExtractAudio(ctx, videoURL, audioPath); err != nil {
		return "", err
	}
	// Validate extracted audio is not empty (can happen if URL expired or video has no audio).
	fi, err := os.Stat(audioPath)
	if err != nil {
		return "", fmt.Errorf("stat extracted audio: %w", err)
	}
	if fi.Size() == 0 {
		os.Remove(audioPath)
		return "", fmt.Errorf("extracted audio %s is empty (0 bytes); source video may lack an audio stream or the URL may have expired", audioPath)
	}
	return audioPath, nil
}

// handleAudioRetention either removes the audio file (keepAudio=false) or
// registers it with the AudioKeeper so the LRU eviction policy applies.
func (p *Processor) handleAudioRetention(audioPath string) {
	if !p.keepAudio {
		os.Remove(audioPath) //nolint:errcheck
		return
	}
	if p.audioKeeper != nil {
		p.audioKeeper.Add(filepath.Base(audioPath))
	}
}
