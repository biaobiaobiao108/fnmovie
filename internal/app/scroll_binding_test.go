package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWheelBindingUsesSpringAndPreciseInputCancelsMomentum(t *testing.T) {
	state := ui.ScrollState{Y: 40, MaxY: 500}
	var motion smoothScroll
	now := time.Unix(500, 0)
	if !handleSmoothScroll(ui.InputEvent{Kind: ui.InputScroll, DY: 100}, &state, &motion, now, false) {
		t.Fatal("wheel event was not consumed")
	}
	if state.Y != 40 || !motion.Active || motion.Target != 155 {
		t.Fatalf("wheel should animate from 40 toward 155: state=%+v motion=%+v", state, motion)
	}
	state.Y, _ = motion.Advance(now.Add(20 * time.Millisecond))
	before := state.Y
	if !handleSmoothScroll(ui.InputEvent{Kind: ui.InputScroll, DY: 7, Precise: true}, &state, &motion, now, false) || state.Y != before+7 || motion.Active {
		t.Fatalf("precise input must follow immediately and cancel spring: state=%+v motion=%+v", state, motion)
	}
	if !handleSmoothScroll(ui.InputEvent{Kind: ui.InputScroll, DY: 20}, &state, &motion, now, true) || state.Y != before+27 || motion.Active {
		t.Fatal("reduce motion must move immediately without multiplier or momentum")
	}
}

func TestWheelBindingLeavesEdgesAndOtherInputToNativeContainers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		y     float32
		event ui.InputEvent
	}{
		{"top", 0, ui.InputEvent{Kind: ui.InputScroll, DY: -20}},
		{"bottom", 500, ui.InputEvent{Kind: ui.InputScroll, DY: 20}},
		{"horizontal", 100, ui.InputEvent{Kind: ui.InputScroll, DX: 20}},
		{"shift", 100, ui.InputEvent{Kind: ui.InputScroll, DY: 20, Mods: ui.Shift}},
		{"keyboard", 100, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyPageDown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ui.ScrollState{Y: tc.y, MaxY: 500}
			var motion smoothScroll
			if handleSmoothScroll(tc.event, &state, &motion, time.Now(), false) || state.Y != tc.y || motion.Active {
				t.Fatalf("unhandled event changed or consumed scroll: state=%+v motion=%+v", state, motion)
			}
		})
	}
}

func TestSmoothBindingStopsOnNativeMovementAndResize(t *testing.T) {
	var state ui.ScrollState
	var motion smoothScroll
	tester := ui.NewTester(func(c *ui.Context) {
		advanceSmoothScroll(c, &state, &motion)
		e := ui.Scroll(c).Grow(1).FillHeight().Children(func() {
			for range 30 {
				ui.Box(c).Height(40)
			}
		})
		bindSmoothScroll(c, e, &state, &motion)
	}, 300, 200)
	tester.Frame()
	motion.Add(state.Y, 100, state.MaxY, time.Now())
	state.Y = 300 // Simulate native keyboard/thumb movement before the next frame.
	tester.Frame()
	if motion.Active || state.Y != 300 {
		t.Fatalf("old spring pulled native offset back: y=%v active=%v", state.Y, motion.Active)
	}
	motion.Add(state.Y, 100, state.MaxY, time.Now())
	tester.SetSize(400, 250)
	if motion.Active || state.Y != 300 {
		t.Fatalf("resize retained old momentum: y=%v active=%v", state.Y, motion.Active)
	}
	motion.Add(state.Y, 100, state.MaxY, time.Now())
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	if motion.Active || state.Y != 300 {
		t.Fatalf("reduce motion retained old momentum: y=%v active=%v", state.Y, motion.Active)
	}
}

func TestSmoothBindingBoundaryBubblesToOuterScroll(t *testing.T) {
	var outer, inner ui.ScrollState
	var motion smoothScroll
	tester := ui.NewTester(func(c *ui.Context) {
		advanceSmoothScroll(c, &inner, &motion)
		ui.Scroll(c).Grow(1).FillHeight().TrackScroll(&outer).Children(func() {
			e := ui.Scroll(c).Height(120).Children(func() {
				ui.Text(c, "inner").Height(30)
			})
			bindSmoothScroll(c, e, &inner, &motion)
			ui.Box(c).Height(1200)
		})
	}, 300, 200)
	tester.Frame()
	tester.Scroll(100, 40, 0, 50)
	if outer.Y != 50 || inner.Y != 0 || motion.Active {
		t.Fatalf("wheel over non-scrolling child did not bubble: outer=%+v inner=%+v", outer, inner)
	}
}

func TestSmoothBindingContentMeasurementsPreserveMomentum(t *testing.T) {
	var state ui.ScrollState
	var motion smoothScroll
	rows := 30
	tester := ui.NewTester(func(c *ui.Context) {
		advanceSmoothScroll(c, &state, &motion)
		e := ui.Scroll(c).Grow(1).FillHeight().Children(func() {
			for range rows {
				ui.Box(c).Height(40)
			}
		})
		bindSmoothScroll(c, e, &state, &motion)
	}, 300, 200)
	tester.Frame()
	oldMax := state.MaxY
	motion.Add(state.Y, 100, state.MaxY, time.Now())
	// Keep the test independent of how long CPU rendering takes. No time has
	// elapsed for this spring yet; only the content's measured range changes.
	motion.Last = time.Now().Add(time.Hour)
	rows = 25
	tester.Frame()
	tester.Frame()
	if state.MaxY >= oldMax || !motion.Active || motion.Target != 100 {
		t.Fatalf("height refinement cancelled wheel momentum: max=%v old=%v motion=%+v", state.MaxY, oldMax, motion)
	}
	rows = 1
	tester.Frame()
	tester.Frame()
	if motion.Active || state.Y != 0 || motion.Target != 0 {
		t.Fatalf("removed scroll range should stop at new edge: state=%+v motion=%+v", state, motion)
	}
}
