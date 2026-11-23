package app

import "testing"

func TestBundledLibMpvOpenGLBindings(t *testing.T) {
	if err := loadDarwinRenderAPI(testMpvLibrary(t)); err != nil {
		t.Fatal(err)
	}
}
