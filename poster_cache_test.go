package main

import (
	"container/list"
	"image"
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
