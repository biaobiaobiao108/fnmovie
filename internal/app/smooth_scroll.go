package app

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
	// Binding measurements detect native thumb/key movement and viewport changes.
	bound                   bool
	boundWidth, boundHeight float32
}

// Stop halts ongoing animation immediately and locks to position.
func (s *smoothScroll) Stop(position float32) {
	s.Position = position
	s.Target = position
	s.Velocity = 0
	s.Active = false
}

func (s *smoothScroll) Add(position, delta, maxY float32, now time.Time) {
	if !s.Active {
		s.Position = position
		s.Target = position
		s.Velocity = 0
		s.Last = now
	} else if math.Abs(float64(s.Position-position)) > 4.0 {
		// External scroll jump (e.g. thumb drag or resize) - resync position
		s.Position = position
	}
	s.Target = max(0, min(maxY, s.Target+delta))
	// Add smooth velocity kick in the direction of the delta to eliminate initial lag
	kick := float64(delta) * 7.0
	// If reversing direction, prioritize the new direction over opposing momentum
	if (delta > 0 && s.Velocity < 0) || (delta < 0 && s.Velocity > 0) {
		s.Velocity = kick
	} else {
		s.Velocity += kick
	}
	const maxVel = 2600.0
	if s.Velocity > maxVel {
		s.Velocity = maxVel
	} else if s.Velocity < -maxVel {
		s.Velocity = -maxVel
	}
	s.Active = math.Abs(float64(s.Target-s.Position)) > 0.1 || math.Abs(s.Velocity) > 0.1
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
	dt = min(dt, 0.05)
	const omega = 22.0
	x := float64(s.Position - s.Target)
	e := math.Exp(-omega * dt)
	term := s.Velocity + omega*x
	nextX := (x + term*dt) * e
	s.Velocity = (s.Velocity - omega*term*dt) * e
	s.Position = s.Target + float32(nextX)
	if math.Abs(nextX) < 0.25 && math.Abs(s.Velocity) < 6 {
		s.Position = s.Target
		s.Velocity = 0
		s.Active = false
	}
	return s.Position, s.Active
}
