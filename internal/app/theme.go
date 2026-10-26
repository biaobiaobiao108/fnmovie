package app

import "github.com/egoist/mygo/ui"

const (
	movieWindowBackground    = "#111311"
	movieSidebarWidth        = 220
	movieMainHorizontalInset = 28
)

var movieColors = struct {
	background, sidebar, surface, surfaceHover, surfacePressed                   ui.Color
	text, textMuted, border, accent, accentHover, accentPressed, accentText      ui.Color
	selected, danger, favorite, placeholder, placeholderText, rating, ratingText ui.Color
}{
	background:      ui.Hex(movieWindowBackground),
	sidebar:         ui.Hex("#171a18"),
	surface:         ui.Hex("#1d211e"),
	surfaceHover:    ui.Hex("#292e2a"),
	surfacePressed:  ui.Hex("#343a35"),
	text:            ui.Hex("#f1f4f1"),
	textMuted:       ui.Hex("#a2aaa4"),
	border:          ui.Hex("#303632"),
	accent:          ui.Hex("#83bd98"),
	accentHover:     ui.Hex("#9ad0aa"),
	accentPressed:   ui.Hex("#669879"),
	accentText:      ui.Hex("#101712"),
	selected:        ui.Hex("#26372c"),
	danger:          ui.Hex("#f18a7f"),
	favorite:        ui.Hex("#f27b91"),
	placeholder:     ui.Hex("#262b27"),
	placeholderText: ui.Hex("#7e8981"),
	rating:          ui.Hex("#d8b66e"),
	ratingText:      ui.Hex("#211d14"),
}

func movieTheme() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = movieColors.background
	t.Surface = movieColors.surface
	t.SurfaceHover = movieColors.surfaceHover
	t.SurfacePressed = movieColors.surfacePressed
	t.Text = movieColors.text
	t.TextMuted = movieColors.textMuted
	t.Border = movieColors.border
	t.Accent = movieColors.accent
	t.AccentHover = movieColors.accentHover
	t.AccentPressed = movieColors.accentPressed
	t.AccentText = movieColors.accentText
	t.Danger = movieColors.danger
	t.Warning = movieColors.rating
	t.Success = movieColors.accent
	t.Selection = movieColors.accent.Alpha(0.32)
	t.Focus = movieColors.accent.Alpha(0.55)
	t.Scrollbar = ui.Transparent
	t.Radius = 9
	return t
}
