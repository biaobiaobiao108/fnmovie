package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func writeAtomicFile(path, pattern string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := verifyReplaceDirectory(temporary, path); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

func verifyReplaceDirectory(source, destination string) error {
	sourceInfo, err := os.Stat(filepath.Dir(source))
	if err != nil {
		return fmt.Errorf("stat temporary file directory: %w", err)
	}
	destinationInfo, err := os.Stat(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("stat destination directory: %w", err)
	}
	if !sourceInfo.IsDir() || !destinationInfo.IsDir() || !os.SameFile(sourceInfo, destinationInfo) {
		return fmt.Errorf("temporary file and destination are in different directories")
	}
	return nil
}

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
