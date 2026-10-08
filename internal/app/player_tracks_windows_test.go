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
