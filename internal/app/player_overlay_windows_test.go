//go:build windows

package app

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Run separately: FNMOVIE_NATIVE_UI_TEST=1 go test ./internal/app
// -run TestNativePlaybackOverlayComposition -count=1. It opens only synthetic
// content, never a NAS, and checks the actual desktop composite, not a UI mock.
var nativeOverlayResult error

func TestMain(m *testing.M) {
	if os.Getenv("FNMOVIE_NATIVE_UI_TEST") == "1" {
		nativeOverlayResult = runNativePlaybackOverlayComposition()
	}
	os.Exit(m.Run())
}

func TestNativePlaybackOverlayComposition(t *testing.T) {
	if os.Getenv("FNMOVIE_NATIVE_UI_TEST") != "1" {
		t.Skip("opt in to native desktop overlay verification")
	}
	if nativeOverlayResult != nil {
		t.Fatal(nativeOverlayResult)
	}
}

func runNativePlaybackOverlayComposition() error {
	result := make(chan error, 1)
	mygo.App.WhenReady(func() {
		a := &appState{player: NewPlayer(), playback: PlaybackState{Active: true, Title: "透明控件验证", Duration: 7200, Position: 1200, Volume: 80, Speed: 1}}
		a.window = mygo.NewWindow(mygo.WindowOptions{Title: "FnMovie overlay test", Width: 1000, Height: 650, AlwaysOnTop: true, Content: ui.View(func(c *ui.Context) {
			ui.Box(c).Fill().Background(ui.RGB(24, 164, 98))
		})})
		a.createPlayerOverlay()
		a.window.Focus()
		go func() {
			time.Sleep(800 * time.Millisecond)
			a.window.Update(func() {
				bounds := a.window.ContentBounds()
				scale := windowScale(a.window.NativeHandle())
				if png, err := a.window.CapturePage(); err == nil {
					_ = os.MkdirAll("../../out/overlay-check", 0755)
					_ = os.WriteFile("../../out/overlay-check/main.png", png, 0600)
				}
				if png, err := a.playerOverlayWindow.CapturePage(); err == nil {
					_ = os.WriteFile("../../out/overlay-check/controls.png", png, 0600)
				}
				getDC := syscall.NewLazyDLL("user32.dll").NewProc("GetDC")
				release := syscall.NewLazyDLL("user32.dll").NewProc("ReleaseDC")
				getPixel := syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")
				dc, _, _ := getDC.Call(0)
				defer release.Call(0, dc)
				checks := []struct {
					x, y  int
					clear bool
				}{
					{10, 10, true}, {bounds.Width - 10, 20, true}, {10, bounds.Height - 40, true},
					{45, bounds.Height - 45, false},
				}
				var err error
				for _, check := range checks {
					color, _, _ := getPixel.Call(dc, uintptr(float64(bounds.X+check.x)*scale), uintptr(float64(bounds.Y+check.y)*scale))
					r, g, b := int(color&255), int((color>>8)&255), int((color>>16)&255)
					if check.clear && (absInt(r-24) > 3 || absInt(g-164) > 3 || absInt(b-98) > 3) {
						err = fmt.Errorf("transparent margin (%d,%d) = RGB(%d,%d,%d), want video background", check.x, check.y, r, g, b)
						break
					}
					if !check.clear && (g < 55 || g > 130) {
						err = fmt.Errorf("control backplate green=%d, want translucent blend rather than opaque black", g)
						break
					}
				}
				a.closePlayerOverlay()
				if err != nil {
					result <- err
					mygo.App.Quit()
					return
				}
				go checkNativePlaybackSessions(a, result)
			})
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return <-result
}

func checkNativePlaybackSessions(a *appState, result chan<- error) {
	// Reach the actual app -> API -> local proxy -> libmpv path with synthetic
	// media. Cancel A while its API request is blocked, then play B for longer
	// than the reported ten-second failure window.
	cwd, err := os.Getwd()
	if err != nil {
		result <- err
		a.window.Update(mygo.App.Quit)
		return
	}
	if err = os.Chdir("../.."); err != nil {
		result <- err
		a.window.Update(mygo.App.Quit)
		return
	}
	defer os.Chdir(cwd)
	data := make([]byte, 44+16000*20)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/play/info":
			var body struct {
				ID string `json:"item_guid"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.ID == "cancelled" {
				close(entered)
				<-release
			}
			fmt.Fprint(w, `{"code":0,"data":{"media_guid":"synthetic","ts":0}}`)
		case "/api/v1/stream":
			fmt.Fprint(w, `{"code":0,"data":{"video_stream":{"duration":20},"direct_link_qualities":[{"url":"/media.wav","resolution":"test"}]}}`)
		case "/media.wav":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(data)
		case "/api/v1/play/record":
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	onUI := func(fn func()) { done := make(chan struct{}); a.window.Update(func() { fn(); close(done) }); <-done }
	finish := func(err error) { onUI(func() { a.closePlayer(); _ = os.Chdir(cwd); result <- err; mygo.App.Quit() }) }
	if err := checkNativeSeriesCast(a, onUI); err != nil {
		finish(err)
		return
	}
	onUI(func() {
		a.playback = PlaybackState{}
		a.server = NewServer(server.URL, "synthetic-token")
		a.startPlayback(MediaItem{ID: "cancelled", Title: "取消请求"})
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		finish(fmt.Errorf("cancelled request did not reach API"))
		return
	}
	onUI(func() { a.cancelPlaybackLoading(); a.startPlayback(MediaItem{ID: "current", Title: "当前播放"}) })
	close(release)
	deadline := time.Now().Add(8 * time.Second)
	for {
		ready, failed := false, ""
		onUI(func() {
			ready = a.playback.Active && a.playerOverlayWindow != nil && a.playerHeaderWindow != nil
			failed = a.status
		})
		if ready {
			break
		}
		if time.Now().After(deadline) {
			finish(fmt.Errorf("new playback/controls never became ready: %s", failed))
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(12 * time.Second)
	var finalErr error
	onUI(func() {
		if !a.playback.Active || !a.player.Running() || a.playback.Position < 10 {
			finalErr = fmt.Errorf("playback unexpectedly stopped: active=%t running=%t position=%v status=%s", a.playback.Active, a.player.Running(), a.playback.Position, a.status)
			return
		}
		a.showPlayerOverlay()
	})
	if finalErr != nil {
		finish(finalErr)
		return
	}
	time.Sleep(400 * time.Millisecond)
	onUI(func() {
		if a.playerOverlayOpacity < 0.99 || !a.playerOverlayVisible {
			finalErr = fmt.Errorf("playback controls failed to reappear")
		}
	})
	finish(finalErr)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Exercise the asynchronous credits path on the real UI dispatcher, including
// changing seasons while an older response is still in flight.
func checkNativeSeriesCast(a *appState, onUI func(func())) error {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/person/list/old-season":
			close(entered)
			<-release
			fmt.Fprint(w, `{"code":0,"data":{"list":[{"guid":"old-person","name":"旧季度演员"}]}}`)
		case "/api/v1/person/list/current-season":
			fmt.Fprint(w, `{"code":0,"data":{"list":[]}}`)
		case "/api/v1/person/list/first-episode":
			fmt.Fprint(w, `{"code":0,"data":{"list":[{"guid":"current-person","name":"当前演员"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	onUI(func() {
		a.server = NewServer(server.URL, "synthetic-token")
		a.selected = &MediaItem{ID: "tv-root", IsSeries: true}
		a.selectedSeasonID = "old-season"
		a.loadSeriesCast("old-season", nil)
	})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		return fmt.Errorf("season credits request never started")
	}
	onUI(func() {
		a.selectedSeasonID = "current-season"
		a.loadSeriesCast("current-season", []MediaItem{{ID: "first-episode"}})
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		ready := false
		onUI(func() {
			ready = !a.castLoading && len(a.selected.Cast) == 1 && a.selected.Cast[0].ID == "current-person"
		})
		if ready {
			break
		}
		if time.Now().After(deadline) {
			close(release)
			return fmt.Errorf("episode credits fallback failed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(release)
	time.Sleep(150 * time.Millisecond)
	var err error
	onUI(func() {
		if len(a.selected.Cast) != 1 || a.selected.Cast[0].ID != "current-person" {
			err = fmt.Errorf("old season credits replaced the current season")
		}
		a.selected = nil
	})
	return err
}
