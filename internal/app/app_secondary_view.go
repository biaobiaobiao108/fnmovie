package app

import "github.com/egoist/mygo/ui"

func (a *appState) secondaryPage(c *ui.Context) {
	key := a.navigationKey()
	var backdropURL string
	if a.selectedPerson != nil {
		backdropURL = a.selectedPerson.Profile
	} else if a.selected != nil {
		backdropURL = a.selected.Backdrop
		if backdropURL == "" {
			backdropURL = a.selected.Poster
		}
	}
	backdrop, tint := a.imageForURLWithTint(backdropURL, 1280, 720)
	theme := immersiveDetailTheme(tint)
	c.SetTheme(theme)

	root := ui.Box(c.Key("secondary-page:" + key)).Grow(1).Fill().Clip().Background(theme.Background)
	root.Children(func() {
		if backdrop != nil {
			ui.Image(c, backdrop).Absolute().Top(-100).Left(-100).Right(-100).Bottom(-100).
				Fit(ui.Cover).Opacity(0.30).Blur(34).PassThrough()
		}
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, rect ui.Rect) {
			p.FillGradient(rect, ui.LinearGradient{
				From:  tint.Mix(ui.Hex("#0b0c10"), 0.48).Alpha(0.62),
				To:    ui.Hex("#0b0c10").Alpha(0.84),
				Angle: 90,
			}, 0)
			p.FillGradient(rect, ui.LinearGradient{
				From:  ui.RGBA(7, 8, 11, 0.12),
				To:    ui.RGBA(7, 8, 11, 0.30),
				Angle: 0,
			}, 0)
		})
		ui.Column(c.Key("secondary-content:" + key)).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Transition(pageTransition()).Children(func() {
			a.secondaryHeader(c)
			if a.selectedPerson != nil {
				a.personView(c, *a.selectedPerson)
			} else if a.selected != nil {
				a.detailView(c, *a.selected)
			}
		})
	})
}

func immersiveDetailTheme(tint ui.Color) *ui.Theme {
	accent := tint.Mix(ui.RGB(255, 255, 255), 0.28)
	if tint == (ui.Color{}) {
		accent = movieColors.accent.Mix(ui.RGB(255, 255, 255), 0.2)
	}
	t := *movieTheme()
	t.Background = ui.Hex("#0b0c10")
	t.Surface = ui.RGBA(255, 255, 255, 0.09)
	t.SurfaceHover = ui.RGBA(255, 255, 255, 0.14)
	t.SurfacePressed = ui.RGBA(255, 255, 255, 0.19)
	t.Text = ui.Hex("#ffffff")
	t.TextMuted = ui.RGBA(255, 255, 255, 0.68)
	t.Border = ui.RGBA(255, 255, 255, 0.13)
	t.Accent = accent
	t.AccentHover = accent.Mix(ui.RGB(255, 255, 255), 0.12)
	t.AccentPressed = accent.Mix(ui.RGB(0, 0, 0), 0.12)
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#ff8f86")
	t.Selection = accent.Alpha(0.42)
	t.Focus = accent.Alpha(0.7)
	t.Scrollbar = ui.Transparent
	t.Radius = 12
	return &t
}

func (a *appState) secondaryHeader(c *ui.Context) {
	personPage := a.selectedPerson != nil
	titleBar := c.TitleBar()
	leftPadding := float32(24)
	if titleBar.Left > 0 {
		leftPadding = titleBar.Left + 8
	}
	rightPadding := float32(24)
	if titleBar.Right > 0 {
		rightPadding = titleBar.Right + 8
	}
	label := "电影详情"
	if personPage {
		label = "演职人员"
	} else if a.selected != nil && a.selected.IsSeries {
		label = "剧集详情"
	}
	ui.Row(c.Key("secondary-titlebar")).Height(movieTitleBarHeight).FillWidth().Padding(5, rightPadding, 5, leftPadding).
		Gap(10).AlignItems(ui.Center).DragWindow().Children(func() {
		if personPage {
			a.personBackButton(c)
		} else {
			a.detailBackButton(c)
		}
		ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Spacer(c)
	})
}

func (a *appState) personBackButton(c *ui.Context) {
	button := actionButton(c, "").Size(36, 34).Padding(0).Radius(9).BorderWidth(0).
		Background(ui.Color{}).TextColor(c.Theme().TextMuted).Label("返回")
	if button.Hovered() {
		button.Background(c.Theme().SurfaceHover)
	}
	button.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18) })
	if button.Clicked() {
		a.closePerson()
		a.resumeDetailLoading()
	}
}
