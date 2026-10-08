package app

import (
	"math"
	"time"
)

// seekFeedback belongs to the UI thread. Pending targets are presentation only;
// watch history continues to use actual player snapshots.
type seekFeedback struct {
	Pending                     bool
	Target                      float64
	RequestID                   uint64
	RequestedAt, LastObservedAt time.Time
	Error                       string
}

func (s *seekFeedback) begin(target float64, request uint64, now time.Time) {
	s.Pending, s.Target, s.RequestID, s.RequestedAt, s.Error = true, target, request, now, ""
}

func (s *seekFeedback) position(snapshot PlayerSnapshot, previous float64) float64 {
	if snapshot.ObservedAt.Before(s.RequestedAt) || snapshot.ObservedAt.Before(s.LastObservedAt) || snapshot.SeekRequestID < s.RequestID {
		if s.Pending {
			return s.Target
		}
		return previous
	}
	s.LastObservedAt = snapshot.ObservedAt
	if s.Pending {
		if snapshot.SeekError != "" || snapshot.Error != "" {
			s.Pending = false
			s.Error = "跳转失败，请重试"
		} else if s.confirmed(snapshot) {
			s.Pending = false
		} else {
			return s.Target
		}
	}
	return snapshot.Position
}

func (s *seekFeedback) confirmed(snapshot PlayerSnapshot) bool {
	if snapshot.SeekAcknowledgedAt.IsZero() || snapshot.ObservedAt.Before(snapshot.SeekAcknowledgedAt) || snapshot.Seeking || snapshot.Buffering {
		return false
	}
	// A busy UI can receive its first settled sample after playback has already
	// advanced. Allow that forward progress, while rejecting positions behind
	// the target and never advancing the presentation during remote buffering.
	elapsed := snapshot.ObservedAt.Sub(snapshot.SeekAcknowledgedAt).Seconds()
	forward := math.Max(2, math.Max(0, snapshot.Speed)*(elapsed+2))
	return snapshot.Position >= s.Target-2 && snapshot.Position <= s.Target+forward
}

func (a *appState) requestPlayerSeek(target float64) {
	if a.player == nil {
		return
	}
	target = maxFloat(0, target)
	if a.playback.Duration > 0 {
		target = minFloat(target, a.playback.Duration)
	}
	now := time.Now()
	request, err := a.player.RequestSeekTo(target)
	a.seekFeedback.begin(target, request, now)
	if err != nil {
		a.seekFeedback.Pending = false
		a.seekFeedback.Error = "跳转失败，请重试"
		return
	}
	a.playback.Position, a.seekSliderPosition = target, target
	a.markPlayerOverlayActivity()
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Invalidate()
	}
}

func (a *appState) requestPlayerSeekRelative(delta float64) {
	a.requestPlayerSeek(a.seekDisplayPosition() + delta)
}
