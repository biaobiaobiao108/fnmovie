package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *appState) playerDanmakuPanel(c *ui.Context) {
	if !a.danmaku.settingsInit {
		a.prepareDanmakuPanel()
	}
	ui.Column(c).FillWidth().Grow(1).MinWidth(0).Gap(6).Children(func() {
		enabled := !a.settings.Danmaku.Disabled
		toggleLabel := "关闭弹幕"
		if !enabled {
			toggleLabel = "开启弹幕"
		}
		ui.Row(c).Height(30).AlignItems(ui.Center).Gap(8).Children(func() {
			status := a.danmaku.status
			if status == "" {
				status = "播放时自动匹配，也可搜索选集"
			}
			ui.Text(c, status).Grow(1).MinWidth(0).MaxLines(2).FontSize(10.5).TextColor(ui.RGBA(255, 255, 255, 0.62))
			ui.Button(c.Key("danmaku-toggle"), toggleLabel).Height(28).Padding(0, 10).Radius(8).
				Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGBA(255, 255, 255, 0.90)).
				OnClick(a.toggleDanmakuEnabled)
			playerOverlayIconButton(c, "close", "关闭弹幕设置", 28, 15).OnClick(a.closePlayerOverlayMenu)
		})

		ui.Tabs(c, &a.danmaku.panelTab, "播放设置", "搜索选集")
		ui.Scroll(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
			if a.danmaku.panelTab == 1 {
				a.playerDanmakuSearchView(c)
			} else {
				a.playerDanmakuSettingsView(c)
			}
		})
	})
}

func (a *appState) playerDanmakuSettingsView(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		a.danmakuSliderRow(c, "danmaku-font", "字体大小", &a.danmaku.fontScale, 0.6, 2.2, func(value float64) string {
			return fmt.Sprintf("%.1f×", value)
		})
		a.danmakuSliderRow(c, "danmaku-opacity", "不透明度", &a.danmaku.opacity, 20, 100, func(value float64) string {
			return fmt.Sprintf("%.0f%%", value)
		})
		a.danmakuSliderRow(c, "danmaku-speed", "滚动时长", &a.danmaku.speed, 4, 20, func(value float64) string {
			return fmt.Sprintf("%.0f 秒", value)
		})
		a.danmakuSliderRow(c, "danmaku-area", "覆盖范围", &a.danmaku.area, 10, 90, func(value float64) string {
			return fmt.Sprintf("%.0f%%", value)
		})
		a.danmakuSliderRow(c, "danmaku-offset", "时间偏移", &a.danmaku.offset, -30, 30, func(value float64) string {
			return fmt.Sprintf("%+.0f 秒", value)
		})

		ui.Row(c).Height(30).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "显示位置").Width(74).Shrink(0).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.60))
			for _, anchor := range []struct {
				id    string
				label string
			}{{"top", "顶部"}, {"middle", "居中"}, {"bottom", "底部"}} {
				anchor := anchor
				a.danmakuChoiceButton(c, "danmaku-anchor-"+anchor.id, anchor.label, a.settings.Danmaku.Anchor == anchor.id, func() {
					a.setDanmakuAnchor(anchor.id)
				})
			}
		})

		ui.Row(c).Height(30).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "显示类型").Width(74).Shrink(0).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.60))
			ui.Checkbox(c.Key("danmaku-scroll"), &a.danmaku.showScroll, "滚动").OnChange(a.applyDanmakuPanelSettings)
			ui.Checkbox(c.Key("danmaku-top"), &a.danmaku.showTop, "顶部").OnChange(a.applyDanmakuPanelSettings)
			ui.Checkbox(c.Key("danmaku-bottom"), &a.danmaku.showBottom, "底部").OnChange(a.applyDanmakuPanelSettings)
		})
		ui.Row(c).Height(30).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "弹幕样式").Width(74).Shrink(0).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.60))
			ui.Checkbox(c.Key("danmaku-outline"), &a.danmaku.outline, "描边").OnChange(a.applyDanmakuPanelSettings)
			ui.Text(c, fmt.Sprintf("%d 条已缓存", len(a.danmaku.comments))).FontSize(10.5).TextColor(ui.RGBA(255, 255, 255, 0.46))
		})

		ui.Text(c, "弹幕服务器").FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.60))
		ui.Row(c).Height(34).AlignItems(ui.Center).Gap(6).Children(func() {
			input := ui.TextInput(c.Key("danmaku-server"), &a.danmaku.serverURL).Grow(1).MinWidth(0).
				FontSize(11.5).Placeholder("http://NAS:9321").Label("弹幕服务器")
			submitted := input.Submitted()
			ui.Button(c.Key("danmaku-server-apply"), "应用").Height(32).Padding(0, 14).Radius(8).
				Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGBA(255, 255, 255, 0.90)).
				OnClick(func() { a.applyDanmakuServerSettings(true) })
			if submitted {
				a.applyDanmakuServerSettings(true)
			}
		})
		ui.Row(c).Height(34).AlignItems(ui.Center).Gap(6).Children(func() {
			tokenInput := ui.TextInput(c.Key("danmaku-token"), &a.danmaku.token).Grow(1).MinWidth(0).
				FontSize(11.5).Placeholder("访问令牌（可选，danmu_api 的 TOKEN）").Label("弹幕访问令牌")
			if tokenInput.Submitted() {
				a.applyDanmakuServerSettings(true)
			}
			ui.Button(c.Key("danmaku-token-apply"), "应用").Height(32).Padding(0, 14).Radius(8).
				Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGBA(255, 255, 255, 0.90)).
				OnClick(func() { a.applyDanmakuServerSettings(true) })
		})
		ui.Text(c, "部分 danmu_api 实例需要访问令牌；留空表示无需令牌。").FontSize(10).TextColor(ui.RGBA(255, 255, 255, 0.42))
		ui.Button(c.Key("danmaku-rematch"), "重新匹配当前影片").Height(30).FillWidth().Radius(8).
			Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(255, 255, 255, 0.86)).
			OnClick(a.retryDanmaku)
	})
}

func (a *appState) playerDanmakuSearchView(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Row(c).Height(34).AlignItems(ui.Center).Gap(6).Children(func() {
			input := ui.TextInput(c.Key("danmaku-search"), &a.danmaku.searchQuery).Grow(1).MinWidth(0).
				FontSize(11.5).Placeholder("搜索番剧名称").Label("搜索番剧")
			submitted := input.Submitted()
			ui.Button(c.Key("danmaku-search-button"), "搜索").Height(32).Padding(0, 14).Radius(8).
				Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGBA(255, 255, 255, 0.90)).
				OnClick(a.danmakuSearch)
			if submitted {
				a.danmakuSearch()
			}
		})
		ui.Button(c.Key("danmaku-auto-match"), "自动匹配当前影片").Height(30).FillWidth().Radius(8).
			Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(255, 255, 255, 0.86)).
			OnClick(a.retryDanmaku)

		if a.danmaku.searchLoading {
			ui.Row(c).Height(26).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, "正在搜索…").FontSize(10.5).TextColor(ui.RGBA(255, 255, 255, 0.62))
			})
		}
		if a.danmaku.searchError != "" {
			ui.Text(c, a.danmaku.searchError).MaxLines(2).FontSize(10.5).TextColor(playerOverlayAccent())
		}
		if len(a.danmaku.searchResults) > 0 {
			ui.Text(c, "搜索结果").FontSize(10.5).Bold().TextColor(ui.RGBA(255, 255, 255, 0.58))
			for index, anime := range a.danmaku.searchResults {
				if index >= 8 {
					break
				}
				anime := anime
				label := anime.Title
				if anime.EpisodeCount > 0 {
					label += fmt.Sprintf("  (%d 集)", anime.EpisodeCount)
				}
				a.danmakuListItem(c, fmt.Sprintf("danmaku-anime-%d", index), label, false, func() {
					a.danmakuLoadEpisodes(anime)
				})
			}
		}
		if a.danmaku.episodesLoading {
			ui.Row(c).Height(26).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, "正在读取剧集…").FontSize(10.5).TextColor(ui.RGBA(255, 255, 255, 0.62))
			})
		}
		if a.danmaku.episodesError != "" {
			ui.Text(c, a.danmaku.episodesError).MaxLines(2).FontSize(10.5).TextColor(playerOverlayAccent())
		}
		if len(a.danmaku.episodes) > 0 {
			title := strings.TrimSpace(a.danmaku.selectedAnime.Title)
			if title != "" {
				title += " · 选择剧集"
			} else {
				title = "选择剧集"
			}
			ui.Text(c, title).FontSize(10.5).Bold().TextColor(ui.RGBA(255, 255, 255, 0.58))
			for index, episode := range a.danmaku.episodes {
				if index >= 30 {
					break
				}
				episode := episode
				label := episode.Title
				if episode.Number > 0 {
					label = fmt.Sprintf("第 %d 集", episode.Number)
					if episode.Title != "" {
						label += "  " + episode.Title
					}
				}
				a.danmakuListItem(c, fmt.Sprintf("danmaku-episode-%d", index), label, false, func() {
					a.danmakuSelectEpisode(episode)
				})
			}
		}
	})
}

func (a *appState) danmakuSliderRow(c *ui.Context, key, label string, value *float64, lo, hi float64, format func(float64) string) {
	ui.Row(c).Height(30).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, label).Width(74).Shrink(0).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.60))
		slider := ui.Slider(c.Key(key), value, lo, hi).Grow(1).Label(label)
		ui.Text(c, format(*value)).Width(58).Shrink(0).TextAlign(ui.End).FontSize(10.5).
			TextColor(ui.RGBA(255, 255, 255, 0.74)).SingleLine()
		slider.OnChange(a.applyDanmakuPanelSettings)
	})
}

func (a *appState) danmakuChoiceButton(c *ui.Context, key, label string, active bool, onClick func()) {
	button := ui.Button(c.Key(key), label).Height(28).Padding(0, 12).Radius(7)
	if active {
		button.Background(playerOverlayAccent().Alpha(0.24)).TextColor(playerOverlayAccent())
	} else {
		button.Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(255, 255, 255, 0.86))
	}
	button.OnClick(onClick)
}

func (a *appState) danmakuListItem(c *ui.Context, key, label string, selected bool, onClick func()) {
	button := ui.Button(c.Key(key), "").Height(30).Padding(5, 8).Radius(7).BorderWidth(0).
		Background(ui.RGBA(0, 0, 0, 0)).FillWidth().MinWidth(0).Clip().Label(label)
	if selected {
		button.Background(playerOverlayAccent().Alpha(0.24))
	} else if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.10))
	}
	button.Children(func() {
		textColor := ui.RGBA(255, 255, 255, 0.88)
		if selected {
			textColor = playerOverlayAccent()
		}
		ui.Text(c, label).FontSize(11).TextColor(textColor).Grow(1).MinWidth(0).SingleLine()
	})
	button.OnClick(onClick)
}
