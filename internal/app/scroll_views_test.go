package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSidebarVirtualizesAndKeepsItsPlaceAcrossPageChanges(t *testing.T) {
	a := &appState{section: "movies", catalogs: map[string]*CatalogState{}}
	for i := range 1000 {
		a.libraries = append(a.libraries, MediaLibrary{ID: fmt.Sprintf("lib-%d", i), Name: fmt.Sprintf("库条目-%d", i)})
	}
	tester := ui.NewTester(a.view, 1280, 800)
	assertBoundedRows(t, tester, "库条目-")
	tester.Wheel(80, 400, 0, 100)
	tester.Advance(200 * time.Millisecond)
	if a.sidebarScroll.Y <= 0 {
		t.Fatal("sidebar mouse wheel did not glide through its tracked position")
	}
	a.sidebarList.ScrollTo(700, ui.Start)
	tester.Frame()
	if !tester.HasText("库条目-700") {
		t.Fatal("virtual library row not built after scrolling to it")
	}
	a.selected = &MediaItem{ID: "movie", Title: "影片详情"}
	tester.Frame()
	if tester.HasText("库条目-700") || tester.HasText("首页") {
		t.Fatal("immersive detail should temporarily cover the sidebar")
	}
	tester.Click("返回影视库")
	if !tester.HasText("库条目-700") {
		t.Fatal("returning from detail should restore the sidebar position")
	}
	assertBoundedRows(t, tester, "库条目-")
}

func TestSeriesVirtualizesEpisodesAndPreservesAnchorDuringMetadataUpdates(t *testing.T) {
	series := MediaItem{ID: "series", Title: "长剧集", IsSeries: true, Seasons: []MediaSeason{{ID: "season", Title: "第一季"}}}
	a := &appState{selected: &series, selectedSeasonID: "season", catalogs: map[string]*CatalogState{}}
	for i := range 1000 {
		a.seriesEpisodes = append(a.seriesEpisodes, MediaItem{ID: fmt.Sprintf("ep-%d", i), Title: fmt.Sprintf("分集条目-%d", i), EpisodeNumber: i + 1})
	}
	tester := ui.NewTester(a.view, 1280, 800)
	assertBoundedRows(t, tester, "分集条目-")
	tester.Wheel(650, 500, 0, 100)
	tester.Advance(200 * time.Millisecond)
	if a.detailScroll.Y <= 0 {
		t.Fatal("episode list mouse wheel did not glide through its tracked position")
	}
	a.detailList.ScrollTo(703, ui.Start)
	tester.Frame()
	if !tester.HasText("分集条目-700") {
		t.Fatal("virtual episode row not built after scrolling to it")
	}
	a.selected.Cast = []CastMember{{ID: "actor", Name: "新增演员"}}
	tester.Frame()
	if !tester.HasText("分集条目-700") {
		t.Fatal("cast metadata update moved the episode anchor")
	}
	assertBoundedRows(t, tester, "分集条目-")
}

func TestMovieDetailUsesNativeWheelGlide(t *testing.T) {
	movie := MediaItem{ID: "movie", Title: "电影"}
	for i := range 40 {
		movie.Sources = append(movie.Sources, StreamSource{Name: fmt.Sprintf("版本-%d", i)})
	}
	a := &appState{selected: &movie, catalogs: map[string]*CatalogState{}}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.Wheel(650, 600, 0, 100)
	tester.Advance(75 * time.Millisecond)
	if a.detailScroll.Y <= 0 || a.detailScroll.Y >= 100 {
		t.Fatalf("movie detail mouse wheel should glide over frames, got %v", a.detailScroll.Y)
	}
}

func assertBoundedRows(t *testing.T, tester *ui.Tester, prefix string) {
	t.Helper()
	count := 0
	for _, text := range tester.Texts() {
		if strings.HasPrefix(text, prefix) {
			count++
		}
	}
	if count == 0 || count > 40 {
		t.Fatalf("long list should build visible rows plus overscan, got %d", count)
	}
}
