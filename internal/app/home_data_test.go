package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiveNASHomeReadOnly(t *testing.T) {
	if os.Getenv("FNMOVIE_LIVE_HOME_TEST") != "1" {
		t.Skip("opt in to read-only NAS home verification")
	}
	base, user, password := os.Getenv("FNMOVIE_SERVER"), os.Getenv("FNMOVIE_USER"), os.Getenv("FNMOVIE_PASSWORD")
	if base == "" || user == "" || password == "" {
		t.Fatal("provide NAS credentials through environment variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	server := NewServer(base, "")
	if _, err := server.LoginContext(ctx, user, password); err != nil {
		t.Fatal(err)
	}
	libraries, err := server.LibrariesContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	heroes, err := server.HomeHighlightsContext(ctx, libraries, 12345)
	if err != nil || len(heroes) == 0 || len(heroes) > 8 {
		t.Fatalf("home highlights count=%d error=%v", len(heroes), err)
	}
	for _, item := range heroes {
		if item.ID == "" || item.Title == "" || item.Kind != "movie" && item.Kind != "tv" {
			t.Fatal("invalid home highlight")
		}
	}
	records, err := server.ContinueWatchingContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read-only NAS verification: %d highlights, %d continue records", len(heroes), len(records))
}

func TestContinueWatchingRecordsAndDetailHierarchy(t *testing.T) {
	var infoRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/play/list":
			writeJSON(t, w, `{"code":0,"data":[{"guid":"episode","title":"Episode title","type":"Episode","parent_guid":"season","tv_title":"Series","ts":25,"duration":100,"single_child_guid":"do-not-use"},{"guid":"movie","title":"Movie","type":"Movie","ts":-2},{"title":"Invalid"}]}`)
		case "/api/v1/play/info":
			var body struct {
				ItemGUID string `json:"item_guid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode play info request: %v", err)
			}
			infoRequests = append(infoRequests, body.ItemGUID)
			switch body.ItemGUID {
			case "episode":
				writeJSON(t, w, `{"code":0,"data":{"ts":125}}`)
			case "movie":
				writeJSON(t, w, `{"code":0,"data":{"ts":42}}`)
			default:
				t.Errorf("unexpected play info item GUID %q", body.ItemGUID)
				writeJSON(t, w, `{"code":0,"data":{}}`)
			}
		case "/api/v1/item/episode":
			writeJSON(t, w, `{"code":0,"data":{"guid":"episode","title":"Episode title","type":"Episode","parent_guid":"season"}}`)
		case "/api/v1/item/season":
			writeJSON(t, w, `{"code":0,"data":{"guid":"season","title":"Season","type":"Season","parent_guid":"tv"}}`)
		case "/api/v1/item/tv":
			writeJSON(t, w, `{"code":0,"data":{"guid":"tv","title":"Series","type":"TV","backdrops":["wide.jpg"],"logos":[{"url":"logo.png"}],"genres":[{"name":"Drama"}],"production_countries":[{"name":"China"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	records, err := client.ContinueWatchingContext(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	if records[0].RecordGUID != "episode" || records[0].Media.ID != "episode" || records[0].Position != 100 || records[1].RecordGUID != "movie" || records[1].Position != 42 || records[1].Duration != 0 {
		t.Fatalf("normalization=%+v", records)
	}
	if !reflect.DeepEqual(infoRequests, []string{"episode", "movie"}) {
		t.Fatalf("play info requests=%v", infoRequests)
	}
	detail, err := client.ContinueWatchingDetailContext(context.Background(), records[0])
	if err != nil || detail.ID != "tv" || !detail.IsSeries || detail.Backdrop != "wide.jpg" || detail.Logo != "logo.png" || !reflect.DeepEqual(detail.Genres, []string{"Drama"}) || !reflect.DeepEqual(detail.Countries, []string{"China"}) {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

func TestContinueWatchingKeepsRecordsWhenOnePlayInfoLookupFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/play/list":
			writeJSON(t, w, `{"code":0,"data":[{"guid":"first","title":"First","type":"Movie","ts":10},{"guid":"missing","title":"Missing","type":"Movie","ts":20},{"guid":"last","title":"Last","type":"Movie","ts":30}]}`)
		case "/api/v1/play/info":
			var body struct {
				ItemGUID string `json:"item_guid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode play info request: %v", err)
			}
			if body.ItemGUID == "missing" {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writeJSON(t, w, `{"code":0,"data":{"ts":5}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	items, err := NewServer(server.URL, "token").ContinueWatchingContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected a per-record info error, got %v", err)
	}
	if len(items) != 2 || items[0].RecordGUID != "first" || items[1].RecordGUID != "last" {
		t.Fatalf("successful records/order not preserved: %+v", items)
	}
	if items[0].Position != 5 || items[1].Position != 5 {
		t.Fatalf("progress did not come from play/info: %+v", items)
	}
}

func TestHomeEndpointsEmptyAuthAndCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "bad" {
			writeJSON(t, w, `{"code":401,"msg":"unauthorized"}`)
			return
		}
		writeJSON(t, w, `{"code":0,"data":[]}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	items, err := client.ContinueWatchingContext(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("empty=%+v %v", items, err)
	}
	client.SetToken("bad")
	if _, err = client.ContinueWatchingContext(context.Background()); err == nil {
		t.Fatal("missing auth error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.ContinueWatchingContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, _, err = client.HomeCandidatesPageContext(context.Background(), "", 1, 60); err == nil {
		t.Fatal("unscoped home request allowed")
	}
}

func TestHomeRequestGateBoundsCombinedEndpointsAndCancelsWaiter(t *testing.T) {
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		writeJSON(t, w, `{"code":0,"data":[]}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); _, _ = client.ContinueWatchingContext(context.Background()) }()
	go func() {
		defer workers.Done()
		_, _ = client.ItemDetailContext(context.Background(), MediaItem{ID: "movie"})
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("request did not enter")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := client.HomeCandidatesPageContext(ctx, "library", 1, 60)
	close(release)
	workers.Wait()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting request: %v", err)
	}
	select {
	case <-entered:
		t.Fatal("third request escaped shared two-request gate")
	default:
	}
}

func TestHomeSamplingLibraryIsolationStableAndBounded(t *testing.T) {
	var mu sync.Mutex
	pages := map[string]int{}
	var active, maxActive atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/item/") {
			n := active.Add(1)
			defer active.Add(-1)
			for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
			}
			time.Sleep(time.Millisecond)
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/item/")
			writeJSON(t, w, fmt.Sprintf(`{"code":0,"data":{"guid":%q,"title":%q,"type":"Movie","backdrops":["wide.jpg"]}}`, id, id))
			return
		}
		var body struct {
			Library string `json:"ancestor_guid"`
			Page    int    `json:"page"`
			Size    int    `json:"page_size"`
			Tags    struct {
				Types []string `json:"type"`
			} `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Library == "" || !reflect.DeepEqual(body.Tags.Types, []string{"Movie", "TV"}) {
			t.Errorf("unscoped/wrong filter %+v", body)
		}
		mu.Lock()
		pages[body.Library]++
		mu.Unlock()
		if body.Library == "broken" {
			http.Error(w, "offline", 500)
			return
		}
		count := 6000
		if body.Library == "small" {
			count = 3
		}
		list := []map[string]any{}
		for i := (body.Page - 1) * body.Size; i < body.Page*body.Size && i < count; i++ {
			id := fmt.Sprintf("%s-%d", body.Library, i)
			list = append(list, map[string]any{"guid": id, "title": id, "type": "Movie", "ancestor_guid": body.Library})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": count, "list": list}})
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	libraries := []MediaLibrary{{ID: "large"}, {ID: "small"}, {ID: "broken"}}
	one, err := client.HomeHighlightsContext(context.Background(), libraries, 42)
	if err == nil || len(one) != 8 {
		t.Fatalf("expected partial success %+v %v", one, err)
	}
	two, _ := client.HomeHighlightsContext(context.Background(), libraries, 42)
	seen := map[string]bool{}
	for i, item := range one {
		if seen[item.ID] || item.ID != two[i].ID || item.Backdrop != "wide.jpg" {
			t.Fatalf("sampling %+v %+v", one, two)
		}
		seen[item.ID] = true
	}
	if maxActive.Load() > 2 {
		t.Fatalf("detail concurrency=%d", maxActive.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if pages["large"] > 34 {
		t.Fatalf("unbounded pages=%v", pages)
	}
}

func TestHomeSamplingInsufficientAndDuplicateLibraries(t *testing.T) {
	var pages atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			pages.Add(1)
			writeJSON(t, w, `{"code":0,"data":{"total":3,"list":[{"guid":"a","title":"A","type":"Movie"},{"guid":"b","title":"B","type":"TV"},{"guid":"a","title":"A","type":"Movie"}]}}`)
			return
		}
		writeJSON(t, w, `{"code":0,"data":{}}`)
	}))
	defer server.Close()
	items, err := NewServer(server.URL, "").HomeHighlightsContext(context.Background(), []MediaLibrary{{ID: "library"}, {ID: "library"}}, 1)
	if err != nil || len(items) != 2 || pages.Load() != 1 {
		t.Fatalf("items=%+v err=%v pages=%d", items, err, pages.Load())
	}
}

func TestHomeSessionSetStableAcrossRetry(t *testing.T) {
	existing := []MediaItem{{ID: "first", Title: "First"}, {ID: "second", Title: "Second"}}
	result := mergeHomeHeroesStable(existing, []MediaItem{{ID: "second", Backdrop: "recovered.jpg"}, {ID: "third", Title: "Third"}, {ID: "first"}})
	if len(result) != 3 || result[0].ID != "first" || result[1].ID != "second" || result[2].ID != "third" || result[1].Backdrop != "recovered.jpg" || existing[1].Backdrop != "" {
		t.Fatalf("retry changed session order or failed to recover metadata: %+v", result)
	}
	for _, scale := range []float64{1, 1.5, 2, 3} {
		width, height := homeHeroPixelSize(2500, 1400, scale)
		if width > 1600 || height > 900 {
			t.Fatalf("hero decode exceeds budget: %dx%d", width, height)
		}
	}
}

func TestHomeMetadataDoesNotDisplayNumericGenreIDs(t *testing.T) {
	genres := metadataNames([]any{float64(19), "7", map[string]any{"id": 18}, map[string]any{"name": "剧情"}, "西部", "3D"})
	if !reflect.DeepEqual(genres, []string{"剧情", "西部", "3D"}) {
		t.Fatalf("numeric IDs leaked into hero genres: %v", genres)
	}
	item := normalizeItem(map[string]any{"guid": "movie", "title": "Movie", "type": "Movie", "vote_average": 8.7, "release_date": "1992-01-01", "genres": []any{float64(19), float64(7)}, "production_countries": []any{map[string]any{"iso_3166_1": "US"}}})
	metadata := homeHeroMetadata(item)
	if strings.Contains(metadata, "19、7") || len(item.Genres) != 0 || !strings.Contains(metadata, "★ 8.7") || !strings.Contains(metadata, "1992") || !strings.Contains(metadata, "US") {
		t.Fatalf("invalid home hero metadata: %s", metadata)
	}
}
