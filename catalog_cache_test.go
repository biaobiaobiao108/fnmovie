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

func TestCatalogCacheMigrationDropsOldHierarchyPagesButKeepsLibraries(t *testing.T) {
	disk := catalogDisk{
		Libraries: map[string][]MediaLibrary{"server": {{ID: "library", Name: "剧集"}}},
		Pages:     map[string]CatalogPage{"server:library": {Items: []MediaItem{{ID: "old", Kind: "tv"}}}},
		Details:   map[string]MediaItem{"server:old": {ID: "old"}},
	}
	migrateCatalogDisk(&disk)
	if disk.Version != catalogCacheVersion || len(disk.Pages) != 0 || len(disk.Details) != 0 {
		t.Fatalf("old hierarchy cache was not invalidated: %+v", disk)
	}
	if len(disk.Libraries["server"]) != 1 || disk.Libraries["server"][0].ID != "library" {
		t.Fatalf("library cache should be retained: %+v", disk.Libraries)
	}
}

func TestCatalogOnlyAutoLoadsMoreWhenProjectionIsEmptyAndCanContinue(t *testing.T) {
	state := &CatalogState{NextPage: 2}
	if !catalogNeedsMoreVisibleItems(0, state) {
		t.Fatal("empty projection with additional pages should continue loading")
	}
	state.Loading = true
	if catalogNeedsMoreVisibleItems(0, state) {
		t.Fatal("must not start duplicate page requests")
	}
	state.Loading, state.Exhausted = false, true
	if catalogNeedsMoreVisibleItems(0, state) {
		t.Fatal("exhausted catalog should show the empty state")
	}
}

func TestSystemCategoryCatalogScopesStaySeparateFromLibrariesAndEachOther(t *testing.T) {
	if catalogCacheScope("", "movie") == catalogCacheScope("", "tv") {
		t.Fatal("movie and TV system categories must have independent disk caches")
	}
	if catalogCacheScope("library", "tv") == catalogCacheScope("library", "") {
		t.Fatal("filtered personal library pages must not reuse the unfiltered cache")
	}
	if catalogStateKey("", "movie", "") == catalogStateKey("", "tv", "") {
		t.Fatal("movie and TV categories must not share in-memory request state")
	}
}
