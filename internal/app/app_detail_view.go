package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *appState) openDetail(item MediaItem) {
	a.cancelDetailRequests()
	a.detailRequest++
	detailRequest := a.detailRequest
	a.closePerson()
	serverURL := a.settings.ServerURL
	username := a.settings.Username
	cachedDetail := false
	if a.server != nil && a.catalogCache != nil {
		if cached, ok := a.catalogCache.Detail(serverURL, item.ID, username); ok {
			item = cached
			cachedDetail = true
		}
	}
	a.selected = &item
	a.detailList = ui.ListState{}
	a.detailScroll = ui.ScrollState{}
	a.detailSynopsisScroll = ui.ScrollState{}
	a.detailSynopsisOpen = false
	a.seriesLoading, a.seriesError, a.selectedSeasonID = false, "", ""
	a.castLoading, a.castError = len(item.Cast) == 0, ""
	a.seriesEpisodes, a.seriesEpisodeCache = nil, nil
	a.seriesEpisodeLoading, a.seriesEpisodeError = false, ""
	a.seriesEpisodeRequest++
	a.seriesRootCast = append([]CastMember(nil), item.Cast...)
	a.seriesRootCastError = ""
	a.seriesRootCastLoading = item.IsSeries
	a.seriesCastRequest++
	if item.IsSeries && cachedDetail && len(item.Seasons) > 0 {
		a.selectedSeasonID = item.Seasons[0].ID
		a.loadSeriesSeason(a.selectedSeasonID, false)
	}
	if a.server == nil {
		a.castLoading = false
		return
	}
	a.loadDetailParts(item, detailRequest, serverURL, username)
}

func (a *appState) detailView(c *ui.Context, item MediaItem) {
	if item.IsSeries {
		a.seriesDetailView(c, item)
		return
	}
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Gap(16).Children(func() {
		a.detailHeading(c, item)
		ui.Row(c).Grow(1).FillHeight().FillWidth().MaxWidth(1320).AlignSelf(ui.Center).AlignItems(ui.Center).Gap(32).Children(func() {
			a.detailPosterActions(c, item)
			scroll := ui.Scroll(c.Key("movie-detail:" + item.ID)).Grow(1).AlignSelf(ui.Stretch).FillHeight().Children(func() {
				ui.Column(c).Padding(detailContentInset(c), 0, 20, 0).Gap(17).Children(func() {
					_, posterHeight := detailPosterSize(c)
					ui.Column(c).Height(float32(posterHeight)).Gap(detailCastGap(c)).Children(func() {
						a.detailIntro(c, item)
						ui.Column(c).Height(200).Label("详情演职人员").Children(func() { a.castSection(c, item) })
					})
					if len(item.Sources) > 0 {
						ui.Text(c, "可播放版本").FontSize(15).Bold().TextColor(t.TextMuted)
						for _, source := range item.Sources {
							ui.Row(c).Gap(10).Padding(10, 12).Radius(8).Background(t.Surface).Children(func() {
								ui.Text(c, source.Name).Grow(1)
								ui.Text(c, source.Quality).TextColor(t.TextMuted)
								if a.playbackLoading && a.playbackLoadingID == item.ID {
									ui.Spinner(c).FontSize(12)
								} else if actionButton(c, "播放").Clicked() {
									a.startPlayback(item)
								}
							})
						}
					}
				})
			})
			bindSmoothScroll(scroll, &a.detailScroll)
			bindScrollEdgeFades(scroll, &a.detailScroll, t.Background)
		})
	})
}

func (a *appState) seriesDetailView(c *ui.Context, item MediaItem) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Gap(16).Children(func() {
		a.detailHeading(c, item)
		ui.Row(c).Grow(1).FillHeight().FillWidth().MaxWidth(1320).AlignSelf(ui.Center).AlignItems(ui.Center).Gap(32).Children(func() {
			a.detailPosterActions(c, item)
			episodes := a.seriesEpisodes
			showEpisodes := !a.seriesEpisodeLoading && a.seriesEpisodeError == "" && len(episodes) > 0
			rows := 4
			if showEpisodes {
				rows = 3 + len(episodes)
			}
			a.detailList.Key = func(i int) any {
				if i < 3 {
					return []string{"overview", "cast", "seasons"}[i]
				}
				if showEpisodes {
					return "episode:" + episodes[i-3].ID
				}
				return "episode-status"
			}
			list := ui.List(c, &a.detailList, rows, func(i int) {
				switch i {
				case 0:
					a.detailIntro(c, item)
				case 1:
					gap := max(float32(0), detailCastGap(c)-2) // List supplies the other 2 DIP.
					ui.Column(c).Height(200+gap).Padding(gap, 0, 0, 0).Label("详情演职人员").Children(func() { a.castSection(c, item) })
				case 2:
					ui.Column(c).Gap(14).Padding(16, 0, 12, 0).Children(func() {
						if a.seriesLoading {
							ui.Text(c, "正在读取季度与剧集…").FontSize(12).TextColor(t.TextMuted)
						} else if a.seriesError != "" {
							ui.Row(c).Gap(8).Children(func() {
								ui.Text(c, a.seriesError).FontSize(11).TextColor(ui.Hex("#ad5148"))
								if actionButton(c, "重试").Clicked() {
									a.openDetail(item)
								}
							})
						}
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "剧集").FontSize(14).Bold()
							if !a.seriesEpisodeLoading && len(a.seriesEpisodes) > 0 {
								ui.Text(c, fmt.Sprintf("%d 集", len(a.seriesEpisodes))).FontSize(11).TextColor(t.TextMuted)
							}
						})
						ui.ScrollHorizontal(c).Height(42).Gap(8).Children(func() {
							for _, season := range item.Seasons {
								label := season.Title
								if strings.TrimSpace(label) == "" {
									label = fmt.Sprintf("第%d季", season.Number)
								}
								button := actionButton(c, label).Padding(7, 11).BorderWidth(0).Radius(8)
								if a.selectedSeasonID == season.ID {
									button.Background(t.Accent).TextColor(t.AccentText)
								} else {
									button.Background(ui.Hex("#eeece6")).TextColor(t.TextMuted)
								}
								if button.Clicked() {
									a.selectSeriesSeason(season.ID)
								}
							}
						})
					})
				default:
					if !showEpisodes {
						ui.Column(c).Children(func() {
							if a.seriesEpisodeLoading {
								ui.Text(c, "正在读取本季集数…").Padding(12, 10).FontSize(12).TextColor(t.TextMuted)
								return
							}
							if a.seriesEpisodeError != "" {
								ui.Row(c).Padding(12, 10).Gap(8).Children(func() {
									ui.Text(c, a.seriesEpisodeError).Grow(1).FontSize(11).TextColor(ui.Hex("#ad5148"))
									if actionButton(c, "重试").Clicked() {
										a.loadSeriesSeason(a.selectedSeasonID, true)
									}
								})
								return
							}
							if len(a.seriesEpisodes) == 0 {
								ui.Text(c, "这个季度还没有集数").Padding(12, 10).FontSize(12).TextColor(t.TextMuted)
								return
							}
						})
						return
					}
					episode := episodes[i-3]
					row := actionButton(c.Key(episode.ID), "").FillWidth().Padding(10, 8).Gap(12).AlignItems(ui.Center).
						BorderWidth(0).Radius(8).Background(ui.Transparent).TextColor(t.Text).Label("播放 " + episode.Title).Disabled(a.playbackLoading)
					if row.Hovered() {
						row.Background(ui.Hex("#eeece6"))
					}
					row.Children(func() {
						number := ""
						if episode.EpisodeNumber > 0 {
							number = fmt.Sprintf("%02d", episode.EpisodeNumber)
						}
						ui.Text(c, number).Width(30).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, episode.Title).FontSize(13).Bold().SingleLine()
							if overview := episodeOverview(episode); overview != "" {
								ui.Text(c, overview).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
							}
						})
						if a.playbackLoading && a.playbackLoadingID == episode.ID {
							ui.Spinner(c).FontSize(12)
						} else {
							ui.Icon(c, playerOverlayIcons["play"]).Size(18, 18).TextColor(t.Accent)
						}
					})
					if row.Clicked() {
						a.startPlayback(episode)
					}
				}
			}).Grow(1).AlignSelf(ui.Stretch).FillHeight().Gap(2).Padding(detailContentInset(c), 0, 20, 0)
			bindSmoothScroll(list, &a.detailScroll)
			bindScrollEdgeFades(list, &a.detailScroll, t.Background)
		})
	})
}

func (a *appState) detailPosterActions(c *ui.Context, item MediaItem) {
	width, height := detailPosterSize(c)
	ui.Column(c).Width(float32(width)).Gap(12).Children(func() {
		if poster := a.imageFor(item, width, height); poster != nil {
			ui.Image(c, poster).Size(float32(width), float32(height)).Fit(ui.Cover).Radius(13).Label("影片海报")
		} else {
			ui.Box(c).Size(float32(width), float32(height)).Radius(13).Background(ui.Hex("#e8e8e2")).Label("影片海报")
		}
		ui.Row(c).Height(42).FillWidth().Justify(ui.Center).Gap(13).AlignItems(ui.Center).Children(func() {
			playItem, label := item, "▶  立即播放"
			available := true
			if item.IsSeries {
				label = "▶  播放本季首集"
				available = len(a.seriesEpisodes) > 0
				if available {
					playItem = a.seriesEpisodes[0]
				}
			}
			loading := a.playbackLoading && a.playbackLoadingID == playItem.ID
			if loading {
				button := detailPlayButton(c, "").Label("取消准备播放")
				button.Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).Size(12, 12)
						ui.Text(c, "正在准备播放…").FontSize(12)
					})
				})
				if button.Clicked() {
					a.cancelPlaybackLoading()
				}
			} else if detailPlayButton(c, label).Disabled(!available).Clicked() {
				a.startPlayback(playItem)
			}
			a.favoriteButton(c, item)
		})
	})
}

// Shared fixed slots keep playback and credits anchored as metadata arrives.
func (a *appState) detailIntro(c *ui.Context, item MediaItem) {
	lines := detailSynopsisLines(c)
	ui.Column(c).Children(func() {
		ui.Text(c, item.Title).FillWidth().Height(72).FixedLineHeight(36).FontSize(26).Bold().MaxLines(2)
		ui.Box(c).Height(24)
		ui.Text(c, item.Subtitle()).Height(20).FontSize(14).TextColor(c.Theme().TextMuted).SingleLine()
		ui.Box(c).Height(12)
		ui.Column(c.Key("detail-synopsis:" + item.ID)).Height(float32(lines*22 + 22)).FillWidth().Label("影片简介").Children(func() {
			text := item.Overview
			if strings.TrimSpace(text) == "" {
				text = "暂无简介"
			}
			ui.Text(c, text).Height(float32(lines * 22)).MaxWidth(860).FontSize(15).FixedLineHeight(22).MaxLines(lines).TextColor(c.Theme().TextMuted)
			if strings.TrimSpace(item.Overview) != "" {
				if actionButton(c, "展开简介").AlignSelf(ui.Start).Height(22).Padding(0).BorderWidth(0).Background(ui.Transparent).FontSize(12).TextColor(c.Theme().Accent).Clicked() {
					a.detailSynopsisOpen = true
					a.detailSynopsisScroll = ui.ScrollState{}
				}
			}
		})
	})
}

func (a *appState) detailSynopsisDialog(c *ui.Context) {
	if a.selected == nil || a.playback.Active {
		a.detailSynopsisOpen = false
		return
	}
	item := *a.selected
	width, height := c.Size()
	ui.Modal(c, &a.detailSynopsisOpen, func() {
		ui.Column(c).Width(min(float32(760), width-64)).Padding(24).Gap(16).Radius(12).Background(c.Theme().Surface).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, item.Title).Grow(1).FontSize(20).Bold().SingleLine()
				if actionButton(c, "收起简介").FontSize(12).Clicked() {
					a.detailSynopsisOpen = false
				}
			})
			scroll := ui.Scroll(c).Height(min(float32(440), height-180)).FillWidth().Children(func() {
				ui.Text(c, item.Overview).FontSize(15).FixedLineHeight(22).TextColor(c.Theme().TextMuted)
			})
			bindSmoothScroll(scroll, &a.detailSynopsisScroll)
		})
	})
}

func (a *appState) detailHeading(c *ui.Context, item MediaItem) {
	ui.Row(c).FillWidth().MaxWidth(1320).AlignSelf(ui.Center).Height(44).Gap(12).AlignItems(ui.Center).Children(func() {
		a.detailBackButton(c)
	})
}

func (a *appState) detailBackButton(c *ui.Context) {
	button := actionButton(c, "").Size(36, 34).Padding(0).Radius(9).BorderWidth(0).
		Background(ui.Color{}).TextColor(c.Theme().TextMuted).Label("返回影视库")
	if button.Hovered() {
		button.Background(ui.Hex("#e9ece6"))
	}
	button.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18) })
	if button.Clicked() {
		if a.playbackLoading {
			a.cancelPlaybackLoading()
		}
		a.closeDetail()
		a.castLoading = false
		a.seriesLoading = false
	}
}

func (a *appState) favoriteButton(c *ui.Context, item MediaItem) {
	pending := a.favoritePending[item.ID]
	if a.selectedPerson != nil && a.selectedPerson.ID == item.ID && a.personFavoriteLoading {
		pending = true
	}
	icon := "♡"
	color := ui.Hex("#59645e")
	if item.Favorite {
		icon, color = "♥", ui.Hex("#df5b79")
	}
	button := actionButton(c, "").Size(42, 42).Padding(0).Radius(21).BorderWidth(0).
		Background(ui.Color{}).TextColor(color).Label(map[bool]string{true: "取消收藏", false: "收藏"}[item.Favorite]).Disabled(pending)
	button.Children(func() { ui.Text(c, icon).FontSize(25).TextColor(color) })
	if button.Clicked() {
		a.toggleFavorite(item)
	}
}

func (a *appState) castSection(c *ui.Context, item MediaItem) {
	t := c.Theme()
	ui.Column(c).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "演职人员").FontSize(16).Bold()
			if a.castLoading {
				ui.Text(c, "正在读取…").FontSize(10).TextColor(t.TextMuted)
			}
		})
		if len(item.Cast) == 0 {
			if a.castError != "" {
				ui.Row(c).Gap(10).Children(func() {
					ui.Text(c, a.castError).FontSize(11).TextColor(t.TextMuted).MaxLines(2)
					if actionButton(c, "重试").Clicked() {
						a.openDetail(item)
					}
				})
			} else if !a.castLoading {
				ui.Text(c, "服务器暂未提供演职人员信息").FontSize(11).TextColor(t.TextMuted)
			}
			return
		}
		ui.ScrollHorizontal(c).Height(164).Gap(16).Children(func() {
			for _, person := range item.Cast {
				p := person
				btn := actionButton(c, "").Width(120).Padding(8, 8).Radius(12).BorderWidth(0).
					Background(ui.Color{}).Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond}).
					Label("查看 " + p.Name + " 的作品")
				if btn.Hovered() {
					btn.Background(ui.Hex("#eeece6"))
				}
				btn.Children(func() {
					ui.Column(c).AlignItems(ui.Center).Gap(8).Children(func() {
						if image := a.imageForURL(p.Profile, 84, 84); image != nil {
							ui.Image(c, image).Size(84, 84).Fit(ui.Cover).Radius(42)
						} else {
							ui.Box(c).Size(84, 84).Radius(42).Background(ui.Hex("#e5e3dc")).Center().Children(func() {
								nameRune := "•"
								if runes := []rune(p.Name); len(runes) > 0 {
									nameRune = string(runes[:1])
								}
								ui.Text(c, nameRune).FontSize(26).Bold().TextColor(t.TextMuted)
							})
						}
						ui.Text(c, p.Name).FontSize(15).Bold().TextAlign(ui.Center).SingleLine()
						role := p.Role
						if role == "" {
							role = p.Job
						}
						ui.Text(c, role).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).SingleLine()
					})
				})
				if btn.Clicked() {
					a.openPerson(p)
				}
			}
		})
	})
}

func (a *appState) personView(c *ui.Context, person CastMember) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Gap(16).Children(func() {
		// 顶部导航栏
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			backBtn := actionButton(c, "").Size(36, 34).Padding(0).Radius(9).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Label("返回")
			if backBtn.Hovered() {
				backBtn.Background(ui.Hex("#e9ece6"))
			}
			backBtn.Children(func() { ui.Icon(c, playerOverlayIcons["back"]).Size(18, 18) })
			if backBtn.Clicked() {
				a.closePerson()
				a.resumeDetailLoading()
			}
			ui.Text(c, "演职人员").FontSize(13).TextColor(t.TextMuted)
			ui.Text(c, "/").FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, person.Name).FontSize(13).Bold()
		})

		// Compact identity on the page background; no stretched card backplate.
		ui.Row(c).Gap(20).AlignItems(ui.Center).Padding(8, 0, 16, 0).Children(func() {
			if image := a.imageForURL(person.Profile, 84, 84); image != nil {
				ui.Image(c, image).Size(84, 84).Fit(ui.Cover).Radius(42)
			} else {
				ui.Box(c).Size(84, 84).Radius(42).Background(ui.Hex("#e5e3dc")).Center().Children(func() {
					nameRune := "人"
					if runes := []rune(person.Name); len(runes) > 0 {
						nameRune = string(runes[:1])
					}
					ui.Text(c, nameRune).FontSize(24).Bold().TextColor(t.TextMuted)
				})
			}
			ui.Column(c).Gap(6).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					a.favoriteButton(c, MediaItem{ID: person.ID, Title: person.Name, Kind: "person", Favorite: person.Favorite})
					ui.Text(c, person.Name).FontSize(22).Bold()
				})
				roleDesc := person.Role
				if roleDesc != "" {
					roleDesc = "代表角色：" + roleDesc
				} else if person.Job != "" {
					roleDesc = person.Job
				}
				if roleDesc != "" {
					ui.Text(c, roleDesc).FontSize(13).TextColor(t.TextMuted)
				}
				countText := ""
				if len(a.personItems) > 0 && a.personCached {
					countText = fmt.Sprintf("已缓存 %d 部作品", len(a.personItems))
					if a.personLoading {
						countText += "，正在刷新…"
					}
				} else if len(a.personItems) > 0 {
					countText = fmt.Sprintf("影视库中共有 %d 部参演与相关作品", len(a.personItems))
				} else if a.personLoading {
					countText = "正在读取作品列表…"
				}
				if countText != "" {
					ui.Text(c, countText).FontSize(11).TextColor(t.Accent)
				}
			})
		})

		// 作品网格展示
		ui.Column(c).Grow(1).FillHeight().Gap(10).Children(func() {
			ui.Text(c, "参演与相关作品").FontSize(15).Bold()
			if a.personError != "" && len(a.personItems) > 0 {
				ui.Text(c, a.personError).FontSize(12).TextColor(ui.Hex("#ad5148"))
			}
			if a.personLoading && len(a.personItems) == 0 {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(24, 0).Children(func() {
					ui.Spinner(c).FontSize(16)
					ui.Text(c, "正在读取作品列表…").FontSize(13).TextColor(t.TextMuted)
				})
			} else if a.personError != "" && len(a.personItems) == 0 {
				ui.Row(c).Gap(10).Padding(16, 0).Children(func() {
					ui.Text(c, a.personError).FontSize(12).TextColor(ui.Hex("#ad5148"))
					if actionButton(c, "重试").Clicked() {
						a.openPerson(person)
					}
				})
			} else if len(a.personItems) == 0 {
				ui.Text(c, "在当前影视库中没有找到该演职人员的作品。").FontSize(13).TextColor(t.TextMuted).Padding(24, 0)
			} else {
				items := a.personItems
				grid := ui.GridView(c, &a.personGrid, len(items), 150, 286, func(i int) {
					if i >= len(items) {
						return
					}
					item := items[i]
					card := actionButton(c.Key("person-"+item.ID), "").Padding(0).BorderWidth(0).
						Background(ui.Color{}).TextColor(t.Text)
					card.Children(func() {
						ui.Column(c).Gap(8).Center().Children(func() {
							cover := ui.Box(c).Size(150, 225).Radius(10).Clip().BorderWidth(0)
							cover.Children(func() {
								poster := a.imageFor(item, 150, 225)
								if poster != nil {
									ui.Image(c, poster).Size(150, 225).Fit(ui.Cover)
								} else {
									ui.Box(c).Size(150, 225).Background(ui.Hex("#e8e8e2")).Center().Children(func() {
										ui.Text(c, "FILM").FontSize(10).Bold().TextColor(ui.Hex("#969d96"))
									})
								}
								if item.Rating != "" {
									ui.Text(c, item.Rating).Absolute().Top(10).Left(10).Padding(3, 7).Radius(6).
										Background(t.Accent).FontSize(12).Bold().TextColor(t.AccentText)
								}
							})
							ui.Text(c, item.Title).FontSize(14).Bold().TextAlign(ui.Center).SingleLine()
							subtitle := item.Subtitle()
							if subtitle == "" {
								subtitle = item.Year
							}
							if subtitle != "" {
								ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted).SingleLine()
							}
						})
					})
					if card.Clicked() {
						a.closePerson()
						a.openDetail(item)
					}
				}).Grow(1)
				bindSmoothScroll(grid, &a.personScroll)
				bindScrollEdgeFades(grid, &a.personScroll, c.Theme().Background)
			}
		})
	})
}
