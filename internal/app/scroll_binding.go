package app

import "github.com/egoist/mygo/ui"

// bindSmoothScroll gives each region its own remembered position. MyGo 0.6
// owns wheel gliding, precise-input handling, reduced motion and edge bubbling.
func bindSmoothScroll(element ui.Element, state *ui.ScrollState) {
	element.TrackScroll(state)
}
