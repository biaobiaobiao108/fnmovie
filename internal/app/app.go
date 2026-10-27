package app

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/icon.png
var appIconPNG []byte

const applicationWindowTitle = "哞哩影院"

func windowTitleForPlayback(playback PlaybackState) string {
	if playback.Active {
		if title := strings.Join(strings.Fields(playback.Title), " "); title != "" {
			return title
		}
	}
	return applicationWindowTitle
}

func (a *appState) syncWindowTitle() {
	if a.window == nil {
		return
	}
	title := windowTitleForPlayback(a.playback)
	if a.window.Title() != title {
		// Keep the native title current for task switching and accessibility,
		// even while the title bar itself is hidden.
		a.window.SetTitle(title)
	}
}

// Run starts the native desktop application.
func Run() {
	configureCompatibleAppPaths()
	settings, err := LoadSettings()
	if err != nil {
		log.Printf("settings: %v", err)
	}
	app := &appState{
		settings: settings, serverAddress: settings.ServerURL, player: NewPlayer(), posters: NewPosterLoader(),
		navigationUIState: navigationUIState{section: "home"},
		sessionUIState:    sessionUIState{status: "连接飞牛影视服务器后开始浏览", loginOpen: true},
		catalogUIState:    catalogUIState{catalogs: map[string]*CatalogState{}, catalogCache: NewCatalogCache()},
		detailUIState:     detailUIState{favoritePending: map[string]bool{}},
	}
	if settings.ServerURL != "" {
		app.server = NewServer(settings.ServerURL, "")
		if settings.Username != "" {
			app.username = settings.Username
			if credentials, err := ReadCredential(settings.ServerURL); err == nil && credentials.Username == settings.Username {
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
			Title: applicationWindowTitle, Width: 1280, Height: 800, MinWidth: 960, MinHeight: 640,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: movieTitleBarHeight,
			Vibrancy: mygo.VibrancySidebar, BackgroundColor: movieWindowBackground,
			StateKey: "main", Content: ui.View(app.view),
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
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		app.resetHome()
		app.home.Closed = true
		app.closeDetail()
		app.closePerson()
		app.stopPlaybackAndWait(ctx)
		app.playbackProgressReporter.close(ctx)
		app.posters.Close()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *appState) view(c *ui.Context) {
	a.syncHomeLifecycle()
	c.SetTheme(movieTheme())
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	if a.loggedIn && a.searchPending && c.Now().Sub(a.searchChangedAt) >= 300*time.Millisecond {
		a.searchPending = false
		a.loadLibrary()
	}
	a.advancePlayerFullscreen(c)
	if a.playback.Active {
		c.Root().Background(ui.RGB(0, 0, 0))
		a.advancePlayerOverlayAnimation(c)
		a.playerView(c)
		return
	}
	secondary := a.selectedPerson != nil || a.selected != nil
	host := ui.Box(c.Key("navigation-host")).Fill()
	if secondary {
		host.Background(c.Theme().Background)
	}
	host.Children(func() {
		key := "main-shell"
		if secondary {
			key = a.navigationKey()
		}
		ui.Box(c.Key(key)).Fill().Transition(pageTransition()).Children(func() {
			if secondary {
				a.secondaryPage(c)
			} else {
				a.primaryPage(c)
			}
		})
	})
	if !secondary {
		a.loginModal(c)
		a.syncHomeCarousel()
	}
	a.detailSynopsisDialog(c)
}

func (a *appState) primaryPage(c *ui.Context) {
	t := c.Theme()
	titleBar := c.TitleBar()
	titleHeight := titleBar.Height
	if titleHeight <= 0 {
		titleHeight = movieTitleBarHeight
	}
	background := t.Background
	if c.Vibrancy() {
		background = ui.Transparent
	}
	ui.Row(c).Fill().Background(background).Children(func() {
		a.sidebar(c)
		main := ui.Box(c.Key("main-content")).Grow(1).FillHeight().MinWidth(0).Background(t.Background)
		main.Children(func() {
			ui.Column(c).Grow(1).FillHeight().Padding(0, movieMainHorizontalInset, 12, movieMainHorizontalInset).Children(func() {
				ui.Column(c.Key(a.navigationKey())).Grow(1).FillHeight().Transition(pageTransition()).Children(func() {
					switch {
					case strings.TrimSpace(a.query) != "":
						a.libraryView(c)
					case a.section == "home":
						a.homeView(c)
					default:
						a.libraryView(c)
					}
				})
			})
		})
		edge := ui.Box(c.Key("window-title-material")).Absolute().Top(0).Left(movieSidebarWidth).
			Right(0).Height(titleHeight).PassThrough().
			Material(glass.ScrollEdge{Hard: true, Background: t.Background})
		shown := float32(0)
		if a.contentUnderTitleBar(titleHeight) {
			shown = 1
		}
		edge.Opacity(edge.AnimateTo("shown", shown, ui.Animation{Duration: 180 * time.Millisecond}))
		ui.Box(c.Key("window-drag-region")).Absolute().Top(0).Left(movieSidebarWidth).
			Right(titleBar.Right).Height(titleHeight).DragWindow()
	})
}

func (a *appState) contentUnderTitleBar(titleHeight float32) bool {
	if strings.TrimSpace(a.query) == "" && a.section == "home" {
		return a.home.AllContinue && len(a.home.Continue) > 0 &&
			a.home.GridScroll.Y > max(0, float32(movieTitleBarHeight+16)-titleHeight)+0.5
	}
	return len(a.visibleItems()) > 0 && a.catalogScroll.Y > max(0, float32(movieTitleBarHeight)-titleHeight)+0.5
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

// bindSoftScrollEdges fades clipped content into the page surface while it
// passes a scroll viewport's edge. The soft MyGo material is a gradient and
// avoids the heavy frosted band of a hard scroll edge.
func bindSoftScrollEdges(element ui.Element, state *ui.ScrollState, background ui.Color) {
	element.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		edgeHeight := min(float32(18), rect.H/3)
		if state.Y > 1 {
			glass.ScrollEdge{Background: background}.PaintMaterial(p,
				ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: edgeHeight}, [4]float32{})
		}
		if state.MaxY-state.Y > 1 {
			glass.ScrollEdge{Bottom: true, Background: background}.PaintMaterial(p,
				ui.Rect{X: rect.X, Y: rect.Y + rect.H - edgeHeight, W: rect.W, H: edgeHeight}, [4]float32{})
		}
	})
}

func detailPosterSize(c *ui.Context) (int, int) {
	width, _ := c.Size()
	posterWidth := min(max(280, min(460, int(float64(width)*0.31))), int((detailViewportHeight(c)-54)*2/3))
	return posterWidth, posterWidth * 3 / 2
}

func detailPlayButton(c *ui.Context, label string) ui.Element {
	return primaryActionButton(c, label).Width(136).Height(32).Padding(6, 10).Radius(8).BorderWidth(0).FontSize(13)
}

func detailViewportHeight(c *ui.Context) float32 {
	_, height := c.Size()
	return max(float32(280), height-float32(movieTitleBarHeight+58))
}

func detailSynopsisLines(c *ui.Context) int {
	width, _ := c.Size()
	return min(5, max(2, int(width/400)))
}

func episodeOverview(episode MediaItem) string {
	if episode.Overview != "" {
		return episode.Overview
	}
	return firstString(episode.Raw, "overview", "description", "summary")
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

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr != nil && apiErr.Code == 401 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sign") || strings.Contains(message, "401") || strings.Contains(message, "unauthor")
}
