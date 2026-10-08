package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarIconsAreLoadedAndNonNil(t *testing.T) {
	requiredKeys := []string{"home", "movies", "tv", "favorites", "library", "login", "logout"}
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
