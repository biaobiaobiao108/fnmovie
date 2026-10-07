package main

import (
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
	mu       sync.Mutex
	url      string
	active   bool
	position float64
	duration float64
	paused   bool
	viewport ui.Rect
	started  time.Time
	process  playerProcess
}

func NewPlayer() *Player { return &Player{} }

func (p *Player) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active && p.process.running()
}

func (p *Player) Start(streamURL string, parent uintptr, duration, resumeAt float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		p.process.stop()
	}
	findMpv := func() string {
		resourceDir, _ := mygo.App.Path(mygo.PathResources)
		candidates := []string{
			filepath.Join(resourceDir, "player", "mpv.exe"),
			filepath.Join(resourceDir, "windows-amd64", "player", "mpv.exe"),
			filepath.Join("player", "mpv.exe"),
			filepath.Join("resources", "player", "mpv.exe"),
			"mpv.exe",
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return ""
	}
	binary := findMpv()
	if binary == "" {
		return fmt.Errorf("找不到播放器文件 player\\mpv.exe；请将 mpv Windows 发行版放入 player 目录")
	}
	if err := p.process.start(binary, streamURL, parent, resumeAt); err != nil {
		p.process.stop()
		return err
	}
	p.active = true
	p.url = streamURL
	p.position, p.duration, p.paused = resumeAt, duration, false
	p.started = time.Now()
	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.process.stop()
	p.active = false
}

func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.process.command("cycle", "pause")
}
func (p *Player) CycleSubtitle() { p.command("cycle", "sub") }
func (p *Player) CycleAudio()    { p.command("cycle", "audio") }
func (p *Player) AdjustVolume(delta int) {
	p.command("add", "volume", strconv.Itoa(delta))
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
	p.process.command("seek", strconv.FormatFloat(seconds, 'f', 1, 64), "relative")
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
	p.process.command("seek", strconv.FormatFloat(seconds, 'f', 1, 64), "absolute")
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
	property := "sid"
	if kind == "audio" {
		property = "aid"
	}
	if id <= 0 {
		p.command("set_property", property, "no")
		return
	}
	p.command("set_property", property, id)
}

func (p *Player) Snapshot() PlayerSnapshot {
	_, _ = p.Position()
	state := p.process.snapshot()
	p.mu.Lock()
	if p.active {
		p.paused = state.Paused
		if state.Duration > 0 {
			p.duration = state.Duration
		}
		p.position = state.Position
	}
	p.mu.Unlock()
	return state
}

func (p *Player) Position() (float64, float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
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
	p.process.setViewport(rect)
	p.mu.Unlock()
}

func (p *Player) command(args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.process.command(args...)
}
