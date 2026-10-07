package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const fullscreenFadePhase = 80 * time.Millisecond

type playerFullscreenMotion struct {
	started                time.Time
	target, fadingIn       bool
	alpha, originalOpacity float64
}

func (a *appState) fullscreenFade() float64 {
	if a.fullscreenMotion.started.IsZero() {
		return 1
	}
	return a.fullscreenMotion.alpha
}

func (a *appState) requestPlayerFullscreen(target, reduceMotion bool) {
	if a.window == nil {
		return
	}
	a.cancelPlayerFullscreen()
	if a.window.IsFullScreen() == target {
		return
	}
	a.closePlayerOverlayMenu()
	if reduceMotion {
		a.window.SetFullScreen(target)
		a.syncPlayerOverlay()
		return
	}
	a.fullscreenMotion = playerFullscreenMotion{started: time.Now(), target: target, alpha: 1, originalOpacity: a.window.Opacity()}
	a.window.Invalidate()
}

func (a *appState) togglePlayerFullscreen(reduceMotion bool) {
	if a.window == nil {
		return
	}
	target := !a.window.IsFullScreen()
	if !a.fullscreenMotion.started.IsZero() {
		target = !a.fullscreenMotion.target
	}
	a.requestPlayerFullscreen(target, reduceMotion)
}

// Fade the native compositor, change geometry once while transparent, then
// fade back in. No per-frame video resize, screenshots, timers or goroutines.
func (a *appState) advancePlayerFullscreen(c *ui.Context) {
	m := &a.fullscreenMotion
	if m.started.IsZero() || a.window == nil {
		return
	}
	if c.Preferences().ReduceMotion {
		target := m.target
		a.cancelPlayerFullscreen()
		a.window.SetFullScreen(target)
		a.syncPlayerOverlay()
		return
	}
	progress := max(0, min(1, float64(c.Now().Sub(m.started))/float64(fullscreenFadePhase)))
	eased := progress * progress * (3 - 2*progress)
	m.alpha = 1 - eased
	if m.fadingIn {
		m.alpha = eased
	}
	a.window.SetOpacity(m.alpha * m.originalOpacity)
	a.setPlayerOverlayOpacity(a.playerOverlayOpacity)
	if progress >= 1 {
		if m.fadingIn {
			a.cancelPlayerFullscreen()
			return
		}
		a.window.SetFullScreen(m.target)
		a.syncPlayerOverlay()
		m.started, m.fadingIn = c.Now(), true
	}
	c.AnimationFrame()
}

func (a *appState) cancelPlayerFullscreen() {
	if a.fullscreenMotion.started.IsZero() {
		return
	}
	opacity := a.fullscreenMotion.originalOpacity
	a.fullscreenMotion = playerFullscreenMotion{}
	if a.window != nil {
		a.window.SetOpacity(opacity)
	}
	a.setPlayerOverlayOpacity(a.playerOverlayOpacity)
}
