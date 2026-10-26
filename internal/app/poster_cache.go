package app

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
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
	tint   ui.Color
	bytes  int64
}

type posterRequest struct {
	generation uint64
	key        posterKey
	server     *Server
	url        string
	ctx        context.Context
	cancel     context.CancelFunc
	onLoaded   func()
}

// PosterLoader bounds both network/decode concurrency and retained bitmap
// memory. Keys include the target pixel size so one poster can be reused at
// card, hero, and detail sizes without retaining full-resolution artwork.
type PosterLoader struct {
	mu            sync.Mutex
	queue         chan posterRequest
	pending       map[posterKey]uint64
	failed        map[posterKey]time.Time
	cancellations map[posterKey]context.CancelFunc
	entries       map[posterKey]*list.Element
	lru           *list.List
	used          int64
	maxBytes      int64
	diskDir       string
	diskMu        sync.Mutex
	paused        bool
	closed        bool
	generation    uint64
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
	if p.paused || p.closed {
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
	ctx, cancel := context.WithCancel(context.Background())
	if p.cancellations == nil {
		p.cancellations = make(map[posterKey]context.CancelFunc)
	}
	p.cancellations[key] = cancel
	request := posterRequest{generation: p.generation, key: key, server: server, url: remoteURL, ctx: ctx, cancel: cancel, onLoaded: onLoaded}
	select {
	case p.queue <- request:
		p.mu.Unlock()
	default:
		delete(p.pending, key)
		delete(p.cancellations, key)
		cancel()
		p.mu.Unlock()
	}
	return nil
}

// Cached returns an already decoded image without scheduling extra work.
// It lets a preview remain visible while its full backdrop is loading.
func (p *PosterLoader) Cached(remoteURL string, width, height int) *ui.Bitmap {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paused {
		return nil
	}
	if element := p.entries[posterKey{URL: remoteURL, Width: width, Height: height}]; element != nil {
		p.lru.MoveToFront(element)
		return element.Value.(*posterEntry).bitmap
	}
	return nil
}

func (p *PosterLoader) Tint(remoteURL string, width, height int) (ui.Color, bool) {
	if p == nil {
		return ui.Color{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paused {
		return ui.Color{}, false
	}
	if element := p.entries[posterKey{URL: remoteURL, Width: width, Height: height}]; element != nil {
		p.lru.MoveToFront(element)
		return element.Value.(*posterEntry).tint, true
	}
	return ui.Color{}, false
}

func (p *PosterLoader) worker() {
	for request := range p.queue {
		p.mu.Lock()
		obsolete := p.closed || p.paused || request.generation != p.generation
		p.mu.Unlock()
		if obsolete {
			if request.cancel != nil {
				request.cancel()
			}
			continue
		}
		bitmap, size, tint, err := p.loadPosterWithTint(request)
		if request.cancel != nil {
			request.cancel()
		}
		p.mu.Lock()
		// A decode already in flight may finish after playback starts or after
		// browsing resumes. It must not refill the cache or erase a new request.
		if p.closed || p.paused || request.generation != p.generation {
			p.mu.Unlock()
			continue
		}
		if p.pending[request.key] != request.generation {
			p.mu.Unlock()
			continue
		}
		delete(p.pending, request.key)
		delete(p.cancellations, request.key)
		if err != nil {
			p.failed[request.key] = time.Now().Add(5 * time.Second)
		} else {
			delete(p.failed, request.key)
			p.putTintLocked(request.key, bitmap, size, tint)
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
	if p.closed || p.paused == active {
		p.mu.Unlock()
		return
	}
	p.paused = active
	p.cancelRequestsLocked()
	if active {
		clear(p.entries)
		p.lru.Init()
		p.used = 0
	}
	p.mu.Unlock()
}

// CancelPending invalidates and cancels active and queued requests while
// leaving the loader available for subsequent browsing requests.
func (p *PosterLoader) CancelPending() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.cancelRequestsLocked()
}

func (p *PosterLoader) cancelRequestsLocked() {
	p.generation++
	clear(p.pending)
	clear(p.failed)
	for _, cancel := range p.cancellations {
		cancel()
	}
	clear(p.cancellations)
	for {
		select {
		case request, ok := <-p.queue:
			if !ok {
				return
			}
			if request.cancel != nil {
				request.cancel()
			}
		default:
			return
		}
	}
}

// Close cancels active and queued downloads and stops the poster workers.
func (p *PosterLoader) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.paused = true
	p.cancelRequestsLocked()
	clear(p.entries)
	p.lru.Init()
	p.used = 0
	if p.queue != nil {
		close(p.queue)
	}
	p.mu.Unlock()
}

func (p *PosterLoader) loadPoster(request posterRequest) (*ui.Bitmap, int64, error) {
	bitmap, size, _, err := p.loadPosterWithTint(request)
	return bitmap, size, err
}

func (p *PosterLoader) loadPosterWithTint(request posterRequest) (*ui.Bitmap, int64, ui.Color, error) {
	ctx := request.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, ui.Color{}, err
	}
	if p.diskDir == "" {
		return loadPosterWithTintContext(ctx, request.server, request.url, request.key.Width, request.key.Height)
	}
	path := posterSourcePath(p.diskDir, request.url)
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 30*24*time.Hour {
		if data, err := os.ReadFile(path); err == nil {
			if bitmap, size, tint, decodeErr := decodePosterWithTint(data, request.key.Width, request.key.Height); decodeErr == nil {
				_ = os.Chtimes(path, time.Now(), time.Now())
				return bitmap, size, tint, nil
			}
		}
	}
	data, err := request.server.FetchImageContext(ctx, request.url)
	if err != nil {
		return nil, 0, ui.Color{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, ui.Color{}, err
	}
	bitmap, size, tint, err := decodePosterWithTint(data, request.key.Width, request.key.Height)
	if err != nil {
		return nil, 0, ui.Color{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, ui.Color{}, err
	}
	p.cachePosterSource(path, data)
	return bitmap, size, tint, nil
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
	removeStaleCacheTemps(dir, "poster-", time.Hour, time.Now())
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
	p.putTintLocked(key, bitmap, size, movieColors.accent)
}

func (p *PosterLoader) putTintLocked(key posterKey, bitmap *ui.Bitmap, size int64, tint ui.Color) {
	if old := p.entries[key]; old != nil {
		p.used -= old.Value.(*posterEntry).bytes
		p.lru.Remove(old)
		delete(p.entries, key)
	}
	if size > p.maxBytes {
		return
	}
	entry := &posterEntry{key: key, bitmap: bitmap, tint: tint, bytes: size}
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
	return loadPosterContext(context.Background(), server, remoteURL, maxWidth, maxHeight)
}

func loadPosterContext(ctx context.Context, server *Server, remoteURL string, maxWidth, maxHeight int) (*ui.Bitmap, int64, error) {
	data, err := server.FetchImageContext(ctx, remoteURL)
	if err != nil {
		return nil, 0, err
	}
	return decodePoster(data, maxWidth, maxHeight)
}

func loadPosterWithTintContext(ctx context.Context, server *Server, remoteURL string, maxWidth, maxHeight int) (*ui.Bitmap, int64, ui.Color, error) {
	data, err := server.FetchImageContext(ctx, remoteURL)
	if err != nil {
		return nil, 0, ui.Color{}, err
	}
	return decodePosterWithTint(data, maxWidth, maxHeight)
}

func decodePoster(data []byte, maxWidth, maxHeight int) (*ui.Bitmap, int64, error) {
	bitmap, size, _, err := decodePosterWithTint(data, maxWidth, maxHeight)
	return bitmap, size, err
}

func decodePosterWithTint(data []byte, maxWidth, maxHeight int) (*ui.Bitmap, int64, ui.Color, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, ui.Color{}, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 12_000_000 {
		return nil, 0, ui.Color{}, errInvalidPosterDimensions
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, ui.Color{}, err
	}
	imageSize := fitPosterSize(source.Bounds().Size(), image.Pt(maxWidth, maxHeight))
	if imageSize.X <= 0 || imageSize.Y <= 0 {
		return nil, 0, ui.Color{}, errInvalidPosterDimensions
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
	return bitmap, bytes, dominantPosterTint(resized), nil
}

func dominantPosterTint(img image.Image) ui.Color {
	bounds := img.Bounds()
	stepX, stepY := max(1, bounds.Dx()/24), max(1, bounds.Dy()/24)
	const buckets = 18
	var weight [buckets]float64
	var sum [buckets][3]float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			rawR, rawG, rawB, _ := img.At(x, y).RGBA()
			r, g, b := float64(rawR>>8)/255, float64(rawG>>8)/255, float64(rawB>>8)/255
			h, saturation, value := posterHSV(r, g, b)
			if saturation < 0.15 || value < 0.15 {
				continue
			}
			bucket := int(h/360*buckets) % buckets
			w := saturation * saturation * value
			weight[bucket] += w
			sum[bucket][0], sum[bucket][1], sum[bucket][2] = sum[bucket][0]+r*w, sum[bucket][1]+g*w, sum[bucket][2]+b*w
		}
	}
	best := 0
	for i := 1; i < buckets; i++ {
		if weight[i] > weight[best] {
			best = i
		}
	}
	if weight[best] < 2 {
		return movieColors.accent
	}
	w := weight[best]
	return ui.RGB(uint8(sum[best][0]/w*255), uint8(sum[best][1]/w*255), uint8(sum[best][2]/w*255))
}

func posterHSV(r, g, b float64) (h, saturation, value float64) {
	maxValue, minValue := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	value = maxValue
	if maxValue == 0 {
		return 0, 0, 0
	}
	saturation = (maxValue - minValue) / maxValue
	delta := maxValue - minValue
	switch {
	case delta == 0:
		h = 0
	case maxValue == r:
		h = math.Mod((g-b)/delta, 6) * 60
	case maxValue == g:
		h = ((b-r)/delta + 2) * 60
	default:
		h = ((r-g)/delta + 4) * 60
	}
	if h < 0 {
		h += 360
	}
	return h, saturation, value
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
