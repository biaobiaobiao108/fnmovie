package app

import (
	_ "embed"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
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
			BackgroundColor: movieWindowBackground, StateKey: "main", Content: ui.View(app.view),
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
		app.resetHome()
		app.home.Closed = true
		app.closeDetail()
		app.closePerson()
		app.stopPlaybackAndWait()
		app.posters.Close()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *appState) view(c *ui.Context) {
	a.syncHomeLifecycle()
	c.SetTheme(movieTheme())
	if a.loggedIn && a.searchPending && c.Now().Sub(a.searchChangedAt) >= 300*time.Millisecond {
		a.searchPending = false
		a.loadLibrary()
	}
	a.advancePlayerFullscreen(c)
	if a.playback.Active {
		a.advancePlayerOverlayAnimation(c)
		a.playerView(c)
		return
	}
	t := c.Theme()
	titleBar := c.TitleBar()
	ui.Row(c).Fill().Background(t.Background).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).FillHeight().Padding(movieTitleBarHeight, movieMainHorizontalInset, 12, movieMainHorizontalInset).Children(func() {
			ui.Column(c.Key(a.navigationKey())).Grow(1).FillHeight().Transition(pageTransition()).Children(func() {
				switch {
				case a.selectedPerson != nil:
					a.personView(c, *a.selectedPerson)
				case a.selected != nil:
					a.detailView(c, *a.selected)
				case strings.TrimSpace(a.query) != "":
					a.libraryView(c)
				case a.section == "home":
					a.homeView(c)
				default:
					a.libraryView(c)
				}
			})
		})
		ui.Box(c.Key("window-drag-region")).Absolute().Top(0).Left(movieSidebarWidth).
			Right(titleBar.Right).Height(titleBar.Height).DragWindow()
	})
	a.loginModal(c)
	a.detailSynopsisDialog(c)
	a.syncHomeCarousel()
}

// Draw after the scroll content without adding an input-catching overlay.
func bindScrollEdgeFades(element ui.Element, state *ui.ScrollState, background ui.Color) {
	element.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		fadeHeight := min(float32(24), rect.H/2)
		if state.Y > 1 {
			p.FillGradient(ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: fadeHeight}, ui.LinearGradient{
				From: background, To: background.Alpha(0), Angle: 180,
			}, 0)
		}
		if state.MaxY-state.Y > 1 {
			p.FillGradient(ui.Rect{X: rect.X, Y: rect.Y + rect.H - fadeHeight, W: rect.W, H: fadeHeight}, ui.LinearGradient{
				From: background.Alpha(0), To: background, Angle: 180,
			}, 0)
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

func detailPosterSize(c *ui.Context) (int, int) {
	width, _ := c.Size()
	posterWidth := min(max(250, min(360, int(float64(width)*0.26))), int((detailViewportHeight(c)-78)*2/3))
	return posterWidth, posterWidth * 3 / 2
}

func detailPlayButton(c *ui.Context, label string) ui.Element {
	return primaryActionButton(c, label).Width(136).Height(32).Padding(6, 10).Radius(8).BorderWidth(0).FontSize(13)
}

func detailViewportHeight(c *ui.Context) float32 {
	_, height := c.Size()
	// Window-button inset, outer padding and back navigation row.
	return max(float32(240), height-118)
}

func detailContentInset(c *ui.Context) float32 {
	_, height := detailPosterSize(c)
	return max(float32(0), (detailViewportHeight(c)-54-float32(height))/2)
}

func detailSynopsisLines(c *ui.Context) int {
	_, height := detailPosterSize(c)
	return max(1, min(8, (height-366)/22))
}

func detailCastGap(c *ui.Context) float32 {
	_, height := detailPosterSize(c)
	return max(float32(0), float32(height)-150-float32(detailSynopsisLines(c)*22)-200)
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
