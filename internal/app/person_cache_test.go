package app

import (
	"fmt"
	"testing"
	"time"
)

func TestPersonItemsCacheScopeAndCopies(t *testing.T) {
	cache := newPersonItemsCache()
	items := []MediaItem{{ID: "film", Genres: []string{"drama"}, Raw: map[string]any{"nested": map[string]any{"title": "original"}}}}
	cache.Put("server", "account", "person", items)
	items[0].Genres[0] = "changed"
	items[0].Raw["nested"].(map[string]any)["title"] = "changed"
	for _, scope := range [][3]string{{"other", "account", "person"}, {"server", "other", "person"}, {"server", "account", "other"}} {
		if _, ok := cache.Get(scope[0], scope[1], scope[2]); ok {
			t.Fatalf("cache crossed scope %v", scope)
		}
	}
	cached, ok := cache.Get("server", "account", "person")
	if !ok || cached[0].Genres[0] != "drama" || cached[0].Raw["nested"].(map[string]any)["title"] != "original" {
		t.Fatalf("stored cache aliases source: %v", cached)
	}
	cached[0].Genres[0] = "read mutation"
	cached[0].Raw["nested"].(map[string]any)["title"] = "read mutation"
	again, _ := cache.Get("server", "account", "person")
	if again[0].Genres[0] != "drama" || again[0].Raw["nested"].(map[string]any)["title"] != "original" {
		t.Fatal("read cache aliases returned items")
	}
}

func TestPersonItemsCacheBoundsAndLRU(t *testing.T) {
	cache := newPersonItemsCache()
	items := make([]MediaItem, personCacheItemLimit+30)
	for index := range items {
		items[index].ID = fmt.Sprint(index)
	}
	for index := 0; index < personCacheLimit; index++ {
		cache.Put("server", "account", fmt.Sprint(index), items)
	}
	if cached, ok := cache.Get("server", "account", "0"); !ok || len(cached) != personCacheItemLimit || cached[len(cached)-1].ID != "119" {
		t.Fatal("cache did not preserve bounded prefix")
	}
	cache.Put("server", "account", "new", nil)
	if len(cache.entries) != personCacheLimit {
		t.Fatalf("scope count %d exceeds limit", len(cache.entries))
	}
	if _, ok := cache.Get("server", "account", "1"); ok {
		t.Fatal("least recently used scope was retained")
	}
	if _, ok := cache.Get("server", "account", "0"); !ok {
		t.Fatal("recently read scope was evicted")
	}
	if cached, ok := cache.Get("server", "account", "new"); !ok || len(cached) != 0 {
		t.Fatal("successful empty result was not cached")
	}
}

func TestPersonItemsCacheExpiryAndClear(t *testing.T) {
	cache := newPersonItemsCache()
	now := time.Unix(1000, 0)
	cache.clock = func() time.Time { return now }
	cache.Put("server", "account", "person", []MediaItem{{ID: "film"}})
	now = now.Add(personCacheTTL - time.Second)
	if _, ok := cache.Get("server", "account", "person"); !ok {
		t.Fatal("cache expired too soon")
	}
	now = now.Add(time.Second)
	if _, ok := cache.Get("server", "account", "person"); ok {
		t.Fatal("reading cache extended its freshness")
	}
	cache.Put("server", "account", "old", nil)
	now = now.Add(personCacheTTL)
	cache.Put("server", "account", "new", nil)
	if len(cache.entries) != 1 {
		t.Fatal("put did not discard expired scopes")
	}
	cache.Clear()
	if _, ok := cache.Get("server", "account", "new"); ok || len(cache.entries) != 0 {
		t.Fatal("clear retained account data")
	}
}
