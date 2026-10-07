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

func TestEpisodicMediaCollapseIntoSeriesCardsInAnyLibrary(t *testing.T) {
	items := []MediaItem{
		{ID: "s1e2", Kind: "tv", SeriesTitle: "旅途", SeasonNumber: 1, EpisodeNumber: 2, Year: "2010", Raw: map[string]any{"parent_guid": "season-1"}},
		{ID: "s2e1", Kind: "tv", SeriesTitle: "旅途", SeasonNumber: 2, EpisodeNumber: 1, Year: "2011", Raw: map[string]any{"parent_guid": "season-2"}},
		{ID: "other", Kind: "tv", SeriesTitle: "别的", EpisodeNumber: 1},
		{ID: "movie", Kind: "movie", Title: "电影"},
	}
	got := deriveVisibleItems(items, mediaViewKey{Section: "library", LibraryID: "anime", GroupEpisodes: true})
	if len(got) != 3 {
		t.Fatalf("grouped items = %d, want two shows and the movie", len(got))
	}
	series := got[0]
	if !series.IsSeries || series.Title != "旅途" || len(series.Episodes) != 2 {
		t.Fatalf("series projection = %#v", series)
	}
	if series.Episodes[0].ID != "s1e2" || series.Episodes[1].ID != "s2e1" {
		t.Fatalf("episodes not retained in source order: %#v", series.Episodes)
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
