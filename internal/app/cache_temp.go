package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// removeStaleCacheTemps removes abandoned atomic-write files while leaving
// recent files alone in case another application instance is still writing.
func removeStaleCacheTemps(dir, prefix string, maxAge time.Duration, now time.Time) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || filepath.Ext(entry.Name()) != ".tmp" {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) <= maxAge {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
			removed++
		}
	}
	return removed
}
