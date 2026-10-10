package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// A modest first page gets the library on screen quickly. More items are
// fetched as the virtualized grid approaches the end of the current page.
const catalogPageSize = 60

type CatalogPage struct {
	Items     []MediaItem `json:"items"`
	Total     int         `json:"total"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type CatalogState struct {
	Items             []MediaItem
	Total             int
	NextPage          int
	Loading           bool
	Exhausted         bool
	PageAutoRequested bool
	Err               string
	Cancel            func()
	UpdatedAt         time.Time
	LastUsed          time.Time
	Refreshed         bool
}

type HomeHeroesCache struct {
	Items     []MediaItem `json:"items"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type HomeContinueCache struct {
	Items     []ContinueWatchingItem `json:"items"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

type catalogDisk struct {
	Version      int                          `json:"version"`
	Libraries    map[string][]MediaLibrary    `json:"libraries"`
	Pages        map[string]CatalogPage       `json:"pages"`
	Details      map[string]MediaItem         `json:"details"`
	HomeHeroes   map[string]HomeHeroesCache   `json:"homeHeroes,omitempty"`
	HomeContinue map[string]HomeContinueCache `json:"homeContinue,omitempty"`
	// Usage records the last write time for each cache entry. It is keyed by
	// cache kind and entry key so identical page/detail keys stay independent.
	Usage map[string]time.Time `json:"usage,omitempty"`
}

const catalogCacheVersion = 3

// These limits bound the persistent catalog file across all servers and
// accounts. Details are the largest entries, while pages contain many items.
const (
	catalogMaxLibrarySets  = 32
	catalogMaxPages        = 256
	catalogMaxDetails      = 512
	catalogMaxHomeScopes   = 16
	homeContinueCacheLimit = 200
)

type CatalogCache struct {
	mu   sync.Mutex
	path string
	data catalogDisk
}

func catalogPageExhausted(pageCount, loadedCount, total, pageSize int) bool {
	if pageCount == 0 {
		return true
	}
	if total > 0 {
		return loadedCount >= total
	}
	return pageCount < pageSize
}

func catalogNeedsMoreVisibleItems(visibleCount int, state *CatalogState) bool {
	return visibleCount == 0 && state != nil && !state.Loading && !state.Exhausted &&
		state.Err == "" && !state.PageAutoRequested
}

func NewCatalogCache() *CatalogCache {
	dir, err := os.UserConfigDir()
	if err != nil {
		data := newCatalogDisk()
		return &CatalogCache{data: data}
	}
	dir = filepath.Join(dir, "FnMovie")
	_ = os.MkdirAll(dir, 0700)
	c := &CatalogCache{path: filepath.Join(dir, "catalog.json"), data: newCatalogDisk()}
	if raw, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(raw, &c.data)
	}
	migrateCatalogDisk(&c.data)
	return c
}

func newCatalogDisk() catalogDisk {
	return catalogDisk{
		Version: catalogCacheVersion,
		Pages:   map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{},
		Details: map[string]MediaItem{}, HomeHeroes: map[string]HomeHeroesCache{},
		HomeContinue: map[string]HomeContinueCache{}, Usage: map[string]time.Time{},
	}
}

func migrateCatalogDisk(data *catalogDisk) {
	if data.Version < catalogCacheVersion {
		// Earlier caches were not isolated by account. None of their entries
		// can safely be assigned to the next account that logs in.
		data.Libraries = map[string][]MediaLibrary{}
		data.Pages = map[string]CatalogPage{}
		data.Details = map[string]MediaItem{}
		data.Version = catalogCacheVersion
	}
	if data.Pages == nil {
		data.Pages = map[string]CatalogPage{}
	}
	if data.Libraries == nil {
		data.Libraries = map[string][]MediaLibrary{}
	}
	if data.Details == nil {
		data.Details = map[string]MediaItem{}
	}
	if data.HomeHeroes == nil {
		data.HomeHeroes = map[string]HomeHeroesCache{}
	}
	if data.HomeContinue == nil {
		data.HomeContinue = map[string]HomeContinueCache{}
	}
	if data.Usage == nil {
		data.Usage = map[string]time.Time{}
	}
	trimCatalogEntries(data.Libraries, data.Usage, "libraries", catalogMaxLibrarySets)
	trimCatalogEntries(data.Pages, data.Usage, "pages", catalogMaxPages)
	trimCatalogEntries(data.Details, data.Usage, "details", catalogMaxDetails)
	trimCatalogEntries(data.HomeHeroes, data.Usage, "home-heroes", catalogMaxHomeScopes)
	trimCatalogEntries(data.HomeContinue, data.Usage, "home-continue", catalogMaxHomeScopes)
}

func catalogUsageKey(kind, key string) string { return kind + "\x00" + key }

func recordCatalogWrite(usage map[string]time.Time, kind, key string) {
	latest := time.Time{}
	for usageKey, at := range usage {
		if strings.HasPrefix(usageKey, kind+"\x00") && at.After(latest) {
			latest = at
		}
	}
	now := time.Now()
	if !now.After(latest) {
		now = latest.Add(time.Nanosecond)
	}
	usage[catalogUsageKey(kind, key)] = now
}

func (c *CatalogCache) ensureUsageLocked() {
	if c.data.Usage == nil {
		c.data.Usage = map[string]time.Time{}
	}
}

// trimCatalogEntries removes the oldest written entries. Entries from older
// cache files have no usage timestamp, so they are evicted first; key order is
// the deterministic tie breaker for equally old entries.
func trimCatalogEntries[T any](entries map[string]T, usage map[string]time.Time, kind string, limit int) {
	if len(entries) <= limit {
		return
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ti := usage[catalogUsageKey(kind, keys[i])]
		tj := usage[catalogUsageKey(kind, keys[j])]
		if ti.Equal(tj) {
			return keys[i] < keys[j]
		}
		return ti.Before(tj)
	})
	for _, key := range keys[:len(keys)-limit] {
		delete(entries, key)
		delete(usage, catalogUsageKey(kind, key))
	}
}

func catalogServerKey(serverURL string, username ...string) string {
	account := ""
	if len(username) > 0 {
		account = strings.TrimSpace(username[0])
	}
	sum := sha256.Sum256([]byte(normalizeServerURL(serverURL) + "\x00" + account))
	return hex.EncodeToString(sum[:12])
}

func catalogPageKey(serverURL, libraryID string, username ...string) string {
	return catalogServerKey(serverURL, username...) + ":" + libraryID
}

func catalogCacheScope(libraryID, mediaType string) string {
	if mediaType == "" {
		return libraryID
	}
	if libraryID == "" {
		return "@system:" + mediaType
	}
	return libraryID + "@type:" + mediaType
}

func homeHeroesCacheKey(serverURL string, libraries []MediaLibrary, username ...string) string {
	ids := make([]string, 0, len(libraries))
	seen := make(map[string]bool, len(libraries))
	for _, library := range libraries {
		if library.ID != "" && !seen[library.ID] {
			seen[library.ID] = true
			ids = append(ids, library.ID)
		}
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\x00")))
	return catalogServerKey(serverURL, username...) + ":" + hex.EncodeToString(sum[:12])
}

func homeCacheMedia(item MediaItem) MediaItem {
	item.Genres = append([]string(nil), item.Genres...)
	item.Countries = append([]string(nil), item.Countries...)
	item.Raw = nil
	item.Sources = nil
	item.Cast = nil
	item.Episodes = nil
	item.Seasons = nil
	return item
}

func cloneHomeHeroes(items []MediaItem) []MediaItem {
	if len(items) > homeHighlightLimit {
		items = items[:homeHighlightLimit]
	}
	out := make([]MediaItem, len(items))
	for i, item := range items {
		out[i] = homeCacheMedia(item)
	}
	return out
}

func cloneHomeContinue(items []ContinueWatchingItem) []ContinueWatchingItem {
	if len(items) > homeContinueCacheLimit {
		items = items[:homeContinueCacheLimit]
	}
	out := make([]ContinueWatchingItem, len(items))
	for i, item := range items {
		item.Media = homeCacheMedia(item.Media)
		out[i] = item
	}
	return out
}

func (c *CatalogCache) HomeHeroes(serverURL string, libraries []MediaLibrary, username ...string) ([]MediaItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data.HomeHeroes[homeHeroesCacheKey(serverURL, libraries, username...)]
	return cloneHomeHeroes(entry.Items), ok
}

func (c *CatalogCache) SetHomeHeroes(serverURL string, libraries []MediaLibrary, items []MediaItem, username ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureUsageLocked()
	key := homeHeroesCacheKey(serverURL, libraries, username...)
	c.data.HomeHeroes[key] = HomeHeroesCache{Items: cloneHomeHeroes(items), UpdatedAt: time.Now()}
	recordCatalogWrite(c.data.Usage, "home-heroes", key)
	trimCatalogEntries(c.data.HomeHeroes, c.data.Usage, "home-heroes", catalogMaxHomeScopes)
	c.saveAndLogLocked()
}

func (c *CatalogCache) HomeContinue(serverURL string, username ...string) ([]ContinueWatchingItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data.HomeContinue[catalogServerKey(serverURL, username...)]
	return cloneHomeContinue(entry.Items), ok
}

func (c *CatalogCache) SetHomeContinue(serverURL string, items []ContinueWatchingItem, username ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureUsageLocked()
	key := catalogServerKey(serverURL, username...)
	c.data.HomeContinue[key] = HomeContinueCache{Items: cloneHomeContinue(items), UpdatedAt: time.Now()}
	recordCatalogWrite(c.data.Usage, "home-continue", key)
	trimCatalogEntries(c.data.HomeContinue, c.data.Usage, "home-continue", catalogMaxHomeScopes)
	c.saveAndLogLocked()
}

func (c *CatalogCache) Libraries(serverURL string, username ...string) []MediaLibrary {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]MediaLibrary(nil), c.data.Libraries[catalogServerKey(serverURL, username...)]...)
}

func (c *CatalogCache) SetLibraries(serverURL string, libraries []MediaLibrary, username ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureUsageLocked()
	key := catalogServerKey(serverURL, username...)
	c.data.Libraries[key] = append([]MediaLibrary(nil), libraries...)
	recordCatalogWrite(c.data.Usage, "libraries", key)
	trimCatalogEntries(c.data.Libraries, c.data.Usage, "libraries", catalogMaxLibrarySets)
	c.saveAndLogLocked()
}

func (c *CatalogCache) Page(serverURL, libraryID string, username ...string) (CatalogPage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	page, ok := c.data.Pages[catalogPageKey(serverURL, libraryID, username...)]
	page.Items = append([]MediaItem(nil), page.Items...)
	return page, ok
}

func (c *CatalogCache) SetPage(serverURL, libraryID string, page CatalogPage, username ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureUsageLocked()
	page.Items = append([]MediaItem(nil), page.Items...)
	page.UpdatedAt = time.Now()
	c.data.Pages[catalogPageKey(serverURL, libraryID, username...)] = page
	key := catalogPageKey(serverURL, libraryID, username...)
	recordCatalogWrite(c.data.Usage, "pages", key)
	trimCatalogEntries(c.data.Pages, c.data.Usage, "pages", catalogMaxPages)
	c.saveAndLogLocked()
}

func (c *CatalogCache) Detail(serverURL, itemID string, username ...string) (MediaItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.data.Details[catalogServerKey(serverURL, username...)+":"+itemID]
	return item, ok
}

func (c *CatalogCache) SetDetail(serverURL string, item MediaItem, username ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item.ID == "" {
		return
	}
	c.setDetailLocked(serverURL, item, username...)
	c.saveAndLogLocked()
}

// SetDetailAsync updates the in-memory cache immediately and persists it in a
// background goroutine. The returned channel receives one save error (or nil)
// and then closes, so UI callbacks can leave disk I/O to the caller thread.
func (c *CatalogCache) SetDetailAsync(serverURL string, item MediaItem, username ...string) <-chan error {
	done := make(chan error, 1)
	if item.ID == "" {
		done <- nil
		close(done)
		return done
	}
	c.mu.Lock()
	c.setDetailLocked(serverURL, item, username...)
	c.mu.Unlock()
	go func() {
		err := c.save()
		if err != nil {
			log.Printf("fnmovie: save catalog cache after detail update: %v", err)
		}
		done <- err
		close(done)
	}()
	return done
}

func (c *CatalogCache) setDetailLocked(serverURL string, item MediaItem, username ...string) {
	if item.ID == "" {
		return
	}
	c.ensureUsageLocked()
	if c.data.Details == nil {
		c.data.Details = map[string]MediaItem{}
	}
	key := catalogServerKey(serverURL, username...) + ":" + item.ID
	c.data.Details[key] = item
	recordCatalogWrite(c.data.Usage, "details", key)
	trimCatalogEntries(c.data.Details, c.data.Usage, "details", catalogMaxDetails)
}

func (c *CatalogCache) save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

func (c *CatalogCache) saveAndLogLocked() {
	if err := c.saveLocked(); err != nil {
		log.Printf("fnmovie: save catalog cache: %v", err)
	}
}

func (c *CatalogCache) saveLocked() error {
	if c.path == "" {
		return nil
	}
	c.data.Version = catalogCacheVersion
	data, err := json.Marshal(c.data)
	if err != nil {
		return fmt.Errorf("encode catalog cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return fmt.Errorf("create catalog cache directory: %w", err)
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(c.path), "catalog-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary catalog cache: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temporary catalog cache: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("flush temporary catalog cache: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary catalog cache: %w", err)
	}
	if err := verifyCatalogReplaceDirectory(tmpPath, c.path); err != nil {
		return fmt.Errorf("verify temporary catalog cache location: %w", err)
	}
	if err := replaceCatalogFile(tmpPath, c.path); err != nil {
		return fmt.Errorf("replace catalog cache: %w", err)
	}
	return nil
}

func verifyCatalogReplaceDirectory(source, destination string) error {
	sourceInfo, err := os.Stat(filepath.Dir(source))
	if err != nil {
		return fmt.Errorf("stat temporary file directory: %w", err)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("temporary file parent is not a directory")
	}
	destinationInfo, err := os.Stat(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("stat destination directory: %w", err)
	}
	if !destinationInfo.IsDir() {
		return fmt.Errorf("destination parent is not a directory")
	}
	if !os.SameFile(sourceInfo, destinationInfo) {
		return fmt.Errorf("temporary file and destination are in different directories")
	}
	return nil
}
