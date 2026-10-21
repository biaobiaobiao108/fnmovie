package app

import (
	"context"

	"github.com/egoist/mygo/ui"
)

func (a *appState) selectSeriesSeason(seasonID string) {
	if a.seriesEpisodeCancel != nil {
		a.seriesEpisodeCancel()
		a.seriesEpisodeCancel = nil
	}
	if a.seriesCastCancel != nil {
		a.seriesCastCancel()
		a.seriesCastCancel = nil
	}
	a.selectedSeasonID = seasonID
	a.seriesEpisodes = nil
	a.seriesEpisodeError = ""
	a.seriesCastRequest++
	if a.selected != nil {
		a.selected.Cast = append([]CastMember(nil), a.seriesRootCast...)
		a.castLoading = len(a.seriesRootCast) == 0
		a.castError = ""
	}
	if items, ok := a.seriesEpisodeCache[seasonID]; ok {
		a.seriesEpisodes = items
		a.seriesEpisodeLoading = false
		a.loadSeriesCast(seasonID, items)
		return
	}
	a.loadSeriesSeason(seasonID, false)
}

func (a *appState) loadSeriesSeason(seasonID string, force bool) {
	if a.server == nil || seasonID == "" {
		return
	}
	if !force {
		if items, ok := a.seriesEpisodeCache[seasonID]; ok {
			a.seriesEpisodes = items
			a.seriesEpisodeLoading = false
			return
		}
	}
	server := a.server
	if a.seriesEpisodeCancel != nil {
		a.seriesEpisodeCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.seriesEpisodeCancel = cancel
	a.seriesEpisodeRequest++
	requestID := a.seriesEpisodeRequest
	selectedID := ""
	if a.selected != nil {
		selectedID = a.selected.ID
	}
	a.seriesEpisodeLoading = true
	a.seriesEpisodeError = ""
	go func() {
		items, err := server.SeasonEpisodesContext(ctx, seasonID)
		a.window.Update(func() {
			defer cancel()
			if ctx.Err() != nil || a.server != server || a.selected == nil || a.selected.ID != selectedID || a.selectedSeasonID != seasonID || requestID != a.seriesEpisodeRequest {
				return
			}
			a.seriesEpisodeCancel = nil
			a.seriesEpisodeLoading = false
			if err != nil {
				a.seriesEpisodeError = "集数读取失败：" + err.Error()
				a.loadSeriesCast(seasonID, nil)
				return
			}
			if a.seriesEpisodeCache == nil {
				a.seriesEpisodeCache = map[string][]MediaItem{}
			}
			a.seriesEpisodeCache[seasonID] = items
			a.seriesEpisodes = items
			a.loadSeriesCast(seasonID, items)
		})
	}()
}

// Some libraries attach credits to a season or episode rather than the TV
// root. Follow server IDs already returned by the hierarchy, at most two
// extra requests for the selected season, and reject stale season callbacks.
func (a *appState) loadSeriesCast(seasonID string, episodes []MediaItem) {
	if a.selected == nil || !a.selected.IsSeries {
		return
	}
	a.seriesCastRequest++
	if a.seriesCastCancel != nil {
		a.seriesCastCancel()
		a.seriesCastCancel = nil
	}
	if a.seriesRootCastLoading {
		return
	}
	if len(a.seriesRootCast) > 0 {
		a.selected.Cast = append([]CastMember(nil), a.seriesRootCast...)
		a.castLoading, a.castError = false, ""
		return
	}
	if a.server == nil {
		a.castLoading = false
		return
	}
	requestID, selectedID, server := a.seriesCastRequest, a.selected.ID, a.server
	episodeID := ""
	if len(episodes) > 0 {
		episodeID = episodes[0].ID
	}
	a.castLoading = true
	ctx, cancel := context.WithCancel(context.Background())
	a.seriesCastCancel = cancel
	go func() {
		people, err := server.PeopleContext(ctx, seasonID)
		if len(people) == 0 && episodeID != "" {
			people, err = server.PeopleContext(ctx, episodeID)
		}
		a.window.Update(func() {
			defer cancel()
			if ctx.Err() != nil || a.server != server || a.selected == nil || a.selected.ID != selectedID || a.selectedSeasonID != seasonID || a.seriesCastRequest != requestID {
				return
			}
			a.seriesCastCancel = nil
			a.castLoading = false
			if len(people) > 0 {
				a.selected.Cast = people
				a.castError = ""
			} else if err != nil {
				a.castError = "演职人员读取失败：" + err.Error()
			} else {
				a.castError = ""
				a.selected.Cast = nil
			}
		})
	}()
}

func (a *appState) openPerson(person CastMember) {
	a.cancelDetailRequests()
	if a.personCancel != nil {
		a.personCancel()
		a.personCancel = nil
	}
	a.selectedPerson = &person
	a.personFavoriteLoading = false
	a.personItems = nil
	a.personCached = false
	a.personLoading = true
	a.personError = ""
	a.personGrid = ui.GridState{}
	a.personScroll = ui.ScrollState{}
	a.personRequest++
	reqID := a.personRequest
	if a.server == nil {
		a.personLoading = false
		return
	}
	server := a.server
	personID := person.ID
	if personID == "" {
		a.personLoading = false
		a.personError = "该演职人员缺少服务器标识"
		return
	}
	if a.personCache == nil {
		a.personCache = newPersonItemsCache()
	}
	serverURL, username := a.settings.ServerURL, a.settings.Username
	if cached, ok := a.personCache.Get(serverURL, username, personID); ok {
		a.personItems = cached
		a.personCached = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.personCancel = cancel
	a.personFavoriteLoading = true
	go func() {
		item, err := server.ItemDetailContext(ctx, MediaItem{ID: personID})
		a.window.Update(func() {
			if ctx.Err() != nil || a.server != server || a.selectedPerson == nil || a.selectedPerson.ID != personID || a.personRequest != reqID {
				return
			}
			a.personFavoriteLoading = false
			if err == nil {
				a.updateFavoriteProjection(personID, item.Favorite)
			} else {
				a.status = "演员收藏状态读取失败：" + err.Error()
			}
		})
	}()
	go func() {
		items, err := server.PersonItemsContext(ctx, personID)
		a.window.Update(func() {
			if ctx.Err() != nil || a.server != server || a.selectedPerson == nil || a.selectedPerson.ID != personID || a.personRequest != reqID {
				return
			}
			a.personLoading = false
			if err != nil {
				a.personError = "作品读取失败：" + err.Error()
				return
			}
			a.personItems = items
			a.personCached = false
			a.personCache.Put(serverURL, username, personID, items)
		})
	}()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) closePerson() {
	if a.personCancel != nil {
		a.personCancel()
		a.personCancel = nil
	}
	a.personRequest++
	a.selectedPerson = nil
	a.personFavoriteLoading = false
	a.personItems = nil
	a.personLoading = false
	a.personCached = false
	a.personError = ""
	if a.window != nil {
		a.window.Invalidate()
	}
}
