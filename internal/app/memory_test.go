package app

import (
	"fmt"
	"testing"
	"time"
)

func TestCatalogStatesKeepRecentAndActiveResults(t *testing.T) {
	a := &appState{catalogs: map[string]*CatalogState{}}
	cancelled := false
	for i := 0; i < 12; i++ {
		a.catalogs[fmt.Sprint(i)] = &CatalogState{LastUsed: time.Unix(int64(i), 0)}
	}
	a.catalogs["1"].Cancel = func() { cancelled = true }
	active := a.catalogs["0"]
	a.trimCatalogStates("0")
	if len(a.catalogs) != catalogStateLimit || a.catalogs["0"] != active || !cancelled {
		t.Fatalf("wrong retained states: count=%d active=%v cancelled=%v", len(a.catalogs), a.catalogs["0"] == active, cancelled)
	}
	for _, key := range []string{"1", "2", "3", "4"} {
		if a.catalogs[key] != nil {
			t.Fatalf("old state %s retained", key)
		}
	}
	if a.catalogs["11"] == nil {
		t.Fatal("recent result evicted")
	}
}

func TestCloseDetailReleasesEpisodesAndInvalidatesRequests(t *testing.T) {
	a := &appState{selected: &MediaItem{}, seriesEpisodes: []MediaItem{{ID: "episode"}},
		seriesEpisodeCache: map[string][]MediaItem{"season": {{ID: "episode"}}},
		seriesRootCast:     []CastMember{{}}, seriesEpisodeRequest: 2, seriesCastRequest: 3}
	a.closeDetail()
	if a.selected != nil || a.seriesEpisodes != nil || a.seriesEpisodeCache != nil || a.seriesRootCast != nil {
		t.Fatal("closed detail retained derived metadata")
	}
	if a.seriesEpisodeRequest != 3 || a.seriesCastRequest != 4 {
		t.Fatal("pending requests still valid")
	}
}
