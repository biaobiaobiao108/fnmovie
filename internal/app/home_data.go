package app

import (
	"context"
	"errors"
	"math/rand"
	"sync"
)

const homeHighlightLimit = 8
const homeSamplePageSize = 60

type homeSampleLibrary struct {
	id    string
	total int
	pages map[int][]MediaItem
}

// HomeHighlightsContext samples global positions, so each title has the same
// probability regardless of its library's size. It retains only bounded pages.
// seed is chosen once by the caller for each application/account session.
func (s *Server) HomeHighlightsContext(ctx context.Context, libraries []MediaLibrary, seed int64) ([]MediaItem, error) {
	rng := rand.New(rand.NewSource(seed))
	var available []homeSampleLibrary
	var failures []error
	seenLibraries := make(map[string]bool)
	total := 0
	for _, library := range libraries {
		if library.ID == "" || seenLibraries[library.ID] {
			continue
		}
		seenLibraries[library.ID] = true
		items, count, err := s.HomeCandidatesPageContext(ctx, library.ID, 1, homeSamplePageSize)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if count < 1 {
			continue
		}
		available = append(available, homeSampleLibrary{id: library.ID, total: count, pages: map[int][]MediaItem{1: items}})
		total += count
	}
	if total == 0 {
		return []MediaItem{}, errors.Join(failures...)
	}
	positions := make(map[int]bool)
	seenItems := make(map[string]bool)
	items := make([]MediaItem, 0, homeHighlightLimit)
	fetches := 0
	// Initial eight positions and at most eight additional sample pages. The
	// position-attempt bound also prevents duplicate/stale servers spinning.
	for attempts := 0; attempts < 128 && len(items) < homeHighlightLimit && len(positions) < total; attempts++ {
		position := rng.Intn(total)
		if positions[position] {
			continue
		}
		positions[position] = true
		var library *homeSampleLibrary
		for i := range available {
			if position < available[i].total {
				library = &available[i]
				break
			}
			position -= available[i].total
		}
		page := position/homeSamplePageSize + 1
		batch, cached := library.pages[page]
		if !cached {
			if fetches >= homeHighlightLimit+8 {
				break
			}
			fetches++
			var err error
			batch, _, err = s.HomeCandidatesPageContext(ctx, library.id, page, homeSamplePageSize)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			library.pages[page] = batch
			if err != nil {
				failures = append(failures, err)
				continue
			}
		}
		offset := position % homeSamplePageSize
		if offset >= len(batch) {
			continue
		}
		item := batch[offset]
		if seenItems[item.ID] {
			continue
		}
		seenItems[item.ID] = true
		items = append(items, item)
	}
	// Two workers, no goroutine per title, including when image metadata is
	// absent. The base item stays useful if an individual detail request fails.
	jobs := make(chan int)
	var workers sync.WaitGroup
	var errorMu sync.Mutex
	for worker := 0; worker < 2; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				detail, err := s.ItemDetailContext(ctx, items[index])
				if err != nil {
					errorMu.Lock()
					failures = append(failures, err)
					errorMu.Unlock()
					continue
				}
				items[index] = detail
			}
		}()
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return items, errors.Join(failures...)
}
