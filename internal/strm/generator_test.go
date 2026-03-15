package strm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/strm"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	url := "http://openlist:5244/d/jav/ABC-123.mp4"
	if err := strm.Generate(dir, "ABC-123", url); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "ABC-123.strm"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != url {
		t.Errorf("expected %q, got %q", url, string(content))
	}
}
