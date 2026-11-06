package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestDanmakuSettingsNormalize(t *testing.T) {
	settings := DanmakuSettings{}.normalized()
	if settings.FontScale != 1 || settings.Opacity != 0.9 || settings.SpeedSeconds != 10 ||
		settings.AreaPercent != 50 || settings.Anchor != "top" || settings.MaxComments != 20000 {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
	settings = DanmakuSettings{FontScale: 9, Opacity: -1, SpeedSeconds: 1, AreaPercent: 5, Anchor: "center", MaxComments: 1}.normalized()
	if settings.FontScale != 2.5 || settings.Opacity != 0.9 || settings.SpeedSeconds != 4 ||
		settings.AreaPercent != 10 || settings.Anchor != "top" || settings.MaxComments != 100 {
		t.Fatalf("clamps failed: %+v", settings)
	}
}

func TestDefaultDanmakuServerURL(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"http://nas.example:5666/v":  "http://nas.example:9321",
		"https://nas.example:5666":   "http://nas.example:9321",
		"nas.example:5666":           "http://nas.example:9321",
		"http://192.168.1.20:5666/v": "http://192.168.1.20:9321",
		"http://[fe80::1]:5666":      "http://[fe80::1]:9321",
	}
	for input, want := range cases {
		if got := defaultDanmakuServerURL(input); got != want {
			t.Errorf("defaultDanmakuServerURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDanmakuMatchNameAndEpisode(t *testing.T) {
	fileItem := MediaItem{Title: "第一集", SeriesTitle: "某某番剧", Raw: map[string]any{"file_name": "/media/Show.Name.S02E05.1080p.mkv"}}
	if got := danmakuMatchName(fileItem); got != "Show.Name.S02E05.1080p.mkv" {
		t.Fatalf("match name = %q", got)
	}
	if got := danmakuEpisodeNumber(fileItem); got != 5 {
		t.Fatalf("episode = %d, want 5", got)
	}
	seriesItem := MediaItem{Title: "第三集", SeriesTitle: "某某番剧", EpisodeNumber: 3}
	if got := danmakuMatchName(seriesItem); got != "某某番剧" {
		t.Fatalf("series match name = %q", got)
	}
	if got := danmakuEpisodeNumber(seriesItem); got != 3 {
		t.Fatalf("series episode = %d, want 3", got)
	}
	if got := parseDanmakuEpisodeNumber("第12集"); got != 12 {
		t.Fatalf("Chinese episode = %d", got)
	}
	if got := parseDanmakuEpisodeNumber("S01E09"); got != 9 {
		t.Fatalf("season episode = %d", got)
	}
}

func TestParseDanmakuCommentFormats(t *testing.T) {
	comment, ok := parseDanmakuComment(map[string]any{
		"p": "12.5,5,16711680,0,0,0,0", "m": "顶部红色",
	})
	if !ok || comment.Time != 12.5 || comment.Mode != 5 || comment.Color != 0xFF0000 || comment.Text != "顶部红色" {
		t.Fatalf("p-format comment = %+v ok=%v", comment, ok)
	}
	comment, ok = parseDanmakuComment(map[string]any{"time": 3.25, "mode": 4, "c": 65280, "m": "底部"})
	if !ok || comment.Time != 3.25 || comment.Mode != 4 || comment.Color != 0x00FF00 {
		t.Fatalf("flat comment = %+v ok=%v", comment, ok)
	}
	if _, ok := parseDanmakuComment(map[string]any{"p": "1,7,0", "m": "advanced"}); ok {
		t.Fatal("advanced mode should be skipped")
	}
}

func TestRenderDanmakuASS(t *testing.T) {
	comments := []DanmakuComment{
		{Time: 1, Mode: 1, Color: 0xFFFFFF, Text: `普通 } 弹幕 { 文本 \ 结束`},
		{Time: 2, Mode: 5, Color: 0xFF0000, Text: "顶部"},
		{Time: 3, Mode: 4, Color: 0xFFFFFF, Text: "底部"},
		{Time: 4, Mode: 7, Color: 0xFFFFFF, Text: "高级"},
	}
	ass := renderDanmakuASS(comments, DanmakuRenderOptions{
		FontScale: 1, Opacity: 0.8, SpeedSeconds: 10, AreaPercent: 50, Anchor: "top",
		ShowScroll: true, ShowTop: true, ShowBottom: true, Outline: true,
	})
	if !strings.Contains(ass, "[Events]") || !strings.Contains(ass, "Style: Danmaku,Microsoft YaHei") {
		t.Fatal("ASS header is missing")
	}
	if count := strings.Count(ass, "Dialogue:"); count != 3 {
		t.Fatalf("dialogue count = %d, want 3", count)
	}
	if !strings.Contains(ass, `{\an8\pos(960,0)`) || !strings.Contains(ass, `{\an2\pos(960,1067)`) {
		t.Fatal("fixed comments are not anchored at the expected lanes")
	}
	if !strings.Contains(ass, `{\c&H0000FF&}`) {
		t.Fatal("red comment color was not converted to BGR")
	}
	if !strings.Contains(ass, `\move(`) || !strings.Contains(ass, `\{`) || !strings.Contains(ass, `\\`) {
		t.Fatal("scrolling text or ASS escaping is missing")
	}
}

func TestRenderDanmakuASSRespectsFiltersAndOffset(t *testing.T) {
	comments := []DanmakuComment{
		{Time: 10, Mode: 1, Color: 0xFFFFFF, Text: "滚动"},
		{Time: 10, Mode: 5, Color: 0xFFFFFF, Text: "顶部"},
		{Time: 10, Mode: 4, Color: 0xFFFFFF, Text: "底部"},
		{Time: 1, Mode: 1, Color: 0xFFFFFF, Text: "负偏移"},
	}
	ass := renderDanmakuASS(comments, DanmakuRenderOptions{
		FontScale: 1, Opacity: 1, SpeedSeconds: 8, AreaPercent: 40, Anchor: "bottom", OffsetSecond: -3,
		ShowScroll: false, ShowTop: true, ShowBottom: false, Outline: false,
	})
	if strings.Count(ass, "Dialogue:") != 1 || !strings.Contains(ass, "顶部") {
		t.Fatalf("hidden types were rendered: %s", ass)
	}
	if !strings.Contains(ass, "0:00:07.00") {
		t.Fatalf("offset was not applied: %s", ass)
	}
	if strings.Contains(ass, "负偏移") {
		t.Fatal("comments shifted before zero should be dropped")
	}
}

func TestDanmakuClientProtocol(t *testing.T) {
	var matchedName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v2/match" && r.Method == http.MethodPost:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			matchedName = body["fileName"]
			_, _ = w.Write([]byte(`{"matches":[{"episodeId":901,"animeId":42,"animeTitle":"测试番剧","episodeTitle":"第1集"}]}`))
		case r.URL.Path == "/api/v2/search/anime":
			if r.URL.Query().Get("keyword") != "测试" {
				t.Errorf("search keyword = %q", r.URL.Query().Get("keyword"))
			}
			_, _ = w.Write([]byte(`{"animes":[{"animeId":42,"animeTitle":"测试番剧","episodeCount":12}]}`))
		case r.URL.Path == "/api/v2/bangumi/42":
			_, _ = w.Write([]byte(`{"bangumi":{"episodes":[{"episodeId":902,"episodeNumber":2,"episodeTitle":"第二集"}]}}`))
		case r.URL.Path == "/api/v2/comment/902":
			if r.URL.Query().Get("format") != "json" {
				t.Errorf("comment format = %q", r.URL.Query().Get("format"))
			}
			_, _ = w.Write([]byte(`{"comments":[{"p":"1.5,5,16711680,0,0,0,0","m":"顶部"},{"p":"2,1,16777215","m":"滚动"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewDanmakuClient(server.URL + "/api/v2")
	ctx := context.Background()
	matches, err := client.Match(ctx, "Show.S01E01.mkv")
	if err != nil || len(matches) != 1 || matches[0].EpisodeID != "901" || matchedName != "Show.S01E01.mkv" {
		t.Fatalf("match = %+v err=%v name=%q", matches, err, matchedName)
	}
	animes, err := client.Search(ctx, "测试")
	if err != nil || len(animes) != 1 || animes[0].ID != "42" || animes[0].EpisodeCount != 12 {
		t.Fatalf("search = %+v err=%v", animes, err)
	}
	episodes, err := client.Episodes(ctx, "42")
	if err != nil || len(episodes) != 1 || episodes[0].ID != "902" || episodes[0].Number != 2 {
		t.Fatalf("episodes = %+v err=%v", episodes, err)
	}
	comments, err := client.Comments(ctx, "902", 100)
	if err != nil || len(comments) != 2 || comments[0].Mode != 5 || comments[0].Color != 0xFF0000 {
		t.Fatalf("comments = %+v err=%v", comments, err)
	}
}

func TestResolveDanmakuSelectsTargetEpisode(t *testing.T) {
	var commentPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/match":
			_, _ = w.Write([]byte(`{"matches":[{"episodeId":901,"animeId":42,"animeTitle":"测试番剧","episodeTitle":"第1集"}]}`))
		case "/api/v2/bangumi/42":
			_, _ = w.Write([]byte(`{"episodes":[{"episodeId":901,"episodeNumber":1},{"episodeId":902,"episodeNumber":2,"episodeTitle":"第二集"}]}`))
		case "/api/v2/comment/902":
			commentPath = r.URL.Path
			_, _ = w.Write([]byte(`{"comments":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	item := MediaItem{Title: "第二集", SeriesTitle: "测试番剧", EpisodeNumber: 2}
	comments, label, err := resolveDanmaku(context.Background(), NewDanmakuClient(server.URL), item, 100)
	if err != nil || comments == nil || commentPath != "/api/v2/comment/902" {
		t.Fatalf("resolve comments=%v label=%q err=%v path=%q", comments, label, err, commentPath)
	}
	if !strings.Contains(label, "第2集") {
		t.Fatalf("label = %q", label)
	}
}

func TestDanmakuSettingsPersist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := Settings{Danmaku: DanmakuSettings{
		ServerURL: "http://nas:9321", FontScale: 1.4, Opacity: 0.7, SpeedSeconds: 12,
		AreaPercent: 35, Anchor: "middle", OffsetSecond: -2, HideTop: true, NoOutline: true, MaxComments: 5000,
	}}
	if err := SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings()
	if err != nil || got != want {
		t.Fatalf("danmaku settings did not round trip: got=%+v want=%+v err=%v", got, want, err)
	}
}

func TestCancelDanmakuClearsSessionFiles(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/danmaku.ass"
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &appState{}
	a.danmaku.assDir = dir
	a.danmaku.assPath = path
	a.danmaku.request = 7
	a.cancelDanmaku()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("danmaku temp directory was not removed: %v", err)
	}
	if a.danmaku.request <= 7 || a.danmaku.assPath != "" {
		t.Fatalf("cancel did not invalidate the session: %+v", a.danmaku)
	}
}

func TestPlayerDanmakuSearchTabLayout(t *testing.T) {
	a := &appState{settings: Settings{Danmaku: DanmakuSettings{}.normalized()}, playerOverlayMenu: "danmaku"}
	a.prepareDanmakuPanel()
	a.danmaku.panelTab = 1
	a.danmaku.searchQuery = "测试番剧"
	tester := ui.NewTester(a.playerMenuView, playerDanmakuMenuWidth, playerDanmakuMenuHeight)
	tester.Frame()
	for _, text := range []string{"搜索选集", "自动匹配当前影片", "搜索"} {
		if !tester.HasText(text) {
			t.Fatalf("search tab is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestFilterDanmakuSubtitleTracks(t *testing.T) {
	tracks := []PlayerTrack{
		{ID: 1, Title: "中文", External: true},
		{ID: 2, Title: "danmaku", External: true},
		{ID: 3, Title: "fnmovie-danmaku.ass", External: true},
		{ID: 4, Title: "danmaku", External: false},
	}
	filtered := filterDanmakuSubtitleTracks(tracks)
	if len(filtered) != 2 || filtered[0].ID != 1 || filtered[1].ID != 4 {
		t.Fatalf("filtered tracks = %+v", filtered)
	}
}

func TestDanmakuMenuBoundsFitSmallWindows(t *testing.T) {
	a := &appState{playerOverlayMenu: "danmaku"}
	large := a.playerMenuBounds(mygo.Rectangle{Width: 1280, Height: 800})
	if large.Width != playerDanmakuMenuWidth || large.Height != playerDanmakuMenuHeight {
		t.Fatalf("large danmaku menu bounds = %+v", large)
	}
	small := a.playerMenuBounds(mygo.Rectangle{Width: 400, Height: 300})
	if small.Width > 360 || small.Height > 138 || small.Width <= 0 || small.Height <= 0 {
		t.Fatalf("small danmaku menu bounds = %+v", small)
	}
}

func TestPlayerDanmakuPanelLayout(t *testing.T) {
	a := &appState{settings: Settings{Danmaku: DanmakuSettings{}.normalized()}, playerOverlayMenu: "danmaku"}
	a.prepareDanmakuPanel()
	tester := ui.NewTester(a.playerMenuView, playerDanmakuMenuWidth, playerDanmakuMenuHeight)
	tester.Frame()
	panel, ok := tester.Find("播放选项菜单")
	if !ok || panel.W != playerDanmakuMenuWidth || panel.H != playerDanmakuMenuHeight {
		t.Fatalf("danmaku panel bounds = %+v ok=%v", panel, ok)
	}
	for _, text := range []string{"播放设置", "搜索选集", "字体大小", "弹幕服务器", "关闭弹幕"} {
		if !tester.HasText(text) {
			t.Fatalf("danmaku panel is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestDanmakuShortcutMapping(t *testing.T) {
	if got := playerShortcutAction(0, ui.KeyD, false); got != playerToggleDanmaku {
		t.Fatalf("D shortcut = %q, want %q", got, playerToggleDanmaku)
	}
}
