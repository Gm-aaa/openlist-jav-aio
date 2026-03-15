package subtitle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/subtitle"
)

func TestHasExternalSubtitle_Found(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ABC-123.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0644)
	if !subtitle.HasExternalSubtitle(dir, "ABC-123") {
		t.Error("expected external subtitle found")
	}
}

func TestHasExternalSubtitle_Missing(t *testing.T) {
	dir := t.TempDir()
	if subtitle.HasExternalSubtitle(dir, "ABC-123") {
		t.Error("expected no external subtitle")
	}
}

func TestHasExternalSubtitle_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	// File with upper-case ID, lookup with lower-case prefix
	os.WriteFile(filepath.Join(dir, "ABC-123.srt"), []byte("sub"), 0644)
	if !subtitle.HasExternalSubtitle(dir, "abc-123") {
		t.Error("expected case-insensitive match")
	}
}

func TestHasExternalSubtitle_WrongExtension(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ABC-123.vtt"), []byte("WEBVTT"), 0644)
	if subtitle.HasExternalSubtitle(dir, "ABC-123") {
		t.Error("expected .vtt to not count as external .srt subtitle")
	}
}

func TestAudioKeeper_Evict(t *testing.T) {
	dir := t.TempDir()
	k := subtitle.NewAudioKeeper(dir, 2)
	// Add 3 files — oldest should be evicted
	k.Add("file1.aac")
	k.Add("file2.aac")
	os.WriteFile(filepath.Join(dir, "file1.aac"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "file2.aac"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "file3.aac"), []byte("c"), 0644)
	k.Add("file3.aac")
	// file1 should have been evicted
	if _, err := os.Stat(filepath.Join(dir, "file1.aac")); !os.IsNotExist(err) {
		t.Error("expected file1 to be evicted")
	}
}

func TestAudioKeeper_NoEvictUnderMax(t *testing.T) {
	dir := t.TempDir()
	k := subtitle.NewAudioKeeper(dir, 3)
	os.WriteFile(filepath.Join(dir, "f1.aac"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "f2.aac"), []byte("b"), 0644)
	k.Add("f1.aac")
	k.Add("f2.aac")
	// Both files should still exist
	for _, f := range []string{"f1.aac", "f2.aac"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to still exist: %v", f, err)
		}
	}
}

func TestAudioKeeper_KeepsMostRecent(t *testing.T) {
	dir := t.TempDir()
	k := subtitle.NewAudioKeeper(dir, 2)
	for _, f := range []string{"a.aac", "b.aac", "c.aac"} {
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644)
		k.Add(f)
	}
	// a.aac evicted when c was added; b and c should remain
	for _, f := range []string{"b.aac", "c.aac"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to remain: %v", f, err)
		}
	}
}
