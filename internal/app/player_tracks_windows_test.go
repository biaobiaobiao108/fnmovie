//go:build windows

package app

import (
	"math/bits"
	"testing"
)

func TestMPVNodeFlagReadsOnlyFlagValue(t *testing.T) {
	upperBitsOnly := mpvNode{value: uintptr(1) << (bits.UintSize / 2), format: mpvFormatFlag}
	trueFlag := mpvNode{value: uintptr(1), format: mpvFormatFlag}
	wrongFormat := mpvNode{value: uintptr(1), format: mpvFormatInt64}

	if mpvNodeFlag(upperBitsOnly) {
		t.Fatal("flag must ignore unspecified upper bits")
	}
	if !mpvNodeFlag(trueFlag) {
		t.Fatal("flag value 1 should be true")
	}
	if mpvNodeFlag(wrongFormat) {
		t.Fatal("non-flag node must not be interpreted as a flag")
	}
}

func TestPlayerTrackRefreshOnlyRunsAfterRelevantEvents(t *testing.T) {
	var refresh playerTrackRefreshState
	if refresh.due() {
		t.Fatal("track refresh should not be due before media is loaded")
	}
	refresh.observe(21, 0) // Playback restart does not change the available tracks.
	if refresh.due() {
		t.Fatal("unrelated player event marked tracks dirty")
	}
	refresh.observe(mpvEventFileLoaded, 0)
	if !refresh.due() {
		t.Fatal("media load should refresh the track list")
	}
	refresh.refreshed()
	if refresh.due() {
		t.Fatal("track list stayed dirty after a successful refresh")
	}
	refresh.observe(mpvEventPropertyChange, 0)
	if refresh.due() {
		t.Fatal("unrelated property change marked tracks dirty")
	}
	refresh.observe(mpvEventPropertyChange, mpvTrackListObserveUserdata)
	if !refresh.due() {
		t.Fatal("observed track-list changes should refresh track metadata")
	}
}
