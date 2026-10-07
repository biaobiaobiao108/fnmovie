package main

import (
	"bytes"
	"container/list"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	posterWorkers     = 4
	posterQueueLimit  = 64
	posterCacheBudget = int64(256 << 20)
)

type posterKey struct {
	URL    string
	Width  int
	Height int
}

type posterEntry struct {
	key    posterKey
	bitmap *ui.Bitmap
	bytes  int64
}

type posterRequest struct {
	key      posterKey
	server   *Server
	url      string
	onLoaded func()
}

// PosterLoader bounds both network/decode concurrency and retained bitmap
// memory. Keys include the target pixel size so one poster can be reused at
// card, hero, and detail sizes without retaining full-resolution artwork.
type PosterLoader struct {
	mu       sync.Mutex
	queue    chan posterRequest
	pending  map[posterKey]struct{}
	failed   map[posterKey]time.Time
	entries  map[posterKey]*list.Element
	lru      *list.List
	used     int64
	maxBytes int64
}

func NewPosterLoader() *PosterLoader {
	p := &PosterLoader{
		queue: make(chan posterRequest, posterQueueLimit), pending: map[posterKey]struct{}{},
		failed: map[posterKey]time.Time{}, entries: map[posterKey]*list.Element{},
		lru: list.New(), maxBytes: posterCacheBudget,
	}
	for i := 0; i < posterWorkers; i++ {
		go p.worker()
	}
	return p
}

func (p *PosterLoader) GetOrRequest(server *Server, remoteURL string, width, height int, onLoaded func()) *ui.Bitmap {
	if p == nil || server == nil || remoteURL == "" || width <= 0 || height <= 0 {
		return nil
	}
	key := posterKey{URL: remoteURL, Width: width, Height: height}
	p.mu.Lock()
	if element := p.entries[key]; element != nil {
		p.lru.MoveToFront(element)
		bitmap := element.Value.(*posterEntry).bitmap
		p.mu.Unlock()
		return bitmap
	}
	if _, exists := p.pending[key]; exists || time.Now().Before(p.failed[key]) || len(p.pending) >= posterQueueLimit {
		p.mu.Unlock()
		return nil
	}
	p.pending[key] = struct{}{}
	request := posterRequest{key: key, server: server, url: remoteURL, onLoaded: onLoaded}
	select {
	case p.queue <- request:
		p.mu.Unlock()
	default:
		delete(p.pending, key)
		p.mu.Unlock()
	}
	return nil
}

func (p *PosterLoader) worker() {
	for request := range p.queue {
		bitmap, size, err := loadPoster(request.server, request.url, request.key.Width, request.key.Height)
		p.mu.Lock()
		delete(p.pending, request.key)
		if err != nil {
			p.failed[request.key] = time.Now().Add(5 * time.Second)
		} else {
			delete(p.failed, request.key)
			p.putLocked(request.key, bitmap, size)
		}
		p.mu.Unlock()
		if err != nil {
			log.Printf("poster load failed: %v", err)
		} else if request.onLoaded != nil {
			request.onLoaded()
		}
	}
}

func (p *PosterLoader) putLocked(key posterKey, bitmap *ui.Bitmap, size int64) {
	if old := p.entries[key]; old != nil {
		p.used -= old.Value.(*posterEntry).bytes
		p.lru.Remove(old)
		delete(p.entries, key)
	}
	if size > p.maxBytes {
		return
	}
	entry := &posterEntry{key: key, bitmap: bitmap, bytes: size}
	p.entries[key] = p.lru.PushFront(entry)
	p.used += size
	for p.used > p.maxBytes {
		old := p.lru.Back()
		if old == nil {
			break
		}
		victim := old.Value.(*posterEntry)
		delete(p.entries, victim.key)
		p.used -= victim.bytes
		p.lru.Remove(old)
	}
}

func (p *PosterLoader) SetBudget(bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if bytes < 0 {
		bytes = 0
	}
	p.maxBytes = bytes
	for p.used > p.maxBytes {
		old := p.lru.Back()
		if old == nil {
			break
		}
		victim := old.Value.(*posterEntry)
		delete(p.entries, victim.key)
		p.used -= victim.bytes
		p.lru.Remove(old)
	}
}

func (p *PosterLoader) Stats() (items int, bytes int64, pending int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries), p.used, len(p.pending)
}

func loadPoster(server *Server, remoteURL string, maxWidth, maxHeight int) (*ui.Bitmap, int64, error) {
	data, err := server.FetchImage(remoteURL)
	if err != nil {
		return nil, 0, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 12_000_000 {
		return nil, 0, errInvalidPosterDimensions
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	imageSize := fitPosterSize(source.Bounds().Size(), image.Pt(maxWidth, maxHeight))
	if imageSize.X <= 0 || imageSize.Y <= 0 {
		return nil, 0, errInvalidPosterDimensions
	}
	var resized image.Image = source
	if imageSize != source.Bounds().Size() {
		dst := image.NewRGBA(image.Rect(0, 0, imageSize.X, imageSize.Y))
		draw.CatmullRom.Scale(dst, dst.Bounds(), source, source.Bounds(), draw.Over, nil)
		resized = dst
	}
	bitmap := ui.NewBitmap(resized)
	// Account for the RGBA source plus the bitmap's lazily generated mip levels.
	bytes := int64(imageSize.X) * int64(imageSize.Y) * 4 * 4 / 3
	return bitmap, bytes, nil
}

var errInvalidPosterDimensions = posterError("poster has invalid or excessive dimensions")

type posterError string

func (e posterError) Error() string { return string(e) }

func fitPosterSize(source, target image.Point) image.Point {
	if source.X <= 0 || source.Y <= 0 || target.X <= 0 || target.Y <= 0 {
		return image.Point{}
	}
	scaleX := float64(target.X) / float64(source.X)
	scaleY := float64(target.Y) / float64(source.Y)
	scale := min(1.0, min(scaleX, scaleY))
	return image.Pt(max(1, int(float64(source.X)*scale+0.5)), max(1, int(float64(source.Y)*scale+0.5)))
}
