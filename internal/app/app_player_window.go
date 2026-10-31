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
	a.playerWindow = mygo.NewWindow(mygo.WindowOptions{
		Title: title, X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
		MinWidth: 640, MinHeight: 360, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: movieTitleBarHeight,
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
	a.playbackLoadingPlayer = nil
	a.playerOverlayHooks = false
}

func (a *appState) playerWindowView(c *ui.Context) {
	c.SetTheme(movieTheme())
	c.Root().Background(ui.RGB(0, 0, 0))
	a.advancePlayerFullscreen(c)
	a.advancePlayerOverlayAnimation(c)
	a.playerView(c)
}
