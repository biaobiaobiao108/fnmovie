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
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPlaybackDropsPostersAndRejectsObsoleteDecode(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 20, 30))); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	serverHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		_, _ = w.Write(data.Bytes())
	}))
	defer serverHTTP.Close()
	defer unblock()
	server := NewServer(serverHTTP.URL, "")
	server.client = serverHTTP.Client()
	loader := &PosterLoader{
		queue: make(chan posterRequest, posterQueueLimit), pending: map[posterKey]uint64{},
		failed: map[posterKey]time.Time{}, entries: map[posterKey]*list.Element{},
		lru: list.New(), maxBytes: posterCacheBudget,
	}
	workerDone := make(chan struct{})
	go func() { loader.worker(); close(workerDone) }()
	defer func() { unblock(); close(loader.queue); <-workerDone }()
	var obsoleteCallback atomic.Bool
	loader.putLocked(posterKey{URL: "cached"}, ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1))), 4)
	loader.GetOrRequest(server, serverHTTP.URL, 20, 30, func() { obsoleteCallback.Store(true) })
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("initial image request did not start")
	}
	loader.GetOrRequest(server, serverHTTP.URL+"/queued", 20, 30, nil)
	loader.SetPlayback(true)
	if items, size, pending := loader.Stats(); items != 0 || size != 0 || pending != 0 || len(loader.queue) != 0 {
		t.Fatalf("playback retained work: items=%d bytes=%d pending=%d", items, size, pending)
	}
	loader.GetOrRequest(server, serverHTTP.URL, 20, 30, nil)
	if _, _, pending := loader.Stats(); pending != 0 {
		t.Fatal("playback scheduled a poster request")
	}
	loader.SetPlayback(false)
	loaded := make(chan struct{})
	loader.GetOrRequest(server, serverHTTP.URL, 20, 30, func() { close(loaded) })
	unblock()
	select {
	case <-loaded:
	case <-time.After(3 * time.Second):
		t.Fatal("return navigation did not reload the poster")
	}
	if obsoleteCallback.Load() || calls.Load() != 2 {
		t.Fatalf("obsolete decode was accepted or queued work ran: callback=%v calls=%d", obsoleteCallback.Load(), calls.Load())
	}
	if items, size, pending := loader.Stats(); items != 1 || size <= 0 || pending != 0 {
		t.Fatalf("resumed cache invalid: items=%d bytes=%d pending=%d", items, size, pending)
	}
}

func TestHomeKeepsDecodedPreviewWhileBackdropLoads(t *testing.T) {
	server := NewServer("http://example.test", "")
	loader := &PosterLoader{
		queue: make(chan posterRequest, posterQueueLimit), pending: map[posterKey]uint64{},
		failed: map[posterKey]time.Time{}, entries: map[posterKey]*list.Element{},
		lru: list.New(), maxBytes: posterCacheBudget,
	}
	a := &appState{server: server, posters: loader, displayScale: 1}
	item := MediaItem{Poster: "/preview.jpg", Backdrop: "/backdrop.jpg"}
	w, h := homeHeroPixelSize(800, 450, 1)
	preview := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	loader.putLocked(posterKey{URL: server.imageURL(item.Poster), Width: w, Height: h}, preview, 4)
	if got := a.homeHeroImage(item, 800, 450); got != preview || len(loader.queue) != 1 {
		t.Fatal("loading backdrop removed preview or scheduled extra preview work")
	}
	backdrop := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 1)))
	loader.putLocked(posterKey{URL: server.imageURL(item.Backdrop), Width: w, Height: h}, backdrop, 8)
	if got := a.homeHeroImage(item, 800, 450); got != backdrop {
		t.Fatal("loaded backdrop did not replace preview")
	}
	loader.SetPlayback(true)
	if loader.Cached(server.imageURL(item.Poster), w, h) != nil {
		t.Fatal("preview survived playback cache release")
	}
}

// Synthetic artwork isolates CPU cache retention, not whole-process/video memory.
func BenchmarkPosterRetention(b *testing.B) {
	for _, budget := range []struct {
		name  string
		bytes int64
	}{
		{"previous256MiB", 256 << 20}, {"current64MiB", posterCacheBudget},
	} {
		b.Run(budget.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				loader := &PosterLoader{entries: map[posterKey]*list.Element{}, lru: list.New(), maxBytes: budget.bytes}
				for i := 0; i < 120; i++ {
					bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 512, 512)))
					loader.putLocked(posterKey{Width: i}, bitmap, 512*512*4*4/3)
				}
				runtime.GC()
				runtime.ReadMemStats(&after)
				b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/(1<<20), "heap-MiB")
				b.ReportMetric(float64(loader.used)/(1<<20), "budgeted-MiB")
				runtime.KeepAlive(loader)
			}
		})
	}
}

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
		pending: map[posterKey]uint64{}, failed: map[posterKey]time.Time{},
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
