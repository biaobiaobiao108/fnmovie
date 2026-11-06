package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DanmakuSettings controls how danmaku are matched and rendered. Boolean
// fields use negative names so the zero value of a freshly created settings
// file still means "enabled with the defaults".
type DanmakuSettings struct {
	Disabled     bool    `json:"disabled,omitempty"`
	ServerURL    string  `json:"serverUrl,omitempty"`
	FontScale    float64 `json:"fontScale,omitempty"`
	Opacity      float64 `json:"opacity,omitempty"`
	SpeedSeconds float64 `json:"speedSeconds,omitempty"`
	AreaPercent  int     `json:"areaPercent,omitempty"`
	Anchor       string  `json:"anchor,omitempty"`
	OffsetSecond int     `json:"offsetSeconds,omitempty"`
	HideScroll   bool    `json:"hideScroll,omitempty"`
	HideTop      bool    `json:"hideTop,omitempty"`
	HideBottom   bool    `json:"hideBottom,omitempty"`
	NoOutline    bool    `json:"noOutline,omitempty"`
	MaxComments  int     `json:"maxComments,omitempty"`
}

func (s DanmakuSettings) normalized() DanmakuSettings {
	s.ServerURL = strings.TrimSpace(s.ServerURL)
	if s.FontScale <= 0 {
		s.FontScale = 1
	}
	s.FontScale = math.Min(2.5, math.Max(0.5, s.FontScale))
	if s.Opacity <= 0 {
		s.Opacity = 0.9
	}
	s.Opacity = math.Min(1, math.Max(0.15, s.Opacity))
	if s.SpeedSeconds <= 0 {
		s.SpeedSeconds = 10
	}
	s.SpeedSeconds = math.Min(30, math.Max(4, s.SpeedSeconds))
	if s.AreaPercent <= 0 {
		s.AreaPercent = 50
	}
	s.AreaPercent = min(100, max(10, s.AreaPercent))
	switch s.Anchor {
	case "top", "middle", "bottom":
	default:
		s.Anchor = "top"
	}
	s.OffsetSecond = min(300, max(-300, s.OffsetSecond))
	if s.MaxComments <= 0 {
		s.MaxComments = 20000
	}
	s.MaxComments = min(200000, max(100, s.MaxComments))
	return s
}

// danmakuServerURL prefers the explicitly configured server and otherwise
// derives the conventional danmu_api port from the fnOS server address.
func (a *appState) danmakuServerURL() string {
	if configured := strings.TrimSpace(a.settings.Danmaku.ServerURL); configured != "" {
		return configured
	}
	return defaultDanmakuServerURL(a.settings.ServerURL)
}

func defaultDanmakuServerURL(serverURL string) string {
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return ""
	}
	if !strings.Contains(serverURL, "://") {
		serverURL = "http://" + serverURL
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(parsed.Hostname(), "9321")
}

// DanmakuClient speaks the danmu_api v2 protocol used by the reference
// clients. It accepts both the documented response envelope and the flatter
// shapes returned by some community server builds.
type DanmakuClient struct {
	baseURL string
	client  *http.Client
}

func NewDanmakuClient(serverURL string) *DanmakuClient {
	return &DanmakuClient{
		baseURL: normalizeDanmakuBaseURL(serverURL),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func normalizeDanmakuBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	raw = strings.TrimRight(raw, "/")
	for strings.HasSuffix(strings.ToLower(raw), "/api/v2") {
		raw = raw[:len(raw)-len("/api/v2")]
		raw = strings.TrimRight(raw, "/")
	}
	return raw
}

type DanmakuMatch struct {
	EpisodeID    string
	AnimeID      string
	AnimeTitle   string
	EpisodeTitle string
}

type DanmakuAnime struct {
	ID           string
	Title        string
	EpisodeCount int
}

type DanmakuEpisode struct {
	ID     string
	Number int
	Title  string
}

type DanmakuComment struct {
	Time  float64
	Mode  int
	Color uint32
	Text  string
}

func (c *DanmakuClient) Match(ctx context.Context, fileName string) ([]DanmakuMatch, error) {
	if c.baseURL == "" {
		return nil, errors.New("未配置弹幕服务器")
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return nil, errors.New("影片名称不能为空")
	}
	var raw any
	if err := c.postJSON(ctx, "/api/v2/match", map[string]string{"fileName": fileName}, &raw); err != nil {
		return nil, err
	}
	matches := make([]DanmakuMatch, 0)
	for _, value := range firstDanmakuList(raw, "matches", "data") {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		matches = append(matches, DanmakuMatch{
			EpisodeID:    firstString(object, "episodeId", "episode_id"),
			AnimeID:      firstString(object, "animeId", "anime_id"),
			AnimeTitle:   firstString(object, "animeTitle", "anime_title", "title"),
			EpisodeTitle: firstString(object, "episodeTitle", "episode_title"),
		})
	}
	return matches, nil
}

func (c *DanmakuClient) Search(ctx context.Context, keyword string) ([]DanmakuAnime, error) {
	if c.baseURL == "" {
		return nil, errors.New("未配置弹幕服务器")
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("请输入番剧名称")
	}
	path := "/api/v2/search/anime?keyword=" + url.QueryEscape(keyword)
	var raw any
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	animes := make([]DanmakuAnime, 0)
	for _, value := range firstDanmakuList(raw, "animes", "data", "result") {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		anime := DanmakuAnime{
			ID:           firstString(object, "animeId", "anime_id", "id"),
			Title:        firstString(object, "animeTitle", "anime_title", "title", "name"),
			EpisodeCount: firstInt(object, "episodeCount", "episode_count"),
		}
		if anime.ID != "" && anime.Title != "" {
			animes = append(animes, anime)
		}
	}
	return animes, nil
}

func (c *DanmakuClient) Episodes(ctx context.Context, animeID string) ([]DanmakuEpisode, error) {
	if c.baseURL == "" {
		return nil, errors.New("未配置弹幕服务器")
	}
	animeID = strings.TrimSpace(animeID)
	if animeID == "" {
		return nil, errors.New("番剧 ID 不能为空")
	}
	var raw any
	if err := c.getJSON(ctx, "/api/v2/bangumi/"+url.PathEscape(animeID), &raw); err != nil {
		return nil, err
	}
	episodes := make([]DanmakuEpisode, 0)
	for _, value := range firstDanmakuList(raw, "episodes", "bangumi", "data") {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		episode := DanmakuEpisode{
			ID:     firstString(object, "episodeId", "episode_id", "id"),
			Number: firstInt(object, "episodeNumber", "episode_number", "episode", "number"),
			Title:  firstString(object, "episodeTitle", "episode_title", "title", "name"),
		}
		if episode.Number <= 0 {
			episode.Number = parseDanmakuEpisodeNumber(episode.Title)
		}
		if episode.ID != "" {
			episodes = append(episodes, episode)
		}
	}
	sort.SliceStable(episodes, func(i, j int) bool {
		if episodes[i].Number != episodes[j].Number {
			return episodes[i].Number < episodes[j].Number
		}
		return episodes[i].ID < episodes[j].ID
	})
	return episodes, nil
}

func (c *DanmakuClient) Comments(ctx context.Context, episodeID string, maxComments int) ([]DanmakuComment, error) {
	if c.baseURL == "" {
		return nil, errors.New("未配置弹幕服务器")
	}
	episodeID = strings.TrimSpace(episodeID)
	if episodeID == "" {
		return nil, errors.New("剧集 ID 不能为空")
	}
	var raw any
	if err := c.getJSON(ctx, "/api/v2/comment/"+url.PathEscape(episodeID)+"?format=json", &raw); err != nil {
		return nil, err
	}
	comments := make([]DanmakuComment, 0)
	for _, value := range firstDanmakuList(raw, "comments", "data", "result") {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		comment, ok := parseDanmakuComment(object)
		if !ok {
			continue
		}
		comments = append(comments, comment)
	}
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].Time < comments[j].Time })
	return sampleDanmakuComments(comments, maxComments), nil
}

func parseDanmakuComment(object map[string]any) (DanmakuComment, bool) {
	text := firstString(object, "m", "text", "content", "message")
	if text == "" {
		return DanmakuComment{}, false
	}
	comment := DanmakuComment{Mode: 1, Color: 0xFFFFFF, Text: text}
	if progress := firstString(object, "p", "progress"); progress != "" {
		parts := strings.Split(progress, ",")
		comment.Time, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if len(parts) > 1 {
			comment.Mode, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
		if len(parts) > 2 {
			if color, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 32); err == nil {
				comment.Color = uint32(color) & 0xFFFFFF
			}
		}
	} else {
		comment.Time = firstFloat(object, "time", "t", "position")
		comment.Mode = firstInt(object, "mode", "type")
		comment.Color = uint32(firstInt(object, "c", "color")) & 0xFFFFFF
	}
	if comment.Mode == 0 {
		comment.Mode = 1
	}
	if comment.Color == 0 {
		comment.Color = 0xFFFFFF
	}
	// Modes 7-9 are advanced/scripted comments that cannot be represented as
	// plain ASS text; drop them instead of rendering broken glyphs.
	if comment.Mode >= 7 && comment.Mode <= 9 {
		return DanmakuComment{}, false
	}
	if comment.Time < 0 {
		return DanmakuComment{}, false
	}
	comment.Text = cleanDanmakuText(comment.Text)
	return comment, comment.Text != ""
}

func sampleDanmakuComments(comments []DanmakuComment, maxComments int) []DanmakuComment {
	if maxComments <= 0 || len(comments) <= maxComments {
		return comments
	}
	sampled := make([]DanmakuComment, 0, maxComments)
	step := float64(len(comments)) / float64(maxComments)
	for i := 0; i < maxComments; i++ {
		index := int(float64(i) * step)
		if index >= len(comments) {
			index = len(comments) - 1
		}
		sampled = append(sampled, comments[index])
	}
	return sampled
}

func (c *DanmakuClient) getJSON(ctx context.Context, path string, out *any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *DanmakuClient) postJSON(ctx context.Context, path string, body any, out *any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *DanmakuClient) doJSON(ctx context.Context, method, path string, body any, out *any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "FnMovie/1.3 (+danmaku)")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("连接弹幕服务器失败：%w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("读取弹幕数据失败：%w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if len(message) > 120 {
			message = message[:120]
		}
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("弹幕服务器返回 %s：%s", response.Status, message)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("解析弹幕数据失败：%w", err)
	}
	return nil
}

func firstDanmakuList(value any, keys ...string) []any {
	switch current := value.(type) {
	case []any:
		return current
	case map[string]any:
		for _, key := range keys {
			child, exists := current[key]
			if !exists {
				continue
			}
			if list, ok := child.([]any); ok {
				return list
			}
			if list := firstDanmakuList(child, keys...); len(list) > 0 {
				return list
			}
		}
	}
	return nil
}

func firstInt(object map[string]any, keys ...string) int {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		switch current := value.(type) {
		case float64:
			return int(current)
		case int:
			return current
		case json.Number:
			number, _ := current.Int64()
			return int(number)
		case string:
			number, _ := strconv.Atoi(strings.TrimSpace(current))
			return number
		}
	}
	return 0
}

func firstFloat(object map[string]any, keys ...string) float64 {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		switch current := value.(type) {
		case float64:
			return current
		case json.Number:
			number, _ := current.Float64()
			return number
		case string:
			number, _ := strconv.ParseFloat(strings.TrimSpace(current), 64)
			return number
		}
	}
	return 0
}

var (
	danmakuSeasonEpisodePattern  = regexp.MustCompile(`(?i)s(?:\d{1,2})[\s._-]*e(\d{1,4})`)
	danmakuChineseEpisodePattern = regexp.MustCompile(`第\s*(\d{1,4})\s*[集话話]`)
	danmakuEnglishEpisodePattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])e(?:p)?[\s._-]*(\d{1,4})(?:[^0-9]|$)`)
)

func parseDanmakuEpisodeNumber(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	for _, pattern := range []*regexp.Regexp{danmakuSeasonEpisodePattern, danmakuChineseEpisodePattern, danmakuEnglishEpisodePattern} {
		if match := pattern.FindStringSubmatch(text); len(match) > 1 {
			if number, err := strconv.Atoi(match[1]); err == nil {
				return number
			}
		}
	}
	return 0
}

// danmakuMatchName prefers the original file name because danmu_api's matcher
// understands release tags such as S01E02 and 1080p. The display title is a
// fallback for servers that only expose localized metadata.
func danmakuMatchName(item MediaItem) string {
	for _, key := range []string{"file_name", "fileName", "filename", "file_path", "filePath", "path"} {
		if name := firstString(item.Raw, key); name != "" {
			return strings.TrimSpace(filepath.Base(name))
		}
	}
	if title := strings.TrimSpace(item.SeriesTitle); title != "" {
		return title
	}
	return strings.TrimSpace(item.Title)
}

func danmakuEpisodeNumber(item MediaItem) int {
	if item.EpisodeNumber > 0 {
		return item.EpisodeNumber
	}
	if name := danmakuMatchName(item); name != "" {
		if number := parseDanmakuEpisodeNumber(name); number > 0 {
			return number
		}
	}
	return parseDanmakuEpisodeNumber(item.Title)
}

func resolveDanmaku(ctx context.Context, client *DanmakuClient, item MediaItem, maxComments int) ([]DanmakuComment, string, error) {
	target := danmakuEpisodeNumber(item)
	name := danmakuMatchName(item)
	if name == "" {
		return nil, "", errors.New("无法确定影片名称")
	}

	matches, matchErr := client.Match(ctx, name)
	var chosen DanmakuMatch
	if len(matches) > 0 {
		chosen = matches[0]
	}
	animeID := chosen.AnimeID
	animeTitle := chosen.AnimeTitle

	useMatchedEpisode := func() ([]DanmakuComment, string, error, bool) {
		if chosen.EpisodeID == "" {
			return nil, "", nil, false
		}
		if target > 0 {
			matchedNumber := parseDanmakuEpisodeNumber(chosen.EpisodeTitle)
			if matchedNumber > 0 && matchedNumber != target {
				return nil, "", nil, false
			}
		}
		comments, err := client.Comments(ctx, chosen.EpisodeID, maxComments)
		if err != nil {
			return nil, "", err, true
		}
		label := strings.TrimSpace(strings.Join([]string{animeTitle, chosen.EpisodeTitle}, " "))
		if label == "" {
			label = name
		}
		return comments, label, nil, true
	}
	if comments, label, err, handled := useMatchedEpisode(); handled {
		return comments, label, err
	}

	// The matcher often identifies the series but not the exact episode.
	// Falling back to the bangumi episode list keeps auto-match accurate for
	// multi-season libraries and for servers that only return anime IDs.
	if animeID == "" {
		query := strings.TrimSpace(item.SeriesTitle)
		if query == "" {
			query = strings.TrimSpace(item.Title)
		}
		if query != "" {
			if animes, err := client.Search(ctx, query); err == nil && len(animes) > 0 {
				animeID, animeTitle = animes[0].ID, animes[0].Title
			}
		}
	}
	if animeID != "" {
		if episodes, err := client.Episodes(ctx, animeID); err == nil && len(episodes) > 0 {
			episode := episodes[0]
			if target > 0 {
				found := false
				for _, candidate := range episodes {
					if candidate.Number == target {
						episode = candidate
						found = true
						break
					}
				}
				if !found {
					return nil, "", fmt.Errorf("该番剧没有第 %d 集", target)
				}
			}
			comments, err := client.Comments(ctx, episode.ID, maxComments)
			if err != nil {
				return nil, "", err
			}
			label := strings.TrimSpace(strings.Join([]string{animeTitle, episode.Title}, " "))
			if episode.Number > 0 {
				label = strings.TrimSpace(fmt.Sprintf("%s 第%d集", animeTitle, episode.Number))
				if episode.Title != "" {
					label += " " + episode.Title
				}
			}
			if label == "" {
				label = name
			}
			return comments, label, nil
		}
	}
	if matchErr != nil {
		return nil, "", matchErr
	}
	return nil, "", errors.New("未找到匹配的弹幕剧集，请在弹幕面板中搜索选集")
}

type DanmakuRenderOptions struct {
	FontScale    float64
	Opacity      float64
	SpeedSeconds float64
	AreaPercent  int
	Anchor       string
	OffsetSecond int
	ShowScroll   bool
	ShowTop      bool
	ShowBottom   bool
	Outline      bool
}

func (s DanmakuSettings) renderOptions() DanmakuRenderOptions {
	s = s.normalized()
	return DanmakuRenderOptions{
		FontScale: s.FontScale, Opacity: s.Opacity, SpeedSeconds: s.SpeedSeconds,
		AreaPercent: s.AreaPercent, Anchor: s.Anchor, OffsetSecond: s.OffsetSecond,
		ShowScroll: !s.HideScroll, ShowTop: !s.HideTop, ShowBottom: !s.HideBottom,
		Outline: !s.NoOutline,
	}
}

const (
	danmakuCanvasWidth   = 1920.0
	danmakuCanvasHeight  = 1080.0
	danmakuStaticSeconds = 5.0
	danmakuLineGap       = 1.45
)

type danmakuLane struct {
	start float64
	width float64
	end   float64
	used  bool
}

func renderDanmakuASS(comments []DanmakuComment, opts DanmakuRenderOptions) string {
	if opts.FontScale <= 0 {
		opts.FontScale = 1
	}
	if opts.Opacity <= 0 {
		opts.Opacity = 1
	}
	opts.Opacity = math.Min(1, math.Max(0.05, opts.Opacity))
	if opts.SpeedSeconds <= 0 {
		opts.SpeedSeconds = 10
	}
	opts.SpeedSeconds = math.Min(30, math.Max(4, opts.SpeedSeconds))
	if opts.AreaPercent <= 0 {
		opts.AreaPercent = 50
	}
	opts.AreaPercent = min(100, max(10, opts.AreaPercent))

	fontSize := 44 * opts.FontScale
	lineHeight := fontSize * danmakuLineGap
	areaHeight := danmakuCanvasHeight * float64(opts.AreaPercent) / 100
	laneCount := max(1, int(areaHeight/lineHeight))
	baseY := danmakuBaseY(areaHeight, opts.Anchor)
	alpha := int(math.Round((1 - opts.Opacity) * 255))
	outline := 0
	if opts.Outline {
		outline = 2
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, `[Script Info]
Title: FnMovie Danmaku
ScriptType: v4.00+
WrapStyle: 2
ScaledBorderAndShadow: yes
PlayResX: 1920
PlayResY: 1080

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Danmaku,Microsoft YaHei,%d,&H%02XFFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,%d,0,7,0,0,0,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`, int(math.Round(fontSize)), alpha, outline)

	sorted := make([]DanmakuComment, len(comments))
	copy(sorted, comments)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Time < sorted[j].Time })

	scrollLanes := make([]danmakuLane, laneCount)
	topLanes := make([]danmakuLane, laneCount)
	bottomLanes := make([]danmakuLane, min(3, max(1, int(danmakuCanvasHeight/lineHeight))))
	gapX := fontSize * 0.6

	for _, comment := range sorted {
		if comment.Mode >= 7 && comment.Mode <= 9 {
			continue
		}
		text := cleanDanmakuText(comment.Text)
		if text == "" {
			continue
		}
		start := comment.Time + float64(opts.OffsetSecond)
		if start < 0 {
			continue
		}
		width := danmakuTextWidth(text, fontSize) + gapX*2
		colorTag := danmakuColorTag(comment.Color)

		switch {
		case comment.Mode == 4:
			if !opts.ShowBottom {
				continue
			}
			lane := freeDanmakuStaticLane(bottomLanes, start)
			if lane < 0 {
				continue
			}
			y := danmakuCanvasHeight - lineHeight*0.2 - float64(lane)*lineHeight
			fmt.Fprintf(&builder, "Dialogue: 0,%s,%s,Danmaku,,0,0,0,,%s{\\an2\\pos(%.0f,%.0f)}%s\n",
				formatASSTime(start), formatASSTime(start+danmakuStaticSeconds), colorTag, danmakuCanvasWidth/2, y, escapeASSText(text))
		case comment.Mode == 5:
			if !opts.ShowTop {
				continue
			}
			lane := freeDanmakuStaticLane(topLanes, start)
			if lane < 0 {
				continue
			}
			y := baseY + float64(lane)*lineHeight
			fmt.Fprintf(&builder, "Dialogue: 0,%s,%s,Danmaku,,0,0,0,,%s{\\an8\\pos(%.0f,%.0f)}%s\n",
				formatASSTime(start), formatASSTime(start+danmakuStaticSeconds), colorTag, danmakuCanvasWidth/2, y, escapeASSText(text))
		default:
			if !opts.ShowScroll {
				continue
			}
			lane := freeDanmakuScrollLane(scrollLanes, start, width, opts.SpeedSeconds)
			if lane < 0 {
				continue
			}
			y := baseY + float64(lane)*lineHeight
			startX := danmakuCanvasWidth + gapX
			// Keep a generous off-screen margin: font metrics vary by system and
			// a too-small estimate makes long comments vanish while still visible.
			endX := -width - gapX - fontSize*2
			fmt.Fprintf(&builder, "Dialogue: 0,%s,%s,Danmaku,,0,0,0,,%s{\\an7\\move(%.0f,%.0f,%.0f,%.0f,0,%d)}%s\n",
				formatASSTime(start), formatASSTime(start+opts.SpeedSeconds), colorTag,
				startX, y, endX, y, int(opts.SpeedSeconds*1000), escapeASSText(text))
		}
	}
	return builder.String()
}

func danmakuBaseY(areaHeight float64, anchor string) float64 {
	switch anchor {
	case "middle":
		return (danmakuCanvasHeight - areaHeight) / 2
	case "bottom":
		return danmakuCanvasHeight - areaHeight
	default:
		return 0
	}
}

func freeDanmakuScrollLane(lanes []danmakuLane, start, width, duration float64) int {
	for i := range lanes {
		lane := lanes[i]
		if !lane.used {
			lanes[i] = danmakuLane{start: start, width: width, used: true}
			return i
		}
		elapsed := start - lane.start
		required := math.Max(
			duration*(lane.width+40)/(danmakuCanvasWidth+lane.width),
			duration*(width+40)/(danmakuCanvasWidth+width),
		)
		if elapsed >= required {
			lanes[i] = danmakuLane{start: start, width: width, used: true}
			return i
		}
	}
	return -1
}

func freeDanmakuStaticLane(lanes []danmakuLane, start float64) int {
	for i := range lanes {
		if !lanes[i].used || start >= lanes[i].end {
			lanes[i] = danmakuLane{start: start, end: start + danmakuStaticSeconds, used: true}
			return i
		}
	}
	return -1
}

func danmakuTextWidth(text string, fontSize float64) float64 {
	width := 0.0
	for _, r := range text {
		switch {
		case r == ' ':
			width += fontSize * 0.36
		case r < 0x80:
			// Proportional Latin fonts are wider than a flat half-em estimate,
			// especially for uppercase and digits. Overestimating here keeps
			// lanes clear and guarantees the text leaves the screen on time.
			switch {
			case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				width += fontSize * 0.68
			default:
				width += fontSize * 0.60
			}
		case r >= 0x2E80:
			width += fontSize * 1.06
		default:
			width += fontSize * 0.82
		}
	}
	return width
}

func danmakuColorTag(color uint32) string {
	r := (color >> 16) & 0xFF
	g := (color >> 8) & 0xFF
	b := color & 0xFF
	if r == 0xFF && g == 0xFF && b == 0xFF {
		return ""
	}
	return fmt.Sprintf(`{\c&H%02X%02X%02X&}`, b, g, r)
}

func cleanDanmakuText(text string) string {
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.TrimSpace(text)
	var builder strings.Builder
	for _, r := range text {
		if r < 0x20 || r == 0x7F {
			continue
		}
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

func escapeASSText(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `{`, `\{`)
	text = strings.ReplaceAll(text, `}`, `\}`)
	return text
}

func formatASSTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	centis := int((seconds - float64(int(seconds))) * 100)
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, secs, centis)
}
