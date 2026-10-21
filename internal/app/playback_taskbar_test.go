package app

import "testing"

func TestPlaybackTaskbarValue(t *testing.T) {
	tests := []struct {
		name     string
		playback PlaybackState
		want     float64
		ok       bool
	}{
		{name: "inactive", playback: PlaybackState{Duration: 100}, ok: false},
		{name: "unknown duration", playback: PlaybackState{Active: true}, ok: false},
		{name: "current position", playback: PlaybackState{Active: true, Position: 25, Duration: 100}, want: 0.25, ok: true},
		{name: "before start", playback: PlaybackState{Active: true, Position: -10, Duration: 100}, want: 0, ok: true},
		{name: "after end", playback: PlaybackState{Active: true, Position: 120, Duration: 100}, want: 1, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := playbackTaskbarValue(tt.playback)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("playbackTaskbarValue(%+v) = %v, %v; want %v, %v", tt.playback, got, ok, tt.want, tt.ok)
			}
		})
	}
}
