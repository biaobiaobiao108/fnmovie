//go:build windows

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestNativeHomeVerification(t *testing.T) {
	if os.Getenv("FNMOVIE_NATIVE_HOME_TEST") != "1" {
		t.Skip("opt in to native home verification")
	}
	if nativeOverlayResult != nil {
		t.Fatal(nativeOverlayResult)
	}
}

func TestHomeNavigationAndVirtualization(t *testing.T) {
	a := &appState{section: "home", catalogs: map[string]*CatalogState{}, home: homeState{Heroes: []MediaItem{{ID: "one", Title: "第一张海报"}, {ID: "two", Title: strings.Repeat("长标题", 20)}}}}
	tester := ui.NewTester(a.view, 1280, 800)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Frame()
	if err := tester.Click("下一张海报"); err != nil {
		t.Fatal(err)
	}
	if a.home.HeroIndex != 1 || a.selected != nil {
		t.Fatalf("arrow navigated away from carousel: index=%d selected=%v", a.home.HeroIndex, a.selected)
	}
	if !tester.HasText("还没有观看记录") {
		t.Fatal("empty continue state is not visible")
	}
	a.home.Continue = make([]ContinueWatchingItem, 1000)
	for i := range a.home.Continue {
		a.home.Continue[i] = ContinueWatchingItem{RecordGUID: fmt.Sprintf("record-%d", i), Media: MediaItem{Title: fmt.Sprintf("继续影片 %d", i)}, Position: 120, Duration: 3600}
	}
	a.home.AllContinue = true
	tester.Frame()
	count := 0
	for _, text := range tester.Texts() {
		if strings.HasPrefix(text, "继续影片 ") {
			count++
		}
	}
	if count == 0 || count > 40 {
		t.Fatalf("1000 records created %d visible card titles, want bounded grid", count)
	}
	if err := tester.Click("返回首页"); err != nil || a.home.AllContinue {
		t.Fatalf("return to home: %v", err)
	}
}

func TestContinuePlaybackUsesCurrentServerPosition(t *testing.T) {
	var requested string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/play/info":
			var body struct {
				ID string `json:"item_guid"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			requested = body.ID
			fmt.Fprint(w, `{"code":0,"data":{"media_guid":"media-episode","ts":321}}`)
		case "/api/v1/stream":
			fmt.Fprint(w, `{"code":0,"data":{"video_stream":{"duration":7200}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	record := ContinueWatchingItem{RecordGUID: "episode-guid", Media: MediaItem{ID: "tv-guid", Title: "剧集"}, Position: 12, Duration: 7200}
	item := record.Media
	item.ID = record.RecordGUID
	stream, err := NewServer(server.URL, "synthetic-token").Playback(item)
	if err != nil || requested != record.RecordGUID || stream.ResumeAt != 321 || stream.ResumeAt == record.Position {
		t.Fatalf("request GUID=%q, result=%+v, err=%v", requested, stream, err)
	}
}

// All requests and artwork are synthetic; this test never changes NAS history.
func runNativeHomeVerification() error {
	var artwork bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 800, 450))
	for y := 0; y < 450; y++ {
		for x := 0; x < 800; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(30 + x/5), G: uint8(80 + y/4), B: 100, A: 255})
		}
	}
	_ = png.Encode(&artwork, img)
	var requests atomic.Int32
	var blockOld atomic.Bool
	oldEntered, oldRelease := make(chan struct{}), make(chan struct{})
	resumeRequest := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(artwork.Bytes())
			return
		}
		if strings.HasSuffix(r.URL.Path, "/play/list") {
			requests.Add(1)
			if blockOld.CompareAndSwap(true, false) {
				close(oldEntered)
				<-oldRelease
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []map[string]any{{"guid": "resume", "title": "合成观看记录", "type": "Movie", "backdrops": []string{"/art.png"}, "ts": 125, "duration": 7200}}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/play/info") {
			var body struct {
				ID string `json:"item_guid"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			resumeRequest <- body.ID
			// Stop before libmpv loading; the API position behavior is checked
			// separately above and native decode is covered by the player suite.
			fmt.Fprint(w, `{"code":400,"msg":"synthetic playback boundary"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	result := make(chan error, 1)
	mygo.App.WhenReady(func() {
		a := &appState{section: "home", loggedIn: true, catalogs: map[string]*CatalogState{}, server: NewServer(server.URL, "synthetic-token"), posters: NewPosterLoader(), displayScale: 1}
		a.home.HeroAttempted = true
		for i := 0; i < 8; i++ {
			a.home.Heroes = append(a.home.Heroes, MediaItem{ID: fmt.Sprintf("hero-%d", i), Title: fmt.Sprintf("合成大海报 %d · %s", i+1, strings.Repeat("超长影片标题", 4)), Backdrop: server.URL + "/art.png", Rating: "8.7", Year: "2026", Genres: []string{"剧情", "冒险"}, Countries: []string{"中国"}, Overview: strings.Repeat("这是原生首页画面验证用的简介，检查长文字换行与海报边缘间距。", 6)})
		}
		a.window = mygo.NewWindow(mygo.WindowOptions{Title: "FnMovie synthetic home verification", Width: 1280, Height: 800, Content: ui.View(a.view)})
		a.loadContinueWatching(true)
		go func() {
			update := func(fn func() error) error {
				done := make(chan error, 1)
				a.window.Update(func() { done <- fn() })
				return <-done
			}
			fail := func(err error) { result <- err; mygo.App.Quit() }
			deadline := time.Now().Add(4 * time.Second)
			for {
				time.Sleep(100 * time.Millisecond)
				loaded := false
				_ = update(func() error { loaded = len(a.home.Continue) == 1 && !a.home.ContinueLoading; return nil })
				if loaded {
					break
				}
				if time.Now().After(deadline) {
					fail(fmt.Errorf("async continue results never entered UI; requests=%d", requests.Load()))
					return
				}
			}
			for _, size := range [][2]int{{960, 640}, {1280, 800}, {1920, 1080}} {
				_ = update(func() error { a.window.SetSize(size[0], size[1]); return nil })
				time.Sleep(350 * time.Millisecond)
				if err := update(func() error {
					data, err := a.window.CapturePage()
					if err != nil {
						return err
					}
					dir := filepath.Join("..", "..", "out", "home-check")
					if err := os.MkdirAll(dir, 0755); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(dir, fmt.Sprintf("native-%dx%d.png", size[0], size[1])), data, 0600)
				}); err != nil {
					fail(err)
					return
				}
			}
			_ = update(func() error {
				record := ContinueWatchingItem{RecordGUID: "episode-guid", Media: MediaItem{ID: "tv-guid", Title: "测试分集"}, Position: 12}
				tester := ui.NewTester(func(c *ui.Context) { a.homeContinueCard(c, record, 272) }, 300, 224)
				return tester.Click("继续观看 测试分集")
			})
			select {
			case guid := <-resumeRequest:
				if guid != "episode-guid" {
					fail(fmt.Errorf("continue card requested %q, want actual Episode GUID", guid))
					return
				}
			case <-time.After(2 * time.Second):
				fail(fmt.Errorf("continue card did not enter existing playback API"))
				return
			}
			time.Sleep(120 * time.Millisecond)
			if err := update(func() error {
				a.home.HeroHover = true
				a.syncHomeCarousel()
				if a.home.Timer != nil {
					return fmt.Errorf("hover failed to pause timer")
				}
				a.home.HeroHover = false
				a.syncHomeCarousel()
				if a.home.Timer == nil {
					return fmt.Errorf("leaving hover failed to resume timer")
				}
				return nil
			}); err != nil {
				fail(err)
				return
			}
			blockOld.Store(true)
			_ = update(func() error { a.loadContinueWatching(true); return nil })
			select {
			case <-oldEntered:
			case <-time.After(2 * time.Second):
				close(oldRelease)
				fail(fmt.Errorf("stale-request fixture did not reach server"))
				return
			}
			_ = update(func() error { a.resetHome(); a.home.Closed = true; return nil })
			close(oldRelease)
			time.Sleep(120 * time.Millisecond)
			if err := update(func() error {
				if a.home.Timer != nil || len(a.home.Continue) != 0 || a.home.Generation != 1 || !a.home.ContinueUpdatedAt.IsZero() {
					return fmt.Errorf("old account response repopulated reset home state")
				}
				return nil
			}); err != nil {
				fail(err)
				return
			}
			result <- nil
			mygo.App.Quit()
		}()
	})
	mygo.App.Run()
	return <-result
}
