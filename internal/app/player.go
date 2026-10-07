package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type PlaybackState struct {
	Active         bool
	Title          string
	URL            string
	Position       float64
	Duration       float64
	Paused         bool
	Volume         float64
	Muted          bool
	Speed          float64
	AudioOutput    string
	AudioParams    string
	AudioTracks    []PlayerTrack
	SubtitleTracks []PlayerTrack
	Quality        string
	Error          string
}

type PlayerTrack struct {
	ID       int
	Title    string
	Language string
	Selected bool
	External bool
}

type PlayerSnapshot struct {
	Loaded         bool
	Position       float64
	Duration       float64
	Paused         bool
	Volume         float64
	Muted          bool
	Speed          float64
	AudioOutput    string
	AudioParams    string
	AudioTracks    []PlayerTrack
	SubtitleTracks []PlayerTrack
	Error          string
}

func (s PlaybackState) PositionText() string { return formatClock(s.Position) }
func (s PlaybackState) DurationText() string { return formatClock(s.Duration) }

func formatClock(value float64) string {
	seconds := int(value)
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

type Player struct {
	mu          sync.Mutex
	url         string
	active      bool
	position    float64
	duration    float64
	paused      bool
	viewport    ui.Rect
	started     time.Time
	process     *playerProcess
	startCancel context.CancelFunc
	generation  uint64
	surface     uintptr
}

func NewPlayer() *Player { return &Player{} }

func (p *Player) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active && p.process != nil && p.process.running()
}

func (p *Player) Start(streamURL string, parent uintptr, duration, resumeAt float64) error {
	return p.StartContext(context.Background(), streamURL, parent, duration, resumeAt)
}

func (p *Player) StartContext(ctx context.Context, streamURL string, parent uintptr, duration, resumeAt float64) error {
	p.mu.Lock()
	if p.startCancel != nil {
		p.startCancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	p.startCancel = cancel
	p.generation++
	generation := p.generation
	if p.surface != 0 {
		parent = p.surface
	}
	previous := p.process
	p.process, p.active = nil, false
	p.mu.Unlock()
	defer cancel()
	if previous != nil {
		previous.stop()
	}
	findLibMpv := func() string {
		resourceDir, _ := mygo.App.Path(mygo.PathResources)
		candidates := []string{
			filepath.Join(resourceDir, "player", "libmpv-2.dll"),
			filepath.Join(resourceDir, "windows-amd64", "player", "libmpv-2.dll"),
			filepath.Join("resources", "windows-amd64", "player", "libmpv-2.dll"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return ""
	}
	dll := findLibMpv()
	if dll == "" {
		return fmt.Errorf("找不到 libmpv-2.dll；请检查应用资源目录")
	}
	process := &playerProcess{}
	if err := process.startContext(ctx, dll, streamURL, parent, resumeAt); err != nil {
		process.stop()
		return err
	}
	p.mu.Lock()
	if generation != p.generation || ctx.Err() != nil {
		p.mu.Unlock()
		process.stop()
		return context.Canceled
	}
	defer p.mu.Unlock()
	p.startCancel = nil
	p.process = process
	p.active = true
	p.url = streamURL
	p.position, p.duration, p.paused = resumeAt, duration, false
	p.started = time.Now()
	return nil
}

// PrepareSurface and ReleaseSurface must run on the window's UI thread.
func (p *Player) PrepareSurface(parent uintptr) error {
	surface, err := createPlayerSurface(parent)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.surface = surface
	p.mu.Unlock()
	return nil
}

func (p *Player) ShowSurface() {
	p.mu.Lock()
	defer p.mu.Unlock()
	showPlayerSurface(p.surface)
}

func (p *Player) ReleaseSurface() {
	p.mu.Lock()
	surface := p.surface
	p.surface = 0
	p.mu.Unlock()
	destroyPlayerSurface(surface)
}

func (p *Player) Stop() {
	p.mu.Lock()
	if p.startCancel != nil {
		p.startCancel()
		p.startCancel = nil
	}
	p.generation++
	process := p.process
	p.active = false
	p.mu.Unlock()
	if process != nil {
		process.stop()
	}
}

func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process != nil {
		p.process.command("cycle", "pause")
	}
}
func (p *Player) ToggleMute() { p.command("cycle", "mute") }
func (p *Player) AdjustVolume(delta int) {
	p.command("add", "volume", strconv.Itoa(delta))
}

func (p *Player) SetSpeed(value float64) {
	if value < 0.25 {
		value = 0.25
	}
	if value > 4 {
		value = 4
	}
	p.command("set_property", "speed", value)
}

func (p *Player) Seek(seconds float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paused {
		p.position += seconds
	} else {
		p.position += time.Since(p.started).Seconds() + seconds
		p.started = time.Now()
	}
	if p.position < 0 {
		p.position = 0
	}
	if p.duration > 0 && p.position > p.duration {
		p.position = p.duration
	}
	if p.process != nil {
		p.process.command("seek", strconv.FormatFloat(seconds, 'f', 1, 64), "relative")
	}
}

func (p *Player) SeekTo(seconds float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if seconds < 0 {
		seconds = 0
	}
	if p.duration > 0 && seconds > p.duration {
		seconds = p.duration
	}
	p.position = seconds
	p.started = time.Now()
	if p.process != nil {
		p.process.command("seek", strconv.FormatFloat(seconds, 'f', 1, 64), "absolute")
	}
}

func (p *Player) SetVolume(value float64) {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	p.command("set_property", "volume", value)
}

func (p *Player) SelectTrack(kind string, id int) {
	property, value := playerTrackSelection(kind, id)
	p.command("set_property", property, value)
}

func playerTrackSelection(kind string, id int) (property string, value any) {
	property = "sid"
	if kind == "audio" {
		property = "aid"
	}
	if id <= 0 {
		return property, "no"
	}
	return property, id
}

func (p *Player) Snapshot() PlayerSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == nil {
		return PlayerSnapshot{Position: p.position, Duration: p.duration}
	}
	state := p.process.snapshot()
	if p.active {
		p.paused = state.Paused
		if state.Duration > 0 {
			p.duration = state.Duration
		}
		p.position = state.Position
	}
	return state
}

func (p *Player) Position() (float64, float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == nil {
		return p.position, p.duration
	}
	actualPosition, actualDuration, observed := p.process.position()
	if actualDuration > 0 {
		p.duration = actualDuration
	}
	if observed {
		return actualPosition, p.duration
	}
	position := p.position
	if !p.paused && !p.started.IsZero() {
		position += time.Since(p.started).Seconds()
	}
	return position, p.duration
}

func (p *Player) SetViewport(rect ui.Rect) {
	p.mu.Lock()
	p.viewport = rect
	if p.process != nil {
		p.process.setViewport(rect)
	}
	p.mu.Unlock()
}

func (p *Player) PointerActivity() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == nil {
		return nil
	}
	return p.process.pointerActivity()
}

func (p *Player) command(args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process != nil {
		p.process.command(args...)
	}
}
