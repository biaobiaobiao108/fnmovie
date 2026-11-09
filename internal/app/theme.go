package app

import "github.com/egoist/mygo/ui"

const (
	movieWindowBackground    = "#0d0f12"
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
	sidebar:         ui.Hex("#13161b"),
	surface:         ui.Hex("#1a1e24"),
	surfaceHover:    ui.RGBA(255, 255, 255, 0.06),
	surfacePressed:  ui.RGBA(255, 255, 255, 0.10),
	text:            ui.Hex("#f0f2f5"),
	textMuted:       ui.RGBA(240, 242, 245, 0.60),
	border:          ui.RGBA(255, 255, 255, 0.08),
	accent:          ui.Hex("#5e85a8"),
	accentHover:     ui.Hex("#6f97bc"),
	accentPressed:   ui.Hex("#4f7394"),
	accentText:      ui.Hex("#ffffff"),
	selected:        ui.RGBA(94, 133, 168, 0.18),
	danger:          ui.Hex("#e05656"),
	favorite:        ui.Hex("#e0526b"),
	placeholder:     ui.Hex("#1a1e24"),
	placeholderText: ui.Hex("#4e5666"),
	rating:          ui.Hex("#2a2415"),
	ratingText:      ui.Hex("#e5a93c"),
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
	t.Warning = ui.Hex("#e5a93c")
	t.Success = movieColors.accent
	t.Selection = movieColors.accent.Alpha(0.32)
	t.Focus = movieColors.accent.Alpha(0.55)
	t.Scrollbar = ui.Transparent
	t.Radius = 8
	return t
}
