package main

import "testing"

func TestDerivedVisibleItemsFiltersByLibraryAndTab(t *testing.T) {
	items := []MediaItem{
		{ID: "a", Kind: "movie", Favorite: true, AddedAt: "3", Raw: map[string]any{"ancestor_guid": "library-a"}},
		{ID: "b", Kind: "movie", Favorite: false, AddedAt: "2", Raw: map[string]any{"ancestor_guid": "library-b"}},
		{ID: "c", Kind: "tv", Favorite: true, AddedAt: "1", Raw: map[string]any{"ancestor_guid": "library-a"}},
	}
	got := deriveVisibleItems(items, mediaViewKey{Section: "library", LibraryID: "library-a", Tab: 1})
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("library movie projection = %#v, want only movie a", got)
	}
	got = deriveVisibleItems(items, mediaViewKey{Section: "favorites", Tab: 2})
	if len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("favorite TV projection = %#v, want only TV c", got)
	}
}

func TestMediaViewCacheReusesAndInvalidatesProjection(t *testing.T) {
	cache := mediaViewCache{}
	key := mediaViewKey{Revision: 1, Section: "home"}
	items := []MediaItem{{ID: "movie", Kind: "movie"}}
	first := cache.Get(key, items)
	second := cache.Get(key, items)
	if len(first) != 1 || len(second) != 1 || &first[0] != &second[0] {
		t.Fatal("same key should reuse the cached projection")
	}
	items[0].Favorite = true
	if stale := cache.Get(key, items); stale[0].Favorite {
		t.Fatal("a cached projection should remain unchanged until invalidated")
	}
	cache.Invalidate()
	updated := cache.Get(key, items)
	if len(updated) != 1 || !updated[0].Favorite {
		t.Fatalf("invalidated projection did not reflect data change: %#v", updated)
	}
}
