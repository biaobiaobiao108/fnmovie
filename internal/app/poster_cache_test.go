package app

import (
	"bytes"
	"container/list"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFitPosterSizeDownscalesWithoutUpscaling(t *testing.T) {
	tests := []struct {
		name   string
		source image.Point
		limit  image.Point
		want   image.Point
	}{
		{name: "portrait", source: image.Pt(1000, 1500), limit: image.Pt(336, 504), want: image.Pt(336, 504)},
		{name: "landscape", source: image.Pt(1600, 900), limit: image.Pt(336, 504), want: image.Pt(336, 189)},
		{name: "small source stays small", source: image.Pt(120, 180), limit: image.Pt(336, 504), want: image.Pt(120, 180)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitPosterSize(tt.source, tt.limit); got != tt.want {
				t.Fatalf("fitPosterSize(%v, %v) = %v, want %v", tt.source, tt.limit, got, tt.want)
			}
		})
	}
}

func TestPosterLoaderReusesDiskSourceAcrossTargetSizes(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 100, 200))
	source.Set(10, 10, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	serverHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(encoded.Bytes())
	}))
	defer serverHTTP.Close()
	server := NewServer(serverHTTP.URL, "")
	server.client = serverHTTP.Client()
	loader := &PosterLoader{diskDir: filepath.Join(t.TempDir(), "posters")}
	if err := os.MkdirAll(loader.diskDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range []image.Point{image.Pt(50, 50), image.Pt(20, 20)} {
		request := posterRequest{server: server, url: serverHTTP.URL + "/poster", key: posterKey{URL: serverHTTP.URL + "/poster", Width: size.X, Height: size.Y}}
		if _, _, err := loader.loadPoster(request); err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("poster network requests=%d, want 1 when reusing disk cache", got)
	}
}

func TestPosterLoaderEvictsLeastRecentlyUsed(t *testing.T) {
	loader := &PosterLoader{
		pending: map[posterKey]struct{}{}, failed: map[posterKey]time.Time{},
		entries: map[posterKey]*list.Element{}, lru: list.New(), maxBytes: 8,
	}
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	keyA, keyB, keyC := posterKey{URL: "a"}, posterKey{URL: "b"}, posterKey{URL: "c"}
	loader.putLocked(keyA, bitmap, 4)
	loader.putLocked(keyB, bitmap, 4)
	loader.lru.MoveToFront(loader.entries[keyA])
	loader.putLocked(keyC, bitmap, 4)
	if loader.entries[keyA] == nil || loader.entries[keyC] == nil || loader.entries[keyB] != nil {
		t.Fatalf("LRU entries after eviction: A=%v B=%v C=%v", loader.entries[keyA] != nil, loader.entries[keyB] != nil, loader.entries[keyC] != nil)
	}
	if loader.used != 8 {
		t.Fatalf("cache uses %d bytes, want 8", loader.used)
	}
}

func TestPosterDiskWritesEnforceBudgetAcrossConcurrentWorkers(t *testing.T) {
	loader := &PosterLoader{diskDir: t.TempDir()}
	oldPath := posterSourcePath(loader.diskDir, "old")
	old, err := os.Create(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Truncate(posterDiskCacheBudget - 2*1024); err != nil {
		_ = old.Close()
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, past, past); err != nil {
		t.Fatal(err)
	}
	loader.prunePosterDisk()
	var workers sync.WaitGroup
	for _, name := range []string{"a", "b", "c", "d"} {
		workers.Add(1)
		go func(name string) {
			defer workers.Done()
			loader.cachePosterSource(posterSourcePath(loader.diskDir, name), make([]byte, 1024))
		}(name)
	}
	workers.Wait()
	entries, err := os.ReadDir(loader.diskDir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if total > posterDiskCacheBudget {
		t.Fatalf("concurrent writes exceeded disk budget: %d", total)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("oldest source should be evicted, stat error=%v", err)
	}
	for _, name := range []string{"a", "b", "c", "d"} {
		if info, err := os.Stat(posterSourcePath(loader.diskDir, name)); err != nil || info.Size() != 1024 {
			t.Fatalf("new source %q was not retained: %v", name, err)
		}
	}
}
