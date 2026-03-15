package subtitle

import (
	"os"
	"path/filepath"
	"sync"
)

// AudioKeeper manages a bounded collection of audio files, evicting the oldest
// file from disk whenever the count exceeds max.
//
// It is safe for concurrent use.
type AudioKeeper struct {
	dir   string
	max   int
	files []string
	mu    sync.Mutex
}

// NewAudioKeeper creates an AudioKeeper that stores files in dir and retains at
// most max files on disk.
func NewAudioKeeper(dir string, max int) *AudioKeeper {
	return &AudioKeeper{dir: dir, max: max}
}

// Add registers a new audio filename. If the total count would exceed max, the
// oldest registered file is removed from disk before the new one is appended.
func (k *AudioKeeper) Add(filename string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.files = append(k.files, filename)
	if len(k.files) > k.max {
		oldest := k.files[0]
		k.files = k.files[1:]
		os.Remove(filepath.Join(k.dir, oldest)) //nolint:errcheck — best-effort delete
	}
}

// Files returns a snapshot of the currently tracked filenames (oldest first).
func (k *AudioKeeper) Files() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]string, len(k.files))
	copy(out, k.files)
	return out
}
