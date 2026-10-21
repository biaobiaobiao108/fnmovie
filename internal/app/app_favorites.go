package app

func (a *appState) toggleFavorite(item MediaItem) {
	if a.server == nil || a.favoritePending[item.ID] {
		return
	}
	server := a.server
	if a.favoritePending == nil {
		a.favoritePending = map[string]bool{}
	}
	a.favoritePending[item.ID] = true
	if a.window != nil {
		a.window.Invalidate()
	}
	go func() {
		if err := server.ToggleFavorite(item); err != nil {
			a.window.Update(func() {
				if a.server != server {
					return
				}
				delete(a.favoritePending, item.ID)
				a.status = "收藏状态更新失败：" + err.Error()
			})
			return
		}
		a.window.Update(func() {
			if a.server != server {
				return
			}
			favorite := !item.Favorite
			delete(a.favoritePending, item.ID)
			a.updateFavoriteProjection(item.ID, favorite)
			if a.selected != nil && a.selected.ID == item.ID {
				a.selected.Favorite = favorite
				if a.catalogCache != nil {
					cache := a.catalogCache
					serverURL, username := a.settings.ServerURL, a.settings.Username
					item := cloneHomeProgress([]MediaItem{*a.selected})[0]
					cache.SetDetailAsync(serverURL, item, username)
				}
			}
			a.status = "收藏状态已更新"
			if state := a.catalogs[a.currentCatalogKey()]; state != nil {
				state.Refreshed = false
			}
			a.window.Invalidate()
		})
	}()
}

func (a *appState) updateFavoriteProjection(itemID string, favorite bool) {
	if a.selectedPerson != nil && a.selectedPerson.ID == itemID {
		a.selectedPerson.Favorite = favorite
	}
	if a.selected != nil {
		for i := range a.selected.Cast {
			if a.selected.Cast[i].ID == itemID {
				a.selected.Cast[i].Favorite = favorite
			}
		}
	}
	for _, state := range a.catalogs {
		for i := range state.Items {
			if state.Items[i].ID == itemID {
				state.Items[i].Favorite = favorite
				state.Refreshed = false
			}
		}
	}
	for i := range a.items {
		if a.items[i].ID == itemID {
			a.items[i].Favorite = favorite
		}
	}
	a.dataRevision++
	a.mediaView.Invalidate()
}
