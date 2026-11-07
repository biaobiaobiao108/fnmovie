package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FN Connect is the official fnOS relay lookup used by the FN ID login mode.
// The endpoint is public and authenticates the client with the same two
// signatures the official web client uses.
const (
	fnConnectEndpoint = "https://5ddd.com/api/v1/fn/con"
	fnConnectPath     = "/api/v1/fn/con"
	fnConnectPrefix   = "NDzZTVxnRKP8Z0jXg1VAMonaG8akvh"
	fnConnectAPIKey   = "zIGtkc3dqZnJpd29qZXJqa2w7c"
	fnConnectHost     = "5ddd.com"
)

type FNIDResult struct {
	ServerURL string
	Relay     bool
}

type fnConnectResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data map[string]any `json:"data"`
}

func normalizeFNIDInput(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil {
			raw = strings.Trim(parsed.Path, "/")
		}
	} else if strings.Contains(raw, "/") {
		parts := strings.Split(strings.Trim(raw, "/"), "/")
		if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
			raw = parts[1]
		} else if len(parts) > 0 {
			raw = parts[0]
		}
	}
	if index := strings.Index(raw, "/"); index >= 0 {
		raw = raw[:index]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func isValidFNID(fnID string) bool {
	if len(fnID) < 6 || len(fnID) > 30 || fnID[0] < 'a' || fnID[0] > 'z' || fnID[len(fnID)-1] == '-' {
		return false
	}
	if strings.Contains(fnID, "--") {
		return false
	}
	for _, r := range fnID {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func fnConnectHeaders(fnID string, timestamp int64, nonce string, body []byte) (authx, fnSign string) {
	bodyDigest := md5Hex(body)
	signSource := strings.Join([]string{
		fnConnectPrefix, fnConnectPath, nonce, strconv.FormatInt(timestamp, 10), bodyDigest, fnConnectAPIKey,
	}, "_")
	authx = fmt.Sprintf("nonce=%s&timestamp=%d&sign=%s", nonce, timestamp, md5Hex([]byte(signSource)))
	fnSign = sha256Hex([]byte(fmt.Sprintf("trim_connect`%s`%d`anna", fnID, timestamp)))
	return authx, fnSign
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ResolveFNID asks FN Connect for the access addresses of a FN ID and returns
// the first usable server URL with the fnOS API prefix.
func ResolveFNID(ctx context.Context, raw string) (FNIDResult, error) {
	fnID := normalizeFNIDInput(raw)
	if !isValidFNID(fnID) {
		return FNIDResult{}, errors.New("FN ID 格式不正确（6-30 位小写字母、数字或连字符，并以字母开头）")
	}
	body, err := json.Marshal(map[string]string{"fnId": fnID})
	if err != nil {
		return FNIDResult{}, err
	}
	timestamp := time.Now().UnixMilli()
	nonce := strconv.FormatInt(100000+time.Now().UnixNano()%900000, 10)
	authx, fnSign := fnConnectHeaders(fnID, timestamp, nonce, body)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fnConnectEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return FNIDResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("authx", authx)
	request.Header.Set("fn-sign", fnSign)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) FnMovie")
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return FNIDResult{}, fmt.Errorf("连接 FN Connect 失败：%w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return FNIDResult{}, fmt.Errorf("读取 FN Connect 响应失败：%w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return FNIDResult{}, fmt.Errorf("FN Connect 返回 HTTP %d", response.StatusCode)
	}
	var payload fnConnectResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return FNIDResult{}, fmt.Errorf("解析 FN Connect 响应失败：%w", err)
	}
	if payload.Code != 0 {
		message := strings.TrimSpace(payload.Msg)
		if message == "" {
			message = "未知错误"
		}
		if payload.Code == 3000006 {
			return FNIDResult{}, fmt.Errorf("未找到 FN ID“%s”，请检查是否输入正确", fnID)
		}
		return FNIDResult{}, fmt.Errorf("FN Connect：%s（%d）", message, payload.Code)
	}
	result, ok := fnConnectServerURL(payload.Data)
	if !ok {
		return FNIDResult{}, errors.New("FN Connect 没有返回可用的访问地址")
	}
	return result, nil
}

func fnConnectServerURL(data map[string]any) (FNIDResult, bool) {
	port := mapOf(data["port"])
	httpPort := firstInt(port, "httpPort", "http_port", "http")
	httpsPort := firstInt(port, "httpsPort", "https_port", "https")
	if httpsPort <= 0 {
		httpsPort = 443
	}
	// The official relay is the most reliable address and already terminates
	// TLS, so prefer it over a possibly stale public IP or DDNS name.
	for _, host := range stringsOf(data["fn"]) {
		if value, ok := relayServerURL(host); ok {
			return FNIDResult{ServerURL: value, Relay: true}, true
		}
	}
	for _, host := range stringsOf(data["publicIpv4"]) {
		if value := hostServerURL(host, "http", httpPort); value != "" {
			return FNIDResult{ServerURL: value}, true
		}
	}
	for _, host := range stringsOf(data["publicIpv6"]) {
		if value := hostServerURL(host, "http", httpPort); value != "" {
			return FNIDResult{ServerURL: value}, true
		}
	}
	for _, host := range stringsOf(data["ddns"]) {
		if value := hostServerURL(host, "https", httpsPort); value != "" {
			return FNIDResult{ServerURL: value}, true
		}
	}
	return FNIDResult{}, false
}

func relayServerURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", false
	}
	parsed.Path = "/v"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), true
}

func hostServerURL(host, scheme string, port int) string {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || port <= 0 {
		return ""
	}
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return ""
		}
		host = parsed.Hostname()
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s://%s/v", scheme, net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port)))
}

func mapOf(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return nil
}

func stringsOf(value any) []string {
	array, ok := value.([]any)
	if !ok {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return []string{text}
		}
		return nil
	}
	values := make([]string, 0, len(array))
	for _, item := range array {
		if text := anyString(item); text != "" {
			values = append(values, text)
		}
	}
	return values
}
