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
	return s.HomeHighlightsProgressContext(ctx, libraries, seed, nil)
}

// HomeHighlightsProgressContext publishes usable candidates before fetching
// their optional details. Callbacks run serially and own independent snapshots.
func (s *Server) HomeHighlightsProgressContext(ctx context.Context, libraries []MediaLibrary, seed int64, onProgress func([]MediaItem)) ([]MediaItem, error) {
	rng := rand.New(rand.NewSource(seed))
	var available []homeSampleLibrary
	var failures []error
	seenLibraries := make(map[string]bool)
	var unique []MediaLibrary
	for _, library := range libraries {
		if library.ID == "" || seenLibraries[library.ID] {
			continue
		}
		seenLibraries[library.ID] = true
		unique = append(unique, library)
	}
	type libraryResult struct {
		items []MediaItem
		count int
		err   error
	}
	results := make([]libraryResult, len(unique))
	libraryJobs := make(chan int, 2)
	var libraryWorkers sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		libraryWorkers.Add(1)
		go func() {
			defer libraryWorkers.Done()
			for index := range libraryJobs {
				if ctx.Err() != nil {
					continue
				}
				items, count, err := s.HomeCandidatesPageContext(ctx, unique[index].ID, 1, homeSamplePageSize)
				results[index] = libraryResult{items: items, count: count, err: err}
			}
		}()
	}
libraryDispatch:
	for index := range unique {
		select {
		case libraryJobs <- index:
		case <-ctx.Done():
			break libraryDispatch
		}
	}
	close(libraryJobs)
	libraryWorkers.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	total := 0
	// Assemble in library input order, not completion order: a session seed
	// must select the same titles regardless of server response timing.
	for index, result := range results {
		if result.err != nil {
			failures = append(failures, result.err)
			continue
		}
		if result.count < 1 {
			continue
		}
		available = append(available, homeSampleLibrary{id: unique[index].ID, total: result.count, pages: map[int][]MediaItem{1: result.items}})
		total += result.count
	}
	if total == 0 {
		return []MediaItem{}, errors.Join(failures...)
	}
	positions := make(map[int]bool)
	seenItems := make(map[string]bool)
	items := make([]MediaItem, 0, homeHighlightLimit)
	publish := func() {
		if onProgress != nil && ctx.Err() == nil {
			onProgress(cloneHomeProgress(items))
		}
	}
	fetches := 0
	// Initial eight positions and at most eight additional sample pages. The
	// position-attempt bound also prevents duplicate/stale servers spinning.
	for attempts := 0; attempts < 128 && len(items) < homeHighlightLimit && len(positions) < total; attempts++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
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
		publish()
	}
	// Two workers, no goroutine per title, including when image metadata is
	// absent. The base item stays useful if an individual detail request fails.
	type detailJob struct {
		index int
		item  MediaItem
	}
	type detailResult struct {
		index int
		item  MediaItem
		err   error
	}
	jobs := make(chan detailJob, len(items))
	details := make(chan detailResult, len(items))
	var workers sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				detail, err := s.ItemDetailContext(ctx, job.item)
				details <- detailResult{index: job.index, item: detail, err: err}
			}
		}()
	}
	for index := range items {
		jobs <- detailJob{index: index, item: items[index]}
	}
	close(jobs)
	defer workers.Wait()
	for range items {
		select {
		case result := <-details:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if result.err != nil {
				failures = append(failures, result.err)
				continue
			}
			items[result.index] = result.item
			publish()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return items, errors.Join(failures...)
}

func cloneHomeProgress(items []MediaItem) []MediaItem {
	out := append([]MediaItem(nil), items...)
	for index := range out {
		item := &out[index]
		item.Genres = append([]string(nil), item.Genres...)
		item.Countries = append([]string(nil), item.Countries...)
		item.Cast = append([]CastMember(nil), item.Cast...)
		item.Sources = append([]StreamSource(nil), item.Sources...)
		item.Episodes = cloneHomeProgress(item.Episodes)
		item.Seasons = append([]MediaSeason(nil), item.Seasons...)
		for season := range item.Seasons {
			item.Seasons[season].Episodes = cloneHomeProgress(item.Seasons[season].Episodes)
		}
		if item.Raw != nil {
			item.Raw = cloneHomeProgressValue(item.Raw).(map[string]any)
		}
	}
	return out
}

func cloneHomeProgressValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, entry := range value {
			out[key] = cloneHomeProgressValue(entry)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for index, entry := range value {
			out[index] = cloneHomeProgressValue(entry)
		}
		return out
	default:
		return value
	}
}
