//go:build windows

package app

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	dwmThemeDLL        = syscall.NewLazyDLL("dwmapi.dll")
	setWindowAttribute = dwmThemeDLL.NewProc("DwmSetWindowAttribute")
)

func setWindowTheme(window *mygo.Window) {
	applyWindowTheme(window, false)
}

func (a *appState) syncWindowTheme(secondary bool) {
	if a.window == nil || a.window.NativeHandle() == 0 {
		return
	}
	if a.windowIsDark == secondary && a.windowThemeSet {
		return
	}
	a.windowIsDark = secondary
	a.windowThemeSet = true
	applyWindowTheme(a.window, secondary)
}

func applyWindowTheme(window *mygo.Window, dark bool) {
	if window == nil || window.NativeHandle() == 0 || setWindowAttribute.Find() != nil {
		return
	}
	hwnd := window.NativeHandle()
	darkMode := uint32(0)
	caption := uint32(0x00F3F1F0) // #f0f1f3 as BGR
	text := uint32(0x001F1B19)    // #191b1f
	border := uint32(0x00E5E5E9)  // #e9e5e5
	if dark {
		darkMode = 1
		caption = uint32(0x00100C0B) // #0b0c10 as BGR
		text = uint32(0x00FFFFFF)
		border = uint32(0x00181412)
	}
	for _, attribute := range []struct {
		id    uintptr
		value *uint32
	}{{20, &darkMode}, {35, &caption}, {36, &text}, {34, &border}} {
		setWindowAttribute.Call(hwnd, attribute.id, uintptr(unsafe.Pointer(attribute.value)), unsafe.Sizeof(*attribute.value))
	}
}

func restoreWindowFocus(window *mygo.Window) {
	if window == nil {
		return
	}
	mygo.App.Focus()
	window.Focus()
}
