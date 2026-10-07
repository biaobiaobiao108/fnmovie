package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogCachePersistsDetailAndCastByServer(t *testing.T) {
	cache := NewCatalogCache()
	cache.path = filepath.Join(t.TempDir(), "catalog.json")
	item := MediaItem{ID: "movie-1", Title: "Film", Poster: "/poster.jpg", Cast: []CastMember{{ID: "person-1", Name: "Actor", Role: "Lead"}}}
	cache.SetDetail("http://nas.example/v", item)

	reloaded := &CatalogCache{path: cache.path, data: catalogDisk{}}
	if raw, err := os.ReadFile(cache.path); err == nil {
		if err := json.Unmarshal(raw, &reloaded.data); err != nil {
			t.Fatal(err)
		}
	}
	if reloaded.data.Details == nil {
		t.Fatal("detail cache was not persisted")
	}
	got, ok := reloaded.Detail("http://nas.example/v", "movie-1")
	if !ok || got.Title != item.Title || got.Poster != item.Poster || len(got.Cast) != 1 || got.Cast[0].Role != "Lead" {
		t.Fatalf("cached detail = %#v, found=%t", got, ok)
	}
	if _, ok := reloaded.Detail("http://other-nas.example/v", "movie-1"); ok {
		t.Fatal("detail cache leaked across server URLs")
	}
}
