package main

import (
	"sort"
	"strings"
)

type mediaViewKey struct {
	Revision  uint64
	Section   string
	LibraryID string
	Query     string
	Tab       int
}

// mediaViewCache keeps the filtered/sorted projection stable across ordinary
// UI frames. The source slices can contain thousands of catalog entries.
type mediaViewCache struct {
	key   mediaViewKey
	valid bool
	items []MediaItem
}

func (c *mediaViewCache) Get(key mediaViewKey, source []MediaItem) []MediaItem {
	if c.valid && c.key == key {
		return c.items
	}
	c.items = deriveVisibleItems(source, key)
	c.key, c.valid = key, true
	return c.items
}

func (c *mediaViewCache) Invalidate() {
	c.valid = false
	c.items = nil
}

func deriveVisibleItems(source []MediaItem, key mediaViewKey) []MediaItem {
	out := make([]MediaItem, 0, len(source))
	for _, item := range source {
		if key.Section == "library" {
			if itemLibraryID, hasLibraryID := itemLibrary(item.Raw); hasLibraryID && itemLibraryID != key.LibraryID {
				continue
			}
		}
		if key.Section == "movies" && item.Kind == "tv" || key.Section == "tv" && item.Kind != "tv" {
			continue
		}
		if key.Section == "favorites" && !item.Favorite || key.Section == "history" && !item.Watched {
			continue
		}
		switch key.Tab {
		case 1:
			if item.Kind == "tv" {
				continue
			}
		case 2:
			if item.Kind != "tv" {
				continue
			}
		}
		out = append(out, item)
	}
	if key.Tab == 3 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].AddedAt > out[j].AddedAt })
	} else if key.Section == "home" {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Watched != out[j].Watched {
				return out[i].Watched
			}
			return out[i].AddedAt > out[j].AddedAt
		})
	}
	return out
}

func normalizeMediaQuery(query string) string { return strings.TrimSpace(query) }
