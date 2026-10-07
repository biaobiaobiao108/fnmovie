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
	// DWM color attributes are supported on Windows 11. Windows 10 simply
	// keeps its system titlebar when these calls are rejected.
	caption := uint32(0x00EDF2F4) // #f4f2ed as a Win32 COLORREF (BGR).
	text := uint32(0x00272A25)    // #252a27.
	border := uint32(0x00DFE5E5)  // #e5e5df.
	for _, attribute := range []struct {
		id    uintptr
		value *uint32
	}{{35, &caption}, {36, &text}, {34, &border}} {
		setWindowAttribute.Call(hwnd, attribute.id, uintptr(unsafe.Pointer(attribute.value)), unsafe.Sizeof(*attribute.value))
	}
}
