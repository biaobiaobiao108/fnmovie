package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Library icons share the sidebar's monochrome appearance and inherit its
// selected/idle color. Parse once rather than allocating an icon per list row.
var libraryNavigationIcons = map[string]*ui.SVG{
	"movie":   ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="5" width="16" height="14" rx="2"/><path d="M8 5v14M16 5v14M4 9h4M4 15h4M16 9h4M16 15h4"/></svg>`)),
	"tv":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="13" rx="2"/><path d="M8 21h8M12 17v4M8 9h8M8 12h5"/></svg>`)),
	"library": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="8" width="16" height="12" rx="2"/><path d="M7 4h10M5.5 6h13"/><path d="m10 12 5 2-5 2z"/></svg>`)),
}

func libraryNavigationIcon(library MediaLibrary) *ui.SVG {
	// Use the server's explicit category, never guesses based on library names.
	switch strings.ToLower(strings.TrimSpace(library.Kind)) {
	case "movie", "movies", "film":
		return libraryNavigationIcons["movie"]
	case "tv", "series", "television", "show":
		return libraryNavigationIcons["tv"]
	default:
		return libraryNavigationIcons["library"]
	}
}
