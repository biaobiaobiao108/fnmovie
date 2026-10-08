package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const homeHeroFadeDuration = 180 * time.Millisecond

var homeDetailIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7v.1"/></svg>`))

func (a *appState) homeView(c *ui.Context) {
	if a.home.AllContinue {
		a.homeContinueGrid(c)
		return
	}
	advanceSmoothScroll(c, &a.home.Scroll, &a.home.ScrollAnimation)
	scroll := ui.Scroll(c).Key("home-scroll").Grow(1).FillHeight().FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(28).Padding(0, 0, 16, 0).Children(func() {
			a.homeCarousel(c)
			if len(a.home.Heroes) > 0 && a.home.HeroErr != "" {
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "部分推荐读取失败："+a.home.HeroErr).FontSize(13).TextColor(ui.Hex("#ad5148")).MaxLines(2).Grow(1)
					if actionButton(c, "重试").Key("home-hero-retry").Disabled(a.home.HeroLoading).Clicked() {
						a.loadHomeHeroes(true)
					}
				})
			}
			ui.Column(c).FillWidth().Gap(14).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, "继续观看").FontSize(22).Bold().Grow(1)
					if len(a.home.Continue) > 0 {
						if actionButton(c, fmt.Sprintf("全部 · %d ›", len(a.home.Continue))).Key("home-continue-all").BorderWidth(0).Background(ui.Transparent).TextColor(c.Theme().TextMuted).Clicked() {
							a.home.AllContinue = true
						}
					}
				})
				a.homeContinueStatus(c)
				if len(a.home.Continue) > 0 {
					ui.ScrollHorizontal(c).Key("home-continue-row").Height(224).TrackScroll(&a.home.RowScroll).Gap(18).Children(func() {
						for _, item := range a.home.Continue[:min(10, len(a.home.Continue))] {
							a.homeContinueCard(c, item, 272)
						}
					})
				}
			})
		})
	})
	bindSmoothScroll(c, scroll, &a.home.Scroll, &a.home.ScrollAnimation)
}

func (a *appState) homeCarousel(c *ui.Context) {
	width, _ := c.Size()
	hero := ui.Box(c).Key("home-carousel").FillWidth().Radius(18).Clip().Background(ui.Hex("#27352e"))
	contentWidth := hero.Bounds().W
	if contentWidth <= 0 {
		contentWidth = max(1, width-273)
	}
	height := max(float32(340), min(float32(560), contentWidth*9/16))
	hero.Height(height)
	a.home.HeroHover = hero.Hovered()
	if len(a.home.Heroes) == 0 {
		hero.Center().Children(func() {
			ui.Column(c).Gap(14).Center().MaxWidth(440).Children(func() {
				if a.home.HeroLoading {
					ui.Spinner(c).FontSize(20).TextColor(ui.Hex("#ffffff"))
					ui.Text(c, "正在挑选你的影视库…").FontSize(15).TextColor(ui.Hex("#ffffff"))
				} else {
					label := "影视库中还没有可展示的影片"
					if a.home.HeroErr != "" {
						label = a.home.HeroErr
					}
					ui.Text(c, label).FontSize(15).TextColor(ui.Hex("#ffffff")).MaxLines(3).TextAlign(ui.Center)
					if actionButton(c, "重新加载").Key("home-hero-retry").Clicked() {
						a.loadHomeHeroes(true)
					}
				}
			})
		})
		return
	}
	index := max(0, min(len(a.home.Heroes)-1, a.home.HeroIndex))
	item := a.home.Heroes[index]
	fade := float32(1)
	if !c.Preferences().ReduceMotion && !a.home.HeroChangedAt.IsZero() {
		fade = max(float32(0), min(float32(1), float32(c.Now().Sub(a.home.HeroChangedAt))/float32(homeHeroFadeDuration)))
	}
	hero.Children(func() {
		if fade < 1 && a.home.HeroPrevious >= 0 && a.home.HeroPrevious < len(a.home.Heroes) && a.home.HeroPrevious != index {
			previous := a.home.Heroes[a.home.HeroPrevious]
			if image := a.homeHeroImage(previous, int(contentWidth), int(height)); image != nil {
				ui.Image(c, image).Absolute().Top(0).Left(0).Fill().Fit(ui.Cover)
			}
			c.AnimationFrame()
		}
		if image := a.homeHeroImage(item, int(contentWidth), int(height)); image != nil {
			ui.Image(c, image).Absolute().Top(0).Left(0).Fill().Fit(ui.Cover).Opacity(fade)
		}
		ui.Box(c).Absolute().Top(0).Left(0).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, ui.LinearGradient{From: ui.RGBA(8, 14, 11, 0), To: ui.RGBA(8, 14, 11, 0.90), Angle: 180}, 0)
		})
		body := actionButton(c, "").Key("home-hero-detail").Absolute().Top(0).Left(0).Fill().Padding(0).BorderWidth(0).Background(ui.Transparent).Label("查看 " + item.Title + " 详情")
		body.Children(func() {
			ui.Column(c).Absolute().Bottom(50).Left(32).Right(32).Gap(12).Children(func() {
				if logo := a.imageForURL(item.Logo, 240, 88); logo != nil {
					ui.Image(c, logo).Size(240, 88).Fit(ui.Contain)
				} else {
					ui.Text(c, item.Title).FontSize(34).Bold().TextColor(ui.Hex("#ffffff")).MaxLines(2)
				}
				ui.Text(c, homeHeroMetadata(item)).FontSize(14).Bold().TextColor(ui.RGBA(255, 255, 255, 0.92)).MaxLines(2)
				if item.Overview != "" {
					ui.Text(c, item.Overview).MaxWidth(760).FontSize(14).TextColor(ui.RGBA(255, 255, 255, 0.86)).MaxLines(3)
				}
			})
		})
		if body.Clicked() {
			a.openDetail(item)
		}
		if len(a.home.Heroes) > 1 {
			for _, direction := range []int{-1, 1} {
				label, icon := "上一张海报", "back"
				if direction > 0 {
					label, icon = "下一张海报", "forward"
				}
				button := actionButton(c, "").Key(fmt.Sprintf("home-hero-arrow-%d", direction)).Absolute().Top(height/2-20).Size(40, 40).Padding(0).Radius(20).BorderWidth(0).Background(ui.RGBA(8, 14, 11, 0.48)).TextColor(ui.Hex("#ffffff")).Label(label)
				if direction < 0 {
					button.Left(16)
				} else {
					button.Right(16)
				}
				button.Children(func() {
					if icon == "back" {
						ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18)
					} else {
						ui.Text(c, "›").FontSize(28).TextColor(ui.Hex("#ffffff"))
					}
				})
				if button.Clicked() {
					a.selectHomeHero((index+direction+len(a.home.Heroes))%len(a.home.Heroes), c.Now())
				}
			}
		}
		ui.Row(c).Absolute().Bottom(18).Left(max(float32(0), (contentWidth-float32(len(a.home.Heroes)*22))/2)).Gap(6).Children(func() {
			for i := range a.home.Heroes {
				button := actionButton(c, "").Key(fmt.Sprintf("home-hero-dot-%d", i)).Size(16, 16).Padding(0).BorderWidth(0).Background(ui.Transparent).Label(fmt.Sprintf("显示第 %d 张海报", i+1))
				button.Children(func() {
					color, w := ui.RGBA(255, 255, 255, 0.40), float32(6)
					if i == index {
						color, w = ui.Hex("#ffffff"), 16
					}
					ui.Box(c).Size(w, 6).Radius(3).Background(color)
				})
				if button.Clicked() {
					a.selectHomeHero(i, c.Now())
				}
			}
		})
	})
	if fade >= 1 && len(a.home.Heroes) > 1 {
		a.homeHeroImage(a.home.Heroes[(index+1)%len(a.home.Heroes)], int(contentWidth), int(height))
	}
}

func homeHeroMetadata(item MediaItem) string {
	parts := make([]string, 0, 5)
	if item.Rating != "" {
		parts = append(parts, "★ "+item.Rating)
	}
	if item.Year != "" {
		parts = append(parts, item.Year)
	}
	if len(item.Countries) > 0 {
		parts = append(parts, strings.Join(item.Countries, " / "))
	}
	if len(item.Genres) > 0 {
		parts = append(parts, strings.Join(item.Genres, "、"))
	}
	return strings.Join(parts, "  ·  ")
}

func (a *appState) homeContinueStatus(c *ui.Context) {
	if a.home.ContinueLoading {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).FontSize(13)
			ui.Text(c, "正在读取观看记录…").FontSize(13).TextColor(c.Theme().TextMuted)
		})
	} else if a.home.ContinueErr != "" {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.home.ContinueErr).FontSize(13).TextColor(ui.Hex("#ad5148")).MaxLines(2).Grow(1)
			if actionButton(c, "重试").Key("home-continue-retry").Clicked() {
				a.loadContinueWatching(true)
			}
		})
	} else if len(a.home.Continue) == 0 {
		ui.Text(c, "还没有观看记录，挑一部喜欢的影片开始吧。").FontSize(14).TextColor(c.Theme().TextMuted).Padding(24, 0)
	}
}

func (a *appState) homeContinueCard(c *ui.Context, item ContinueWatchingItem, width float32) {
	media := item.Media
	height := width * 9 / 16
	loading := a.playbackLoading && a.playbackLoadingID == item.RecordGUID
	ui.Column(c).Key("home-continue:" + item.RecordGUID).Width(width).Gap(8).Children(func() {
		button := actionButton(c, "").Key("resume:"+item.RecordGUID).Size(width, height).Padding(0).BorderWidth(0).Radius(12).Clip().Background(ui.Hex("#dce1db")).Label("继续观看 " + media.Title)
		button.Children(func() {
			image := a.imageForURL(media.Backdrop, int(width), int(height))
			if image == nil {
				image = a.imageFor(media, int(width), int(height))
			}
			if image != nil {
				ui.Image(c, image).Size(width, height).Fit(ui.Cover)
			}
			ui.Box(c).Absolute().Bottom(0).Left(0).Width(width).Height(68).Draw(func(p *ui.Painter, r ui.Rect) {
				p.FillGradient(r, ui.LinearGradient{From: ui.RGBA(8, 14, 11, 0), To: ui.RGBA(8, 14, 11, 0.78), Angle: 180}, 0)
			})
			ui.Text(c, formatClock(item.Position)+" / "+formatClock(item.Duration)).Absolute().Bottom(24).Right(12).FontSize(12).Bold().TextColor(ui.Hex("#ffffff"))
			ui.Box(c).Absolute().Bottom(12).Left(12).Right(12).Height(4).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(r, ui.RGBA(255, 255, 255, 0.32), 2)
				progress := float32(0)
				if item.Duration > 0 && !math.IsNaN(item.Duration) && !math.IsNaN(item.Position) {
					progress = float32(max(0, min(1, item.Position/item.Duration)))
				}
				p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * progress, H: r.H}, ui.Hex("#ffffff"), 2)
			})
			if loading {
				ui.Box(c).Absolute().Top(height/2-17).Left(width/2-17).Size(34, 34).Center().Radius(17).Background(ui.RGBA(8, 14, 11, 0.5)).Children(func() { ui.Spinner(c).FontSize(20).TextColor(ui.Hex("#ffffff")) })
			} else if button.Hovered() {
				ui.Box(c).Absolute().Top(height/2-20).Left(width/2-20).Size(40, 40).Center().Radius(20).Background(ui.RGBA(8, 14, 11, 0.5)).Children(func() { ui.Icon(c, playerOverlayIcons["play"]).Size(20, 20).TextColor(ui.Hex("#ffffff")) })
			}
		})
		if button.Clicked() {
			if loading {
				a.cancelPlaybackLoading()
			} else {
				a.playContinue(item)
			}
		}
		ui.Row(c).Width(width).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, media.Title).FontSize(15).Bold().SingleLine()
				if subtitle := homeContinueSubtitle(media); subtitle != "" {
					ui.Text(c, subtitle).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine()
				}
			})
			detail := actionButton(c, "").Key("resume-detail:"+item.RecordGUID).Size(30, 30).Padding(0).BorderWidth(0).Background(ui.Transparent).TextColor(c.Theme().TextMuted).Label("查看 " + media.Title + " 详情")
			detail.Children(func() { ui.Icon(c, homeDetailIcon).Size(18, 18) })
			if detail.Clicked() {
				a.openContinueDetail(item)
			}
		})
	})
}

func homeContinueSubtitle(item MediaItem) string {
	parts := make([]string, 0, 3)
	if item.SeriesTitle != "" && item.SeriesTitle != item.Title {
		parts = append(parts, item.SeriesTitle)
	}
	if item.SeasonNumber > 0 {
		parts = append(parts, fmt.Sprintf("第 %d 季", item.SeasonNumber))
	}
	if item.EpisodeNumber > 0 {
		parts = append(parts, fmt.Sprintf("第 %d 集", item.EpisodeNumber))
	}
	if len(parts) == 0 {
		return item.Subtitle()
	}
	return strings.Join(parts, " · ")
}

func (a *appState) homeContinueGrid(c *ui.Context) {
	ui.Column(c).Grow(1).FillHeight().FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			back := actionButton(c, "").Key("home-continue-back").Size(34, 34).Padding(0).BorderWidth(0).Background(ui.Transparent).Label("返回首页")
			back.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18) })
			if back.Clicked() {
				a.home.AllContinue = false
			}
			ui.Text(c, fmt.Sprintf("继续观看 · %d", len(a.home.Continue))).FontSize(22).Bold()
		})
		a.homeContinueStatus(c)
		advanceSmoothScroll(c, &a.home.GridScroll, &a.home.GridAnimation)
		grid := ui.GridView(c, &a.home.Grid, len(a.home.Continue), 272, 224, func(i int) {
			if i < len(a.home.Continue) {
				a.homeContinueCard(c, a.home.Continue[i], 272)
			}
		}).Grow(1).FillHeight().FillWidth()
		bindSmoothScroll(c, grid, &a.home.GridScroll, &a.home.GridAnimation)
	})
}
