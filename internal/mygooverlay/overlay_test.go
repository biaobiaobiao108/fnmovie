package fnmovieoverlay

import "testing"

func TestIgnoreMouseEventsStylePreservesLayeredPresenter(t *testing.T) {
	const layered = uintptr(0x00080000)
	style := ignoreMouseEventsStyle(layered, true)
	if style&layered == 0 || style&0x00000020 == 0 {
		t.Fatalf("click-through style lost layered presentation: %#x", style)
	}
	style = ignoreMouseEventsStyle(style, false)
	if style&layered == 0 || style&0x00000020 != 0 {
		t.Fatalf("disabling click-through changed unrelated styles: %#x", style)
	}
}
