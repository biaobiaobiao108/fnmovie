package app

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestPlayerMenusKeepUniformBoundsAndTruncateLongTrackNames(t *testing.T) {
	for _, option := range []string{"subtitle", "audio", "speed"} {
		for _, count := range []int{1, 30} {
			a := &appState{player: NewPlayer(), playback: PlaybackState{Speed: 1}, playerOverlayMenu: option}
			for i := range count {
				a.playerMenuTracks = append(a.playerMenuTracks, PlayerTrack{ID: i + 1, Title: strings.Repeat("很长的轨道名称", 30)})
			}
			tester := ui.NewTester(a.playerMenuView, playerMenuWidth, playerMenuHeight)
			panel, ok := tester.Find("播放选项菜单")
			if !ok || panel.W != playerMenuWidth || panel.H != playerMenuHeight {
				t.Fatalf("%s/%d menu expanded with track content: %+v", option, count, panel)
			}
			if option != "speed" {
				track, ok := tester.Find(playerTrackLabel(a.playerMenuTracks[0]))
				if !ok || track.H <= 0 || track.Y >= playerMenuHeight {
					t.Fatalf("first %s track must remain visible: %+v", option, track)
				}
			}
			if a.playerOverlayContentHeight() != playerOverlayControlHeight {
				t.Fatal("menu resized the transport window")
			}
		}
	}
}

func TestPlayerTransportRemainsCentered(t *testing.T) {
	for _, width := range []int{960, 1280, 1920} {
		for _, speed := range []float64{0.25, 1, 4} {
			t.Run(fmt.Sprintf("width_%d_speed_%g", width, speed), func(t *testing.T) {
				a := &appState{player: NewPlayer(), playback: PlaybackState{Duration: 7200, Volume: 80, Speed: speed}}
				tester := ui.NewTester(a.playerTransport, width, playerOverlayControlHeight)
				area, ok := tester.Find("播放控件区域")
				fade, fadeOK := tester.Find("播放沉浸渐隐")
				expectedGutter := float32(max(16, (width-1760)/2))
				if !ok || area.X != expectedGutter || area.X+area.W > float32(width)-expectedGutter+0.5 ||
					area.Y < 0 || area.Y+area.H > playerOverlayControlHeight ||
					!fadeOK || fade.W != float32(width) || fade.H != playerOverlayControlHeight {
					t.Fatalf("transparent transport or its fade is clipped: area=%+v fade=%+v", area, fade)
				}
				axis, axisOK := tester.Find("播放时间轴")
				controls, controlsOK := tester.Find("播放控制栏")
				if !axisOK || !controlsOK || axis.X != area.X || axis.Y != area.Y || axis.Y+axis.H >= controls.Y || controls.Y+controls.H > area.Y+area.H {
					t.Fatalf("timeline and controls should fit the unboxed transport area: axis=%+v controls=%+v area=%+v", axis, controls, area)
				}
				play, ok := tester.Find("播放或暂停")
				if !ok {
					t.Fatal("play button missing")
				}
				if math.Abs(float64(play.X+play.W/2)-float64(width)/2) > 0.5 {
					t.Fatalf("play center %.2f != window center %.2f", play.X+play.W/2, float64(width)/2)
				}
				progress, progressOK := tester.Find("播放进度")
				if !progressOK || math.Abs(float64(progress.X+progress.W/2)-float64(width)/2) > 0.5 {
					t.Fatalf("progress rail is not centered: %+v (found=%v)", progress, progressOK)
				}
				operations, operationsOK := tester.Find("影片操作")
				if !operationsOK || operations.X <= play.X+play.W || operations.X+operations.W > area.X+area.W+0.5 {
					t.Fatalf("movie options should stay beside the centered transport: play=%+v options=%+v area=%+v", play, operations, area)
				}
				if _, ok := tester.Find("音量"); !ok {
					t.Fatal("volume slider is missing its accessible label")
				}
				back, backOK := tester.Find("快退 10 秒")
				forward, forwardOK := tester.Find("快进 10 秒")
				if !backOK || !forwardOK || math.Abs(float64(back.X+back.W/2+forward.X+forward.W/2)-float64(width)) > 0.5 {
					t.Fatalf("seek controls not symmetric: back=%+v forward=%+v", back, forward)
				}
				mute, muteOK := tester.Find("静音")
				volume, volumeOK := tester.Find("音量")
				speed, speedOK := tester.Find("播放速度")
				subtitles, subtitlesOK := tester.Find("字幕")
				audio, audioOK := tester.Find("音轨")
				fullscreen, fullscreenOK := tester.Find("切换全屏")
				if !muteOK || !volumeOK || !speedOK || !subtitlesOK || !audioOK || !fullscreenOK ||
					mute.X <= forward.X+forward.W || volume.X <= mute.X+mute.W || speed.X <= volume.X+volume.W ||
					subtitles.X <= speed.X+speed.W || audio.X <= subtitles.X+subtitles.W ||
					fullscreen.X <= audio.X+audio.W || fullscreen.X+fullscreen.W > area.X+area.W+0.5 {
					t.Fatalf("right-side movie controls overlap or overflow: mute=%+v volume=%+v speed=%+v subtitles=%+v audio=%+v fullscreen=%+v area=%+v", mute, volume, speed, subtitles, audio, fullscreen, area)
				}
			})
		}
	}
}

func TestPlayerHeaderShowsTitleAndKeepsPlaybackDetailsOutOfTransport(t *testing.T) {
	const title = "春日影"
	const quality = "4K HDR"
	a := &appState{playback: PlaybackState{Title: title, Quality: quality}}
	header := ui.NewTester(a.playerHeaderView, 1280, playerOverlayHeaderHeight)
	back, hasBack := header.Find("返回影片库")
	titleBounds, hasTitle := header.Find(title)
	if !hasBack || !hasTitle || back.Y < 0 || back.Y+back.H > playerOverlayHeaderHeight ||
		titleBounds.X < back.X+back.W || titleBounds.Y < 0 || titleBounds.Y+titleBounds.H > playerOverlayHeaderHeight ||
		header.HasText(quality) {
		t.Fatalf("header should show the title after back navigation and omit quality: back=%+v title=%+v texts=%v", back, titleBounds, header.Texts())
	}
	transport := ui.NewTester(a.playerTransport, 1280, playerOverlayControlHeight)
	play, hasPlay := transport.Find("播放或暂停")
	if !hasPlay || transport.HasText(title) || transport.HasText(quality) {
		t.Fatalf("bottom transport should leave the title in the top bar and omit quality: play=%+v texts=%v", play, transport.Texts())
	}
}

func TestPlayerOverlayThemeUsesMutedMovieAccent(t *testing.T) {
	theme := playerOverlayTheme()
	want := movieColors.accent.Mix(ui.RGB(255, 255, 255), 0.38)
	if theme.Accent != want || theme.Background != ui.Transparent || theme.Surface != ui.Transparent {
		t.Fatalf("player overlay theme should use the movie accent on a transparent dark surface: %+v", theme)
	}
	if theme.TextMuted.A >= 255 {
		t.Fatalf("secondary player text should be subdued, got alpha %d", theme.TextMuted.A)
	}
}

func TestPlayerSeekFeedbackIsCenteredAndVisible(t *testing.T) {
	bounds := playerSeekFeedbackBounds(mygo.Rectangle{X: 40, Y: 60, Width: 1280, Height: 800})
	if bounds.X+bounds.Width/2 != 40+1280/2 || bounds.Y+bounds.Height/2 != 60+800/2 {
		t.Fatalf("seek feedback bounds are not centered: %+v", bounds)
	}
	a := &appState{seekFeedback: seekFeedback{Pending: true}}
	tester := ui.NewTester(a.playerSeekFeedbackView, playerSeekFeedbackWidth, playerSeekFeedbackHeight)
	tester.Frame()
	spinner, hasSpinner := tester.Find("正在跳转")
	if !hasSpinner || spinner.W <= 0 || spinner.H <= 0 || tester.HasText("跳转中") || tester.HasText("跳转失败") {
		t.Fatalf("seek feedback should contain only its centered spinner: spinner=%+v texts=%v", spinner, tester.Texts())
	}
}

func TestPlayerSeekFailureRemainsVisibleAndShowsMessage(t *testing.T) {
	feedback := seekFeedback{Error: "跳转失败，请重试"}
	if !playerSeekFeedbackShouldShow(feedback, false) || playerSeekFeedbackShouldShow(feedback, true) {
		t.Fatal("seek error should remain visible unless the window is minimized")
	}
	a := &appState{seekFeedback: feedback}
	tester := ui.NewTester(a.playerSeekFeedbackView, playerSeekFeedbackWidth, playerSeekFeedbackHeight)
	tester.Frame()
	if !tester.HasText(feedback.Error) || tester.HasText("正在跳转") {
		t.Fatalf("seek error message is not shown: texts=%v", tester.Texts())
	}
}

func TestVolumeDragReleaseResetsOverlayIdleTimer(t *testing.T) {
	previous := time.Now().Add(-time.Hour)
	a := &appState{volumeDragging: true, playerOverlayLastInput: previous}
	a.updatePlayerVolumeDragging(false)
	if a.volumeDragging {
		t.Fatal("volume drag should end on release")
	}
	if !a.playerOverlayLastInput.After(previous) {
		t.Fatal("volume drag release did not refresh overlay idle timer")
	}
}

func TestPlayerShortcutMapping(t *testing.T) {
	tests := []struct {
		name       string
		mods       ui.Modifiers
		key        ui.Key
		fullscreen bool
		want       playerAction
	}{
		{name: "space toggles playback", key: ui.KeySpace, want: playerTogglePause},
		{name: "left seeks back", key: ui.KeyLeft, want: playerSeekBack},
		{name: "right seeks forward", key: ui.KeyRight, want: playerSeekForward},
		{name: "up raises volume", key: ui.KeyUp, want: playerVolumeUp},
		{name: "down lowers volume", key: ui.KeyDown, want: playerVolumeDown},
		{name: "m toggles mute", key: ui.KeyM, want: playerToggleMute},
		{name: "f toggles full screen", key: ui.KeyF, want: playerToggleFull},
		{name: "escape exits full screen first", key: ui.KeyEscape, fullscreen: true, want: playerExitFull},
		{name: "escape returns to library", key: ui.KeyEscape, want: playerReturn},
		{name: "modifier shortcuts are ignored", mods: ui.Ctrl, key: ui.KeySpace, want: playerShortcutIgnore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := playerShortcutAction(tt.mods, tt.key, tt.fullscreen); got != tt.want {
				t.Fatalf("playerShortcutAction() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlayerTrackSelectionProperty(t *testing.T) {
	tests := []struct {
		kind         string
		id           int
		wantProperty string
		wantValue    any
	}{
		{kind: "audio", id: 2, wantProperty: "aid", wantValue: 2},
		{kind: "subtitle", id: 3, wantProperty: "sid", wantValue: 3},
		{kind: "subtitle", id: 0, wantProperty: "sid", wantValue: "no"},
	}
	for _, tt := range tests {
		property, value := playerTrackSelection(tt.kind, tt.id)
		if property != tt.wantProperty || value != tt.wantValue {
			t.Errorf("selection(%q, %d) = (%q, %#v), want (%q, %#v)", tt.kind, tt.id, property, value, tt.wantProperty, tt.wantValue)
		}
	}
}

func TestPlayerTrackLabelContainsStableLanguageTitleAndID(t *testing.T) {
	track := PlayerTrack{ID: 4, Language: "chi", Title: "AAC 5.1", External: true}
	if got, want := playerTrackLabel(track), "chi · AAC 5.1 · 轨道 4 · 外挂"; got != want {
		t.Fatalf("track label = %q, want %q", got, want)
	}
}

func TestNormalizeSelectedTracksKeepsOneSubtitleHighlight(t *testing.T) {
	tracks := []PlayerTrack{
		{ID: 2, Selected: true, MainSelection: 1, HasMainSelection: true},
		{ID: 3, Selected: true, MainSelection: 0, HasMainSelection: true},
		{ID: 4, Selected: true, MainSelection: 1, HasMainSelection: true},
	}

	normalizeSelectedTracks(tracks, true)

	for i, track := range tracks {
		if got, want := track.Selected, i == 1; got != want {
			t.Errorf("track %d selected = %v, want %v", track.ID, got, want)
		}
	}
}

func TestNormalizeSelectedTracksFallsBackToOneSelectedTrack(t *testing.T) {
	tracks := []PlayerTrack{{ID: 1, Selected: true}, {ID: 2, Selected: true}}

	normalizeSelectedTracks(tracks, true)

	if !tracks[0].Selected || tracks[1].Selected {
		t.Fatalf("fallback selection = [%v, %v], want only first track selected", tracks[0].Selected, tracks[1].Selected)
	}
}

func TestPlayerDoubleClickCallback(t *testing.T) {
	player := NewPlayer()
	called := false
	player.SetOnDoubleClick(func() {
		called = true
	})
	if player.onDoubleClick == nil {
		t.Fatal("onDoubleClick should be set on player")
	}
	player.onDoubleClick()
	if !called {
		t.Fatal("onDoubleClick was not invoked")
	}
}
