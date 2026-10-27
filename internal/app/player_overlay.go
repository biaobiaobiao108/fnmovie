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
	// Keep the progress and transport comfortably inside the transparent overlay.
	playerOverlayControlHeight = 118
	playerOverlayFadeDuration  = 180 * time.Millisecond
	playerOverlayTargetOpacity = 1.0
	playerSeekFeedbackWidth    = 180
	playerSeekFeedbackHeight   = 56
	playerMenuWidth            = 300
	playerMenuHeight           = 330
)

var playerOverlayIcons = map[string]*ui.SVG{
	"back":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>`)),
	"play":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="M7 4.8c0-1.4 1.5-2.2 2.7-1.5l11.5 6.7c1.2.7 1.2 2.5 0 3.2L9.7 20c-1.2.7-2.7-.2-2.7-1.5V4.8z"/></svg>`)),
	"pause":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><rect x="5.5" y="4" width="4.5" height="16" rx="1.4"/><rect x="14" y="4" width="4.5" height="16" rx="1.4"/></svg>`)),
	"volume":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H3v6h3l5 4V5z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.5 5.5a9 9 0 0 1 0 13"/></svg>`)),
	"volume-low":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H3v6h3l5 4V5z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/></svg>`)),
	"muted":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H3v6h3l5 4V5z"/><path d="m16 9 6 6m0-6-6 6"/></svg>`)),
	"fullscreen":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6"/><path d="M9 21H3v-6"/><path d="M21 3l-7 7"/><path d="M3 21l7-7"/></svg>`)),
	"window":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 14h6v6"/><path d="M20 10h-6V4"/><path d="M10 14L3 21"/><path d="M14 10l7-7"/></svg>`)),
	"seek-back-10":    ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4.5A8.5 8.5 0 1 0 20.5 13"/><path d="M12 1.5l-3 3 3 3"/></svg>`)),
	"seek-forward-10": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4.5A8.5 8.5 0 1 1 3.5 13"/><path d="M12 1.5l3 3-3 3"/></svg>`)),
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
	a.playerSeekFeedbackContent = overlay.View(a.playerSeekFeedbackView)
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
	a.playerSeekFeedbackWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: "跳转状态", X: bounds.X, Y: bounds.Y, Width: playerSeekFeedbackWidth, Height: playerSeekFeedbackHeight,
		Parent: a.window, Frameless: true, SkipTaskbar: true, Transparent: true, Hidden: true, DisableShadow: true,
		BackgroundColor: "#00000000", DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		Content: a.playerSeekFeedbackContent,
	})
	overlay.SetIgnoreMouseEvents(a.playerSeekFeedbackWindow, true)
	a.registerPlayerOverlayHooks()
	a.syncPlayerOverlay()
	a.overlayMu.Lock()
	a.playerOverlayVisible = true
	a.playerOverlayLastInput = time.Now()
	a.playerOverlayOpacity = 0
	a.overlayMu.Unlock()
	a.playerOverlayWindow.ShowInactive()
	a.playerHeaderWindow.ShowInactive()
	a.syncPlayerOverlay()
	a.animatePlayerOverlay(playerOverlayTargetOpacity)
}

func (a *appState) registerPlayerOverlayHooks() {
	if a.playerOverlayHooks || a.window == nil {
		return
	}
	a.playerOverlayHooks = true
	a.window.OnResize(a.syncPlayerOverlay)
	a.window.OnMove(a.syncPlayerOverlay)
	a.window.OnMinimize(func() {
		a.cancelPlayerFullscreen()
		if a.player != nil {
			a.player.HideSurface()
		}
		a.hidePlayerOverlay(false)
		a.syncPlayerSeekFeedbackVisibility()
	})
	a.window.OnRestore(func() {
		a.syncPlayerOverlay()
		if a.playback.Active {
			if a.player != nil {
				a.player.ShowSurface()
			}
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
	// Playback progress and pointer activity do not change window geometry.
	// Leave native surfaces alone unless their actual bounds need to change.
	if a.playerOverlayWindow.ContentBounds() != controlBounds {
		a.playerOverlayWindow.SetContentBounds(controlBounds)
	}
	if a.playerHeaderWindow.ContentBounds() != headerBounds {
		a.playerHeaderWindow.SetContentBounds(headerBounds)
	}
	if a.playerSeekFeedbackWindow != nil {
		feedbackBounds := playerSeekFeedbackBounds(bounds)
		if a.playerSeekFeedbackWindow.ContentBounds() != feedbackBounds {
			a.playerSeekFeedbackWindow.SetContentBounds(feedbackBounds)
		}
		a.syncPlayerSeekFeedbackVisibility()
	}
	a.playerOverlayWindow.Invalidate()
	a.playerHeaderWindow.Invalidate()
	if a.playerMenuWindow != nil {
		if menuBounds := playerMenuBounds(bounds); a.playerMenuWindow.ContentBounds() != menuBounds {
			a.playerMenuWindow.SetContentBounds(menuBounds)
		}
		a.playerMenuWindow.Invalidate()
	}
	if a.player != nil {
		width, height := a.window.ContentBounds().Width, a.window.ContentBounds().Height
		a.player.SetViewport(ui.Rect{W: float32(width), H: float32(height)})
		a.player.SetWindowButtonsVisible(!a.window.IsFullScreen(), a.window.NativeHandle())
	}
	if a.playerHeaderWindow != nil {
		setPlayerWindowButtonsCutout(a.playerHeaderWindow.NativeHandle(), a.window.NativeHandle(), !a.window.IsFullScreen())
	}
	if a.player != nil {
		video := a.player.surface
		for _, overlayWindow := range []*mygo.Window{a.playerHeaderWindow, a.playerOverlayWindow, a.playerMenuWindow, a.playerSeekFeedbackWindow} {
			if overlayWindow != nil {
				placePlayerOverlayAboveSurface(video, overlayWindow.NativeHandle())
			}
		}
	}
}

func playerSeekFeedbackBounds(bounds mygo.Rectangle) mygo.Rectangle {
	width := min(playerSeekFeedbackWidth, max(1, bounds.Width-24))
	height := min(playerSeekFeedbackHeight, max(1, bounds.Height-24))
	return mygo.Rectangle{
		X: bounds.X + (bounds.Width-width)/2, Y: bounds.Y + (bounds.Height-height)/2,
		Width: width, Height: height,
	}
}

func (a *appState) syncPlayerSeekFeedbackVisibility() {
	if a.playerSeekFeedbackWindow == nil || a.window == nil {
		return
	}
	visible := playerSeekFeedbackShouldShow(a.seekFeedback, a.window.IsMinimized())
	if visible == a.playerSeekFeedbackVisible {
		if visible {
			a.playerSeekFeedbackWindow.Invalidate()
		}
		return
	}
	a.playerSeekFeedbackVisible = visible
	if visible {
		if a.playerSeekFeedbackContent != nil {
			a.playerSeekFeedbackContent.SetOpacity(a.playerSeekFeedbackWindow, a.fullscreenFade())
		}
		a.playerSeekFeedbackWindow.ShowInactive()
		a.playerSeekFeedbackWindow.Invalidate()
	} else {
		a.playerSeekFeedbackWindow.Hide()
	}
}

func playerSeekFeedbackShouldShow(feedback seekFeedback, minimized bool) bool {
	return !minimized && (feedback.Pending || feedback.Error != "")
}

func (a *appState) playerOverlayContentHeight() int {
	// Opening a menu must never move or resize the transport surface. Windows
	// otherwise moves the last composited progress bar before its next paint.
	return playerOverlayControlHeight
}

func playerMenuBounds(bounds mygo.Rectangle) mygo.Rectangle {
	width := min(playerMenuWidth, max(1, bounds.Width-40))
	height := min(playerMenuHeight, max(1, bounds.Height-playerOverlayControlHeight-playerOverlayHeaderHeight))
	return mygo.Rectangle{X: bounds.X + max(0, bounds.Width-width-20), Y: bounds.Y + max(0, bounds.Height-playerOverlayControlHeight-height), Width: width, Height: height}
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
		a.playerOverlayWindow.ShowInactive()
		a.playerHeaderWindow.ShowInactive()
		if a.playerMenuWindow != nil {
			a.playerMenuWindow.ShowInactive()
		}
		a.syncPlayerOverlay()
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
		if a.playerMenuWindow != nil {
			a.playerMenuWindow.Hide()
		}
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
	// The main window also drives the fade when an owned surface is hidden or
	// temporarily awaiting its first frame after a native fullscreen change.
	if a.window != nil {
		a.window.Invalidate()
	}
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Invalidate()
	}
	if a.playerHeaderWindow != nil {
		a.playerHeaderWindow.Invalidate()
	}
}

func (a *appState) setPlayerOverlayOpacity(alpha float64) {
	alpha *= a.fullscreenFade()
	if a.playerOverlayContent != nil && a.playerOverlayWindow != nil {
		a.playerOverlayContent.SetOpacity(a.playerOverlayWindow, alpha)
	}
	if a.playerHeaderContent != nil && a.playerHeaderWindow != nil {
		a.playerHeaderContent.SetOpacity(a.playerHeaderWindow, alpha)
	}
	if a.playerMenuContent != nil && a.playerMenuWindow != nil {
		a.playerMenuContent.SetOpacity(a.playerMenuWindow, alpha)
	}
	if a.playerSeekFeedbackContent != nil && a.playerSeekFeedbackWindow != nil {
		a.playerSeekFeedbackContent.SetOpacity(a.playerSeekFeedbackWindow, a.fullscreenFade())
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
			// Idle fading must not reactivate the application while the user is
			// in another window. Restore shortcuts only after an owned control
			// window actually had keyboard focus.
			ownedFocus := a.playerOverlayHasFocus()
			a.playerOverlayWindow.Hide()
			a.playerHeaderWindow.Hide()
			if a.playerMenuWindow != nil {
				a.playerMenuWindow.Hide()
			}
			if ownedFocus {
				restoreWindowFocus(a.window)
			}
		})
	}
}

func (a *appState) closePlayerOverlay() {
	a.cancelPlayerFullscreen()
	ownedFocus := a.playerOverlayHasFocus()
	a.destroyPlayerMenuWindow()
	if a.playerOverlayMonitorDone != nil {
		close(a.playerOverlayMonitorDone)
		a.playerOverlayMonitorDone = nil
	}
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Destroy()
		a.playerOverlayWindow = nil
	}
	if a.playerHeaderWindow != nil {
		a.playerHeaderWindow.Destroy()
		a.playerHeaderWindow = nil
	}
	if a.playerSeekFeedbackWindow != nil {
		a.playerSeekFeedbackWindow.Destroy()
		a.playerSeekFeedbackWindow = nil
	}
	if ownedFocus && a.window != nil {
		restoreWindowFocus(a.window)
	}
	a.overlayMu.Lock()
	a.playerOverlayVisible, a.playerOverlayPinned, a.playerOverlaySeeking = false, false, false
	a.playerOverlayTransition++
	a.playerOverlayAnimationStart = time.Time{}
	a.overlayMu.Unlock()
	a.playerOverlayContent, a.playerHeaderContent, a.playerSeekFeedbackContent = nil, nil, nil
	a.playerSeekFeedbackVisible = false
	a.playerOverlayMenu = ""
	a.seekDragging = false
	a.seekFeedback = seekFeedback{}
	a.volumeDragging = false
}

func (a *appState) playerOverlayHasFocus() bool {
	if a.playerOverlayWindow != nil && a.playerOverlayWindow.IsFocused() {
		return true
	}
	if a.playerHeaderWindow != nil && a.playerHeaderWindow.IsFocused() {
		return true
	}
	return a.playerMenuWindow != nil && a.playerMenuWindow.IsFocused()
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

func (a *appState) updatePlayerVolumeDragging(pressed bool) {
	if a.volumeDragging == pressed {
		return
	}
	wasDragging := a.volumeDragging
	a.volumeDragging = pressed
	a.setPlayerOverlaySeeking(a.seekDragging || a.volumeDragging)
	if wasDragging && !pressed {
		a.markPlayerOverlayActivity()
	}
}

func (a *appState) playerOverlayView(c *ui.Context) {
	a.advancePlayerOverlayAnimation(c)
	c.Root().Background(ui.Transparent)
	c.SetTheme(playerOverlayTheme())
	a.playerShortcuts(c)
	a.playerTransport(c)
}

func playerOverlayTheme() *ui.Theme {
	theme := *ui.DarkTheme()
	accent := playerOverlayAccent()
	theme.Background = ui.Transparent
	theme.Surface = ui.Transparent
	theme.SurfaceHover = ui.RGBA(255, 255, 255, 0.10)
	theme.SurfacePressed = ui.RGBA(255, 255, 255, 0.16)
	theme.Accent = accent
	theme.AccentHover = accent.Mix(ui.RGB(255, 255, 255), 0.12)
	theme.AccentPressed = accent.Mix(ui.RGB(0, 0, 0), 0.12)
	theme.AccentText = ui.RGB(20, 24, 28)
	theme.Focus = accent.Alpha(0.72)
	theme.Selection = accent.Alpha(0.48)
	theme.TextMuted = ui.RGBA(255, 255, 255, 0.68)
	return &theme
}

func playerOverlayAccent() ui.Color {
	return movieColors.accent.Mix(ui.RGB(255, 255, 255), 0.38)
}

func (a *appState) playerSeekFeedbackView(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	theme := ui.DarkTheme()
	theme.Background, theme.Surface = ui.Transparent, ui.Transparent
	theme.TextMuted = ui.RGBA(255, 255, 255, 0.78)
	c.SetTheme(theme)
	if a.seekFeedback.Pending {
		ui.Row(c).Absolute().Fill().Center().Children(func() {
			ui.Spinner(c).Size(22, 22).Label("正在跳转")
		})
	} else if a.seekFeedback.Error != "" {
		ui.Row(c).Absolute().Fill().Center().Children(func() {
			ui.Text(c, a.seekFeedback.Error).FontSize(13).Bold().TextColor(ui.RGB(255, 255, 255)).SingleLine()
		})
	}
}

func (a *appState) playerMenuView(c *ui.Context) {
	a.advancePlayerOverlayAnimation(c)
	c.Root().Background(ui.Transparent)
	c.SetTheme(playerOverlayTheme())
	a.playerShortcuts(c)
	a.playerPopover(c)
}

func (a *appState) playerHeaderView(c *ui.Context) {
	a.advancePlayerOverlayAnimation(c)
	c.Root().Background(ui.Transparent)
	c.SetTheme(playerOverlayTheme())
	ui.Row(c).Absolute().Top(10).Left(20).Right(148).Height(40).Gap(10).AlignItems(ui.Center).DragWindow().Children(func() {
		playerOverlayIconButton(c, "back", "返回影片库", 38, 17).OnClick(a.stopPlayback)
		title := strings.TrimSpace(a.playback.Title)
		if title == "" {
			title = "正在播放"
		}
		ui.Text(c, title).Grow(1).MinWidth(0).FontSize(14.5).Bold().TextColor(ui.RGBA(255, 255, 255, 0.94)).SingleLine()
		if errorMessage := strings.TrimSpace(a.playback.Error); errorMessage != "" {
			ui.Text(c, errorMessage).Shrink(0).FontSize(11).TextColor(playerOverlayAccent()).SingleLine()
		}
	})
}

// Keep the constructor-applied edit until its change notice is handled,
// rather than restoring an old playback snapshot.
func (a *appState) playerSeekSlider(c *ui.Context, maxPosition float64, commit func(float64)) {
	slider := ui.Slider(c.Key("player-seek"), &a.seekSliderPosition, 0, maxPosition).Grow(1).Label("播放进度")
	pressed, changed := slider.Pressed(), slider.Changed()
	released := !pressed && a.seekDragging
	if !pressed && !changed && !a.seekDragging && !a.seekFeedback.Pending {
		a.seekSliderPosition = minFloat(maxPosition, maxFloat(0, a.playback.Position))
	}
	if pressed != a.seekDragging {
		a.seekDragging = pressed
		a.setPlayerOverlaySeeking(a.seekDragging || a.volumeDragging)
	}
	if released {
		commit(a.seekSliderPosition)
		a.markPlayerOverlayActivity()
	}
	slider.OnChange(func() {
		// Pointer edits remain local during a drag; keyboard edits commit once.
		if !pressed && !released {
			commit(a.seekSliderPosition)
		}
	})
}

func (a *appState) playerTransport(c *ui.Context) {
	c.SetTheme(playerOverlayTheme())
	maxPosition := maxFloat(1, a.playback.Duration)
	width, _ := c.Size()
	gutter := float32(max(16, (int(width)-1760)/2))
	ui.Box(c).Label("播放沉浸渐隐").Absolute().Fill().PassThrough().LinearGradient(ui.LinearGradient{
		Angle: 180,
		Stops: []ui.GradientStop{
			{Color: ui.RGBA(0, 0, 0, 0), At: 0},
			{Color: ui.RGBA(0, 0, 0, 0.04), At: 0.30},
			{Color: ui.RGBA(0, 0, 0, 0.32), At: 0.68},
			{Color: ui.RGBA(0, 0, 0, 0.68), At: 1},
		}})
	ui.Column(c).Label("播放控件区域").Absolute().Bottom(12).Left(gutter).Right(gutter).Gap(7).Children(func() {
		// The progress line sits above the information row, as on MyGo Music's
		// immersive player. Time labels stay at its ends while the slider fills
		// the space between them.
		ui.Row(c).Label("播放时间轴").AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, formatClock(a.seekDisplayPosition())).Width(54).Shrink(0).TextAlign(ui.End).FontSize(10.5).FontFeatures("tnum").TextColor(ui.RGBA(255, 255, 255, 0.72))
			a.playerSeekSlider(c, maxPosition, a.requestPlayerSeek)
			ui.Text(c, formatClock(a.playback.Duration)).Width(54).Shrink(0).TextAlign(ui.Start).FontSize(10.5).FontFeatures("tnum").TextColor(ui.RGBA(255, 255, 255, 0.54))
		})

		// Three equal columns keep the transport centered regardless of the
		// width of the track and volume controls.
		ui.Row(c).Label("播放控制栏").Height(48).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Box(c).Grow(1).Basis(0).MinWidth(0)

			ui.Row(c).Label("居中播放操作").Grow(1).Basis(0).Gap(10).Center().Children(func() {
				playerOverlayIconButton(c, "seek-back-10", "快退 10 秒", 42, 24).OnClick(func() {
					a.requestPlayerSeekRelative(-10)
					a.markPlayerOverlayActivity()
				})

				playBtn := playerOverlayButton(c, "播放或暂停", 48, ui.RGBA(255, 255, 255, 0.96))
				playBtn.Children(func() { playerOverlayPlayPause(c, playBtn, a.playback.Paused, 26) })
				playBtn.OnClick(func() {
					a.player.TogglePause()
					a.playback.Paused = !a.playback.Paused
					a.markPlayerOverlayActivity()
				})

				playerOverlayIconButton(c, "seek-forward-10", "快进 10 秒", 42, 24).OnClick(func() {
					a.requestPlayerSeekRelative(10)
					a.markPlayerOverlayActivity()
				})
			})

			ui.Row(c).Label("影片操作").Grow(1).Basis(0).Gap(3).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				volumeIcon := "volume"
				if a.playback.Muted || a.playback.Volume <= 0 {
					volumeIcon = "muted"
				} else if a.playback.Volume <= 35 {
					volumeIcon = "volume-low"
				}
				playerOverlayIconButton(c, volumeIcon, "静音", 34, 19).OnClick(func() {
					a.player.ToggleMute()
					a.playback.Muted = !a.playback.Muted
					a.markPlayerOverlayActivity()
				})
				volume := ui.Slider(c.Key("player-volume"), &a.playback.Volume, 0, 100).Width(56).Label("音量")
				volumePressed := volume.Pressed()
				volume.OnChange(func() {
					a.player.SetVolume(a.playback.Volume)
					a.markPlayerOverlayActivity()
				})
				a.updatePlayerVolumeDragging(volumePressed)

				a.playerOptionButton(c, "speed")
				a.playerOptionButton(c, "subtitle")
				a.playerOptionButton(c, "audio")
				fullscreenIcon := "fullscreen"
				if a.window != nil && a.window.IsFullScreen() {
					fullscreenIcon = "window"
				}
				reduceMotion := c.Preferences().ReduceMotion
				playerOverlayIconButton(c, fullscreenIcon, "切换全屏", 34, 19).OnClick(func() {
					if a.window != nil {
						a.togglePlayerFullscreen(reduceMotion)
					}
					a.markPlayerOverlayActivity()
				})
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

	var btn ui.Element
	if iconName != "" {
		btn = ui.ButtonBase(c.Key("player-option-"+option)).Size(34, 34).Center().Radius(17).
			TextColor(ui.RGBA(255, 255, 255, 0.84)).Label(accessibleLabel).Disabled(disabled)
		if active {
			btn.Background(playerOverlayAccent().Alpha(0.20)).TextColor(playerOverlayAccent())
		} else if btn.Hovered() && !disabled {
			btn.Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGB(255, 255, 255))
		} else if disabled {
			btn.TextColor(ui.RGBA(255, 255, 255, 0.38))
		}
		btn.Children(func() {
			ui.Icon(c, playerOverlayIcons[iconName]).Size(19, 19)
		})
	} else {
		btn = ui.ButtonBase(c.Key("player-option-"+option)).Height(34).Padding(0, 7).Radius(17).
			TextColor(ui.RGBA(255, 255, 255, 0.84)).Label(accessibleLabel).Disabled(disabled)
		if active {
			btn.Background(playerOverlayAccent().Alpha(0.20)).TextColor(playerOverlayAccent())
		} else if btn.Hovered() && !disabled {
			btn.Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGB(255, 255, 255))
		}
		labelColor := ui.RGBA(255, 255, 255, 0.88)
		if active {
			labelColor = playerOverlayAccent()
		}
		btn.Children(func() {
			ui.Text(c, label).FontSize(12).Bold().TextColor(labelColor).SingleLine()
		})
	}

	btn.OnClick(func() {
		if a.playerOverlayMenu == option {
			a.closePlayerOverlayMenu()
		} else {
			a.openPlayerOverlayMenu(option)
		}
		a.markPlayerOverlayActivity()
	})
}

func (a *appState) openPlayerOverlayMenu(option string) {
	a.destroyPlayerMenuWindow()
	a.playerOverlayMenu = option
	switch option {
	case "subtitle":
		a.playerMenuTracks = append(a.playerMenuTracks[:0], a.playback.SubtitleTracks...)
	case "audio":
		a.playerMenuTracks = append(a.playerMenuTracks[:0], a.playback.AudioTracks...)
	default:
		a.playerMenuTracks = nil
	}
	a.setPlayerOverlayPinned(true)
	if a.window == nil || a.playerOverlayWindow == nil {
		return
	}
	bounds := playerMenuBounds(a.window.ContentBounds())
	a.playerMenuContent = overlay.View(a.playerMenuView)
	a.playerMenuWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: "播放选项", X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
		Parent: a.window, Frameless: true, SkipTaskbar: true, Transparent: true, Hidden: true, DisableShadow: true,
		BackgroundColor: "#00000000", DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		Content: a.playerMenuContent,
	})
	// A new menu surface contains no previous transport/menu bitmap. It stays
	// transparent until its own first paint, and never shifts the control bar.
	a.playerMenuContent.SetOpacity(a.playerMenuWindow, a.playerOverlayOpacity)
	a.playerMenuWindow.Show()
	a.syncPlayerOverlay()
	a.playerOverlayWindow.Invalidate()
}

func (a *appState) destroyPlayerMenuWindow() {
	if a.playerMenuWindow != nil {
		a.playerMenuWindow.Destroy()
		a.playerMenuWindow = nil
	}
	a.playerMenuContent = nil
}

func (a *appState) playerPopover(c *ui.Context) {
	if a.playerOverlayMenu == "" {
		return
	}
	ui.Column(c).Label("播放选项菜单").Fill().MinWidth(0).Padding(8).Gap(3).Radius(18).Clip().
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
		ui.Scroll(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
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
					playerAppleMenuItem(c, label, absFloat(a.playback.Speed-speed) < 0.01).OnClick(func() {
						a.player.SetSpeed(speed)
						a.playback.Speed = speed
						a.closePlayerOverlayMenu()
					})
				}
			}
		})
	})
}

func (a *appState) playerTrackMenuItem(c *ui.Context, label string, trackID int, selected, subtitle bool) {
	playerAppleMenuItem(c, label, selected).OnClick(func() {
		kind := "audio"
		if subtitle {
			kind = "subtitle"
		}
		a.player.SelectTrack(kind, trackID)
		a.closePlayerOverlayMenu()
	})
}

func playerAppleMenuItem(c *ui.Context, label string, selected bool) ui.Element {
	btn := ui.Button(c, "").Height(34).Padding(6, 10).Radius(9).BorderWidth(0).
		Background(ui.RGBA(0, 0, 0, 0)).FillWidth().MinWidth(0).Clip().Label(label).
		Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	txtColor := ui.RGBA(255, 255, 255, 0.88)
	if selected {
		btn.Background(playerOverlayAccent().Alpha(0.24))
		txtColor = playerOverlayAccent()
	} else if btn.Hovered() {
		btn.Background(ui.RGBA(255, 255, 255, 0.10))
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(txtColor).Grow(1).MinWidth(0).SingleLine()
	})
	return btn
}

func playerOverlayIconButton(c *ui.Context, name, label string, size, iconSize float32) ui.Element {
	b := playerOverlayButton(c, label, size, ui.RGBA(255, 255, 255, 0.88))
	b.Children(func() {
		if strings.HasPrefix(name, "seek-") {
			ui.Box(c).Size(iconSize, iconSize).Children(func() {
				ui.Icon(c, playerOverlayIcons[name]).Absolute().Top(0).Left(0).Size(iconSize, iconSize)
				ui.Text(c, "10").Absolute().Left(0).Right(0).Top(iconSize * 0.28).TextAlign(ui.Center).
					FontSize(8.5).Bold().TextColor(ui.RGB(255, 255, 255)).SingleLine()
			})
			return
		}
		ui.Icon(c, playerOverlayIcons[name]).Size(iconSize, iconSize)
	})
	return b
}

func playerOverlayButton(c *ui.Context, label string, size float32, color ui.Color) ui.Element {
	b := ui.ButtonBase(c.Key("player-control-"+label)).Size(size, size).Center().Radius(size / 2).
		Label(label).TextColor(color).FocusRing(false)
	background := ui.Transparent
	if b.Pressed() {
		background = ui.RGBA(255, 255, 255, 0.20)
	} else if b.Hovered() {
		background = ui.RGBA(255, 255, 255, 0.13)
	}
	b.Background(b.AnimateColor("player-control-hover", background, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.90
	}
	b.AnimatePose(pose, ui.Snappy)
	return b
}

func playerOverlayPlayPause(c *ui.Context, button ui.Element, paused bool, size float32) {
	playing := float32(1)
	if paused {
		playing = 0
	}
	v := button.AnimateTo("playing", playing, ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.3}})
	ui.Box(c).Size(size, size).Children(func() {
		for _, icon := range []struct {
			svg   *ui.SVG
			shown float32
		}{{playerOverlayIcons["play"], 1 - v}, {playerOverlayIcons["pause"], v}} {
			if icon.shown <= 0.01 {
				continue
			}
			scale := 0.4 + 0.6*icon.shown
			ui.Icon(c, icon.svg).Absolute().Top(0).Left(0).Size(size, size).
				Scale(scale, scale).Opacity(min(icon.shown, 1))
		}
	})
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
	ownedFocus := a.playerOverlayHasFocus()
	a.destroyPlayerMenuWindow()
	a.playerOverlayMenu = ""
	a.playerMenuTracks = nil
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Invalidate()
	}
	a.setPlayerOverlayPinned(false)
	a.markPlayerOverlayActivity()
	if ownedFocus && a.window != nil {
		restoreWindowFocus(a.window)
	}
}

func (a *appState) seekDisplayPosition() float64 {
	if a.seekDragging || a.seekFeedback.Pending {
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
			action := playerShortcutAction(0, key, fullscreen)
			if action == playerToggleFull {
				a.togglePlayerFullscreen(c.Preferences().ReduceMotion)
			} else if action == playerExitFull {
				a.requestPlayerFullscreen(false, c.Preferences().ReduceMotion)
			} else {
				a.performPlayerAction(action)
			}
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
		a.requestPlayerSeekRelative(-10)
	case playerSeekForward:
		a.requestPlayerSeekRelative(10)
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
			a.togglePlayerFullscreen(false)
		}
	case playerExitFull:
		if a.window != nil {
			a.requestPlayerFullscreen(false, false)
		}
	case playerReturn:
		a.stopPlayback()
	}
	if action != playerShortcutIgnore {
		a.markPlayerOverlayActivity()
	}
}

func playerIconButton(c *ui.Context, name, label string) ui.Element {
	b := ui.Button(c, "").Size(38, 36).Padding(0).Radius(9).BorderWidth(0).
		Background(ui.RGBA(0, 0, 0, 0)).TextColor(ui.RGB(255, 255, 255)).Label(label).
		Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	if b.Hovered() {
		b.Background(ui.RGBA(255, 255, 255, 0.14))
	}
	b.Children(func() { ui.Icon(c, playerOverlayIcons[name]).Size(19, 19) })
	return b
}

func playerTextButton(c *ui.Context, label string) ui.Element {
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
