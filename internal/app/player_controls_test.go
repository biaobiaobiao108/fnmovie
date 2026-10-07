package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

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
