//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

var (
	user32Player           = syscall.NewLazyDLL("user32.dll")
	procEnumChildWindows   = user32Player.NewProc("EnumChildWindows")
	procGetWindowThreadPID = user32Player.NewProc("GetWindowThreadProcessId")
	procGetDPIForWindow    = user32Player.NewProc("GetDpiForWindow")
	procGetClassNameW      = user32Player.NewProc("GetClassNameW")
	procGetWindowRect      = user32Player.NewProc("GetWindowRect")
	procGetCursorPos       = user32Player.NewProc("GetCursorPos")
	procSetWindowPos       = user32Player.NewProc("SetWindowPos")
	procIsWindow           = user32Player.NewProc("IsWindow")
)

type playerProcess struct {
	cmd          *exec.Cmd
	pipeName     string
	parent       uintptr
	child        uintptr
	pid          uint32
	viewport     ui.Rect
	done         chan struct{}
	ipcMu        sync.Mutex
	writeMu      sync.Mutex
	ipc          *os.File
	positionBits atomic.Uint64
	durationBits atomic.Uint64
	positionSeen atomic.Bool
	loaded       chan struct{}
	loadOnce     sync.Once
	pointerDone  chan struct{}
	stateMu      sync.RWMutex
	state        PlayerSnapshot
	exitStatus   string
	logPath      string
}

func (p *playerProcess) start(binary, streamURL string, parent uintptr, resumeAt float64) error {
	// MyGo paints native UI into its MyGoSurface child HWND. Hosting mpv in
	// the top-level window puts the UI surface in front of the video, leaving
	// a black viewport even while mpv decodes. Give mpv the drawing surface.
	surface := findMyGoSurface(parent)
	if surface == 0 {
		return fmt.Errorf("无法找到 MyGo 原生绘制区域，无法嵌入播放器")
	}
	parent = surface
	pipeName := fmt.Sprintf(`\\.\pipe\fnmovie-mpv-%d-%d`, os.Getpid(), time.Now().UnixNano())
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("定位播放器日志目录失败：%w", err)
	}
	configDir = filepath.Join(configDir, "FnMovie")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("创建播放器日志目录失败：%w", err)
	}
	logPath := filepath.Join(configDir, "mpv.log")
	mpvConfigDir := filepath.Join(configDir, "mpv")
	if err := os.MkdirAll(mpvConfigDir, 0700); err != nil {
		return fmt.Errorf("创建播放器配置目录失败：%w", err)
	}
	oscScript := filepath.Join(filepath.Dir(binary), "scripts", "osc.lua")
	if _, err := os.Stat(oscScript); err != nil {
		return fmt.Errorf("找不到 mpv 原生控制栏脚本：%w", err)
	}
	args := []string{
		"--force-window=yes", "--no-border", "--osc=no", "--osd-level=1", "--cursor-autohide=2600", "--keepaspect=yes", "--input-default-bindings=yes", "--input-cursor=yes",
		// Prefer Windows D3D11 hardware decoding when the GPU and codec support
		// it. mpv keeps its software decoder as a fallback when initialization
		// fails, which is safer than forcing a specific decoder.
		"--hwdec=auto-safe", "--gpu-context=d3d11",
		"--script=" + oscScript,
		"--script-opts=osc-layout=floating,osc-icon_style=fluent,osc-floatingalpha=92,osc-background_color=#1C1C1E,osc-timecode_color=#F5F5F7,osc-buttons_color=#F5F5F7,osc-small_buttonsL_color=#D1D1D6,osc-small_buttonsR_color=#D1D1D6,osc-title_color=#F5F5F7,osc-visibility=auto,osc-deadzonesize=0,osc-hidetimeout=2500,osc-fadeduration=220,osc-fadein=yes,osc-custom_button_1_content=返回,osc-custom_button_1_mbtn_left_command=quit",
		"--no-config", "--config-dir=" + mpvConfigDir,
		"--wid=" + strconv.FormatUint(uint64(parent), 10),
		"--input-ipc-server=" + pipeName,
		"--title=飞牛影视播放器",
		"--log-file=" + logPath, "--msg-level=all=warn",
	}
	if resumeAt > 0 {
		args = append(args, "--start="+strconv.FormatFloat(resumeAt, 'f', 2, 64))
	}
	args = append(args, streamURL)
	cmd := exec.Command(binary, args...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 mpv 失败：%w", err)
	}
	p.cmd, p.pipeName, p.parent, p.pid, p.done = cmd, pipeName, parent, uint32(cmd.Process.Pid), make(chan struct{})
	p.loaded, p.loadOnce, p.logPath = make(chan struct{}), sync.Once{}, logPath
	p.stateMu.Lock()
	p.state = PlayerSnapshot{}
	p.stateMu.Unlock()
	p.positionBits.Store(math.Float64bits(resumeAt))
	p.durationBits.Store(0)
	p.positionSeen.Store(false)
	go func() {
		waitErr := cmd.Wait()
		p.stateMu.Lock()
		if waitErr != nil {
			p.exitStatus = waitErr.Error()
		} else {
			p.exitStatus = "exit code 0"
		}
		p.stateMu.Unlock()
		close(p.done)
	}()
	go p.connectIPC()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if p.child = findPlayerChild(parent, p.pid); p.child != 0 {
				p.setViewport(p.viewport)
				return
			}
			select {
			case <-p.done:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
	select {
	case <-p.loaded:
		// The native OSC is auto-hidden during playback. Reveal it once after
		// the first frame so the user immediately sees the transport controls.
		// The OSC then returns to its configured 2.5-second auto-hide behavior.
		for attempt := 0; attempt < 10; attempt++ {
			if err := p.sendCommand(map[string]any{"command": []any{"script-message-to", "osc", "osc-show"}}); err == nil {
				break
			}
			if !p.running() {
				return fmt.Errorf("mpv exited while showing playback controls: %s", p.logSummary())
			}
			time.Sleep(100 * time.Millisecond)
		}
		p.pointerDone = make(chan struct{})
		go p.watchPointer(p.done, p.parent, p.pid, p.pointerDone)
		return nil
	case <-p.done:
		return fmt.Errorf("mpv 未能载入媒体：%s", p.logSummary())
	case <-time.After(25 * time.Second):
		return fmt.Errorf("等待 mpv 载入媒体超时：%s", p.logSummary())
	}
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

func (p *playerProcess) stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	quitErr := p.sendCommand(map[string]any{"command": []any{"quit"}})
	if quitErr != nil {
		// If the IPC pipe is wedged, stop the owned process directly instead
		// of leaving the UI blocked while trying to write a graceful quit.
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	case <-time.After(func() time.Duration {
		if quitErr != nil {
			return 2 * time.Second
		}
		return 1500 * time.Millisecond
	}()):
		_ = p.cmd.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
			_ = exec.Command("taskkill.exe", "/PID", strconv.Itoa(p.cmd.Process.Pid), "/T", "/F").Run()
			select {
			case <-p.done:
			case <-time.After(2 * time.Second):
				log.Printf("mpv process %d did not exit after forced shutdown", p.cmd.Process.Pid)
			}
		}
	}
	if p.pointerDone != nil {
		select {
		case <-p.pointerDone:
		case <-time.After(2300 * time.Millisecond):
		}
		p.pointerDone = nil
	}
	p.ipcMu.Lock()
	if p.ipc != nil {
		_ = p.ipc.Close()
		p.ipc = nil
	}
	p.ipcMu.Unlock()
	p.cmd = nil
	p.child = 0
}

// watchPointer explicitly wakes the mpv OSC when the system pointer moves over
// the embedded video child. Some Windows hosts do not forward MOUSE_MOVE to a
// child window consistently after the OSC hides; this keeps the normal mpv
// auto-hide behavior while making the controls reliably recoverable.
func (p *playerProcess) watchPointer(done <-chan struct{}, parent uintptr, pid uint32, finished chan<- struct{}) {
	defer close(finished)
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	type point struct{ X, Y int32 }
	var last point
	hasLast := false
	lastShow := time.Time{}
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		select {
		case <-done:
			return
		default:
		}

		var cursor point
		if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); ok == 0 {
			continue
		}
		moved := !hasLast || cursor != last
		last, hasLast = cursor, true
		if !moved {
			continue
		}

		child := findPlayerChild(parent, pid)
		if child == 0 {
			continue
		}
		var rect [4]int32
		if ok, _, _ := procGetWindowRect.Call(child, uintptr(unsafe.Pointer(&rect[0]))); ok == 0 {
			continue
		}
		if cursor.X < rect[0] || cursor.X >= rect[2] || cursor.Y < rect[1] || cursor.Y >= rect[3] {
			continue
		}
		if time.Since(lastShow) < 90*time.Millisecond {
			continue
		}
		p.command("script-message-to", "osc", "osc-show")
		lastShow = time.Now()
	}
}

func (p *playerProcess) command(args ...any) {
	if err := p.sendCommand(map[string]any{"command": args}); err != nil {
		name := "unknown"
		if len(args) > 0 {
			if command, ok := args[0].(string); ok {
				name = command
			}
		}
		log.Printf("mpv IPC %s command failed: %v", name, err)
	}
}

func (p *playerProcess) sendCommand(payload map[string]any) error {
	if p.pipeName == "" || !p.running() {
		return fmt.Errorf("mpv is not running")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.ipcMu.Lock()
	file := p.ipc
	p.ipcMu.Unlock()
	if file == nil {
		return fmt.Errorf("mpv control channel is not ready")
	}
	written := make(chan error, 1)
	go func() {
		_, writeErr := file.Write(data)
		written <- writeErr
	}()
	select {
	case err = <-written:
		if err != nil {
			return err
		}
	case <-p.done:
		return fmt.Errorf("mpv exited before accepting the command")
	case <-time.After(2 * time.Second):
		p.ipcMu.Lock()
		if p.ipc == file {
			p.ipc = nil
		}
		p.ipcMu.Unlock()
		_ = file.Close() // Closing the Windows pipe interrupts a blocked write.
		return fmt.Errorf("mpv control channel write timed out")
	}
	return nil
}

func (p *playerProcess) connectIPC() {
	for p.running() {
		file, err := os.OpenFile(p.pipeName, os.O_RDWR, 0)
		if err != nil {
			select {
			case <-p.done:
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		p.ipcMu.Lock()
		if !p.running() {
			p.ipcMu.Unlock()
			_ = file.Close()
			return
		}
		p.ipc = file
		p.ipcMu.Unlock()
		_ = p.sendCommand(map[string]any{"command": []any{"observe_property", 1, "time-pos"}})
		_ = p.sendCommand(map[string]any{"command": []any{"observe_property", 2, "duration"}})
		_ = p.sendCommand(map[string]any{"command": []any{"observe_property", 3, "pause"}})
		_ = p.sendCommand(map[string]any{"command": []any{"observe_property", 4, "volume"}})
		_ = p.sendCommand(map[string]any{"command": []any{"observe_property", 5, "track-list"}})
		p.readIPC(file)
		p.ipcMu.Lock()
		if p.ipc == file {
			p.ipc = nil
		}
		p.ipcMu.Unlock()
		_ = file.Close()
		select {
		case <-p.done:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (p *playerProcess) readIPC(file *os.File) {
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var event struct {
			Event  string          `json:"event"`
			Name   string          `json:"name"`
			Data   json.RawMessage `json:"data"`
			Reason string          `json:"reason"`
			Error  string          `json:"error"`
		}
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		switch event.Event {
		case "file-loaded", "playback-restart":
			p.stateMu.Lock()
			p.state.Loaded = true
			p.state.Error = ""
			p.stateMu.Unlock()
			p.loadOnce.Do(func() { close(p.loaded) })
			continue
		case "end-file":
			log.Printf("mpv end-file: reason=%s error=%s", event.Reason, event.Error)
			if event.Reason == "error" {
				message := strings.TrimSpace(event.Error)
				if message == "" {
					message = p.logSummary()
				}
				p.stateMu.Lock()
				p.state.Error = message
				p.stateMu.Unlock()
			}
			continue
		case "property-change":
		default:
			continue
		}
		if event.Name == "track-list" {
			p.updateTracks(event.Data)
			continue
		}
		var value float64
		switch event.Name {
		case "time-pos":
			if json.Unmarshal(event.Data, &value) == nil {
				p.positionBits.Store(math.Float64bits(value))
				p.positionSeen.Store(true)
				p.stateMu.Lock()
				p.state.Loaded = true
				p.stateMu.Unlock()
				p.loadOnce.Do(func() { close(p.loaded) })
			}
		case "duration":
			if json.Unmarshal(event.Data, &value) == nil {
				p.durationBits.Store(math.Float64bits(value))
			}
		case "pause":
			var paused bool
			if json.Unmarshal(event.Data, &paused) == nil {
				p.stateMu.Lock()
				p.state.Paused = paused
				p.stateMu.Unlock()
			}
		case "volume":
			if json.Unmarshal(event.Data, &value) == nil {
				p.stateMu.Lock()
				p.state.Volume = value
				p.stateMu.Unlock()
			}
		}
	}
}

func (p *playerProcess) updateTracks(data json.RawMessage) {
	var values []map[string]any
	if json.Unmarshal(data, &values) != nil {
		return
	}
	audio, subtitles := make([]PlayerTrack, 0), make([]PlayerTrack, 0)
	for _, value := range values {
		track := PlayerTrack{
			ID: int(intValue(value["id"])), Title: firstString(value, "title", "lang", "codec"),
			Language: firstString(value, "lang"), Selected: anyBool(value["selected"]), External: anyBool(value["external"]),
		}
		if name := firstString(value, "title"); name != "" && track.Language != "" && name != track.Language {
			track.Title = name + " · " + track.Language
		}
		switch strings.ToLower(firstString(value, "type")) {
		case "audio":
			audio = append(audio, track)
		case "sub":
			subtitles = append(subtitles, track)
		}
	}
	p.stateMu.Lock()
	p.state.AudioTracks, p.state.SubtitleTracks = audio, subtitles
	p.stateMu.Unlock()
}

func intValue(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

func (p *playerProcess) snapshot() PlayerSnapshot {
	position, duration, _ := p.position()
	p.stateMu.RLock()
	state := p.state
	state.AudioTracks = append([]PlayerTrack(nil), state.AudioTracks...)
	state.SubtitleTracks = append([]PlayerTrack(nil), state.SubtitleTracks...)
	p.stateMu.RUnlock()
	state.Position, state.Duration = position, duration
	if state.Duration <= 0 {
		state.Duration = duration
	}
	return state
}

func (p *playerProcess) logSummary() string {
	data, err := os.ReadFile(p.logPath)
	if err != nil {
		return "请检查 NAS 网络及视频格式"
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 600 {
		text = text[len(text)-600:]
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "请检查 NAS 网络及视频格式"
	}
	return text
}

func (p *playerProcess) exitSummary() string {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	if p.exitStatus == "" {
		return "process still running"
	}
	return p.exitStatus
}

func (p *playerProcess) position() (float64, float64, bool) {
	return math.Float64frombits(p.positionBits.Load()), math.Float64frombits(p.durationBits.Load()), p.positionSeen.Load()
}

func (p *playerProcess) setViewport(rect ui.Rect) {
	p.viewport = rect
	if p.child == 0 || procIsWindow.Find() != nil {
		return
	}
	valid, _, _ := procIsWindow.Call(p.child)
	if valid == 0 {
		p.child = findPlayerChild(p.parent, p.pid)
	}
	if p.child == 0 {
		return
	}
	dpi := uintptr(96)
	if value, _, _ := procGetDPIForWindow.Call(p.parent); value > 0 {
		dpi = value
	}
	scale := float64(dpi) / 96.0
	x, y := int32(float64(rect.X)*scale), int32(float64(rect.Y)*scale)
	w, h := int32(float64(rect.W)*scale), int32(float64(rect.H)*scale)
	const swpNoActivate = 0x0010
	const swpShowWindow = 0x0040
	procSetWindowPos.Call(p.child, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate|swpShowWindow)
	if os.Getenv("FNMOVIE_DEBUG_PLAYER_WINDOW") == "1" {
		var hostRect, childRect [4]int32
		procGetWindowRect.Call(p.parent, uintptr(unsafe.Pointer(&hostRect[0])))
		procGetWindowRect.Call(p.child, uintptr(unsafe.Pointer(&childRect[0])))
		log.Printf("mpv embed geometry: host=%v child=%v viewport=%v dpi=%d", hostRect, childRect, rect, dpi)
	}
}

func findPlayerChild(parent uintptr, pid uint32) uintptr {
	var result uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var processID uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
		if processID == pid {
			result = hwnd
			return 0
		}
		return 1
	})
	procEnumChildWindows.Call(parent, callback, 0)
	return result
}

func findMyGoSurface(parent uintptr) uintptr {
	var result uintptr
	debug := os.Getenv("FNMOVIE_DEBUG_PLAYER_WINDOW") == "1"
	if debug {
		var parentClass [128]uint16
		length, _, _ := procGetClassNameW.Call(parent, uintptr(unsafe.Pointer(&parentClass[0])), uintptr(len(parentClass)))
		log.Printf("looking for MyGo surface under hwnd=%#x class=%q", parent, syscall.UTF16ToString(parentClass[:length]))
	}
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		className := make([]uint16, 128)
		length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&className[0])), uintptr(len(className)))
		name := syscall.UTF16ToString(className[:length])
		if debug {
			log.Printf("player host child hwnd=%#x class=%q", hwnd, name)
		}
		if length > 0 && name == "MyGoSurface" {
			result = hwnd
			return 0
		}
		return 1
	})
	procEnumChildWindows.Call(parent, callback, 0)
	return result
}
