package whisper_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/whisper"
)

func TestRunner_MissingBin(t *testing.T) {
	r := whisper.NewRunner("/nonexistent/whisperJAV", "medium", "ja", nil)
	_, err := r.Transcribe(nil, "/some/audio.aac", t.TempDir(), "ABC-123")
	if err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestRunner_SRTPath(t *testing.T) {
	outDir := t.TempDir()
	expected := filepath.Join(outDir, "ABC-123.srt")
	got := whisper.SRTPath(outDir, "ABC-123")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestNewRunner_DefaultLogger(t *testing.T) {
	r := whisper.NewRunner("whisperjav", "large-v3", "ja", nil)
	if r == nil {
		t.Error("expected non-nil runner")
	}
}

func TestRunner_AudioExtensions(t *testing.T) {
	// Verify SRTPath works regardless of audio extension
	outDir := t.TempDir()
	for _, javID := range []string{"ABC-123", "TEST-456", "LOWER-789"} {
		got := whisper.SRTPath(outDir, javID)
		if filepath.Ext(got) != ".srt" {
			t.Errorf("expected .srt extension for %s, got %s", javID, got)
		}
		if _, err := os.Stat(filepath.Dir(got)); err != nil {
			t.Errorf("output directory does not exist: %v", err)
		}
	}
}
