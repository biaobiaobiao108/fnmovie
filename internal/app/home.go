package app

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type homeState struct {
	ViewportHandle, HeroHandle                                  ui.Handle
	Heroes                                                      []MediaItem
	HeroIndex, HeroPrevious                                     int
	HeroChangedAt                                               time.Time
	HeroHover, HeroLoading, HeroAttempted                       bool
	HeroErr                                                     string
	Continue                                                    []ContinueWatchingItem
	ContinueLoading                                             bool
	ContinueErr                                                 string
	ContinueUpdatedAt                                           time.Time
	AllContinue                                                 bool
	Scroll, GridScroll, RowScroll                               ui.ScrollState
	ScrollAnimation, GridAnimation                              smoothScroll
	Grid                                                        ui.GridState
	Generation, ContinueRequest, DetailRequest, TimerGeneration uint64
	HeroCancel, ContinueCancel, DetailCancel                    context.CancelFunc
	Timer                                                       *time.Timer
	LibrariesReady, Visible, Closed                             bool
}

func (a *appState) resetHome() {
	a.pauseHomeCarousel()
	for _, cancel := range []context.CancelFunc{a.home.HeroCancel, a.home.ContinueCancel, a.home.DetailCancel} {
		if cancel != nil {
			cancel()
		}
	}
	a.home = homeState{Generation: a.home.Generation + 1}
}

func (a *appState) loadHomeHeroes(retry bool) {
	if a.server == nil || a.window == nil || !a.loggedIn || !a.home.LibrariesReady || a.home.Closed || a.home.HeroLoading || a.home.HeroAttempted && !retry {
		return
	}
	if a.home.HeroCancel != nil {
		a.home.HeroCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.home.HeroCancel = cancel
	a.home.HeroLoading, a.home.HeroAttempted, a.home.HeroErr = true, true, ""
	server, generation := a.server, a.home.Generation
	libraries := append([]MediaLibrary(nil), a.libraries...)
	seed := time.Now().UnixNano()
	go func() {
		items, err := server.HomeHighlightsContext(ctx, libraries, seed)
		a.window.Update(func() {
			defer cancel()
			if ctx.Err() != nil || a.server != server || a.home.Generation != generation || a.home.Closed {
				return
			}
			a.home.HeroLoading, a.home.HeroCancel = false, nil
			if err != nil {
				a.home.HeroErr = err.Error()
			}
			// Retrying failed libraries must not reshuffle an existing session set.
			a.home.Heroes = mergeHomeHeroesStable(a.home.Heroes, items)
			a.syncHomeCarousel()
		})
	}()
}

func (a *appState) loadContinueWatching(force bool) {
	if a.server == nil || a.window == nil || !a.loggedIn || a.home.Closed {
		return
	}
	if !force && (a.home.ContinueLoading || !a.home.ContinueUpdatedAt.IsZero() && time.Since(a.home.ContinueUpdatedAt) < 30*time.Second) {
		return
	}
	if a.home.ContinueCancel != nil {
		a.home.ContinueCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.home.ContinueCancel = cancel
	a.home.ContinueRequest++
	a.home.ContinueLoading, a.home.ContinueErr = true, ""
	server, generation, request := a.server, a.home.Generation, a.home.ContinueRequest
	go func() {
		items, err := server.ContinueWatchingContext(ctx)
		a.window.Update(func() {
			defer cancel()
			if ctx.Err() != nil || a.server != server || a.home.Generation != generation || a.home.ContinueRequest != request || a.home.Closed {
				return
			}
			a.home.ContinueLoading, a.home.ContinueCancel = false, nil
			if err != nil {
				a.home.ContinueErr = err.Error()
				if len(items) > 0 {
					a.home.Continue, a.home.ContinueUpdatedAt = items, time.Now()
				}
				return
			}
			a.home.Continue, a.home.ContinueUpdatedAt = items, time.Now()
		})
	}()
}

func (a *appState) syncHomeLifecycle() {
	visible := a.window != nil && a.section == "home" && strings.TrimSpace(a.query) == "" && a.selected == nil && a.selectedPerson == nil && !a.playback.Active && !a.home.Closed && a.loggedIn
	if !visible && a.home.DetailCancel != nil {
		a.home.DetailCancel()
		a.home.DetailCancel = nil
	}
	if visible && !a.home.Visible {
		a.loadHomeHeroes(false)
		a.loadContinueWatching(false)
	}
	a.home.Visible = visible
	a.syncHomeCarousel()
}

func (a *appState) pauseHomeCarousel() {
	a.home.TimerGeneration++
	if a.home.Timer != nil {
		a.home.Timer.Stop()
		a.home.Timer = nil
	}
}

func (a *appState) homeCarouselEligible() bool {
	return a.section == "home" && strings.TrimSpace(a.query) == "" && a.selected == nil && a.selectedPerson == nil && !a.playback.Active && !a.playbackLoading && !a.home.AllContinue && !a.home.HeroHover && len(a.home.Heroes) > 1 && a.loggedIn && !a.home.Closed && a.window != nil && !a.window.IsMinimized()
}

func (a *appState) syncHomeCarousel() {
	if !a.homeCarouselEligible() {
		a.pauseHomeCarousel()
		return
	}
	if a.home.Timer != nil {
		return
	}
	generation, timerGeneration := a.home.Generation, a.home.TimerGeneration
	window := a.window
	a.home.Timer = time.AfterFunc(8*time.Second, func() {
		window.Update(func() {
			if generation != a.home.Generation || timerGeneration != a.home.TimerGeneration {
				return
			}
			a.home.Timer = nil
			if a.homeCarouselEligible() {
				a.selectHomeHero((a.home.HeroIndex+1)%len(a.home.Heroes), time.Now())
			}
		})
	})
}

func (a *appState) selectHomeHero(index int, now time.Time) {
	if index < 0 || index >= len(a.home.Heroes) || index == a.home.HeroIndex {
		return
	}
	a.home.HeroPrevious, a.home.HeroIndex, a.home.HeroChangedAt = a.home.HeroIndex, index, now
	a.pauseHomeCarousel()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *appState) playContinue(record ContinueWatchingItem) {
	item := record.Media
	item.ID = record.RecordGUID
	if item.ID == "" {
		return
	}
	a.startPlayback(item)
}

func (a *appState) openContinueDetail(record ContinueWatchingItem) {
	if a.server == nil || a.window == nil || a.home.Closed {
		return
	}
	if a.home.DetailCancel != nil {
		a.home.DetailCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.home.DetailCancel = cancel
	a.home.DetailRequest++
	server, generation, request := a.server, a.home.Generation, a.home.DetailRequest
	go func() {
		item, err := server.ContinueWatchingDetailContext(ctx, record)
		a.window.Update(func() {
			defer cancel()
			if ctx.Err() != nil || a.server != server || a.home.Generation != generation || a.home.DetailRequest != request || a.home.Closed || a.section != "home" || a.selected != nil || a.selectedPerson != nil || a.playback.Active {
				return
			}
			a.home.DetailCancel = nil
			if err != nil {
				a.home.ContinueErr = "详情读取失败：" + err.Error()
				return
			}
			a.openDetail(item)
		})
	}()
}

func homeHeroPixelSize(width, height int, scale float64) (int, int) {
	if scale <= 0 {
		scale = 1
	}
	w, h := float64(max(1, width))*scale, float64(max(1, height))*scale
	fit := min(1.0, min(1600/w, 900/h))
	return max(1, int(math.Ceil(w*fit))), max(1, int(math.Ceil(h*fit)))
}

func (a *appState) homeHeroImage(item MediaItem, width, height int) *ui.Bitmap {
	if a.server == nil || a.posters == nil {
		return nil
	}
	w, h := homeHeroPixelSize(width, height, a.displayScale)
	onLoaded := func() {
		if a.window != nil {
			a.window.Update(func() { a.window.Invalidate() })
		}
	}
	if item.Backdrop != "" {
		remote := a.server.imageURL(item.Backdrop)
		if bitmap := a.posters.GetOrRequest(a.server, remote, w, h, onLoaded); bitmap != nil {
			return bitmap
		}
		if !a.posters.HasFailed(remote, w, h) {
			return nil
		}
	}
	if item.Poster == "" {
		return nil
	}
	return a.posters.GetOrRequest(a.server, a.server.imageURL(item.Poster), w, h, onLoaded)
}

func mergeHomeHeroesStable(existing, incoming []MediaItem) []MediaItem {
	seen := make(map[string]int)
	result := make([]MediaItem, 0, homeHighlightLimit)
	for _, group := range [][]MediaItem{existing, incoming} {
		for _, item := range group {
			if item.ID == "" {
				continue
			}
			if index, ok := seen[item.ID]; ok {
				result[index] = mergeMediaDetail(result[index], item)
			} else if len(result) < homeHighlightLimit {
				seen[item.ID] = len(result)
				result = append(result, item)
			}
		}
	}
	return result
}
