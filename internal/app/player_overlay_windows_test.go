//go:build windows

package app

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

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
	} else if os.Getenv("FNMOVIE_NATIVE_HOME_TEST") == "1" {
		nativeOverlayResult = runNativeHomeVerification()
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
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title: "FnMovie overlay test", Width: 1000, Height: 650, AlwaysOnTop: true,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: movieTitleBarHeight,
			Vibrancy: mygo.VibrancySidebar, BackgroundColor: movieWindowBackground,
			Content: ui.View(func(c *ui.Context) {
				a.advancePlayerFullscreen(c)
				a.advancePlayerOverlayAnimation(c)
				if a.player.Running() {
					a.playerView(c)
					return
				}
				ui.Box(c).Fill().Background(ui.RGB(24, 164, 98))
			}),
		})
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
					x, y   int
					header bool
					clear  bool
				}{
					{10, 10, false, true}, {bounds.Width - 10, 20, false, true}, {10, bounds.Height - playerOverlayControlHeight - 10, false, true},
					{bounds.Width - 10, bounds.Height - 45, false, false},
				}
				var err error
				for _, check := range checks {
					color, _, _ := getPixel.Call(dc, uintptr(float64(bounds.X+check.x)*scale), uintptr(float64(bounds.Y+check.y)*scale))
					r, g, b := int(color&255), int((color>>8)&255), int((color>>16)&255)
					if check.header && (absInt(r-int(movieColors.sidebar.R)) > 3 || absInt(g-int(movieColors.sidebar.G)) > 3 || absInt(b-int(movieColors.sidebar.B)) > 3) {
						err = fmt.Errorf("player titlebar RGB(%d,%d,%d), want the matched titlebar surface", r, g, b)
						break
					}
					if check.clear && (absInt(r-24) > 3 || absInt(g-164) > 3 || absInt(b-98) > 3) {
						err = fmt.Errorf("transparent margin (%d,%d) = RGB(%d,%d,%d), want video background", check.x, check.y, r, g, b)
						break
					}
					if !check.header && !check.clear && (g < 45 || g > 130) {
						err = fmt.Errorf("control fade green=%d, want translucent blend rather than opaque black", g)
						break
					}
				}
				if err != nil {
					a.closePlayerOverlay()
					result <- err
					mygo.App.Quit()
					return
				}
				go func() {
					err := checkNativePlayerMenus(a)
					a.window.Update(func() {
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
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return <-result
}

// Scan the actual desktop while repeatedly opening fresh native menu surfaces.
// Pixels outside the menu and bottom control bar must remain the green video
// background, including during the first frame when the old implementation
// moved the last progress-bar bitmap into the middle of the video.
func checkNativePlayerMenus(a *appState) error {
	onUI := func(fn func()) { done := make(chan struct{}); a.window.Update(func() { fn(); close(done) }); <-done }
	var transportBounds mygo.Rectangle
	var mainBounds mygo.Rectangle
	onUI(func() { transportBounds = a.playerOverlayWindow.ContentBounds(); mainBounds = a.window.ContentBounds() })
	getDC := syscall.NewLazyDLL("user32.dll").NewProc("GetDC")
	release := syscall.NewLazyDLL("user32.dll").NewProc("ReleaseDC")
	getPixel := syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")
	for _, option := range []string{"subtitle", "audio", "speed", "subtitle", "audio"} {
		var err error
		onUI(func() {
			tracks := make([]PlayerTrack, 25)
			for i := range tracks {
				tracks[i] = PlayerTrack{ID: i + 1, Title: strings.Repeat("长轨道名称", 25)}
			}
			if option == "subtitle" {
				tracks = tracks[:1]
			}
			a.playback.AudioTracks, a.playback.SubtitleTracks = tracks, tracks
			a.openPlayerOverlayMenu(option)
			// OS window placement may finish while this test starts. Compare
			// placement relative to the video, allowing legitimate main-window moves.
			currentMain, currentTransport := a.window.ContentBounds(), a.playerOverlayWindow.ContentBounds()
			if currentTransport.Width != transportBounds.Width || currentTransport.Height != transportBounds.Height || currentTransport.X-currentMain.X != transportBounds.X-mainBounds.X || currentTransport.Y-currentMain.Y != transportBounds.Y-mainBounds.Y {
				err = fmt.Errorf("%s menu moved or resized the transport surface: before=%+v after=%+v main=%+v", option, transportBounds, a.playerOverlayWindow.ContentBounds(), a.window.ContentBounds())
			}
			if a.playerMenuWindow.ContentBounds().Width != playerMenuWidth || a.playerMenuWindow.ContentBounds().Height != playerMenuHeight {
				err = fmt.Errorf("%s native menu has inconsistent dimensions: %+v", option, a.playerMenuWindow.ContentBounds())
			}
		})
		if err != nil {
			return err
		}
		for frame := range 5 {
			onUI(func() {
				wantTracks := 25
				if option == "subtitle" {
					wantTracks = 1
				}
				if option != "speed" && len(a.playerMenuTracks) != wantTracks {
					err = fmt.Errorf("%s menu lost its track snapshot: %d", option, len(a.playerMenuTracks))
					return
				}
				bounds := a.window.ContentBounds()
				scale := windowScale(a.window.NativeHandle())
				dc, _, _ := getDC.Call(0)
				defer release.Call(0, dc)
				for y := playerOverlayHeaderHeight + 20; y < bounds.Height-playerOverlayControlHeight; y += 20 {
					color, _, _ := getPixel.Call(dc, uintptr(float64(bounds.X+bounds.Width/2)*scale), uintptr(float64(bounds.Y+y)*scale))
					r, g, b := int(color&255), int((color>>8)&255), int((color>>16)&255)
					if absInt(r-24) > 3 || absInt(g-164) > 3 || absInt(b-98) > 3 {
						err = fmt.Errorf("%s menu frame %d left a ghost at center y=%d: RGB(%d,%d,%d), main=%+v menu=%+v controls=%+v", option, frame, y, r, g, b, bounds, a.playerMenuWindow.ContentBounds(), a.playerOverlayWindow.ContentBounds())
						break
					}
				}
			})
			if err != nil {
				return err
			}
			time.Sleep(20 * time.Millisecond)
		}
		onUI(func() {
			if png, captureErr := a.playerMenuWindow.CapturePage(); captureErr == nil {
				_ = os.WriteFile("../../out/overlay-check/menu-"+option+".png", png, 0600)
			}
			a.closePlayerOverlayMenu()
			if a.playerMenuWindow != nil {
				err = fmt.Errorf("closed menu surface was retained")
			}
		})
		if err != nil {
			return err
		}
	}
	return checkNativeFullscreen(a, onUI)
}

func checkNativeFullscreen(a *appState, onUI func(func())) error {
	for _, target := range []bool{true, false} {
		onUI(func() { a.requestPlayerFullscreen(target, false) })
		deadline := time.Now().Add(3 * time.Second)
		for {
			ready := false
			onUI(func() {
				ready = a.fullscreenMotion.started.IsZero() && a.window.IsFullScreen() == target && a.window.Opacity() > 0.99
			})
			if ready {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("fullscreen transition to %t failed or left window translucent", target)
			}
			time.Sleep(20 * time.Millisecond)
		}
		var err error
		onUI(func() {
			main, controls := a.window.ContentBounds(), a.playerOverlayWindow.ContentBounds()
			if controls.Height != playerOverlayControlHeight || controls.Y+controls.Height != main.Y+main.Height {
				err = fmt.Errorf("fullscreen transition detached controls: main=%+v controls=%+v", main, controls)
			}
		})
		if err != nil {
			return err
		}
	}
	// A quick reversed request cancels the pending transition and restores opacity.
	onUI(func() { a.togglePlayerFullscreen(false); a.togglePlayerFullscreen(false) })
	var err error
	onUI(func() {
		if !a.fullscreenMotion.started.IsZero() || a.window.Opacity() < 0.99 || a.window.IsFullScreen() {
			err = fmt.Errorf("reversed fullscreen request left stale transition state")
		}
		a.requestPlayerFullscreen(true, true)
		if !a.fullscreenMotion.started.IsZero() || !a.window.IsFullScreen() {
			err = fmt.Errorf("reduced-motion fullscreen request was not immediate")
		}
		a.requestPlayerFullscreen(false, true)
	})
	return err
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
	data := nativeTestAVI()
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
			fmt.Fprint(w, `{"code":0,"data":{"video_stream":{"duration":20},"direct_link_qualities":[{"url":"/media.avi","resolution":"test"}]}}`)
		case "/media.avi":
			w.Header().Set("Content-Type", "video/x-msvideo")
			_, _ = w.Write(data)
		case "/api/v1/play/record":
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	onUI := func(fn func()) { done := make(chan struct{}); a.window.Update(func() { fn(); close(done) }); <-done }
	finish := func(err error) {
		onUI(func() {
			a.closePlayer()
			caption := findMyGoCaptionButtons(a.window.NativeHandle())
			parent, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetParent").Call(caption)
			if err == nil && (caption == 0 || parent != a.window.NativeHandle()) {
				err = fmt.Errorf("MyGo caption controls were not restored to the main window")
			}
			_ = os.Chdir(cwd)
			result <- err
			mygo.App.Quit()
		})
	}
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
	var running bool
	onUI(func() {
		running = a.player.Running()
	})
	if !running {
		finish(fmt.Errorf("playback state active but libmpv is not running: status=%s", a.status))
		return
	}
	time.Sleep(800 * time.Millisecond)
	if err := checkNativeVideoFrames(a, onUI, 5*time.Second); err != nil {
		finish(fmt.Errorf("video was not visible before fullscreen: %w", err))
		return
	}
	if err := checkNativePlayerWindowLifecycle(a, onUI); err != nil {
		finish(err)
		return
	}
	if err := checkNativeFullscreen(a, onUI); err != nil {
		finish(err)
		return
	}
	if err := checkNativeStableVideoBounds(a, onUI); err != nil {
		finish(err)
		return
	}
	if err := checkNativeSeekFeedback(a, onUI); err != nil {
		finish(err)
		return
	}
	var focusErr error
	onUI(func() {
		restoreWindowFocus(a.window)
		foregroundBefore, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow").Call()
		a.player.ShowSurface()
		a.hidePlayerOverlay(false)
		a.showPlayerOverlay()
		foreground, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow").Call()
		// Windows may reject activation when another application owns the
		// foreground lock. Automatic controls must preserve either owner.
		if foreground != foregroundBefore {
			focusErr = fmt.Errorf("automatic control display changed foreground focus: before=%x after=%x main=%x controls=%x header=%x", foregroundBefore, foreground, a.window.NativeHandle(), a.playerOverlayWindow.NativeHandle(), a.playerHeaderWindow.NativeHandle())
		}
	})
	if focusErr != nil {
		finish(focusErr)
		return
	}
	if err := checkNativeVideoFrames(a, onUI, 12*time.Second); err != nil {
		finish(err)
		return
	}
	var finalErr error
	onUI(func() {
		if !a.playback.Active || !a.player.Running() || a.playback.Position < 10 {
			finalErr = fmt.Errorf("playback unexpectedly stopped: active=%t running=%t position=%v status=%s", a.playback.Active, a.player.Running(), a.playback.Position, a.status)
			return
		}
		bounds := a.window.ContentBounds()
		scale := windowScale(a.window.NativeHandle())
		user := syscall.NewLazyDLL("user32.dll")
		dc, _, _ := user.NewProc("GetDC").Call(0)
		color, _, _ := syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel").Call(dc, uintptr(float64(bounds.X+bounds.Width/2)*scale), uintptr(float64(bounds.Y+bounds.Height/2)*scale))
		user.NewProc("ReleaseDC").Call(0, dc)
		r, g, b := int(color&255), int((color>>8)&255), int((color>>16)&255)
		if r > 60 || g < 55 || g > 110 || b < 125 || b > 200 {
			finalErr = fmt.Errorf("D3D11 video lost after fullscreen transitions: RGB(%d,%d,%d)", r, g, b)
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
			finalErr = fmt.Errorf("playback controls failed to reappear: alpha=%v visible=%t fullscreenMotion=%+v animationStart=%v", a.playerOverlayOpacity, a.playerOverlayVisible, a.fullscreenMotion, a.playerOverlayAnimationStart)
		}
	})
	finish(finalErr)
}

func checkNativeSeekFeedback(a *appState, onUI func(func())) error {
	for _, target := range []float64{5, 0} {
		var err error
		onUI(func() {
			started := time.Now()
			if target == 5 {
				// Queue several seeks before the engine has produced a new snapshot.
				a.requestPlayerSeek(9)
				a.requestPlayerSeek(2)
			}
			a.requestPlayerSeek(target)
			if time.Since(started) > 150*time.Millisecond || a.seekDisplayPosition() != target || !a.seekFeedback.Pending {
				err = fmt.Errorf("seek did not immediately present target %.1f: position=%.1f pending=%t", target, a.seekDisplayPosition(), a.seekFeedback.Pending)
			}
		})
		if err != nil {
			return err
		}
		deadline := time.Now().Add(6 * time.Second)
		for {
			confirmed := false
			onUI(func() {
				confirmed = !a.seekFeedback.Pending
				if a.seekFeedback.Error != "" {
					err = fmt.Errorf("native seek failed: %s", a.seekFeedback.Error)
				}
				if a.seekFeedback.Pending && a.seekDisplayPosition() != target {
					err = fmt.Errorf("native seek rolled back before confirmation")
				}
			})
			if err != nil {
				return err
			}
			if confirmed {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("libmpv never confirmed seek %.1f", target)
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	return nil
}

func isNativeWindowAbove(user *syscall.LazyDLL, candidate, anchor uintptr) bool {
	getWindow := user.NewProc("GetWindow")
	for hwnd, i := anchor, 0; hwnd != 0 && i < 1000; i++ {
		hwnd, _, _ = getWindow.Call(hwnd, 3) // GW_HWNDPREV: window above in z-order
		if hwnd == candidate {
			return true
		}
	}
	return false
}

func checkNativePlayerWindowLifecycle(a *appState, onUI func(func())) error {
	user := syscall.NewLazyDLL("user32.dll")
	getRect, getClient := user.NewProc("GetWindowRect"), user.NewProc("GetClientRect")
	clientToScreen, setWindowPos := user.NewProc("ClientToScreen"), user.NewProc("SetWindowPos")
	showWindow, isVisible := user.NewProc("ShowWindow"), user.NewProc("IsWindowVisible")
	getLong := user.NewProc("GetWindowLongPtrW")
	readWindowRect := func(hwnd uintptr) [4]int32 {
		var rect [4]int32
		getRect.Call(hwnd, uintptr(unsafe.Pointer(&rect[0])))
		return rect
	}
	clientScreenRect := func(hwnd uintptr) [4]int32 {
		var client [4]int32
		getClient.Call(hwnd, uintptr(unsafe.Pointer(&client[0])))
		var origin playerPoint
		clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin)))
		return [4]int32{origin.x, origin.y, origin.x + client[2], origin.y + client[3]}
	}
	waitForBounds := func() error {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			ready := false
			onUI(func() {
				want, got := clientScreenRect(a.window.NativeHandle()), readWindowRect(a.player.surface)
				ready = want == got
			})
			if ready {
				return nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		var want, got [4]int32
		onUI(func() {
			want, got = clientScreenRect(a.window.NativeHandle()), readWindowRect(a.player.surface)
		})
		return fmt.Errorf("video popup did not follow owner client bounds: want=%v got=%v", want, got)
	}
	main, video := a.window.NativeHandle(), a.player.surface
	if owner, _, _ := user.NewProc("GetParent").Call(video); owner != main {
		return fmt.Errorf("video popup owner mismatch: owner=%x main=%x", owner, main)
	}
	style, _, _ := getLong.Call(video, ^uintptr(19)) // GWL_EXSTYLE (-20)
	if style&0x08000000 == 0 {
		return fmt.Errorf("video popup can activate and steal playback shortcuts: exstyle=%x", style)
	}
	if err := waitForBounds(); err != nil {
		return err
	}
	captionButtons := findMyGoCaptionButtons(main)
	captionParent, _, _ := user.NewProc("GetParent").Call(captionButtons)
	if captionButtons == 0 || captionParent != main {
		return fmt.Errorf("MyGo's native caption controls are not attached to the main window")
	}

	// Move and resize the owner and wait for the popup to match its client area.
	mainRect := readWindowRect(main)
	if ok, _, _ := setWindowPos.Call(main, 0, uintptr(mainRect[0]+16), uintptr(mainRect[1]+12), 0, 0, 0x0001|0x0002|0x0004|0x0010); ok == 0 {
		return fmt.Errorf("could not move the native test window")
	}
	if err := waitForBounds(); err != nil {
		return err
	}
	mainRect = readWindowRect(main)
	if ok, _, _ := setWindowPos.Call(main, 0, uintptr(mainRect[0]), uintptr(mainRect[1]), uintptr(mainRect[2]-mainRect[0]+24), uintptr(mainRect[3]-mainRect[1]+18), 0x0004|0x0010); ok == 0 {
		return fmt.Errorf("could not resize the native test window")
	}
	if err := waitForBounds(); err != nil {
		return err
	}

	// The transport, header and menus are separate owned surfaces above video.
	var zErr error
	onUI(func() {
		restoreWindowFocus(a.window)
		a.showPlayerOverlay()
		if !isNativeWindowAbove(user, a.playerHeaderWindow.NativeHandle(), video) {
			zErr = fmt.Errorf("playback header is behind video")
			return
		}
		if !isNativeWindowAbove(user, a.playerOverlayWindow.NativeHandle(), video) {
			zErr = fmt.Errorf("playback controls are not above video")
			return
		}
		a.openPlayerOverlayMenu("subtitle")
		if a.playerMenuWindow == nil {
			zErr = fmt.Errorf("playback menu is behind video")
		} else {
			if !isNativeWindowAbove(user, a.playerMenuWindow.NativeHandle(), video) {
				zErr = fmt.Errorf("playback menu is behind video")
			}
		}
		a.closePlayerOverlayMenu()
	})
	if zErr != nil {
		return zErr
	}

	// Owned surfaces hide with a minimized owner and are shown again on restore.
	var minimizedVisible, shownWhileMinimized uintptr
	onUI(func() {
		showWindow.Call(main, 6) // SW_MINIMIZE
		minimizedVisible, _, _ = isVisible.Call(video)
		a.player.ShowSurface()
		shownWhileMinimized, _, _ = isVisible.Call(video)
	})
	if minimizedVisible != 0 || shownWhileMinimized != 0 {
		return fmt.Errorf("video popup remained visible while owner was minimized: before-show=%d after-show=%d", minimizedVisible, shownWhileMinimized)
	}
	onUI(func() { showWindow.Call(main, 9) }) // SW_RESTORE
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		visible, _, _ := isVisible.Call(video)
		if visible != 0 {
			return waitForBounds()
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("video popup did not return after restoring owner")
}

func checkNativeVideoFrames(a *appState, onUI func(func()), duration time.Duration) error {
	user := syscall.NewLazyDLL("user32.dll")
	getDC, releaseDC := user.NewProc("GetDC"), user.NewProc("ReleaseDC")
	getPixel := syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")
	deadline := time.Now().Add(duration)
	for frame := 0; time.Now().Before(deadline); frame++ {
		var frameErr error
		onUI(func() {
			bounds := a.window.ContentBounds()
			scale := windowScale(a.window.NativeHandle())
			dc, _, _ := getDC.Call(0)
			defer releaseDC.Call(0, dc)
			for _, offset := range []int{-40, 0, 40} {
				color, _, _ := getPixel.Call(dc, uintptr(float64(bounds.X+bounds.Width/2)*scale), uintptr(float64(bounds.Y+bounds.Height/2+offset)*scale))
				r, g, b := int(color&255), int((color>>8)&255), int((color>>16)&255)
				if absInt(r-24) > 12 || absInt(g-80) > 12 || absInt(b-164) > 12 {
					frameErr = fmt.Errorf("video frame %d flashed at offset %d: RGB(%d,%d,%d)", frame, offset, r, g, b)
					return
				}
			}
		})
		if frameErr != nil {
			return frameErr
		}
		time.Sleep(16 * time.Millisecond)
	}
	return nil
}

// A progress repaint must not send geometry changes to a stationary video HWND.
// Such notifications unnecessarily disturb the active D3D11 swap chain.
func checkNativeStableVideoBounds(a *appState, onUI func(func())) error {
	user := syscall.NewLazyDLL("user32.dll")
	setProc := user.NewProc("SetWindowLongPtrW")
	callProc := user.NewProc("CallWindowProcW")
	var changes atomic.Int32
	var host, original uintptr
	callback := syscall.NewCallback(func(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
		if message == 0x0047 {
			changes.Add(1)
		} // WM_WINDOWPOSCHANGED
		result, _, _ := callProc.Call(original, hwnd, uintptr(message), wparam, lparam)
		return result
	})
	onUI(func() {
		host = a.player.surface
		original, _, _ = setProc.Call(host, ^uintptr(3), callback) // GWLP_WNDPROC (-4)
	})
	if host == 0 || original == 0 {
		return fmt.Errorf("video geometry observer could not attach")
	}
	defer onUI(func() { setProc.Call(host, ^uintptr(3), original) })
	onUI(func() {
		for range 20 {
			fitPlayerSurface(host)
			a.syncPlayerOverlay()
		}
	})
	if count := changes.Load(); count != 0 {
		return fmt.Errorf("stationary video received %d redundant geometry changes", count)
	}
	return nil
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

// A short uncompressed AVI exercises libmpv's actual D3D11 video surface.
// Its blue frame differs from the green parent, so a lost/black GPU surface
// after a layered-window/fullscreen transition cannot pass the pixel check.
func nativeTestAVI() []byte {
	chunk := func(tag string, payload []byte) []byte {
		result := make([]byte, 8+len(payload)+(len(payload)&1))
		copy(result, tag)
		binary.LittleEndian.PutUint32(result[4:], uint32(len(payload)))
		copy(result[8:], payload)
		return result
	}
	words := func(values ...uint32) []byte {
		b := make([]byte, len(values)*4)
		for i, v := range values {
			binary.LittleEndian.PutUint32(b[i*4:], v)
		}
		return b
	}
	const width, height, frameSize, frames = 160, 104, 160 * 104 * 3, 100
	header := chunk("avih", words(200000, frameSize*5, 0, 0, frames, 0, 1, frameSize, width, height, 0, 0, 0, 0))
	stream := append([]byte("vidsDIB "), words(0, 0, 0, 1, 5, 0, frames, frameSize, 0xffffffff, 0)...)
	stream = append(stream, words(0, width|height<<16)...)
	format := words(40, width, height, 1|24<<16, 0, frameSize, 0, 0, 0, 0)
	streamList := append([]byte("strl"), chunk("strh", stream)...)
	streamList = append(streamList, chunk("strf", format)...)
	headers := append([]byte("hdrl"), header...)
	headers = append(headers, chunk("LIST", streamList)...)
	frame := make([]byte, frameSize)
	for i := 0; i < len(frame); i += 3 {
		frame[i], frame[i+1], frame[i+2] = 164, 80, 24
	}
	movie := []byte("movi")
	for range frames {
		movie = append(movie, chunk("00db", frame)...)
	}
	payload := append([]byte("AVI "), chunk("LIST", headers)...)
	payload = append(payload, chunk("LIST", movie)...)
	return chunk("RIFF", payload)
}

func TestUpdatePlayerOverlayShapes(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	user32 := syscall.NewLazyDLL("user32.dll")
	gdi32 := syscall.NewLazyDLL("gdi32.dll")
	procCreateRectRgn := gdi32.NewProc("CreateRectRgn")
	procGetWindowRgn := user32.NewProc("GetWindowRgn")
	procPtInRegion := gdi32.NewProc("PtInRegion")
	procDeleteObject := gdi32.NewProc("DeleteObject")
	procCreateWindowExW := user32.NewProc("CreateWindowExW")
	procDestroyWindow := user32.NewProc("DestroyWindow")
	procDefWindowProcW := user32.NewProc("DefWindowProcW")
	procRegisterClassExW := user32.NewProc("RegisterClassExW")

	className, _ := syscall.UTF16PtrFromString("TestOverlayShapesClass")
	var wc struct {
		CbSize        uint32
		Style         uint32
		LpfnWndProc   uintptr
		CbClsExtra    int32
		CbWndExtra    int32
		HInstance     uintptr
		HIcon         uintptr
		HCursor       uintptr
		HbrBackground uintptr
		LpszMenuName  *uint16
		LpszClassName *uint16
		HIconSm       uintptr
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = procDefWindowProcW.Addr()
	wc.LpszClassName = className
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	parent, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0,
		0x00CF0000, 100, 100, 800, 600, 0, 0, 0, 0,
	)
	defer procDestroyWindow.Call(parent)

	overlay, _, _ := procCreateWindowExW.Call(
		0x00000080|0x08000000, uintptr(unsafe.Pointer(className)), 0,
		0x80000000|0x04000000, 100, 500, 800, 200, parent, 0, 0, 0,
	)
	defer procDestroyWindow.Call(overlay)

	header, _, _ := procCreateWindowExW.Call(
		0x00000080|0x08000000, uintptr(unsafe.Pointer(className)), 0,
		0x80000000|0x04000000, 100, 100, 800, 50, parent, 0, 0, 0,
	)
	defer procDestroyWindow.Call(header)

	updatePlayerOverlayShapes(parent, header, overlay)

	// Verify overlay bottom corners are clipped
	rgn, _, _ := procCreateRectRgn.Call(0, 0, 0, 0)
	defer procDeleteObject.Call(rgn)

	ret, _, _ := procGetWindowRgn.Call(overlay, rgn)
	if ret == 0 {
		t.Fatalf("overlay window should have a region set")
	}

	// Bottom-left corner (0, 199) and bottom-right corner (799, 199) must be clipped (PtInRegion == 0)
	inBL, _, _ := procPtInRegion.Call(rgn, 0, 199)
	inBR, _, _ := procPtInRegion.Call(rgn, 799, 199)
	inCenter, _, _ := procPtInRegion.Call(rgn, 400, 199)
	if inBL != 0 || inBR != 0 {
		t.Errorf("overlay bottom corners should be clipped: BL=%d BR=%d", inBL, inBR)
	}
	if inCenter == 0 {
		t.Errorf("overlay bottom center should be within region: %d", inCenter)
	}

	// Verify header top corners are clipped
	retHd, _, _ := procGetWindowRgn.Call(header, rgn)
	if retHd == 0 {
		t.Fatalf("header window should have a region set")
	}
	inTL, _, _ := procPtInRegion.Call(rgn, 0, 0)
	inTR, _, _ := procPtInRegion.Call(rgn, 799, 0)
	inHdCenter, _, _ := procPtInRegion.Call(rgn, 400, 0)
	if inTL != 0 || inTR != 0 {
		t.Errorf("header top corners should be clipped: TL=%d TR=%d", inTL, inTR)
	}
	if inHdCenter == 0 {
		t.Errorf("header top center should be within region: %d", inHdCenter)
	}

	// When parent is maximized (or in fullscreen), regions should be cleared.
	procShowWindow := user32.NewProc("ShowWindow")
	procShowWindow.Call(parent, 3) // SW_MAXIMIZE
	updatePlayerOverlayShapes(parent, header, overlay)

	retMaxOverlay, _, _ := procGetWindowRgn.Call(overlay, rgn)
	if retMaxOverlay != 0 {
		t.Errorf("overlay should not have region in maximized/fullscreen mode")
	}
	retMaxHeader, _, _ := procGetWindowRgn.Call(header, rgn)
	if retMaxHeader != 0 {
		t.Errorf("header should not have region in maximized/fullscreen mode")
	}
}
