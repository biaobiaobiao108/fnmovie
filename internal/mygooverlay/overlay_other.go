//go:build !windows

package fnmovieoverlay

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/surface"
)

type presenter struct{}

func attach(*surface.Conn) *presenter                         { return &presenter{} }
func (*presenter) close()                                     {}
func (*presenter) setOpacity(w *mygo.Window, opacity float64) { w.SetOpacity(opacity) }
