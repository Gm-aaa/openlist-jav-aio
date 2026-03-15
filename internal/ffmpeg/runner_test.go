package ffmpeg_test

import (
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/ffmpeg"
)

func TestSetup(t *testing.T) {
	dir, err := ffmpeg.Setup(t.TempDir())
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if dir == "" {
		t.Error("expected non-empty dir")
	}
}
