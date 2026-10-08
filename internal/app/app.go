package app

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	overlay "github.com/egoist/mygo/fnmovieoverlay"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/icon.png
var appIconPNG []byte

type appState struct {
	home                         homeState
	window                       *mygo.Window
	settings                     Settings
	serverAddress                string
	server                       *Server
	player                       *Player
	playerProxy                  *PlaybackProxy
	section                      string
	query                        string
	username                     string
	password                     string
	status                       string
	busy                         bool
	loginCancel                  context.CancelFunc
	libraryLoading               bool
	libraryCancel                context.CancelFunc
	libraryErr                   string
	loggedIn                     bool
	loginOpen                    bool
	libraries                    []MediaLibrary
	libraryID                    string
	items                        []MediaItem
	catalogs                     map[string]*CatalogState
	catalogCache                 *CatalogCache
	mediaView                    mediaViewCache
	dataRevision                 uint64
	searchChangedAt              time.Time
	searchPending                bool
	grid                         ui.GridState
	catalogScroll                ui.ScrollState
	catalogScrollKey             string
	catalogScrollAnimation       smoothScroll
	sidebarList                  ui.ListState
	sidebarScroll                ui.ScrollState
	sidebarScrollAnimation       smoothScroll
	detailList                   ui.ListState
	detailScroll                 ui.ScrollState
	detailScrollAnimation        smoothScroll
	personScroll                 ui.ScrollState
	personScrollAnimation        smoothScroll
	posters                      *PosterLoader
	selected                     *MediaItem
	seriesLoading                bool
	seriesError                  string
	castLoading                  bool
	castError                    string
	selectedSeasonID             string
	seriesEpisodes               []MediaItem
	seriesEpisodeCache           map[string][]MediaItem
	seriesEpisodeLoading         bool
	seriesEpisodeError           string
	seriesEpisodeRequest         uint64
	seriesRootCast               []CastMember
	seriesCastRequest            uint64
	favoritePending              map[string]bool
	playback                     PlaybackState
	selectedTab                  int
	playingItem                  *MediaItem
	icon                         *ui.Bitmap
	displayScale                 float64
	playerOverlayWindow          *mygo.Window
	playerHeaderWindow           *mygo.Window
	playerMenuWindow             *mygo.Window
	playerOverlayContent         *overlay.Content
	playerHeaderContent          *overlay.Content
	playerMenuContent            *overlay.Content
	fullscreenMotion             playerFullscreenMotion
	playerOverlayHooks           bool
	playerOverlayMonitorDone     chan struct{}
	overlayMu                    sync.Mutex
	playerOverlayVisible         bool
	playerOverlayPinned          bool
	playerOverlaySeeking         bool
	playerOverlayLastInput       time.Time
	playerOverlayMenu            string
	playerMenuTracks             []PlayerTrack
	playerOverlayTransition      uint64
	playerOverlayOpacity         float64
	playerOverlayAnimationStart  time.Time
	playerOverlayAnimationFrom   float64
	playerOverlayAnimationTarget float64
	seekSliderPosition           float64
	seekDragging                 bool
	seekFeedback                 seekFeedback
	volumeDragging               bool
	playbackLoading              bool
	playbackLoadingID            string
	playbackLoadingTitle         string
	playbackCancel               context.CancelFunc
	playbackRequest              uint64
	playbackTrackCancel          context.CancelFunc
	selectedPerson               *CastMember
	personItems                  []MediaItem
	personLoading                bool
	personError                  string
	personRequest                uint64
	personGrid                   ui.GridState
}

// Run starts the native desktop application.
func Run() {
	configureCompatibleAppPaths()
	settings, err := LoadSettings()
	if err != nil {
		log.Printf("settings: %v", err)
	}
	icon, iconErr := ui.DecodeBitmap(appIconPNG)
	if iconErr != nil {
		log.Printf("app icon: %v", iconErr)
	}
	app := &appState{settings: settings, serverAddress: settings.ServerURL, section: "home", status: "连接飞牛影视服务器后开始浏览", loginOpen: true, player: NewPlayer(), posters: NewPosterLoader(), catalogs: map[string]*CatalogState{}, catalogCache: NewCatalogCache(), favoritePending: map[string]bool{}, icon: icon}
	if settings.ServerURL != "" {
		app.server = NewServer(settings.ServerURL, "")
		if settings.Username != "" {
			app.username = settings.Username
			if credentials, err := ReadCredential(settings.ServerURL); err == nil {
				app.password = credentials.Password
				if credentials.Token != "" {
					app.server.SetToken(credentials.Token)
					app.loggedIn = true
				}
			}
		}
	}

	mygo.App.WhenReady(func() {
		app.window = mygo.NewWindow(mygo.WindowOptions{
			Title: "哞哩影院", Width: 1280, Height: 800, MinWidth: 960, MinHeight: 640,
			BackgroundColor: "#f7f6f2", StateKey: "main", Content: ui.View(app.view),
		})
		setWindowTheme(app.window)
		app.displayScale = windowScale(app.window.NativeHandle())
		app.window.OnResize(app.updateDisplayScale)
		app.window.OnMove(app.updateDisplayScale)
		app.window.OnMinimize(app.pauseHomeCarousel)
		app.window.OnRestore(func() { app.syncHomeLifecycle() })
		if err := app.window.SetIcon(appIconPNG); err != nil {
			log.Printf("set app icon: %v", err)
		}
		if app.loggedIn {
			app.loginOpen = false
			app.loadLibraries()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { app.resetHome(); app.home.Closed = true; app.stopPlaybackAndWait() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *appState) view(c *ui.Context) {
	a.syncHomeLifecycle()
	c.SetTheme(movieTheme())
	a.advancePlayerFullscreen(c)
	if a.playback.Active {
		a.advancePlayerOverlayAnimation(c)
		a.playerView(c)
		return
	}
	ui.Row(c).Fill().Background(ui.Hex("#f7f6f2")).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).FillHeight().Padding(16, 34, 12, 34).Gap(18).Children(func() {
			a.topbar(c)
			ui.Column(c).Key(a.navigationKey()).Grow(1).FillHeight().Transition(pageTransition()).Children(func() {
				switch {
				case a.selectedPerson != nil:
					a.personView(c, *a.selectedPerson)
				case a.selected != nil:
					a.detailView(c, *a.selected)
				case a.section == "home":
					a.homeView(c)
				default:
					a.libraryView(c)
				}
			})
		})
	})
	a.loginModal(c)
	a.syncHomeCarousel()
}

func movieTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f7f6f2")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#f2f1ec")
	t.SurfacePressed = ui.Hex("#e8e7e1")
	t.Text = ui.Hex("#1c201d")
	t.TextMuted = ui.Hex("#747d77")
	t.Border = ui.Hex("#e4e2dc")
	t.Accent = ui.Hex("#2d5a49")
	t.AccentHover = ui.Hex("#366d58")
	t.AccentPressed = ui.Hex("#244639")
	t.AccentText = ui.Hex("#ffffff")
	t.Radius = 11
	// Keep native scrollbars hidden. The catalog draws its own very subtle
	// thumb; the library sidebar intentionally has no visible scrollbar.
	t.Scrollbar = ui.Transparent
	return t
}

func (a *appState) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(205).FillHeight().Padding(24, 16).Gap(8).Background(ui.Hex("#f4f2ed")).Border(1, t.Border).Children(func() {
		ui.Row(c).Padding(0, 10).Gap(11).Children(func() {
			if a.icon != nil {
				ui.Image(c, a.icon).Size(38, 38).Radius(10).Fit(ui.Cover)
			}
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "哞哩影院").FontSize(16).Bold()
			})
		})
		ui.Box(c).Height(15)
		a.navButton(c, "", "首页", "home")
		a.navButton(c, "▣", "电影", "movies")
		a.navButton(c, "▤", "电视节目", "tv")
		a.navButton(c, "♡", "我的收藏", "favorites")
		ui.Text(c, "影视库").Padding(6, 14).FontSize(11).Bold().TextColor(t.TextMuted)
		libraries := a.libraries
		a.sidebarList.Key = func(i int) any { return libraries[i].ID }
		advanceSmoothScroll(c, &a.sidebarScroll, &a.sidebarScrollAnimation)
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
		bindSmoothScroll(c, list, &a.sidebarScroll, &a.sidebarScrollAnimation)
		if a.loggedIn {
			exitBtn := actionButton(c, "").Justify(ui.Start).Padding(10, 14).Gap(10).Radius(9).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond})
			if exitBtn.Hovered() {
				exitBtn.Background(ui.Hex("#eeece6"))
			}
			exitBtn.Children(func() {
				ui.Text(c, "⇥").FontSize(16).Width(20).TextAlign(ui.Center)
				ui.Text(c, "退出登录").FontSize(13)
			})
			if exitBtn.Clicked() {
				a.logout()
			}
		} else if !a.loggedIn {
			loginBtn := actionButton(c, "").Justify(ui.Start).Padding(10, 14).Gap(10).Radius(9).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted)
			if loginBtn.Hovered() {
				loginBtn.Background(ui.Hex("#eeece6"))
			}
			loginBtn.Children(func() {
				ui.Text(c, "🔑").FontSize(14).Width(20).TextAlign(ui.Center)
				ui.Text(c, "登录到服务器").FontSize(13)
			})
			if loginBtn.Clicked() {
				a.loginOpen = true
			}
		}
	})
}

func (a *appState) navButton(c *ui.Context, icon, label, key string) {
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
		if key == "home" {
			ui.Icon(c, homeNavigationIcon).Size(20, 20)
		} else {
			ui.Text(c, icon).FontSize(16).Width(20).TextAlign(ui.Center)
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
			label := map[string]string{"home": "首页", "movies": "电影", "tv": "电视节目", "favorites": "我的收藏", "settings": "服务器设置", "library": a.selectedLibraryName()}[a.section]
			if label == "" {
				label = "影视库"
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
		if a.loggedIn && a.section != "settings" && (a.section != "home" || a.selected != nil || a.selectedPerson != nil) {
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
			if ui.SearchField(c, &a.query).Width(270).Placeholder("搜索影片、演员或导演").Changed() {
				a.searchChangedAt = c.Now()
				a.searchPending = true
				c.After(320 * time.Millisecond)
			}
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
			ui.TextInput(c, &a.password).Password().Placeholder("密码").Label("密码").Submitted()
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
			ui.TextInput(c, &a.password).Password().Placeholder("密码").Label("密码").Submitted()
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
	if a.section == "settings" {
		a.settingsView(c)
		return
	}
	items := a.visibleItems()
	key := a.currentCatalogKey() + "\x00" + a.section
	if key != a.catalogScrollKey {
		a.catalogScrollKey = key
		a.catalogScroll = ui.ScrollState{}
		a.catalogScrollAnimation.Stop(0)
		a.grid = ui.GridState{}
	}
	advanceSmoothScroll(c, &a.catalogScroll, &a.catalogScrollAnimation)
	state := a.catalogs[a.currentCatalogKey()]
	if len(items) == 0 {
		if catalogNeedsMoreVisibleItems(len(items), state) {
			a.loadNextCatalogPage(false)
		}
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Text(c, "⌕").FontSize(34).TextColor(t.TextMuted)
			if state != nil && state.Loading && len(state.Items) > 0 {
				ui.Text(c, "正在加载媒体…").FontSize(13).TextColor(t.TextMuted)
			} else if a.status != "" {
				ui.Text(c, a.status).FontSize(13).TextColor(t.TextMuted)
			} else if state != nil && state.Loading {
				ui.Text(c, "正在读取影视库…").FontSize(13).TextColor(t.TextMuted)
			} else if state != nil && state.Err != "" {
				ui.Text(c, state.Err).FontSize(13).TextColor(ui.Hex("#ad5148")).MaxLines(3)
			} else {
				ui.Text(c, "这里还没有内容").FontSize(14).TextColor(t.TextMuted)
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
	gridColumns := max(1, int((windowWidth-273)/170))
	grid := ui.GridView(c, &a.grid, gridCount, 168, 326, func(i int) {
		if i >= len(items) {
			if state != nil && !state.Loading && !state.Exhausted && !state.PageAutoRequested && state.Err == "" {
				a.loadNextCatalogPage(false)
			}
			ui.Box(c).Size(168, 252).Center().Children(func() {
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
			a.requestPoster(items[next], 168, 252)
		}
		card := actionButton(c, "").Key(item.ID).Padding(0).BorderWidth(0).Background(ui.Color{}).TextColor(t.Text)
		card.Children(func() {
			ui.Column(c).Gap(8).Center().Children(func() {
				cover := ui.Box(c).Size(168, 252).Radius(10).Clip().BorderWidth(0)
				cover.Children(func() {
					poster := a.imageFor(item, 168, 252)
					if poster != nil {
						ui.Image(c, poster).Size(168, 252).Fit(ui.Cover)
					} else {
						ui.Box(c).Size(168, 252).Background(ui.Hex("#e8e8e2")).Center().Children(func() {
							ui.Text(c, "FILM").FontSize(10).Bold().TextColor(ui.Hex("#969d96"))
						})
					}
					if item.Rating != "" {
						ui.Text(c, item.Rating).Absolute().Top(10).Left(10).Padding(4, 8).Radius(6).
							Background(ui.Hex("#376655")).FontSize(14).Bold().TextColor(ui.Hex("#fffefa"))
					}
				})
				ui.Text(c, item.Title).FontSize(16).Bold().TextAlign(ui.Center).SingleLine()
				ui.Text(c, cardCaption(item)).FontSize(14).TextColor(t.TextMuted).TextAlign(ui.Center).SingleLine()
			})
		})
		if card.Clicked() {
			if item.Kind == "person" || strings.EqualFold(firstString(item.Raw, "type"), "Person") {
				a.openPerson(CastMember{
					ID:      item.ID,
					Name:    item.Title,
					Profile: item.Poster,
				})
			} else {
				a.openDetail(item)
			}
		}
	}).Grow(1)
	bindSmoothScroll(c, grid, &a.catalogScroll, &a.catalogScrollAnimation)
	background := t.Background
	grid.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		const fadeHeight = 28
		if a.catalogScroll.Y > 1 {
			p.FillGradient(ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: fadeHeight}, ui.LinearGradient{
				From: background, To: background.Alpha(0), Angle: 180,
			}, 0)
		}
		p.FillGradient(ui.Rect{X: rect.X, Y: rect.Y + rect.H - fadeHeight, W: rect.W, H: fadeHeight}, ui.LinearGradient{
			From: background.Alpha(0), To: background, Angle: 180,
		}, 0)
		if a.catalogScroll.MaxY > 0 && rect.H > 40 {
			track := ui.Rect{X: rect.X + rect.W - 4, Y: rect.Y + 12, W: 3, H: rect.H - 24}
			thumbHeight := max(float32(26), track.H*track.H/(track.H+a.catalogScroll.MaxY))
			thumbY := track.Y + (track.H-thumbHeight)*float32(a.catalogScroll.Y/max(float32(1), a.catalogScroll.MaxY))
			p.Fill(ui.Rect{X: track.X, Y: thumbY, W: track.W, H: min(thumbHeight, track.H)}, ui.RGBA(92, 104, 96, 0.24), 1.5)
		}
	})
}

func cardCaption(item MediaItem) string {
	if item.Kind == "person" {
		return "影人 / 演员"
	}
	if item.Kind != "tv" {
		return item.Year
	}
	if count := firstString(item.Raw, "season_count", "seasonCount", "season_num"); count != "" && count != "0" {
		return "共 " + count + " 季 · " + item.Year
	}
	if count := firstString(item.Raw, "episode_count", "episodeCount", "episodes"); count != "" && count != "0" {
		return "共 " + count + " 集 · " + item.Year
	}
	return item.Year
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

func (a *appState) openDetail(item MediaItem) {
	a.catalogScrollAnimation.Stop(a.catalogScroll.Y)
	a.personScrollAnimation.Stop(a.personScroll.Y)
	a.closePerson()
	serverURL := a.settings.ServerURL
	username := a.settings.Username
	cachedDetail := false
	if a.server != nil && a.catalogCache != nil {
		if cached, ok := a.catalogCache.Detail(serverURL, item.ID, username); ok {
			item = cached
			cachedDetail = true
		}
	}
	a.selected = &item
	a.detailList = ui.ListState{}
	a.detailScroll = ui.ScrollState{}
	a.detailScrollAnimation.Stop(0)
	a.seriesLoading, a.seriesError, a.selectedSeasonID = false, "", ""
	a.castLoading, a.castError = true, ""
	a.seriesEpisodes, a.seriesEpisodeCache = nil, nil
	a.seriesEpisodeLoading, a.seriesEpisodeError = false, ""
	a.seriesEpisodeRequest++
	a.seriesRootCast = nil
	a.seriesCastRequest++
	if item.IsSeries && cachedDetail && len(item.Seasons) > 0 {
		a.selectedSeasonID = item.Seasons[0].ID
		a.loadSeriesSeason(a.selectedSeasonID, false)
	}
	if a.server == nil {
		a.castLoading = false
		return
	}
	server := a.server
	cache := a.catalogCache
	if item.IsSeries {
		a.seriesLoading = true
		go func() {
			detail, detailErr := server.Detail(item)
			seasons, seasonErr := server.SeriesSeasons(item)
			people, peopleErr := server.People(item.ID)
			if detailErr != nil {
				detail = item
			}
			if peopleErr == nil {
				detail.Cast = people
			} else {
				detail.Cast = item.Cast
			}
			if detailErr == nil {
				detail.Seasons = seasons
				cache.SetDetail(serverURL, detail, username)
			}
			a.window.Update(func() {
				if a.server != server || a.selected == nil || a.selected.ID != item.ID {
					return
				}
				a.seriesLoading = false
				a.seriesRootCast = append([]CastMember(nil), people...)
				a.castLoading = len(people) == 0
				updated := detail
				updated.Seasons = seasons
				a.selected = &updated
				if seasonErr != nil {
					a.seriesError = "季度列表读取失败：" + seasonErr.Error()
				}
				if peopleErr != nil {
					a.castError = "演职员读取失败：" + peopleErr.Error()
				}
				if detailErr != nil {
					a.status = "详情读取失败：" + detailErr.Error()
				}
				if len(seasons) > 0 && a.selectedSeasonID == "" {
					a.selectSeriesSeason(seasons[0].ID)
				} else if a.selectedSeasonID != "" {
					a.loadSeriesCast(a.selectedSeasonID, a.seriesEpisodes)
				} else {
					a.castLoading = false
				}
			})
		}()
		return
	}
	go func() {
		detail, err := server.Detail(item)
		people, peopleErr := server.People(item.ID)
		if err == nil {
			if peopleErr == nil {
				detail.Cast = people
			} else {
				detail.Cast = item.Cast
			}
			cache.SetDetail(serverURL, detail, username)
		}
		a.window.Update(func() {
			if a.server != server || a.selected == nil || a.selected.ID != item.ID {
				return
			}
			a.castLoading = false
			if err != nil {
				a.status = "详情读取失败：" + err.Error()
				if peopleErr == nil && len(people) > 0 {
					item.Cast = people
					a.selected = &item
				}
			} else {
				a.selected = &detail
			}
			if peopleErr != nil {
				a.castError = "演职员读取失败：" + peopleErr.Error()
			}
		})
	}()
}

func (a *appState) detailView(c *ui.Context, item MediaItem) {
	if item.IsSeries {
		a.seriesDetailView(c, item)
		return
	}
	t := c.Theme()
	posterWidth, posterHeight := detailPosterSize(c)
	ui.Column(c).Grow(1).FillHeight().Gap(8).Children(func() {
		a.detailBackButton(c)
		ui.Row(c).Grow(1).FillHeight().AlignItems(ui.Start).Gap(30).Children(func() {
			if poster := a.imageFor(item, posterWidth, posterHeight); poster != nil {
				ui.Image(c, poster).Size(float32(posterWidth), float32(posterHeight)).Fit(ui.Cover).Radius(13)
			} else {
				ui.Box(c).Size(float32(posterWidth), float32(posterHeight)).Radius(13).Background(ui.Hex("#e8e8e2"))
			}
			advanceSmoothScroll(c, &a.detailScroll, &a.detailScrollAnimation)
			scroll := ui.Scroll(c).Key("movie-detail:" + item.ID).Grow(1).AlignSelf(ui.Stretch).FillHeight().Children(func() {
				ui.Column(c).Padding(8, 0).Gap(17).Children(func() {
					ui.Text(c, item.Title).FontSize(34).Bold()
					ui.Text(c, item.Subtitle()).FontSize(15).TextColor(t.TextMuted)
					if item.Overview != "" {
						ui.Text(c, item.Overview).FontSize(16).TextColor(ui.Hex("#66716b"))
					}
					ui.Row(c).Gap(13).AlignItems(ui.Center).Children(func() {
						if a.playbackLoading && a.playbackLoadingID == item.ID {
							btn := actionButton(c, "").Padding(9, 16).Radius(8).BorderWidth(0).
								Background(t.Accent).TextColor(t.AccentText).Disabled(true)
							btn.Children(func() {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
									ui.Spinner(c).FontSize(13)
									ui.Text(c, "正在准备播放…").FontSize(13).Bold()
								})
							})
						} else {
							if primaryActionButton(c, "▶  立即播放").Clicked() {
								a.startPlayback(item)
							}
						}
						a.playbackCancelButton(c)
						a.favoriteButton(c, item)
					})
					a.castSection(c, item)
					if len(item.Sources) > 0 {
						ui.Text(c, "可播放版本").FontSize(15).Bold().TextColor(t.TextMuted)
						for _, source := range item.Sources {
							ui.Row(c).Gap(10).Padding(10, 12).Radius(8).Background(t.Surface).Children(func() {
								ui.Text(c, source.Name).Grow(1)
								ui.Text(c, source.Quality).TextColor(t.TextMuted)
								if a.playbackLoading && a.playbackLoadingID == item.ID {
									ui.Spinner(c).FontSize(12)
								} else if actionButton(c, "播放").Clicked() {
									a.startPlayback(item)
								}
							})
						}
					}
				})
			})
			bindSmoothScroll(c, scroll, &a.detailScroll, &a.detailScrollAnimation)
		})
	})
}

func (a *appState) seriesDetailView(c *ui.Context, item MediaItem) {
	t := c.Theme()
	posterWidth, posterHeight := detailPosterSize(c)
	ui.Column(c).Grow(1).FillHeight().Gap(8).Children(func() {
		a.detailBackButton(c)
		ui.Row(c).Grow(1).FillHeight().AlignItems(ui.Start).Gap(30).Children(func() {
			if poster := a.imageFor(item, posterWidth, posterHeight); poster != nil {
				ui.Image(c, poster).Size(float32(posterWidth), float32(posterHeight)).Fit(ui.Cover).Radius(13)
			} else {
				ui.Box(c).Size(float32(posterWidth), float32(posterHeight)).Radius(13).Background(ui.Hex("#e8e8e2"))
			}
			episodes := a.seriesEpisodes
			showEpisodes := !a.seriesEpisodeLoading && a.seriesEpisodeError == "" && len(episodes) > 0
			rows := 4
			if showEpisodes {
				rows = 3 + len(episodes)
			}
			a.detailList.Key = func(i int) any {
				if i < 3 {
					return []string{"overview", "cast", "seasons"}[i]
				}
				if showEpisodes {
					return "episode:" + episodes[i-3].ID
				}
				return "episode-status"
			}
			advanceSmoothScroll(c, &a.detailScroll, &a.detailScrollAnimation)
			list := ui.List(c, &a.detailList, rows, func(i int) {
				switch i {
				case 0:
					ui.Column(c).Gap(14).Padding(0, 0, 12, 0).Children(func() {
						ui.Text(c, item.Title).FontSize(28).Bold()
						ui.Text(c, item.Subtitle()).FontSize(15).TextColor(t.TextMuted)
						if item.Overview != "" {
							ui.Text(c, item.Overview).FontSize(15).TextColor(t.TextMuted).MaxLines(5)
						}
						ui.Row(c).Gap(13).AlignItems(ui.Center).Children(func() {
							if len(a.seriesEpisodes) > 0 {
								firstEp := a.seriesEpisodes[0]
								if a.playbackLoading && a.playbackLoadingID == firstEp.ID {
									btn := actionButton(c, "").Padding(9, 16).Radius(8).BorderWidth(0).
										Background(t.Accent).TextColor(t.AccentText).Disabled(true)
									btn.Children(func() {
										ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
											ui.Spinner(c).FontSize(13)
											ui.Text(c, "正在准备首集…").FontSize(13).Bold()
										})
									})
								} else if primaryActionButton(c, "▶  播放本季首集").Clicked() {
									a.startPlayback(firstEp)
								}
							}
							a.playbackCancelButton(c)
							a.favoriteButton(c, item)
						})
					})
				case 1:
					ui.Column(c).Padding(0, 0, 12, 0).Children(func() { a.castSection(c, item) })
				case 2:
					ui.Column(c).Gap(14).Padding(0, 0, 12, 0).Children(func() {
						if a.seriesLoading {
							ui.Text(c, "正在读取季度与剧集…").FontSize(12).TextColor(t.TextMuted)
						} else if a.seriesError != "" {
							ui.Row(c).Gap(8).Children(func() {
								ui.Text(c, a.seriesError).FontSize(11).TextColor(ui.Hex("#ad5148"))
								if actionButton(c, "重试").Clicked() {
									a.openDetail(item)
								}
							})
						}
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "剧集").FontSize(14).Bold()
							if !a.seriesEpisodeLoading && len(a.seriesEpisodes) > 0 {
								ui.Text(c, fmt.Sprintf("%d 集", len(a.seriesEpisodes))).FontSize(11).TextColor(t.TextMuted)
							}
						})
						ui.ScrollHorizontal(c).Height(42).Gap(8).Children(func() {
							for _, season := range item.Seasons {
								label := season.Title
								if strings.TrimSpace(label) == "" {
									label = fmt.Sprintf("第%d季", season.Number)
								}
								button := actionButton(c, label).Padding(7, 11).BorderWidth(0).Radius(8)
								if a.selectedSeasonID == season.ID {
									button.Background(t.Accent).TextColor(t.AccentText)
								} else {
									button.Background(ui.Hex("#eeece6")).TextColor(t.TextMuted)
								}
								if button.Clicked() {
									a.selectSeriesSeason(season.ID)
								}
							}
						})
					})
				default:
					if !showEpisodes {
						ui.Column(c).Children(func() {
							if a.seriesEpisodeLoading {
								ui.Text(c, "正在读取本季集数…").Padding(12, 10).FontSize(12).TextColor(t.TextMuted)
								return
							}
							if a.seriesEpisodeError != "" {
								ui.Row(c).Padding(12, 10).Gap(8).Children(func() {
									ui.Text(c, a.seriesEpisodeError).Grow(1).FontSize(11).TextColor(ui.Hex("#ad5148"))
									if actionButton(c, "重试").Clicked() {
										a.loadSeriesSeason(a.selectedSeasonID, true)
									}
								})
								return
							}
							if len(a.seriesEpisodes) == 0 {
								ui.Text(c, "这个季度还没有集数").Padding(12, 10).FontSize(12).TextColor(t.TextMuted)
								return
							}
						})
						return
					}
					episode := episodes[i-3]
					row := actionButton(c, "").Key(episode.ID).FillWidth().Padding(10, 8).Gap(12).AlignItems(ui.Center).
						BorderWidth(0).Radius(8).Background(ui.Transparent).TextColor(t.Text).Label("播放 " + episode.Title).Disabled(a.playbackLoading)
					if row.Hovered() {
						row.Background(ui.Hex("#eeece6"))
					}
					row.Children(func() {
						number := ""
						if episode.EpisodeNumber > 0 {
							number = fmt.Sprintf("%02d", episode.EpisodeNumber)
						}
						ui.Text(c, number).Width(30).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, episode.Title).FontSize(13).Bold().SingleLine()
							if overview := episodeOverview(episode); overview != "" {
								ui.Text(c, overview).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
							}
						})
						if a.playbackLoading && a.playbackLoadingID == episode.ID {
							ui.Spinner(c).FontSize(12)
						} else {
							ui.Icon(c, playerOverlayIcons["play"]).Size(18, 18).TextColor(t.Accent)
						}
					})
					if row.Clicked() {
						a.startPlayback(episode)
					}
				}
			}).Grow(1).AlignSelf(ui.Stretch).FillHeight().Gap(2).Padding(8, 0)
			bindSmoothScroll(c, list, &a.detailScroll, &a.detailScrollAnimation)
		})
	})
}

func detailPosterSize(c *ui.Context) (int, int) {
	width, height := c.Size()
	posterWidth := max(270, min(360, int(float64(width)*0.23)))
	posterHeight := max(390, min(540, int(float64(height)*0.70)))
	return posterWidth, posterHeight
}

func episodeOverview(episode MediaItem) string {
	if episode.Overview != "" {
		return episode.Overview
	}
	return firstString(episode.Raw, "overview", "description", "summary")
}

func (a *appState) detailBackButton(c *ui.Context) {
	button := actionButton(c, "").Size(36, 34).Padding(0).Radius(9).BorderWidth(0).
		Background(ui.Color{}).TextColor(c.Theme().TextMuted).Label("返回影视库")
	if button.Hovered() {
		button.Background(ui.Hex("#e9ece6"))
	}
	button.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18) })
	if button.Clicked() {
		if a.playbackLoading {
			a.cancelPlaybackLoading()
		}
		a.closeDetail()
		a.castLoading = false
		a.seriesLoading = false
	}
}

func (a *appState) favoriteButton(c *ui.Context, item MediaItem) {
	pending := a.favoritePending[item.ID]
	icon := "♡"
	color := ui.Hex("#59645e")
	if item.Favorite {
		icon, color = "♥", ui.Hex("#df5b79")
	}
	button := actionButton(c, "").Size(42, 42).Padding(0).Radius(21).BorderWidth(0).
		Background(ui.Color{}).TextColor(color).Label(map[bool]string{true: "取消收藏", false: "收藏"}[item.Favorite]).Disabled(pending)
	button.Children(func() { ui.Text(c, icon).FontSize(25).TextColor(color) })
	if button.Clicked() {
		a.toggleFavorite(item)
	}
}

func (a *appState) selectSeriesSeason(seasonID string) {
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
	a.seriesEpisodeRequest++
	requestID := a.seriesEpisodeRequest
	selectedID := ""
	if a.selected != nil {
		selectedID = a.selected.ID
	}
	a.seriesEpisodeLoading = true
	a.seriesEpisodeError = ""
	go func() {
		items, err := server.SeasonEpisodes(seasonID)
		a.window.Update(func() {
			if a.server != server || a.selected == nil || a.selected.ID != selectedID || a.selectedSeasonID != seasonID || requestID != a.seriesEpisodeRequest {
				return
			}
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
	go func() {
		people, err := server.People(seasonID)
		if len(people) == 0 && episodeID != "" {
			people, err = server.People(episodeID)
		}
		a.window.Update(func() {
			if a.server != server || a.selected == nil || a.selected.ID != selectedID || a.selectedSeasonID != seasonID || a.seriesCastRequest != requestID {
				return
			}
			a.castLoading = false
			if len(people) > 0 {
				a.selected.Cast = people
				a.castError = ""
			} else if err != nil {
				a.castError = "演职人员读取失败：" + err.Error()
			} else {
				a.castError = ""
			}
		})
	}()
}

func (a *appState) settingsView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(620).Gap(14).Children(func() {
		ui.Text(c, "服务器连接").FontSize(18).Bold()
		ui.Text(c, a.settings.ServerURL).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).Children(func() {
			if primaryActionButton(c, "切换账户或服务器").Clicked() {
				a.loginOpen = true
			}
			if actionButton(c, "退出登录").Clicked() {
				a.logout()
			}
		})
		ui.Text(c, "账户凭据由 Windows 凭据管理器保护；服务器地址与界面偏好保存在本机。").FontSize(11).TextColor(t.TextMuted).MaxLines(2)
		if a.status != "" {
			ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

func (a *appState) logout() {
	a.resetHome()
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

func (a *appState) visibleItems() []MediaItem {
	source := a.items
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

func (a *appState) imageFor(item MediaItem, width, height int) *ui.Bitmap {
	server := a.server
	if server == nil || a.posters == nil {
		return nil
	}
	urlValue := item.Poster
	if urlValue == "" {
		urlValue = firstString(item.Raw, "poster_path", "posterPath", "image_url", "imageUrl", "cover_url", "coverUrl")
	}
	if urlValue == "" {
		return nil
	}
	return a.requestPoster(item, width, height)
}

func (a *appState) requestPoster(item MediaItem, width, height int) *ui.Bitmap {
	server := a.server
	if server == nil || a.posters == nil {
		return nil
	}
	urlValue := item.Poster
	if urlValue == "" {
		urlValue = firstString(item.Raw, "poster_path", "posterPath", "image_url", "imageUrl", "cover_url", "coverUrl")
	}
	if urlValue == "" {
		return nil
	}
	scale := a.displayScale
	if scale <= 0 {
		scale = 1
		if a.window != nil {
			scale = windowScale(a.window.NativeHandle())
		}
	}
	pixelWidth := max(1, int(math.Ceil(float64(width)*scale)))
	pixelHeight := max(1, int(math.Ceil(float64(height)*scale)))
	window := a.window
	return a.posters.GetOrRequest(server, server.imageURL(urlValue), pixelWidth, pixelHeight, func() {
		if window != nil {
			window.Update(func() { window.Invalidate() })
		}
	})
}

func (a *appState) castSection(c *ui.Context, item MediaItem) {
	t := c.Theme()
	ui.Column(c).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "演职人员").FontSize(16).Bold()
			if a.castLoading {
				ui.Text(c, "正在读取…").FontSize(10).TextColor(t.TextMuted)
			}
		})
		if len(item.Cast) == 0 {
			if a.castError != "" {
				ui.Row(c).Gap(10).Children(func() {
					ui.Text(c, a.castError).FontSize(11).TextColor(t.TextMuted).MaxLines(2)
					if actionButton(c, "重试").Clicked() {
						a.openDetail(item)
					}
				})
			} else if !a.castLoading {
				ui.Text(c, "服务器暂未提供演职人员信息").FontSize(11).TextColor(t.TextMuted)
			}
			return
		}
		ui.ScrollHorizontal(c).Height(164).Gap(16).Children(func() {
			for _, person := range item.Cast {
				p := person
				btn := actionButton(c, "").Width(120).Padding(8, 8).Radius(12).BorderWidth(0).
					Background(ui.Color{}).Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond}).
					Label("查看 " + p.Name + " 的作品")
				if btn.Hovered() {
					btn.Background(ui.Hex("#eeece6"))
				}
				btn.Children(func() {
					ui.Column(c).AlignItems(ui.Center).Gap(8).Children(func() {
						if image := a.imageForURL(p.Profile, 84, 84); image != nil {
							ui.Image(c, image).Size(84, 84).Fit(ui.Cover).Radius(42)
						} else {
							ui.Box(c).Size(84, 84).Radius(42).Background(ui.Hex("#e5e3dc")).Center().Children(func() {
								nameRune := "•"
								if runes := []rune(p.Name); len(runes) > 0 {
									nameRune = string(runes[:1])
								}
								ui.Text(c, nameRune).FontSize(26).Bold().TextColor(t.TextMuted)
							})
						}
						ui.Text(c, p.Name).FontSize(15).Bold().TextAlign(ui.Center).SingleLine()
						role := p.Role
						if role == "" {
							role = p.Job
						}
						ui.Text(c, role).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).SingleLine()
					})
				})
				if btn.Clicked() {
					a.openPerson(p)
				}
			}
		})
	})
}

func (a *appState) openPerson(person CastMember) {
	a.catalogScrollAnimation.Stop(a.catalogScroll.Y)
	a.detailScrollAnimation.Stop(a.detailScroll.Y)
	a.selectedPerson = &person
	a.personItems = nil
	a.personLoading = true
	a.personError = ""
	a.personGrid = ui.GridState{}
	a.personScroll = ui.ScrollState{}
	a.personScrollAnimation.Stop(0)
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
	go func() {
		items, err := server.PersonItems(personID)
		a.window.Update(func() {
			if a.server != server || a.selectedPerson == nil || a.selectedPerson.ID != personID || a.personRequest != reqID {
				return
			}
			a.personLoading = false
			if err != nil {
				a.personError = "作品读取失败：" + err.Error()
				return
			}
			a.personItems = items
		})
	}()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) closePerson() {
	a.personRequest++
	a.selectedPerson = nil
	a.personItems = nil
	a.personLoading = false
	a.personError = ""
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) personView(c *ui.Context, person CastMember) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Gap(16).Children(func() {
		// 顶部导航栏
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			backBtn := actionButton(c, "").Size(36, 34).Padding(0).Radius(9).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Label("返回")
			if backBtn.Hovered() {
				backBtn.Background(ui.Hex("#e9ece6"))
			}
			backBtn.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18) })
			if backBtn.Clicked() {
				a.closePerson()
			}
			ui.Text(c, "演职人员").FontSize(13).TextColor(t.TextMuted)
			ui.Text(c, "/").FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, person.Name).FontSize(13).Bold()
		})

		// Compact identity on the page background; no stretched card backplate.
		ui.Row(c).Gap(20).AlignItems(ui.Center).Padding(8, 0, 16, 0).Children(func() {
			if image := a.imageForURL(person.Profile, 84, 84); image != nil {
				ui.Image(c, image).Size(84, 84).Fit(ui.Cover).Radius(42)
			} else {
				ui.Box(c).Size(84, 84).Radius(42).Background(ui.Hex("#e5e3dc")).Center().Children(func() {
					nameRune := "人"
					if runes := []rune(person.Name); len(runes) > 0 {
						nameRune = string(runes[:1])
					}
					ui.Text(c, nameRune).FontSize(24).Bold().TextColor(t.TextMuted)
				})
			}
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, person.Name).FontSize(22).Bold()
				roleDesc := person.Role
				if roleDesc != "" {
					roleDesc = "代表角色：" + roleDesc
				} else if person.Job != "" {
					roleDesc = person.Job
				}
				if roleDesc != "" {
					ui.Text(c, roleDesc).FontSize(13).TextColor(t.TextMuted)
				}
				countText := ""
				if len(a.personItems) > 0 {
					countText = fmt.Sprintf("影视库中共有 %d 部参演与相关作品", len(a.personItems))
				} else if a.personLoading {
					countText = "正在读取作品列表…"
				}
				if countText != "" {
					ui.Text(c, countText).FontSize(11).TextColor(t.Accent)
				}
			})
		})

		// 作品网格展示
		ui.Column(c).Grow(1).FillHeight().Gap(10).Children(func() {
			ui.Text(c, "参演与相关作品").FontSize(15).Bold()
			if a.personLoading {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(24, 0).Children(func() {
					ui.Spinner(c).FontSize(16)
					ui.Text(c, "正在读取作品列表…").FontSize(13).TextColor(t.TextMuted)
				})
			} else if a.personError != "" {
				ui.Row(c).Gap(10).Padding(16, 0).Children(func() {
					ui.Text(c, a.personError).FontSize(12).TextColor(ui.Hex("#ad5148"))
					if actionButton(c, "重试").Clicked() {
						a.openPerson(person)
					}
				})
			} else if len(a.personItems) == 0 {
				ui.Text(c, "在当前影视库中没有找到该演职人员的作品。").FontSize(13).TextColor(t.TextMuted).Padding(24, 0)
			} else {
				items := a.personItems
				advanceSmoothScroll(c, &a.personScroll, &a.personScrollAnimation)
				grid := ui.GridView(c, &a.personGrid, len(items), 168, 326, func(i int) {
					if i >= len(items) {
						return
					}
					item := items[i]
					card := actionButton(c, "").Key("person-" + item.ID).Padding(0).BorderWidth(0).
						Background(ui.Color{}).TextColor(t.Text)
					card.Children(func() {
						ui.Column(c).Gap(8).Center().Children(func() {
							cover := ui.Box(c).Size(168, 252).Radius(10).Clip().BorderWidth(0)
							cover.Children(func() {
								poster := a.imageFor(item, 168, 252)
								if poster != nil {
									ui.Image(c, poster).Size(168, 252).Fit(ui.Cover)
								} else {
									ui.Box(c).Size(168, 252).Background(ui.Hex("#e8e8e2")).Center().Children(func() {
										ui.Text(c, "FILM").FontSize(10).Bold().TextColor(ui.Hex("#969d96"))
									})
								}
								if item.Rating != "" {
									ui.Text(c, item.Rating).Absolute().Top(10).Left(10).Padding(3, 7).Radius(6).
										Background(t.Accent).FontSize(12).Bold().TextColor(t.AccentText)
								}
							})
							ui.Text(c, item.Title).FontSize(14).Bold().TextAlign(ui.Center).SingleLine()
							subtitle := item.Subtitle()
							if subtitle == "" {
								subtitle = item.Year
							}
							if subtitle != "" {
								ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted).SingleLine()
							}
						})
					})
					if card.Clicked() {
						a.closePerson()
						a.openDetail(item)
					}
				}).Grow(1)
				bindSmoothScroll(c, grid, &a.personScroll, &a.personScrollAnimation)
			}
		})
	})
}

func (a *appState) imageForURL(remoteURL string, width, height int) *ui.Bitmap {
	if a.server == nil || a.posters == nil || strings.TrimSpace(remoteURL) == "" {
		return nil
	}
	scale := a.displayScale
	if scale <= 0 {
		scale = 1
		if a.window != nil {
			scale = windowScale(a.window.NativeHandle())
		}
	}
	w, h := max(1, int(math.Ceil(float64(width)*scale))), max(1, int(math.Ceil(float64(height)*scale)))
	window := a.window
	return a.posters.GetOrRequest(a.server, a.server.imageURL(remoteURL), w, h, func() {
		if window != nil {
			window.Update(func() { window.Invalidate() })
		}
	})
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
			a.libraries = libraries
			a.home.LibrariesReady = true
			a.loadHomeHeroes(false)
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
			a.loadLibrary()
		})
	}()
}

func (a *appState) loadLibrary() {
	if a.server == nil {
		return
	}
	if a.section == "home" {
		a.loadHomeHeroes(false)
		a.loadContinueWatching(false)
		return
	}
	if a.section == "library" && strings.TrimSpace(a.libraryID) == "" {
		a.items = nil
		a.status = "无法读取所选影视库：服务器没有返回有效的影视库标识"
		a.window.Invalidate()
		return
	}
	server, libraryID, query := a.server, a.libraryID, strings.TrimSpace(a.query)
	mediaType := a.currentMediaType()
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

func (a *appState) currentCatalogKey() string {
	return catalogStateKey(a.libraryID, a.currentMediaType(), strings.TrimSpace(a.query))
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

func catalogMediaType(section string) string {
	switch section {
	case "movies":
		return "movie"
	case "tv":
		return "tv"
	case "favorites":
		return "favorite"
	default:
		return ""
	}
}

func catalogStateKey(libraryID, mediaType, query string) string {
	if mediaType == "" {
		return libraryID + "\x00" + query
	}
	return libraryID + "\x01" + mediaType + "\x00" + query
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
		items, total, err := server.MediaPageContext(ctx, libraryID, mediaType, query, page, catalogPageSize)
		if err != nil && isAuthError(err) {
			if credentials, readErr := ReadCredential(serverURL); readErr == nil && credentials.Username == username && credentials.Password != "" {
				if result, loginErr := server.LoginContext(ctx, credentials.Username, credentials.Password); loginErr == nil {
					credentials.Token = result.Token
					refreshedCredential = &credentials
					items, total, err = server.MediaPageContext(ctx, libraryID, mediaType, query, page, catalogPageSize)
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

func appendUniqueItems(existing, incoming []MediaItem) []MediaItem {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, item := range existing {
		seen[item.ID] = struct{}{}
	}
	for _, item := range incoming {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		existing = append(existing, item)
	}
	return existing
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
	a.fetchCatalogPage(ctx, key, a.server, a.settings.ServerURL, a.libraryID, a.currentMediaType(), strings.TrimSpace(a.query), state.NextPage, false)
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sign") || strings.Contains(message, "401") || strings.Contains(message, "unauthor")
}

func (a *appState) startPlayback(item MediaItem) {
	if a.server == nil {
		return
	}
	if a.playback.Active {
		a.stopPlayback()
	}
	if a.playbackCancel != nil {
		a.playbackCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.playbackRequest++
	requestID := a.playbackRequest
	a.playbackCancel = cancel
	a.playbackLoading = true
	a.playbackLoadingID = item.ID
	a.playbackLoadingTitle = item.Title
	if a.window != nil {
		a.window.Invalidate()
	}
	server := a.server
	nativeHandle := uintptr(0)
	if a.window != nil {
		nativeHandle = a.window.NativeHandle()
	}
	requestPlayer := NewPlayer()
	if err := requestPlayer.PrepareSurface(nativeHandle); err != nil {
		a.cancelPlaybackLoading()
		a.status = "播放器绘制区域创建失败：" + err.Error()
		return
	}
	cleanup := func() {
		requestPlayer.Stop()
		a.window.Update(func() { requestPlayer.ReleaseSurface() })
	}
	go func() {
		stream, err := server.Playback(item)
		if ctx.Err() != nil {
			cleanup()
			return
		}
		if err != nil {
			cleanup()
			a.window.Update(func() {
				if ctx.Err() != nil {
					return
				}
				a.playbackLoading = false
				a.playbackLoadingID = ""
				a.playbackLoadingTitle = ""
				a.playbackCancel = nil
				a.status = "无法获取播放源：" + err.Error()
			})
			return
		}
		item.MediaID = stream.MediaID
		localURL, proxy, err := server.ProxyPlayback(stream.URL)
		if ctx.Err() != nil {
			if proxy != nil {
				proxy.Close()
			}
			cleanup()
			return
		}
		if err != nil {
			cleanup()
			a.window.Update(func() {
				if ctx.Err() != nil {
					return
				}
				a.playbackLoading = false
				a.playbackLoadingID = ""
				a.playbackLoadingTitle = ""
				a.playbackCancel = nil
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放代理启动失败：" + err.Error()
			})
			return
		}

		// 在后台 goroutine 中等待媒体载入，绝不在 UI 线程中阻塞！
		startErr := requestPlayer.StartContext(ctx, localURL, nativeHandle, stream.Duration, stream.ResumeAt)
		if ctx.Err() != nil {
			proxy.Close()
			cleanup()
			return
		}

		a.window.Update(func() {
			if ctx.Err() != nil || a.server != server || a.playbackRequest != requestID {
				proxy.Close()
				requestPlayer.Stop()
				requestPlayer.ReleaseSurface()
				return
			}
			if startErr != nil {
				proxy.Close()
				requestPlayer.Stop()
				requestPlayer.ReleaseSurface()
				a.playerProxy = nil
				a.playbackLoading = false
				a.playbackLoadingID = ""
				a.playbackLoadingTitle = ""
				a.playbackCancel = nil
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放器启动失败：" + startErr.Error()
				return
			}
			a.player = requestPlayer
			a.posters.SetPlayback(true)
			requestPlayer.ShowSurface()
			a.playerProxy = proxy
			a.playingItem = &item
			a.playback = PlaybackState{Active: true, Title: item.Title, URL: stream.URL, Position: stream.ResumeAt, Duration: stream.Duration, Volume: 100, Speed: 1, Quality: stream.Quality}
			a.playbackLoading = false
			a.playbackLoadingID = ""
			a.playbackLoadingTitle = ""
			a.playbackCancel = nil
			a.seekSliderPosition = stream.ResumeAt
			a.seekFeedback = seekFeedback{}
			a.window.Invalidate()
			a.createPlayerOverlay()
			a.startPlayerOverlayMonitor()
			trackCtx, trackCancel := context.WithCancel(context.Background())
			a.playbackTrackCancel = trackCancel
			go a.trackPlayback(trackCtx, item, server, requestPlayer)
		})
	}()
}

func (a *appState) cancelPlaybackLoading() {
	a.playbackRequest++
	if a.playbackCancel != nil {
		a.playbackCancel()
		a.playbackCancel = nil
	}
	a.playbackLoading = false
	a.playbackLoadingID = ""
	a.playbackLoadingTitle = ""
	if a.window != nil {
		a.window.Invalidate()
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

func (a *appState) trackPlayback(ctx context.Context, item MediaItem, server *Server, player *Player) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastProgress := time.Now()
	for player.Running() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !player.Running() {
			break
		}
		snapshot := player.Snapshot()
		position, duration := snapshot.Position, snapshot.Duration
		a.window.Update(func() {
			if ctx.Err() != nil || a.player != player || !a.playback.Active || a.playingItem == nil || a.playingItem.ID != item.ID {
				return
			}
			a.playback.Position = a.seekFeedback.position(snapshot, a.playback.Position)
			a.playback.Paused = snapshot.Paused
			if !a.volumeDragging {
				a.playback.Volume = snapshot.Volume
			}
			a.playback.Muted = snapshot.Muted
			if snapshot.Speed > 0 {
				a.playback.Speed = snapshot.Speed
			}
			a.playback.AudioOutput = snapshot.AudioOutput
			a.playback.AudioParams = snapshot.AudioParams
			a.playback.AudioTracks = snapshot.AudioTracks
			a.playback.SubtitleTracks = snapshot.SubtitleTracks
			if duration > 0 {
				a.playback.Duration = duration
			}
			if a.playerOverlayWindow != nil {
				a.playerOverlayWindow.Invalidate()
			}
		})
		if time.Since(lastProgress) >= 30*time.Second {
			lastProgress = time.Now()
			go func() {
				if err := server.UpdateProgressContext(ctx, item.ID, item.MediaID, position, duration); err != nil && ctx.Err() == nil {
					a.window.Update(func() {
						if ctx.Err() == nil && a.player == player {
							a.status = "进度同步失败：" + err.Error()
						}
					})
				}
			}()
		}
	}
	if ctx.Err() != nil {
		return
	}
	snapshot := player.Snapshot()
	a.window.Update(func() {
		if ctx.Err() == nil && a.player == player && a.playback.Active && a.playingItem != nil && a.playingItem.ID == item.ID {
			if snapshot.Error != "" {
				a.stopPlayback()
				a.playback = PlaybackState{Active: true, Title: item.Title, Error: snapshot.Error}
				a.window.Invalidate()
			} else {
				// A clean mpv exit (including natural completion) returns to the
				// library instead of leaving a frozen video surface behind.
				a.stopPlayback()
			}
		}
	})
}

func (a *appState) stopPlayback() {
	server, item, position, duration := a.closePlayer()
	if server != nil && item != nil {
		go func() {
			if err := server.UpdateProgress(item.ID, item.MediaID, position, duration); err != nil {
				a.window.Update(func() {
					if a.server == server {
						a.status = "进度同步失败：" + err.Error()
					}
				})
			}
			a.window.Update(func() {
				if a.server == server && !a.home.Closed {
					a.loadContinueWatching(true)
				}
			})
		}()
	}
	if a.window != nil {
		restoreWindowFocus(a.window)
		a.window.Invalidate()
	}
}

func (a *appState) stopPlaybackAndWait() {
	server, item, position, duration := a.closePlayer()
	if server == nil || item == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.UpdateProgressContext(ctx, item.ID, item.MediaID, position, duration)
}

func (a *appState) closePlayer() (*Server, *MediaItem, float64, float64) {
	a.cancelPlaybackLoading()
	if a.playbackTrackCancel != nil {
		a.playbackTrackCancel()
		a.playbackTrackCancel = nil
	}
	var server *Server
	var item *MediaItem
	var position, duration float64
	if a.server != nil && a.playingItem != nil {
		server = a.server
		copy := *a.playingItem
		item = &copy
		position, duration = a.player.Position()
	}
	a.closePlayerOverlay()
	a.player.Stop()
	a.player.ReleaseSurface()
	a.playerProxy.Close()
	a.playerProxy = nil
	a.playingItem = nil
	a.playback = PlaybackState{}
	a.posters.SetPlayback(false)
	return server, item, position, duration
}

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
					a.catalogCache.SetDetail(a.settings.ServerURL, *a.selected, a.settings.Username)
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
