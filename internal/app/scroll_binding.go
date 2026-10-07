package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// advanceSmoothScroll runs before constructing the view, so its position is
// current when TrackScroll adopts it. Virtual lists may build missing rows in
// their layout pass. Each container owns independent persistent states. Native
// thumb/key movement and changed viewport geometry cancel old momentum.
func advanceSmoothScroll(c *ui.Context, state *ui.ScrollState, motion *smoothScroll) {
	width, height := c.Size()
	if motion.Active && (math.Abs(float64(state.Y-motion.Position)) > 0.01 ||
		motion.bound && (width != motion.boundWidth || height != motion.boundHeight) ||
		c.Preferences().ReduceMotion) {
		motion.Stop(state.Y)
	}
	motion.bound = true
	motion.boundWidth, motion.boundHeight = width, height
	if motion.Active {
		// A virtual list refines estimated heights while scrolling. A changing
		// range must not cancel momentum unless the new edge has been reached.
		motion.Target = max(0, min(state.MaxY, motion.Target))
		if state.MaxY <= 0 || state.Y >= state.MaxY && motion.Target >= state.MaxY || state.Y <= 0 && motion.Target <= 0 {
			motion.Stop(max(0, min(state.MaxY, state.Y)))
			state.Y = motion.Position
			return
		}
		position, active := motion.Advance(c.Now())
		state.Y = max(0, min(state.MaxY, position))
		if active {
			c.AnimationFrame()
		}
	}
}

// bindSmoothScroll attaches wheel handling after the container is built. Call
// advanceSmoothScroll before building it; do not also advance the spring elsewhere.
func bindSmoothScroll(c *ui.Context, element *ui.Element, state *ui.ScrollState, motion *smoothScroll) {
	element.TrackScroll(state).HandleInput(func(event ui.InputEvent) bool {
		if !handleSmoothScroll(event, state, motion, c.Now(), c.Preferences().ReduceMotion) {
			return false
		}
		if motion.Active {
			c.AnimationFrame()
		}
		return true
	})
}

func handleSmoothScroll(event ui.InputEvent, state *ui.ScrollState, motion *smoothScroll, now time.Time, reduceMotion bool) bool {
	if event.Kind != ui.InputScroll || event.DY == 0 || event.Mods&ui.Shift != 0 {
		return false
	}
	if event.DY < 0 && state.Y <= 0 || event.DY > 0 && state.Y >= state.MaxY {
		motion.Stop(state.Y)
		return false
	}
	if event.Precise || reduceMotion {
		state.Y = max(0, min(state.MaxY, state.Y+event.DY))
		motion.Stop(state.Y)
		return true
	}
	motion.Add(state.Y, event.DY*1.15, state.MaxY, now)
	return true
}
