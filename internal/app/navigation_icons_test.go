package app

import (
	"container/list"
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarIconsAreLoadedAndNonNil(t *testing.T) {
	requiredKeys := []string{"home", "search", "movies", "tv", "favorites", "library", "login", "logout"}
	for _, key := range requiredKeys {
		icon := sidebarNavIcon(key)
		if icon == nil {
			t.Fatalf("expected sidebar icon %q to be loaded, got nil", key)
		}
	}
}

func TestLibraryNavigationIconsResolveCorrectly(t *testing.T) {
	cases := []struct {
		kind     string
		expected *ui.SVG
	}{
		{"movie", sidebarNavIcon("movies")},
		{"Movie", sidebarNavIcon("movies")},
		{"movies", sidebarNavIcon("movies")},
		{"film", sidebarNavIcon("movies")},
		{"tv", sidebarNavIcon("tv")},
		{"TV", sidebarNavIcon("tv")},
		{"series", sidebarNavIcon("tv")},
		{"show", sidebarNavIcon("tv")},
		{"other", sidebarNavIcon("library")},
		{"", sidebarNavIcon("library")},
	}

	for _, tc := range cases {
		lib := MediaLibrary{Kind: tc.kind}
		got := libraryNavigationIcon(lib)
		if got != tc.expected {
			t.Errorf("for kind %q, expected icon %v, got %v", tc.kind, tc.expected, got)
		}
	}
}

func TestSidebarRendersAllIconsWithoutPanic(t *testing.T) {
	a := &appState{
		section:  "movies",
		loggedIn: true,
		libraries: []MediaLibrary{
			{ID: "m1", Name: "电影库", Kind: "movie"},
			{ID: "t1", Name: "电视库", Kind: "tv"},
		},
		catalogs: map[string]*CatalogState{},
	}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.Frame()

	// Switch to logged out state and re-render
	a.loggedIn = false
	tester.Frame()
}

// A library displays its loaded cover and keeps its category icon when no cover is available.
func TestSidebarUsesLibraryCoversWhenAvailable(t *testing.T) {
	server := NewServer("http://synthetic.invalid", "token")
	coverURL := server.imageURL("/cover.jpg")
	cover := image.NewRGBA(image.Rect(0, 0, 22, 22))
	for y := range 22 {
		for x := range 22 {
			cover.SetRGBA(x, y, color.RGBA{R: 180, G: 40, B: 120, A: 255})
		}
	}
	posters := &PosterLoader{entries: map[posterKey]*list.Element{}, lru: list.New(), maxBytes: posterCacheBudget}
	posters.putLocked(posterKey{URL: coverURL, Width: 22, Height: 22}, ui.NewBitmap(cover), 22*22*4)
	a := &appState{
		section: "movies", loggedIn: true, server: server, posters: posters,
		libraries: []MediaLibrary{
			{ID: "movies", Name: "电影库", Kind: "movie", Poster: "/cover.jpg"},
			{ID: "series", Name: "剧集库", Kind: "tv"},
		},
		catalogs: map[string]*CatalogState{},
	}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.Frame()
	if _, ok := tester.Find("影视库封面：电影库"); !ok {
		t.Fatal("library cover image was not rendered")
	}
	if _, ok := tester.Find("影视库类型：剧集库"); !ok {
		t.Fatal("library without a cover did not render its category icon")
	}
}
