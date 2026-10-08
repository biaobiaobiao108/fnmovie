package app

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestGlobalSearchScopeAndProjectionFromEveryPage(t *testing.T) {
	items := []MediaItem{
		{ID: "movie", Title: "搜索电影", Kind: "movie", Raw: map[string]any{"library_guid": "other"}},
		{ID: "tv", Title: "搜索剧集", Kind: "tv", IsSeries: true},
		{ID: "person", Title: "搜索演员", Kind: "person"},
		{ID: "episode", Title: "搜索分集", Kind: "episode"},
	}
	for _, section := range []string{"home", "movies", "tv", "favorites", "library", "settings"} {
		t.Run(section, func(t *testing.T) {
			a := &appState{section: section, libraryID: "selected", query: " keyword ", selectedTab: 2,
				catalogs: map[string]*CatalogState{catalogStateKey("", "", "keyword"): {Items: items}}}
			lib, kind, query := a.catalogRequestScope()
			if lib != "" || kind != "" || query != "keyword" {
				t.Fatalf("search inherited page scope: %q %q %q", lib, kind, query)
			}
			if got := a.visibleItems(); !reflect.DeepEqual(got, items) {
				t.Fatalf("global results filtered or reordered: %+v", got)
			}
			if a.navigationKey() != "search" {
				t.Fatal("search must share one navigation container")
			}
		})
	}
}

func TestClearingGlobalSearchRestoresLibraryScope(t *testing.T) {
	local := MediaItem{ID: "local", Kind: "movie", Raw: map[string]any{"library_guid": "selected"}}
	a := &appState{section: "library", libraryID: "selected", query: "keyword",
		libraries: []MediaLibrary{{ID: "selected", Kind: "movie"}},
		catalogs: map[string]*CatalogState{
			catalogStateKey("", "", "keyword"):       {Items: []MediaItem{{ID: "global", Kind: "tv"}}},
			catalogStateKey("selected", "movie", ""): {Items: []MediaItem{local, {ID: "foreign", Kind: "movie", Raw: map[string]any{"library_guid": "other"}}}},
		}}
	_ = a.visibleItems()
	a.query = " "
	lib, kind, query := a.catalogRequestScope()
	if lib != "selected" || kind != "movie" || query != "" {
		t.Fatalf("browse scope not restored: %q %q %q", lib, kind, query)
	}
	if got := a.visibleItems(); !reflect.DeepEqual(got, []MediaItem{local}) {
		t.Fatalf("browse isolation not restored: %+v", got)
	}
}

func TestGlobalSearchDoesNotShowPreviousCatalogDuringDebounce(t *testing.T) {
	a := &appState{section: "movies", query: "new query", items: []MediaItem{{ID: "old", Kind: "movie"}}}
	if got := a.visibleItems(); len(got) != 0 {
		t.Fatalf("stale browse cards shown as search results: %+v", got)
	}
}

func TestGlobalSearchUIFromHomeAndSettings(t *testing.T) {
	for _, section := range []string{"home", "settings"} {
		t.Run(section, func(t *testing.T) {
			a := &appState{section: section, query: "keyword", loggedIn: true,
				catalogs: map[string]*CatalogState{catalogStateKey("", "", "keyword"): {
					Items: []MediaItem{{ID: "result", Title: "全局结果影片", Kind: "movie"}}, Exhausted: true,
				}}}
			tester := ui.NewTester(a.view, 1280, 800)
			if !tester.HasText("全局结果影片") || !tester.HasText("全局搜索") {
				t.Fatalf("search results not rendered from %s: %v", section, tester.Texts())
			}
			if tester.HasText("继续观看") {
				t.Fatal("home carousel remained visible during search")
			}
		})
	}
}

func TestGlobalSearchInputLeavesDetails(t *testing.T) {
	for _, person := range []bool{false, true} {
		a := &appState{section: "library", libraryID: "selected", loggedIn: true,
			catalogs: map[string]*CatalogState{}}
		if person {
			a.selectedPerson = &CastMember{ID: "actor", Name: "演员详情"}
		} else {
			a.selected = &MediaItem{ID: "movie", Title: "影片详情", Kind: "movie"}
		}
		tester := ui.NewTester(a.view, 1280, 800)
		_, ok := tester.Find("全局搜索")
		if !ok {
			t.Fatal("detail page missing global search input")
		}
		tester.Click("全局搜索")
		tester.Type("keyword")
		if a.query != "keyword" || a.selected != nil || a.selectedPerson != nil || !a.searchPending {
			t.Fatalf("detail search did not switch to global results: query=%q, pending=%v", a.query, a.searchPending)
		}
		if !tester.HasText("正在搜索…") {
			t.Fatal("search debounce missing visible pending state")
		}
	}
}
