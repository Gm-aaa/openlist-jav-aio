package strm

import (
	"fmt"
	"os"
	"path/filepath"
)

// Generate writes a .strm file containing the playback URL.
// The URL should be the OpenList /d/ path to support 302 redirect.
func Generate(outDir, javID, url string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	return os.WriteFile(filepath.Join(outDir, javID+".strm"), []byte(url+"\n"), 0644)
}
