package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

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

	rectSource, okSource := tester.Find("4K 原画")
	if !okSource || rectSource.W <= 0 || rectSource.H <= 0 {
		t.Fatalf("Movie source version not visible: ok=%v, rect=%+v", okSource, rectSource)
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
			{ID: "ep-1", Title: "这是一个美好的日子", EpisodeNumber: 1},
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

	rectSeason, okSeason := tester.Find("第 1 季")
	if !okSeason || rectSeason.W <= 0 || rectSeason.H <= 0 {
		t.Fatalf("Series season button not visible: ok=%v, rect=%+v", okSeason, rectSeason)
	}

	rectEp1, okEp1 := tester.Find("这是一个美好的日子")
	if !okEp1 || rectEp1.W <= 0 || rectEp1.H <= 0 {
		t.Fatalf("Episode 1 not visible: ok=%v, rect=%+v", okEp1, rectEp1)
	}

	rectCast, okCast := tester.Find("大卫·马丁内斯")
	if !okCast || rectCast.W <= 0 || rectCast.H <= 0 {
		t.Fatalf("Cast not visible: ok=%v, rect=%+v", okCast, rectCast)
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

	// Episode 1 should be visible
	r1, ok1 := tester.Find("第1集")
	if !ok1 || r1.W <= 0 || r1.H <= 0 {
		t.Fatalf("Episode 1 should be visible initially, got ok=%v, rect=%+v", ok1, r1)
	}

	// Scroll down within the scroll container
	tester.Scroll(600, 300, 0, 800)

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

	// Click back button (Tooltip: "返回影视库")
	// detailBackButton is placed at X=239, Y=70 with Size 36x34
	tester.ClickAt(250, 85)

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

	if _, ok := headerTester.Find("流浪地球 2"); !ok {
		t.Fatalf("player header title missing")
	}
	if _, ok := headerTester.Find("4K HDR"); !ok {
		t.Fatalf("player header quality badge missing")
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

	// Close person view
	app.closePerson()
	if app.selectedPerson != nil {
		t.Fatalf("expected selectedPerson to be nil after closePerson")
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
	app.openPerson(CastMember{ID: personItem.ID, Name: personItem.Title})
	if app.selectedPerson == nil || app.selectedPerson.Name != "新垣结衣" {
		t.Fatalf("expected selectedPerson to be set")
	}

	// Switch section to movies
	app.section = "movies"
	app.closePerson()
	if app.selectedPerson != nil {
		t.Fatalf("expected selectedPerson to be nil after switching section")
	}
}
