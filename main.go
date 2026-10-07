package main

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
	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.png
var appIconPNG []byte

type appState struct {
	window                   *mygo.Window
	settings                 Settings
	server                   *Server
	player                   *Player
	playerProxy              *PlaybackProxy
	section                  string
	query                    string
	username                 string
	password                 string
	status                   string
	busy                     bool
	libraryLoading           bool
	libraryErr               string
	loggedIn                 bool
	loginOpen                bool
	libraries                []MediaLibrary
	libraryID                string
	items                    []MediaItem
	catalogs                 map[string]*CatalogState
	catalogCache             *CatalogCache
	mediaView                mediaViewCache
	dataRevision             uint64
	searchChangedAt          time.Time
	searchPending            bool
	grid                     ui.GridState
	catalogScroll            ui.ScrollState
	catalogScrollKey         string
	catalogScrollAnimation   smoothScroll
	posters                  *PosterLoader
	selected                 *MediaItem
	seriesLoading            bool
	seriesError              string
	selectedSeasonID         string
	seriesEpisodes           []MediaItem
	seriesEpisodeCache       map[string][]MediaItem
	seriesEpisodeLoading     bool
	seriesEpisodeError       string
	seriesEpisodeRequest     uint64
	playback                 PlaybackState
	selectedTab              int
	playingItem              *MediaItem
	icon                     *ui.Bitmap
	displayScale             float64
	playerOverlayWindow      *mygo.Window
	playerHeaderWindow       *mygo.Window
	playerOverlayHooks       bool
	playerOverlayMonitorDone chan struct{}
	overlayMu                sync.Mutex
	playerOverlayVisible     bool
	playerOverlayPinned      bool
	playerOverlaySeeking     bool
	playerOverlayLastInput   time.Time
	playerOverlayMenu        string
	playerOverlayTransition  uint64
	playerOverlayOpacity     float64
	seekSliderPosition       float64
	seekDragging             bool
	volumeDragging           bool
}

func main() {
	settings, err := LoadSettings()
	if err != nil {
		log.Printf("settings: %v", err)
	}
	icon, iconErr := ui.DecodeBitmap(appIconPNG)
	if iconErr != nil {
		log.Printf("app icon: %v", iconErr)
	}
	app := &appState{settings: settings, section: "home", status: "连接飞牛影视服务器后开始浏览", loginOpen: true, player: NewPlayer(), posters: NewPosterLoader(), catalogs: map[string]*CatalogState{}, catalogCache: NewCatalogCache(), icon: icon}
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
			Title: "飞牛影视", Width: 1280, Height: 800, MinWidth: 960, MinHeight: 640,
			BackgroundColor: "#f7f6f2", StateKey: "main", Content: ui.View(app.view),
		})
		setWindowTheme(app.window)
		app.displayScale = windowScale(app.window.NativeHandle())
		app.window.OnResize(app.updateDisplayScale)
		app.window.OnMove(app.updateDisplayScale)
		if err := app.window.SetIcon(appIconPNG); err != nil {
			log.Printf("set app icon: %v", err)
		}
		if app.loggedIn {
			app.loginOpen = false
			app.loadLibraries()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { app.stopPlaybackAndWait() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *appState) view(c *ui.Context) {
	c.SetTheme(movieTheme())
	if a.playback.Active {
		a.playerView(c)
		return
	}
	ui.Row(c).Fill().Background(ui.Hex("#f7f6f2")).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).FillHeight().Padding(28, 34).Gap(20).Children(func() {
			a.topbar(c)
			switch {
			case a.selected != nil:
				a.detailView(c, *a.selected)
			default:
				a.libraryView(c)
			}
		})
	})
	a.loginModal(c)
}

func movieTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f7f6f2")
	t.Surface = ui.Hex("#fffefa")
	t.SurfaceHover = ui.Hex("#f0f1ec")
	t.SurfacePressed = ui.Hex("#e7ebe5")
	t.Text = ui.Hex("#252a27")
	t.TextMuted = ui.Hex("#78837d")
	t.Border = ui.Hex("#e5e5df")
	t.Accent = ui.Hex("#376655")
	t.AccentHover = ui.Hex("#427764")
	t.AccentPressed = ui.Hex("#2f594b")
	t.AccentText = ui.Hex("#ffffff")
	t.Radius = 11
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
				ui.Text(c, "飞牛影视").FontSize(16).Bold()
				ui.Text(c, "FN MOVIES").FontSize(9).TextColor(t.TextMuted).LetterSpacing(1.2)
			})
		})
		ui.Box(c).Height(15)
		a.navButton(c, "⌂", "全部影片", "home")
		a.navButton(c, "♡", "我的收藏", "favorites")
		a.navButton(c, "◷", "观看记录", "history")
		ui.Text(c, "影视库").Padding(0, 10).FontSize(10).Bold().TextColor(t.TextMuted)
		ui.Scroll(c).Grow(1).Children(func() {
			for _, library := range a.libraries {
				selected := a.section == "library" && a.libraryID == library.ID
				button := ui.Button(c, "").Padding(11, 12).Gap(12).Radius(9).BorderWidth(0).Background(ui.Color{}).TextColor(t.TextMuted).
					Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond})
				if selected {
					button.Background(ui.Hex("#e6eee8")).TextColor(t.Accent)
				} else if button.Hovered() {
					button.Background(ui.Hex("#eeece6"))
				}
				button.Children(func() {
					ui.Text(c, library.Name).FontSize(13).SingleLine()
				})
				if button.Clicked() {
					a.section, a.libraryID, a.selected, a.query = "library", library.ID, nil, ""
					a.selectedTab = 0
					a.grid = ui.GridState{}
					a.loadLibrary()
				}
			}
		})
		if a.loggedIn && ui.Button(c, "⇥  退出登录").Padding(11, 12).Gap(12).Radius(9).BorderWidth(0).
			Background(ui.Color{}).TextColor(t.TextMuted).Transition(ui.ElementTransition{Colors: true, Duration: 150 * time.Millisecond}).Clicked() {
			a.logout()
		} else if !a.loggedIn && ui.Button(c, "登录到服务器").Padding(10, 12).Clicked() {
			a.loginOpen = true
		}
	})
}

func (a *appState) navButton(c *ui.Context, icon, label, key string) {
	selected := a.section == key
	e := ui.Button(c, "").Padding(11, 12).Gap(12).Radius(9).BorderWidth(0)
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
		ui.Text(c, icon).FontSize(16).Width(19)
		ui.Text(c, label).FontSize(13)
	})
	if e.Clicked() {
		a.section = key
		a.selected = nil
		a.query = ""
		a.grid = ui.GridState{}
		if a.loggedIn && (key == "home" || key == "favorites" || key == "history") {
			a.libraryID = ""
			a.loadLibrary()
		}
	}
}

func (a *appState) topbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(14).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			label := map[string]string{"home": "全部影片", "favorites": "我的收藏", "history": "观看记录", "settings": "服务器设置", "library": a.selectedLibraryName()}[a.section]
			if label == "" {
				label = "影视库"
			}
			ui.Text(c, label).FontSize(27).Bold()
		})
		if a.loggedIn && a.section != "settings" {
			if ui.SearchField(c, &a.query).Width(270).Placeholder("搜索影片、演员或导演").Changed() {
				a.searchChangedAt = c.Now()
				a.searchPending = true
				c.After(320 * time.Millisecond)
			}
		}
		if a.searchPending && c.Now().Sub(a.searchChangedAt) >= 300*time.Millisecond {
			a.searchPending = false
			a.loadLibrary()
		}
		if a.busy || a.libraryLoading || a.currentCatalogLoading() {
			ui.Text(c, "加载中").FontSize(11).TextColor(t.TextMuted)
		}
		if state := a.catalogs[a.currentCatalogKey()]; state != nil && state.Err != "" && len(state.Items) > 0 {
			if ui.Button(c, "重试").Padding(5, 9).Clicked() {
				a.loadLibrary()
			}
		}
		if a.libraryErr != "" && !a.libraryLoading {
			if ui.Button(c, "重试影视库").Padding(5, 9).Clicked() {
				a.loadLibraries()
			}
		}
	})
}

func (a *appState) connectionView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Fill().Center().Children(func() {
		ui.Column(c).Width(440).Padding(34).Gap(16).Radius(16).Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Text(c, "连接你的影视库").FontSize(22).Bold()
			ui.Text(c, "输入飞牛影视地址和账户信息，完成连接后即可浏览与播放媒体。").FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.settings.ServerURL).Placeholder("http://192.168.31.86/v").Label("服务器地址")
			ui.TextInput(c, &a.username).Placeholder("飞牛影视用户名").Label("用户名")
			ui.TextInput(c, &a.password).Password().Placeholder("密码").Label("密码").Submitted()
			if a.status != "" {
				ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted)
			}
			if ui.PrimaryButton(c, "连接并登录").Disabled(a.busy).Clicked() {
				a.login()
			}
		})
	})
}

func (a *appState) loginModal(c *ui.Context) {
	ui.Modal(c, &a.loginOpen, func() {
		t := c.Theme()
		ui.Column(c).Width(430).Padding(28, 30).Gap(15).Radius(16).Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Text(c, "连接飞牛影视").FontSize(22).Bold()
			ui.Text(c, "登录后浏览你的媒体库并在本机播放影片。").FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.settings.ServerURL).Placeholder("http://192.168.31.86:5666/v").Label("服务器地址")
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
				if ui.PrimaryButton(c, "连接并登录").Disabled(a.busy).Clicked() {
					a.login()
				}
				if a.loggedIn && ui.Button(c, "取消").Clicked() {
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
		a.catalogScrollAnimation = smoothScroll{}
		a.grid = ui.GridState{}
	}
	state := a.catalogs[a.currentCatalogKey()]
	if len(items) == 0 {
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Text(c, "⌕").FontSize(34).TextColor(t.TextMuted)
			if a.status != "" {
				ui.Text(c, a.status).FontSize(13).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "这里还没有内容").FontSize(14).TextColor(t.TextMuted)
			}
			if ui.Button(c, "重新加载").Clicked() {
				a.loadLibrary()
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
					if ui.Button(c, "加载失败，点击重试").Padding(8, 10).Clicked() {
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
		card := ui.Button(c, "").Key(item.ID).Padding(0).BorderWidth(0).Background(ui.Color{}).TextColor(t.Text)
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
						ui.Text(c, item.Rating).Absolute().Top(10).Left(10).Padding(4, 9).Radius(6).
							Background(ui.Hex("#252821")).FontSize(16).Bold().TextColor(ui.Hex("#f1c531"))
					}
				})
				ui.Text(c, item.Title).FontSize(16).Bold().TextAlign(ui.Center).SingleLine()
				ui.Text(c, cardCaption(item)).FontSize(14).TextColor(t.TextMuted).TextAlign(ui.Center).SingleLine()
			})
		})
		if card.Clicked() {
			a.openDetail(item)
		}
	})
	grid.TrackScroll(&a.catalogScroll).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll {
			return false
		}
		now := c.Now()
		current, active := a.catalogScrollAnimation.Position(now)
		if !active {
			current = a.catalogScroll.Y
		}
		base := current
		if a.catalogScrollAnimation.Active {
			base = a.catalogScrollAnimation.To
		}
		duration := catalogScrollDuration
		if ev.Precise {
			duration = 110 * time.Millisecond
		}
		target := max(float32(0), min(a.catalogScroll.MaxY, base+ev.DY))
		a.catalogScrollAnimation.Retarget(current, target, now, duration)
		a.advanceCatalogScroll(c)
		return true
	})
	a.advanceCatalogScroll(c)
	background := t.Background
	grid.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		const fadeHeight = 28
		p.FillGradient(ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: fadeHeight}, ui.LinearGradient{
			From: background, To: background.Alpha(0), Angle: 180,
		}, 0)
		p.FillGradient(ui.Rect{X: rect.X, Y: rect.Y + rect.H - fadeHeight, W: rect.W, H: fadeHeight}, ui.LinearGradient{
			From: background.Alpha(0), To: background, Angle: 180,
		}, 0)
	})
}

func cardCaption(item MediaItem) string {
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
					if ui.PrimaryButton(c, "▶  继续播放").Clicked() {
						a.startPlayback(item)
					}
				} else if ui.PrimaryButton(c, "▶  立即播放").Clicked() {
					a.startPlayback(item)
				}
				if ui.Button(c, "查看详情").Clicked() {
					a.openDetail(item)
				}
			})
		})
	})
}

func (a *appState) openDetail(item MediaItem) {
	a.selected = &item
	a.seriesLoading, a.seriesError, a.selectedSeasonID = false, "", ""
	a.seriesEpisodes, a.seriesEpisodeCache = nil, nil
	a.seriesEpisodeLoading, a.seriesEpisodeError = false, ""
	a.seriesEpisodeRequest++
	if a.server == nil {
		return
	}
	server := a.server
	if item.IsSeries {
		a.seriesLoading = true
		if len(item.Episodes) == 0 {
			a.seriesLoading = false
			a.seriesError = "当前目录中没有可用于识别剧集的条目。"
			return
		}
		episode := item.Episodes[0]
		go func() {
			seasons, err := server.SeriesSeasons(episode)
			a.window.Update(func() {
				if a.selected == nil || a.selected.ID != item.ID {
					return
				}
				a.seriesLoading = false
				if err != nil && len(seasons) == 0 {
					a.seriesError = "剧集列表读取失败：" + err.Error()
					return
				}
				updated := *a.selected
				updated.Seasons = seasons
				a.selected = &updated
				if len(seasons) > 0 {
					a.selectSeriesSeason(seasons[0].ID)
				}
				if err != nil {
					a.seriesError = "部分剧集读取失败：" + err.Error()
				}
			})
		}()
		return
	}
	go func() {
		detail, err := server.Detail(item)
		a.window.Update(func() {
			if err != nil {
				a.status = "详情读取失败：" + err.Error()
				return
			}
			if a.selected != nil && a.selected.ID == item.ID {
				a.selected = &detail
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
	ui.Row(c).Gap(28).Grow(1).Children(func() {
		if poster := a.imageFor(item, 250, 365); poster != nil {
			ui.Image(c, poster).Size(250, 365).Fit(ui.Cover).Radius(12)
		} else {
			ui.Box(c).Size(250, 365).Radius(12).Background(ui.Hex("#e8e8e2"))
		}
		ui.Column(c).Grow(1).Padding(12, 0).Gap(14).Children(func() {
			ui.Text(c, item.Title).FontSize(30).Bold()
			ui.Text(c, item.Subtitle()).FontSize(13).TextColor(t.TextMuted)
			if item.Overview != "" {
				ui.Text(c, item.Overview).FontSize(13).TextColor(ui.Hex("#66716b"))
			}
			ui.Row(c).Gap(10).Children(func() {
				if ui.PrimaryButton(c, "▶  立即播放").Clicked() {
					a.startPlayback(item)
				}
				if ui.Button(c, "♡  收藏").Clicked() {
					a.toggleFavorite(item)
				}
			})
			if len(item.Sources) > 0 {
				ui.Text(c, "可播放版本").FontSize(12).Bold().TextColor(t.TextMuted)
				for _, source := range item.Sources {
					ui.Row(c).Gap(10).Padding(10, 12).Radius(8).Background(t.Surface).Children(func() {
						ui.Text(c, source.Name).Grow(1)
						ui.Text(c, source.Quality).TextColor(t.TextMuted)
						if ui.Button(c, "播放").Clicked() {
							a.startPlayback(item)
						}
					})
				}
			}
			ui.Spacer(c)
			if ui.Button(c, "← 返回影视库").Clicked() {
				a.selected = nil
			}
		})
	})
}

func (a *appState) seriesDetailView(c *ui.Context, item MediaItem) {
	t := c.Theme()
	ui.Row(c).Gap(26).Grow(1).Children(func() {
		if poster := a.imageFor(item, 230, 345); poster != nil {
			ui.Image(c, poster).Size(230, 345).Fit(ui.Cover).Radius(12)
		} else {
			ui.Box(c).Size(230, 345).Radius(12).Background(ui.Hex("#e8e8e2"))
		}
		ui.Column(c).Grow(1).Padding(8, 0).Gap(12).Children(func() {
			ui.Text(c, item.Title).FontSize(28).Bold()
			ui.Text(c, item.Subtitle()).FontSize(13).TextColor(t.TextMuted)
			if item.Overview != "" {
				ui.Text(c, item.Overview).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
			}
			if a.seriesLoading {
				ui.Text(c, "正在读取季度与剧集…").FontSize(12).TextColor(t.TextMuted)
			} else if a.seriesError != "" {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, a.seriesError).FontSize(11).TextColor(ui.Hex("#ad5148"))
					if ui.Button(c, "重试").Clicked() {
						a.openDetail(item)
					}
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				for _, season := range item.Seasons {
					label := season.Title
					if strings.TrimSpace(label) == "" {
						label = fmt.Sprintf("第%d季", season.Number)
					}
					button := ui.Button(c, label).Padding(7, 11).BorderWidth(0).Radius(8)
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
			ui.Scroll(c).Grow(1).Children(func() {
				if a.seriesEpisodeLoading {
					ui.Text(c, "正在读取本季集数…").Padding(12, 10).FontSize(12).TextColor(t.TextMuted)
					return
				}
				if a.seriesEpisodeError != "" {
					ui.Row(c).Padding(12, 10).Gap(8).Children(func() {
						ui.Text(c, a.seriesEpisodeError).Grow(1).FontSize(11).TextColor(ui.Hex("#ad5148"))
						if ui.Button(c, "重试").Clicked() {
							a.loadSeriesSeason(a.selectedSeasonID, true)
						}
					})
					return
				}
				if len(a.seriesEpisodes) == 0 {
					ui.Text(c, "这个季度还没有集数").Padding(12, 10).FontSize(12).TextColor(t.TextMuted)
					return
				}
				for _, episode := range a.seriesEpisodes {
					ui.Row(c).Padding(8, 10).Gap(10).AlignItems(ui.Center).Children(func() {
						number := ""
						if episode.EpisodeNumber > 0 {
							number = fmt.Sprintf("%02d", episode.EpisodeNumber)
						}
						ui.Text(c, number).Width(30).FontSize(12).TextColor(t.TextMuted)
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, episode.Title).FontSize(13).Bold().SingleLine()
							if overview := firstString(episode.Raw, "overview", "description", "summary"); overview != "" {
								ui.Text(c, overview).FontSize(10).TextColor(t.TextMuted).MaxLines(1)
							}
						})
						if ui.Button(c, "播放").Clicked() {
							a.startPlayback(episode)
						}
					})
				}
			})
			ui.Spacer(c)
			if ui.Button(c, "← 返回影视库").Clicked() {
				a.selected = nil
			}
		})
	})
}

func (a *appState) selectSeriesSeason(seasonID string) {
	a.selectedSeasonID = seasonID
	a.seriesEpisodes = nil
	a.seriesEpisodeError = ""
	if items, ok := a.seriesEpisodeCache[seasonID]; ok {
		a.seriesEpisodes = items
		a.seriesEpisodeLoading = false
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
			if a.selected == nil || a.selected.ID != selectedID || a.selectedSeasonID != seasonID || requestID != a.seriesEpisodeRequest {
				return
			}
			a.seriesEpisodeLoading = false
			if err != nil {
				a.seriesEpisodeError = "集数读取失败：" + err.Error()
				return
			}
			if a.seriesEpisodeCache == nil {
				a.seriesEpisodeCache = map[string][]MediaItem{}
			}
			a.seriesEpisodeCache[seasonID] = items
			a.seriesEpisodes = items
		})
	}()
}

func (a *appState) settingsView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(620).Gap(14).Children(func() {
		ui.Text(c, "服务器连接").FontSize(18).Bold()
		ui.Text(c, a.settings.ServerURL).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).Children(func() {
			if ui.PrimaryButton(c, "切换账户或服务器").Clicked() {
				a.loginOpen = true
			}
			if ui.Button(c, "退出登录").Clicked() {
				a.loggedIn = false
				a.items = nil
				a.libraries = nil
				a.cancelCatalogRequests()
				a.catalogs = map[string]*CatalogState{}
				a.selected = nil
				a.mediaView.Invalidate()
				a.dataRevision++
				a.server = nil
				_ = DeleteCredential(a.settings.ServerURL)
				_ = SaveSettings(a.settings)
				a.status = "已退出登录"
				a.loginOpen = true
			}
		})
		ui.Text(c, "账户凭据由 Windows 凭据管理器保护；服务器地址与界面偏好保存在本机。").FontSize(11).TextColor(t.TextMuted).MaxLines(2)
		if a.status != "" {
			ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

func (a *appState) logout() {
	serverURL := a.settings.ServerURL
	a.cancelCatalogRequests()
	a.loggedIn = false
	a.loginOpen = true
	a.libraryLoading = false
	a.libraries = nil
	a.items = nil
	a.catalogs = map[string]*CatalogState{}
	a.selected = nil
	a.libraryID = ""
	a.query = ""
	a.password = ""
	a.mediaView.Invalidate()
	a.dataRevision++
	a.server = nil
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
				if ui.Button(c, "返回影片库").Clicked() {
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

func (a *appState) advanceCatalogScroll(c *ui.Context) {
	if !a.catalogScrollAnimation.Active {
		return
	}
	position, active := a.catalogScrollAnimation.Position(c.Now())
	a.catalogScroll.Y = position
	if active {
		c.AnimationFrame()
	}
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
	if a.password == "" {
		if credentials, err := ReadCredential(a.settings.ServerURL); err == nil {
			a.password = credentials.Password
		}
	}
	if strings.TrimSpace(a.password) == "" {
		a.status = "请输入账户密码后再连接"
		return
	}
	a.busy = true
	a.status = "正在连接服务器…"
	serverURL, username, password := a.settings.ServerURL, a.username, a.password
	settings := a.settings
	if a.window != nil {
		a.window.Invalidate()
	}
	go func() {
		server := NewServer(serverURL, "")
		result, err := server.Login(username, password)
		if err == nil {
			settings.Username = username
			err = WriteCredential(serverURL, Credential{Username: username, Password: password, Token: result.Token})
			if err == nil {
				err = SaveSettings(settings)
			}
		}
		a.window.Update(func() {
			a.busy = false
			if err != nil {
				a.status = "连接失败：" + err.Error()
				a.loginOpen = true
				return
			}
			a.server = server
			a.settings = settings
			a.loggedIn = true
			a.loginOpen = false
			a.mediaView.Invalidate()
			a.dataRevision++
			a.cancelCatalogRequests()
			a.catalogs = map[string]*CatalogState{}
			a.items = nil
			a.selected = nil
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
	serverURL, username, password := a.settings.ServerURL, a.username, a.password
	if cached := a.catalogCache.Libraries(serverURL); len(a.libraries) == 0 && len(cached) > 0 {
		a.libraries = cached
		a.loadLibrary()
	}
	a.window.Invalidate()
	go func() {
		libraries, err := server.Libraries()
		if err != nil && isAuthError(err) && username != "" && password != "" {
			if result, loginErr := server.Login(username, password); loginErr == nil {
				_ = WriteCredential(serverURL, Credential{Username: username, Password: password, Token: result.Token})
				libraries, err = server.Libraries()
			}
		}
		a.window.Update(func() {
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
			a.catalogCache.SetLibraries(serverURL, libraries)
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
						a.section = "home"
						a.status = "所选影视库已不可访问，已返回全部影片"
					}
					a.libraryID = ""
				}
			}
			if len(libraries) == 0 {
				if len(a.items) == 0 {
					a.status = "此账户没有可访问的影视库"
				}
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
	if a.section == "library" && strings.TrimSpace(a.libraryID) == "" {
		a.items = nil
		a.status = "无法读取所选影视库：服务器没有返回有效的影视库标识"
		a.window.Invalidate()
		return
	}
	server, libraryID, query := a.server, a.libraryID, strings.TrimSpace(a.query)
	serverURL := a.settings.ServerURL
	key := libraryID + "\x00" + query
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
			if cached, ok := a.catalogCache.Page(serverURL, libraryID); ok {
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
	if state.Refreshed && !state.UpdatedAt.IsZero() && time.Since(state.UpdatedAt) < 5*time.Minute {
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
	a.fetchCatalogPage(ctx, key, server, serverURL, libraryID, query, 1, true)
}

func (a *appState) currentCatalogKey() string {
	return a.libraryID + "\x00" + strings.TrimSpace(a.query)
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

func (a *appState) fetchCatalogPage(ctx context.Context, key string, server *Server, serverURL, libraryID, query string, page int, first bool) {
	go func() {
		items, total, err := server.LibraryPageContext(ctx, libraryID, query, page, catalogPageSize)
		if err != nil && isAuthError(err) {
			if credentials, readErr := ReadCredential(serverURL); readErr == nil && credentials.Password != "" {
				if result, loginErr := server.Login(credentials.Username, credentials.Password); loginErr == nil {
					_ = WriteCredential(serverURL, Credential{Username: credentials.Username, Password: credentials.Password, Token: result.Token})
					items, total, err = server.LibraryPageContext(ctx, libraryID, query, page, catalogPageSize)
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		a.window.Update(func() {
			state := a.catalogs[key]
			if state == nil {
				return
			}
			if ctx.Err() != nil {
				return
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
				a.catalogCache.SetPage(serverURL, libraryID, CatalogPage{Items: state.Items, Total: total})
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
	a.fetchCatalogPage(ctx, key, a.server, a.settings.ServerURL, a.libraryID, strings.TrimSpace(a.query), state.NextPage, false)
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
	server := a.server
	go func() {
		stream, err := server.Playback(item)
		a.window.Update(func() {
			if err != nil {
				a.status = "无法获取播放源：" + err.Error()
				return
			}
			item.MediaID = stream.MediaID
			a.playingItem = &item
			a.playback = PlaybackState{Active: true, Title: item.Title, URL: stream.URL, Position: stream.ResumeAt, Duration: stream.Duration, Volume: 100, Speed: 1, Quality: stream.Quality}
			localURL, proxy, err := server.ProxyPlayback(stream.URL)
			if err != nil {
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放代理启动失败：" + err.Error()
				return
			}
			a.playerProxy = proxy
			if err := a.player.Start(localURL, a.window.NativeHandle(), stream.Duration, stream.ResumeAt); err != nil {
				proxy.Close()
				a.playerProxy = nil
				a.playback.Active = false
				a.playingItem = nil
				a.status = "播放器启动失败：" + err.Error()
				return
			}
			a.window.Invalidate()
			a.seekSliderPosition = stream.ResumeAt
			a.createPlayerOverlay()
			a.startPlayerOverlayMonitor()
			go a.trackPlayback(item, a.server)
		})
	}()
}

func (a *appState) trackPlayback(item MediaItem, server *Server) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastProgress := time.Now()
	for a.player.Running() {
		<-ticker.C
		if !a.player.Running() {
			return
		}
		snapshot := a.player.Snapshot()
		position, duration := snapshot.Position, snapshot.Duration
		a.window.Update(func() {
			if !a.playback.Active || a.playingItem == nil || a.playingItem.ID != item.ID {
				return
			}
			a.playback.Position = position
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
				if err := server.UpdateProgress(item.ID, item.MediaID, position, duration); err != nil {
					a.window.Update(func() { a.status = "进度同步失败：" + err.Error() })
				}
			}()
		}
	}
	snapshot := a.player.Snapshot()
	a.window.Update(func() {
		if a.playback.Active && a.playingItem != nil && a.playingItem.ID == item.ID {
			if snapshot.Error != "" {
				a.playback.Error = snapshot.Error
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
				a.window.Update(func() { a.status = "进度同步失败：" + err.Error() })
			}
		}()
	}
	if a.window != nil {
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
	a.playerProxy.Close()
	a.playerProxy = nil
	a.playingItem = nil
	a.playback = PlaybackState{}
	return server, item, position, duration
}

func (a *appState) toggleFavorite(item MediaItem) {
	if a.server == nil {
		return
	}
	server := a.server
	go func() {
		if err := server.ToggleFavorite(item); err != nil {
			a.window.Update(func() { a.status = "收藏状态更新失败：" + err.Error() })
			return
		}
		a.window.Update(func() {
			favorite := !item.Favorite
			a.updateFavoriteProjection(item.ID, favorite)
			a.status = "收藏状态已更新"
			if state := a.catalogs[a.currentCatalogKey()]; state != nil {
				state.Refreshed = false
			}
			a.loadLibrary()
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
