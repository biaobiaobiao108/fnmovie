package app

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	posterWorkers     = 2
	posterQueueLimit  = 64
	posterCacheBudget = int64(64 << 20)
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
	generation uint64
	key        posterKey
	server     *Server
	url        string
	onLoaded   func()
}

// PosterLoader bounds both network/decode concurrency and retained bitmap
// memory. Keys include the target pixel size so one poster can be reused at
// card, hero, and detail sizes without retaining full-resolution artwork.
type PosterLoader struct {
	mu         sync.Mutex
	queue      chan posterRequest
	pending    map[posterKey]uint64
	failed     map[posterKey]time.Time
	entries    map[posterKey]*list.Element
	lru        *list.List
	used       int64
	maxBytes   int64
	diskDir    string
	diskMu     sync.Mutex
	paused     bool
	generation uint64
}

func NewPosterLoader() *PosterLoader {
	p := &PosterLoader{
		queue: make(chan posterRequest, posterQueueLimit), pending: map[posterKey]uint64{},
		failed: map[posterKey]time.Time{}, entries: map[posterKey]*list.Element{},
		lru: list.New(), maxBytes: posterCacheBudget,
	}
	if dir, err := os.UserCacheDir(); err == nil {
		p.diskDir = filepath.Join(dir, "FnMovie", "posters")
		if os.MkdirAll(p.diskDir, 0700) != nil {
			p.diskDir = ""
		}
	}
	p.prunePosterDisk()
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
	if p.paused {
		p.mu.Unlock()
		return nil
	}
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
	p.pending[key] = p.generation
	request := posterRequest{generation: p.generation, key: key, server: server, url: remoteURL, onLoaded: onLoaded}
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
		p.mu.Lock()
		obsolete := p.paused || request.generation != p.generation
		p.mu.Unlock()
		if obsolete {
			continue
		}
		bitmap, size, err := p.loadPoster(request)
		p.mu.Lock()
		// A decode already in flight may finish after playback starts or after
		// browsing resumes. It must not refill the cache or erase a new request.
		if p.paused || request.generation != p.generation {
			p.mu.Unlock()
			continue
		}
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

// SetPlayback releases browsing artwork and drops queued work while the video
// owns the viewport. Sources remain on disk for inexpensive return navigation.
func (p *PosterLoader) SetPlayback(active bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paused == active {
		return
	}
	p.paused = active
	p.generation++
	clear(p.pending)
	clear(p.failed)
	if active {
		clear(p.entries)
		p.lru.Init()
		p.used = 0
	}
	for {
		select {
		case <-p.queue:
		default:
			return
		}
	}
}

func (p *PosterLoader) loadPoster(request posterRequest) (*ui.Bitmap, int64, error) {
	if p.diskDir == "" {
		return loadPoster(request.server, request.url, request.key.Width, request.key.Height)
	}
	path := posterSourcePath(p.diskDir, request.url)
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 30*24*time.Hour {
		if data, err := os.ReadFile(path); err == nil {
			if bitmap, size, decodeErr := decodePoster(data, request.key.Width, request.key.Height); decodeErr == nil {
				_ = os.Chtimes(path, time.Now(), time.Now())
				return bitmap, size, nil
			}
		}
	}
	data, err := request.server.FetchImage(request.url)
	if err != nil {
		return nil, 0, err
	}
	bitmap, size, err := decodePoster(data, request.key.Width, request.key.Height)
	if err != nil {
		return nil, 0, err
	}
	p.cachePosterSource(path, data)
	return bitmap, size, nil
}

func posterSourcePath(dir, remoteURL string) string {
	sum := sha256.Sum256([]byte(remoteURL))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".img")
}

func writePosterSource(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "poster-*.tmp")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Windows Rename does not replace an existing destination. The file is a
	// source cache and can always be downloaded again if a concurrent write
	// wins this small replacement race.
	_ = os.Remove(path)
	if err := os.Rename(tempPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil // another worker completed the same image concurrently
		}
		return err
	}
	return nil
}

// cachePosterSource reserves space before creating the temporary file. Serialize
// disk writes and eviction so concurrent workers cannot each spend the same
// remaining budget. Network requests and image decoding remain concurrent.
func (p *PosterLoader) cachePosterSource(path string, data []byte) {
	p.diskMu.Lock()
	defer p.diskMu.Unlock()
	if int64(len(data)) > posterDiskCacheBudget {
		return
	}
	_ = os.Remove(path)
	if p.prunePosterDiskLocked(int64(len(data))) {
		_ = writePosterSource(path, data)
	}
}

func (p *PosterLoader) prunePosterDisk() {
	p.diskMu.Lock()
	defer p.diskMu.Unlock()
	p.prunePosterDiskLocked(0)
}

func (p *PosterLoader) prunePosterDiskLocked(reserved int64) bool {
	dir := p.diskDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	type diskEntry struct {
		path string
		size int64
		at   time.Time
	}
	files := make([]diskEntry, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) == ".tmp" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > 30*24*time.Hour && os.Remove(path) == nil {
			continue
		}
		files = append(files, diskEntry{path: path, size: info.Size(), at: info.ModTime()})
		total += info.Size()
	}
	if total+reserved <= posterDiskCacheBudget {
		return true
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	for _, file := range files {
		if total+reserved <= posterDiskCacheBudget {
			break
		}
		if os.Remove(file.path) == nil {
			total -= file.size
		}
	}
	return total+reserved <= posterDiskCacheBudget
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

// A pending backdrop must not cause an additional full-size poster decode.
func (p *PosterLoader) HasFailed(remoteURL string, width, height int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Now().Before(p.failed[posterKey{URL: remoteURL, Width: width, Height: height}])
}

func loadPoster(server *Server, remoteURL string, maxWidth, maxHeight int) (*ui.Bitmap, int64, error) {
	data, err := server.FetchImage(remoteURL)
	if err != nil {
		return nil, 0, err
	}
	return decodePoster(data, maxWidth, maxHeight)
}

func decodePoster(data []byte, maxWidth, maxHeight int) (*ui.Bitmap, int64, error) {
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
		// Poster thumbnails are intentionally bounded to card size. A bicubic
		// filter adds noticeable CPU work when several newly-visible posters
		// arrive during a fling; bilinear keeps those updates cheap while the
		// original-resolution artwork remains untouched in the catalog.
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), source, source.Bounds(), draw.Over, nil)
		resized = dst
	}
	bitmap := ui.NewBitmap(resized)
	// Account for the RGBA source plus the bitmap's lazily generated mip levels.
	bytes := int64(imageSize.X) * int64(imageSize.Y) * 4 * 4 / 3
	return bitmap, bytes, nil
}

const posterDiskCacheBudget = int64(512 << 20)

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
