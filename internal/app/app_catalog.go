package app

import (
	"context"
	"strings"
	"time"
)

func (a *appState) visibleItems() []MediaItem {
	source := a.items
	if query := strings.TrimSpace(a.query); query != "" {
		source = nil
		if state := a.catalogs[a.currentCatalogKey()]; state != nil {
			source = state.Items
		}
		return a.mediaView.Get(mediaViewKey{Revision: a.dataRevision, Section: "search", Query: query}, source)
	}
	if a.section == "library" {
		// The empty ID is also the all-media catalog key. Fail closed if a
		// library selection goes stale instead of showing that shared catalog.
		if strings.TrimSpace(a.libraryID) == "" {
			return nil
		}
		if state := a.catalogs[a.currentCatalogKey()]; state != nil {
			source = state.Items
		} else {
			source = nil
		}
	}
	key := mediaViewKey{
		Revision: a.dataRevision, Section: a.section, LibraryID: a.libraryID,
		Query: normalizeMediaQuery(a.query), Tab: a.selectedTab, GroupEpisodes: true,
	}
	return a.mediaView.Get(key, source)
}

func (a *appState) currentCatalogKey() string {
	libraryID, mediaType, query := a.catalogRequestScope()
	return catalogStateKey(libraryID, mediaType, query)
}

func (a *appState) catalogRequestScope() (libraryID, mediaType, query string) {
	if query = strings.TrimSpace(a.query); query != "" {
		return "", "", query
	}
	return a.libraryID, a.currentMediaType(), ""
}

func (a *appState) currentMediaType() string {
	if a.section == "library" {
		for _, lib := range a.libraries {
			if lib.ID == a.libraryID {
				switch strings.ToLower(strings.TrimSpace(lib.Kind)) {
				case "tv":
					return "tv"
				case "movie":
					return "movie"
				default:
					return ""
				}
			}
		}
		return ""
	}
	return catalogMediaType(a.section)
}

func (a *appState) cancelCatalogRequests() {
	for _, state := range a.catalogs {
		if state.Cancel != nil {
			state.Cancel()
			state.Cancel = nil
			state.Loading = false
		}
	}
}

func (a *appState) currentCatalogLoading() bool {
	state := a.catalogs[a.currentCatalogKey()]
	return state != nil && state.Loading
}

func (a *appState) fetchCatalogPage(ctx context.Context, key string, server *Server, serverURL, libraryID, mediaType, query string, page int, first bool) {
	username := a.settings.Username
	go func() {
		var refreshedCredential *Credential
		requestPage := func() ([]MediaItem, int, error) {
			if query != "" {
				return server.GlobalSearchPageContext(ctx, query, page, catalogPageSize)
			}
			return server.MediaPageContext(ctx, libraryID, mediaType, "", page, catalogPageSize)
		}
		items, total, err := requestPage()
		if err != nil && isAuthError(err) {
			if credentials, readErr := ReadCredential(serverURL); readErr == nil && credentials.Username == username && credentials.Password != "" {
				if result, loginErr := server.LoginContext(ctx, credentials.Username, credentials.Password); loginErr == nil {
					credentials.Token = result.Token
					refreshedCredential = &credentials
					items, total, err = requestPage()
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		a.window.Update(func() {
			if a.server != server {
				return
			}
			state := a.catalogs[key]
			if state == nil {
				return
			}
			if ctx.Err() != nil {
				return
			}
			if refreshedCredential != nil {
				_ = WriteCredential(serverURL, *refreshedCredential)
			}
			state.Loading = false
			state.Cancel = nil
			if err != nil {
				state.Err = "影视库读取失败：" + err.Error()
				if query != "" {
					state.Err = "搜索失败：" + err.Error()
				}
				state.PageAutoRequested = true
				if key == a.currentCatalogKey() {
					a.status = state.Err
					if isAuthError(err) {
						a.loggedIn = false
						a.loginOpen = true
					}
				}
				return
			}
			if first {
				state.Items = items
			} else {
				state.Items = appendUniqueItems(state.Items, items)
			}
			a.dataRevision++
			a.mediaView.Invalidate()
			state.Total = total
			state.NextPage = page + 1
			state.Exhausted = catalogPageExhausted(len(items), len(state.Items), total, catalogPageSize)
			state.PageAutoRequested = false
			state.UpdatedAt = time.Now()
			state.Refreshed = true
			state.Err = ""
			if first && query == "" {
				pageItems := append([]MediaItem(nil), state.Items...)
				go a.catalogCache.SetPage(serverURL, catalogCacheScope(libraryID, mediaType), CatalogPage{Items: pageItems, Total: total}, username)
			}
			if key == a.currentCatalogKey() {
				a.items = state.Items
				if len(state.Items) == 0 {
					a.status = "服务器已连接，但没有找到可显示的媒体"
				} else {
					a.status = ""
				}
			}
		})
	}()
}

func (a *appState) loadNextCatalogPage(retry bool) {
	key := a.currentCatalogKey()
	state := a.catalogs[key]
	if a.server == nil || state == nil || state.Loading || state.Exhausted {
		return
	}
	if state.Err != "" && !retry {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.Cancel, state.Loading, state.Err, state.PageAutoRequested = cancel, true, "", true
	libraryID, mediaType, query := a.catalogRequestScope()
	a.fetchCatalogPage(ctx, key, a.server, a.settings.ServerURL, libraryID, mediaType, query, state.NextPage, false)
}
