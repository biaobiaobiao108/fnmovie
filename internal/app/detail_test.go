package app

import (
	"container/list"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSecondaryPageUsesCachedArtworkTint(t *testing.T) {
	server := NewServer("https://synthetic.invalid", "")
	artwork := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := range 720 {
		for x := range 1280 {
			artwork.SetRGBA(x, y, color.RGBA{R: 24, G: 128, B: 210, A: 255})
		}
	}
	tint := ui.RGB(24, 128, 210)
	posters := &PosterLoader{entries: map[posterKey]*list.Element{}, lru: list.New(), maxBytes: posterCacheBudget}
	url := server.imageURL("/backdrop.jpg")
	posters.putTintLocked(posterKey{URL: url, Width: 1280, Height: 720}, ui.NewBitmap(artwork), 1280*720*4, tint)
	item := MediaItem{ID: "movie", Title: "沉浸式详情", Kind: "movie", Backdrop: "/backdrop.jpg"}
	a := &appState{server: server, posters: posters, displayScale: 1, selected: &item, catalogs: map[string]*CatalogState{}}

	bitmap, gotTint := a.imageForURLWithTint("/backdrop.jpg", 1280, 720)
	if bitmap == nil || gotTint.R != tint.R || gotTint.G != tint.G || gotTint.B != tint.B {
		t.Fatalf("detail backdrop = %v, tint=%+v; want cached artwork tint %+v", bitmap != nil, gotTint, tint)
	}
	theme := immersiveDetailTheme(gotTint)
	wantAccent := tint.Mix(ui.RGB(255, 255, 255), 0.28)
	if theme.Accent.R != wantAccent.R || theme.Accent.G != wantAccent.G || theme.Accent.B != wantAccent.B {
		t.Fatalf("theme accent = %+v, want artwork accent %+v", theme.Accent, wantAccent)
	}
	tester := ui.NewTester(a.view, 1280, 800)
	if !tester.HasText("沉浸式详情") || !tester.HasText("电影详情") || tester.HasText("首页") {
		t.Fatal("detail page should fill the window and retain its navigation")
	}
	saveDetailPreview(t, tester, "immersive-artwork")
}

func TestSidebarAlignment(t *testing.T) {
	app := &appState{
		section: "movies",
		libraries: []MediaLibrary{
			{ID: "lib1", Name: "电影库"},
			{ID: "lib2", Name: "动漫"},
			{ID: "lib3", Name: "美剧精选"},
		},
		catalogs: map[string]*CatalogState{},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	items := []string{"电影", "电视节目", "我的收藏", "电影库", "动漫", "美剧精选"}
	var targetX float32 = -1
	for _, name := range items {
		rect, ok := tester.Find(name)
		if !ok {
			t.Fatalf("Sidebar item %q not found", name)
		}
		t.Logf("Sidebar item %q: X=%.1f", name, rect.X)
		if targetX < 0 {
			targetX = rect.X
		} else if rect.X != targetX {
			t.Errorf("Sidebar item %q X=%.1f mismatch targetX=%.1f (not aligned!)", name, rect.X, targetX)
		}
	}
}

func TestMovieDetailViewLayout(t *testing.T) {
	movie := MediaItem{
		ID:       "movie-1",
		Title:    "流浪地球 2",
		Kind:     "movie",
		IsSeries: false,
		Year:     "2023",
		Overview: "人类面临太阳氦闪危机，踏上长达数千年的流浪之旅。",
		Sources: []StreamSource{
			{Name: "4K 原画", Quality: "2160p"},
		},
		Cast: []CastMember{
			{Name: "吴京", Role: "刘培强"},
			{Name: "刘德华", Role: "图恒宇"},
		},
	}
	app := &appState{
		section:   "library",
		libraryID: "movies",
		selected:  &movie,
		catalogs:  map[string]*CatalogState{},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	rectTitle, okTitle := tester.Find("流浪地球 2")
	if !okTitle || rectTitle.W <= 0 || rectTitle.H <= 0 {
		t.Fatalf("Movie title not visible: ok=%v, rect=%+v", okTitle, rectTitle)
	}

	rectOverview, okOverview := tester.Find("人类面临太阳氦闪危机，踏上长达数千年的流浪之旅。")
	if !okOverview || rectOverview.W <= 0 || rectOverview.H <= 0 {
		t.Fatalf("Movie overview not visible: ok=%v, rect=%+v", okOverview, rectOverview)
	}

	rectPlay, okPlay := tester.Find("▶  立即播放")
	if !okPlay || rectPlay.W <= 0 || rectPlay.H <= 0 {
		t.Fatalf("Movie play button not visible: ok=%v, rect=%+v", okPlay, rectPlay)
	}

	rectActor, okActor := tester.Find("吴京")
	if !okActor || rectActor.W <= 0 || rectActor.H <= 0 {
		t.Fatalf("Movie cast not visible: ok=%v, rect=%+v", okActor, rectActor)
	}

	saveDetailPreview(t, tester, "movie")
	rectSource, okSource := tester.Find("4K 原画")
	if !okSource {
		t.Fatalf("Movie source version not visible: ok=%v, rect=%+v", okSource, rectSource)
	}
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Scroll(1000, 700, 0, 400)
	if rectSource, okSource = tester.Find("4K 原画"); !okSource || rectSource.W <= 0 || rectSource.H <= 0 {
		t.Fatalf("Movie source version not reachable: ok=%v, rect=%+v", okSource, rectSource)
	}
}

func TestMovieDetailKeepsSlotsStableForLongSynopsis(t *testing.T) {
	var titleY float32
	for _, long := range []bool{false, true} {
		item := MediaItem{ID: "layout", Kind: "movie", Title: "居中详情", Overview: "简洁的影片介绍。"}
		if long {
			item.Overview = strings.Repeat("长简介保持完整显示，超出窗口时可以滚动浏览。\n", 70)
		}
		a := &appState{section: "movies", selected: &item, catalogs: map[string]*CatalogState{}}
		tester := ui.NewTester(a.view, 1280, 1000)
		title, ok := tester.Find(item.Title)
		if !ok {
			t.Fatal("detail title missing")
		}
		poster, _ := tester.Find("影片海报")
		if title.X <= poster.X+poster.W || title.H != 88 {
			t.Fatalf("title should sit in the information column beside the poster: title=%+v poster=%+v", title, poster)
		}
		if synopsis, ok := tester.Find("影片简介"); !ok || synopsis.H < 100 {
			t.Fatalf("synopsis must reserve complete lines: %+v, found=%v", synopsis, ok)
		}
		if long {
			preview, ok := tester.Find(item.Overview)
			if title.Y != titleY || !ok || preview.H < 75 || math.Mod(float64(preview.H), 25) != 0 {
				t.Fatalf("preview must show whole lines without moving slots: title=%+v preview=%+v", title, preview)
			}
			tester.Click("展开全部简介")
			if !a.detailSynopsisOpen || a.detailSynopsisScroll.MaxY <= 0 {
				t.Fatalf("full synopsis must remain readable: open=%v scroll=%+v", a.detailSynopsisOpen, a.detailSynopsisScroll)
			}
			tester.Click("收起简介")
			if a.detailSynopsisOpen {
				t.Fatal("reading view should close explicitly")
			}
		} else {
			titleY = title.Y
			if a.detailSynopsisScroll.MaxY > 1 {
				t.Fatalf("short synopsis must not overflow: %+v", a.detailSynopsisScroll)
			}
		}
	}
}

func TestMovieAndSeriesShareDetailAnchors(t *testing.T) {
	for _, size := range [][2]int{{960, 640}, {1280, 800}, {1920, 1080}} {
		var moviePlay ui.Rect
		for _, series := range []bool{false, true} {
			item := MediaItem{ID: "anchors", Title: "统一详情布局", Kind: "movie", Overview: "短简介"}
			item.Year = "2024"
			if series {
				item.Kind, item.IsSeries = "tv", true
				item.Overview = strings.Repeat("更长的简介也不改变操作区域位置。", 20)
				item.Cast = []CastMember{{Name: "演员", Role: "主角"}}
				item.Seasons = []MediaSeason{{ID: "season", Title: "第 1 季"}}
			}
			a := &appState{selected: &item, section: "movies", catalogs: map[string]*CatalogState{},
				seriesEpisodes: []MediaItem{{ID: "episode", Title: "第一集"}}}
			tester := ui.NewTester(a.view, size[0], size[1])
			playLabel := "▶  立即播放"
			if series {
				playLabel = "▶  播放本季首集"
			}
			play, playOK := tester.Find(playLabel)
			_, castOK := tester.Find("演职人员")
			if !playOK || !castOK {
				t.Fatal("missing anchored controls")
			}
			poster, posterOK := tester.Find("影片海报")
			title, titleOK := tester.Find(item.Title)
			credits, creditsOK := tester.Find("详情演职人员")
			minimumPosterHeight := map[int]float32{640: 430, 800: 580, 1080: 680}[size[1]]
			if !posterOK || poster.H < minimumPosterHeight || poster.Y < movieTitleBarHeight || play.Y+play.H > float32(size[1]) {
				t.Fatalf("poster and actions should fill the viewport without clipping: size=%v poster=%+v play=%+v", size, poster, play)
			}
			if !posterOK || !titleOK || play.Y < poster.Y+poster.H {
				t.Fatalf("actions must be below the poster: poster=%+v play=%+v", poster, play)
			}
			if !titleOK || !creditsOK || title.X <= poster.X+poster.W || credits.X <= poster.X+poster.W {
				t.Fatalf("information should occupy a spacious column beside the poster: title=%+v credits=%+v poster=%+v", title, credits, poster)
			}
			if !series {
				moviePlay = play
			} else if play.X+play.W/2 != moviePlay.X+moviePlay.W/2 || play.Y != moviePlay.Y {
				t.Fatalf("%v poster actions differ: movie=%+v series=%+v", size, moviePlay, play)
			}
		}
	}
}

func TestSeriesDetailViewLayout(t *testing.T) {
	series := MediaItem{
		ID:       "tv-cyberpunk",
		Title:    "赛博朋克 边缘跑手",
		Kind:     "tv",
		IsSeries: true,
		Year:     "2022",
		Overview: "夜之城，一个充满暴力与科技的未来都市。",
		Seasons: []MediaSeason{
			{ID: "season-1", Title: "第 1 季", Number: 1},
		},
		Cast: []CastMember{
			{Name: "大卫·马丁内斯", Role: "主角"},
		},
	}
	app := &appState{
		section:          "library",
		libraryID:        "anime",
		selected:         &series,
		selectedSeasonID: "season-1",
		seriesEpisodes: []MediaItem{
			{ID: "ep-1", Title: "这是一个美好的日子", EpisodeNumber: 1, Overview: "夜之城的故事由此展开。"},
			{ID: "ep-2", Title: "像个男人一样去战斗", EpisodeNumber: 2},
		},
		catalogs: map[string]*CatalogState{},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	rectTitle, okTitle := tester.Find("赛博朋克 边缘跑手")
	if !okTitle || rectTitle.W <= 0 || rectTitle.H <= 0 {
		t.Fatalf("Series title not visible: ok=%v, rect=%+v", okTitle, rectTitle)
	}

	rectPlay, okPlay := tester.Find("▶  播放本季首集")
	if !okPlay || rectPlay.W <= 0 || rectPlay.H <= 0 {
		t.Fatalf("Series play button not visible: ok=%v, rect=%+v", okPlay, rectPlay)
	}

	saveDetailPreview(t, tester, "series")
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Scroll(900, 700, 0, 500)
	rectSeason, okSeason := tester.Find("第 1 季")
	if !okSeason || rectSeason.W <= 0 || rectSeason.H <= 0 {
		t.Fatalf("Series season button not visible: ok=%v, rect=%+v", okSeason, rectSeason)
	}

	rectCast, okCast := tester.Find("大卫·马丁内斯")
	if !okCast || rectCast.W <= 0 || rectCast.H <= 0 {
		t.Fatalf("Cast not visible: ok=%v, rect=%+v", okCast, rectCast)
	}
	tester.Scroll(900, 700, 0, 400)
	rectEp1, okEp1 := tester.Find("这是一个美好的日子")
	if !okEp1 || rectEp1.W <= 0 || rectEp1.H <= 0 {
		t.Fatalf("Episode 1 not reachable: ok=%v, rect=%+v", okEp1, rectEp1)
	}
	if _, ok := tester.Find("夜之城的故事由此展开。"); !ok {
		t.Fatal("episode overview should be visible")
	}
	if _, ok := tester.Find("播放 这是一个美好的日子"); !ok {
		t.Fatal("episode row must have an accessible play action")
	}
	for _, text := range tester.Texts() {
		if text == "播放" || text == "FN MOVIES" {
			t.Fatal("plain episode play cards and English brand subtitle should be removed")
		}
	}
}

func TestSeriesDetailViewScroll(t *testing.T) {
	var episodes []MediaItem
	for i := 1; i <= 30; i++ {
		episodes = append(episodes, MediaItem{
			ID:            fmt.Sprintf("ep-%d", i),
			Title:         fmt.Sprintf("第%d集", i),
			EpisodeNumber: i,
		})
	}

	series := MediaItem{
		ID:       "tv-long",
		Title:    "超长动画剧集",
		Kind:     "tv",
		IsSeries: true,
		Seasons: []MediaSeason{
			{ID: "s1", Title: "第1季", Number: 1},
		},
	}
	app := &appState{
		section:          "library",
		libraryID:        "anime",
		selected:         &series,
		selectedSeasonID: "s1",
		seriesEpisodes:   episodes,
		catalogs:         map[string]*CatalogState{},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	// The shared information header remains in the same virtualized scroller.
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Scroll(800, 600, 0, 180)
	r1, ok1 := tester.Find("第1集")
	if !ok1 || r1.W <= 0 || r1.H <= 0 {
		t.Fatalf("Episode 1 should be visible initially, got ok=%v, rect=%+v", ok1, r1)
	}
	if !tester.HasText("服务器暂未提供演职人员信息") {
		t.Fatal("missing credits should have a visible empty state")
	}

	// Scroll down within the scroll container
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Scroll(800, 700, 0, 1200)

	// After scrolling, later episodes should become visible
	r25, ok25 := tester.Find("第25集")
	if !ok25 || r25.W <= 0 || r25.H <= 0 {
		t.Fatalf("Episode 25 should be visible after scrolling, got ok=%v, rect=%+v", ok25, r25)
	}
}

func TestDetailBackButton(t *testing.T) {
	movie := MediaItem{
		ID:    "m1",
		Title: "测试电影",
	}
	app := &appState{
		section:   "library",
		libraryID: "movies",
		selected:  &movie,
		catalogs:  map[string]*CatalogState{},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	if app.selected == nil {
		t.Fatalf("expected movie to be selected initially")
	}

	// Click the icon back button, with an accessibility label and no tooltip.
	if err := tester.Click("返回影视库"); err != nil {
		t.Fatal(err)
	}

	if app.selected != nil {
		t.Fatalf("expected selected to be nil after clicking back button")
	}
}

func TestPlaybackLoadingFeedback(t *testing.T) {
	movie := MediaItem{
		ID:    "m1",
		Title: "测试电影",
	}
	app := &appState{
		section:              "library",
		libraryID:            "movies",
		selected:             &movie,
		catalogs:             map[string]*CatalogState{},
		playbackLoading:      true,
		playbackLoadingID:    "m1",
		playbackLoadingTitle: "测试电影",
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	// In detail view, the action button displays loading state
	if _, ok := tester.Find("正在准备播放…"); !ok {
		t.Fatalf("expected '正在准备播放…' in detail view button")
	}
	// Preparing playback must not place a modal over the detail page.
	tester.Click("返回影视库")
	if app.selected != nil {
		t.Fatal("playback loading blocked detail navigation")
	}
	if app.playbackLoading {
		t.Fatal("returning from detail should cancel pending playback")
	}

	// Cancel loading
	app.cancelPlaybackLoading()
	if app.playbackLoading {
		t.Fatalf("expected playbackLoading to be false after cancel")
	}
}

func TestPlayerOverlayViews(t *testing.T) {
	app := &appState{
		playback: PlaybackState{
			Active:   true,
			Title:    "流浪地球 2",
			Duration: 7200,
			Position: 1200,
			Volume:   80,
			Quality:  "4K HDR",
		},
	}

	headerTester := ui.NewTester(func(c *ui.Context) {
		app.playerHeaderView(c)
	}, 1280, 64)

	if _, ok := headerTester.Find("返回影片库"); !ok {
		t.Fatalf("player header back button missing")
	}
	if _, ok := headerTester.Find("流浪地球 2"); !ok {
		t.Fatal("player header should show the current title")
	}
	if _, ok := headerTester.Find("4K HDR"); ok {
		t.Fatal("player header should omit playback quality")
	}

	transportTester := ui.NewTester(func(c *ui.Context) {
		app.playerTransport(c)
	}, 1280, 98)

	if _, ok := transportTester.Find("00:20:00"); !ok {
		t.Fatalf("current position timestamp missing")
	}
	if _, ok := transportTester.Find("02:00:00"); !ok {
		t.Fatalf("duration timestamp missing")
	}
}

func TestPersonViewLayoutAndNavigation(t *testing.T) {
	person := CastMember{
		ID:   "p-jiangwen",
		Name: "姜文",
		Role: "蓝青峰",
	}
	app := &appState{
		section:        "library",
		libraryID:      "movies",
		selectedPerson: &person,
		personItems: []MediaItem{
			{ID: "m1", Title: "邪不压正", Year: "2018", Rating: "7.2"},
			{ID: "m2", Title: "让子弹飞", Year: "2010", Rating: "9.0"},
		},
		catalogs: map[string]*CatalogState{},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	if _, ok := tester.Find("姜文"); !ok {
		t.Fatalf("expected person name '姜文'")
	}
	if _, ok := tester.Find("参演与相关作品"); !ok {
		t.Fatalf("expected '参演与相关作品' section")
	}
	if _, ok := tester.Find("邪不压正"); !ok {
		t.Fatalf("expected '邪不压正' movie card")
	}
	if _, ok := tester.Find("让子弹飞"); !ok {
		t.Fatalf("expected '让子弹飞' movie card")
	}
	saveDetailPreview(t, tester, "person")

	// Close person view
	app.closePerson()
	if app.selectedPerson != nil {
		t.Fatalf("expected selectedPerson to be nil after closePerson")
	}
}

func TestPersonImmersiveLayoutAtMinimumWindow(t *testing.T) {
	person := CastMember{ID: "p-minimum", Name: "最小窗口演员", Job: "演员"}
	a := &appState{
		selectedPerson: &person,
		personItems:    []MediaItem{{ID: "work", Title: "代表作品", Year: "2025"}},
		catalogs:       map[string]*CatalogState{},
	}
	tester := ui.NewTester(a.view, 960, 640)
	portrait, portraitOK := tester.Find("演员头像")
	work, workOK := tester.Find("代表作品")
	if !portraitOK || portrait.W <= 0 || portrait.H <= 0 || !workOK || work.W <= 0 || work.H <= 0 {
		t.Fatalf("actor identity and works should fit the minimum window: portrait=%+v works=%+v", portrait, work)
	}
	if tester.HasText("首页") || !tester.HasText("演职人员") {
		t.Fatal("minimum-size actor page should remain immersive")
	}
}

func TestPersonPortraitScalesWithWindow(t *testing.T) {
	for _, size := range [][3]int{{960, 640, 430}, {1280, 800, 580}, {1920, 1080, 680}} {
		person := CastMember{ID: "person", Name: "演员", Job: "演员"}
		a := &appState{selectedPerson: &person, personItems: []MediaItem{{ID: "work", Title: "代表作品"}}, catalogs: map[string]*CatalogState{}}
		tester := ui.NewTester(a.view, size[0], size[1])
		portrait, ok := tester.Find("演员头像")
		work, workOK := tester.Find("代表作品")
		if !ok || portrait.H < float32(size[2]) || portrait.Y < movieTitleBarHeight || portrait.Y+portrait.H > float32(size[1]) {
			t.Fatalf("portrait does not fit the window: size=%v portrait=%+v", size, portrait)
		}
		if !workOK || work.X <= portrait.X+portrait.W || work.Y+work.H > float32(size[1]) {
			t.Fatalf("works should remain visible beside the portrait: size=%v portrait=%+v work=%+v", size, portrait, work)
		}
	}
}

func saveDetailPreview(t *testing.T, tester *ui.Tester, name string) {
	t.Helper()
	if os.Getenv("FNMOVIE_UI_PREVIEW") != "1" {
		return
	}
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Frame()
	dir := filepath.Join("..", "..", "out", "detail-check")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tester.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestFavoritePersonNavigationAndSectionSwitch(t *testing.T) {
	personItem := MediaItem{
		ID:       "p-yui",
		Title:    "新垣结衣",
		Kind:     "person",
		Favorite: true,
	}
	app := &appState{
		section: "favorites",
		items:   []MediaItem{personItem},
		catalogs: map[string]*CatalogState{
			catalogStateKey("", "favorite", ""): {
				Items: []MediaItem{personItem},
			},
		},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
	}, 1280, 800)

	if _, ok := tester.Find("新垣结衣"); !ok {
		t.Fatalf("expected favorite person card '新垣结衣' to be visible")
	}

	// Click person card to open person view
	app.openPerson(CastMember{ID: personItem.ID, Name: personItem.Title, Favorite: personItem.Favorite})
	if app.selectedPerson == nil || app.selectedPerson.Name != "新垣结衣" {
		t.Fatalf("expected selectedPerson to be set")
	}
	tester.Frame()
	button, ok := tester.Find("取消收藏")
	if !ok {
		t.Fatal("favorite actor must expose an unfavorite button")
	}
	name, ok := tester.Find("新垣结衣")
	if !ok || button.X <= name.X+name.W {
		t.Fatalf("favorite button should follow the actor name: button=%+v name=%+v", button, name)
	}
	app.updateFavoriteProjection(personItem.ID, false)
	tester.Frame()
	if app.selectedPerson.Favorite || app.items[0].Favorite || !tester.HasText("收藏") {
		t.Fatal("actor and favorites list must reflect the updated favorite state")
	}

	// Switch section to movies
	app.section = "movies"
	app.closePerson()
	if app.selectedPerson != nil {
		t.Fatalf("expected selectedPerson to be nil after switching section")
	}
}
