package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNativeScrollBindingUsesWheelGlideAndHonorsDirectInput(t *testing.T) {
	var state ui.ScrollState
	tester := ui.NewTester(func(c *ui.Context) {
		element := ui.Scroll(c).Grow(1).FillHeight().Children(func() {
			for range 30 {
				ui.Box(c).Height(40)
			}
		})
		bindSmoothScroll(element, &state)
	}, 300, 200)
	tester.Frame()

	tester.Wheel(100, 40, 0, 100)
	tester.Advance(75 * time.Millisecond)
	if state.Y <= 0 || state.Y >= 100 {
		t.Fatalf("mouse wheel should glide between its start and target, got %v", state.Y)
	}
	tester.Advance(100 * time.Millisecond)
	if math.Abs(float64(state.Y-100)) > 0.1 {
		t.Fatalf("mouse wheel glide settled at %v, want 100", state.Y)
	}

	tester.Scroll(100, 40, 0, 25)
	if state.Y != 125 {
		t.Fatalf("precise input should follow immediately, got %v", state.Y)
	}

	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Wheel(100, 40, 0, 30)
	if state.Y != 155 {
		t.Fatalf("reduced motion should make wheel input direct, got %v", state.Y)
	}
}
