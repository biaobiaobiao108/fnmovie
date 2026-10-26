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
	if window == nil || window.NativeHandle() == 0 || setWindowAttribute.Find() != nil {
		return
	}
	hwnd := window.NativeHandle()
	// DWM title-bar attributes are supported on Windows 11. Older versions
	// keep their system title-bar colors when these calls are rejected.
	darkMode := uint32(0)
	caption := uint32(0x00F6F4F4) // #f4f4f6 as a Win32 COLORREF (BGR).
	text := uint32(0x001F1D1D)    // #1d1d1f.
	border := uint32(0x00E9E5E5)  // #e5e5e9.
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
