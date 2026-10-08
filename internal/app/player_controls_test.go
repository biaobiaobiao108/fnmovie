package app

import (
	"fmt"
	"math"
	"strings"
	"testing"

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
				panel, ok := tester.Find("播放控制栏")
				if !ok || panel.Y < 6 || panel.Y+panel.H > playerOverlayControlHeight-6 {
					t.Fatalf("transport border clipped by its window: %+v", panel)
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
				back, backOK := tester.Find("快退 10 秒")
				forward, forwardOK := tester.Find("快进 10 秒")
				if !backOK || !forwardOK || math.Abs(float64(back.X+back.W/2+forward.X+forward.W/2)-float64(width)) > 0.5 {
					t.Fatalf("seek controls not symmetric: back=%+v forward=%+v", back, forward)
				}
				mute, muteOK := tester.Find("静音")
				fullscreen, fullscreenOK := tester.Find("切换全屏")
				if !muteOK || !fullscreenOK || mute.X+mute.W >= back.X || fullscreen.X <= forward.X+forward.W || fullscreen.X+fullscreen.W > float32(width-32) {
					t.Fatalf("side controls overlap or overflow: mute=%+v fullscreen=%+v", mute, fullscreen)
				}
			})
		}
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
	status, hasStatus := tester.Find("跳转中")
	if !hasSpinner || !hasStatus || spinner.W <= 0 || spinner.H <= 0 {
		t.Fatalf("center seek feedback is missing its animated status: spinner=%+v status=%+v", spinner, status)
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
