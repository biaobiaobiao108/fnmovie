package app

import (
	"container/list"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestContinueLoadingSpinnerVisibleOverArtwork(t *testing.T) {
	const width, height = 272, 153
	artwork := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			artwork.SetRGBA(x, y, color.RGBA{R: 16, G: 160, B: 48, A: 255})
		}
	}
	url := "http://synthetic.invalid/artwork.png"
	posters := &PosterLoader{entries: map[posterKey]*list.Element{}, lru: list.New(), maxBytes: posterCacheBudget}
	posters.putLocked(posterKey{URL: url, Width: width, Height: height}, ui.NewBitmap(artwork), width*height*4)
	a := &appState{server: NewServer("http://synthetic.invalid", ""), posters: posters, displayScale: 1, playbackLoading: true, playbackLoadingID: "pending-record"}
	record := ContinueWatchingItem{RecordGUID: "pending-record", Media: MediaItem{Title: "加载中的影片", Backdrop: url}, Position: 10, Duration: 60}
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(movieTheme())
		a.homeContinueCard(c, record, width)
	}, width, 220)
	tester.Frame()
	spinner, found := tester.Find("正在准备播放，点击卡片取消")
	if !found || spinner.W != 22 || spinner.H != 22 {
		t.Fatalf("loading progress indicator missing or collapsed: %+v", spinner)
	}
	first := tester.Image()
	bright := 0
	for y := int(spinner.Y); y < int(spinner.Y+spinner.H); y++ {
		for x := int(spinner.X); x < int(spinner.X+spinner.W); x++ {
			pixel := first.RGBAAt(x, y)
			if pixel.R > 180 && pixel.G > 180 && pixel.B > 180 {
				bright++
			}
		}
	}
	if bright < 4 {
		t.Fatalf("white spinner is hidden behind artwork or too dark: %d bright pixels", bright)
	}
	// Check the rendered spokes advance while the remote request is pending.
	time.Sleep(90 * time.Millisecond)
	tester.Frame()
	second := tester.Image()
	changed := false
	for y := int(spinner.Y); y < int(spinner.Y+spinner.H); y++ {
		for x := int(spinner.X); x < int(spinner.X+spinner.W); x++ {
			changed = changed || first.RGBAAt(x, y) != second.RGBAAt(x, y)
		}
	}
	if !changed {
		t.Fatal("pending spinner did not animate")
	}
	if err := tester.Click("继续观看 加载中的影片"); err != nil {
		t.Fatal(err)
	}
	if a.playbackLoading {
		t.Fatal("clicking a pending card did not cancel loading")
	}
	if _, found := tester.Find("正在准备播放，点击卡片取消"); found {
		t.Fatal("cancelled card retained loading indicator")
	}
}

func TestHomeFitsViewport(t *testing.T) {
	for _, size := range [][2]int{{960, 640}, {1280, 800}, {1920, 1080}} {
		for _, status := range []string{"ready", "loading", "error"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], status), func(t *testing.T) {
				a := &appState{section: "home", catalogs: map[string]*CatalogState{}}
				a.home.Heroes = []MediaItem{{ID: "hero", Title: strings.Repeat("长影片标题", 16), Overview: strings.Repeat("这是影片的长简介。", 40), Genres: []string{strings.Repeat("动作、冒险、剧情", 12)}, Countries: []string{strings.Repeat("中国 / 美国 / 英国", 8)}, Rating: "9.5", Year: "2026"}, {ID: "next", Title: "下一张"}}
				for i := 0; i < 10; i++ {
					a.home.Continue = append(a.home.Continue, ContinueWatchingItem{RecordGUID: fmt.Sprintf("record-%d", i), Media: MediaItem{Title: fmt.Sprintf("续播影片 %d", i), SeasonNumber: 1, EpisodeNumber: 2}, Position: 10, Duration: 60})
				}
				if status == "loading" {
					a.home.ContinueLoading = true
				} else if status == "error" {
					a.home.HeroErr = "合成部分加载错误"
					a.home.ContinueErr = "合成观看记录错误"
				}
				tester := ui.NewTester(a.view, size[0], size[1])
				tester.SetPreferences(ui.Preferences{ReduceMotion: true})
				tester.Frame()
				tester.Frame()
				for _, label := range []string{"下一张海报", "继续观看", "续播影片 0", "第 1 季 · 第 2 集"} {
					bounds, ok := tester.Find(label)
					if !ok || bounds.Y < 0 || bounds.Y+bounds.H > float32(size[1]) {
						t.Fatalf("%q outside viewport: %+v (found=%v)", label, bounds, ok)
					}
				}
				title, ok := tester.Find(a.home.Heroes[0].Title)
				if !ok {
					t.Fatal("hero title is not visible")
				}
				for _, label := range []string{"上一张海报", "下一张海报"} {
					arrow, found := tester.Find(label)
					if !found || (arrow.X < title.X+title.W && arrow.X+arrow.W > title.X && arrow.Y < title.Y+title.H && arrow.Y+arrow.H > title.Y) {
						t.Fatalf("%s overlaps title: arrow=%+v title=%+v", label, arrow, title)
					}
				}
				before, _ := tester.Find("下一张海报")
				tester.Scroll(before.X+before.W/2, before.Y+before.H/2, 0, -400)
				after, _ := tester.Find("下一张海报")
				if before != after {
					t.Fatalf("home unexpectedly scrolled vertically: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}
