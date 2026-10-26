package app

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

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

func (a *appState) imageForURL(remoteURL string, width, height int) *ui.Bitmap {
	if a.server == nil || a.posters == nil || strings.TrimSpace(remoteURL) == "" {
		return nil
	}
	scale := a.displayScale
	if scale <= 0 {
		scale = 1
		if a.window != nil {
			scale = windowScale(a.window.NativeHandle())
		}
	}
	w, h := max(1, int(math.Ceil(float64(width)*scale))), max(1, int(math.Ceil(float64(height)*scale)))
	window := a.window
	return a.posters.GetOrRequest(a.server, a.server.imageURL(remoteURL), w, h, func() {
		if window != nil {
			window.Update(func() { window.Invalidate() })
		}
	})
}

func (a *appState) imageForURLWithTint(remoteURL string, width, height int) (*ui.Bitmap, ui.Color) {
	bitmap := a.imageForURL(remoteURL, width, height)
	if a.server == nil || a.posters == nil || strings.TrimSpace(remoteURL) == "" {
		return bitmap, movieColors.accent
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
	if tint, ok := a.posters.Tint(a.server.imageURL(remoteURL), pixelWidth, pixelHeight); ok {
		return bitmap, tint
	}
	return bitmap, movieColors.accent
}
