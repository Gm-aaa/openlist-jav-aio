package scraper_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/scraper"
)

func TestWriteNFO(t *testing.T) {
	dir := t.TempDir()
	info := &scraper.MovieInfo{
		ID:     "ABC-123",
		Title:  "Test Title",
		Studio: "Test Studio",
		Actors: []string{"Actor A", "Actor B"},
		Rating: 7.5,
		Cover:  "https://example.com/cover.jpg",
	}
	if err := scraper.WriteNFO(dir, "ABC-123", info); err != nil {
		t.Fatal(err)
	}
	nfoPath := filepath.Join(dir, "ABC-123.nfo")
	if _, err := os.Stat(nfoPath); err != nil {
		t.Fatalf("NFO not created: %v", err)
	}
	content, _ := os.ReadFile(nfoPath)
	if !containsStr(string(content), "Test Title") {
		t.Error("NFO missing title")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s[1:], sub) || s[:len(sub)] == sub)
}
