package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

func TestCatalogCacheMigrationDropsUnscopedAccountData(t *testing.T) {
	disk := catalogDisk{
		Libraries: map[string][]MediaLibrary{"server": {{ID: "library", Name: "剧集"}}},
		Pages:     map[string]CatalogPage{"server:library": {Items: []MediaItem{{ID: "old", Kind: "tv"}}}},
		Details:   map[string]MediaItem{"server:old": {ID: "old"}},
	}
	migrateCatalogDisk(&disk)
	if disk.Version != catalogCacheVersion || len(disk.Pages) != 0 || len(disk.Details) != 0 {
		t.Fatalf("old hierarchy cache was not invalidated: %+v", disk)
	}
	if len(disk.Libraries) != 0 {
		t.Fatalf("unscoped library cache must be discarded: %+v", disk.Libraries)
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

func TestCatalogCacheIsolatesAccountsOnSameServer(t *testing.T) {
	cache := &CatalogCache{data: catalogDisk{}}
	migrateCatalogDisk(&cache.data)
	server := "http://nas.example/v"
	item := MediaItem{ID: "shared", Title: "Alice's film", Favorite: true}
	cache.SetLibraries(server, []MediaLibrary{{ID: "private", Name: "Alice's library"}}, "alice")
	cache.SetPage(server, "@system:favorite", CatalogPage{Items: []MediaItem{item}}, "alice")
	cache.SetDetail(server, item, "alice")
	if len(cache.Libraries(server, "bob")) != 0 {
		t.Fatal("libraries leaked across accounts")
	}
	if _, ok := cache.Page(server, "@system:favorite", "bob"); ok {
		t.Fatal("favorites leaked across accounts")
	}
	if _, ok := cache.Detail(server, item.ID, "bob"); ok {
		t.Fatal("detail leaked across accounts")
	}
	item.Title, item.Favorite = "Bob's film", false
	cache.SetDetail(server, item, "bob")
	alice, _ := cache.Detail(server, item.ID, "alice")
	bob, _ := cache.Detail(server, item.ID, "bob")
	if alice.Title == bob.Title || !alice.Favorite || bob.Favorite {
		t.Fatalf("account-specific details overwritten: alice=%+v bob=%+v", alice, bob)
	}
}

func TestCatalogCacheEvictsOldestEntriesWithinEachBound(t *testing.T) {
	usage := map[string]time.Time{}
	check := func(kind string, keys []string) {
		t.Helper()
		for i, key := range keys {
			usage[catalogUsageKey(kind, key)] = time.Unix(int64(i+1), 0)
		}
	}
	keys := func(limit int) []string {
		result := make([]string, limit+1)
		for i := range result {
			result[i] = strconv.Itoa(i)
		}
		return result
	}

	libraryKeys := keys(catalogMaxLibrarySets)
	pageKeys := keys(catalogMaxPages)
	detailKeys := keys(catalogMaxDetails)
	check("libraries", libraryKeys)
	check("pages", pageKeys)
	check("details", detailKeys)

	libraries := make(map[string]int, len(libraryKeys))
	for _, key := range libraryKeys {
		libraries[key] = 1
	}
	pages := make(map[string]int, len(pageKeys))
	for _, key := range pageKeys {
		pages[key] = 1
	}
	details := make(map[string]int, len(detailKeys))
	for _, key := range detailKeys {
		details[key] = 1
	}
	trimCatalogEntries(libraries, usage, "libraries", catalogMaxLibrarySets)
	trimCatalogEntries(pages, usage, "pages", catalogMaxPages)
	trimCatalogEntries(details, usage, "details", catalogMaxDetails)

	for _, entry := range []struct {
		kind    string
		limit   int
		entries map[string]int
		oldest  string
		newest  string
	}{
		{"libraries", catalogMaxLibrarySets, libraries, libraryKeys[0], libraryKeys[len(libraryKeys)-1]},
		{"pages", catalogMaxPages, pages, pageKeys[0], pageKeys[len(pageKeys)-1]},
		{"details", catalogMaxDetails, details, detailKeys[0], detailKeys[len(detailKeys)-1]},
	} {
		if len(entry.entries) != entry.limit {
			t.Errorf("%s cache size = %d, want %d", entry.kind, len(entry.entries), entry.limit)
		}
		if _, ok := entry.entries[entry.oldest]; ok {
			t.Errorf("%s cache retained oldest entry %q", entry.kind, entry.oldest)
		}
		if _, ok := entry.entries[entry.newest]; !ok {
			t.Errorf("%s cache evicted newest entry %q", entry.kind, entry.newest)
		}
	}
	if len(usage) != catalogMaxLibrarySets+catalogMaxPages+catalogMaxDetails {
		t.Fatalf("eviction left stale usage records: %d", len(usage))
	}
}
