package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type danmakuUIState struct {
	cancel        context.CancelFunc
	request       uint64
	renderVersion uint64
	assDir        string
	assPath       string
	comments      []DanmakuComment
	status        string
	loading       bool

	panelTab        int
	searchQuery     string
	searchResults   []DanmakuAnime
	searchLoading   bool
	searchError     string
	selectedAnime   DanmakuAnime
	episodes        []DanmakuEpisode
	episodesLoading bool
	episodesError   string
	settingsInit    bool
	serverURL       string
	token           string
	fontScale       float64
	opacity         float64
	speed           float64
	area            float64
	offset          float64
	showScroll      bool
	showTop         bool
	showBottom      bool
	outline         bool
}

func (a *appState) updateDanmakuUI(fn func()) {
	if a.window == nil {
		fn()
		return
	}
	a.window.Update(fn)
}

func (a *appState) invalidateDanmakuUI() {
	if a.playerMenuWindow != nil {
		a.playerMenuWindow.Invalidate()
	}
	if a.playerOverlayWindow != nil {
		a.playerOverlayWindow.Invalidate()
	}
}

func (a *appState) newDanmakuRequest() (context.Context, uint64) {
	if a.danmaku.cancel != nil {
		a.danmaku.cancel()
		a.danmaku.cancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.danmaku.cancel = cancel
	a.danmaku.request++
	a.danmaku.loading = false
	return ctx, a.danmaku.request
}

func (a *appState) ensureDanmakuFile() error {
	if a.danmaku.assPath != "" {
		return nil
	}
	dir, err := os.MkdirTemp("", "fnmovie-danmaku-")
	if err != nil {
		return err
	}
	a.danmaku.assDir = dir
	a.danmaku.assPath = filepath.Join(dir, "fnmovie-danmaku.ass")
	return nil
}

func isDanmakuSubtitleTrack(track PlayerTrack) bool {
	if !track.External {
		return false
	}
	title := strings.ToLower(strings.TrimSpace(track.Title))
	title = strings.TrimSuffix(title, ".ass")
	return title == "弹幕" || title == "danmaku" || strings.Contains(title, "fnmovie-danmaku")
}

// filterDanmakuSubtitleTracks keeps the generated danmaku track out of the
// user's subtitle menu; it is managed by the danmaku panel instead.
func filterDanmakuSubtitleTracks(tracks []PlayerTrack) []PlayerTrack {
	if len(tracks) == 0 {
		return tracks
	}
	filtered := make([]PlayerTrack, 0, len(tracks))
	for _, track := range tracks {
		if !isDanmakuSubtitleTrack(track) {
			filtered = append(filtered, track)
		}
	}
	return filtered
}

// beginDanmaku prepares a playback-scoped danmaku session. The generated ASS
// file lives in a temporary directory that is removed when playback ends.
func (a *appState) beginDanmaku(item MediaItem, player *Player) {
	a.cancelDanmaku()
	settings := a.settings.Danmaku.normalized()
	if settings.Disabled {
		a.danmaku.status = "弹幕已关闭"
		return
	}
	if a.danmakuServerURL() == "" {
		a.danmaku.status = "未配置弹幕服务器，请在弹幕面板中填写"
		return
	}
	a.startDanmakuMatch(item, player)
}

func (a *appState) cancelDanmaku() {
	if a.danmaku.cancel != nil {
		a.danmaku.cancel()
		a.danmaku.cancel = nil
	}
	a.danmaku.request++
	a.danmaku.renderVersion++
	if a.danmaku.assDir != "" {
		_ = os.RemoveAll(a.danmaku.assDir)
	}
	a.danmaku.assDir = ""
	a.danmaku.assPath = ""
	a.danmaku.comments = nil
	a.danmaku.status = ""
	a.danmaku.loading = false
	a.danmaku.searchLoading = false
	a.danmaku.searchResults = nil
	a.danmaku.searchError = ""
	a.danmaku.selectedAnime = DanmakuAnime{}
	a.danmaku.episodes = nil
	a.danmaku.episodesLoading = false
	a.danmaku.episodesError = ""
}

func (a *appState) startDanmakuMatch(item MediaItem, player *Player) {
	if player == nil {
		return
	}
	if a.settings.Danmaku.Disabled {
		a.settings.Danmaku.Disabled = false
		_ = SaveSettings(a.settings)
	}
	serverURL := a.danmakuServerURL()
	if serverURL == "" {
		a.danmaku.loading = false
		a.danmaku.status = "未配置弹幕服务器，请在弹幕面板中填写"
		a.invalidateDanmakuUI()
		return
	}
	if err := a.ensureDanmakuFile(); err != nil {
		a.danmaku.loading = false
		a.danmaku.status = "无法创建弹幕缓存：" + err.Error()
		a.invalidateDanmakuUI()
		return
	}
	ctx, request := a.newDanmakuRequest()
	settings := a.settings.Danmaku.normalized()
	client := NewDanmakuClient(serverURL, settings.Token)
	maxComments := settings.MaxComments
	a.danmaku.loading = true
	a.danmaku.status = "正在匹配弹幕…"
	a.danmaku.searchError = ""
	a.invalidateDanmakuUI()
	go func() {
		comments, label, err := resolveDanmaku(ctx, client, item, maxComments)
		if ctx.Err() != nil {
			return
		}
		a.updateDanmakuUI(func() {
			if request != a.danmaku.request || a.player != player || !a.playback.Active {
				return
			}
			a.danmaku.loading = false
			if err != nil {
				a.danmaku.status = "弹幕匹配失败：" + err.Error()
				a.invalidateDanmakuUI()
				return
			}
			a.applyDanmakuComments(comments, label)
		})
	}()
}

func (a *appState) applyDanmakuComments(comments []DanmakuComment, label string) {
	a.danmaku.comments = comments
	a.danmaku.loading = false
	status := fmt.Sprintf("已加载 %d 条弹幕", len(comments))
	if len(comments) == 0 {
		status = "该剧集暂无弹幕"
	}
	if label != "" {
		status += " · " + label
	}
	a.danmaku.status = status
	a.scheduleDanmakuRender()
	a.invalidateDanmakuUI()
}

func (a *appState) scheduleDanmakuRender() {
	if a.danmaku.assPath == "" {
		return
	}
	settings := a.settings.Danmaku.normalized()
	comments := append([]DanmakuComment(nil), a.danmaku.comments...)
	player := a.player
	path := a.danmaku.assPath
	a.danmaku.renderVersion++
	version := a.danmaku.renderVersion
	go func() {
		time.Sleep(180 * time.Millisecond)
		ass := renderDanmakuASS(comments, settings.renderOptions())
		if err := writeDanmakuFile(path, ass); err != nil {
			a.updateDanmakuUI(func() {
				if version == a.danmaku.renderVersion {
					a.danmaku.status = "写入弹幕字幕失败：" + err.Error()
					a.invalidateDanmakuUI()
				}
			})
			return
		}
		a.updateDanmakuUI(func() {
			if version != a.danmaku.renderVersion || player == nil || a.player != player || !a.playback.Active {
				return
			}
			if !player.SetDanmakuSubtitle(path) {
				a.danmaku.status = "弹幕字幕挂载失败"
			}
			player.SetDanmakuVisible(!a.settings.Danmaku.Disabled)
			a.invalidateDanmakuUI()
		})
	}()
}

func writeDanmakuFile(path, content string) error {
	permanent := path
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(content), 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, permanent); err == nil {
		return nil
	}
	// Some Windows configurations cannot replace a file that libmpv briefly
	// holds; fall back to an in-place write and let sub-reload pick it up.
	if err := os.WriteFile(permanent, []byte(content), 0600); err != nil {
		return err
	}
	return os.Remove(temporary)
}

func (a *appState) prepareDanmakuPanel() {
	settings := a.settings.Danmaku.normalized()
	a.danmaku.serverURL = a.danmakuServerURL()
	a.danmaku.token = settings.Token
	a.danmaku.fontScale = settings.FontScale
	a.danmaku.opacity = settings.Opacity * 100
	a.danmaku.speed = settings.SpeedSeconds
	a.danmaku.area = float64(settings.AreaPercent)
	a.danmaku.offset = float64(settings.OffsetSecond)
	a.danmaku.showScroll = !settings.HideScroll
	a.danmaku.showTop = !settings.HideTop
	a.danmaku.showBottom = !settings.HideBottom
	a.danmaku.outline = !settings.NoOutline
	a.danmaku.settingsInit = true
}

func (a *appState) danmakuSettingsFromPanel() DanmakuSettings {
	settings := a.settings.Danmaku
	settings.ServerURL = strings.TrimSpace(a.danmaku.serverURL)
	settings.Token = strings.TrimSpace(a.danmaku.token)
	settings.FontScale = a.danmaku.fontScale
	settings.Opacity = a.danmaku.opacity / 100
	settings.SpeedSeconds = a.danmaku.speed
	settings.AreaPercent = int(math.Round(a.danmaku.area))
	settings.OffsetSecond = int(math.Round(a.danmaku.offset))
	settings.HideScroll = !a.danmaku.showScroll
	settings.HideTop = !a.danmaku.showTop
	settings.HideBottom = !a.danmaku.showBottom
	settings.NoOutline = !a.danmaku.outline
	return settings.normalized()
}

func (a *appState) applyDanmakuPanelSettings() {
	a.settings.Danmaku = a.danmakuSettingsFromPanel()
	_ = SaveSettings(a.settings)
	a.scheduleDanmakuRender()
	a.invalidateDanmakuUI()
}

func (a *appState) applyDanmakuServerSettings(rematch bool) {
	a.settings.Danmaku = a.danmakuSettingsFromPanel()
	_ = SaveSettings(a.settings)
	a.danmaku.status = "弹幕服务器已更新"
	if rematch {
		a.retryDanmaku()
	} else {
		a.invalidateDanmakuUI()
	}
}

func (a *appState) setDanmakuAnchor(anchor string) {
	a.settings.Danmaku.Anchor = anchor
	a.settings.Danmaku = a.settings.Danmaku.normalized()
	_ = SaveSettings(a.settings)
	a.scheduleDanmakuRender()
	a.invalidateDanmakuUI()
}

func (a *appState) toggleDanmakuEnabled() {
	a.settings.Danmaku.Disabled = !a.settings.Danmaku.Disabled
	a.settings.Danmaku = a.settings.Danmaku.normalized()
	_ = SaveSettings(a.settings)
	enabled := !a.settings.Danmaku.Disabled
	if a.player != nil {
		a.player.SetDanmakuVisible(enabled)
	}
	if enabled {
		a.danmaku.status = "弹幕已开启"
		if !a.danmaku.loading && len(a.danmaku.comments) == 0 && a.playingItem != nil && a.player != nil {
			a.startDanmakuMatch(*a.playingItem, a.player)
		}
	} else {
		a.danmaku.status = "弹幕已关闭"
	}
	a.invalidateDanmakuUI()
}

func (a *appState) retryDanmaku() {
	if a.playingItem == nil || a.player == nil {
		a.danmaku.status = "当前没有正在播放的影片"
		a.invalidateDanmakuUI()
		return
	}
	a.startDanmakuMatch(*a.playingItem, a.player)
}

func (a *appState) danmakuSearch() {
	query := strings.TrimSpace(a.danmaku.searchQuery)
	if query == "" {
		a.danmaku.searchError = "请输入番剧名称"
		a.invalidateDanmakuUI()
		return
	}
	serverURL := a.danmakuServerURL()
	if serverURL == "" {
		a.danmaku.searchError = "请先配置弹幕服务器"
		a.invalidateDanmakuUI()
		return
	}
	ctx, request := a.newDanmakuRequest()
	client := NewDanmakuClient(serverURL, a.settings.Danmaku.Token)
	a.danmaku.searchLoading = true
	a.danmaku.searchError = ""
	a.danmaku.searchResults = nil
	a.danmaku.selectedAnime = DanmakuAnime{}
	a.danmaku.episodes = nil
	a.danmaku.episodesError = ""
	a.danmaku.status = "正在搜索“" + query + "”…"
	a.invalidateDanmakuUI()
	go func() {
		animes, err := client.Search(ctx, query)
		if ctx.Err() != nil {
			return
		}
		a.updateDanmakuUI(func() {
			if request != a.danmaku.request {
				return
			}
			a.danmaku.searchLoading = false
			if err != nil {
				a.danmaku.searchError = err.Error()
				a.danmaku.status = "搜索失败"
			} else {
				a.danmaku.searchResults = animes
				if len(animes) == 0 {
					a.danmaku.searchError = "未找到匹配结果"
					a.danmaku.status = "未找到匹配结果"
				} else {
					a.danmaku.status = fmt.Sprintf("搜索到 %d 部番剧", len(animes))
				}
			}
			a.invalidateDanmakuUI()
		})
	}()
}

func (a *appState) danmakuLoadEpisodes(anime DanmakuAnime) {
	if anime.ID == "" {
		return
	}
	serverURL := a.danmakuServerURL()
	if serverURL == "" {
		a.danmaku.episodesError = "请先配置弹幕服务器"
		a.invalidateDanmakuUI()
		return
	}
	ctx, request := a.newDanmakuRequest()
	client := NewDanmakuClient(serverURL, a.settings.Danmaku.Token)
	a.danmaku.selectedAnime = anime
	a.danmaku.episodes = nil
	a.danmaku.episodesLoading = true
	a.danmaku.episodesError = ""
	a.danmaku.status = "正在读取《" + anime.Title + "》的剧集…"
	a.invalidateDanmakuUI()
	go func() {
		episodes, err := client.Episodes(ctx, anime.ID)
		if ctx.Err() != nil {
			return
		}
		a.updateDanmakuUI(func() {
			if request != a.danmaku.request {
				return
			}
			a.danmaku.episodesLoading = false
			if err != nil {
				a.danmaku.episodesError = err.Error()
				a.danmaku.status = "读取剧集失败"
			} else {
				a.danmaku.episodes = episodes
				if len(episodes) == 0 {
					a.danmaku.episodesError = "该番剧没有剧集"
				} else {
					a.danmaku.status = fmt.Sprintf("《%s》共 %d 集", anime.Title, len(episodes))
				}
			}
			a.invalidateDanmakuUI()
		})
	}()
}

func (a *appState) danmakuSelectEpisode(episode DanmakuEpisode) {
	if episode.ID == "" {
		return
	}
	serverURL := a.danmakuServerURL()
	if serverURL == "" {
		a.danmaku.status = "请先配置弹幕服务器"
		a.invalidateDanmakuUI()
		return
	}
	if err := a.ensureDanmakuFile(); err != nil {
		a.danmaku.status = "无法创建弹幕缓存：" + err.Error()
		a.invalidateDanmakuUI()
		return
	}
	ctx, request := a.newDanmakuRequest()
	client := NewDanmakuClient(serverURL, a.settings.Danmaku.Token)
	maxComments := a.settings.Danmaku.normalized().MaxComments
	a.danmaku.loading = true
	a.danmaku.status = "正在加载弹幕…"
	a.invalidateDanmakuUI()
	go func() {
		comments, err := client.Comments(ctx, episode.ID, maxComments)
		if ctx.Err() != nil {
			return
		}
		a.updateDanmakuUI(func() {
			if request != a.danmaku.request {
				return
			}
			a.danmaku.loading = false
			if err != nil {
				a.danmaku.status = "弹幕加载失败：" + err.Error()
				a.invalidateDanmakuUI()
				return
			}
			label := episode.Title
			if episode.Number > 0 {
				label = fmt.Sprintf("%s 第%d集", a.danmaku.selectedAnime.Title, episode.Number)
				if episode.Title != "" {
					label += " " + episode.Title
				}
			}
			a.applyDanmakuComments(comments, label)
		})
	}()
}
