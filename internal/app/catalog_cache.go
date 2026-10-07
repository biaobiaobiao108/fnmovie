package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
}

const catalogCacheVersion = 3

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
		data := catalogDisk{Version: catalogCacheVersion, Pages: map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{}, Details: map[string]MediaItem{}}
		return &CatalogCache{data: data}
	}
	dir = filepath.Join(dir, "FnMovie")
	_ = os.MkdirAll(dir, 0700)
	c := &CatalogCache{path: filepath.Join(dir, "catalog.json"), data: catalogDisk{Version: catalogCacheVersion, Pages: map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{}, Details: map[string]MediaItem{}}}
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
	c.data.Libraries[catalogServerKey(serverURL, username...)] = append([]MediaLibrary(nil), libraries...)
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
	page.Items = append([]MediaItem(nil), page.Items...)
	page.UpdatedAt = time.Now()
	c.data.Pages[catalogPageKey(serverURL, libraryID, username...)] = page
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
	c.data.Details[catalogServerKey(serverURL, username...)+":"+item.ID] = item
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
