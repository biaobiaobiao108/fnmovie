package main

import (
	"math"
	"time"
)

// smoothScroll integrates wheel input into a critically damped spring. Unlike
// restarting a timed tween for every wheel message, new input changes the
// destination while preserving the current velocity.
type smoothScroll struct {
	Position float32
	Target   float32
	Velocity float64
	Last     time.Time
	Active   bool
}

func (s *smoothScroll) Add(position, delta, maxY float32, now time.Time) {
	if !s.Active {
		s.Position = position
		s.Target = position
		s.Velocity = 0
		s.Last = now
	}
	s.Target = max(0, min(maxY, s.Target+delta))
	s.Active = s.Target != s.Position || math.Abs(s.Velocity) > 0.1
	if !s.Active {
		s.Velocity = 0
	}
}

func (s *smoothScroll) Advance(now time.Time) (float32, bool) {
	if !s.Active {
		return s.Position, false
	}
	dt := now.Sub(s.Last).Seconds()
	s.Last = now
	if dt <= 0 {
		return s.Position, true
	}
	// A long suspend or debugger pause should not launch a huge scroll step.
	dt = min(dt, 0.1)
	const omega = 20.0
	x := float64(s.Position - s.Target)
	e := math.Exp(-omega * dt)
	term := s.Velocity + omega*x
	nextX := (x + term*dt) * e
	s.Velocity = (s.Velocity - omega*term*dt) * e
	s.Position = s.Target + float32(nextX)
	if math.Abs(nextX) < 0.2 && math.Abs(s.Velocity) < 3 {
		s.Position = s.Target
		s.Velocity = 0
		s.Active = false
	}
	return s.Position, s.Active
}
