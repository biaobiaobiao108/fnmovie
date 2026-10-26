package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *appState) sidebar(c *ui.Context) {
	t := c.Theme()
	background := movieColors.sidebar
	top := float32(14)
	if titleBar := c.TitleBar(); titleBar.Left > 0 {
		top = titleBar.Height + 4
	}
	ui.Column(c).Width(movieSidebarWidth).FillHeight().Padding(top, 14, 14, 14).Gap(4).Background(background).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).DragWindow().Children(func() {
		a.globalSearchField(c)
		a.navButton(c, sidebarNavIcon("home"), "首页", "home")
		a.navButton(c, sidebarNavIcon("movies"), "电影", "movies")
		a.navButton(c, sidebarNavIcon("tv"), "电视节目", "tv")
		a.navButton(c, sidebarNavIcon("favorites"), "我的收藏", "favorites")
		ui.Text(c, "影视库").Padding(8, 10, 4, 10).FontSize(11).Bold().TextColor(t.TextMuted)
		if a.libraryErr != "" && !a.libraryLoading {
			ui.Column(c).Padding(0, 10, 6, 10).Gap(6).Children(func() {
				ui.Text(c, "读取影视库失败").FontSize(11).TextColor(t.Danger).MaxLines(2)
				if actionButton(c, "重试").Height(30).Padding(6, 10).Clicked() {
					a.loadLibraries()
				}
			})
		} else if a.libraryLoading && len(a.libraries) == 0 {
			ui.Row(c).Padding(0, 10, 6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, "正在读取影视库…").FontSize(11).TextColor(t.TextMuted)
			})
		}
		libraries := a.libraries
		a.sidebarList.Key = func(i int) any { return libraries[i].ID }
		list := ui.List(c, &a.sidebarList, len(libraries), func(i int) {
			library := libraries[i]
			selected := a.section == "library" && a.libraryID == library.ID
			button := actionButton(c, "").Height(36).Justify(ui.Start).Padding(9, 12).Gap(10).Radius(8).BorderWidth(0).Background(ui.Color{}).TextColor(t.TextMuted).
				Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
			if selected {
				button.Background(movieColors.selected).TextColor(t.Accent)
			} else if button.Hovered() {
				button.Background(t.SurfaceHover).TextColor(t.Text)
			}
			button.Children(func() {
				iconColor := t.TextMuted
				if selected {
					iconColor = t.Accent
				} else if button.Hovered() {
					iconColor = t.Text
				}
				if cover := a.imageForURL(library.Poster, 22, 22); cover != nil {
					ui.Image(c, cover).Size(22, 22).Fit(ui.Cover).Radius(5).Label("影视库封面：" + library.Name)
				} else {
					ui.Icon(c, libraryNavigationIcon(library)).Size(20, 20).TextColor(iconColor).Label("影视库类型：" + library.Name)
				}
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
			exitBtn := actionButton(c, "").Height(36).Justify(ui.Start).Padding(9, 12).Gap(10).Radius(8).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
			if exitBtn.Hovered() {
				exitBtn.Background(t.SurfaceHover).TextColor(t.Text)
			}
			exitBtn.Children(func() {
				ui.Icon(c, sidebarNavIcon("logout")).Size(20, 20).TextColor(t.TextMuted)
				ui.Text(c, "退出登录").FontSize(13)
			})
			if exitBtn.Clicked() {
				a.logout()
			}
		} else if !a.loggedIn {
			loginBtn := actionButton(c, "").Height(36).Justify(ui.Start).Padding(9, 12).Gap(10).Radius(8).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
			if loginBtn.Hovered() {
				loginBtn.Background(t.SurfaceHover).TextColor(t.Text)
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

func (a *appState) globalSearchField(c *ui.Context) {
	t := c.Theme()
	focused := c.Focused(a.searchBox)
	field := ui.Row(c.Key("global-search-field")).Height(32).FillWidth().PaddingX(10).Gap(7).Radius(9).AlignItems(ui.Center).Margin(0, 0, 14, 0)
	if focused {
		field.Background(t.Background).Shadow(0, 0, 0, 3, movieColors.accent.Alpha(0.25)).Border(1, movieColors.accent.Alpha(0.6))
	} else {
		field.Background(t.SurfaceHover).Border(1, t.Border)
	}
	field.Children(func() {
		ui.Icon(c, sidebarNavIcon("search")).Size(14, 14).TextColor(t.TextMuted)
		input := ui.TextInputBase(c.Key("global-search-input"), &a.query).Bind(&a.searchBox).Grow(1).
			Placeholder("搜索影片、演员或导演").FontSize(13).Label("全局搜索")
		if a.loggedIn {
			input.OnChange(func() { a.globalSearchChanged(c) })
		} else {
			input.Disabled(true)
		}
	})
}

func (a *appState) globalSearchChanged(c *ui.Context) {
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
}

func (a *appState) navButton(c *ui.Context, icon *ui.SVG, label, key string) {
	selected := a.section == key
	e := actionButton(c, "").Height(36).Justify(ui.Start).Padding(9, 12).Gap(10).Radius(8).BorderWidth(0)
	e.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	if selected {
		e.Background(movieColors.selected).TextColor(c.Theme().Accent)
	} else {
		e.Background(ui.Color{}).TextColor(c.Theme().TextMuted)
		if e.Hovered() {
			e.Background(c.Theme().SurfaceHover).TextColor(c.Theme().Text)
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
					color = t.Danger
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
	a.libraryHeading(c)
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
				ui.Text(c, state.Err).FontSize(13).TextColor(t.Danger).MaxLines(3)
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
	gridColumns := max(1, int((windowWidth-(movieSidebarWidth+movieMainHorizontalInset*2))/152))
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
						ui.Box(c).Size(150, 225).Background(movieColors.placeholder).Center().Children(func() {
							ui.Text(c, "FILM").FontSize(10).Bold().TextColor(movieColors.placeholderText)
						})
					}
					if item.Rating != "" {
						ui.Text(c, item.Rating).Absolute().Top(10).Left(10).Padding(4, 8).Radius(6).
							Background(movieColors.rating).FontSize(12).Bold().TextColor(movieColors.ratingText)
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

func (a *appState) libraryHeading(c *ui.Context) {
	label := map[string]string{
		"home": "首页", "movies": "电影", "tv": "电视节目", "favorites": "我的收藏", "library": a.selectedLibraryName(),
	}[a.section]
	if label == "" {
		label = "影视库"
	}
	if strings.TrimSpace(a.query) != "" {
		label = "全局搜索"
	}
	ui.Row(c).Height(38).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(26).Bold().Grow(1)
		if a.loggedIn && (a.busy || a.libraryLoading || a.currentCatalogLoading()) {
			ui.Spinner(c).Size(16, 16)
		}
		if state := a.catalogs[a.currentCatalogKey()]; state != nil && state.Err != "" && len(state.Items) > 0 {
			if actionButton(c, "重试").Padding(5, 9).Clicked() {
				a.loadNextCatalogPage(true)
			}
		}
	})
}

func (a *appState) homeHero(c *ui.Context, item MediaItem) {
	t := c.Theme()
	ui.Row(c).Height(224).Padding(20, 22).Gap(24).Radius(14).Background(t.Surface).Children(func() {
		if poster := a.imageFor(item, 136, 184); poster != nil {
			ui.Image(c, poster).Size(136, 184).Fit(ui.Cover).Radius(9)
		} else {
			ui.Box(c).Size(136, 184).Radius(9).Background(movieColors.placeholder).Center().Children(func() {
				ui.Text(c, "FN").FontSize(22).Bold().TextColor(movieColors.placeholderText)
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
				ui.Text(c, item.Overview).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
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
