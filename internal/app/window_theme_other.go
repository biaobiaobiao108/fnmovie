//go:build !windows

package app

import "github.com/egoist/mygo"

func setWindowTheme(*mygo.Window) {}

func (a *appState) syncWindowTheme(bool) {}

func restoreWindowFocus(window *mygo.Window) {
	if window != nil {
		mygo.App.Focus()
		window.Focus()
	}
}
