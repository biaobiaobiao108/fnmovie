package app

import "time"

const catalogStateLimit = 8

// Inactive library/search results are rebuildable; keep recent navigation only.
func (a *appState) trimCatalogStates(active string) {
	if state := a.catalogs[active]; state != nil {
		state.LastUsed = time.Now()
	}
	for len(a.catalogs) > catalogStateLimit {
		oldest := ""
		var oldestTime time.Time
		for key, state := range a.catalogs {
			if key != active && (oldest == "" || state.LastUsed.Before(oldestTime)) {
				oldest, oldestTime = key, state.LastUsed
			}
		}
		if oldest == "" {
			return
		}
		if cancel := a.catalogs[oldest].Cancel; cancel != nil {
			cancel()
		}
		delete(a.catalogs, oldest)
	}
}

func (a *appState) closeDetail() {
	a.cancelDetailRequests()
	a.detailRequest++
	a.selected = nil
	a.seriesEpisodes, a.seriesEpisodeCache, a.seriesRootCast = nil, nil, nil
	a.seriesEpisodeRequest++
	a.seriesCastRequest++
	a.seriesLoading, a.seriesEpisodeLoading, a.castLoading = false, false, false
	a.seriesRootCastLoading = false
	a.seriesError, a.seriesEpisodeError, a.castError, a.selectedSeasonID = "", "", "", ""
}

func (a *appState) isCurrentDetailRequest(server *Server, itemID string, request uint64) bool {
	return a.detailRequest == request && a.server == server && a.selected != nil && a.selected.ID == itemID
}
