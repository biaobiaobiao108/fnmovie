//go:build windows

package fnmovieoverlay

import (
	"log"
	"math"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	gdi32         = syscall.NewLazyDLL("gdi32.dll")
	getWindowLong = user32.NewProc("GetWindowLongPtrW")
	setWindowLong = user32.NewProc("SetWindowLongPtrW")
	updateLayered = user32.NewProc("UpdateLayeredWindow")
	getDC         = user32.NewProc("GetDC")
	getClientRect = user32.NewProc("GetClientRect")
	releaseDC     = user32.NewProc("ReleaseDC")
	setWindowPos  = user32.NewProc("SetWindowPos")
	createDC      = gdi32.NewProc("CreateCompatibleDC")
	deleteDC      = gdi32.NewProc("DeleteDC")
	createDIB     = gdi32.NewProc("CreateDIBSection")
	selectObject  = gdi32.NewProc("SelectObject")
	deleteObject  = gdi32.NewProc("DeleteObject")
)

type bitmapHeader struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, SizeImage uint32
	XPels, YPels           int32
	ClrUsed, ClrImportant  uint32
}
type point struct{ X, Y int32 }
type size struct{ W, H int32 }
type blend struct{ Operation, Flags, Alpha, Format byte }

type presenter struct {
	platform.Surface
	hwnd, dc, bitmap, previous, bits uintptr
	width, height                    int
	alpha                            byte
}

func attach(conn *surface.Conn) *presenter {
	w := conn.Window.(*mygo.Window)
	p := &presenter{Surface: conn.Surface, hwnd: w.NativeHandle()}
	// The child surface remains the native input/accessibility target, while
	// the top-level owned window receives its premultiplied pixels.
	const exStyle = ^uintptr(19) // GWL_EXSTYLE (-20)
	style, _, _ := getWindowLong.Call(p.hwnd, exStyle)
	setWindowLong.Call(p.hwnd, exStyle, style|0x80000) // WS_EX_LAYERED
	conn.Surface = p
	return p
}

func setIgnoreMouseEvents(window *mygo.Window, ignore bool) {
	if window == nil {
		return
	}
	const (
		gwlExStyle      = ^uintptr(19) // GWL_EXSTYLE (-20)
		swpFrameChanged = uintptr(0x0020)
		swpNoActivate   = uintptr(0x0010)
		swpNoZOrder     = uintptr(0x0004)
		swpNoSize       = uintptr(0x0001)
		swpNoMove       = uintptr(0x0002)
	)
	hwnd := window.NativeHandle()
	style, _, _ := getWindowLong.Call(hwnd, gwlExStyle)
	style = ignoreMouseEventsStyle(style, ignore)
	setWindowLong.Call(hwnd, gwlExStyle, style)
	setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpFrameChanged|swpNoActivate|swpNoZOrder|swpNoSize|swpNoMove)
}

// Returning no GPU target selects MyGo's alpha-preserving software rasterizer.
// HWND swap chains and GDI PresentPixels discard alpha on the stock backend.
func (*presenter) Native() platform.SurfaceNative { return platform.SurfaceNative{} }

func (p *presenter) PresentPixels(pix []byte, stride, width, height int) {
	if width <= 0 || height <= 0 || stride < width*4 || len(pix) < stride*height {
		return
	}
	if !p.allocate(width, height) {
		return
	}
	dest := unsafe.Slice((*byte)(unsafe.Pointer(p.bits)), width*height*4)
	for y := 0; y < height; y++ {
		copy(dest[y*width*4:(y+1)*width*4], pix[y*stride:y*stride+width*4])
	}
	p.present()
}

func (p *presenter) allocate(width, height int) bool {
	if p.bitmap != 0 && p.width == width && p.height == height {
		return true
	}
	p.freeBitmap()
	screen, _, _ := getDC.Call(0)
	defer releaseDC.Call(0, screen)
	p.dc, _, _ = createDC.Call(screen)
	header := bitmapHeader{Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}
	header.Size = uint32(unsafe.Sizeof(header))
	p.bitmap, _, _ = createDIB.Call(screen, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&p.bits)), 0, 0)
	if p.dc == 0 || p.bitmap == 0 {
		p.freeBitmap()
		return false
	}
	p.previous, _, _ = selectObject.Call(p.dc, p.bitmap)
	p.width, p.height = width, height
	return true
}

func (p *presenter) present() {
	if p.bitmap == 0 {
		return
	}
	// A menu can resize the window before its new scene is rasterized. Never
	// replay the previous menu bitmap: UpdateLayeredWindow would also reset
	// the window to that stale size, causing a briefly flashing popup.
	var rect [4]int32
	if ok, _, _ := getClientRect.Call(p.hwnd, uintptr(unsafe.Pointer(&rect[0]))); ok == 0 || int(rect[2]) != p.width || int(rect[3]) != p.height {
		return
	}
	sz, origin := size{int32(p.width), int32(p.height)}, point{}
	bl := blend{Alpha: p.alpha, Format: 1} // AC_SRC_ALPHA
	ok, _, err := updateLayered.Call(p.hwnd, 0, 0, uintptr(unsafe.Pointer(&sz)), p.dc, uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&bl)), 2)
	if ok == 0 {
		log.Printf("playback overlay composition: %v", err)
	}
}

func (p *presenter) setOpacity(_ *mygo.Window, opacity float64) {
	p.alpha = byte(math.Round(max(0, min(1, opacity)) * 255))
	p.present()
}

func (p *presenter) freeBitmap() {
	if p.previous != 0 && p.dc != 0 {
		selectObject.Call(p.dc, p.previous)
	}
	if p.bitmap != 0 {
		deleteObject.Call(p.bitmap)
	}
	if p.dc != 0 {
		deleteDC.Call(p.dc)
	}
	p.dc, p.bitmap, p.previous, p.bits = 0, 0, 0, 0
}
func (p *presenter) close() { p.freeBitmap(); p.hwnd = 0 }
