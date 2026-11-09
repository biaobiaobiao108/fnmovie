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
	initialBar := tester.Image().RGBAAt(1000, 20)
	if a.contentUnderTitleBar(movieTitleBarHeight) || initialBar.R != 247 || initialBar.G != 247 || initialBar.B != 246 {
		t.Fatalf("title bar should match the page before scrolling: pixel=%+v", initialBar)
	}
	initial, found := tester.Find("影片 0")
	if !found || initial.Y <= movieTitleBarHeight {
		t.Fatalf("first card starts under title bar: %+v (found=%v)", initial, found)
	}
	tester.Scroll(500, 400, 0, initial.Y-20)
	if !a.contentUnderTitleBar(movieTitleBarHeight) {
		t.Fatal("title bar did not activate when content scrolled underneath")
	}
	if pixel := tester.Image().RGBAAt(1000, 20); pixel == initialBar {
		t.Fatalf("title bar did not render its scroll effect: pixel=%+v", pixel)
	}
	scrolled, found := tester.Find("影片 0")
	if !found || scrolled.Y >= movieTitleBarHeight {
		t.Fatalf("card did not scroll into title bar: initial=%+v scrolled=%+v (found=%v), offset=%v", initial, scrolled, found, a.catalogScroll.Y)
	}
	tester.Scroll(500, 400, 0, -initial.Y)
	if a.contentUnderTitleBar(movieTitleBarHeight) {
		t.Fatal("title bar remained active after returning to the top")
	}
	if pixel := tester.Image().RGBAAt(1000, 20); pixel != initialBar {
		t.Fatalf("title bar did not return to the page color: initial=%+v restored=%+v", initialBar, pixel)
	}
	restored, found := tester.Find("影片 0")
	if !found || restored.Y != initial.Y {
		t.Fatalf("card did not return to its starting position: initial=%+v restored=%+v (found=%v)", initial, restored, found)
	}
}

func TestHomeTitleBarTracksContinueGrid(t *testing.T) {
	a := &appState{section: "home"}
	a.home.GridScroll.Y = 100
	if a.contentUnderTitleBar(movieTitleBarHeight) {
		t.Fatal("home hero should not activate the title bar")
	}
	a.home.AllContinue = true
	a.home.Continue = []ContinueWatchingItem{{}}
	a.home.GridScroll.Y = 16
	if a.contentUnderTitleBar(movieTitleBarHeight) {
		t.Fatal("title bar activated before the continue heading reached it")
	}
	a.home.GridScroll.Y = 17
	if !a.contentUnderTitleBar(movieTitleBarHeight) {
		t.Fatal("title bar did not activate when the continue heading reached it")
	}
}
