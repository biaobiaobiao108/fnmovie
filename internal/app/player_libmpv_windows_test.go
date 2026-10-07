//go:build windows

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledLibMpvInitializes(t *testing.T) {
	path := filepath.Join("..", "..", "resources", "windows-amd64", "player", "libmpv-2.dll")
	if _, err := os.Stat(path); err != nil {
		t.Skip("libmpv LFS asset is not present in this checkout")
	}
	api, err := loadMpvAPI(path)
	if err != nil {
		t.Fatalf("load libmpv: %v", err)
	}
	ctx := api.create()
	if ctx == 0 {
		t.Fatal("mpv_create returned a nil handle")
	}
	if code := setMpvOption(api, ctx, "config", "no"); code < 0 {
		api.terminate(ctx)
		t.Fatalf("disable external libmpv config: %s", mpvError(api, code))
	}
	if code := setMpvOption(api, ctx, "vo", "null"); code < 0 {
		api.terminate(ctx)
		t.Fatalf("select headless video output: %s", mpvError(api, code))
	}
	if code := setMpvOption(api, ctx, "ao", "null"); code < 0 {
		api.terminate(ctx)
		t.Fatalf("select headless audio output: %s", mpvError(api, code))
	}
	if code := api.initialize(ctx); code < 0 {
		api.terminate(ctx)
		t.Fatalf("initialize libmpv: %s", mpvError(api, code))
	}
	api.terminate(ctx)
}
