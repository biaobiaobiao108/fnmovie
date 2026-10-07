package main

import (
	"math"
	"time"
)

const (
	catalogScrollDuration = 175 * time.Millisecond
)

type smoothScroll struct {
	From     float32
	To       float32
	Started  time.Time
	Duration time.Duration
	Active   bool
}

func (s *smoothScroll) Retarget(current, target float32, now time.Time, duration time.Duration) {
	s.From, s.To, s.Started, s.Duration, s.Active = current, target, now, duration, duration > 0 && current != target
}

func (s *smoothScroll) Position(now time.Time) (float32, bool) {
	if !s.Active {
		return s.To, false
	}
	if s.Duration <= 0 {
		s.Active = false
		return s.To, false
	}
	progress := float64(now.Sub(s.Started)) / float64(s.Duration)
	if progress >= 1 {
		s.Active = false
		return s.To, false
	}
	progress = max(0, min(progress, 1))
	eased := 1 - math.Pow(1-progress, 3)
	return s.From + (s.To-s.From)*float32(eased), true
}
