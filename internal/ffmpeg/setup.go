package ffmpeg

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	once     sync.Once
	binDir   string
	setupErr error
)

// Setup extracts embedded ffmpeg binaries to cacheDir (or os.UserCacheDir).
// Safe to call multiple times — extraction only happens once per process.
func Setup(cacheDir string) (dir string, err error) {
	once.Do(func() {
		binDir, setupErr = setup(cacheDir)
	})
	return binDir, setupErr
}

func setup(cacheDir string) (string, error) {
	if cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		cacheDir = filepath.Join(base, "jav-aio", "ffmpeg")
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}

	fs, prefix, exes := platformFS()
	for _, exe := range exes {
		src := prefix + "/" + exe
		dst := filepath.Join(cacheDir, exe)
		if err := extractIfChanged(fs, src, dst); err != nil {
			return "", fmt.Errorf("extract %s: %w", exe, err)
		}
		if err := os.Chmod(dst, 0755); err != nil {
			return "", err
		}
	}
	return cacheDir, nil
}

func platformFS() (embed.FS, string, []string) {
	switch runtime.GOOS {
	case "windows":
		return windowsAMD64, "assets/windows_amd64", []string{"ffmpeg.exe", "ffprobe.exe"}
	default:
		return linuxAMD64, "assets/linux_amd64", []string{"ffmpeg", "ffprobe"}
	}
}

func extractIfChanged(fs embed.FS, src, dst string) error {
	data, err := fs.ReadFile(src)
	if err != nil {
		return err
	}
	// Check if existing file is identical (by hash) to avoid unnecessary writes.
	if existing, err := os.ReadFile(dst); err == nil {
		if sha256hex(existing) == sha256hex(data) {
			return nil
		}
	}
	return os.WriteFile(dst, data, 0644)
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// BinPath returns the full path to an embedded binary (e.g. "ffmpeg", "ffprobe").
func BinPath(cacheDir, name string) string {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		name += ".exe"
	}
	return filepath.Join(cacheDir, name)
}
