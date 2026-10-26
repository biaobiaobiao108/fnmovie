package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSecondaryPagesEnterWithMotion(t *testing.T) {
	for _, test := range []struct {
		name  string
		open  func(*appState)
		label string
	}{
		{"movie", func(a *appState) { a.openDetail(MediaItem{ID: "movie", Title: "测试电影"}) }, "测试电影"},
		{"person", func(a *appState) { a.openPerson(CastMember{ID: "person", Name: "测试演员"}) }, "测试演员"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &appState{section: "movies", catalogs: map[string]*CatalogState{}}
			tester := ui.NewTester(a.view, 1280, 800)
			tester.Frame()
			test.open(a)
			tester.Frame()
			start, ok := tester.Find(test.label)
			if !ok {
				t.Fatalf("page title %q not found", test.label)
			}
			tester.Advance(200 * time.Millisecond)
			end, ok := tester.Find(test.label)
			if !ok || start.Y-end.Y < 5 || start.Y-end.Y > 7 {
				t.Fatalf("page did not move into place: start=%+v end=%+v (found=%v)", start, end, ok)
			}
			if a.selected != nil {
				a.selected.Overview = "异步更新的简介"
			} else {
				a.personError = "异步更新的状态"
			}
			tester.Frame()
			updated, ok := tester.Find(test.label)
			if !ok || updated.Y != end.Y {
				t.Fatalf("metadata update restarted page motion: settled=%+v updated=%+v", end, updated)
			}
		})
	}
}

func TestDetailAndPersonNavigationMovesEachPage(t *testing.T) {
	a := &appState{section: "movies", catalogs: map[string]*CatalogState{}}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.Frame()
	a.openDetail(MediaItem{ID: "movie", Title: "测试电影"})
	tester.Frame()
	tester.Advance(200 * time.Millisecond)

	for _, step := range []struct {
		label string
		open  func()
	}{
		{"测试演员", func() { a.openPerson(CastMember{ID: "person", Name: "测试演员"}) }},
		{"测试电影", a.closePerson},
		{"这里还没有内容", a.closeDetail},
	} {
		step.open()
		tester.Frame()
		start, ok := tester.Find(step.label)
		if !ok {
			t.Fatalf("page title %q not found", step.label)
		}
		tester.Advance(200 * time.Millisecond)
		end, ok := tester.Find(step.label)
		if !ok || start.Y-end.Y < 5 || start.Y-end.Y > 7 {
			t.Fatalf("%s did not move into place: start=%+v end=%+v", step.label, start, end)
		}
	}
}

func TestSecondaryPageRespectsReducedMotion(t *testing.T) {
	a := &appState{section: "movies", catalogs: map[string]*CatalogState{}}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Frame()
	a.openDetail(MediaItem{ID: "movie", Title: "测试电影"})
	tester.Frame()
	start, ok := tester.Find("测试电影")
	if !ok {
		t.Fatal("movie title not found")
	}
	tester.Advance(200 * time.Millisecond)
	end, ok := tester.Find("测试电影")
	if !ok || start.Y != end.Y {
		t.Fatalf("page moved with reduced motion: start=%+v end=%+v", start, end)
	}
}
