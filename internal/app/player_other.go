//go:build !windows

package app

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

type playerProcess struct{}

func (p *playerProcess) start(string, string, uintptr, float64) error {
	return errors.New("embedded mpv playback is only supported on Windows")
}
func (p *playerProcess) running() bool                      { return false }
func (p *playerProcess) stop()                              {}
func (p *playerProcess) command(...any)                     {}
func (p *playerProcess) position() (float64, float64, bool) { return 0, 0, false }
func (p *playerProcess) snapshot() PlayerSnapshot           { return PlayerSnapshot{} }
func (p *playerProcess) setViewport(ui.Rect)                {}
func (p *playerProcess) pointerActivity() <-chan struct{}   { return nil }
func windowScale(uintptr) float64                           { return 1 }
