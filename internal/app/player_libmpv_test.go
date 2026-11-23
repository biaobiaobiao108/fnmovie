//go:build windows || darwin

package app

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func testMpvLibrary(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("FNMOVIE_TEST_LIBMPV"); path != "" {
		return path
	}
	path := filepath.Join("..", "..", "resources", playerResourcePlatform, "player", playerMpvFilename)
	if _, err := os.Stat(path); err != nil {
		t.Skip("bundled libmpv is not present")
	}
	return path
}

func TestPlayerRepeatedMediaEndsAndRetainsProgress(t *testing.T) {
	path := testMpvLibrary(t)
	api, err := loadMpvAPI(path)
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(t.TempDir(), "short.wav")
	data := make([]byte, 44+16000)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 16000)
	if err := os.WriteFile(media, data, 0600); err != nil {
		t.Fatal(err)
	}
	process := &playerProcess{}
	for iteration := 0; iteration < 2; iteration++ {
		ctx := api.create()
		if ctx == 0 {
			t.Fatal("nil mpv instance")
		}
		for _, option := range [][2]string{{"config", "no"}, {"vo", "null"}, {"ao", "null"}} {
			if code := setMpvOption(api, ctx, option[0], option[1]); code < 0 {
				api.terminate(ctx)
				t.Fatal(mpvError(api, code))
			}
		}
		if code := api.initialize(ctx); code < 0 {
			api.terminate(ctx)
			t.Fatal(mpvError(api, code))
		}
		process.resetSession(api, ctx, 0, 0)
		go process.eventLoop()
		if code := process.command("loadfile", media, "replace"); code < 0 {
			process.stop()
			t.Fatal(mpvError(api, code))
		}
		select {
		case <-process.loaded:
		case <-time.After(5 * time.Second):
			process.stop()
			t.Fatal("media did not load on iteration", iteration)
		}
		select {
		case <-process.done:
		case <-time.After(5 * time.Second):
			process.stop()
			t.Fatal("EOF did not end media session")
		}
		state := process.snapshot()
		if process.running() || state.Error != "" || state.Duration < 0.9 || math.Abs(state.Position-state.Duration) > 0.01 {
			process.stop()
			t.Fatalf("invalid EOF snapshot: %+v", state)
		}
		process.stop()
	}
}

func TestPlayerStopCancelsPendingLoadWithoutWaiting(t *testing.T) {
	player := NewPlayer()
	ctx, cancel := context.WithCancel(context.Background())
	player.startCancel = cancel
	started := time.Now()
	player.Stop()
	if ctx.Err() != context.Canceled || time.Since(started) > time.Second {
		t.Fatal("stop did not promptly cancel pending playback")
	}
}

func TestBundledLibMpvInitializes(t *testing.T) {
	path := testMpvLibrary(t)
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

func TestPlayerSnapshotDuringSlowMpvRefresh(t *testing.T) {
	blocked := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	api := &mpvAPI{
		getProperty: func(_ uintptr, _ *byte, format int32, _ unsafe.Pointer) int32 {
			if format == mpvFormatFlag {
				once.Do(func() {
					close(blocked)
					<-release
				})
			}
			return -1
		},
		getPropertyStr: func(uintptr, *byte) uintptr { return 0 },
	}
	p := &playerProcess{}
	p.resetSession(api, 1, 0, 0)
	p.state.AudioTracks = []PlayerTrack{{ID: 1}}
	go func() {
		defer close(finished)
		p.refreshState()
	}()
	defer func() {
		close(release)
		<-finished
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("refresh did not query libmpv")
	}

	snapshot := make(chan PlayerSnapshot, 1)
	go func() { snapshot <- p.snapshot() }()
	select {
	case got := <-snapshot:
		if len(got.AudioTracks) != 1 || got.AudioTracks[0].ID != 1 {
			t.Fatalf("invalid cached tracks: %+v", got.AudioTracks)
		}
	case <-time.After(time.Second):
		t.Fatal("cached snapshot waited for libmpv")
	}
}
