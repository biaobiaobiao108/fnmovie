//go:build !windows

package app

import "github.com/egoist/mygo"

func setWindowTheme(*mygo.Window) {}

func restoreWindowFocus(window *mygo.Window) {
	if window != nil {
		window.Focus()
	}
}
