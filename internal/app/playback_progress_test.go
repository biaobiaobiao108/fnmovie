package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestPlaybackProgressFinalOrdersSessionsAndCoalescesSnapshots(t *testing.T) {
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	seen := make(chan int64, 8)
	var first sync.Once
	var release sync.Once
	releaseHandler := func() { release.Do(func() { close(releaseFirst) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TS int64 `json:"ts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode progress request: %v", err)
		}
		seen <- body.TS
		if body.TS == 10 {
			first.Do(func() { close(firstEntered) })
			select {
			case <-releaseFirst:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(releaseHandler)

	client := NewServer(server.URL, "token")
	client.client = server.Client()
	reporter := newPlaybackProgressReporter()
	old := reporter.newSession(client, MediaItem{ID: "movie", MediaID: "media"}, nil)
	old.periodic(10, 100)
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first periodic update did not start")
	}
	for position := 20; position < 1000; position++ {
		old.periodic(float64(position), 100)
	}
	finalDone := old.finish(30, 100, time.Second)
	newSession := reporter.newSession(client, MediaItem{ID: "movie", MediaID: "media"}, nil)
	newSession.periodic(40, 100)
	reporter.mu.Lock()
	queued := len(reporter.queue)
	reporter.mu.Unlock()
	if queued != 2 {
		t.Fatalf("queued %d reports after coalescing, want old final and one new snapshot", queued)
	}
	releaseHandler()

	want := []int64{10, 30, 40}
	for _, expected := range want {
		select {
		case got := <-seen:
			if got != expected {
				t.Fatalf("progress order=%d, want %d", got, expected)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for progress position %d", expected)
		}
	}
	select {
	case err := <-finalDone:
		if err != nil {
			t.Fatalf("final progress: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("final progress did not complete")
	}
	reporter.close(context.Background())
}

func TestPlaybackProgressTimeoutStartsWhenReportIsSent(t *testing.T) {
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})
	var release sync.Once
	releaseHandler := func() { release.Do(func() { close(releaseFirst) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TS int64 `json:"ts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body.TS {
		case 1:
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-r.Context().Done():
				return
			}
		case 2:
			close(secondEntered)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(releaseHandler)

	client := NewServer(server.URL, "token")
	client.client = server.Client()
	reporter := newPlaybackProgressReporter()
	first := reporter.newSession(client, MediaItem{ID: "one"}, nil)
	first.periodic(1, 10)
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first progress did not start")
	}
	second := reporter.newSession(client, MediaItem{ID: "two"}, nil)
	done := second.finish(2, 10, 300*time.Millisecond)
	time.Sleep(350 * time.Millisecond)
	releaseHandler()
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("final request was incorrectly expired while waiting in the queue")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("queued final report expired before it was sent: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued final report did not complete")
	}
	reporter.close(context.Background())
}

func TestPlaybackProgressCloseCancelsInflightAndQueuedFinals(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
		return nil, r.Context().Err()
	})
	client := NewServer("http://synthetic.invalid", "token")
	client.client = &http.Client{Transport: transport}
	reporter := newPlaybackProgressReporter()
	first := reporter.newSession(client, MediaItem{ID: "one"}, nil)
	firstDone := first.finish(1, 10, 10*time.Second)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("final progress did not start")
	}
	second := reporter.newSession(client, MediaItem{ID: "two"}, nil)
	queuedDone := second.finish(2, 10, 10*time.Second)
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reporter.close(closeCtx)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("close did not cancel the in-flight request")
	}
	for label, done := range map[string]<-chan error{"in-flight final": firstDone, "queued final": queuedDone} {
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s unexpectedly succeeded after reporter close", label)
			}
		case <-time.After(time.Second):
			t.Errorf("%s waiter leaked after reporter close", label)
		}
	}
	select {
	case <-reporter.workerDone:
	default:
		t.Fatal("reporter worker remained alive after close")
	}
}

func TestPlaybackProgressCloseWaitIsBoundedForUnresponsiveTransport(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandler)
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release // Deliberately ignores request cancellation.
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":0}`)),
		}, nil
	})
	client := NewServer("http://synthetic.invalid", "token")
	client.client = &http.Client{Transport: transport}
	reporter := newPlaybackProgressReporter()
	first := reporter.newSession(client, MediaItem{ID: "one"}, nil)
	inFlightDone := first.finish(1, 10, 10*time.Second)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("periodic request did not start")
	}
	second := reporter.newSession(client, MediaItem{ID: "two"}, nil)
	queuedDone := second.finish(2, 10, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	reporter.close(ctx)
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("reporter close exceeded its deadline: %v", elapsed)
	}
	select {
	case err := <-inFlightDone:
		if err == nil {
			t.Fatal("in-flight final report was not canceled during shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight final report waiter leaked")
	}
	select {
	case err := <-queuedDone:
		if err == nil {
			t.Fatal("queued final report was not released during shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("queued final report waiter leaked")
	}
	releaseHandler()
	select {
	case <-reporter.workerDone:
	case <-time.After(time.Second):
		t.Fatal("reporter worker did not exit after the transport returned")
	}
}
