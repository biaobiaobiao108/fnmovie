package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestContinueWatchingProgressPrioritizesVisibleAndPreservesOrder(t *testing.T) {
	firstRelease := make(chan struct{})
	firstEntered := make(chan struct{})
	var active, peak, tail atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/play/list" {
			items := make([]map[string]any, 12)
			for i := range items {
				items[i] = map[string]any{"guid": strconv.Itoa(i), "title": fmt.Sprintf("Film %d", i), "type": "Movie", "duration": 100}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": items})
			return
		}
		var body struct {
			GUID string `json:"item_guid"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		index, _ := strconv.Atoi(body.GUID)
		if index != 0 {
			select {
			case <-firstEntered:
			case <-r.Context().Done():
				return
			}
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		if index >= 10 {
			tail.Add(1)
		}
		if index == 0 {
			close(firstEntered)
			select {
			case <-firstRelease:
			case <-r.Context().Done():
				return
			}
		}
		writeJSON(t, w, `{"code":0,"data":{"ts":12}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	progress := make(chan []ContinueWatchingItem, 12)
	done := make(chan error, 1)
	var final []ContinueWatchingItem
	go func() {
		var err error
		final, err = NewServer(server.URL, "token").ContinueWatchingProgressContext(ctx, func(items []ContinueWatchingItem) { progress <- items })
		done <- err
	}()
	<-firstEntered
	select {
	case partial := <-progress:
		if len(partial) == 0 || partial[0].RecordGUID != "1" {
			t.Fatalf("unconfirmed position exposed: %+v", partial)
		}
		partial[0].Media.Title = "modified snapshot"
	case <-ctx.Done():
		t.Fatal("no partial results while first position was pending")
	}
	if tail.Load() != 0 {
		t.Fatal("history tail requested before visible ten completed")
	}
	close(firstRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 || len(final) != 12 {
		t.Fatalf("peak=%d items=%d", peak.Load(), len(final))
	}
	for i, item := range final {
		if item.RecordGUID != strconv.Itoa(i) || item.Media.Title != fmt.Sprintf("Film %d", i) || item.Position != 12 {
			t.Fatalf("record %d: %+v", i, item)
		}
	}
	for len(progress) > 0 {
		previous := -1
		for _, item := range <-progress {
			index, _ := strconv.Atoi(item.RecordGUID)
			if index <= previous {
				t.Fatal("partial results changed server order")
			}
			previous = index
		}
	}
}

func TestContinueWatchingProgressCancelStopsPublication(t *testing.T) {
	entered := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/play/list" {
			writeJSON(t, w, `{"code":0,"data":[{"guid":"first","title":"First"},{"guid":"second","title":"Second"}]}`)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var publications atomic.Int32
	go func() {
		_, err := NewServer(server.URL, "token").ContinueWatchingProgressContext(ctx, func([]ContinueWatchingItem) { publications.Add(1) })
		done <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("position request not started")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not stop requests")
	}
	if publications.Load() != 0 {
		t.Fatal("published canceled results")
	}
}

func TestPlaybackContextCancelsStreamRequest(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/play/info" {
			writeJSON(t, w, `{"code":0,"data":{"media_guid":"media"}}`)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewServer(server.URL, "token").PlaybackContext(ctx, MediaItem{ID: "movie"})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("stream request not started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream request not canceled")
	}
}
