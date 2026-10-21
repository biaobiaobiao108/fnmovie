package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *appState) sidebar(c *ui.Context) {
	t := c.Theme()
	background := ui.Hex("#f4f2ed")
	ui.Column(c).Width(205).FillHeight().Padding(24, 16).Gap(8).Background(background).Border(1, t.Border).Children(func() {
		ui.Row(c).Padding(0, 10).Gap(11).Children(func() {
			if a.icon != nil {
				ui.Image(c, a.icon).Size(38, 38).Radius(10).Fit(ui.Cover)
			}
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "哞哩影院").FontSize(16).Bold()
			})
		})
		ui.Box(c).Height(15)
		a.navButton(c, sidebarNavIcon("home"), "首页", "home")
		a.navButton(c, sidebarNavIcon("movies"), "电影", "movies")
		a.navButton(c, sidebarNavIcon("tv"), "电视节目", "tv")
		a.navButton(c, sidebarNavIcon("favorites"), "我的收藏", "favorites")
		ui.Text(c, "影视库").Padding(6, 14).FontSize(11).Bold().TextColor(t.TextMuted)
		libraries := a.libraries
		a.sidebarList.Key = func(i int) any { return libraries[i].ID }
		list := ui.List(c, &a.sidebarList, len(libraries), func(i int) {
			library := libraries[i]
			selected := a.section == "library" && a.libraryID == library.ID
			button := actionButton(c, "").Justify(ui.Start).Padding(10, 14).Gap(10).Radius(9).BorderWidth(0).Background(ui.Color{}).TextColor(t.TextMuted).
				Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond})
			if selected {
				button.Background(ui.Hex("#e6eee8")).TextColor(t.Accent)
			} else if button.Hovered() {
				button.Background(ui.Hex("#eeece6"))
			}
			button.Children(func() {
				iconColor := t.TextMuted
				if selected {
					iconColor = t.Accent
				}
				ui.Icon(c, libraryNavigationIcon(library)).Size(20, 20).TextColor(iconColor)
				ui.Text(c, library.Name).FontSize(13).SingleLine()
			})
			if button.Clicked() {
				a.closeDetail()
				a.section, a.libraryID, a.query = "library", library.ID, ""
				a.closePerson()
				a.selectedTab = 0
				a.grid = ui.GridState{}
				a.loadLibrary()
			}
		}).Grow(1)
		bindSmoothScroll(list, &a.sidebarScroll)
		bindScrollEdgeFades(list, &a.sidebarScroll, background)
		if a.loggedIn {
			exitBtn := actionButton(c, "").Justify(ui.Start).Padding(10, 14).Gap(10).Radius(9).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond})
			if exitBtn.Hovered() {
				exitBtn.Background(ui.Hex("#eeece6"))
			}
			exitBtn.Children(func() {
				ui.Icon(c, sidebarNavIcon("logout")).Size(20, 20).TextColor(t.TextMuted)
				ui.Text(c, "退出登录").FontSize(13)
			})
			if exitBtn.Clicked() {
				a.logout()
			}
		} else if !a.loggedIn {
			loginBtn := actionButton(c, "").Justify(ui.Start).Padding(10, 14).Gap(10).Radius(9).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond})
			if loginBtn.Hovered() {
				loginBtn.Background(ui.Hex("#eeece6"))
			}
			loginBtn.Children(func() {
				ui.Icon(c, sidebarNavIcon("login")).Size(20, 20).TextColor(t.TextMuted)
				ui.Text(c, "登录到服务器").FontSize(13)
			})
			if loginBtn.Clicked() {
				a.loginOpen = true
			}
		}
	})
}

func (a *appState) navButton(c *ui.Context, icon *ui.SVG, label, key string) {
	selected := a.section == key
	e := actionButton(c, "").Justify(ui.Start).Padding(10, 14).Gap(10).Radius(9).BorderWidth(0)
	e.Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond})
	if selected {
		e.Background(ui.Hex("#e6eee8")).TextColor(c.Theme().Accent)
	} else {
		e.Background(ui.Color{}).TextColor(c.Theme().TextMuted)
		if e.Hovered() {
			e.Background(ui.Hex("#eeece6"))
		}
	}
	e.Children(func() {
		iconColor := c.Theme().TextMuted
		if selected {
			iconColor = c.Theme().Accent
		}
		if icon != nil {
			ui.Icon(c, icon).Size(20, 20).TextColor(iconColor)
		}
		ui.Text(c, label).FontSize(13)
	})
	if e.Clicked() {
		a.section = key
		if key == "home" {
			a.home.AllContinue = false
		}
		a.closeDetail()
		a.closePerson()
		a.query = ""
		a.grid = ui.GridState{}
		if key != "library" {
			a.libraryID = ""
		}
		if a.loggedIn {
			a.loadLibrary()
		}
	}
}

func (a *appState) topbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(14).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			label := map[string]string{"home": "首页", "movies": "电影", "tv": "电视节目", "favorites": "我的收藏", "library": a.selectedLibraryName()}[a.section]
			if label == "" {
				label = "影视库"
			}
			if strings.TrimSpace(a.query) != "" && a.selected == nil && a.selectedPerson == nil {
				label = "全局搜索"
			}
			ui.Text(c, label).FontSize(27).Bold()
		})
		if state := a.catalogs[a.currentCatalogKey()]; state != nil && state.Err != "" && len(state.Items) > 0 {
			if actionButton(c, "重试").Padding(5, 9).Clicked() {
				a.loadNextCatalogPage(true)
			}
		}
		if a.libraryErr != "" && !a.libraryLoading {
			if actionButton(c, "重试影视库").Padding(5, 9).Clicked() {
				a.loadLibraries()
			}
		}
		if a.loggedIn {
			if a.searchPending && c.Now().Sub(a.searchChangedAt) >= 300*time.Millisecond {
				a.searchPending = false
				a.loadLibrary()
			}
			if a.busy || a.libraryLoading || a.currentCatalogLoading() {
				ui.Box(c).Size(20, 20).Center().Children(func() {
					ui.Text(c, "↻").FontSize(15).TextColor(t.TextMuted)
				})
			} else {
				ui.Box(c).Size(20, 20)
			}
			ui.SearchField(c.Key("global-search"), &a.query).Width(270).Placeholder("全局搜索影片、演员或导演").Label("全局搜索").OnChange(func() {
				a.closeDetail()
				a.closePerson()
				a.selectedTab = 0
				a.status = ""
				if strings.TrimSpace(a.query) == "" {
					a.items = nil
					if state := a.catalogs[a.currentCatalogKey()]; state != nil {
						a.items = state.Items
					}
				}
				a.mediaView.Invalidate()
				a.searchChangedAt = c.Now()
				a.searchPending = true
				c.After(320 * time.Millisecond)
			})
		}
	})
}

func (a *appState) connectionView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Fill().Center().Children(func() {
		ui.Column(c).Width(440).Padding(34).Gap(16).Radius(16).Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Text(c, "连接哞哩影院").FontSize(22).Bold()
			ui.Text(c, "输入飞牛影视服务器地址和账户信息，在哞哩影院浏览与播放你的媒体。").FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.serverAddress).Placeholder("http://nas.example:5666/v").Label("服务器地址")
			ui.TextInput(c, &a.username).Placeholder("飞牛影视用户名").Label("用户名")
			if ui.TextInput(c, &a.password).Password().Placeholder("密码").Label("密码").Submitted() {
				a.login()
			}
			if a.status != "" {
				ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted)
			}
			if primaryActionButton(c, "连接并登录").Disabled(a.busy).Clicked() {
				a.login()
			}
		})
	})
}

func (a *appState) loginModal(c *ui.Context) {
	ui.Modal(c, &a.loginOpen, func() {
		t := c.Theme()
		ui.Column(c).Width(430).Padding(28, 30).Gap(15).Radius(16).Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Text(c, "连接哞哩影院").FontSize(22).Bold()
			ui.Text(c, "使用飞牛影视账户登录，在哞哩影院浏览媒体库并播放影片。").FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.serverAddress).Placeholder("http://nas.example:5666/v").Label("服务器地址")
			ui.TextInput(c, &a.username).Placeholder("飞牛影视用户名").Label("用户名")
			if ui.TextInput(c, &a.password).Password().Placeholder("密码").Label("密码").Submitted() {
				a.login()
			}
			if a.status != "" && !a.loggedIn {
				color := t.TextMuted
				if strings.Contains(a.status, "失败") || strings.Contains(a.status, "无法") {
					color = ui.Hex("#ad5148")
				}
				ui.Text(c, a.status).FontSize(11).TextColor(color).MaxLines(3)
			}
			ui.Row(c).Gap(10).Children(func() {
				if primaryActionButton(c, "连接并登录").Disabled(a.busy).Clicked() {
					a.login()
				}
				if a.loggedIn && actionButton(c, "取消").Clicked() {
					a.loginOpen = false
				}
			})
		})
	})
}

func (a *appState) selectedLibraryName() string {
	for _, library := range a.libraries {
		if library.ID == a.libraryID {
			return library.Name
		}
	}
	return "影视库"
}

func (a *appState) libraryView(c *ui.Context) {
	t := c.Theme()
	items := a.visibleItems()
	key := a.currentCatalogKey() + "\x00" + a.section
	if key != a.catalogScrollKey {
		a.catalogScrollKey = key
		a.catalogScroll = ui.ScrollState{}
		a.grid = ui.GridState{}
	}
	state := a.catalogs[a.currentCatalogKey()]
	if len(items) == 0 {
		if catalogNeedsMoreVisibleItems(len(items), state) {
			a.loadNextCatalogPage(false)
		}
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Text(c, "⌕").FontSize(34).TextColor(t.TextMuted)
			if a.searchPending && strings.TrimSpace(a.query) != "" {
				ui.Text(c, "正在搜索…").FontSize(13).TextColor(t.TextMuted)
			} else if state != nil && state.Loading && len(state.Items) > 0 {
				ui.Text(c, "正在加载媒体…").FontSize(13).TextColor(t.TextMuted)
			} else if a.status != "" {
				ui.Text(c, a.status).FontSize(13).TextColor(t.TextMuted)
			} else if state != nil && state.Loading {
				ui.Text(c, "正在读取影视库…").FontSize(13).TextColor(t.TextMuted)
			} else if state != nil && state.Err != "" {
				ui.Text(c, state.Err).FontSize(13).TextColor(ui.Hex("#ad5148")).MaxLines(3)
			} else {
				label := "这里还没有内容"
				if strings.TrimSpace(a.query) != "" {
					label = "没有找到相关内容"
				}
				ui.Text(c, label).FontSize(14).TextColor(t.TextMuted)
			}
			if actionButton(c, "重新加载").Clicked() {
				if state != nil && state.Err != "" {
					a.loadNextCatalogPage(true)
				} else if state != nil {
					state.Refreshed = false
					a.loadLibrary()
				} else {
					a.loadLibrary()
				}
			}
		})
		return
	}
	gridCount := len(items)
	if state != nil && !state.Exhausted {
		gridCount++
	}
	windowWidth, _ := c.Size()
	gridColumns := max(1, int((windowWidth-273)/152))
	grid := ui.GridView(c, &a.grid, gridCount, 150, 286, func(i int) {
		if i >= len(items) {
			if state != nil && !state.Loading && !state.Exhausted && !state.PageAutoRequested && state.Err == "" {
				a.loadNextCatalogPage(false)
			}
			ui.Box(c).Size(150, 225).Center().Children(func() {
				label := "继续加载…"
				if state != nil && state.Err != "" {
					if actionButton(c, "加载失败，点击重试").Padding(8, 10).Clicked() {
						a.loadNextCatalogPage(true)
					}
					return
				} else if state != nil && state.Loading {
					label = "正在加载…"
				}
				ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
			})
			return
		}
		item := items[i]
		if next := i + gridColumns; next < len(items) {
			a.requestPoster(items[next], 150, 225)
		}
		card := actionButton(c.Key(item.ID), "").Padding(0).BorderWidth(0).Background(ui.Color{}).TextColor(t.Text)
		card.Children(func() {
			ui.Column(c).Gap(8).Center().Children(func() {
				cover := ui.Box(c).Size(150, 225).Radius(10).Clip().BorderWidth(0)
				cover.Children(func() {
					poster := a.imageFor(item, 150, 225)
					if poster != nil {
						ui.Image(c, poster).Size(150, 225).Fit(ui.Cover)
					} else {
						ui.Box(c).Size(150, 225).Background(ui.Hex("#e8e8e2")).Center().Children(func() {
							ui.Text(c, "FILM").FontSize(10).Bold().TextColor(ui.Hex("#969d96"))
						})
					}
					if item.Rating != "" {
						ui.Text(c, item.Rating).Absolute().Top(10).Left(10).Padding(4, 8).Radius(6).
							Background(ui.Hex("#376655")).FontSize(12).Bold().TextColor(ui.Hex("#fffefa"))
					}
				})
				ui.Text(c, item.Title).FontSize(14).Bold().TextAlign(ui.Center).SingleLine()
				ui.Text(c, cardCaption(item)).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).SingleLine()
			})
		})
		if card.Clicked() {
			if item.Kind == "person" || strings.EqualFold(firstString(item.Raw, "type"), "Person") {
				a.openPerson(CastMember{
					ID:       item.ID,
					Favorite: item.Favorite,
					Name:     item.Title,
					Profile:  item.Poster,
				})
			} else {
				a.openDetail(item)
			}
		}
	}).Grow(1)
	bindSmoothScroll(grid, &a.catalogScroll)
	bindScrollEdgeFades(grid, &a.catalogScroll, t.Background)
}

func (a *appState) homeHero(c *ui.Context, item MediaItem) {
	t := c.Theme()
	ui.Row(c).Height(224).Padding(20, 22).Gap(24).Radius(14).Background(ui.Hex("#eeeee8")).Children(func() {
		if poster := a.imageFor(item, 136, 184); poster != nil {
			ui.Image(c, poster).Size(136, 184).Fit(ui.Cover).Radius(9)
		} else {
			ui.Box(c).Size(136, 184).Radius(9).Background(ui.Hex("#e5e5df")).Center().Children(func() {
				ui.Text(c, "FN").FontSize(22).Bold().TextColor(ui.Hex("#7d8b82"))
			})
		}
		ui.Column(c).Grow(1).Center().Gap(11).Children(func() {
			label := "为你精选"
			if item.Watched {
				label = "继续观看"
			}
			ui.Text(c, label).FontSize(10).Bold().LetterSpacing(1.4).TextColor(t.Accent)
			ui.Text(c, item.Title).FontSize(25).Bold().SingleLine()
			ui.Text(c, item.Subtitle()).FontSize(12).TextColor(t.TextMuted)
			if item.Overview != "" {
				ui.Text(c, item.Overview).FontSize(12).TextColor(ui.Hex("#66716b")).MaxLines(2)
			}
			ui.Row(c).Gap(9).Children(func() {
				if item.Watched {
					if primaryActionButton(c, "▶  继续播放").Clicked() {
						a.startPlayback(item)
					}
				} else if primaryActionButton(c, "▶  立即播放").Clicked() {
					a.startPlayback(item)
				}
				if actionButton(c, "查看详情").Clicked() {
					a.openDetail(item)
				}
			})
		})
	})
}

func (a *appState) playerView(c *ui.Context) {
	a.playerShortcuts(c)
	area := ui.Box(c).Fill().Background(ui.Hex("#000000"))
	area.DrawOver(func(_ *ui.Painter, rect ui.Rect) {
		a.player.SetViewport(rect)
	})
	if !a.player.Running() {
		ui.Column(c).Absolute().Fill().Center().Gap(8).Children(func() {
			if a.playback.Error != "" {
				ui.Text(c, "播放已停止").FontSize(18).Bold().TextColor(ui.Hex("#f0e8e6"))
				ui.Text(c, a.playback.Error).FontSize(11).TextColor(ui.Hex("#d6c8c5")).MaxLines(5)
				if actionButton(c, "返回影片库").Clicked() {
					a.stopPlayback()
				}
			} else {
				ui.Text(c, "正在等待播放器画面…").FontSize(13).TextColor(ui.Hex("#c9d0ca"))
			}
		})
	}
}

func (a *appState) playbackCancelButton(c *ui.Context) {
	if !a.playbackLoading {
		return
	}
	if actionButton(c, "取消").BorderWidth(0).Background(ui.Transparent).TextColor(c.Theme().TextMuted).Clicked() {
		a.cancelPlaybackLoading()
	}
}
