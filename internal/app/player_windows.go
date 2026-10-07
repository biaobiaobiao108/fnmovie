//go:build windows

package app

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/ui"
)

var (
	user32Player            = syscall.NewLazyDLL("user32.dll")
	procEnumChildWindows    = user32Player.NewProc("EnumChildWindows")
	procGetWindowThreadPID  = user32Player.NewProc("GetWindowThreadProcessId")
	procGetDPIForWindow     = user32Player.NewProc("GetDpiForWindow")
	procGetClassNameW       = user32Player.NewProc("GetClassNameW")
	procGetWindowRect       = user32Player.NewProc("GetWindowRect")
	procGetCursorPos        = user32Player.NewProc("GetCursorPos")
	procSetWindowPos        = user32Player.NewProc("SetWindowPos")
	procIsWindow            = user32Player.NewProc("IsWindow")
	procCreatePlayerWindow  = user32Player.NewProc("CreateWindowExW")
	procDestroyPlayerWindow = user32Player.NewProc("DestroyWindow")
	procShowPlayerWindow    = user32Player.NewProc("ShowWindow")
	procGetPlayerClientRect = user32Player.NewProc("GetClientRect")
	procGetPlayerParent     = user32Player.NewProc("GetParent")
	procGetPlayerModule     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procReadProcessMemory   = syscall.NewLazyDLL("kernel32.dll").NewProc("ReadProcessMemory")
)

type mpvAPI struct {
	dll            *syscall.DLL
	create         func() uintptr
	setOption      func(uintptr, uintptr, uintptr) int32
	initialize     func(uintptr) int32
	command        func(uintptr, uintptr) int32
	waitEvent      func(uintptr, float64) uintptr
	getProperty    func(uintptr, uintptr, int32, uintptr) int32
	getPropertyStr func(uintptr, uintptr) uintptr
	free           func(uintptr)
	freeNode       func(uintptr)
	wakeup         func(uintptr)
	terminate      func(uintptr)
	errorString    func(int32) uintptr
}

var mpvLoadOnce sync.Once
var mpvLoaded *mpvAPI
var mpvLoadErr error

func loadMpvAPI(path string) (*mpvAPI, error) {
	mpvLoadOnce.Do(func() {
		dll, err := syscall.LoadDLL(path)
		if err != nil {
			mpvLoadErr = fmt.Errorf("加载 libmpv 失败：%w", err)
			return
		}
		api := &mpvAPI{dll: dll}
		register := func(name string, target any) error {
			proc, e := dll.FindProc(name)
			if e != nil {
				return e
			}
			purego.RegisterFunc(target, proc.Addr())
			return nil
		}
		functions := []struct {
			name string
			fn   any
		}{
			{"mpv_create", &api.create}, {"mpv_set_option_string", &api.setOption}, {"mpv_initialize", &api.initialize},
			{"mpv_command", &api.command}, {"mpv_wait_event", &api.waitEvent}, {"mpv_get_property", &api.getProperty},
			{"mpv_get_property_string", &api.getPropertyStr}, {"mpv_free", &api.free}, {"mpv_free_node_contents", &api.freeNode},
			{"mpv_wakeup", &api.wakeup}, {"mpv_terminate_destroy", &api.terminate}, {"mpv_error_string", &api.errorString},
		}
		for _, function := range functions {
			if err := register(function.name, function.fn); err != nil {
				mpvLoadErr = fmt.Errorf("加载 libmpv 接口 %s 失败：%w", function.name, err)
				_ = dll.Release()
				return
			}
		}
		mpvLoaded = api
	})
	return mpvLoaded, mpvLoadErr
}

type mpvNode struct {
	value  uintptr
	format int32
	pad    int32
}

type mpvNodeList struct {
	num    int32
	pad    int32
	values uintptr
	keys   uintptr
}

type mpvEvent struct {
	id       int32
	error    int32
	userdata uint64
	data     uintptr
}

type playerProcess struct {
	commandMu     sync.Mutex
	api           *mpvAPI
	ctx           uintptr
	host          uintptr
	done          chan struct{}
	loaded        chan struct{}
	loadOnce      sync.Once
	events        chan struct{}
	pointerEvents chan struct{}
	pointerDone   chan struct{}
	stateMu       sync.RWMutex
	state         PlayerSnapshot
	exitStatus    string
	positionBits  uint64
	durationBits  uint64
	positionSeen  bool
	stopOnce      sync.Once
	viewport      ui.Rect
	resumeAt      float64
}

func (p *playerProcess) start(dllPath, streamURL string, parent uintptr, resumeAt float64) error {
	return p.startContext(context.Background(), dllPath, streamURL, parent, resumeAt)
}

func (p *playerProcess) startContext(playbackCtx context.Context, dllPath, streamURL string, parent uintptr, resumeAt float64) error {
	if err := playbackCtx.Err(); err != nil {
		return err
	}
	api, err := loadMpvAPI(dllPath)
	if err != nil {
		return err
	}
	host := findMyGoSurface(parent)
	if host == 0 {
		return fmt.Errorf("无法找到 MyGo 原生绘制区域，无法嵌入 libmpv")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("定位播放器配置目录失败：%w", err)
	}
	configDir = filepath.Join(configDir, "FnMovie", "mpv")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("创建播放器配置目录失败：%w", err)
	}
	ctx := api.create()
	if ctx == 0 {
		return fmt.Errorf("libmpv 无法创建播放器实例")
	}
	options := [][2]string{
		{"config", "no"}, {"config-dir", configDir}, {"vo", "gpu-next"}, {"gpu-context", "d3d11"},
		{"hwdec", "auto-safe"}, {"ao", "wasapi"}, {"audio-channels", "auto-safe"},
		{"osc", "no"}, {"osd-level", "0"}, {"input-default-bindings", "no"}, {"input-vo-keyboard", "no"},
		{"keepaspect", "yes"}, {"force-window", "yes"}, {"wid", strconv.FormatUint(uint64(host), 10)},
		{"log-file", filepath.Join(configDir, "mpv.log")}, {"msg-level", "all=warn,ao=info,ad=info,vd=info,ffmpeg=info"},
	}
	for _, option := range options {
		if code := setMpvOption(api, ctx, option[0], option[1]); code < 0 {
			api.terminate(ctx)
			return fmt.Errorf("设置 libmpv 参数 %s 失败：%s", option[0], mpvError(api, code))
		}
	}
	if code := api.initialize(ctx); code < 0 {
		api.terminate(ctx)
		return fmt.Errorf("初始化 libmpv 失败：%s", mpvError(api, code))
	}
	p.resetSession(api, ctx, host, resumeAt)
	go p.eventLoop()
	if code := p.command("loadfile", streamURL, "replace"); code < 0 {
		p.stop()
		return fmt.Errorf("libmpv 载入播放地址失败：%s", mpvError(api, code))
	}
	select {
	case <-playbackCtx.Done():
		p.stop()
		return playbackCtx.Err()
	case <-p.loaded:
		p.pointerDone = make(chan struct{})
		go p.watchPointer(p.done, host, p.pointerDone)
		return nil
	case <-p.done:
		return fmt.Errorf("libmpv 在媒体载入前退出")
	case <-time.After(25 * time.Second):
		p.stop()
		return fmt.Errorf("等待 libmpv 媒体载入超时")
	}
}

func (p *playerProcess) resetSession(api *mpvAPI, ctx, host uintptr, resumeAt float64) {
	p.api, p.ctx, p.host = api, ctx, host
	p.done, p.loaded, p.events = make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	p.pointerEvents, p.pointerDone, p.resumeAt = make(chan struct{}, 1), nil, resumeAt
	p.stopOnce, p.loadOnce = sync.Once{}, sync.Once{}
	p.stateMu.Lock()
	p.exitStatus = ""
	p.positionBits, p.durationBits, p.positionSeen = 0, 0, false
	p.state = PlayerSnapshot{Volume: 100, Speed: 1}
	p.stateMu.Unlock()
}

func setMpvOption(api *mpvAPI, ctx uintptr, name, value string) int32 {
	n, _ := syscall.BytePtrFromString(name)
	v, _ := syscall.BytePtrFromString(value)
	code := api.setOption(ctx, uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(v)))
	runtime.KeepAlive(n)
	runtime.KeepAlive(v)
	return code
}

func mpvError(api *mpvAPI, code int32) string {
	ptr := api.errorString(code)
	if ptr == 0 {
		return fmt.Sprintf("错误 %d", code)
	}
	return cString(ptr)
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

func (p *playerProcess) command(args ...any) int32 {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	if p.api == nil || p.ctx == 0 {
		return -20
	}
	stringsArg := make([]string, 0, len(args))
	for _, arg := range args {
		stringsArg = append(stringsArg, fmt.Sprint(arg))
	}
	if len(stringsArg) > 0 && stringsArg[0] == "set_property" {
		stringsArg[0] = "set"
	}
	ptrs := make([]*byte, len(stringsArg)+1)
	for i, arg := range stringsArg {
		ptrs[i], _ = syscall.BytePtrFromString(arg)
	}
	code := p.api.command(p.ctx, uintptr(unsafe.Pointer(&ptrs[0])))
	runtime.KeepAlive(ptrs)
	return code
}

func (p *playerProcess) eventLoop() {
	defer func() {
		p.stateMu.Lock()
		if p.exitStatus == "" {
			p.exitStatus = "libmpv event loop exited"
		}
		p.stateMu.Unlock()
		close(p.done)
	}()
	ticker := time.NewTicker(180 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.events:
			return
		case <-ticker.C:
		}
		if p.ctx == 0 {
			return
		}
		eventPtr := p.api.waitEvent(p.ctx, 0.02)
		if eventPtr != 0 {
			eventBytes, ok := readCBytes(eventPtr, 24)
			if !ok {
				continue
			}
			event := *(*mpvEvent)(unsafe.Pointer(&eventBytes[0]))
			switch event.id {
			case 1:
				return
			case 7:
				if event.data != 0 {
					endBytes, _ := readCBytes(event.data, 8)
					if len(endBytes) < 8 {
						continue
					}
					end := [2]int32{int32(binary.LittleEndian.Uint32(endBytes)), int32(binary.LittleEndian.Uint32(endBytes[4:]))}
					if p.finishMedia(end[0], end[1]) {
						return
					}
				}
			case 8:
				if p.resumeAt > 0 {
					if code := p.command("seek", strconv.FormatFloat(p.resumeAt, 'f', 2, 64), "absolute", "exact"); code < 0 {
						log.Printf("libmpv resume seek failed: %s", mpvError(p.api, code))
					}
					p.resumeAt = 0
				}
				p.loadOnce.Do(func() { close(p.loaded) })
			}
		}
		p.refreshState()
	}
}

// Redirects open another playlist entry; only terminal end-file events end
// this playback session. Retain the final snapshot after mpv clears properties.
func (p *playerProcess) finishMedia(reason, code int32) bool {
	if reason == 5 {
		return false
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if reason == 4 {
		p.state.Error = mpvError(p.api, code)
	}
	if reason == 0 && p.state.Duration > 0 {
		p.state.Position = p.state.Duration
		p.positionBits = math.Float64bits(p.state.Position)
		p.durationBits = math.Float64bits(p.state.Duration)
		p.positionSeen = true
	}
	p.exitStatus = fmt.Sprintf("libmpv end-file reason=%d", reason)
	return true
}

func (p *playerProcess) refreshState() {
	p.stateMu.RLock()
	previous := p.state
	p.stateMu.RUnlock()
	position := p.getFloat("time-pos", previous.Position)
	duration := p.getFloat("duration", previous.Duration)
	p.stateMu.Lock()
	p.positionBits, p.durationBits, p.positionSeen = math.Float64bits(position), math.Float64bits(duration), duration > 0
	p.state.Loaded = duration > 0
	p.state.Position, p.state.Duration = position, duration
	p.state.Paused = p.getFlag("pause")
	p.state.Muted = p.getFlag("mute")
	p.state.Volume = p.getFloat("volume", 100)
	p.state.Speed = p.getFloat("speed", 1)
	p.state.AudioOutput = p.getString("current-ao")
	p.state.AudioParams = p.getString("audio-params")
	p.state.AudioTracks, p.state.SubtitleTracks = p.getTracks()
	p.stateMu.Unlock()
	if duration > 0 {
		p.loadOnce.Do(func() { close(p.loaded) })
	}
}

func (p *playerProcess) getFloat(name string, fallback float64) float64 {
	var value float64
	if p.getRaw(name, 5, unsafe.Pointer(&value)) < 0 {
		return fallback
	}
	return value
}

func (p *playerProcess) getFlag(name string) bool {
	var value int32
	if p.getRaw(name, 3, unsafe.Pointer(&value)) < 0 {
		return false
	}
	return value != 0
}

func (p *playerProcess) getString(name string) string {
	key, _ := syscall.BytePtrFromString(name)
	ptr := p.api.getPropertyStr(p.ctx, uintptr(unsafe.Pointer(key)))
	runtime.KeepAlive(key)
	if ptr == 0 {
		return ""
	}
	value := cString(ptr)
	p.api.free(ptr)
	return value
}

func (p *playerProcess) getRaw(name string, format int32, data unsafe.Pointer) int32 {
	key, _ := syscall.BytePtrFromString(name)
	code := p.api.getProperty(p.ctx, uintptr(unsafe.Pointer(key)), format, uintptr(data))
	runtime.KeepAlive(key)
	runtime.KeepAlive(data)
	return code
}

func (p *playerProcess) getTracks() ([]PlayerTrack, []PlayerTrack) {
	var node mpvNode
	if p.getRaw("track-list", 6, unsafe.Pointer(&node)) < 0 || node.format != 7 || node.value == 0 {
		return nil, nil
	}
	defer func() {
		p.api.freeNode(uintptr(unsafe.Pointer(&node)))
		runtime.KeepAlive(&node)
	}()
	listBytes, ok := readCBytes(node.value, 24)
	if !ok {
		return nil, nil
	}
	list := *(*mpvNodeList)(unsafe.Pointer(&listBytes[0]))
	if list.num <= 0 || list.num > 1024 {
		return nil, nil
	}
	valueBytes, ok := readCBytes(list.values, int(list.num)*16)
	if !ok {
		return nil, nil
	}
	audio, subtitles := make([]PlayerTrack, 0), make([]PlayerTrack, 0)
	values := unsafe.Slice((*mpvNode)(unsafe.Pointer(&valueBytes[0])), int(list.num))
	for _, value := range values {
		if value.format != 8 || value.value == 0 {
			continue
		}
		fieldsBytes, valid := readCBytes(value.value, 24)
		if !valid {
			continue
		}
		fields := *(*mpvNodeList)(unsafe.Pointer(&fieldsBytes[0]))
		if fields.num < 0 || fields.num > 64 || fields.num == 0 {
			continue
		}
		fieldBytes, valid := readCBytes(fields.values, int(fields.num)*16)
		keyBytes, keysValid := readCBytes(fields.keys, int(fields.num)*8)
		if !valid || !keysValid {
			continue
		}
		fieldValues := unsafe.Slice((*mpvNode)(unsafe.Pointer(&fieldBytes[0])), int(fields.num))
		track := PlayerTrack{}
		kind := ""
		for i, field := range fieldValues {
			key := cString(uintptr(binary.LittleEndian.Uint64(keyBytes[i*8:])))
			str := ""
			if field.format == 1 {
				str = cString(field.value)
			}
			switch key {
			case "id":
				if field.format == 4 {
					track.ID = int(int64(field.value))
				}
			case "title":
				track.Title = str
			case "lang":
				track.Language = str
			case "type":
				kind = str
			case "selected":
				track.Selected = field.value != 0
			case "external":
				track.External = field.value != 0
			}
		}
		if track.Title == "" {
			track.Title = track.Language
		}
		if kind == "audio" {
			audio = append(audio, track)
		} else if kind == "sub" {
			subtitles = append(subtitles, track)
		}
	}
	return audio, subtitles
}

func (p *playerProcess) running() bool {
	if p.done == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *playerProcess) exitSummary() string {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	if p.exitStatus == "" {
		return "libmpv is running"
	}
	return p.exitStatus
}

func (p *playerProcess) stop() {
	p.stopOnce.Do(func() {
		if p.ctx == 0 {
			return
		}
		p.command("quit")
		p.api.wakeup(p.ctx)
		if p.events != nil {
			select {
			case p.events <- struct{}{}:
			default:
			}
		}
		if p.done != nil {
			<-p.done
		}
		p.commandMu.Lock()
		p.api.terminate(p.ctx)
		p.ctx = 0
		p.commandMu.Unlock()
		if p.pointerDone != nil {
			select {
			case <-p.pointerDone:
			case <-time.After(time.Second):
			}
		}
	})
}

func (p *playerProcess) setProperty(name string, value any) {
	if p.ctx == 0 {
		return
	}
	p.command("set", name, value)
}

func (p *playerProcess) runningCommand(args ...any) {
	if code := p.command(args...); code < 0 {
		log.Printf("libmpv command %q failed: %s", strings.Join(toStrings(args), " "), mpvError(p.api, code))
	}
}

func (p *playerProcess) watchPointer(done <-chan struct{}, host uintptr, finished chan<- struct{}) {
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
		case p.pointerEvents <- struct{}{}:
		default:
		}
	}
}

func toStrings(values []any) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = fmt.Sprint(value)
	}
	return result
}

func (p *playerProcess) position() (float64, float64, bool) {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return math.Float64frombits(p.positionBits), math.Float64frombits(p.durationBits), p.positionSeen
}

func (p *playerProcess) snapshot() PlayerSnapshot {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	state := p.state
	state.AudioTracks = append([]PlayerTrack(nil), state.AudioTracks...)
	state.SubtitleTracks = append([]PlayerTrack(nil), state.SubtitleTracks...)
	return state
}

func (p *playerProcess) setViewport(rect ui.Rect) {
	p.viewport = rect
	fitPlayerSurface(p.host)
}

// Reuse MyGo's registered surface class, but leave this child out of MyGo's
// renderer registry. Its default window procedure has no second swap chain.
func createPlayerSurface(parent uintptr) (uintptr, error) {
	parent = findMyGoSurface(parent)
	if parent == 0 {
		return 0, fmt.Errorf("无法找到 MyGo 原生绘制区域")
	}
	class, _ := syscall.UTF16PtrFromString("MyGoSurface")
	module, _, _ := procGetPlayerModule.Call(0)
	host, _, err := procCreatePlayerWindow.Call(0, uintptr(unsafe.Pointer(class)), 0,
		0x40000000|0x02000000|0x04000000, 0, 0, 0, 0, parent, 0, module, 0)
	runtime.KeepAlive(class)
	if host == 0 {
		return 0, fmt.Errorf("创建独立视频绘制区域失败：%w", err)
	}
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
		procSetWindowPos.Call(host, 0, 0, 0, uintptr(rect[2]), uintptr(rect[3]), 0x0010|0x0004)
	}
}

func showPlayerSurface(host uintptr) {
	if host != 0 {
		fitPlayerSurface(host)
		procShowPlayerWindow.Call(host, 5)
	}
}

func destroyPlayerSurface(host uintptr) {
	if host != 0 {
		procDestroyPlayerWindow.Call(host)
	}
}

func (p *playerProcess) pointerActivity() <-chan struct{} { return p.pointerEvents }

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
	var result uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		className := make([]uint16, 128)
		length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&className[0])), uintptr(len(className)))
		if length > 0 && syscall.UTF16ToString(className[:length]) == "MyGoSurface" {
			result = hwnd
			return 0
		}
		return 1
	})
	procEnumChildWindows.Call(parent, callback, 0)
	return result
}
