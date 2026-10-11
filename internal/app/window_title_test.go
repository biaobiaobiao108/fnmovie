package app

import (
	"strings"
	"testing"
)

func TestWindowTitleForPlayback(t *testing.T) {
	longTitle := strings.Repeat("超长影片标题", 20)
	tests := []struct {
		name     string
		playback PlaybackState
		want     string
	}{
		{name: "idle", want: applicationWindowTitle},
		{name: "empty title", playback: PlaybackState{Active: true}, want: applicationWindowTitle},
		{name: "playback title", playback: PlaybackState{Active: true, Title: "  正在播放\n影片标题  "}, want: "正在播放 影片标题"},
		{name: "long title is passed to native caption", playback: PlaybackState{Active: true, Title: longTitle}, want: longTitle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowTitleForPlayback(tt.playback); got != tt.want {
				t.Fatalf("windowTitleForPlayback() = %q, want %q", got, tt.want)
			}
		})
	}
}
