package strm

import (
	"fmt"
	"os"
	"path/filepath"
)

// Generate writes a .strm file containing the playback URL.
// The URL should be the OpenList /d/ path to support 302 redirect.
// If the file already exists with identical content, the write is skipped.
func Generate(outDir, javID, url string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	path := filepath.Join(outDir, javID+".strm")
	content := []byte(url + "\n")
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(content) {
		return nil // already up to date
	}
	return os.WriteFile(path, content, 0644)
}
