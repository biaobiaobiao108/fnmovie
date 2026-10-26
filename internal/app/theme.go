package app

import "github.com/egoist/mygo/ui"

const (
	movieWindowBackground    = "#ffffff"
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
	sidebar:         ui.Hex("#f4f4f6"),
	surface:         ui.Hex("#f4f4f5"),
	surfaceHover:    ui.RGBA(0, 0, 0, 0.04),
	surfacePressed:  ui.RGBA(0, 0, 0, 0.08),
	text:            ui.Hex("#1d1d1f"),
	textMuted:       ui.RGBA(60, 60, 67, 0.62),
	border:          ui.RGBA(0, 0, 0, 0.07),
	accent:          ui.Oklch(0.6, 0.21, 24),
	accentHover:     ui.Oklch(0.55, 0.21, 24),
	accentPressed:   ui.Oklch(0.5, 0.21, 24),
	accentText:      ui.Hex("#ffffff"),
	selected:        ui.RGBA(0, 0, 0, 0.08),
	danger:          ui.Hex("#c0392b"),
	favorite:        ui.Hex("#d94f68"),
	placeholder:     ui.Hex("#e9e9ed"),
	placeholderText: ui.Hex("#8a8a91"),
	rating:          ui.Oklch(0.6, 0.21, 24),
	ratingText:      ui.Hex("#ffffff"),
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
	t.Warning = movieColors.rating
	t.Success = movieColors.accent
	t.Selection = movieColors.accent.Alpha(0.32)
	t.Focus = movieColors.accent.Alpha(0.55)
	t.Scrollbar = ui.Transparent
	t.Radius = 9
	return t
}
