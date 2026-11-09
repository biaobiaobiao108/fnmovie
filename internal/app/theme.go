package app

import "github.com/egoist/mygo/ui"

const (
	movieWindowBackground    = "#f7f7f6"
	movieSidebarWidth        = 232
	movieMainHorizontalInset = 28
	movieTitleBarHeight      = 46
)

var movieColors = struct {
	background, sidebar, surface, surfaceHover, surfacePressed                   ui.Color
	text, textMuted, border, accent, accentHover, accentPressed, accentText      ui.Color
	selected, danger, favorite, placeholder, placeholderText, rating, ratingText ui.Color
}{
	background:      ui.Hex(movieWindowBackground),
	sidebar:         ui.Hex("#f0f1f3"),
	surface:         ui.Hex("#ffffff"),
	surfaceHover:    ui.RGBA(0, 0, 0, 0.04),
	surfacePressed:  ui.RGBA(0, 0, 0, 0.08),
	text:            ui.Hex("#191b1f"),
	textMuted:       ui.RGBA(25, 27, 31, 0.62),
	border:          ui.RGBA(0, 0, 0, 0.08),
	accent:          ui.Hex("#44627a"),
	accentHover:     ui.Hex("#375267"),
	accentPressed:   ui.Hex("#2d4456"),
	accentText:      ui.Hex("#ffffff"),
	selected:        ui.RGBA(68, 98, 122, 0.12),
	danger:          ui.Hex("#c0392b"),
	favorite:        ui.Hex("#d94f68"),
	placeholder:     ui.Hex("#ececee"),
	placeholderText: ui.Hex("#8e929a"),
	rating:          ui.Hex("#2a2415"),
	ratingText:      ui.Hex("#e5a93c"),
}

func movieTheme() *ui.Theme {
	t := ui.LightTheme()
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
	t.Warning = ui.Hex("#9c6d1a")
	t.Success = movieColors.accent
	t.Selection = movieColors.accent.Alpha(0.24)
	t.Focus = movieColors.accent.Alpha(0.48)
	t.Scrollbar = ui.Transparent
	t.Radius = 8
	return t
}
