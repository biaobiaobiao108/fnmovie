package main

import (
	"math"
	"testing"
	"time"
)

func TestSmoothScrollInterpolatesAndFinishesAtTarget(t *testing.T) {
	start := time.Unix(100, 0)
	var scroll smoothScroll
	scroll.Retarget(0, 120, start, catalogScrollDuration)
	first, active := scroll.Position(start.Add(catalogScrollDuration / 2))
	if !active || first <= 0 || first >= 120 {
		t.Fatalf("mid-animation position=%v active=%v", first, active)
	}
	last, active := scroll.Position(start.Add(catalogScrollDuration))
	if active || math.Abs(float64(last-120)) > 0.01 {
		t.Fatalf("finished position=%v active=%v", last, active)
	}
}

func TestSmoothScrollRetargetStartsFromCurrentPosition(t *testing.T) {
	start := time.Unix(200, 0)
	var scroll smoothScroll
	scroll.Retarget(10, 110, start, catalogScrollDuration)
	current, _ := scroll.Position(start.Add(50 * time.Millisecond))
	scroll.Retarget(current, 210, start.Add(50*time.Millisecond), catalogScrollDuration)
	if scroll.From != current || scroll.To != 210 {
		t.Fatalf("retarget state = %#v", scroll)
	}
}
