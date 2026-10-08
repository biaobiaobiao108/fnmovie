package app

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMyGoPlayerSeekPreservesDeferredInputAndCommitsOnRelease(t *testing.T) {
	a := &appState{playback: PlaybackState{Position: 12, Duration: 100}, seekSliderPosition: 12}
	var commits []float64
	commit := func(target float64) {
		commits = append(commits, target)
		a.seekFeedback.Pending = true
		a.seekFeedback.Target = target
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Size(400, 40).Children(func() { a.playerSeekSlider(c, 100, commit) })
	}, 400, 80)
	r, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("seek slider missing")
	}
	y := r.Y + r.H/2
	tt.Press(r.X+r.W*0.3, y)
	tt.Move(r.X+r.W*0.8, y)
	if len(commits) != 0 || !a.seekDragging || a.seekSliderPosition < 70 {
		t.Fatalf("drag lost or committed before release: target=%v dragging=%v commits=%v", a.seekSliderPosition, a.seekDragging, commits)
	}
	tt.Frame()
	if a.seekSliderPosition < 70 {
		t.Fatal("old player snapshot overwrote deferred input")
	}
	tt.Release(r.X+r.W*0.8, y)
	if len(commits) != 1 || commits[0] < 70 || a.seekDragging {
		t.Fatalf("release did not submit latest target once: commits=%v dragging=%v", commits, a.seekDragging)
	}
	tt.Frame()
	if len(commits) != 1 || math.Abs(a.seekSliderPosition-commits[0]) > 0.001 {
		t.Fatal("pending target reset or release repeated")
	}
	// An additional edit supersedes the pending remote target.
	tt.ClickAt(r.X+r.W*0.45, y)
	if len(commits) != 2 || commits[1] < 35 || commits[1] > 55 {
		t.Fatalf("new edit did not supersede pending target: %v", commits)
	}
}

func TestMyGoPlayerSeekKeyboardEditNotOverwrittenBySnapshot(t *testing.T) {
	a := &appState{playback: PlaybackState{Position: 12, Duration: 100}, seekSliderPosition: 12}
	var commits []float64
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Size(400, 40).Children(func() {
			a.playerSeekSlider(c, 100, func(target float64) {
				commits = append(commits, target)
				a.seekFeedback.Pending = true
			})
		})
	}, 400, 80)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	if len(commits) != 1 || commits[0] <= 12 {
		t.Fatalf("keyboard edit reset or not committed: %v", commits)
	}
	tt.Frame()
	if len(commits) != 1 || a.seekSliderPosition != commits[0] {
		t.Fatalf("keyboard change repeated or target reset: position=%v commits=%v", a.seekSliderPosition, commits)
	}
}
