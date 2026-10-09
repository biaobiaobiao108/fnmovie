package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDetailPartsPublishSeasonsWithoutWaitingForMetadataOrPeople(t *testing.T) {
	var active, maximum atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		if strings.Contains(r.URL.Path, "season/list") {
			writeJSON(t, w, `{"code":0,"data":[{"guid":"season","title":"Season 1","type":"Season"}]}`)
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	parts, done := make(chan detailPart, 3), make(chan struct{})
	go func() {
		fetchDetailParts(ctx, NewServer(server.URL, ""), MediaItem{ID: "show", Kind: "tv", IsSeries: true}, func(part detailPart) { parts <- part })
		close(done)
	}()
	select {
	case part := <-parts:
		if part.kind != "seasons" || part.err != nil || len(part.seasons) != 1 {
			t.Fatalf("first region=%+v", part)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("seasons waited for blocked metadata or credits")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("old detail requests did not cancel")
	}
	if len(parts) != 0 || maximum.Load() > 2 {
		t.Fatalf("late publications=%d concurrency=%d", len(parts), maximum.Load())
	}
}

func TestLeavingPagesCancelsRequestsAndAccountResetClearsActorCache(t *testing.T) {
	calls := 0
	a := &appState{
		detailCancel: func() { calls++ }, seriesEpisodeCancel: func() { calls++ },
		seriesCastCancel: func() { calls++ }, personCancel: func() { calls++ },
		personCache: newPersonItemsCache(),
	}
	a.personCache.Put("server", "user", "actor", []MediaItem{{ID: "movie"}})
	a.closeDetail()
	a.closePerson()
	a.resetHome()
	if calls != 4 {
		t.Fatalf("cancelled %d requests, want 4", calls)
	}
	if _, ok := a.personCache.Get("server", "user", "actor"); ok {
		t.Fatal("account reset retained actor cache")
	}
	// Repeated close is idempotent and cannot cancel a later account's work.
	a.closeDetail()
	a.closePerson()
	if calls != 4 {
		t.Fatal("close repeated old cancellation")
	}
}
