package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerSessionLibraryAndPlayback(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authx") == "" {
			t.Error("request is missing the signed authx header")
		}
		if r.URL.Path == "/api/v1/login" {
			writeJSON(t, w, `{"code":0,"data":{"token":"session-token"}}`)
			return
		}
		if r.Header.Get("Authorization") != "session-token" {
			t.Errorf("Authorization must contain the raw session token")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/item/list":
			writeJSON(t, w, `{"code":0,"data":{"total":1,"list":[{"guid":"item-1","title":"Example","type":"Video","release_date":"2024-01-01","is_favorite":true,"watched":true}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/movie/item-1":
			writeJSON(t, w, `{"code":0,"data":{"guid":"item-1","title":"Example","overview":"Description"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/play/info":
			writeJSON(t, w, `{"code":0,"data":{"media_guid":"media-1","ts":37}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stream":
			writeJSON(t, w, `{"code":0,"data":{"video_stream":{"duration":125.5},"direct_link_qualities":[{"bitrate":1000,"resolution":"720p","url":"/play/720.m3u8"},{"bitrate":4000,"resolution":"1080p","url":"/play/1080.m3u8"}]}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/item/favorite":
			writeJSON(t, w, `{"code":0,"data":{}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/play/record":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode play record: %v", err)
			}
			if payload["item_guid"] != "item-1" || payload["media_guid"] != "media-1" || payload["ts"] != float64(15) {
				t.Errorf("progress should include item and media identifiers")
			}
			if _, hasDuration := payload["duration"]; hasDuration {
				t.Error("progress payload should match the web client fields")
			}
			writeJSON(t, w, `{"code":0,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewServer(server.URL, "")
	client.client = server.Client()
	if _, err := client.Login("user", "password"); err != nil {
		t.Fatalf("login: %v", err)
	}
	items, err := client.Library("")
	if err != nil || len(items) != 1 {
		t.Fatalf("library got %d items, err=%v", len(items), err)
	}
	if items[0].Kind != "movie" || !items[0].Favorite || !items[0].Watched {
		t.Fatalf("item fields were not normalized: %#v", items[0])
	}
	detail, err := client.Detail(items[0])
	if err != nil || detail.Overview != "Description" {
		t.Fatalf("detail = %#v, err=%v", detail, err)
	}
	play, err := client.Playback(items[0])
	if err != nil {
		t.Fatalf("playback: %v", err)
	}
	if play.URL != server.URL+"/play/1080.m3u8" || play.MediaID != "media-1" || play.Duration != 125.5 || play.ResumeAt != 37 {
		t.Fatalf("unexpected playback result: %#v", play)
	}
	if err := client.ToggleFavorite(items[0]); err != nil {
		t.Fatalf("favorite: %v", err)
	}
	if err := client.UpdateProgress(items[0].ID, play.MediaID, 15.75, play.Duration); err != nil {
		t.Fatalf("progress: %v", err)
	}
}

func TestServerListsAccessibleLibrariesAndScopesItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "session-token" {
			t.Errorf("missing session token on %s", r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/mdb/list":
			writeJSON(t, w, `{"code":0,"data":[{"guid":"mdb-1","name":"Movies","category":"movie","poster":"/poster.jpg"},{"guid":"mdb-2","name":"Series","category":"tv"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/item/list":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode item list body: %v", err)
			}
			if body["ancestor_guid"] != "mdb-2" {
				t.Errorf("item list should be scoped to selected library, body=%v", body)
			}
			writeJSON(t, w, `{"code":0,"data":{"total":1,"list":[{"guid":"item-2","title":"Series item","mdb_guid":"mdb-2","type":"TV"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewServer(server.URL, "session-token")
	client.client = server.Client()
	libraries, err := client.Libraries()
	if err != nil || len(libraries) != 2 || libraries[0].ID != "mdb-1" || libraries[1].Name != "Series" {
		t.Fatalf("unexpected libraries: %#v, err=%v", libraries, err)
	}
	items, err := client.LibraryItems("mdb-2", "")
	if err != nil || len(items) != 1 || items[0].ID != "item-2" {
		t.Fatalf("unexpected scoped items: %#v, err=%v", items, err)
	}
}

func TestLibraryPageContextRequestsOnlyOnePage(t *testing.T) {
	var gotPage, gotSize atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/item/list" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode page request: %v", err)
		}
		gotPage.Store(int64(body["page"].(float64)))
		gotSize.Store(int64(body["page_size"].(float64)))
		writeJSON(t, w, `{"code":0,"data":{"total":205,"list":[{"guid":"item-101","title":"Page item","mdb_guid":"mdb-1"}]}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	items, total, err := client.LibraryPageContext(t.Context(), "mdb-1", "", 2, catalogPageSize)
	if err != nil || total != 205 || len(items) != 1 || items[0].ID != "item-101" {
		t.Fatalf("page result items=%#v total=%d err=%v", items, total, err)
	}
	if gotPage.Load() != 2 || gotSize.Load() != catalogPageSize {
		t.Fatalf("request page=%d size=%d; expected a bounded second page", gotPage.Load(), gotSize.Load())
	}
}

func TestMediaPageContextUsesSystemCategoryAndFiltersSearchResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/item/list":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode category request: %v", err)
			}
			tags, _ := body["tags"].(map[string]any)
			if got, ok := tags["type"].([]any); !ok || len(got) != 1 || got[0] != "TV" {
				t.Errorf("system TV category should use tags.type=[TV], body=%#v", body)
			}
			if _, hasAncestor := body["ancestor_guid"]; hasAncestor {
				t.Errorf("system category must not be scoped to a personal library: %#v", body)
			}
			writeJSON(t, w, `{"code":0,"data":{"total":2,"list":[{"guid":"show","title":"Show","type":"TV"},{"guid":"movie","title":"Movie","type":"Movie"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/search/list":
			writeJSON(t, w, `{"code":0,"data":{"total":2,"list":[{"guid":"show","title":"Show","type":"TV"},{"guid":"movie","title":"Movie","type":"Movie"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	items, total, err := client.MediaPageContext(t.Context(), "", "tv", "", 1, catalogPageSize)
	if err != nil || total != 2 || len(items) != 1 || items[0].ID != "show" {
		t.Fatalf("system TV page = %#v total=%d err=%v", items, total, err)
	}
	items, total, err = client.MediaPageContext(t.Context(), "", "movie", "keyword", 1, catalogPageSize)
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != "movie" {
		t.Fatalf("movie search page = %#v total=%d err=%v", items, total, err)
	}
}

func TestSeasonEpisodesRequestsOnlySelectedSeasonAndSortsEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/episode/list/season-2" {
			t.Errorf("unexpected season endpoint %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, `{"code":0,"data":[{"guid":"e2","title":"Episode 2","type":"Episode","episode_number":2},{"guid":"e1","title":"Episode 1","type":"Episode","episode_number":1}]}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	items, err := client.SeasonEpisodes("season-2")
	if err != nil || len(items) != 2 {
		t.Fatalf("season episodes=%#v err=%v", items, err)
	}
	if items[0].ID != "e1" || items[1].ID != "e2" {
		t.Fatalf("episodes are not sorted within selected season: %#v", items)
	}
}

func TestSeriesSeasonsUsesSeriesGuidAndPreservesSeasonNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/season/list/show-1" {
			t.Errorf("unexpected seasons endpoint %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, `{"code":0,"data":[{"guid":"season-2","parent_guid":"show-1","title":"第 2 季","type":"Season","season_number":2},{"guid":"season-1","parent_guid":"show-1","title":"第 1 季","type":"Season","season_number":1}]}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	seasons, err := client.SeriesSeasons(MediaItem{ID: "show-1", Kind: "tv", IsSeries: true})
	if err != nil || len(seasons) != 2 {
		t.Fatalf("seasons=%#v err=%v", seasons, err)
	}
	if seasons[0].ID != "season-1" || seasons[1].ID != "season-2" {
		t.Fatalf("seasons not sorted: %#v", seasons)
	}
}

func TestPeopleLoadsCastAndProfilePaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/person/list/movie-1" {
			t.Errorf("unexpected cast request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, `{"code":0,"data":{"total":1,"list":[{"person_guid":"person-1","person_name":"演员甲","character_name":"Alice","job":"Actor","profile_path":"/person/a.jpg","order":0}]}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	people, err := client.People("movie-1")
	if err != nil || len(people) != 1 {
		t.Fatalf("people=%#v err=%v", people, err)
	}
	person := people[0]
	if person.Name != "演员甲" || person.Role != "Alice" || person.Profile != "/person/a.jpg" {
		t.Fatalf("unexpected person data: %#v", person)
	}
}

func TestSystemCollectionsUseFnOSFiltersAndFavoriteEndpoint(t *testing.T) {
	var favoriteCalled, watchedCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		switch r.URL.Path {
		case "/api/v1/favorite/list":
			favoriteCalled = true
			writeJSON(t, w, `{"code":0,"data":{"total":1,"list":[{"guid":"fav","title":"Favorite","type":"Movie"}]}}`)
		case "/api/v1/item/list":
			watchedCalled = true
			tags, _ := body["tags"].(map[string]any)
			if tags["watched"] != "1" {
				t.Errorf("history must request watched items, body=%#v", body)
			}
			writeJSON(t, w, `{"code":0,"data":{"total":1,"list":[{"guid":"seen","title":"Watched","type":"TV"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()

	favorites, total, err := client.MediaPageContext(t.Context(), "", "favorite", "", 1, 100)
	if err != nil || total != 1 || len(favorites) != 1 || !favorites[0].Favorite {
		t.Fatalf("favorite page=%#v total=%d err=%v", favorites, total, err)
	}
	history, total, err := client.MediaPageContext(t.Context(), "", "watched", "", 1, 100)
	if err != nil || total != 1 || len(history) != 1 || !history[0].Watched {
		t.Fatalf("history page=%#v total=%d err=%v", history, total, err)
	}
	if !favoriteCalled || !watchedCalled {
		t.Fatalf("favorite endpoint called=%t watched endpoint called=%t", favoriteCalled, watchedCalled)
	}
}

func TestDetailUsesResourceRouteAndMergesCardFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tv/show-1" {
			t.Errorf("unexpected detail request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, `{"code":0,"data":{"tv":{"guid":"show-1","overview":"Series overview","first_air_date":"2020-01-02","vote_average":8.26,"posters":[{"path":"/posters/show.jpg"}]}}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	item, err := client.Detail(MediaItem{ID: "show-1", Kind: "tv", Title: "Card title", Poster: "/card.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "Card title" || item.Overview != "Series overview" || item.Year != "2020" || item.Rating != "8.3" || item.Poster != "/posters/show.jpg" {
		t.Fatalf("detail fields were not normalized/merged: %#v", item)
	}
}

func TestNormalizeFnOSTypesKeepsSeasonAndEpisodeDistinct(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"TV", "tv"}, {"Season", "season"}, {"Episode", "episode"}, {"Movie", "movie"}, {"Video", "movie"},
	} {
		item := normalizeItem(map[string]any{"guid": "id", "title": "title", "type": test.input})
		if item.Kind != test.want {
			t.Errorf("type %q normalized as %q, want %q", test.input, item.Kind, test.want)
		}
		if test.want == "tv" && !item.IsSeries {
			t.Errorf("TV root should be marked as a series: %#v", item)
		}
	}

	// fnOS returns type: Video with season_number: 0 on generic libraries
	videoItem := normalizeItem(map[string]any{
		"guid": "vid-1", "title": "Video File", "type": "Video", "season_number": 0, "episode_number": 0,
	})
	if videoItem.Kind != "movie" {
		t.Errorf("video with season_number 0 normalized as %q, want movie", videoItem.Kind)
	}
}

func TestLibraryPageContextDoesNotMixLibraries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/item/list" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode scoped request: %v", err)
		}
		if body["ancestor_guid"] != "mdb-selected" {
			t.Errorf("selected library id missing from request: %v", body)
		}
		writeJSON(t, w, `{"code":0,"data":{"total":2,"list":[{"guid":"keep","title":"Selected","mdb_guid":"mdb-selected"},{"guid":"drop","title":"Other","mdbGuid":"mdb-other"}]}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	items, _, err := client.LibraryPageContext(t.Context(), "mdb-selected", "", 1, catalogPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "keep" {
		t.Fatalf("library page mixed items from another library: %#v", items)
	}
}

func TestLibraryPageContextFiltersNASAncestorLibrary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/item/list" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode selected library request: %v", err)
		}
		if body["ancestor_guid"] != "mdb-selected" {
			t.Errorf("selected library must use the fnOS ancestor_guid filter, body=%v", body)
		}
		writeJSON(t, w, `{"code":0,"data":{"total":366,"list":[{"guid":"keep","title":"Selected library item","ancestor_guid":"mdb-selected","ancestor_name":"Selected"},{"guid":"drop","title":"Foreign library item","ancestor_guid":"mdb-other","ancestor_name":"Other"}]}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	items, total, err := client.LibraryPageContext(t.Context(), "mdb-selected", "", 1, catalogPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if total != 366 || len(items) != 1 || items[0].ID != "keep" {
		t.Fatalf("NAS ancestor library filter returned total=%d items=%#v", total, items)
	}
}

func TestLibraryViewUsesSelectedCatalogAndRejectsForeignItems(t *testing.T) {
	app := &appState{
		section:   "library",
		libraryID: "mdb-2",
		items:     []MediaItem{{ID: "stale", Title: "Wrong library", Raw: map[string]any{"mdb_guid": "mdb-1"}}},
		catalogs: map[string]*CatalogState{
			"mdb-1\x00": {Items: []MediaItem{{ID: "one", Title: "One", Raw: map[string]any{"mdb_guid": "mdb-1"}}}},
			"mdb-2\x00": {Items: []MediaItem{
				{ID: "two", Title: "Two", Raw: map[string]any{"mdb_guid": "mdb-2"}},
				{ID: "one-again", Title: "One again", Raw: map[string]any{"library": map[string]any{"guid": "mdb-1"}}},
			}},
		},
	}
	items := app.visibleItems()
	if len(items) != 1 || items[0].ID != "two" {
		t.Fatalf("selected library showed stale or foreign media: %#v", items)
	}
}

func TestLibraryViewNeverFallsBackToAllMediaCatalog(t *testing.T) {
	allMedia := []MediaItem{{ID: "all-media-item", Title: "All media"}}
	app := &appState{
		section:   "library",
		libraryID: "",
		items:     allMedia,
		catalogs:  map[string]*CatalogState{"\x00": {Items: allMedia}},
	}
	if items := app.visibleItems(); len(items) != 0 {
		t.Fatalf("library view leaked all-media catalog when selected library ID was missing: %#v", items)
	}
}

func TestCatalogKeepsPagingWhenServerCapsPageSize(t *testing.T) {
	if catalogPageExhausted(20, 20, 3816, 60) {
		t.Fatal("a server-capped page must still allow loading more when the server reports remaining items")
	}
	if !catalogPageExhausted(20, 3816, 3816, 60) {
		t.Fatal("catalog should stop after reaching the reported total")
	}
	if !catalogPageExhausted(0, 40, 0, 60) {
		t.Fatal("an empty page should mark an unknown-total catalog as exhausted")
	}
}

func TestLibraryPageContextHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := NewServer(server.URL, "token")
	client.client = server.Client()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	_, _, err := client.LibraryPageContext(ctx, "", "", 1, catalogPageSize)
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("canceled catalog request should return promptly, err=%v elapsed=%s", err, time.Since(start))
	}
}

func TestServerReportsAuthenticationAndMissingPlayback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/login") {
			writeJSON(t, w, `{"code":0,"data":{"token":"session-token"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/play/info") {
			writeJSON(t, w, `{"code":401,"msg":"invalid sign"}`)
			return
		}
		writeJSON(t, w, `{"code":0,"data":{"media_guid":"media-1"}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "")
	client.client = server.Client()
	if _, err := client.Login("user", "password"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Playback(MediaItem{ID: "item-1"}); err == nil || !strings.Contains(err.Error(), "invalid sign") {
		t.Fatalf("expected visible auth error, got %v", err)
	}
}

func TestEmptyLibraryAndRangePlaybackFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/item/list":
			writeJSON(t, w, `{"code":0,"data":{"total":0,"list":[]}}`)
		case "/api/v1/play/info":
			writeJSON(t, w, `{"code":0,"data":{"media_guid":"media-1"}}`)
		case "/api/v1/stream":
			writeJSON(t, w, `{"code":0,"data":{"video_stream":{"duration":60},"qualities":[{"resolution":"1080p"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewServer(server.URL, "session-token")
	client.client = server.Client()
	items, err := client.Library("")
	if err != nil || len(items) != 0 {
		t.Fatalf("empty library returned %d items, err=%v", len(items), err)
	}
	play, err := client.Playback(MediaItem{ID: "item-1"})
	if err != nil {
		t.Fatalf("range fallback: %v", err)
	}
	if play.URL != server.URL+"/api/v1/media/range/media-1" || play.Duration != 60 {
		t.Fatalf("unexpected range fallback: %#v", play)
	}
}

func TestServerTimeoutIsReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		writeJSON(t, w, `{"code":0,"data":{"list":[]}}`)
	}))
	defer server.Close()
	client := NewServer(server.URL, "session-token")
	client.client = &http.Client{Timeout: 5 * time.Millisecond}
	if _, err := client.Library(""); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestLoginResolvesServerPortRedirectAndKeepsPrefix(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v/api/v1/login" {
			t.Errorf("redirect lost the deployment prefix: %s", r.URL.Path)
		}
		writeJSON(t, w, `{"code":0,"data":{"token":"session-token"}}`)
	}))
	defer target.Close()
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer front.Close()
	client := NewServer(front.URL+"/v", "")
	client.client = front.Client()
	if _, err := client.Login("user", "password"); err != nil {
		t.Fatal(err)
	}
	if client.baseURL != target.URL+"/v" {
		t.Fatalf("resolved base URL does not retain redirected host and prefix: %s", client.baseURL)
	}
}

func TestPlaybackProxyKeepsTokenOutOfURLAndForwardsRanges(t *testing.T) {
	var sawRange atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "session-token" {
			t.Errorf("proxy did not attach the session token upstream")
		}
		if r.Header.Get("authx") == "" {
			t.Errorf("proxy did not sign the authenticated NAS range request")
		}
		sawRange.Store(r.Header.Get("Range") == "bytes=4-7")
		w.Header().Set("Content-Range", "bytes 4-7/8")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("data"))
	}))
	defer upstream.Close()

	client := NewServer(upstream.URL, "session-token")
	localURL, proxy, err := client.ProxyPlayback(upstream.URL + "/media")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	if strings.Contains(localURL, "session-token") || !strings.HasPrefix(localURL, "http://127.0.0.1:") {
		t.Fatalf("proxy URL should be local and must not contain credentials")
	}
	request, err := http.NewRequest(http.MethodGet, localURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=4-7")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusPartialContent || string(data) != "data" || !sawRange.Load() {
		t.Fatalf("range response was not preserved: status=%d data=%q range=%t", response.StatusCode, data, sawRange.Load())
	}
}

func TestExternalPosterDoesNotReceiveServerCredentials(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("authx") != "" {
			t.Errorf("server credentials must not be sent to an external image host")
		}
		_, _ = w.Write([]byte("image"))
	}))
	defer imageServer.Close()

	client := NewServer("http://nas.example/v", "session-token")
	client.client = imageServer.Client()
	if _, err := client.FetchImage(imageServer.URL + "/poster"); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeItemFormatsRatingToOneDecimal(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "7.938754787572456", want: "7.9"},
		{input: "5.285037066679592", want: "5.3"},
		{input: "8", want: "8.0"},
		{input: "NR", want: "NR"},
		{input: "", want: ""},
	}
	for _, test := range cases {
		item := normalizeItem(map[string]any{"guid": "item", "title": "Film", "rating": test.input})
		if item.Rating != test.want {
			t.Errorf("normalizeItem rating %q = %q, want %q", test.input, item.Rating, test.want)
		}
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}
