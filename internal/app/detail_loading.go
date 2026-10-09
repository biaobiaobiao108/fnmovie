package app

import (
	"context"
	"sync"
)

type detailPart struct {
	kind    string
	item    MediaItem
	people  []CastMember
	seasons []MediaSeason
	err     error
}

// Each region can become usable independently. Two workers bound outstanding
// requests, with seasons first so episode loading need not wait for credits.
func fetchDetailParts(ctx context.Context, server *Server, item MediaItem, publish func(detailPart)) {
	kinds := []string{"metadata", "people"}
	if item.IsSeries {
		kinds = []string{"seasons", "metadata", "people"}
	}
	jobs := make(chan string, len(kinds))
	results := make(chan detailPart, len(kinds))
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for kind := range jobs {
				part := detailPart{kind: kind}
				switch kind {
				case "metadata":
					part.item, part.err = server.DetailContext(ctx, item)
				case "people":
					part.people, part.err = server.PeopleContext(ctx, item.ID)
				case "seasons":
					part.seasons, part.err = server.SeriesSeasonsContext(ctx, item)
				}
				results <- part
			}
		}()
	}
	for _, kind := range kinds {
		jobs <- kind
	}
	close(jobs)
	defer workers.Wait()
	for range kinds {
		select {
		case part := <-results:
			if ctx.Err() != nil {
				return
			}
			publish(part)
		case <-ctx.Done():
			return
		}
	}
}

func (a *appState) cancelDetailRequests() {
	for _, cancel := range []context.CancelFunc{a.detailCancel, a.seriesEpisodeCancel, a.seriesCastCancel} {
		if cancel != nil {
			cancel()
		}
	}
	a.detailCancel, a.seriesEpisodeCancel, a.seriesCastCancel = nil, nil, nil
}

func (a *appState) loadDetailParts(item MediaItem, request uint64, serverURL, username string) {
	if a.window == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.detailCancel = cancel
	server, window, cache := a.server, a.window, a.catalogCache
	a.seriesLoading = item.IsSeries && len(item.Seasons) == 0
	remaining := 2
	if item.IsSeries {
		remaining = 3
	}
	anySuccess := false
	go fetchDetailParts(ctx, server, item, func(part detailPart) {
		window.Update(func() {
			if ctx.Err() != nil || !a.isCurrentDetailRequest(server, item.ID, request) {
				return
			}
			defer func() {
				remaining--
				anySuccess = anySuccess || part.err == nil
				if remaining == 0 && anySuccess && cache != nil {
					snapshot := cloneHomeProgress([]MediaItem{*a.selected})[0]
					go cache.SetDetail(serverURL, snapshot, username)
				}
			}()
			switch part.kind {
			case "metadata":
				if part.err != nil {
					a.status = "详情读取失败：" + part.err.Error()
					return
				}
				// Keep independently loaded credits and seasons when metadata arrives.
				part.item.Cast, part.item.Seasons = nil, nil
				updated := mergeMediaDetail(*a.selected, part.item)
				a.selected = &updated
			case "seasons":
				a.seriesLoading = false
				if part.err != nil {
					a.seriesError = "季度列表读取失败：" + part.err.Error()
					return
				}
				a.selected.Seasons = part.seasons
				if len(part.seasons) > 0 && a.selectedSeasonID == "" {
					a.selectSeriesSeason(part.seasons[0].ID)
				} else if len(part.seasons) == 0 && !a.seriesRootCastLoading {
					a.castLoading = false
				}
			case "people":
				a.seriesRootCastLoading = false
				if item.IsSeries {
					a.seriesRootCast = append([]CastMember(nil), part.people...)
					if len(part.people) > 0 {
						a.selected.Cast = part.people
						a.castLoading, a.castError = false, ""
						if a.seriesCastCancel != nil {
							a.seriesCastCancel()
							a.seriesCastCancel = nil
						}
					} else if a.selectedSeasonID != "" && !a.seriesEpisodeLoading {
						a.loadSeriesCast(a.selectedSeasonID, a.seriesEpisodes)
					} else {
						a.castLoading = a.seriesLoading || a.seriesEpisodeLoading
					}
				} else {
					a.castLoading = false
					if part.err == nil {
						a.selected.Cast = part.people
					}
				}
				if part.err != nil {
					a.castError = "演职员读取失败：" + part.err.Error()
				}
			}
		})
	})
}

func (a *appState) resumeDetailLoading() {
	if a.selected == nil || a.server == nil || a.window == nil {
		return
	}
	a.cancelDetailRequests()
	a.detailRequest++
	a.seriesRootCastLoading = a.selected.IsSeries
	if a.selectedSeasonID != "" {
		a.loadSeriesSeason(a.selectedSeasonID, false)
	}
	a.loadDetailParts(*a.selected, a.detailRequest, a.settings.ServerURL, a.settings.Username)
}
