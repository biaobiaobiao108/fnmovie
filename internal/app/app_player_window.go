package app

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *appState) playbackWindow() *mygo.Window {
	if a.playerWindow != nil {
		return a.playerWindow
	}
	return a.window
}

func (a *appState) createPlayerWindow(title string, requestID uint64) {
	if a.playerWindow != nil {
		return
	}
	bounds := mygo.Rectangle{Width: 1280, Height: 800}
	if a.window != nil {
		bounds = a.window.ContentBounds()
	}
	a.playerWindowRequest = requestID
	a.playerWindowSizedForRequest = 0
	a.playerWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: title, X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
		MinWidth: 320, MinHeight: 180, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: movieTitleBarHeight,
		BackgroundColor: "#000000", StateKey: "player", Content: ui.View(a.playerWindowView),
	})
	a.playerWindow.OnClose(func(e *mygo.CloseEvent) {
		if e.Window != a.playerWindow {
			return
		}
		e.PreventDefault()
		a.stopPlayback()
	})
	setWindowTheme(a.playerWindow)
	if err := a.playerWindow.SetIcon(appIconPNG); err != nil {
		log.Printf("set player window icon: %v", err)
	}
	a.registerPlayerOverlayHooks()
	a.playerWindow.Show()
}

func (a *appState) destroyPlayerWindow() {
	if a.playerWindow != nil {
		a.playerWindow.Destroy()
		a.playerWindow = nil
	}
	a.playerWindowRequest = 0
	a.playerWindowSizedForRequest = 0
	a.playbackLoadingPlayer = nil
	a.playerOverlayHooks = false
}

func (a *appState) sizePlayerWindowForVideo(requestID uint64, snapshot PlayerSnapshot) {
	window := a.playerWindow
	if window == nil || requestID == 0 || requestID != a.playerWindowRequest || a.playerWindowSizedForRequest == requestID || snapshot.VideoWidth <= 0 || snapshot.VideoHeight <= 0 {
		return
	}
	content := window.ContentBounds()
	centerX, centerY := content.X+content.Width/2, content.Y+content.Height/2
	workArea := playerWindowWorkArea(centerX, centerY)
	if workArea.Width <= 0 || workArea.Height <= 0 {
		workArea = mygo.Rectangle{Width: 1600, Height: 900}
	}
	maxWidth := max(1, int(float64(workArea.Width)*0.84))
	maxHeight := max(1, int(float64(workArea.Height)*0.84))
	aspect := float64(snapshot.VideoWidth) / float64(snapshot.VideoHeight)
	width, height := maxWidth, int(float64(maxWidth)/aspect)
	if height > maxHeight {
		height = maxHeight
		width = int(float64(height) * aspect)
	}
	width, height = max(1, width), max(1, height)
	x, y := centerX-width/2, centerY-height/2
	if workArea.Width >= width {
		x = min(max(x, workArea.X), workArea.X+workArea.Width-width)
	}
	if workArea.Height >= height {
		y = min(max(y, workArea.Y), workArea.Y+workArea.Height-height)
	}
	window.SetContentBounds(mygo.Rectangle{X: x, Y: y, Width: width, Height: height})
	a.playerWindowSizedForRequest = requestID
}

func playerWindowWorkArea(centerX, centerY int) mygo.Rectangle {
	displays := mygo.Screen.Displays()
	var nearest mygo.Rectangle
	nearestDistance := int64(^uint64(0) >> 1)
	for _, display := range displays {
		area := display.WorkArea
		if area.Width <= 0 || area.Height <= 0 {
			area = display.Bounds
		}
		if area.Width <= 0 || area.Height <= 0 {
			continue
		}
		if centerX >= area.X && centerX < area.X+area.Width && centerY >= area.Y && centerY < area.Y+area.Height {
			return area
		}
		dx := int64(0)
		if centerX < area.X {
			dx = int64(area.X - centerX)
		} else if centerX >= area.X+area.Width {
			dx = int64(centerX - (area.X + area.Width))
		}
		dy := int64(0)
		if centerY < area.Y {
			dy = int64(area.Y - centerY)
		} else if centerY >= area.Y+area.Height {
			dy = int64(centerY - (area.Y + area.Height))
		}
		distance := dx*dx + dy*dy
		if distance < nearestDistance {
			nearest, nearestDistance = area, distance
		}
	}
	return nearest
}

func (a *appState) playerWindowView(c *ui.Context) {
	c.SetTheme(movieTheme())
	c.Root().Background(ui.RGB(0, 0, 0))
	a.advancePlayerFullscreen(c)
	a.advancePlayerOverlayAnimation(c)
	a.playerView(c)
}
