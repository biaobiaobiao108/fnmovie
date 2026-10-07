package app

import (
	"math"
	"testing"
	"time"
)

func TestSmoothScrollAccumulatesWheelInputWithoutRestartingMotion(t *testing.T) {
	start := time.Unix(100, 0)
	var scroll smoothScroll
	scroll.Add(0, 100, 500, start)
	first, active := scroll.Advance(start.Add(35 * time.Millisecond))
	if !active || first <= 0 || first >= 100 {
		t.Fatalf("first spring position=%v active=%v", first, active)
	}
	scroll.Add(first, 100, 500, start.Add(35*time.Millisecond))
	if scroll.Target != 200 {
		t.Fatalf("accumulated target=%v, want 200", scroll.Target)
	}
	var last float32
	for step := 1; step <= 60; step++ {
		last, active = scroll.Advance(start.Add(time.Duration(step) * 16 * time.Millisecond))
		if !active {
			break
		}
	}
	if active || math.Abs(float64(last-200)) > 0.01 {
		t.Fatalf("settled position=%v active=%v", last, active)
	}
}

func TestSmoothScrollClampsAndCanReverse(t *testing.T) {
	start := time.Unix(200, 0)
	var scroll smoothScroll
	scroll.Add(0, 900, 240, start)
	if scroll.Target != 240 {
		t.Fatalf("target=%v, want max 240", scroll.Target)
	}
	position, _ := scroll.Advance(start.Add(20 * time.Millisecond))
	scroll.Add(position, -80, 240, start.Add(20*time.Millisecond))
	if scroll.Target != 160 {
		t.Fatalf("reversed target=%v, want 160", scroll.Target)
	}
	var active bool
	for step := 1; step <= 60; step++ {
		position, active = scroll.Advance(start.Add(20*time.Millisecond + time.Duration(step)*16*time.Millisecond))
		if !active {
			break
		}
	}
	if math.Abs(float64(position-160)) > 0.01 {
		t.Fatalf("reversed motion settled at %v, want 160", position)
	}
}

func TestSmoothScrollStopAndPositionResync(t *testing.T) {
	start := time.Unix(300, 0)
	var scroll smoothScroll
	scroll.Add(0, 150, 600, start)
	scroll.Advance(start.Add(16 * time.Millisecond))
	if !scroll.Active {
		t.Fatal("scroll should be active")
	}
	scroll.Stop(42)
	if scroll.Active || scroll.Position != 42 || scroll.Target != 42 || scroll.Velocity != 0 {
		t.Fatalf("scroll did not stop at requested position: %#v", scroll)
	}

	// Resync on external position jump while active
	scroll.Add(42, 100, 600, start.Add(32*time.Millisecond))
	// External drag sets position far away to 200
	scroll.Add(200, 50, 600, start.Add(48*time.Millisecond))
	if scroll.Position != 200 {
		t.Fatalf("scroll should resync position to external jump, got %v", scroll.Position)
	}
}
