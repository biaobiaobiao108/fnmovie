package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	clientSignSeed = "NDzZTVxnRKP8Z0jXg1VAMonaG8akvh"
	clientAPIKey   = "16CCEB3D-AB42-077D-36A1-F355324E4237"
)

type Server struct {
	mu      sync.RWMutex
	baseURL string
	token   string
	client  *http.Client
}

type MediaItem struct {
	ID       string
	MediaID  string
	Title    string
	Kind     string
	Year     string
	Rating   string
	Overview string
	Poster   string
	Favorite bool
	Watched  bool
	AddedAt  string
	Raw      map[string]any
	Sources  []StreamSource
}

type MediaLibrary struct {
	ID     string
	Name   string
	Kind   string
	Poster string
	Raw    map[string]any
}

type StreamSource struct {
	Name    string
	Quality string
	URL     string
}

type LoginResult struct {
	Token string
}

type PlaybackResult struct {
	URL      string
	Quality  string
	MediaID  string
	Duration float64
	ResumeAt float64
}

type PlaybackProxy struct {
	server   *http.Server
	listener net.Listener
}

func (p *PlaybackProxy) Close() {
	if p == nil || p.server == nil {
		return
	}
	_ = p.server.Close()
	if p.listener != nil {
		_ = p.listener.Close()
	}
}

type APIError struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (e *APIError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("飞牛影视返回错误码 %d", e.Code)
}

func NewServer(base, token string) *Server {
	return &Server{
		baseURL: normalizeServerURL(base), token: token,
		client: &http.Client{Timeout: 25 * time.Second},
	}
}

func (s *Server) SetToken(token string) {
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
}

func (s *Server) tokenValue() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *Server) baseURLValue() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baseURL
}

func normalizeServerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func (s *Server) endpoint(apiVersion, route string) string {
	return s.baseURLValue() + "/api/" + apiVersion + "/" + strings.TrimLeft(route, "/")
}

func (s *Server) Login(username, password string) (LoginResult, error) {
	var response any
	body := struct {
		Username string `json:"username"`
		Password string `json:"password"`
		AppName  string `json:"app_name"`
	}{username, password, "fnmovie"}
	if err := s.request("POST", "v1", "login", body, &response, ""); err != nil {
		return LoginResult{}, err
	}
	data := firstObject(unwrapData(response))
	token := firstString(data, "token", "access_token", "accessToken", "auth_token", "authToken")
	if token == "" {
		return LoginResult{}, errors.New("登录成功响应中没有会话令牌")
	}
	s.SetToken(token)
	return LoginResult{Token: token}, nil
}

func (s *Server) Libraries() ([]MediaLibrary, error) {
	var response any
	if err := s.request("GET", "v1", "mdb/list", nil, &response, s.tokenValue()); err != nil {
		return nil, err
	}
	objects := findObjects(unwrapData(response))
	// Library records may not have the same fields as media items, so walk
	// arrays and maps directly rather than relying on media title detection.
	data := unwrapData(response)
	if array, ok := data.([]any); ok {
		objects = objects[:0]
		for _, value := range array {
			if object, ok := value.(map[string]any); ok {
				objects = append(objects, object)
			}
		}
	}
	libraries := make([]MediaLibrary, 0, len(objects))
	for _, object := range objects {
		library := MediaLibrary{
			ID:     firstString(object, "guid", "id", "mdb_guid"),
			Name:   firstString(object, "name", "title"),
			Kind:   firstString(object, "category", "type", "view_type"),
			Poster: firstString(object, "poster", "posters", "poster_url"),
			Raw:    object,
		}
		if library.ID != "" {
			libraries = append(libraries, library)
		}
	}
	return libraries, nil
}

func (s *Server) Library(query string) ([]MediaItem, error) {
	return s.LibraryItems("", query)
}

func (s *Server) LibraryItems(libraryID, query string) ([]MediaItem, error) {
	if strings.TrimSpace(query) != "" {
		var response any
		route := "search/list?q=" + url.QueryEscape(strings.TrimSpace(query))
		if err := s.request("GET", "v1", route, nil, &response, s.tokenValue()); err != nil {
			return nil, err
		}
		items := normalizeItems(unwrapData(response))
		return filterLibraryItems(items, libraryID), nil
	}
	const pageSize = 500
	items := make([]MediaItem, 0, pageSize)
	for page := 1; ; page++ {
		body := struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		}{page, pageSize}
		requestBody := map[string]any{"page": body.Page, "page_size": body.PageSize}
		if libraryID != "" {
			requestBody["ancestor_guid"] = libraryID
		}
		var response any
		if err := s.request("POST", "v1", "item/list", requestBody, &response, s.tokenValue()); err != nil {
			return nil, err
		}
		data := unwrapData(response)
		batch := normalizeItems(findItemsList(data))
		items = append(items, batch...)
		total, _ := asInt(valueAt(data, "total"))
		if len(batch) == 0 || total > 0 && int64(len(items)) >= total || len(batch) < pageSize {
			break
		}
	}
	return filterLibraryItems(items, libraryID), nil
}

// LibraryPageContext returns one bounded page so opening a library never
// waits for the server to enumerate every item in it. Search responses on
// current fnOS versions are not consistently paginated, so search is sliced
// client-side after the authenticated server-side query.
func (s *Server) LibraryPageContext(ctx context.Context, libraryID, query string, page, pageSize int) ([]MediaItem, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 100
	}
	if strings.TrimSpace(query) != "" {
		var response any
		route := "search/list?q=" + url.QueryEscape(strings.TrimSpace(query))
		if err := s.requestContext(ctx, "GET", "v1", route, nil, &response, s.tokenValue()); err != nil {
			return nil, 0, err
		}
		items := filterLibraryItems(normalizeItems(unwrapData(response)), libraryID)
		total := len(items)
		start := (page - 1) * pageSize
		if start >= total {
			return []MediaItem{}, total, nil
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		return items[start:end], total, nil
	}
	requestBody := map[string]any{"page": page, "page_size": pageSize}
	if libraryID != "" {
		requestBody["ancestor_guid"] = libraryID
	}
	var response any
	if err := s.requestContext(ctx, "POST", "v1", "item/list", requestBody, &response, s.tokenValue()); err != nil {
		return nil, 0, err
	}
	data := unwrapData(response)
	items := normalizeItems(findItemsList(data))
	items = filterLibraryItems(items, libraryID)
	total, _ := asInt(valueAt(data, "total"))
	if total <= 0 {
		total = int64((page-1)*pageSize + len(items))
		if len(items) == pageSize {
			total++ // indicate that another page may exist when the server omits total
		}
	}
	return items, int(total), nil
}

func filterLibraryItems(items []MediaItem, libraryID string) []MediaItem {
	if libraryID == "" {
		return items
	}
	out := make([]MediaItem, 0, len(items))
	hasLibraryField := false
	for _, item := range items {
		itemLibraryID, hasID := itemLibrary(item.Raw)
		hasLibraryField = hasLibraryField || hasID
		if itemLibraryID == libraryID {
			out = append(out, item)
		}
	}
	// Some fnOS versions scope item/list by ancestor_guid but omit that field
	// from returned records. In that case trust the server-side selection.
	if !hasLibraryField {
		return items
	}
	return out
}

func itemLibrary(raw map[string]any) (string, bool) {
	keys := []string{"mdb_guid", "mdbGuid", "mdb_id", "mdbId", "mediadb_guid", "mediadbGuid", "mediadb_id", "mediadbId", "library_guid", "libraryGuid", "library_id", "libraryId", "ancestor_guid", "ancestorGuid"}
	hasDirectField := false
	for _, key := range keys {
		if value, exists := raw[key]; exists {
			hasDirectField = true
			if id := anyString(value); id != "" {
				return id, true
			}
		}
	}
	for _, key := range []string{"mdb", "mediadb", "library", "ancestor"} {
		if nested, ok := raw[key].(map[string]any); ok {
			if id := firstString(nested, "guid", "id"); id != "" {
				return id, true
			}
		}
	}
	return "", hasDirectField
}

func (s *Server) Detail(item MediaItem) (MediaItem, error) {
	apiRoute := "item/" + url.PathEscape(item.ID)
	var response any
	if err := s.request("GET", "v1", apiRoute, nil, &response, s.tokenValue()); err != nil {
		return item, err
	}
	detail := normalizeItem(firstObject(unwrapData(response)))
	if detail.ID == "" {
		detail = item
	}
	return detail, nil
}

func (s *Server) Playback(item MediaItem) (PlaybackResult, error) {
	var infoResponse any
	infoBody := struct {
		ItemGuid string `json:"item_guid"`
	}{item.ID}
	if err := s.request("POST", "v1", "play/info", infoBody, &infoResponse, s.tokenValue()); err != nil {
		return PlaybackResult{}, err
	}
	info := firstObject(unwrapData(infoResponse))
	mediaGuid := firstString(info, "media_guid")
	if mediaGuid == "" {
		return PlaybackResult{}, errors.New("服务器没有返回媒体播放标识")
	}
	requestBody := struct {
		MediaGuid string              `json:"media_guid"`
		IP        string              `json:"ip"`
		Header    map[string][]string `json:"header"`
	}{mediaGuid, "fnmovie-windows", map[string][]string{"User-Agent": {"FnMovie/0.1"}}}
	var playResponse any
	if err := s.request("POST", "v1", "stream", requestBody, &playResponse, s.tokenValue()); err != nil {
		return PlaybackResult{}, err
	}
	playData := firstObject(unwrapData(playResponse))
	qualities, _ := playData["direct_link_qualities"].([]any)
	if len(qualities) == 0 {
		qualities, _ = playData["qualities"].([]any)
	}
	var selected map[string]any
	for _, value := range qualities {
		quality, _ := value.(map[string]any)
		if quality == nil || firstString(quality, "url") == "" {
			continue
		}
		if selected == nil || qualityBitrate(quality) > qualityBitrate(selected) {
			selected = quality
		}
	}
	video, _ := playData["video_stream"].(map[string]any)
	duration, _ := video["duration"].(float64)
	resumeAt, _ := info["ts"].(float64)
	if resumeAt < 0 || duration > 0 && resumeAt > duration {
		resumeAt = 0
	}
	if selected != nil {
		candidate := s.absoluteURL(firstString(selected, "url"))
		candidateURL, _ := url.Parse(candidate)
		baseURL, _ := url.Parse(s.baseURLValue())
		// Flymoo's direct links can point at an external cloud host and may
		// require browser-only cookies. Prefer a NAS-hosted direct URL; for
		// external links use the authenticated range endpoint, which supports
		// seeks and was verified against the server's Range response.
		if candidateURL != nil && baseURL != nil && strings.EqualFold(candidateURL.Host, baseURL.Host) {
			return PlaybackResult{URL: candidate, Quality: firstString(selected, "resolution"), MediaID: mediaGuid, Duration: duration, ResumeAt: resumeAt}, nil
		}
	}
	return PlaybackResult{URL: s.endpoint("v1", "media/range/"+url.PathEscape(mediaGuid)), Quality: "NAS 原画", MediaID: mediaGuid, Duration: duration, ResumeAt: resumeAt}, nil
}

// ProxyPlayback streams media through a random loopback URL so mpv never needs
// the server token in its command line. Range headers and response bodies pass
// through unchanged, allowing mpv to seek in the original NAS file.
func (s *Server) ProxyPlayback(upstream string) (string, *PlaybackProxy, error) {
	target, err := url.Parse(s.absoluteURL(upstream))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return "", nil, errors.New("服务器返回了无效的播放地址")
	}
	var randomPath [24]byte
	if _, err := rand.Read(randomPath[:]); err != nil {
		return "", nil, fmt.Errorf("无法生成本地播放会话：%w", err)
	}
	key := hex.EncodeToString(randomPath[:])
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("无法启动本地播放代理：%w", err)
	}
	base, _ := url.Parse(s.baseURLValue())
	token := s.tokenValue()
	attachToken := base != nil && strings.EqualFold(base.Host, target.Host)
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			request.URL.Path = target.Path
			request.URL.RawPath = target.RawPath
			request.URL.RawQuery = target.RawQuery
			request.Host = target.Host
			if attachToken && token != "" {
				request.Header.Set("Authorization", token)
				request.Header.Set("authx", signature(request.URL, request.Method, nil))
			} else {
				request.Header.Del("Authorization")
				request.Header.Del("authx")
			}
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+key {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	localServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = localServer.Serve(listener) }()
	return fmt.Sprintf("http://127.0.0.1:%d/%s", listener.Addr().(*net.TCPAddr).Port, key), &PlaybackProxy{server: localServer, listener: listener}, nil
}

func qualityBitrate(quality map[string]any) float64 {
	value, ok := quality["bitrate"].(float64)
	if !ok {
		return 0
	}
	return value
}

func (s *Server) ToggleFavorite(item MediaItem) error {
	body := struct {
		ItemGuid string `json:"item_guid"`
	}{item.ID}
	var response any
	if item.Favorite {
		return s.request("DELETE", "v1", "item/favorite", body, &response, s.tokenValue())
	}
	return s.request("PUT", "v1", "item/favorite", body, &response, s.tokenValue())
}

func (s *Server) UpdateProgress(itemGuid, mediaGuid string, position, duration float64) error {
	return s.UpdateProgressContext(context.Background(), itemGuid, mediaGuid, position, duration)
}

func (s *Server) UpdateProgressContext(ctx context.Context, itemGuid, mediaGuid string, position, _ float64) error {
	body := struct {
		MediaGuid string `json:"media_guid"`
		ItemGuid  string `json:"item_guid"`
		TS        int64  `json:"ts"`
	}{mediaGuid, itemGuid, int64(position)}
	var response any
	return s.requestContext(ctx, "POST", "v1", "play/record", body, &response, s.tokenValue())
}

func (s *Server) absoluteURL(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	base, _ := url.Parse(s.baseURLValue())
	ref, _ := url.Parse(value)
	return base.ResolveReference(ref).String()
}

func (s *Server) FetchImage(value string) ([]byte, error) {
	imageURL := s.absoluteURL(value)
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Trim-Client", "web")
	req.Header.Set("X-Trim-Client-Version", "631")
	base, _ := url.Parse(s.baseURLValue())
	token := s.tokenValue()
	if token != "" && base != nil && strings.EqualFold(parsed.Host, base.Host) {
		req.Header.Set("authx", signature(parsed, http.MethodGet, nil))
		req.Header.Set("Authorization", token)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("图片请求失败：HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 8<<20))
}

func (s *Server) request(method, version, route string, body any, dest any, token string) error {
	return s.requestContext(context.Background(), method, version, route, body, dest, token)
}

func (s *Server) requestContext(ctx context.Context, method, version, route string, body any, dest any, token string) error {
	fullURL := s.endpoint(version, route)
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trim-Client", "web")
	req.Header.Set("X-Trim-Client-Version", "631")
	req.Header.Set("authx", signature(req.URL, method, payload))
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("连接服务器失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		if apiIndex := strings.Index(resp.Request.URL.Path, "/api/"); apiIndex >= 0 {
			resolvedBase := *resp.Request.URL
			resolvedBase.Path = strings.TrimRight(resp.Request.URL.Path[:apiIndex], "/")
			resolvedBase.RawPath = ""
			resolvedBase.RawQuery = ""
			resolvedBase.Fragment = ""
			s.mu.Lock()
			s.baseURL = strings.TrimRight(resolvedBase.String(), "/")
			s.mu.Unlock()
		}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("服务器 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("服务器返回了无法识别的数据")
	}
	var apiError APIError
	if err := json.Unmarshal(data, &apiError); err == nil && apiError.Code != 0 && apiError.Code != 200 && apiError.Code != 2000 {
		return &apiError
	}
	return nil
}

func signature(endpoint *url.URL, method string, body []byte) string {
	nonce := strconv.Itoa(100000 + time.Now().Nanosecond()%900000)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	var digest string
	if strings.EqualFold(method, http.MethodGet) {
		values := endpoint.Query()
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		pairs := make(url.Values)
		for _, key := range keys {
			for _, value := range values[key] {
				pairs.Add(key, value)
			}
		}
		canonical := strings.ReplaceAll(pairs.Encode(), "+", "%20")
		if decoded, err := url.QueryUnescape(canonical); err == nil {
			canonical = decoded
		}
		digest = md5Hex([]byte(canonical))
	} else if len(body) > 0 {
		digest = md5Hex(body)
	} else {
		digest = md5Hex(nil)
	}
	message := strings.Join([]string{clientSignSeed, endpoint.EscapedPath(), nonce, timestamp, digest, clientAPIKey}, "_")
	return "nonce=" + nonce + "&timestamp=" + timestamp + "&sign=" + md5Hex([]byte(message))
}

func md5Hex(value []byte) string {
	sum := md5.Sum(value)
	return hex.EncodeToString(sum[:])
}

func unwrapData(value any) any {
	if m, ok := value.(map[string]any); ok {
		if data, exists := m["data"]; exists {
			return data
		}
	}
	return value
}

func normalizeItems(value any) []MediaItem {
	objects := findObjects(value)
	items := make([]MediaItem, 0, len(objects))
	seen := make(map[string]bool)
	for _, object := range objects {
		item := normalizeItem(object)
		if item.ID == "" || item.Title == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	return items
}

func normalizeItem(object map[string]any) MediaItem {
	item := MediaItem{Raw: object}
	item.ID = firstString(object, "guid", "id", "media_id", "mediaId")
	item.MediaID = firstString(object, "media_guid", "mediaGuid")
	item.Title = firstString(object, "title", "name", "original_title", "display_name", "file_name")
	item.Kind = strings.ToLower(firstString(object, "type", "media_type", "mediaType", "category"))
	if strings.Contains(item.Kind, "tv") || strings.Contains(item.Kind, "series") || firstString(object, "tv_title") != "" || valueAt(object, "season_number") != nil || valueAt(object, "episode_number") != nil {
		item.Kind = "tv"
	} else if item.Kind == "video" {
		item.Kind = "movie"
	}
	item.Year = firstString(object, "year", "release_date", "air_date")
	if len(item.Year) >= 4 {
		item.Year = item.Year[:4]
	}
	item.Rating = formatRating(firstString(object, "rating", "score", "vote_average"))
	item.Overview = firstString(object, "overview", "description", "summary", "plot")
	item.Poster = firstString(object, "poster", "posters", "poster_url", "posterUrl", "image", "image_url", "cover")
	item.Favorite = anyBool(object["favorite"]) || anyBool(object["is_favorite"]) || anyBool(object["isFavorite"])
	item.Watched = anyBool(object["is_watched"]) || anyBool(object["watched"])
	item.AddedAt = firstString(object, "create_time", "created_at", "ts", "release_date")
	return item
}

func formatRating(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	rating, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	return strconv.FormatFloat(rating, 'f', 1, 64)
}

func findObjects(value any) []map[string]any {
	switch current := value.(type) {
	case []any:
		out := make([]map[string]any, 0, len(current))
		for _, child := range current {
			out = append(out, findObjects(child)...)
		}
		return out
	case map[string]any:
		if _, hasTitle := current["title"]; hasTitle {
			return []map[string]any{current}
		}
		if _, hasID := current["guid"]; hasID {
			return []map[string]any{current}
		}
		keys := []string{"list", "items", "data", "results", "medias", "movies", "tv", "records", "rows"}
		for _, key := range keys {
			if nested, ok := current[key]; ok {
				if found := findObjects(nested); len(found) > 0 {
					return found
				}
			}
		}
	}
	return nil
}

func firstObject(value any) map[string]any {
	objects := findObjects(value)
	if len(objects) > 0 {
		return objects[0]
	}
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return nil
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			if text := anyString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func anyString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', 1, 64)
	case json.Number:
		return v.String()
	}
	return ""
}

func anyBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	}
	return false
}

func findItemsList(value any) any {
	if object, ok := value.(map[string]any); ok {
		if list, exists := object["list"]; exists {
			return list
		}
	}
	return value
}

func valueAt(value any, key string) any {
	if object, ok := value.(map[string]any); ok {
		return object[key]
	}
	return nil
}

func asInt(value any) (int64, bool) {
	switch n := value.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		value, err := n.Int64()
		return value, err == nil
	case int:
		return int64(n), true
	}
	return 0, false
}

func (m MediaItem) Subtitle() string {
	parts := make([]string, 0, 2)
	if m.Year != "" {
		parts = append(parts, m.Year)
	}
	if m.Rating != "" {
		parts = append(parts, "★ "+m.Rating)
	}
	return strings.Join(parts, " · ")
}

func (s *Server) imageURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	base, err := url.Parse(s.baseURLValue())
	if err != nil {
		return value
	}
	if strings.HasPrefix(value, "//") {
		return base.Scheme + ":" + value
	}
	basePath := strings.TrimRight(base.Path, "/")
	pathValue := value
	if strings.HasPrefix(pathValue, basePath+"/") {
		base.Path = pathValue
	} else {
		pathValue = strings.TrimLeft(pathValue, "/")
		if strings.HasPrefix(pathValue, "api/") {
			base.Path = basePath + "/" + pathValue
		} else if strings.HasPrefix(pathValue, "sys/img/") {
			base.Path = basePath + "/api/v1/" + pathValue
		} else {
			cleaned := strings.TrimPrefix(path.Clean("/"+pathValue), "/")
			base.Path = basePath + "/api/v1/sys/img/" + cleaned
		}
	}
	return base.String()
}
