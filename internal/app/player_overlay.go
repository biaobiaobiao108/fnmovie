package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	overlay "github.com/egoist/mygo/fnmovieoverlay"
	"github.com/egoist/mygo/ui"
)

const playerOverlayHideDelay = 2500 * time.Millisecond
const (
	playerOverlayHeaderHeight = 64
	// Leave room above the transport panel for its complete rounded border.
	playerOverlayControlHeight = 118
	playerOverlayFadeDuration  = 180 * time.Millisecond
	playerOverlayTargetOpacity = 1.0
)

var playerOverlayIcons = map[string]*ui.SVG{
	"back":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>`)),
	"play":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="M7 4.8c0-1.4 1.5-2.2 2.7-1.5l11.5 6.7c1.2.7 1.2 2.5 0 3.2L9.7 20c-1.2.7-2.7-.2-2.7-1.5V4.8z"/></svg>`)),
	"pause":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16" rx="2"/><rect x="14" y="4" width="4" height="16" rx="2"/></svg>`)),
	"volume":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5L6 9H3v6h3l5 4V5z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.5 5.5a9 9 0 0 1 0 13"/></svg>`)),
	"muted":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5L6 9H3v6h3l5 4V5z"/><line x1="22" y1="9" x2="16" y2="15"/><line x1="16" y1="9" x2="22" y2="15"/></svg>`)),
	"fullscreen":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6"/><path d="M9 21H3v-6"/><path d="M21 3l-7 7"/><path d="M3 21l7-7"/></svg>`)),
	"window":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 14h6v6"/><path d="M20 10h-6V4"/><path d="M10 14L3 21"/><path d="M14 10l7-7"/></svg>`)),
	"seek-back-10":    ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4.5A8.5 8.5 0 1 0 20.5 13"/><path d="M12 1.5l-3 3 3 3"/><text x="12" y="16.5" font-size="8" font-weight="700" text-anchor="middle" fill="currentColor" stroke="none" font-family="-apple-system,system-ui,sans-serif">10</text></svg>`)),
	"seek-forward-10": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4.5A8.5 8.5 0 1 1 3.5 13"/><path d="M12 1.5l3 3-3 3"/><text x="12" y="16.5" font-size="8" font-weight="700" text-anchor="middle" fill="currentColor" stroke="none" font-family="-apple-system,system-ui,sans-serif">10</text></svg>`)),
	"subtitles":       ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="3"/><path d="M7 15h2a2 2 0 0 0 2-2v-2a2 2 0 0 0-2-2H7"/><path d="M13 15h2a2 2 0 0 0 2-2v-2a2 2 0 0 0-2-2h-2"/></svg>`)),
	"audio":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 10v4"/><path d="M7 6v12"/><path d="M11 3v18"/><path d="M15 8v8"/><path d="M19 5v14"/><path d="M22 10v4"/></svg>`)),
}

func (a *appState) createPlayerOverlay() {
	if a.window == nil || a.playerOverlayWindow != nil {
		return
	}
	bounds := a.window.ContentBounds()
	a.playerOverlayContent = overlay.View(a.playerOverlayView)
	a.playerHeaderContent = overlay.View(a.playerHeaderView)
	controlHeight := min(a.playerOverlayContentHeight(), bounds.Height)
	headerHeight := min(playerOverlayHeaderHeight, bounds.Height-controlHeight)
	a.playerOverlayWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: "播放控件", X: bounds.X, Y: bounds.Y + bounds.Height - controlHeight, Width: bounds.Width, Height: controlHeight,
		Parent: a.window, Frameless: true, SkipTaskbar: true, Transparent: true, Hidden: true, DisableShadow: true,
		BackgroundColor: "#00000000", DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		Content: a.playerOverlayContent,
	})
	a.playerHeaderWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: "播放信息", X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: headerHeight,
		Parent: a.window, Frameless: true, SkipTaskbar: true, Transparent: true, Hidden: true, DisableShadow: true,
		BackgroundColor: "#00000000", DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		Content: a.playerHeaderContent,
	})
	a.registerPlayerOverlayHooks()
	a.syncPlayerOverlay()
	a.overlayMu.Lock()
	a.playerOverlayVisible = true
	a.playerOverlayLastInput = time.Now()
	a.playerOverlayOpacity = 0
	a.overlayMu.Unlock()
	a.playerOverlayWindow.Show()
	a.playerHeaderWindow.Show()
	a.animatePlayerOverlay(playerOverlayTargetOpacity)
}

func (a *appState) registerPlayerOverlayHooks() {
	if a.playerOverlayHooks || a.window == nil {
		return
	}
	a.playerOverlayHooks = true
	a.window.OnResize(a.syncPlayerOverlay)
	a.window.OnMove(a.syncPlayerOverlay)
	a.window.OnMinimize(func() { a.hidePlayerOverlay(false) })
	a.window.OnRestore(func() {
		a.syncPlayerOverlay()
		if a.playback.Active {
			a.showPlayerOverlay()
		}
	})
}

func (a *appState) syncPlayerOverlay() {
	if a.window == nil || a.playerOverlayWindow == nil {
		return
	}
	bounds := a.window.ContentBounds()
	controlBounds := bounds
	controlBounds.Height = min(a.playerOverlayContentHeight(), bounds.Height)
	controlBounds.Y += bounds.Height - controlBounds.Height
	headerBounds := bounds
	headerBounds.Height = min(playerOverlayHeaderHeight, bounds.Height-controlBounds.Height)
	a.playerOverlayWindow.SetContentBounds(controlBounds)
	a.playerHeaderWindow.SetContentBounds(headerBounds)
	a.playerOverlayWindow.Invalidate()
	a.playerHeaderWindow.Invalidate()
	if a.player != nil {
		width, height := a.window.ContentBounds().Width, a.window.ContentBounds().Height
		a.player.SetViewport(ui.Rect{W: float32(width), H: float32(height)})
	}
}

func (a *appState) playerOverlayContentHeight() int {
	if a.playerOverlayMenu == "" {
		return playerOverlayControlHeight
	}
	rows := 7
	switch a.playerOverlayMenu {
	case "audio":
		rows = len(a.playerMenuTracks)
	case "subtitle":
		rows = len(a.playerMenuTracks) + 1
	case "speed":
		rows = 7
	}
	menuHeight := min(330, max(90, 36+rows*34))
	height := playerOverlayControlHeight + 118 + menuHeight
	if a.window != nil {
		height = min(height, a.window.ContentBounds().Height)
	}
	return height
}

func (a *appState) startPlayerOverlayMonitor() {
	if a.playerOverlayMonitorDone != nil {
		close(a.playerOverlayMonitorDone)
	}
	done := make(chan struct{})
	a.playerOverlayMonitorDone = done
	activity := a.player.PointerActivity()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-activity:
				if a.window != nil {
					a.window.Update(func() {
						select {
						case <-done:
							return
						default:
						}
						a.showPlayerOverlay()
					})
				}
			case now := <-ticker.C:
				a.overlayMu.Lock()
				hide := a.playerOverlayVisible && !a.playerOverlayPinned && !a.playerOverlaySeeking && now.Sub(a.playerOverlayLastInput) >= playerOverlayHideDelay
				a.overlayMu.Unlock()
				if hide && a.window != nil {
					a.window.Update(func() {
						select {
						case <-done:
							return
						default:
						}
						// Pointer input may have arrived since this callback was
						// queued. Do not hide controls based on stale idle state.
						a.overlayMu.Lock()
						hide := a.playerOverlayVisible && !a.playerOverlayPinned && !a.playerOverlaySeeking && time.Since(a.playerOverlayLastInput) >= playerOverlayHideDelay
						a.overlayMu.Unlock()
						if hide {
							a.hidePlayerOverlay(true)
						}
					})
				}
			}
		}
	}()
}

func (a *appState) showPlayerOverlay() {
	if a.playerOverlayWindow == nil || a.window == nil || a.window.IsMinimized() {
		return
	}
	a.syncPlayerOverlay()
	a.overlayMu.Lock()
	wasVisible := a.playerOverlayVisible
	a.playerOverlayVisible = true
	a.playerOverlayLastInput = time.Now()
	a.overlayMu.Unlock()
	if !wasVisible {
		a.playerOverlayWindow.Show()
		a.playerHeaderWindow.Show()
		a.animatePlayerOverlay(playerOverlayTargetOpacity)
	} else {
		a.playerOverlayWindow.Invalidate()
	}
}

func (a *appState) hidePlayerOverlay(returnFocus bool) {
	if a.playerOverlayWindow == nil || !a.playerOverlayVisible {
		return
	}
	a.overlayMu.Lock()
	a.playerOverlayVisible = false
	a.overlayMu.Unlock()
	if returnFocus {
		a.animatePlayerOverlay(0)
	} else {
		a.overlayMu.Lock()
		a.playerOverlayTransition++
		a.playerOverlayAnimationStart = time.Time{}
		a.overlayMu.Unlock()
		a.playerOverlayWindow.Hide()
		a.playerHeaderWindow.Hide()
		a.setPlayerOverlayOpacity(0)
		a.overlayMu.Lock()
		a.playerOverlayOpacity = 0
		a.overlayMu.Unlock()
	}
}

func (a *appState) animatePlayerOverlay(target float64) {
	a.overlayMu.Lock()
	a.playerOverlayTransition++
	a.playerOverlayAnimationStart = time.Now()
	a.playerOverlayAnimationFrom = a.playerOverlayOpacity
	a.playerOverlayAnimationTarget = target
	a.overlayMu.Unlock()
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Invalidate()
	}
	if a.playerHeaderWindow != nil {
		a.playerHeaderWindow.Invalidate()
	}
}

func (a *appState) setPlayerOverlayOpacity(alpha float64) {
	if a.playerOverlayContent != nil && a.playerOverlayWindow != nil {
		a.playerOverlayContent.SetOpacity(a.playerOverlayWindow, alpha)
	}
	if a.playerHeaderContent != nil && a.playerHeaderWindow != nil {
		a.playerHeaderContent.SetOpacity(a.playerHeaderWindow, alpha)
	}
}

func (a *appState) advancePlayerOverlayAnimation(c *ui.Context) {
	a.overlayMu.Lock()
	started := a.playerOverlayAnimationStart
	if started.IsZero() {
		a.overlayMu.Unlock()
		return
	}
	progress := max(0, min(1, float64(c.Now().Sub(started))/float64(playerOverlayFadeDuration)))
	eased := progress * progress * (3 - 2*progress)
	alpha := a.playerOverlayAnimationFrom + (a.playerOverlayAnimationTarget-a.playerOverlayAnimationFrom)*eased
	a.playerOverlayOpacity = alpha
	hide := progress >= 1 && a.playerOverlayAnimationTarget == 0
	transition := a.playerOverlayTransition
	if progress >= 1 {
		a.playerOverlayAnimationStart = time.Time{}
	}
	a.overlayMu.Unlock()
	a.setPlayerOverlayOpacity(alpha)
	if progress < 1 {
		c.AnimationFrame()
	}
	if hide && a.window != nil {
		a.window.Update(func() {
			if transition != a.playerOverlayTransition || a.playerOverlayVisible || a.playerOverlayWindow == nil {
				return
			}
			a.playerOverlayWindow.Hide()
			a.playerHeaderWindow.Hide()
			restoreWindowFocus(a.window)
		})
	}
}

func (a *appState) closePlayerOverlay() {
	if a.playerOverlayMonitorDone != nil {
		close(a.playerOverlayMonitorDone)
		a.playerOverlayMonitorDone = nil
	}
	if a.window != nil {
		restoreWindowFocus(a.window)
	}
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Destroy()
		a.playerOverlayWindow = nil
	}
	if a.playerHeaderWindow != nil {
		a.playerHeaderWindow.Destroy()
		a.playerHeaderWindow = nil
	}
	if a.window != nil {
		restoreWindowFocus(a.window)
	}
	a.overlayMu.Lock()
	a.playerOverlayVisible, a.playerOverlayPinned, a.playerOverlaySeeking = false, false, false
	a.playerOverlayTransition++
	a.playerOverlayAnimationStart = time.Time{}
	a.overlayMu.Unlock()
	a.playerOverlayContent, a.playerHeaderContent = nil, nil
	a.playerOverlayMenu = ""
	a.seekDragging = false
	a.volumeDragging = false
}

func (a *appState) markPlayerOverlayActivity() {
	a.overlayMu.Lock()
	a.playerOverlayLastInput = time.Now()
	a.overlayMu.Unlock()
}

func (a *appState) setPlayerOverlayPinned(pinned bool) {
	a.overlayMu.Lock()
	a.playerOverlayPinned = pinned
	a.playerOverlayLastInput = time.Now()
	a.overlayMu.Unlock()
}

func (a *appState) setPlayerOverlaySeeking(seeking bool) {
	a.overlayMu.Lock()
	a.playerOverlaySeeking = seeking
	if seeking {
		a.playerOverlayLastInput = time.Now()
	}
	a.overlayMu.Unlock()
}

func (a *appState) playerOverlayView(c *ui.Context) {
	a.advancePlayerOverlayAnimation(c)
	c.Root().Background(ui.Transparent)
	playerOverlayTheme := ui.DarkTheme()
	playerOverlayTheme.Background = ui.Transparent
	playerOverlayTheme.Surface = ui.Transparent
	playerOverlayTheme.SurfaceHover = ui.RGBA(255, 255, 255, 0.10)
	playerOverlayTheme.SurfacePressed = ui.RGBA(255, 255, 255, 0.16)
	c.SetTheme(playerOverlayTheme)
	a.playerShortcuts(c)
	a.playerTransport(c)
	a.playerPopover(c)
}

func (a *appState) playerHeaderView(c *ui.Context) {
	a.advancePlayerOverlayAnimation(c)
	c.Root().Background(ui.Transparent)
	playerOverlayTheme := ui.DarkTheme()
	playerOverlayTheme.Background = ui.Transparent
	playerOverlayTheme.Surface = ui.Transparent
	playerOverlayTheme.SurfaceHover = ui.RGBA(255, 255, 255, 0.12)
	playerOverlayTheme.SurfacePressed = ui.RGBA(255, 255, 255, 0.20)
	c.SetTheme(playerOverlayTheme)
	ui.Row(c).Absolute().Top(10).Left(20).Gap(12).AlignItems(ui.Center).Padding(5, 14, 5, 6).Radius(22).
		Background(ui.RGBA(18, 20, 24, 0.52)).Border(1, ui.RGBA(255, 255, 255, 0.18)).Children(func() {
		backBtn := ui.Button(c, "").Size(32, 32).Padding(0).Radius(16).BorderWidth(0).
			Background(ui.RGBA(255, 255, 255, 0.12)).TextColor(ui.RGB(255, 255, 255)).Label("返回影片库").
			Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
		if backBtn.Hovered() {
			backBtn.Background(ui.RGBA(255, 255, 255, 0.22))
		}
		backBtn.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(16, 16) })
		if backBtn.Clicked() {
			a.stopPlayback()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.playback.Title).FontSize(13).Bold().TextColor(ui.RGB(255, 255, 255)).SingleLine()
			quality := strings.TrimSpace(a.playback.Quality)
			if quality == "" {
				quality = "飞牛影视"
			}
			ui.Box(c).Padding(2, 6).Radius(4).Background(ui.RGBA(255, 255, 255, 0.14)).Children(func() {
				ui.Text(c, quality).FontSize(9).Bold().TextColor(ui.RGBA(255, 255, 255, 0.85)).SingleLine()
			})
		})
	})
}

func (a *appState) playerTransport(c *ui.Context) {
	maxPosition := maxFloat(1, a.playback.Duration)
	if !a.seekDragging {
		a.seekSliderPosition = minFloat(maxPosition, maxFloat(0, a.playback.Position))
	}
	ui.Column(c).Label("播放控制栏").Absolute().Bottom(16).Left(32).Right(32).Padding(8, 18, 10, 18).Gap(4).Radius(22).
		Background(ui.RGBA(18, 20, 24, 0.52)).Border(1, ui.RGBA(255, 255, 255, 0.18)).Children(func() {
		// 上层：Apple 细致时间轨与时间标签
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, formatClock(a.seekDisplayPosition())).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.75))
			slider := ui.Slider(c, &a.seekSliderPosition, 0, maxPosition).Grow(1)
			pressed := slider.Pressed()
			slider.Changed()
			if pressed && !a.seekDragging {
				a.seekDragging = true
				a.setPlayerOverlaySeeking(a.seekDragging || a.volumeDragging)
			}
			if !pressed && a.seekDragging {
				a.seekDragging = false
				a.setPlayerOverlaySeeking(a.seekDragging || a.volumeDragging)
				a.player.SeekTo(a.seekSliderPosition)
				a.markPlayerOverlayActivity()
			}
			ui.Text(c, formatClock(a.playback.Duration)).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.75))
		})
		// 下层：Apple 经典居中核心对称控制区
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			// 左侧：音量区域
			ui.Row(c).Grow(1).Basis(0).Gap(6).AlignItems(ui.Center).Children(func() {
				volumeIcon := "volume"
				if a.playback.Muted || a.playback.Volume <= 0 {
					volumeIcon = "muted"
				}
				if playerAppleIconButton(c, volumeIcon, "静音", 32, 17).Clicked() {
					a.player.ToggleMute()
					a.playback.Muted = !a.playback.Muted
					a.markPlayerOverlayActivity()
				}
				volume := ui.Slider(c, &a.playback.Volume, 0, 100).Width(82)
				volumePressed := volume.Pressed()
				if volume.Changed() {
					a.player.SetVolume(a.playback.Volume)
					a.markPlayerOverlayActivity()
				}
				if volumePressed {
					a.volumeDragging = true
					a.setPlayerOverlaySeeking(a.seekDragging || a.volumeDragging)
				} else if a.volumeDragging {
					a.volumeDragging = false
					a.setPlayerOverlaySeeking(a.seekDragging || a.volumeDragging)
				}
			})

			// 中央核心：-10s、大号圆形高光播放/暂停键、+10s
			ui.Row(c).Grow(1).Basis(0).Gap(14).Center().Children(func() {
				if playerAppleIconButton(c, "seek-back-10", "快退 10 秒", 34, 20).Clicked() {
					a.player.Seek(-10)
					a.markPlayerOverlayActivity()
				}

				playIcon := "play"
				if !a.playback.Paused {
					playIcon = "pause"
				}
				playBtn := ui.Button(c, "").Size(42, 42).Padding(0).Radius(21).BorderWidth(0).
					Background(ui.RGBA(255, 255, 255, 0.95)).TextColor(ui.RGB(18, 20, 24)).Label("播放或暂停").
					Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
				if playBtn.Hovered() {
					playBtn.Background(ui.RGB(255, 255, 255))
				}
				playBtn.Children(func() {
					iconSize := float32(18)
					if playIcon == "pause" {
						iconSize = 16
					}
					ui.Icon(c, playerOverlayIcons[playIcon]).Size(iconSize, iconSize)
				})
				if playBtn.Clicked() {
					a.player.TogglePause()
					a.playback.Paused = !a.playback.Paused
					a.markPlayerOverlayActivity()
				}

				if playerAppleIconButton(c, "seek-forward-10", "快进 10 秒", 34, 20).Clicked() {
					a.player.Seek(10)
					a.markPlayerOverlayActivity()
				}
			})

			// 右侧功能胶囊：倍速、字幕、音轨、全屏
			ui.Row(c).Grow(1).Basis(0).Gap(7).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				a.playerOptionButton(c, "speed")
				a.playerOptionButton(c, "subtitle")
				a.playerOptionButton(c, "audio")
				fullscreenIcon := "fullscreen"
				if a.window != nil && a.window.IsFullScreen() {
					fullscreenIcon = "window"
				}
				if playerAppleIconButton(c, fullscreenIcon, "切换全屏", 32, 17).Clicked() {
					if a.window != nil {
						a.window.ToggleFullScreen()
						a.syncPlayerOverlay()
					}
					a.markPlayerOverlayActivity()
				}
			})
		})
	})
}

func (a *appState) playerOptionButton(c *ui.Context, option string) {
	label, disabled, active := "", false, a.playerOverlayMenu == option
	iconName := ""
	accessibleLabel := ""
	switch option {
	case "speed":
		label = fmt.Sprintf("%.2g×", maxFloat(0.25, a.playback.Speed))
		accessibleLabel = "播放速度"
	case "subtitle":
		disabled = len(a.playback.SubtitleTracks) == 0
		iconName = "subtitles"
		accessibleLabel = "字幕"
	case "audio":
		disabled = len(a.playback.AudioTracks) == 0
		iconName = "audio"
		accessibleLabel = "音轨"
	}

	var btn *ui.Element
	if iconName != "" {
		btn = ui.Button(c, "").Size(32, 32).Padding(0).Radius(16).BorderWidth(0).
			Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(255, 255, 255, 0.9)).
			Label(accessibleLabel).Disabled(disabled).
			Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
		if active {
			btn.Background(ui.RGBA(255, 255, 255, 0.24)).Border(1, ui.RGBA(255, 255, 255, 0.38))
		} else if btn.Hovered() && !disabled {
			btn.Background(ui.RGBA(255, 255, 255, 0.18))
		}
		btn.Children(func() {
			ui.Icon(c, playerOverlayIcons[iconName]).Size(17, 17)
		})
	} else {
		btn = ui.Button(c, "").Height(32).Padding(0, 10).Radius(16).Border(1, ui.RGBA(255, 255, 255, 0.12)).
			Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(255, 255, 255, 0.9)).
			Label(accessibleLabel).Disabled(disabled).
			Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
		if active {
			btn.Background(ui.RGBA(255, 255, 255, 0.24)).Border(1, ui.RGBA(255, 255, 255, 0.38))
		} else if btn.Hovered() && !disabled {
			btn.Background(ui.RGBA(255, 255, 255, 0.18))
		}
		btn.Children(func() {
			ui.Text(c, label).FontSize(12).Bold().TextColor(ui.RGBA(255, 255, 255, 0.92)).SingleLine()
		})
	}

	if btn.Clicked() {
		if a.playerOverlayMenu == option {
			a.closePlayerOverlayMenu()
		} else {
			a.playerOverlayMenu = option
			if option == "subtitle" {
				a.playerMenuTracks = append(a.playerMenuTracks[:0], a.playback.SubtitleTracks...)
			} else if option == "audio" {
				a.playerMenuTracks = append(a.playerMenuTracks[:0], a.playback.AudioTracks...)
			} else {
				a.playerMenuTracks = nil
			}
			a.setPlayerOverlayPinned(true)
		}
		a.syncPlayerOverlay()
		a.markPlayerOverlayActivity()
	}
}

func (a *appState) playerPopover(c *ui.Context) {
	if a.playerOverlayMenu == "" {
		return
	}
	rows := 7
	if a.playerOverlayMenu == "audio" {
		rows = len(a.playerMenuTracks)
	} else if a.playerOverlayMenu == "subtitle" {
		rows = len(a.playerMenuTracks) + 1
	}
	menuHeight := min(330, max(90, 36+rows*34))
	ui.Column(c).Absolute().Bottom(118).Right(20).Width(300).Height(float32(menuHeight)).Padding(8).Gap(3).Radius(18).
		Background(ui.RGBA(20, 22, 26, 0.88)).Border(1, ui.RGBA(255, 255, 255, 0.16)).Children(func() {
		var title string
		switch a.playerOverlayMenu {
		case "subtitle":
			title = "字幕选择"
		case "audio":
			title = "音轨选择"
		case "speed":
			title = "播放速度"
		}
		ui.Text(c, title).Padding(4, 9).FontSize(11).Bold().TextColor(ui.RGBA(255, 255, 255, 0.65))
		ui.Scroll(c).Height(float32(menuHeight - 38)).Children(func() {
			switch a.playerOverlayMenu {
			case "subtitle":
				selected := true
				for _, track := range a.playerMenuTracks {
					selected = selected && !track.Selected
				}
				a.playerTrackMenuItem(c, "关闭字幕", 0, selected, true)
				for _, track := range a.playerMenuTracks {
					a.playerTrackMenuItem(c, playerTrackLabel(track), track.ID, track.Selected, true)
				}
			case "audio":
				for _, track := range a.playerMenuTracks {
					a.playerTrackMenuItem(c, playerTrackLabel(track), track.ID, track.Selected, false)
				}
			case "speed":
				for _, speed := range []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2} {
					label := fmt.Sprintf("%.2g×", speed)
					if playerAppleMenuItem(c, label, absFloat(a.playback.Speed-speed) < 0.01) {
						a.player.SetSpeed(speed)
						a.playback.Speed = speed
						a.closePlayerOverlayMenu()
					}
				}
			}
		})
	})
}

func (a *appState) playerTrackMenuItem(c *ui.Context, label string, trackID int, selected, subtitle bool) {
	if playerAppleMenuItem(c, label, selected) {
		kind := "audio"
		if subtitle {
			kind = "subtitle"
		}
		a.player.SelectTrack(kind, trackID)
		a.closePlayerOverlayMenu()
	}
}

func playerAppleMenuItem(c *ui.Context, label string, selected bool) bool {
	btn := ui.Button(c, "").Padding(6, 10).Radius(9).BorderWidth(0).
		Background(ui.RGBA(0, 0, 0, 0)).FillWidth().
		Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	txtColor := ui.RGBA(255, 255, 255, 0.88)
	if selected {
		btn.Background(ui.RGBA(255, 255, 255, 0.16))
		txtColor = ui.RGB(255, 255, 255)
	} else if btn.Hovered() {
		btn.Background(ui.RGBA(255, 255, 255, 0.10))
	}
	prefix := "   "
	if selected {
		prefix = "✓  "
	}
	btn.Children(func() {
		ui.Text(c, prefix+label).FontSize(12).TextColor(txtColor).SingleLine()
	})
	return btn.Clicked()
}

func playerAppleIconButton(c *ui.Context, name, label string, size, iconSize float32) *ui.Element {
	b := ui.Button(c, "").Size(size, size).Padding(0).Radius(size / 2).BorderWidth(0).
		Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(255, 255, 255, 0.9)).Label(label).
		Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	if b.Hovered() {
		b.Background(ui.RGBA(255, 255, 255, 0.18))
	}
	b.Children(func() { ui.Icon(c, playerOverlayIcons[name]).Size(iconSize, iconSize) })
	return b
}
func playerTrackLabel(track PlayerTrack) string {
	parts := make([]string, 0, 3)
	if language := strings.TrimSpace(track.Language); language != "" {
		parts = append(parts, language)
	}
	if title := strings.TrimSpace(track.Title); title != "" && title != strings.TrimSpace(track.Language) {
		parts = append(parts, title)
	}
	parts = append(parts, fmt.Sprintf("轨道 %d", track.ID))
	label := strings.Join(parts, " · ")
	if track.External {
		label += " · 外挂"
	}
	return label
}

func (a *appState) closePlayerOverlayMenu() {
	a.playerOverlayMenu = ""
	a.playerMenuTracks = nil
	a.syncPlayerOverlay()
	a.setPlayerOverlayPinned(false)
	a.markPlayerOverlayActivity()
}

func (a *appState) seekDisplayPosition() float64 {
	if a.seekDragging {
		return a.seekSliderPosition
	}
	return a.playback.Position
}

func (a *appState) playerShortcuts(c *ui.Context) {
	keys := [...]ui.Key{ui.KeySpace, ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyM, ui.KeyF, ui.KeyEscape}
	fullscreen := a.window != nil && a.window.IsFullScreen()
	for _, key := range keys {
		if c.Shortcut(0, key) {
			if key == ui.KeyEscape && a.playerOverlayMenu != "" {
				a.closePlayerOverlayMenu()
				return
			}
			a.performPlayerAction(playerShortcutAction(0, key, fullscreen))
			return
		}
	}
}

type playerAction string

const (
	playerTogglePause    playerAction = "pause"
	playerSeekBack       playerAction = "seek-back"
	playerSeekForward    playerAction = "seek-forward"
	playerVolumeUp       playerAction = "volume-up"
	playerVolumeDown     playerAction = "volume-down"
	playerToggleMute     playerAction = "mute"
	playerToggleFull     playerAction = "fullscreen"
	playerExitFull       playerAction = "exit-fullscreen"
	playerReturn         playerAction = "return"
	playerShortcutIgnore playerAction = ""
)

func playerShortcutAction(mods ui.Modifiers, key ui.Key, fullscreen bool) playerAction {
	if mods != 0 {
		return playerShortcutIgnore
	}
	switch key {
	case ui.KeySpace:
		return playerTogglePause
	case ui.KeyLeft:
		return playerSeekBack
	case ui.KeyRight:
		return playerSeekForward
	case ui.KeyUp:
		return playerVolumeUp
	case ui.KeyDown:
		return playerVolumeDown
	case ui.KeyM:
		return playerToggleMute
	case ui.KeyF:
		return playerToggleFull
	case ui.KeyEscape:
		if fullscreen {
			return playerExitFull
		}
		return playerReturn
	default:
		return playerShortcutIgnore
	}
}

func (a *appState) performPlayerAction(action playerAction) {
	switch action {
	case playerTogglePause:
		a.player.TogglePause()
		a.playback.Paused = !a.playback.Paused
	case playerSeekBack:
		a.player.Seek(-10)
	case playerSeekForward:
		a.player.Seek(10)
	case playerVolumeUp:
		a.player.AdjustVolume(5)
		a.playback.Volume = minFloat(100, a.playback.Volume+5)
	case playerVolumeDown:
		a.player.AdjustVolume(-5)
		a.playback.Volume = maxFloat(0, a.playback.Volume-5)
	case playerToggleMute:
		a.player.ToggleMute()
		a.playback.Muted = !a.playback.Muted
	case playerToggleFull:
		if a.window != nil {
			a.window.ToggleFullScreen()
			a.syncPlayerOverlay()
		}
	case playerExitFull:
		if a.window != nil {
			a.window.SetFullScreen(false)
			a.syncPlayerOverlay()
		}
	case playerReturn:
		a.stopPlayback()
	}
	if action != playerShortcutIgnore {
		a.markPlayerOverlayActivity()
	}
}

func playerIconButton(c *ui.Context, name, label string) *ui.Element {
	b := ui.Button(c, "").Size(38, 36).Padding(0).Radius(9).BorderWidth(0).
		Background(ui.RGBA(0, 0, 0, 0)).TextColor(ui.RGB(255, 255, 255)).Label(label).
		Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	if b.Hovered() {
		b.Background(ui.RGBA(255, 255, 255, 0.14))
	}
	b.Children(func() { ui.Icon(c, playerOverlayIcons[name]).Size(19, 19) })
	return b
}

func playerTextButton(c *ui.Context, label string) *ui.Element {
	b := ui.Button(c, "").Padding(9, 10).Radius(8).BorderWidth(0).
		Background(ui.RGBA(0, 0, 0, 0)).TextColor(ui.RGBA(255, 255, 255, 0.9)).
		Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	if b.Hovered() {
		b.Background(ui.RGBA(255, 255, 255, 0.13))
	}
	b.Children(func() { ui.Text(c, label).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.92)).SingleLine() })
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
