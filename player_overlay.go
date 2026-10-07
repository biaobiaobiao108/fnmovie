package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const playerOverlayHideDelay = 2500 * time.Millisecond
const (
	playerOverlayHeaderHeight  = 56
	playerOverlayControlHeight = 82
	playerOverlayFadeDuration  = 180 * time.Millisecond
	playerOverlayTargetOpacity = 1.0
)

var playerOverlayIcons = map[string]*ui.SVG{
	"back":       ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/><path d="M9 12h11"/></svg>`)),
	"play":       ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m8 5 12 7-12 7z"/></svg>`)),
	"pause":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M8 5v14M16 5v14"/></svg>`)),
	"volume":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 10v4h4l5 4V6l-5 4z"/><path d="M17 9a5 5 0 0 1 0 6"/><path d="M19 6a9 9 0 0 1 0 12"/></svg>`)),
	"muted":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 10v4h4l5 4V6l-5 4z"/><path d="m17 9 5 6m0-6-5 6"/></svg>`)),
	"fullscreen": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M8 4H4v4m12-4h4v4M4 16v4h4m12-4v4h-4"/></svg>`)),
	"window":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M8 4H4v4m12-4h4v4M4 16v4h4m12-4v4h-4"/><path d="M9 9h6v6H9z"/></svg>`)),
}

func (a *appState) createPlayerOverlay() {
	if a.window == nil || a.playerOverlayWindow != nil {
		return
	}
	bounds := a.window.ContentBounds()
	controlHeight := min(a.playerOverlayContentHeight(), bounds.Height)
	headerHeight := min(playerOverlayHeaderHeight, bounds.Height-controlHeight)
	a.playerOverlayWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: "播放控件", X: bounds.X, Y: bounds.Y + bounds.Height - controlHeight, Width: bounds.Width, Height: controlHeight,
		Parent: a.window, Frameless: true, SkipTaskbar: true, Transparent: true, Opacity: 0,
		BackgroundColor: "#00000000", DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		Content: ui.View(a.playerOverlayView),
	})
	a.playerHeaderWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: "播放信息", X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: headerHeight,
		Parent: a.window, Frameless: true, SkipTaskbar: true, Transparent: true, Opacity: 0,
		BackgroundColor: "#00000000", DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		Content: ui.View(a.playerHeaderView),
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
	menuHeight := min(330, max(90, 34+rows*34))
	height := playerOverlayControlHeight + 94 + menuHeight
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
					a.window.Update(func() { a.showPlayerOverlay() })
				}
			case now := <-ticker.C:
				a.overlayMu.Lock()
				hide := a.playerOverlayVisible && !a.playerOverlayPinned && !a.playerOverlaySeeking && now.Sub(a.playerOverlayLastInput) >= playerOverlayHideDelay
				a.overlayMu.Unlock()
				if hide && a.window != nil {
					a.window.Update(func() { a.hidePlayerOverlay(true) })
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
		a.playerOverlayWindow.Focus()
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
		a.playerOverlayWindow.Hide()
		a.playerHeaderWindow.Hide()
		a.playerOverlayWindow.SetOpacity(0)
		a.playerHeaderWindow.SetOpacity(0)
		a.overlayMu.Lock()
		a.playerOverlayOpacity = 0
		a.overlayMu.Unlock()
	}
}

func (a *appState) animatePlayerOverlay(target float64) {
	a.overlayMu.Lock()
	a.playerOverlayTransition++
	transition := a.playerOverlayTransition
	from := a.playerOverlayOpacity
	a.overlayMu.Unlock()
	go func() {
		started := time.Now()
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for {
			progress := float64(time.Since(started)) / float64(playerOverlayFadeDuration)
			if progress > 1 {
				progress = 1
			}
			eased := progress * progress * (3 - 2*progress)
			alpha := from + (target-from)*eased
			if a.window != nil {
				a.window.Update(func() {
					a.overlayMu.Lock()
					valid := transition == a.playerOverlayTransition
					if valid {
						a.playerOverlayOpacity = alpha
					}
					a.overlayMu.Unlock()
					if !valid || a.playerOverlayWindow == nil || a.playerHeaderWindow == nil {
						return
					}
					a.playerOverlayWindow.SetOpacity(alpha)
					a.playerHeaderWindow.SetOpacity(alpha)
					if progress >= 1 && target == 0 {
						a.playerOverlayWindow.Hide()
						a.playerHeaderWindow.Hide()
						a.window.Focus()
					}
				})
			}
			if progress >= 1 {
				return
			}
			<-ticker.C
		}
	}()
}

func (a *appState) closePlayerOverlay() {
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
	a.overlayMu.Lock()
	a.playerOverlayVisible, a.playerOverlayPinned, a.playerOverlaySeeking = false, false, false
	a.playerOverlayTransition++
	a.overlayMu.Unlock()
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
	playerOverlayTheme := ui.DarkTheme()
	playerOverlayTheme.Background = ui.Transparent
	playerOverlayTheme.Surface = ui.Transparent
	playerOverlayTheme.SurfaceHover = ui.RGBA(255, 255, 255, 0.10)
	playerOverlayTheme.SurfacePressed = ui.RGBA(255, 255, 255, 0.16)
	c.SetTheme(playerOverlayTheme)
	ui.Row(c).Absolute().Top(6).Left(18).Gap(12).AlignItems(ui.Center).Padding(8, 12).Radius(11).
		Background(ui.RGBA(18, 19, 21, 0.54)).Children(func() {
		if playerIconButton(c, "back", "返回影片库").Clicked() {
			a.stopPlayback()
		}
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, a.playback.Title).FontSize(15).Bold().TextColor(ui.RGB(255, 255, 255)).SingleLine()
			quality := strings.TrimSpace(a.playback.Quality)
			if quality == "" {
				quality = "飞牛影视"
			}
			ui.Text(c, quality).FontSize(10).TextColor(ui.RGBA(255, 255, 255, 0.72)).SingleLine()
		})
	})
}

func (a *appState) playerTransport(c *ui.Context) {
	maxPosition := maxFloat(1, a.playback.Duration)
	if !a.seekDragging {
		a.seekSliderPosition = minFloat(maxPosition, maxFloat(0, a.playback.Position))
	}
	ui.Column(c).Absolute().Bottom(5).Left(18).Right(18).Padding(5, 12).Gap(2).Radius(11).
		Background(ui.RGBA(18, 19, 21, 0.54)).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, formatClock(a.seekDisplayPosition())).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.9))
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
			ui.Text(c, formatClock(a.playback.Duration)).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.9))
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			playIcon := "play"
			if !a.playback.Paused {
				playIcon = "pause"
			}
			if playerIconButton(c, playIcon, "播放或暂停").Clicked() {
				a.player.TogglePause()
				a.playback.Paused = !a.playback.Paused
				a.markPlayerOverlayActivity()
			}
			if playerTextButton(c, "−10").Clicked() {
				a.player.Seek(-10)
				a.markPlayerOverlayActivity()
			}
			if playerTextButton(c, "+10").Clicked() {
				a.player.Seek(10)
				a.markPlayerOverlayActivity()
			}
			ui.Spacer(c)
			volumeIcon := "volume"
			if a.playback.Muted || a.playback.Volume <= 0 {
				volumeIcon = "muted"
			}
			if playerIconButton(c, volumeIcon, "静音").Clicked() {
				a.player.ToggleMute()
				a.playback.Muted = !a.playback.Muted
				a.markPlayerOverlayActivity()
			}
			volume := ui.Slider(c, &a.playback.Volume, 0, 100).Width(92)
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
			a.playerOptionButton(c, "speed")
			a.playerOptionButton(c, "subtitle")
			a.playerOptionButton(c, "audio")
			fullscreenIcon := "fullscreen"
			if a.window != nil && a.window.IsFullScreen() {
				fullscreenIcon = "window"
			}
			if playerIconButton(c, fullscreenIcon, "切换全屏").Clicked() {
				if a.window != nil {
					a.window.ToggleFullScreen()
					a.syncPlayerOverlay()
				}
				a.markPlayerOverlayActivity()
			}
		})
	})
}

func (a *appState) playerOptionButton(c *ui.Context, option string) {
	label, disabled := "", false
	switch option {
	case "speed":
		label = fmt.Sprintf("%.2g×", maxFloat(0.25, a.playback.Speed))
	case "subtitle":
		label, disabled = "字幕", len(a.playback.SubtitleTracks) == 0
	case "audio":
		label, disabled = "音轨", len(a.playback.AudioTracks) == 0
	}
	button := playerTextButton(c, label).Disabled(disabled)
	if button.Clicked() {
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
	menuHeight := min(330, max(90, 34+rows*34))
	ui.Column(c).Absolute().Bottom(94).Right(24).Width(320).Height(float32(menuHeight)).Padding(8).Gap(2).Radius(12).
		Background(ui.RGBA(22, 23, 25, 0.78)).Children(func() {
		var title string
		switch a.playerOverlayMenu {
		case "subtitle":
			title = "字幕"
		case "audio":
			title = "音轨"
		case "speed":
			title = "播放速度"
		}
		ui.Text(c, title).Padding(7, 9).FontSize(11).Bold().TextColor(ui.RGBA(255, 255, 255, 0.68))
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
					if playerMenuItem(c, label, absFloat(a.playback.Speed-speed) < 0.01) {
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
	if playerMenuItem(c, label, selected) {
		kind := "audio"
		if subtitle {
			kind = "subtitle"
		}
		a.player.SelectTrack(kind, trackID)
		a.closePlayerOverlayMenu()
	}
}

func playerMenuItem(c *ui.Context, label string, selected bool) bool {
	prefix := "   "
	if selected {
		prefix = "✓  "
	}
	return playerTextButton(c, prefix+label).FillWidth().Clicked()
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
		Background(ui.RGBA(0, 0, 0, 0)).TextColor(ui.RGB(255, 255, 255)).Tooltip(label).
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
