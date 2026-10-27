package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"
)

func (a *appState) startPlayback(item MediaItem) {
	if a.server == nil {
		return
	}
	if a.playback.Active {
		a.stopPlayback()
	}
	if a.playbackCancel != nil {
		a.playbackCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.playbackRequest++
	requestID := a.playbackRequest
	a.playbackCancel = cancel
	a.playbackLoading = true
	a.playbackLoadingID = item.ID
	a.playbackLoadingTitle = item.Title
	if a.window != nil {
		a.window.Invalidate()
	}
	server := a.server
	nativeHandle := uintptr(0)
	if a.window != nil {
		nativeHandle = a.window.NativeHandle()
	}
	requestPlayer := NewPlayer()
	if err := requestPlayer.PrepareSurface(nativeHandle); err != nil {
		a.cancelPlaybackLoading()
		a.status = "播放器绘制区域创建失败：" + err.Error()
		return
	}
	cleanup := func() {
		requestPlayer.Stop()
		a.window.Update(func() { requestPlayer.ReleaseSurface() })
	}
	go func() {
		stream, err := server.PlaybackContext(ctx, item)
		if ctx.Err() != nil {
			cleanup()
			return
		}
		if err != nil {
			cleanup()
			a.window.Update(func() {
				if ctx.Err() != nil {
					return
				}
				a.playbackLoading = false
				a.playbackLoadingID = ""
				a.playbackLoadingTitle = ""
				a.playbackCancel = nil
				a.status = "无法获取播放源：" + err.Error()
			})
			return
		}
		item.MediaID = stream.MediaID
		localURL, proxy, err := server.ProxyPlayback(stream.URL)
		if ctx.Err() != nil {
			if proxy != nil {
				proxy.Close()
			}
			cleanup()
			return
		}
		if err != nil {
			cleanup()
			a.window.Update(func() {
				if ctx.Err() != nil {
					return
				}
				a.playbackLoading = false
				a.playbackLoadingID = ""
				a.playbackLoadingTitle = ""
				a.playbackCancel = nil
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放代理启动失败：" + err.Error()
			})
			return
		}

		// 在后台 goroutine 中等待媒体载入，绝不在 UI 线程中阻塞！
		startErr := requestPlayer.StartContext(ctx, localURL, nativeHandle, stream.Duration, stream.ResumeAt)
		if ctx.Err() != nil {
			proxy.Close()
			cleanup()
			return
		}

		a.window.Update(func() {
			if ctx.Err() != nil || a.server != server || a.playbackRequest != requestID {
				proxy.Close()
				requestPlayer.Stop()
				requestPlayer.ReleaseSurface()
				return
			}
			if startErr != nil {
				proxy.Close()
				requestPlayer.Stop()
				requestPlayer.ReleaseSurface()
				a.playerProxy = nil
				a.playbackLoading = false
				a.playbackLoadingID = ""
				a.playbackLoadingTitle = ""
				a.playbackCancel = nil
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放器启动失败：" + startErr.Error()
				return
			}
			a.player = requestPlayer
			a.posters.SetPlayback(true)
			a.cancelDetailRequests()
			if a.personCancel != nil {
				a.personCancel()
				a.personCancel = nil
			}
			requestPlayer.ShowSurface()
			a.window.SetWindowButtonsTheme(mygo.ThemeDark)
			a.playerProxy = proxy
			a.playingItem = &item
			a.playback = PlaybackState{Active: true, Title: item.Title, URL: stream.URL, Position: stream.ResumeAt, Duration: stream.Duration, Volume: 100, Speed: 1, Quality: stream.Quality}
			a.syncWindowTitle()
			a.syncPlaybackTaskbarProgress()
			a.playbackLoading = false
			a.playbackLoadingID = ""
			a.playbackLoadingTitle = ""
			a.playbackCancel = nil
			a.seekSliderPosition = stream.ResumeAt
			a.seekFeedback = seekFeedback{}
			if a.playbackProgressReporter == nil {
				a.playbackProgressReporter = newPlaybackProgressReporter()
			}
			var session *playbackProgressSession
			session = a.playbackProgressReporter.newSession(server, item, func(err error) {
				a.window.Update(func() {
					if a.server == server && a.playbackProgressSession == session && a.player == requestPlayer {
						a.status = "进度同步失败：" + err.Error()
					}
				})
			})
			a.playbackProgressSession = session
			a.window.Invalidate()
			a.createPlayerOverlay()
			a.startPlayerOverlayMonitor()
			trackCtx, trackCancel := context.WithCancel(context.Background())
			a.playbackTrackCancel = trackCancel
			go a.trackPlayback(trackCtx, item, requestPlayer, session)
		})
	}()
}

func (a *appState) cancelPlaybackLoading() {
	a.playbackRequest++
	if a.playbackCancel != nil {
		a.playbackCancel()
		a.playbackCancel = nil
	}
	a.playbackLoading = false
	a.playbackLoadingID = ""
	a.playbackLoadingTitle = ""
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) trackPlayback(ctx context.Context, item MediaItem, player *Player, progress *playbackProgressSession) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastProgress := time.Now()
	for player.Running() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !player.Running() {
			break
		}
		snapshot := player.Snapshot()
		position, duration := snapshot.Position, snapshot.Duration
		a.window.Update(func() {
			if ctx.Err() != nil || a.player != player || !a.playback.Active || a.playingItem == nil || a.playingItem.ID != item.ID {
				return
			}
			seekWasPending, seekError := a.seekFeedback.Pending, a.seekFeedback.Error
			a.playback.Position = a.seekFeedback.position(snapshot, a.playback.Position)
			if seekWasPending != a.seekFeedback.Pending || seekError != a.seekFeedback.Error {
				a.syncPlayerSeekFeedbackVisibility()
			}
			a.playback.Paused = snapshot.Paused
			if !a.volumeDragging {
				a.playback.Volume = snapshot.Volume
			}
			a.playback.Muted = snapshot.Muted
			if snapshot.Speed > 0 {
				a.playback.Speed = snapshot.Speed
			}
			a.playback.AudioOutput = snapshot.AudioOutput
			a.playback.AudioParams = snapshot.AudioParams
			a.playback.AudioTracks = snapshot.AudioTracks
			a.playback.SubtitleTracks = snapshot.SubtitleTracks
			if duration > 0 {
				a.playback.Duration = duration
			}
			a.syncPlaybackTaskbarProgress()
			if a.playerOverlayWindow != nil {
				a.playerOverlayWindow.Invalidate()
			}
		})
		if time.Since(lastProgress) >= 30*time.Second {
			lastProgress = time.Now()
			progress.periodic(position, duration)
		}
	}
	if ctx.Err() != nil {
		return
	}
	snapshot := player.Snapshot()
	a.window.Update(func() {
		if ctx.Err() == nil && a.player == player && a.playback.Active && a.playingItem != nil && a.playingItem.ID == item.ID {
			if snapshot.Error != "" {
				a.stopPlayback()
				a.playback = PlaybackState{Active: true, Title: item.Title, Error: snapshot.Error}
				a.syncWindowTitle()
				a.window.Invalidate()
			} else {
				// A clean mpv exit (including natural completion) returns to the
				// library instead of leaving a frozen video surface behind.
				a.stopPlayback()
			}
		}
	})
}

func (a *appState) stopPlayback() {
	server, item, position, duration, progress := a.closePlayer()
	if server != nil && item != nil && !a.home.Closed && a.selectedPerson == nil {
		a.resumeDetailLoading()
	}
	if server != nil && item != nil {
		if progress == nil {
			if a.playbackProgressReporter == nil {
				a.playbackProgressReporter = newPlaybackProgressReporter()
			}
			progress = a.playbackProgressReporter.newSession(server, *item, nil)
		}
		done := progress.finish(position, duration, playbackProgressTimeout)
		go func() {
			if err := <-done; err != nil {
				a.window.Update(func() {
					if a.server == server {
						a.status = "进度同步失败：" + err.Error()
					}
				})
			}
			a.window.Update(func() {
				if a.server == server && !a.home.Closed {
					a.loadContinueWatching(true)
				}
			})
		}()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) stopPlaybackAndWait(ctx context.Context) {
	server, item, position, duration, progress := a.closePlayer()
	var done <-chan error
	if server != nil && item != nil {
		if progress == nil {
			if a.playbackProgressReporter == nil {
				a.playbackProgressReporter = newPlaybackProgressReporter()
			}
			progress = a.playbackProgressReporter.newSession(server, *item, nil)
		}
		done = progress.finish(position, duration, 3*time.Second)
	} else if a.playbackProgressReporter != nil {
		done = a.playbackProgressReporter.flush()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}

func (a *appState) closePlayer() (*Server, *MediaItem, float64, float64, *playbackProgressSession) {
	a.cancelPlaybackLoading()
	if a.playbackTrackCancel != nil {
		a.playbackTrackCancel()
		a.playbackTrackCancel = nil
	}
	progress := a.playbackProgressSession
	a.playbackProgressSession = nil
	var server *Server
	var item *MediaItem
	var position, duration float64
	if a.server != nil && a.playingItem != nil {
		server = a.server
		copy := *a.playingItem
		item = &copy
		position, duration = a.player.Position()
	}
	a.clearPlaybackTaskbarProgress()
	a.closePlayerOverlay()
	if a.player != nil {
		// Cover the native buttons with video while returning their theme to the
		// light app surface, avoiding a visible color flash during player exit.
		if a.window != nil {
			a.player.SetCaptionButtonCutout(false, a.window.NativeHandle())
			a.window.SetWindowButtonsTheme(mygo.ThemeSystem)
		}
	}
	a.player.Stop()
	a.player.ReleaseSurface()
	a.playerProxy.Close()
	a.playerProxy = nil
	a.playingItem = nil
	a.playback = PlaybackState{}
	a.syncWindowTitle()
	a.posters.SetPlayback(false)
	return server, item, position, duration, progress
}
