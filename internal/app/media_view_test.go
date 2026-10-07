package app

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

func TestSystemCategoryProjectionSeparatesMoviesAndTVPrograms(t *testing.T) {
	items := []MediaItem{
		{ID: "movie", Kind: "movie"},
		{ID: "show", Kind: "tv", IsSeries: true},
		{ID: "season", Kind: "season"},
		{ID: "episode", Kind: "episode"},
	}
	for section, want := range map[string]string{"movies": "movie", "tv": "show"} {
		got := deriveVisibleItems(items, mediaViewKey{Section: section})
		if len(got) != 1 || got[0].ID != want {
			t.Errorf("%s category = %#v, want only %q", section, got, want)
		}
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

func TestLibraryProjectionKeepsShowsAndMoviesButHidesSeasonAndEpisodeNodes(t *testing.T) {
	items := []MediaItem{
		{ID: "series", Kind: "tv", IsSeries: true, Title: "旅途"},
		{ID: "season-1", Kind: "season", Title: "第1季"},
		{ID: "s1e2", Kind: "episode", Title: "第二集", EpisodeNumber: 2, Raw: map[string]any{"parent_guid": "season-1"}},
		{ID: "movie", Kind: "movie", Title: "电影"},
	}
	got := deriveVisibleItems(items, mediaViewKey{Section: "library", LibraryID: "anime", GroupEpisodes: true})
	if len(got) != 2 {
		t.Fatalf("top-level library items = %d, want one show and one movie", len(got))
	}
	if got[0].ID != "series" || !got[0].IsSeries || got[1].ID != "movie" {
		t.Fatalf("unexpected top-level projection: %#v", got)
	}
}

func TestEpisodeAndPersonProjectionRemainsAvailableToFavorites(t *testing.T) {
	items := []MediaItem{
		{ID: "episode", Kind: "episode", SeriesTitle: "Show", Favorite: true},
		{ID: "actor", Kind: "person", Title: "新垣结衣", Favorite: true},
	}
	got := deriveVisibleItems(items, mediaViewKey{Section: "favorites", GroupEpisodes: true})
	if len(got) != 2 {
		t.Fatalf("favorites projection should keep both episode and person, got %d: %#v", len(got), got)
	}
}

func TestSeriesGroupsUseSeriesIDAcrossSeasonsAndKeepRemakesSeparate(t *testing.T) {
	items := []MediaItem{
		{ID: "s1e1", Kind: "tv", SeriesID: "series-a", SeriesTitle: "同名剧", SeasonNumber: 1, EpisodeNumber: 1},
		{ID: "s2e1", Kind: "tv", SeriesID: "series-a", SeriesTitle: "同名剧", SeasonNumber: 2, EpisodeNumber: 1},
		{ID: "remake", Kind: "tv", SeriesID: "series-b", SeriesTitle: "同名剧", SeasonNumber: 1, EpisodeNumber: 1},
	}
	got := groupSeriesEpisodes(items)
	if len(got) != 2 {
		t.Fatalf("expected two distinct shows, got %d: %+v", len(got), got)
	}
	if len(got[0].Episodes) != 2 || len(got[1].Episodes) != 1 {
		t.Fatalf("episodes from seasons/remake grouped incorrectly: %+v", got)
	}
}

func TestSeriesEpisodesWithoutNumberStillGroupByShow(t *testing.T) {
	items := []MediaItem{
		{ID: "one", Kind: "tv", SeriesTitle: "小镇", Raw: map[string]any{"season_number": "1"}},
		{ID: "two", Kind: "tv", SeriesTitle: "小镇", Raw: map[string]any{"season_number": "2"}},
	}
	got := groupSeriesEpisodes(items)
	if len(got) != 1 || !got[0].IsSeries || len(got[0].Episodes) != 2 {
		t.Fatalf("episodes with missing numbering should remain grouped: %+v", got)
	}
}

func TestNormalizeItemRecognizesTVCategoryWhenTypeIsGeneric(t *testing.T) {
	item := normalizeItem(map[string]any{"guid": "show", "title": "剧集", "type": "folder", "category": "TV"})
	if item.Kind != "tv" || !item.IsSeries {
		t.Fatalf("normalized show = %+v", item)
	}
}

func TestNormalizeItemInfersSeasonAndEpisodeFromParentIDs(t *testing.T) {
	season := normalizeItem(map[string]any{"guid": "season", "title": "第 1 季", "season_guid": "season"})
	episode := normalizeItem(map[string]any{"guid": "episode", "title": "第 1 集", "tv_guid": "show"})
	if season.Kind != "season" || episode.Kind != "episode" {
		t.Fatalf("inferred hierarchy kinds season=%q episode=%q", season.Kind, episode.Kind)
	}
}
