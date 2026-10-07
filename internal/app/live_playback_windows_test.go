//go:build windows

package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestLiveNASPlayback is opt-in. It uses the signed-in test account and
// restores the original watch position after proving playback and seeking.
func TestLiveNASPlayback(t *testing.T) {
	if os.Getenv("FNMOVIE_LIVE_TEST") != "1" {
		t.Skip("set FNMOVIE_LIVE_TEST=1 to run the NAS playback check")
	}
	if runtime.GOOS != "windows" {
		t.Skip("embedded mpv is Windows-only")
	}
	base, username, password := os.Getenv("FNMOVIE_SERVER"), os.Getenv("FNMOVIE_USER"), os.Getenv("FNMOVIE_PASSWORD")
	hwnd, err := strconv.ParseUint(os.Getenv("FNMOVIE_PARENT_HWND"), 10, 64)
	if base == "" || username == "" || password == "" || err != nil || hwnd == 0 {
		t.Fatal("live playback test configuration is incomplete")
	}

	server := NewServer(base, "")
	if _, err := server.Login(username, password); err != nil {
		t.Fatalf("NAS login failed: %v", err)
	}
	libraries, err := server.Libraries()
	if err != nil {
		t.Fatalf("NAS library list failed: %v", err)
	}
	if len(libraries) == 0 {
		t.Fatal("the test account has no accessible media libraries")
	}

	var item MediaItem
	for _, library := range libraries {
		items, listErr := server.LibraryItems(library.ID, "")
		if listErr != nil {
			t.Fatalf("media list failed: %v", listErr)
		}
		for _, candidate := range items {
			if candidate.Kind == "movie" {
				item = candidate
				break
			}
			if item.ID == "" {
				item = candidate
			}
		}
		if item.Kind == "movie" {
			break
		}
	}
	if item.ID == "" {
		t.Fatal("no video item is accessible to the test account")
	}
	if item.Kind != "movie" {
		t.Log("the test account exposes no movie item; verifying the video pipeline with its accessible video instead")
	}

	stream, err := server.Playback(item)
	if err != nil {
		t.Fatalf("NAS did not provide a playback URL: %v", err)
	}
	localURL, proxy, err := server.ProxyPlayback(stream.URL)
	if err != nil {
		t.Fatalf("playback proxy failed: %v", err)
	}
	defer proxy.Close()
	player := NewPlayer()
	player.SetViewport(ui.Rect{X: 24, Y: 80, W: 560, H: 300})
	if err := player.Start(localURL, uintptr(hwnd), stream.Duration, stream.ResumeAt); err != nil {
		t.Fatalf("mpv did not load the NAS stream: %v", err)
	}
	defer player.Stop()

	liveSeconds := 65
	if configured := os.Getenv("FNMOVIE_PLAYBACK_SECONDS"); configured != "" {
		if parsed, parseErr := strconv.Atoi(configured); parseErr == nil && parsed >= 10 {
			liveSeconds = parsed
		}
	}
	deadline := time.Now().Add(time.Duration(liveSeconds) * time.Second)
	start, _ := player.Position()
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		if !player.Running() {
			t.Fatalf("mpv exited during live playback: %+v", player.Snapshot())
		}
	}
	position, duration := player.Position()
	if duration <= 0 || position-start < 5 {
		t.Fatalf("playback position did not advance: start=%.1f current=%.1f duration=%.1f", start, position, duration)
	}
	player.TogglePause()
	time.Sleep(2 * time.Second)
	if !player.Running() {
		t.Fatalf("mpv exited while paused: %s; %s", player.process.exitSummary(), player.Snapshot().Error)
	}
	if paused := player.Snapshot().Paused; !paused {
		t.Fatal("mpv did not enter paused state")
	}
	pausedPosition, _ := player.Position()
	player.TogglePause()
	time.Sleep(3 * time.Second)
	if !player.Running() {
		t.Fatalf("mpv exited after resume: %s; %s", player.process.exitSummary(), player.Snapshot().Error)
	}
	if paused := player.Snapshot().Paused; paused {
		t.Fatal("mpv remained paused after resume")
	}
	resumedPosition, _ := player.Position()
	if pausedPosition-position > 2 || resumedPosition <= pausedPosition {
		t.Fatalf("pause/resume did not work: before=%.1f paused=%.1f resumed=%.1f", position, pausedPosition, resumedPosition)
	}
	player.SetVolume(37)
	time.Sleep(time.Second)
	if volume := player.Snapshot().Volume; volume < 30 || volume > 45 {
		t.Fatalf("volume control did not take effect: %.1f", volume)
	}
	player.SetVolume(100)

	seekTo := resumedPosition + 20
	if duration > 0 && seekTo >= duration {
		seekTo = position - 20
	}
	if seekTo < 0 {
		seekTo = 0
	}
	player.SeekTo(seekTo)
	time.Sleep(3 * time.Second)
	seekPosition, _ := player.Position()
	if seekTo > 0 && seekPosition < seekTo-8 {
		t.Fatalf("seek did not take effect: expected around %.1f, observed %.1f", seekTo, seekPosition)
	}

	player.Stop()
	if player.Running() {
		t.Fatal("mpv remained running after stop")
	}
	progressCtx, cancelProgress := context.WithTimeout(context.Background(), 8*time.Second)
	err = server.UpdateProgressContext(progressCtx, item.ID, stream.MediaID, seekPosition, duration)
	cancelProgress()
	if err != nil {
		t.Fatalf("progress sync failed: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = server.UpdateProgressContext(ctx, item.ID, stream.MediaID, stream.ResumeAt, duration)
	}()
	restoreCtx, cancelRestore := context.WithTimeout(context.Background(), 8*time.Second)
	err = server.UpdateProgressContext(restoreCtx, item.ID, stream.MediaID, stream.ResumeAt, duration)
	cancelRestore()
	if err != nil {
		t.Fatalf("could not restore the original watch position: %v", err)
	}
	state := player.Snapshot()
	t.Log(fmt.Sprintf("live stream decoded for %d seconds; seek worked; tracks: audio=%d subtitles=%d; output=%s params=%s", liveSeconds, len(state.AudioTracks), len(state.SubtitleTracks), state.AudioOutput, state.AudioParams))
}

func TestLiveNASCatalog(t *testing.T) {
	if os.Getenv("FNMOVIE_LIVE_TEST") != "1" {
		t.Skip("set FNMOVIE_LIVE_TEST=1 to run the NAS catalog check")
	}
	base, username, password := os.Getenv("FNMOVIE_SERVER"), os.Getenv("FNMOVIE_USER"), os.Getenv("FNMOVIE_PASSWORD")
	if base == "" || username == "" || password == "" {
		t.Fatal("live NAS catalog test configuration is incomplete")
	}
	server := NewServer(base, "")
	if _, err := server.Login(username, password); err != nil {
		t.Fatalf("NAS login failed: %v", err)
	}
	libraries, err := server.Libraries()
	if err != nil {
		t.Fatalf("NAS library list failed: %v", err)
	}
	t.Logf("NAS returned %d libraries", len(libraries))
	for _, library := range libraries {
		items, total, err := server.LibraryPageContext(t.Context(), library.ID, "", 1, catalogPageSize)
		if err != nil {
			t.Errorf("library %q first page failed: %v", library.Name, err)
			continue
		}
		t.Logf("library %q: first page %d items; server total %d", library.Name, len(items), total)
		if len(items) > 0 {
			for _, item := range items {
				if itemLibraryID, hasLibraryID := itemLibrary(item.Raw); hasLibraryID && itemLibraryID != library.ID {
					t.Errorf("library %q returned foreign item %q from library %q", library.Name, item.ID, itemLibraryID)
				}
			}
			stream, playErr := server.Playback(items[0])
			if playErr != nil {
				t.Errorf("library %q first media has no playback source: %v", library.Name, playErr)
			} else {
				t.Logf("library %q playback source resolved (%s)", library.Name, stream.Quality)
			}
		}
	}
}

func TestLiveNASPeople(t *testing.T) {
	if os.Getenv("FNMOVIE_LIVE_TEST") != "1" {
		t.Skip("set FNMOVIE_LIVE_TEST=1 to run the NAS check")
	}
	base, username, password := os.Getenv("FNMOVIE_SERVER"), os.Getenv("FNMOVIE_USER"), os.Getenv("FNMOVIE_PASSWORD")
	server := NewServer(base, "")
	if _, err := server.Login(username, password); err != nil {
		t.Fatalf("NAS login failed: %v", err)
	}
	libraries, err := server.Libraries()
	if err != nil || len(libraries) == 0 {
		t.Fatalf("NAS library list failed: %v", err)
	}
	var sample MediaItem
	for _, lib := range libraries {
		items, _, err := server.LibraryPageContext(t.Context(), lib.ID, "", 1, 10)
		if err == nil && len(items) > 0 {
			sample = items[0]
			break
		}
	}
	t.Logf("Testing with sample media item: ID=%s Title=%s Raw=%+v", sample.ID, sample.Title, sample.Raw)

	// Test POST person/list with body
	body := map[string]any{"guid": sample.ID, "page": 1, "page_size": 200}
	var postResp any
	postErr := server.request("POST", "v1", "person/list/"+url.PathEscape(sample.ID), body, &postResp, server.tokenValue())
	t.Logf("POST person/list/%s (with body): err=%v, resp=%+v", sample.ID, postErr, postResp)

	// Test POST person/item/list with person_guid
	people, err := server.People(sample.ID)
	if err != nil || len(people) == 0 {
		t.Fatalf("People failed: %v", err)
	}
	firstPerson := people[0]
	t.Logf("First person: ID=%s Name=%s", firstPerson.ID, firstPerson.Name)
	itemsBody := map[string]any{"person_guid": firstPerson.ID, "page": 1, "page_size": 20}
	var itemsResp any
	itemsErr := server.request("POST", "v1", "person/item/list", itemsBody, &itemsResp, server.tokenValue())
	t.Logf("POST person/item/list (for %s): err=%v, resp=%+v", firstPerson.Name, itemsErr, itemsResp)
}

func TestLiveNASFavorites(t *testing.T) {
	if os.Getenv("FNMOVIE_LIVE_TEST") != "1" {
		t.Skip("set FNMOVIE_LIVE_TEST=1 to run the NAS check")
	}
	base, username, password := os.Getenv("FNMOVIE_SERVER"), os.Getenv("FNMOVIE_USER"), os.Getenv("FNMOVIE_PASSWORD")
	server := NewServer(base, "")
	if _, err := server.Login(username, password); err != nil {
		t.Fatalf("NAS login failed: %v", err)
	}

	favItems, total, favErr := server.MediaPageContext(t.Context(), "", "favorite", "", 1, 50)
	if favErr != nil {
		t.Fatalf("Favorite MediaPageContext failed: %v", favErr)
	}
	t.Logf("Favorites: total=%d, itemsCount=%d", total, len(favItems))
}
