package app

import "time"

const (
	personCacheLimit     = 8
	personCacheItemLimit = 120
	personCacheTTL       = 5 * time.Minute
)

type personItemsCacheKey struct {
	serverURL, username, personGUID string
}

type personItemsCacheEntry struct {
	items     []MediaItem
	updatedAt time.Time
	used      uint64
}

// personItemsCache is owned by the application UI thread. Cached items provide
// an immediate preview; callers refresh in the background to obtain all works.
type personItemsCache struct {
	entries map[personItemsCacheKey]personItemsCacheEntry
	clock   func() time.Time
	serial  uint64
}

func newPersonItemsCache() *personItemsCache {
	return &personItemsCache{
		entries: make(map[personItemsCacheKey]personItemsCacheEntry),
		clock:   time.Now,
	}
}

func (cache *personItemsCache) Get(serverURL, username, personGUID string) ([]MediaItem, bool) {
	if cache == nil {
		return nil, false
	}
	key := personItemsCacheKey{serverURL, username, personGUID}
	entry, ok := cache.entries[key]
	if !ok {
		return nil, false
	}
	if cache.clock().Sub(entry.updatedAt) >= personCacheTTL {
		delete(cache.entries, key)
		return nil, false
	}
	cache.serial++
	entry.used = cache.serial
	cache.entries[key] = entry
	return cloneHomeProgress(entry.items), true
}

func (cache *personItemsCache) Put(serverURL, username, personGUID string, items []MediaItem) {
	if cache == nil || personGUID == "" {
		return
	}
	now := cache.clock()
	for key, entry := range cache.entries {
		if now.Sub(entry.updatedAt) >= personCacheTTL {
			delete(cache.entries, key)
		}
	}
	key := personItemsCacheKey{serverURL, username, personGUID}
	if _, exists := cache.entries[key]; !exists && len(cache.entries) >= personCacheLimit {
		var oldestKey personItemsCacheKey
		oldestUse := ^uint64(0)
		for candidate, entry := range cache.entries {
			if entry.used < oldestUse {
				oldestKey, oldestUse = candidate, entry.used
			}
		}
		delete(cache.entries, oldestKey)
	}
	if len(items) > personCacheItemLimit {
		items = items[:personCacheItemLimit]
	}
	cache.serial++
	cache.entries[key] = personItemsCacheEntry{
		items: cloneHomeProgress(items), updatedAt: now, used: cache.serial,
	}
}

func (cache *personItemsCache) Clear() {
	if cache != nil {
		clear(cache.entries)
		cache.serial = 0
	}
}
