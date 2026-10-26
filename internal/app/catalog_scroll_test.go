package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCatalogScrollsBehindTitleBar(t *testing.T) {
	a := &appState{section: "movies", catalogs: map[string]*CatalogState{}}
	for i := 0; i < 80; i++ {
		a.items = append(a.items, MediaItem{ID: fmt.Sprintf("movie-%d", i), Title: fmt.Sprintf("影片 %d", i), Kind: "movie"})
	}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Frame()
	tester.Frame()
	initial, found := tester.Find("影片 0")
	if !found || initial.Y <= movieTitleBarHeight {
		t.Fatalf("first card starts under title bar: %+v (found=%v)", initial, found)
	}
	tester.Scroll(500, 400, 0, initial.Y-20)
	scrolled, found := tester.Find("影片 0")
	if !found || scrolled.Y >= movieTitleBarHeight {
		t.Fatalf("card did not scroll into title bar: initial=%+v scrolled=%+v (found=%v), offset=%v", initial, scrolled, found, a.catalogScroll.Y)
	}
	tester.Scroll(500, 400, 0, -initial.Y)
	restored, found := tester.Find("影片 0")
	if !found || restored.Y != initial.Y {
		t.Fatalf("card did not return to its starting position: initial=%+v restored=%+v (found=%v)", initial, restored, found)
	}
}
