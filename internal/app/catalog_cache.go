package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type catalogDisk struct {
	Version   int                       `json:"version"`
	Libraries map[string][]MediaLibrary `json:"libraries"`
	Pages     map[string]CatalogPage    `json:"pages"`
	Details   map[string]MediaItem      `json:"details"`
	// Usage records the last write time for each cache entry. It is keyed by
	// cache kind and entry key so identical page/detail keys stay independent.
	Usage map[string]time.Time `json:"usage,omitempty"`
}

const catalogCacheVersion = 3

// These limits bound the persistent catalog file across all servers and
// accounts. Details are the largest entries, while pages contain many items.
const (
	catalogMaxLibrarySets = 32
	catalogMaxPages       = 256
	catalogMaxDetails     = 512
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
		data := catalogDisk{Version: catalogCacheVersion, Pages: map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{}, Details: map[string]MediaItem{}, Usage: map[string]time.Time{}}
		return &CatalogCache{data: data}
	}
	dir = filepath.Join(dir, "FnMovie")
	_ = os.MkdirAll(dir, 0700)
	c := &CatalogCache{path: filepath.Join(dir, "catalog.json"), data: catalogDisk{Version: catalogCacheVersion, Pages: map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{}, Details: map[string]MediaItem{}, Usage: map[string]time.Time{}}}
	if raw, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(raw, &c.data)
	}
	migrateCatalogDisk(&c.data)
	return c
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
	if data.Usage == nil {
		data.Usage = map[string]time.Time{}
	}
	trimCatalogEntries(data.Libraries, data.Usage, "libraries", catalogMaxLibrarySets)
	trimCatalogEntries(data.Pages, data.Usage, "pages", catalogMaxPages)
	trimCatalogEntries(data.Details, data.Usage, "details", catalogMaxDetails)
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
	c.saveLocked()
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
	c.saveLocked()
}

func (c *CatalogCache) Detail(serverURL, itemID string, username ...string) (MediaItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.data.Details[catalogServerKey(serverURL, username...)+":"+itemID]
	return item, ok
}

func (c *CatalogCache) SetDetail(serverURL string, item MediaItem, username ...string) {
	if item.ID == "" {
		return
	}
	c.mu.Lock()
	c.ensureUsageLocked()
	key := catalogServerKey(serverURL, username...) + ":" + item.ID
	c.data.Details[key] = item
	recordCatalogWrite(c.data.Usage, "details", key)
	trimCatalogEntries(c.data.Details, c.data.Usage, "details", catalogMaxDetails)
	c.saveLocked()
	c.mu.Unlock()
}

func (c *CatalogCache) saveLocked() {
	if c.path == "" {
		return
	}
	c.data.Version = catalogCacheVersion
	data, err := json.Marshal(c.data)
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Rename(tmp, c.path)
	}
}
