package app

import (
	"context"
	"strings"
	"time"
)

func (a *appState) logout() {
	a.resetHome()
	a.posters.CancelPending()
	serverURL := a.settings.ServerURL
	if a.loginCancel != nil {
		a.loginCancel()
		a.loginCancel = nil
	}
	a.busy = false
	a.cancelPlaybackLoading()
	a.closePerson()
	if a.libraryCancel != nil {
		a.libraryCancel()
		a.libraryCancel = nil
	}
	a.cancelCatalogRequests()
	a.loggedIn = false
	a.loginOpen = true
	a.libraryLoading = false
	a.libraries = nil
	a.items = nil
	a.catalogs = map[string]*CatalogState{}
	a.closeDetail()
	a.libraryID = ""
	a.query = ""
	a.password = ""
	a.mediaView.Invalidate()
	a.dataRevision++
	a.server = nil
	a.favoritePending = map[string]bool{}
	a.seriesEpisodeRequest++
	a.section = "home"
	a.status = "已退出登录"
	_ = DeleteCredential(serverURL)
	_ = SaveSettings(a.settings)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) updateDisplayScale() {
	if a.window == nil {
		return
	}
	scale := windowScale(a.window.NativeHandle())
	if scale == a.displayScale {
		return
	}
	a.displayScale = scale
	a.window.Invalidate()
}

func (a *appState) login() {
	if a.busy {
		return
	}
	serverURL := strings.TrimSpace(a.serverAddress)
	if serverURL == "" {
		a.status = "请输入服务器地址后再连接"
		return
	}
	if a.password == "" {
		if credentials, err := ReadCredential(serverURL); err == nil && credentials.Username == a.username {
			a.password = credentials.Password
		}
	}
	if strings.TrimSpace(a.password) == "" {
		a.status = "请输入账户密码后再连接"
		return
	}
	a.busy = true
	a.status = "正在连接服务器…"
	username, password := a.username, a.password
	settings := a.settings
	settings.ServerURL = serverURL
	ctx, cancel := context.WithCancel(context.Background())
	a.loginCancel = cancel
	if a.window != nil {
		a.window.Invalidate()
	}
	go func() {
		server := NewServer(serverURL, "")
		result, err := server.LoginContext(ctx, username, password)
		a.window.Update(func() {
			if ctx.Err() != nil {
				return
			}
			a.loginCancel = nil
			defer cancel()
			if err == nil {
				settings.Username = username
				err = WriteCredential(serverURL, Credential{Username: username, Password: password, Token: result.Token})
				if err == nil {
					err = SaveSettings(settings)
				}
			}
			a.busy = false
			if err != nil {
				a.status = "连接失败：" + err.Error()
				a.loginOpen = true
				return
			}
			a.server = server
			a.resetHome()
			a.posters.CancelPending()
			a.settings = settings
			if a.libraryCancel != nil {
				a.libraryCancel()
				a.libraryCancel = nil
			}
			a.libraryLoading = false
			a.libraries = nil
			a.closePerson()
			a.favoritePending = map[string]bool{}
			a.seriesEpisodeRequest++
			a.loggedIn = true
			a.loginOpen = false
			a.mediaView.Invalidate()
			a.dataRevision++
			a.cancelCatalogRequests()
			a.catalogs = map[string]*CatalogState{}
			a.items = nil
			a.closeDetail()
			a.section = "home"
			a.libraryID = ""
			a.status = "连接成功，正在读取影视库…"
			a.loadLibraries()
		})
	}()
}

func (a *appState) loadLibraries() {
	if a.server == nil || a.libraryLoading {
		return
	}
	a.libraryLoading = true
	a.libraryErr = ""
	server := a.server
	serverURL, username, password := a.settings.ServerURL, a.settings.Username, a.password
	ctx, cancel := context.WithCancel(context.Background())
	a.libraryCancel = cancel
	if cached := a.catalogCache.Libraries(serverURL, username); len(a.libraries) == 0 && len(cached) > 0 {
		a.libraries = cached
		a.home.LibrariesReady = true
		a.loadHomeHeroes(false)
		a.loadLibrary()
	}
	a.window.Invalidate()
	go func() {
		libraries, err := server.LibrariesContext(ctx)
		var refreshedToken string
		if err != nil && isAuthError(err) && username != "" && password != "" {
			if result, loginErr := server.LoginContext(ctx, username, password); loginErr == nil {
				refreshedToken = result.Token
				libraries, err = server.LibrariesContext(ctx)
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			a.catalogCache.SetLibraries(serverURL, libraries, username)
		}
		a.window.Update(func() {
			if ctx.Err() != nil || a.server != server {
				return
			}
			if refreshedToken != "" {
				_ = WriteCredential(serverURL, Credential{Username: username, Password: password, Token: refreshedToken})
			}
			a.libraryCancel = nil
			a.libraryLoading = false
			if err != nil {
				a.libraryErr = err.Error()
				a.status = "影视库列表读取失败：" + err.Error()
				if isAuthError(err) {
					a.loggedIn = false
					a.loginOpen = true
				}
				return
			}
			a.libraryErr = ""
			if !sameHomeLibraryScope(a.libraries, libraries) {
				if a.home.HeroCancel != nil {
					a.home.HeroCancel()
					a.home.HeroCancel = nil
				}
				a.home.Heroes = nil
				a.home.HeroIndex, a.home.HeroPrevious = 0, 0
				a.home.HeroLoading, a.home.HeroAttempted, a.home.HeroFromCache = false, false, false
			}
			a.libraries = libraries
			a.home.LibrariesReady = true
			if a.libraryID != "" {
				found := false
				for _, library := range libraries {
					if library.ID == a.libraryID {
						found = true
						break
					}
				}
				if !found {
					if a.section == "library" {
						a.section = "movies"
						a.status = "所选影视库已不可访问，已返回电影"
					}
					a.libraryID = ""
				}
			}
			if len(libraries) == 0 {
				a.status = "没有个人影视库，正在加载系统分类"
				a.loadLibrary()
				return
			}
			if a.libraryID == "" {
				a.libraryID = ""
			}
			a.loadHomeHeroes(false)
			a.loadLibrary()
		})
	}()
}

func (a *appState) loadLibrary() {
	if a.server == nil {
		return
	}
	if a.section == "home" && strings.TrimSpace(a.query) == "" {
		a.cancelCatalogRequests()
		a.loadHomeHeroes(false)
		a.loadContinueWatching(false)
		return
	}
	if a.section == "library" && strings.TrimSpace(a.libraryID) == "" && strings.TrimSpace(a.query) == "" {
		a.cancelCatalogRequests()
		a.items = nil
		a.status = "无法读取所选影视库：服务器没有返回有效的影视库标识"
		a.window.Invalidate()
		return
	}
	server := a.server
	libraryID, mediaType, query := a.catalogRequestScope()
	serverURL := a.settings.ServerURL
	username := a.settings.Username
	key := catalogStateKey(libraryID, mediaType, query)
	for otherKey, other := range a.catalogs {
		if otherKey != key && other.Cancel != nil {
			other.Cancel()
			other.Cancel = nil
			other.Loading = false
		}
	}
	state := a.catalogs[key]
	if state == nil {
		state = &CatalogState{NextPage: 1}
		a.catalogs[key] = state
		if query == "" {
			if cached, ok := a.catalogCache.Page(serverURL, catalogCacheScope(libraryID, mediaType), username); ok {
				state.Items = cached.Items
				state.Total = cached.Total
				state.NextPage = 2
				state.UpdatedAt = cached.UpdatedAt
				state.Exhausted = catalogPageExhausted(len(cached.Items), len(cached.Items), cached.Total, catalogPageSize)
				a.dataRevision++
			}
		}
	}
	a.items = state.Items
	a.trimCatalogStates(key)
	if len(state.Items) > 0 {
		a.status = ""
	} else if state.Err != "" {
		a.status = state.Err
	} else {
		a.status = "正在读取首批影片…"
	}
	if state.Loading {
		a.window.Invalidate()
		return
	}
	if state.Err == "" && state.Refreshed && !state.UpdatedAt.IsZero() && time.Since(state.UpdatedAt) < 5*time.Minute {
		a.window.Invalidate()
		return
	}
	if state.Cancel != nil {
		state.Cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.Cancel, state.Loading, state.Err = cancel, true, ""
	state.NextPage = 1
	a.window.Invalidate()
	a.fetchCatalogPage(ctx, key, server, serverURL, libraryID, mediaType, query, 1, true)
}
