//go:build windows || darwin

package app

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

type mpvAPI struct {
	create          func() uintptr
	setOption       func(uintptr, *byte, *byte) int32
	initialize      func(uintptr) int32
	command         func(uintptr, **byte) int32
	commandAsync    func(uintptr, uint64, **byte) int32
	observeProperty func(uintptr, uint64, *byte, int32) int32
	waitEvent       func(uintptr, float64) uintptr
	getProperty     func(uintptr, *byte, int32, unsafe.Pointer) int32
	getPropertyStr  func(uintptr, *byte) uintptr
	free            func(uintptr)
	freeNode        func(*mpvNode)
	wakeup          func(uintptr)
	terminate       func(uintptr)
	errorString     func(int32) uintptr
}

var mpvLoadOnce sync.Once
var mpvLoaded *mpvAPI
var mpvLoadErr error

func loadMpvAPI(path string) (*mpvAPI, error) {
	mpvLoadOnce.Do(func() {
		lookup, closeLib, err := openMpvLibrary(path)
		if err != nil {
			mpvLoadErr = fmt.Errorf("加载 libmpv 失败：%w", err)
			return
		}
		api := &mpvAPI{}
		functions := []struct {
			name string
			fn   any
		}{
			{"mpv_create", &api.create}, {"mpv_set_option_string", &api.setOption}, {"mpv_initialize", &api.initialize},
			{"mpv_command", &api.command}, {"mpv_wait_event", &api.waitEvent}, {"mpv_get_property", &api.getProperty},
			{"mpv_command_async", &api.commandAsync}, {"mpv_observe_property", &api.observeProperty},
			{"mpv_get_property_string", &api.getPropertyStr}, {"mpv_free", &api.free}, {"mpv_free_node_contents", &api.freeNode},
			{"mpv_wakeup", &api.wakeup}, {"mpv_terminate_destroy", &api.terminate}, {"mpv_error_string", &api.errorString},
		}
		for _, function := range functions {
			address, err := lookup(function.name)
			if err != nil {
				mpvLoadErr = fmt.Errorf("加载 libmpv 接口 %s 失败：%w", function.name, err)
				closeLib()
				return
			}
			purego.RegisterFunc(function.fn, address)
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

type playerPoint struct{ x, y int32 }

const (
	mpvFormatFlag  = 3
	mpvFormatInt64 = 4

	mpvFormatNode               = 6
	mpvEventFileLoaded          = 8
	mpvEventPropertyChange      = 22
	mpvTrackListObserveUserdata = 1
)

func mpvNodeFlag(node mpvNode) bool {
	return node.format == mpvFormatFlag && uint32(node.value) != 0
}

type playerTrackRefreshState struct {
	dirty bool
}

func (s *playerTrackRefreshState) observe(eventID int32, userdata uint64) {
	switch eventID {
	case mpvEventFileLoaded:
		s.dirty = true
	case mpvEventPropertyChange:
		if userdata == mpvTrackListObserveUserdata {
			s.dirty = true
		}
	}
}

func (s playerTrackRefreshState) due() bool { return s.dirty }

func (s *playerTrackRefreshState) refreshed() { s.dirty = false }

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
	commandMu         sync.RWMutex
	queueMu           sync.Mutex
	commands          [][]any
	commandReady      chan struct{}
	commandDone       chan struct{}
	api               *mpvAPI
	ctx               uintptr
	host              uintptr
	done              chan struct{}
	loaded            chan struct{}
	loadOnce          sync.Once
	events            chan struct{}
	pointerEvents     chan struct{}
	pointerDone       chan struct{}
	stateMu           sync.RWMutex
	state             PlayerSnapshot
	exitStatus        string
	positionBits      uint64
	durationBits      uint64
	positionSeen      bool
	trackRefresh      playerTrackRefreshState
	trackListObserved bool
	releaseRender     func()
	stopOnce          sync.Once
	resumeAt          float64
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
	host := playerMpvHost(parent)
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
		{"config", "no"}, {"config-dir", configDir},
		{"hwdec", "auto-safe"}, {"audio-channels", "auto-safe"},
		{"osc", "no"}, {"osd-level", "0"}, {"input-default-bindings", "no"}, {"input-vo-keyboard", "no"},
		{"load-console", "no"}, {"load-commands", "no"}, {"load-stats-overlay", "no"}, {"load-select", "no"},
		{"keepaspect", "yes"},
		{"log-file", filepath.Join(configDir, "mpv.log")}, {"msg-level", "all=warn,ao=info,ad=info,vd=info,ffmpeg=info"},
	}
	for _, option := range append(options, playerMpvOptions(host)...) {
		if code := setMpvOption(api, ctx, option[0], option[1]); code < 0 {
			api.terminate(ctx)
			return fmt.Errorf("设置 libmpv 参数 %s 失败：%s", option[0], mpvError(api, code))
		}
	}
	if code := api.initialize(ctx); code < 0 {
		api.terminate(ctx)
		return fmt.Errorf("初始化 libmpv 失败：%s", mpvError(api, code))
	}
	releaseRender, err := startMpvRenderer(dllPath, ctx, host)
	if err != nil {
		api.terminate(ctx)
		return err
	}
	p.resetSession(api, ctx, host, resumeAt)
	p.releaseRender = releaseRender
	p.commandReady, p.commandDone = make(chan struct{}, 1), make(chan struct{})
	p.trackListObserved = p.observeTrackList()
	go p.eventLoop()
	go p.runCommands()
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
		go watchPlayerPointer(p.done, host, p.pointerEvents, p.pointerDone)
		return nil
	case <-p.done:
		if message := p.snapshot().Error; message != "" {
			return fmt.Errorf("%s", message)
		}
		return fmt.Errorf("libmpv 在媒体载入前退出")
	case <-time.After(25 * time.Second):
		p.stop()
		return fmt.Errorf("等待 libmpv 媒体载入超时")
	}
}

func (p *playerProcess) observeTrackList() bool {
	name, _ := syscall.BytePtrFromString("track-list")
	code := p.api.observeProperty(p.ctx, mpvTrackListObserveUserdata, name, mpvFormatNode)
	if code < 0 {
		log.Printf("监听 libmpv track-list 变化失败，将使用定时刷新：%s", mpvError(p.api, code))
		return false
	}
	return true
}

func (p *playerProcess) resetSession(api *mpvAPI, ctx, host uintptr, resumeAt float64) {
	p.api, p.ctx, p.host = api, ctx, host
	p.done, p.loaded, p.events = make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	p.pointerEvents, p.pointerDone, p.resumeAt = make(chan struct{}, 1), nil, resumeAt
	p.stopOnce, p.loadOnce = sync.Once{}, sync.Once{}
	p.stateMu.Lock()
	p.exitStatus = ""
	p.positionBits, p.durationBits, p.positionSeen = 0, 0, false
	p.trackRefresh = playerTrackRefreshState{}
	p.trackListObserved = false
	p.state = PlayerSnapshot{Volume: 100, Speed: 1}
	p.stateMu.Unlock()
}

func setMpvOption(api *mpvAPI, ctx uintptr, name, value string) int32 {
	n, _ := syscall.BytePtrFromString(name)
	v, _ := syscall.BytePtrFromString(value)
	return api.setOption(ctx, n, v)
}

func mpvError(api *mpvAPI, code int32) string {
	ptr := api.errorString(code)
	if ptr == 0 {
		return fmt.Sprintf("错误 %d", code)
	}
	return cString(ptr)
}

func (p *playerProcess) command(args ...any) int32 {
	p.commandMu.RLock()
	defer p.commandMu.RUnlock()
	if p.api == nil || p.ctx == 0 {
		return -20
	}
	return p.api.command(p.ctx, mpvCommandArgs(args))
}

func (p *playerProcess) queueCommand(args ...any) {
	if p.commandReady == nil || !p.running() {
		return
	}
	p.queueMu.Lock()
	p.commands = append(p.commands, args)
	p.queueMu.Unlock()
	select {
	case p.commandReady <- struct{}{}:
	default:
	}
}

// Keep control commands ordered without waiting for libmpv on the UI thread.
func (p *playerProcess) runCommands() {
	defer close(p.commandDone)
	for {
		select {
		case <-p.done:
			return
		case <-p.commandReady:
		}
		for p.running() {
			p.queueMu.Lock()
			if len(p.commands) == 0 {
				p.queueMu.Unlock()
				break
			}
			args := p.commands[0]
			p.commands[0] = nil
			p.commands = p.commands[1:]
			p.queueMu.Unlock()
			if code := p.command(args...); code < 0 {
				log.Printf("libmpv command %v: %s", args[0], mpvError(p.api, code))
			}
		}
	}
}

func mpvCommandArgs(args []any) **byte {
	ptrs := make([]*byte, len(args)+1)
	for i, arg := range args {
		value := fmt.Sprint(arg)
		if i == 0 && value == "set_property" {
			value = "set"
		}
		ptrs[i], _ = syscall.BytePtrFromString(value)
	}
	return &ptrs[0]
}

func (p *playerProcess) seekTo(seconds float64) (uint64, error) {
	p.commandMu.RLock()
	defer p.commandMu.RUnlock()
	if p.ctx == 0 {
		return 0, fmt.Errorf("播放器已关闭")
	}
	p.stateMu.Lock()
	p.state.SeekRequestID++
	request := p.state.SeekRequestID
	p.state.SeekError = ""
	p.state.SeekAcknowledgedAt = time.Time{}
	p.stateMu.Unlock()
	args := []string{"seek", strconv.FormatFloat(seconds, 'f', 3, 64), "absolute"}
	ptrs := make([]*byte, len(args)+1)
	for i, arg := range args {
		ptrs[i], _ = syscall.BytePtrFromString(arg)
	}
	code := p.api.commandAsync(p.ctx, request, &ptrs[0])
	if code < 0 {
		return request, fmt.Errorf("%s", mpvError(p.api, code))
	}
	return request, nil
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
		// Drain a bounded batch: a seek acknowledgement must not wait behind
		// startup/reconfiguration events one polling interval at a time.
		fileLoaded := false
		for batch := 0; batch < 64; batch++ {
			eventPtr := p.api.waitEvent(p.ctx, 0)
			if eventPtr == 0 {
				break
			}
			eventBytes, ok := readCBytes(eventPtr, 24)
			if !ok {
				continue
			}
			event := *(*mpvEvent)(unsafe.Pointer(&eventBytes[0]))
			if event.id == 0 {
				break
			}
			p.trackRefresh.observe(event.id, event.userdata)
			switch event.id {
			case 5: // MPV_EVENT_COMMAND_REPLY, only the newest user seek can fail its UI request.
				p.stateMu.Lock()
				if event.userdata != 0 && event.userdata == p.state.SeekRequestID {
					p.state.SeekAcknowledgedAt = time.Now()
					if event.error < 0 {
						p.state.SeekError = mpvError(p.api, event.error)
					}
				}
				p.stateMu.Unlock()
			case 1:
				return
			case 7:
				if event.data != 0 {
					endBytes, ok := readCBytes(event.data, 8)
					if !ok {
						continue
					}
					end := [2]int32{int32(binary.LittleEndian.Uint32(endBytes)), int32(binary.LittleEndian.Uint32(endBytes[4:]))}
					if p.finishMedia(end[0], end[1]) {
						return
					}
				}
			case mpvEventFileLoaded:
				fileLoaded = true
				if p.resumeAt > 0 {
					p.queueCommand("seek", strconv.FormatFloat(p.resumeAt, 'f', 2, 64), "absolute", "exact")
					p.resumeAt = 0
				}
			}
		}
		p.refreshState()
		if fileLoaded {
			p.loadOnce.Do(func() { close(p.loaded) })
		}
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
		log.Printf("libmpv playback ended with error %d: %s", code, mpvError(p.api, code))
		p.state.Error = "无法继续读取或解码视频，请关闭播放窗口后重试。"
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
	observedAt := time.Now()
	p.stateMu.RLock()
	previous := p.state
	p.stateMu.RUnlock()
	next := PlayerSnapshot{
		ObservedAt:     observedAt,
		Position:       p.getFloat("time-pos", previous.Position),
		Duration:       p.getFloat("duration", previous.Duration),
		VideoWidth:     p.getInt("dwidth", previous.VideoWidth),
		VideoHeight:    p.getInt("dheight", previous.VideoHeight),
		Seeking:        p.getFlag("seeking"),
		Buffering:      p.getFlag("paused-for-cache"),
		Paused:         p.getFlag("pause"),
		Muted:          p.getFlag("mute"),
		Volume:         p.getFloat("volume", 100),
		Speed:          p.getFloat("speed", 1),
		AudioOutput:    p.getString("current-ao"),
		AudioParams:    p.getString("audio-params"),
		AudioTracks:    previous.AudioTracks,
		SubtitleTracks: previous.SubtitleTracks,
	}
	next.Loaded = next.Duration > 0
	if p.trackRefresh.due() || !p.trackListObserved {
		if audio, subtitles, ok := p.getTracks(); ok {
			next.AudioTracks, next.SubtitleTracks = audio, subtitles
			p.trackRefresh.refreshed()
		}
	}
	// Keep cached snapshots available while libmpv waits on the decoder or demuxer.
	p.stateMu.Lock()
	next.SeekRequestID = p.state.SeekRequestID
	next.SeekError = p.state.SeekError
	next.SeekAcknowledgedAt = p.state.SeekAcknowledgedAt
	p.positionBits, p.durationBits, p.positionSeen = math.Float64bits(next.Position), math.Float64bits(next.Duration), next.Loaded
	p.state = next
	p.stateMu.Unlock()
	if next.Loaded {
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

func (p *playerProcess) getInt(name string, fallback int) int {
	var value int64
	if p.getRaw(name, mpvFormatInt64, unsafe.Pointer(&value)) < 0 || value <= 0 || value > int64(^uint(0)>>1) {
		return fallback
	}
	return int(value)
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
	ptr := p.api.getPropertyStr(p.ctx, key)
	if ptr == 0 {
		return ""
	}
	value := cString(ptr)
	p.api.free(ptr)
	return value
}

func (p *playerProcess) getRaw(name string, format int32, data unsafe.Pointer) int32 {
	key, _ := syscall.BytePtrFromString(name)
	return p.api.getProperty(p.ctx, key, format, data)
}

func (p *playerProcess) getTracks() ([]PlayerTrack, []PlayerTrack, bool) {
	var node mpvNode
	if p.getRaw("track-list", mpvFormatNode, unsafe.Pointer(&node)) < 0 {
		return nil, nil, false
	}
	defer p.api.freeNode(&node)
	if node.format != 7 || node.value == 0 {
		return nil, nil, false
	}
	listBytes, ok := readCBytes(node.value, 24)
	if !ok {
		return nil, nil, false
	}
	list := *(*mpvNodeList)(unsafe.Pointer(&listBytes[0]))
	if list.num < 0 || list.num > 1024 {
		return nil, nil, false
	}
	audio, subtitles := make([]PlayerTrack, 0), make([]PlayerTrack, 0)
	if list.num == 0 {
		return audio, subtitles, true
	}
	valueBytes, ok := readCBytes(list.values, int(list.num)*16)
	if !ok {
		return nil, nil, false
	}
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
		track := PlayerTrack{MainSelection: -1}
		kind := ""
		for i, field := range fieldValues {
			key := cString(uintptr(binary.LittleEndian.Uint64(keyBytes[i*8:])))
			str := ""
			if field.format == 1 {
				str = cString(field.value)
			}
			switch key {
			case "id":
				if field.format == mpvFormatInt64 {
					track.ID = int(int64(field.value))
				}
			case "title":
				track.Title = str
			case "lang":
				track.Language = str
			case "type":
				kind = str
			case "selected":
				track.Selected = mpvNodeFlag(field)
			case "external":
				track.External = mpvNodeFlag(field)
			case "main-selection":
				if field.format == mpvFormatInt64 {
					track.MainSelection = int(int64(field.value))
					track.HasMainSelection = true
				}
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
	normalizeSelectedTracks(audio, false)
	normalizeSelectedTracks(subtitles, true)
	return audio, subtitles, true
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
		if p.releaseRender != nil {
			p.releaseRender()
			p.releaseRender = nil
		}
		p.api.commandAsync(p.ctx, 0, mpvCommandArgs([]any{"quit"}))
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
		if p.commandDone != nil {
			<-p.commandDone
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

func (p *playerProcess) pointerActivity() <-chan struct{} { return p.pointerEvents }
