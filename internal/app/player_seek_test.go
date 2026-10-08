package app

import (
	"testing"
	"time"
)

func TestSeekFeedbackRejectsStaleSnapshots(t *testing.T) {
	now := time.Unix(1000, 0)
	s := seekFeedback{}
	s.begin(120, 7, now)
	for _, snapshot := range []PlayerSnapshot{
		{ObservedAt: now.Add(-time.Second), SeekRequestID: 7, Position: 20},
		{ObservedAt: now.Add(time.Second), SeekRequestID: 6, Position: 30},
	} {
		if got := s.position(snapshot, 20); got != 120 || !s.Pending {
			t.Fatalf("stale snapshot moved pending target: got %v, state %+v", got, s)
		}
	}
	s.position(PlayerSnapshot{ObservedAt: now.Add(3 * time.Second), SeekRequestID: 7, Position: 120, SeekAcknowledgedAt: now.Add(2 * time.Second)}, 120)
	if s.Pending {
		t.Fatal("confirmed seek remains pending")
	}
	for _, snapshot := range []PlayerSnapshot{
		{ObservedAt: now.Add(2 * time.Second), SeekRequestID: 7, Position: 20},
		{ObservedAt: now.Add(4 * time.Second), SeekRequestID: 6, Position: 30},
	} {
		if got := s.position(snapshot, 121); got != 121 {
			t.Fatalf("stale snapshot rolled back confirmed position: %v", got)
		}
	}
}

func TestSeekFeedbackRetainsTargetDuringRemoteDelay(t *testing.T) {
	now := time.Unix(1000, 0)
	s := seekFeedback{}
	s.begin(300, 1, now)
	for _, elapsed := range []time.Duration{time.Second, 31 * time.Second, 3 * time.Minute} {
		snapshot := PlayerSnapshot{ObservedAt: now.Add(elapsed), SeekRequestID: 1, Position: 10}
		if got := s.position(snapshot, 10); got != 300 || !s.Pending {
			t.Fatalf("remote delay %v cleared presentation target: %v %+v", elapsed, got, s)
		}
	}
}

func TestSeekFeedbackRequiresAcknowledgementAndSettledObservation(t *testing.T) {
	now := time.Unix(1000, 0)
	tests := []struct {
		name     string
		snapshot PlayerSnapshot
	}{
		{"no acknowledgement", PlayerSnapshot{ObservedAt: now.Add(time.Second), SeekRequestID: 2, Position: 100}},
		{"observation before acknowledgement", PlayerSnapshot{ObservedAt: now.Add(time.Second), SeekRequestID: 2, SeekAcknowledgedAt: now.Add(2 * time.Second), Position: 100}},
		{"seeking", PlayerSnapshot{ObservedAt: now.Add(2 * time.Second), SeekRequestID: 2, SeekAcknowledgedAt: now.Add(time.Second), Seeking: true, Position: 100}},
		{"buffering", PlayerSnapshot{ObservedAt: now.Add(2 * time.Second), SeekRequestID: 2, SeekAcknowledgedAt: now.Add(time.Second), Buffering: true, Position: 100}},
		{"distant old position", PlayerSnapshot{ObservedAt: now.Add(2 * time.Second), SeekRequestID: 2, SeekAcknowledgedAt: now.Add(time.Second), Position: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := seekFeedback{}
			s.begin(100, 2, now)
			if got := s.position(tt.snapshot, 10); got != 100 || !s.Pending {
				t.Fatalf("unsettled snapshot confirmed seek: %v %+v", got, s)
			}
			settled := PlayerSnapshot{ObservedAt: now.Add(3 * time.Second), SeekRequestID: 2, SeekAcknowledgedAt: now.Add(time.Second), Position: 101, Speed: 1}
			if got := s.position(settled, 100); got != 101 || s.Pending {
				t.Fatalf("settled snapshot did not restore actual progress: %v %+v", got, s)
			}
		})
	}
}

func TestSeekFeedbackLatestRequestWins(t *testing.T) {
	now := time.Unix(1000, 0)
	s := seekFeedback{}
	s.begin(100, 1, now)
	s.begin(200, 2, now.Add(time.Second))
	s.begin(50, 3, now.Add(2*time.Second))
	for _, old := range []PlayerSnapshot{
		{ObservedAt: now.Add(3 * time.Second), SeekRequestID: 1, SeekAcknowledgedAt: now.Add(time.Second), Position: 100},
		{ObservedAt: now.Add(4 * time.Second), SeekRequestID: 2, SeekAcknowledgedAt: now.Add(2 * time.Second), Position: 200, SeekError: "old command failed"},
	} {
		if got := s.position(old, 200); got != 50 || !s.Pending || s.Error != "" {
			t.Fatalf("old request overrode latest seek: %v %+v", got, s)
		}
	}
	if got := s.position(PlayerSnapshot{ObservedAt: now.Add(5 * time.Second), SeekRequestID: 3, SeekAcknowledgedAt: now.Add(4 * time.Second), Position: 50}, 200); got != 50 || s.Pending {
		t.Fatalf("latest request failed to confirm: %v %+v", got, s)
	}
}

func TestSeekFeedbackErrorRestoresActualPosition(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, snapshot := range []PlayerSnapshot{
		{ObservedAt: now.Add(time.Second), SeekRequestID: 5, Position: 12, SeekError: "seek command failed"},
		{ObservedAt: now.Add(time.Second), SeekRequestID: 5, Position: 12, Error: "playback failed"},
	} {
		s := seekFeedback{}
		s.begin(90, 5, now)
		if got := s.position(snapshot, 90); got != 12 || s.Pending || s.Error == "" {
			t.Fatalf("error did not restore actual progress: %v %+v", got, s)
		}
		s.begin(80, 6, now.Add(2*time.Second))
		if s.Error != "" || !s.Pending {
			t.Fatalf("retry did not clear error: %+v", s)
		}
	}
}

func TestSeekFeedbackConfirmationAccountsForSpeed(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, tt := range []struct {
		speed, position float64
		pending         bool
	}{
		{0.5, 102, false}, {1, 103.1, true}, {2, 106, false}, {2, 106.1, true}, {4, 112, false},
	} {
		s := seekFeedback{}
		s.begin(100, 1, now)
		snapshot := PlayerSnapshot{ObservedAt: now.Add(time.Second), SeekRequestID: 1, SeekAcknowledgedAt: now, Position: tt.position, Speed: tt.speed}
		got := s.position(snapshot, 100)
		want := tt.position
		if tt.pending {
			want = 100
		}
		if got != want || s.Pending != tt.pending {
			t.Fatalf("speed %v position %v: got %v pending %v, want %v pending %v", tt.speed, tt.position, got, s.Pending, want, tt.pending)
		}
	}
}

func TestSeekFeedbackConfirmsAfterDelayedUIObservation(t *testing.T) {
	now := time.Unix(1000, 0)
	s := seekFeedback{}
	s.begin(100, 1, now)
	snapshot := PlayerSnapshot{ObservedAt: now.Add(15 * time.Second), SeekRequestID: 1, SeekAcknowledgedAt: now.Add(time.Second), Position: 114, Speed: 1}
	if got := s.position(snapshot, 100); got != 114 || s.Pending {
		t.Fatalf("UI delay stranded a completed seek: %v %+v", got, s)
	}
	s.begin(200, 2, now)
	snapshot.SeekRequestID, snapshot.Position = 2, 100
	if got := s.position(snapshot, 100); got != 200 || !s.Pending {
		t.Fatalf("an old position behind the target confirmed a delayed seek: %v %+v", got, s)
	}
}

func TestRequestPlayerSeekClampsProgress(t *testing.T) {
	for _, tt := range []struct{ duration, target, want float64 }{
		{100, -50, 0}, {100, 150, 100}, {100, 40, 40}, {0, 150, 150}, {0, -50, 0},
	} {
		a := &appState{player: NewPlayer(), playback: PlaybackState{Duration: tt.duration, Position: 10}}
		a.requestPlayerSeek(tt.target)
		if a.seekFeedback.Target != tt.want || !a.seekFeedback.Pending || a.seekSliderPosition != tt.want || a.playback.Position != tt.want {
			t.Fatalf("duration %v target %v: feedback %+v slider %v playback %v, want %v", tt.duration, tt.target, a.seekFeedback, a.seekSliderPosition, a.playback.Position, tt.want)
		}
		if snapshot := a.player.Snapshot(); snapshot.Position != tt.want {
			t.Fatalf("player target %v, want %v", snapshot.Position, tt.want)
		}
	}
}

func TestRequestPlayerSeekRelativeUsesPendingTarget(t *testing.T) {
	a := &appState{player: NewPlayer(), playback: PlaybackState{Duration: 500, Position: 100}}
	a.requestPlayerSeek(200)
	// Actual playback can still be at the previous position while remote demuxing.
	a.playback.Position = 100
	a.requestPlayerSeekRelative(10)
	if a.seekFeedback.Target != 210 || a.seekSliderPosition != 210 {
		t.Fatalf("relative seek used stale actual position: %+v slider %v", a.seekFeedback, a.seekSliderPosition)
	}
}
