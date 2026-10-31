package app

import (
	"context"
	"time"
)

func (a *appState) startPlayback(item MediaItem) {
	if a.server == nil {
		return
	}
	if a.playback.Active || a.playbackLoading {
		a.stopPlayback()
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
	requestPlayer := NewPlayer()
	requestPlayer.SetOnDoubleClick(func() {
		if window := a.playbackWindow(); window != nil {
			window.Update(func() {
				if a.playbackWindow() != nil && (a.playback.Active || a.playbackLoading) {
					a.togglePlayerFullscreen(false)
				}
			})
		}
	})
	go func() {
		stream, err := server.PlaybackContext(ctx, item)
		if ctx.Err() != nil {
			a.cleanupPlayerRequest(requestID, requestPlayer)
			return
		}
		if err != nil {
			a.cleanupPlayerRequest(requestID, requestPlayer)
			a.window.Update(func() {
				if ctx.Err() != nil {
					return
				}
				a.clearPlaybackLoadingState()
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
			a.cleanupPlayerRequest(requestID, requestPlayer)
			return
		}
		if err != nil {
			a.cleanupPlayerRequest(requestID, requestPlayer)
			a.window.Update(func() {
				if ctx.Err() != nil {
					return
				}
				a.clearPlaybackLoadingState()
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放代理启动失败：" + err.Error()
			})
			return
		}

		a.window.Update(func() {
			a.startPlayerWindow(ctx, requestID, server, item, stream, localURL, proxy, requestPlayer)
		})
	}()
}

func (a *appState) cleanupPlayerRequest(requestID uint64, player *Player) {
	player.Stop()
	if a.window == nil {
		player.ReleaseSurface()
		return
	}
	a.window.Update(func() {
		player.ReleaseSurface()
		if a.playerWindowRequest == requestID && a.playbackLoadingPlayer == player {
			if a.player == player {
				a.player = NewPlayer()
			}
			a.destroyPlayerWindow()
		}
	})
}

func (a *appState) startPlayerWindow(ctx context.Context, requestID uint64, server *Server, item MediaItem, stream PlaybackResult, localURL string, proxy *PlaybackProxy, player *Player) {
	if ctx.Err() != nil || a.server != server || a.playbackRequest != requestID {
		proxy.Close()
		a.cleanupPlayerRequest(requestID, player)
		return
	}
	a.player = player
	a.playbackLoadingPlayer = player
	a.createPlayerWindow(windowTitleForPlayback(PlaybackState{Active: true, Title: item.Title}), requestID)
	nativeHandle := a.playerWindow.NativeHandle()
	if err := player.PrepareSurface(nativeHandle); err != nil {
		a.failPlayerStartup(requestID, player, proxy, "播放器绘制区域创建失败："+err.Error())
		return
	}
	go func() {
		// Wait for media loading off the UI thread.
		startErr := player.StartContext(ctx, localURL, nativeHandle, stream.Duration, stream.ResumeAt)
		if ctx.Err() != nil {
			proxy.Close()
			a.cleanupPlayerRequest(requestID, player)
			return
		}
		a.window.Update(func() {
			a.finishPlayerStartup(ctx, requestID, server, item, stream, proxy, player, startErr)
		})
	}()
}

func (a *appState) finishPlayerStartup(ctx context.Context, requestID uint64, server *Server, item MediaItem, stream PlaybackResult, proxy *PlaybackProxy, player *Player, startErr error) {
	if ctx.Err() != nil || a.server != server || a.playbackRequest != requestID || a.playerWindowRequest != requestID {
		proxy.Close()
		player.Stop()
		player.ReleaseSurface()
		if a.playerWindowRequest == requestID {
			if a.player == player {
				a.player = NewPlayer()
			}
			a.destroyPlayerWindow()
		}
		return
	}
	if startErr != nil {
		a.failPlayerStartup(requestID, player, proxy, "播放器启动失败："+startErr.Error())
		return
	}
	a.playingItem = &item
	a.playback = PlaybackState{Active: true, Title: item.Title, URL: stream.URL, Position: stream.ResumeAt, Duration: stream.Duration, Volume: 100, Speed: 1, Quality: stream.Quality}
	a.sizePlayerWindowForVideo(requestID, player.Snapshot())
	player.ShowSurface()
	a.playbackLoadingPlayer = nil
	a.playerProxy = proxy
	a.syncWindowTitle()
	a.syncPlaybackTaskbarProgress()
	a.clearPlaybackLoadingState()
	a.seekSliderPosition = stream.ResumeAt
	a.seekFeedback = seekFeedback{}
	if a.playbackProgressReporter == nil {
		a.playbackProgressReporter = newPlaybackProgressReporter()
	}
	var session *playbackProgressSession
	session = a.playbackProgressReporter.newSession(server, item, func(err error) {
		a.window.Update(func() {
			if a.server == server && a.playbackProgressSession == session && a.player == player {
				a.status = "进度同步失败：" + err.Error()
			}
		})
	})
	a.playbackProgressSession = session
	if a.playerWindow != nil {
		a.playerWindow.Invalidate()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
	a.createPlayerOverlay()
	a.startPlayerOverlayMonitor()
	trackCtx, trackCancel := context.WithCancel(context.Background())
	a.playbackTrackCancel = trackCancel
	go a.trackPlayback(trackCtx, item, player, session)
}

func (a *appState) failPlayerStartup(requestID uint64, player *Player, proxy *PlaybackProxy, message string) {
	proxy.Close()
	player.Stop()
	player.ReleaseSurface()
	if a.player == player {
		a.player = NewPlayer()
	}
	if a.playerWindowRequest == requestID {
		a.destroyPlayerWindow()
	}
	a.playerProxy = nil
	a.playback.Active = false
	a.playingItem = nil
	a.clearPlaybackLoadingState()
	a.status = message
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) clearPlaybackLoadingState() {
	a.playbackLoading = false
	a.playbackLoadingID = ""
	a.playbackLoadingTitle = ""
	a.playbackCancel = nil
}

func (a *appState) cancelPlaybackLoading() {
	requestID := a.playbackRequest
	a.playbackRequest++
	if a.playbackCancel != nil {
		a.playbackCancel()
		a.playbackCancel = nil
	}
	if player := a.playbackLoadingPlayer; player != nil {
		player.Stop()
		player.ReleaseSurface()
		if a.player == player {
			a.player = NewPlayer()
		}
		if a.playerWindowRequest == requestID {
			a.destroyPlayerWindow()
		}
	}
	a.clearPlaybackLoadingState()
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
			a.sizePlayerWindowForVideo(a.playerWindowRequest, snapshot)
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
				server, failedItem, position, duration, progress := a.closePlayer(true)
				a.playback = PlaybackState{Active: true, Title: item.Title, Error: snapshot.Error}
				a.syncWindowTitle()
				if a.playerWindow != nil {
					a.playerWindow.Invalidate()
				}
				a.finishPlaybackProgress(server, failedItem, position, duration, progress)
			} else {
				// A clean mpv exit (including natural completion) returns to the
				// library instead of leaving a frozen video surface behind.
				a.stopPlayback()
			}
		}
	})
}

func (a *appState) stopPlayback() {
	server, item, position, duration, progress := a.closePlayer(false)
	a.finishPlaybackProgress(server, item, position, duration, progress)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) finishPlaybackProgress(server *Server, item *MediaItem, position, duration float64, progress *playbackProgressSession) {
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
}

func (a *appState) stopPlaybackAndWait(ctx context.Context) {
	server, item, position, duration, progress := a.closePlayer(false)
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

func (a *appState) closePlayer(keepWindow bool) (*Server, *MediaItem, float64, float64, *playbackProgressSession) {
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
		if window := a.playbackWindow(); window != nil {
			a.player.SetCaptionButtonCutout(false, window.NativeHandle())
		}
	}
	a.player.Stop()
	a.player.ReleaseSurface()
	a.playerProxy.Close()
	a.playerProxy = nil
	a.playingItem = nil
	a.playback = PlaybackState{}
	a.syncWindowTitle()
	if !keepWindow {
		a.destroyPlayerWindow()
	}
	return server, item, position, duration, progress
}
