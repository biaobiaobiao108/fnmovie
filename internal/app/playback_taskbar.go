package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
)

func playbackTaskbarValue(playback PlaybackState) (float64, bool) {
	if !playback.Active || playback.Duration <= 0 || math.IsNaN(playback.Position) || math.IsInf(playback.Position, 0) || math.IsNaN(playback.Duration) || math.IsInf(playback.Duration, 0) {
		return 0, false
	}
	value := playback.Position / playback.Duration
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return max(0, min(1, value)), true
}

func (a *appState) syncPlaybackTaskbarProgress() {
	if a.window == nil {
		return
	}
	value, active := playbackTaskbarValue(a.playback)
	if !active {
		a.clearPlaybackTaskbarProgress()
		return
	}
	now := time.Now()
	delta := math.Abs(value - a.taskbarProgress)
	if a.taskbarProgressSet && (delta < 0.000001 || delta < 0.005 && now.Sub(a.taskbarProgressUpdatedAt) < time.Second) {
		return
	}
	a.window.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressNormal, Value: value})
	a.taskbarProgress = value
	a.taskbarProgressUpdatedAt = now
	a.taskbarProgressSet = true
}

func (a *appState) clearPlaybackTaskbarProgress() {
	if a.taskbarProgressSet && a.window != nil {
		a.window.SetProgressBar(mygo.ProgressBar{})
	}
	a.taskbarProgress = 0
	a.taskbarProgressUpdatedAt = time.Time{}
	a.taskbarProgressSet = false
}
