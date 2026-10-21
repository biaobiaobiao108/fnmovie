package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveStaleCacheTempsOnlyRemovesOldMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	oldTemp := filepath.Join(dir, "poster-abandoned.tmp")
	recentTemp := filepath.Join(dir, "poster-active.tmp")
	otherTemp := filepath.Join(dir, "catalog-abandoned.tmp")
	poster := filepath.Join(dir, "poster-cached.img")
	for _, path := range []string{oldTemp, recentTemp, otherTemp, poster} {
		if err := os.WriteFile(path, []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	if err := os.Chtimes(oldTemp, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(otherTemp, old, old); err != nil {
		t.Fatal(err)
	}

	if removed := removeStaleCacheTemps(dir, "poster-", time.Hour, now); removed != 1 {
		t.Fatalf("removed %d files, want only the stale poster temp", removed)
	}
	for _, path := range []string{oldTemp} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stale temp still exists: %s", path)
		}
	}
	for _, path := range []string{recentTemp, otherTemp, poster} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("unrelated or recent file was removed: %s (%v)", path, err)
		}
	}
}
