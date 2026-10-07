package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
	Refreshed         bool
}

type catalogDisk struct {
	Libraries map[string][]MediaLibrary `json:"libraries"`
	Pages     map[string]CatalogPage    `json:"pages"`
}

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

func NewCatalogCache() *CatalogCache {
	dir, err := os.UserConfigDir()
	if err != nil {
		return &CatalogCache{data: catalogDisk{Pages: map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{}}}
	}
	dir = filepath.Join(dir, "FnMovie")
	_ = os.MkdirAll(dir, 0700)
	c := &CatalogCache{path: filepath.Join(dir, "catalog.json"), data: catalogDisk{Pages: map[string]CatalogPage{}, Libraries: map[string][]MediaLibrary{}}}
	if raw, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(raw, &c.data)
	}
	if c.data.Pages == nil {
		c.data.Pages = map[string]CatalogPage{}
	}
	if c.data.Libraries == nil {
		c.data.Libraries = map[string][]MediaLibrary{}
	}
	return c
}

func catalogServerKey(serverURL string) string {
	sum := sha256.Sum256([]byte(normalizeServerURL(serverURL)))
	return hex.EncodeToString(sum[:12])
}

func catalogPageKey(serverURL, libraryID string) string {
	return catalogServerKey(serverURL) + ":" + libraryID
}

func (c *CatalogCache) Libraries(serverURL string) []MediaLibrary {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]MediaLibrary(nil), c.data.Libraries[catalogServerKey(serverURL)]...)
}

func (c *CatalogCache) SetLibraries(serverURL string, libraries []MediaLibrary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Libraries[catalogServerKey(serverURL)] = append([]MediaLibrary(nil), libraries...)
	c.saveLocked()
}

func (c *CatalogCache) Page(serverURL, libraryID string) (CatalogPage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	page, ok := c.data.Pages[catalogPageKey(serverURL, libraryID)]
	page.Items = append([]MediaItem(nil), page.Items...)
	return page, ok
}

func (c *CatalogCache) SetPage(serverURL, libraryID string, page CatalogPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	page.Items = append([]MediaItem(nil), page.Items...)
	page.UpdatedAt = time.Now()
	c.data.Pages[catalogPageKey(serverURL, libraryID)] = page
	c.saveLocked()
}

func (c *CatalogCache) saveLocked() {
	if c.path == "" {
		return
	}
	data, err := json.Marshal(c.data)
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Rename(tmp, c.path)
	}
}
