package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Sidebar and library icons share a unified monochrome appearance and inherit
// the item's selected/idle/hover color. Parse once rather than allocating an icon per list row.
var sidebarNavigationIcons = map[string]*ui.SVG{
	"home":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="m3 10 9-7 9 7M5 9v11h5v-6h4v6h5V9"/></svg>`)),
	"movies":    ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="5" width="16" height="14" rx="2"/><path d="M8 5v14M16 5v14M4 9h4M4 15h4M16 9h4M16 15h4"/></svg>`)),
	"tv":        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="13" rx="2"/><path d="M8 21h8M12 17v4M8 9h8M8 12h5"/></svg>`)),
	"favorites": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/></svg>`)),
	"library":   ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="8" width="16" height="12" rx="2"/><path d="M7 4h10M5.5 6h13"/><path d="m10 12 5 2-5 2z"/></svg>`)),
	"login":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/></svg>`)),
	"logout":    ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>`)),
}

var libraryNavigationIcons = map[string]*ui.SVG{
	"movie":   sidebarNavigationIcons["movies"],
	"tv":      sidebarNavigationIcons["tv"],
	"library": sidebarNavigationIcons["library"],
}

var homeNavigationIcon = sidebarNavigationIcons["home"]

func sidebarNavIcon(key string) *ui.SVG {
	return sidebarNavigationIcons[key]
}

func libraryNavigationIcon(library MediaLibrary) *ui.SVG {
	// Use the server's explicit category, never guesses based on library names.
	switch strings.ToLower(strings.TrimSpace(library.Kind)) {
	case "movie", "movies", "film":
		return sidebarNavigationIcons["movies"]
	case "tv", "series", "television", "show":
		return sidebarNavigationIcons["tv"]
	default:
		return sidebarNavigationIcons["library"]
	}
}
