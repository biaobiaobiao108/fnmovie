package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestGlobalSearchPageContextKeepsAllResourceTypesAndServerOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search/list" {
			t.Errorf("unexpected search request: %s %s", r.Method, r.URL)
		}
		if query := r.URL.Query(); len(query) != 1 || query.Get("q") != "keyword" {
			t.Errorf("global search must contain only the trimmed query: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "synthetic-token" {
			t.Error("search missing authentication")
		}
		writeJSON(t, w, `{"code":0,"data":{"list":[{"guid":"film","title":"Film","type":"Movie","is_favorite":false},{"guid":"series","title":"Series","type":"TV"},{"guid":"actor","name":"Actor","type":"Person"},{"guid":"episode","title":"Episode","type":"Episode"}]}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "synthetic-token")
	for _, tc := range []struct {
		page int
		ids  []string
	}{
		{1, []string{"film", "series"}},
		{2, []string{"actor", "episode"}},
		{3, []string{}},
	} {
		items, total, err := client.GlobalSearchPageContext(t.Context(), "  keyword  ", tc.page, 2)
		if err != nil || total != 4 {
			t.Fatalf("page %d: total=%d err=%v", tc.page, total, err)
		}
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Errorf("page %d IDs=%v, want %v", tc.page, ids, tc.ids)
		}
	}
	items, _, err := client.GlobalSearchPageContext(t.Context(), "keyword", 0, 0)
	if err != nil || len(items) != 4 || items[0].Favorite || items[2].Kind != "person" || items[3].Kind != "episode" {
		t.Fatalf("global search changed resource or favorite semantics: items=%v err=%v", items, err)
	}
}

func TestGlobalSearchPageContextRejectsBlankQueryWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writeJSON(t, w, `{"code":0,"data":[]}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "synthetic-token")
	for _, query := range []string{"", " \t\n "} {
		if _, _, err := client.GlobalSearchPageContext(t.Context(), query, 1, 100); err == nil {
			t.Fatal("empty query must not enumerate the full catalog")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("blank query sent a server request")
	}
}

func TestGlobalSearchPageContextAuthenticationAndCancellation(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(t, w, `{"code":401,"msg":"authentication required"}`)
		}))
		defer server.Close()
		if _, _, err := NewServer(server.URL, "synthetic-token").GlobalSearchPageContext(t.Context(), "keyword", 1, 100); err == nil {
			t.Fatal("authentication failure should be returned")
		}
	})
	t.Run("cancel in flight", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, _, err := NewServer(server.URL, "synthetic-token").GlobalSearchPageContext(ctx, "keyword", 1, 100)
			result <- err
		}()
		<-started
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled search err=%v", err)
		}
	})
}
