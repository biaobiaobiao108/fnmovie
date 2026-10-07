// Package fnmovieoverlay adapts MyGo native content to per-pixel Windows
// transparency. This narrow module lives under MyGo's import namespace because
// its Content contract uses MyGo internal surface types. Keep its version in
// sync with the application's MyGo dependency; no MyGo module-cache edits are
// needed. Only the small playback control windows use software composition.
package fnmovieoverlay

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/surface"
	"github.com/egoist/mygo/ui"
)

type Content struct {
	view      *ui.Content
	presenter *presenter
}

func View(view func(*ui.Context)) *Content { return &Content{view: ui.View(view)} }

func (c *Content) AttachContent(conn *surface.Conn) {
	c.presenter = attach(conn)
	c.view.AttachContent(conn)
	detach := conn.Detach
	conn.Detach = func() {
		if detach != nil {
			detach()
		}
		c.presenter.close()
	}
}

// SetOpacity runs on the MyGo UI thread. Windows uses the per-pixel presenter's
// constant alpha, never SetLayeredWindowAttributes, which disables subsequent
// UpdateLayeredWindow calls.
func (c *Content) SetOpacity(window *mygo.Window, opacity float64) {
	if c.presenter != nil {
		c.presenter.setOpacity(window, opacity)
	}
}
