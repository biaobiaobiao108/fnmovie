//go:build windows

package app

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

var (
	user32Player              = syscall.NewLazyDLL("user32.dll")
	procFindWindowExW         = user32Player.NewProc("FindWindowExW")
	procGetWindowThreadPID    = user32Player.NewProc("GetWindowThreadProcessId")
	procGetDPIForWindow       = user32Player.NewProc("GetDpiForWindow")
	procGetClassNameW         = user32Player.NewProc("GetClassNameW")
	procGetWindowRect         = user32Player.NewProc("GetWindowRect")
	procGetCursorPos          = user32Player.NewProc("GetCursorPos")
	procSetWindowPos          = user32Player.NewProc("SetWindowPos")
	procGetPlayerWindow       = user32Player.NewProc("GetWindow")
	procGetForegroundWindow   = user32Player.NewProc("GetForegroundWindow")
	procGetAncestor           = user32Player.NewProc("GetAncestor")
	procIsWindow              = user32Player.NewProc("IsWindow")
	procIsIconic              = user32Player.NewProc("IsIconic")
	procCreatePlayerWindow    = user32Player.NewProc("CreateWindowExW")
	procDestroyPlayerWindow   = user32Player.NewProc("DestroyWindow")
	procShowPlayerWindow      = user32Player.NewProc("ShowWindow")
	procGetPlayerClientRect   = user32Player.NewProc("GetClientRect")
	procGetPlayerParent       = user32Player.NewProc("GetParent")
	procClientToScreen        = user32Player.NewProc("ClientToScreen")
	procSetWindowLongPtrW     = user32Player.NewProc("SetWindowLongPtrW")
	procCallWindowProcW       = user32Player.NewProc("CallWindowProcW")
	procReleaseCapture        = user32Player.NewProc("ReleaseCapture")
	procSendMessageW          = user32Player.NewProc("SendMessageW")
	procSetWindowRgn          = user32Player.NewProc("SetWindowRgn")
	procIsZoomed              = user32Player.NewProc("IsZoomed")
	gdi32Player               = syscall.NewLazyDLL("gdi32.dll")
	procCreateRoundRectRgn    = gdi32Player.NewProc("CreateRoundRectRgn")
	procCreateRectRgn         = gdi32Player.NewProc("CreateRectRgn")
	procCombineRgn            = gdi32Player.NewProc("CombineRgn")
	procDeleteObject          = gdi32Player.NewProc("DeleteObject")
	procDwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	procGetPlayerModule       = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procReadProcessMemory     = syscall.NewLazyDLL("kernel32.dll").NewProc("ReadProcessMemory")
)

type playerSurfaceState struct {
	originalProc  uintptr
	onDoubleClick func()
	lastClickTime time.Time
	lastClickX    int16
	lastClickY    int16
	viewport      ui.Rect
	shape         [3]int32
	mu            sync.Mutex
}

var (
	playerSurfaces               sync.Map // uintptr -> *playerSurfaceState
	playerSurfaceWndProcInitOnce sync.Once
	playerSurfaceWndProcCallback uintptr
)

func absDiff(a, b int16) int16 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

func playerSurfaceWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	val, ok := playerSurfaces.Load(hwnd)
	if !ok {
		return 0
	}
	state := val.(*playerSurfaceState)
	original := state.originalProc

	switch msg {
	case 0x0203: // WM_LBUTTONDBLCLK
		state.mu.Lock()
		state.lastClickTime = time.Time{}
		fn := state.onDoubleClick
		state.mu.Unlock()
		if fn != nil {
			fn()
		}
	case 0x0201: // WM_LBUTTONDOWN
		state.mu.Lock()
		now := time.Now()
		x := int16(lParam & 0xffff)
		y := int16((lParam >> 16) & 0xffff)
		isDbl := !state.lastClickTime.IsZero() &&
			now.Sub(state.lastClickTime) <= 450*time.Millisecond &&
			absDiff(x, state.lastClickX) <= 6 &&
			absDiff(y, state.lastClickY) <= 6
		if isDbl {
			state.lastClickTime = time.Time{}
			fn := state.onDoubleClick
			state.mu.Unlock()
			if fn != nil {
				fn()
			}
		} else {
			state.lastClickTime = now
			state.lastClickX = x
			state.lastClickY = y
			state.mu.Unlock()
		}
	}

	res, _, _ := procCallWindowProcW.Call(original, hwnd, uintptr(msg), wParam, lParam)
	return res
}

func initPlayerSurfaceSubclass(host uintptr) {
	playerSurfaceWndProcInitOnce.Do(func() {
		playerSurfaceWndProcCallback = syscall.NewCallback(playerSurfaceWndProc)
	})
	original, _, _ := procSetWindowLongPtrW.Call(host, ^uintptr(3), playerSurfaceWndProcCallback)
	if original != 0 {
		state := &playerSurfaceState{originalProc: original}
		playerSurfaces.Store(host, state)
	}
}

func setPlayerSurfaceDoubleClick(host uintptr, fn func()) {
	if host == 0 {
		return
	}
	if val, ok := playerSurfaces.Load(host); ok {
		state := val.(*playerSurfaceState)
		state.mu.Lock()
		state.onDoubleClick = fn
		state.mu.Unlock()
	}
}

func setPlayerSurfaceViewport(host uintptr, rect ui.Rect) {
	if host == 0 {
		return
	}
	if val, ok := playerSurfaces.Load(host); ok {
		state := val.(*playerSurfaceState)
		state.mu.Lock()
		state.viewport = rect
		state.mu.Unlock()
		fitPlayerSurface(host)
	}
}

type playerHeaderState struct {
	originalProc  uintptr
	owner         uintptr
	onDoubleClick func()
	lastClickTime time.Time
	lastClickX    int16
	lastClickY    int16
	mu            sync.Mutex
}

var (
	playerHeaders               sync.Map // uintptr -> *playerHeaderState
	playerHeaderWndProcInitOnce sync.Once
	playerHeaderWndProcCallback uintptr
)

func playerHeaderWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	val, ok := playerHeaders.Load(hwnd)
	if !ok {
		return 0
	}
	state := val.(*playerHeaderState)
	original := state.originalProc
	owner := state.owner

	switch msg {
	case 0x00A1: // WM_NCLBUTTONDOWN
		if owner != 0 {
			procReleaseCapture.Call()
			var pt playerPoint
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			lParamPos := uintptr(uint32(pt.x)) | (uintptr(uint32(pt.y)) << 16)
			res, _, _ := procSendMessageW.Call(owner, 0x00A1, 2 /* HTCAPTION */, lParamPos)
			return res
		}
	case 0x00A3, 0x0203: // WM_NCLBUTTONDBLCLK, WM_LBUTTONDBLCLK
		state.mu.Lock()
		state.lastClickTime = time.Time{}
		fn := state.onDoubleClick
		state.mu.Unlock()
		if fn != nil {
			fn()
		}
		return 0
	case 0x0201: // WM_LBUTTONDOWN
		state.mu.Lock()
		now := time.Now()
		x := int16(lParam & 0xffff)
		y := int16((lParam >> 16) & 0xffff)
		isDbl := !state.lastClickTime.IsZero() &&
			now.Sub(state.lastClickTime) <= 450*time.Millisecond &&
			absDiff(x, state.lastClickX) <= 6 &&
			absDiff(y, state.lastClickY) <= 6
		if isDbl {
			state.lastClickTime = time.Time{}
			fn := state.onDoubleClick
			state.mu.Unlock()
			if fn != nil {
				fn()
			}
			return 0
		}
		state.lastClickTime = now
		state.lastClickX = x
		state.lastClickY = y
		state.mu.Unlock()
	}

	res, _, _ := procCallWindowProcW.Call(original, hwnd, uintptr(msg), wParam, lParam)
	return res
}

func setupPlayerHeaderWindowDrag(header, owner uintptr, onDoubleClick func()) {
	if header == 0 || owner == 0 {
		return
	}
	playerHeaderWndProcInitOnce.Do(func() {
		playerHeaderWndProcCallback = syscall.NewCallback(playerHeaderWndProc)
	})
	original, _, _ := procSetWindowLongPtrW.Call(header, ^uintptr(3), playerHeaderWndProcCallback)
	if original != 0 {
		state := &playerHeaderState{
			originalProc:  original,
			owner:         owner,
			onDoubleClick: onDoubleClick,
		}
		playerHeaders.Store(header, state)
	}
}

func cleanupPlayerHeaderWindowDrag(header uintptr) {
	if header != 0 {
		if val, ok := playerHeaders.LoadAndDelete(header); ok {
			state := val.(*playerHeaderState)
			if state.originalProc != 0 {
				procSetWindowLongPtrW.Call(header, ^uintptr(3), state.originalProc)
			}
		}
	}
}

// Reuse MyGo's registered surface class for libmpv, but keep the swap chain in
// an owned popup. A nested surface is composited behind MyGo's DComp target.
func createPlayerSurface(parent uintptr) (uintptr, error) {
	if findMyGoSurface(parent) == 0 {
		return 0, fmt.Errorf("无法找到 MyGo 原生绘制区域")
	}
	class, _ := syscall.UTF16PtrFromString("MyGoSurface")
	module, _, _ := procGetPlayerModule.Call(0)
	// Do not nest the libmpv swap chain under MyGo's DirectComposition surface:
	// DWM draws that surface above child HWND swap chains. An owned popup uses
	// the normal HWND composition path and follows the host window's z-order.
	host, _, err := procCreatePlayerWindow.Call(0x00000080|0x08000000, uintptr(unsafe.Pointer(class)), 0,
		0x80000000|0x04000000, 0, 0, 0, 0, parent, 0, module, 0)
	runtime.KeepAlive(class)
	if host == 0 {
		return 0, fmt.Errorf("创建独立视频绘制区域失败：%w", err)
	}
	initPlayerSurfaceSubclass(host)
	fitPlayerSurface(host)
	return host, nil
}

func fitPlayerSurface(host uintptr) {
	if host == 0 {
		return
	}
	parent, _, _ := procGetPlayerParent.Call(host)
	var rect [4]int32
	if ok, _, _ := procGetPlayerClientRect.Call(parent, uintptr(unsafe.Pointer(&rect[0]))); ok != 0 {
		var origin playerPoint
		procClientToScreen.Call(parent, uintptr(unsafe.Pointer(&origin)))
		scale := windowScale(parent)

		targetX := origin.x
		targetY := origin.y
		targetW := rect[2]
		targetH := rect[3]

		if val, ok := playerSurfaces.Load(host); ok {
			state := val.(*playerSurfaceState)
			state.mu.Lock()
			vp := state.viewport
			state.mu.Unlock()
			if vp.W > 0 && vp.H > 0 {
				leftOffset := int32(math.Round(float64(vp.X) * scale))
				topOffset := int32(math.Round(float64(vp.Y) * scale))
				if leftOffset == 0 && topOffset == 0 {
					targetX = origin.x
					targetY = origin.y
					targetW = rect[2]
					targetH = rect[3]
				} else {
					targetX = origin.x + leftOffset
					targetY = origin.y + topOffset
					targetW = int32(math.Round(float64(vp.W) * scale))
					targetH = int32(math.Round(float64(vp.H) * scale))
					if targetX+targetW > origin.x+rect[2] {
						targetW = origin.x + rect[2] - targetX
					}
					if targetY+targetH > origin.y+rect[3] {
						targetH = origin.y + rect[3] - targetY
					}
				}
			}
		}

		var childBounds [4]int32
		procGetWindowRect.Call(host, uintptr(unsafe.Pointer(&childBounds[0])))
		if childBounds[0] != targetX || childBounds[1] != targetY || childBounds[2]-childBounds[0] != targetW || childBounds[3]-childBounds[1] != targetH {
			procSetWindowPos.Call(host, 0, uintptr(targetX), uintptr(targetY), uintptr(targetW), uintptr(targetH), 0x0010|0x0004|0x0200)
		}
		updatePlayerSurfaceShape(host, parent, targetW, targetH, scale)
	}
}

func updatePlayerSurfaceShape(host, parent uintptr, width, height int32, scale float64) {
	if host == 0 || parent == 0 || width <= 0 || height <= 0 {
		return
	}
	isZoomed, _, _ := procIsZoomed.Call(parent)
	isFullScreen := isZoomed != 0
	if !isFullScreen {
		var winRect, clientRect [4]int32
		if okW, _, _ := procGetWindowRect.Call(parent, uintptr(unsafe.Pointer(&winRect[0]))); okW != 0 {
			if okC, _, _ := procGetPlayerClientRect.Call(parent, uintptr(unsafe.Pointer(&clientRect[0]))); okC != 0 {
				if winRect[2]-winRect[0] == clientRect[2] && winRect[3]-winRect[1] == clientRect[3] {
					isFullScreen = true
				}
			}
		}
	}

	radius := max(6, int32(math.Round(8*scale)))
	if isFullScreen {
		radius = 0
	}
	shape := [3]int32{width, height, radius}
	if value, ok := playerSurfaces.Load(host); ok {
		state := value.(*playerSurfaceState)
		state.mu.Lock()
		unchanged := state.shape == shape
		state.shape = shape
		state.mu.Unlock()
		if unchanged {
			return
		}
	}
	if isFullScreen {
		procSetWindowRgn.Call(host, 0, 1)
		if procDwmSetWindowAttribute.Find() == nil {
			pref := int32(1) // DWMWCP_DONOTROUND
			procDwmSetWindowAttribute.Call(host, 33, uintptr(unsafe.Pointer(&pref)), 4)
		}
	} else {
		diameter := radius * 2
		rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(width+1), uintptr(height+1), uintptr(diameter), uintptr(diameter))
		if rgn != 0 {
			if accepted, _, _ := procSetWindowRgn.Call(host, rgn, 1); accepted == 0 {
				procDeleteObject.Call(rgn)
			}
		}
		if procDwmSetWindowAttribute.Find() == nil {
			pref := int32(2) // DWMWCP_ROUND
			procDwmSetWindowAttribute.Call(host, 33, uintptr(unsafe.Pointer(&pref)), 4)
		}
	}
}

func updatePlayerOverlayShapes(parent, header, overlay uintptr) {
	if parent == 0 {
		return
	}
	isZoomed, _, _ := procIsZoomed.Call(parent)
	isFullScreen := isZoomed != 0
	if !isFullScreen {
		var winRect, clientRect [4]int32
		if okW, _, _ := procGetWindowRect.Call(parent, uintptr(unsafe.Pointer(&winRect[0]))); okW != 0 {
			if okC, _, _ := procGetPlayerClientRect.Call(parent, uintptr(unsafe.Pointer(&clientRect[0]))); okC != 0 {
				if winRect[2]-winRect[0] == clientRect[2] && winRect[3]-winRect[1] == clientRect[3] {
					isFullScreen = true
				}
			}
		}
	}

	if isFullScreen {
		if header != 0 {
			procSetWindowRgn.Call(header, 0, 1)
		}
		if overlay != 0 {
			procSetWindowRgn.Call(overlay, 0, 1)
		}
		return
	}

	scale := windowScale(parent)
	radius := int32(math.Round(8 * scale))
	if radius < 6 {
		radius = 6
	}
	diameter := radius * 2

	var parentRect [4]int32
	procGetPlayerClientRect.Call(parent, uintptr(unsafe.Pointer(&parentRect[0])))
	fullH := parentRect[3]

	if overlay != 0 {
		var ovRect [4]int32
		if ok, _, _ := procGetPlayerClientRect.Call(overlay, uintptr(unsafe.Pointer(&ovRect[0]))); ok != 0 && ovRect[2] > 0 && ovRect[3] > 0 {
			w := ovRect[2]
			h := ovRect[3]
			parentH := fullH
			if parentH < h {
				parentH = h
			}
			top := -(parentH - h)
			roundRgn, _, _ := procCreateRoundRectRgn.Call(0, uintptr(top), uintptr(w+1), uintptr(h+1), uintptr(diameter), uintptr(diameter))
			rectRgn, _, _ := procCreateRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1))
			destRgn, _, _ := procCreateRectRgn.Call(0, 0, 0, 0)
			procCombineRgn.Call(destRgn, roundRgn, rectRgn, 1) // RGN_AND
			procDeleteObject.Call(roundRgn)
			procDeleteObject.Call(rectRgn)
			if accepted, _, _ := procSetWindowRgn.Call(overlay, destRgn, 1); accepted == 0 {
				procDeleteObject.Call(destRgn)
			}
		}
	}

	if header != 0 {
		var hdRect [4]int32
		if ok, _, _ := procGetPlayerClientRect.Call(header, uintptr(unsafe.Pointer(&hdRect[0]))); ok != 0 && hdRect[2] > 0 && hdRect[3] > 0 {
			w := hdRect[2]
			h := hdRect[3]
			parentH := fullH
			if parentH < h {
				parentH = h
			}
			roundRgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(parentH+1), uintptr(diameter), uintptr(diameter))
			rectRgn, _, _ := procCreateRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1))
			destRgn, _, _ := procCreateRectRgn.Call(0, 0, 0, 0)
			procCombineRgn.Call(destRgn, roundRgn, rectRgn, 1) // RGN_AND
			procDeleteObject.Call(roundRgn)
			procDeleteObject.Call(rectRgn)
			if accepted, _, _ := procSetWindowRgn.Call(header, destRgn, 1); accepted == 0 {
				procDeleteObject.Call(destRgn)
			}
		}
	}
}

func showPlayerSurface(host uintptr) {
	if host != 0 {
		owner, _, _ := procGetPlayerParent.Call(host)
		if minimized, _, _ := procIsIconic.Call(owner); minimized != 0 {
			hidePlayerSurface(host)
			return
		}
		fitPlayerSurface(host)
		procSetWindowPos.Call(host, 0, 0, 0, 0, 0, 0x0010|0x0001|0x0002) // HWND_TOP, preserve bounds, no activation
		procShowPlayerWindow.Call(host, 4)                               // SW_SHOWNOACTIVATE
	}
}

func destroyPlayerSurface(host uintptr) {
	if host != 0 {
		if val, ok := playerSurfaces.LoadAndDelete(host); ok {
			state := val.(*playerSurfaceState)
			if state.originalProc != 0 {
				procSetWindowLongPtrW.Call(host, ^uintptr(3), state.originalProc)
			}
		}
		procDestroyPlayerWindow.Call(host)
	}
}

func hidePlayerSurface(host uintptr) {
	if host != 0 {
		procShowPlayerWindow.Call(host, 0)
	}
}

func placePlayerOverlayAboveSurface(video, overlay uintptr) {
	if video == 0 || overlay == 0 {
		return
	}
	owner, _, _ := procGetPlayerParent.Call(video)
	foreground, _, _ := procGetForegroundWindow.Call()
	rootOwner, _, _ := procGetAncestor.Call(foreground, 3) // GA_ROOTOWNER
	if rootOwner != owner {
		return // never raise playback overlays over an unrelated foreground app
	}
	// Pointer activity reaches this function frequently. Keep the first overlay
	// above the video without issuing redundant Z-order updates each time.
	hwnd, _, _ := procGetPlayerWindow.Call(video, 3)
	for i := 0; hwnd != 0 && i < 1000; i++ {
		if hwnd == overlay {
			return
		}
		hwnd, _, _ = procGetPlayerWindow.Call(hwnd, 3)
	}
	procSetWindowPos.Call(overlay, 0, 0, 0, 0, 0, 0x0010|0x0001|0x0002) // HWND_TOP, no activation
}

func windowScale(handle uintptr) float64 {
	if handle == 0 {
		return 1
	}
	if dpi, _, _ := procGetDPIForWindow.Call(handle); dpi > 0 {
		return float64(dpi) / 96
	}
	return 1
}

func findMyGoSurface(parent uintptr) uintptr {
	className := make([]uint16, 128)
	length, _, _ := procGetClassNameW.Call(parent, uintptr(unsafe.Pointer(&className[0])), uintptr(len(className)))
	if length > 0 && syscall.UTF16ToString(className[:length]) == "MyGoSurface" {
		return parent
	}
	return findPlayerChildWindow(parent, "MyGoSurface")
}

func findMyGoCaptionButtons(parent uintptr) uintptr {
	return findPlayerChildWindow(parent, "MyGoCaption")
}

func findPlayerChildWindow(parent uintptr, className string) uintptr {
	if parent == 0 {
		return 0
	}
	// MyGo creates both widgets directly under its top-level HWND. A new
	// syscall callback per enumeration would permanently occupy a Go callback slot.
	class, _ := syscall.UTF16PtrFromString(className)
	child, _, _ := procFindWindowExW.Call(parent, 0, uintptr(unsafe.Pointer(class)), 0)
	return child
}

func openMpvLibrary(path string) (func(string) (uintptr, error), func(), error) {
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, nil, err
	}
	return func(name string) (uintptr, error) {
		proc, err := dll.FindProc(name)
		if err != nil {
			return 0, err
		}
		return proc.Addr(), nil
	}, func() { _ = dll.Release() }, nil
}

const playerMpvFilename = "libmpv-2.dll"
const playerResourcePlatform = "windows-amd64"

func playerMpvHost(parent uintptr) uintptr { return findMyGoSurface(parent) }

func playerMpvOptions(host uintptr) [][2]string {
	return [][2]string{
		{"vo", "gpu-next"}, {"gpu-context", "d3d11"}, {"ao", "wasapi"},
		{"force-window", "yes"}, {"wid", strconv.FormatUint(uint64(host), 10)},
	}
}

func startMpvRenderer(string, uintptr, uintptr) (func(), error) { return nil, nil }

func readCBytes(address uintptr, size int) ([]byte, bool) {
	if address == 0 || size <= 0 {
		return nil, false
	}
	buffer := make([]byte, size)
	var read uintptr
	process, _ := syscall.GetCurrentProcess()
	ok, _, _ := procReadProcessMemory.Call(uintptr(process), address,
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), uintptr(unsafe.Pointer(&read)))
	return buffer, ok != 0 && read == uintptr(size)
}

func watchPlayerPointer(done <-chan struct{}, host uintptr, activity, finished chan<- struct{}) {
	defer close(finished)
	ticker := time.NewTicker(35 * time.Millisecond)
	defer ticker.Stop()
	type point struct{ X, Y int32 }
	var previous point
	seen := false
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		var cursor point
		if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); ok == 0 || seen && cursor == previous {
			continue
		}
		previous, seen = cursor, true
		var rect [4]int32
		if ok, _, _ := procGetWindowRect.Call(host, uintptr(unsafe.Pointer(&rect[0]))); ok == 0 || cursor.X < rect[0] || cursor.X >= rect[2] || cursor.Y < rect[1] || cursor.Y >= rect[3] {
			continue
		}
		select {
		case activity <- struct{}{}:
		default:
		}
	}
}

func cString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var result strings.Builder
	for offset := uintptr(0); ; offset += 128 {
		buffer, ok := readCBytes(ptr+offset, 128)
		if !ok {
			break
		}
		if end := strings.IndexByte(string(buffer), 0); end >= 0 {
			result.Write(buffer[:end])
			break
		}
		result.Write(buffer)
	}
	return result.String()
}
