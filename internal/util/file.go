// Package util provides shared file-system helpers used across the project.
package util

import (
	"fmt"
	"os"
	"strings"
)

// AtomicRename moves src to dst. Falls back to read+write+remove if rename
// fails (e.g. cross-filesystem).
func AtomicRename(src, dst string) error {
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

// IsValidSRT reads the file at path and returns true if it contains at least
// one SRT timecode marker (" --> "), indicating a non-empty subtitle file.
func IsValidSRT(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), " --> ")
}
