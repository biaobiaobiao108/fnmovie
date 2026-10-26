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
	// keep their system title bar when these calls are rejected.
	darkMode := uint32(1)
	caption := uint32(0x00111311) // #111311 as a Win32 COLORREF (BGR).
	text := uint32(0x00F1F4F1)    // #f1f4f1.
	border := uint32(0x00323630)  // #303632.
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
