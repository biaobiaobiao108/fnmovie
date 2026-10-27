package app

import "testing"

func TestLoadHomeCancelsOtherCatalogRequests(t *testing.T) {
	cancelled := 0
	searchKey := catalogStateKey("", "", "演员")
	movieKey := catalogStateKey("", "movie", "")
	homeKey := catalogStateKey("", "", "")
	search := &CatalogState{Loading: true, Cancel: func() { cancelled++ }}
	movies := &CatalogState{Loading: true, Cancel: func() { cancelled++ }}
	staleHome := &CatalogState{Loading: true, Cancel: func() { cancelled++ }}
	a := &appState{
		server:  NewServer("http://synthetic.invalid", ""),
		section: "home",
		catalogs: map[string]*CatalogState{
			searchKey: search,
			movieKey:  movies,
			homeKey:   staleHome,
		},
	}

	a.loadLibrary()

	if cancelled != 3 {
		t.Fatalf("cancelled %d stale catalog requests, want 3", cancelled)
	}
	for name, state := range map[string]*CatalogState{"search": search, "movies": movies, "stale home scope": staleHome} {
		if state.Loading || state.Cancel != nil {
			t.Errorf("%s request remains active after returning home: %+v", name, state)
		}
	}
}
