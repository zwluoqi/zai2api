package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AdsPowerSession struct {
	ProfileID  string
	DebugPort  int
	ControlURL string
	apiBase    string
}

func NewAdsPowerSession(opts *RegisterOptions) (*AdsPowerSession, error) {
	apiBase := strings.TrimRight(opts.AdsPowerAPI, "/")
	if err := cleanupAdsPowerProfiles(apiBase, "zai-"); err != nil {
		fmt.Printf("AdsPower 清理旧环境失败: %v\n", err)
	}
	profileID, err := createAdsPowerProfile(apiBase, opts)
	if err != nil {
		return nil, err
	}
	debugPort, controlURL, err := startAdsPowerBrowser(apiBase, profileID)
	if err != nil {
		_ = deleteAdsPowerProfile(apiBase, profileID)
		return nil, err
	}
	return &AdsPowerSession{
		ProfileID:  profileID,
		DebugPort:  debugPort,
		ControlURL: controlURL,
		apiBase:    apiBase,
	}, nil
}

func (s *AdsPowerSession) Close() {
	if s == nil || s.ProfileID == "" {
		return
	}
	_, _ = adsPowerGet(s.apiBase, "/api/v1/browser/stop", map[string]string{"user_id": s.ProfileID})
	time.Sleep(time.Second)
	_ = deleteAdsPowerProfile(s.apiBase, s.ProfileID)
}

func createAdsPowerProfile(apiBase string, opts *RegisterOptions) (string, error) {
	fp := map[string]any{
		"automatic_timezone": "0",
		"timezone":           opts.FingerprintTZ,
		"ua":                 opts.FingerprintUA,
		"language":           []string{opts.FingerprintLang, strings.Split(opts.FingerprintLang, "-")[0], "en-US", "en"},
		"language_switch":    "1",
		"flash":              "block",
		"webrtc":             "disabled",
		"location":           "ask",
		"longitude":          "139.6917",
		"latitude":           "35.6895",
		"accuracy":           "1000",
	}
	payload := map[string]any{
		"name":               fmt.Sprintf("zai-register-%d", time.Now().Unix()),
		"group_id":           opts.AdsPowerGroupID,
		"user_proxy_config":  adsPowerProxyConfig(opts.Proxy),
		"fingerprint_config": fp,
	}
	data, err := adsPowerPost(apiBase, "/api/v1/user/create", payload)
	if err != nil {
		return "", err
	}
	id, _ := data["id"].(string)
	if id == "" {
		return "", fmt.Errorf("AdsPower 创建 profile 未返回 id: %v", data)
	}
	return id, nil
}

func startAdsPowerBrowser(apiBase, profileID string) (int, string, error) {
	data, err := adsPowerGet(apiBase, "/api/v1/browser/start", map[string]string{
		"user_id":   profileID,
		"open_tabs": "1",
		"headless":  "0",
	})
	if err != nil {
		return 0, "", err
	}
	controlURL := adsPowerControlURL(data)
	switch v := data["debug_port"].(type) {
	case float64:
		port := int(v)
		return resolveControlURL(port, controlURL)
	case string:
		var port int
		_, _ = fmt.Sscanf(v, "%d", &port)
		if port > 0 {
			return resolveControlURL(port, controlURL)
		}
	}
	return 0, "", fmt.Errorf("AdsPower 启动浏览器未返回 debug_port: %v", data)
}

func adsPowerControlURL(data map[string]any) string {
	if ws, ok := data["ws"].(map[string]any); ok {
		if puppeteer, _ := ws["puppeteer"].(string); strings.HasPrefix(puppeteer, "ws://") || strings.HasPrefix(puppeteer, "wss://") {
			return puppeteer
		}
	}
	for _, key := range []string{"ws_url", "webSocketDebuggerUrl", "websocket_url"} {
		if value, _ := data[key].(string); strings.HasPrefix(value, "ws://") || strings.HasPrefix(value, "wss://") {
			return value
		}
	}
	return ""
}

func resolveControlURL(debugPort int, controlURL string) (int, string, error) {
	if controlURL != "" {
		return debugPort, controlURL, nil
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/json/version", debugPort), nil)
	if err != nil {
		return 0, "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("获取 AdsPower DevTools 地址失败: %w", err)
	}
	defer resp.Body.Close()
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return 0, "", fmt.Errorf("解析 AdsPower DevTools 地址失败: %w", err)
	}
	if version.WebSocketDebuggerURL == "" {
		return 0, "", fmt.Errorf("AdsPower DevTools 未返回 webSocketDebuggerUrl")
	}
	return debugPort, version.WebSocketDebuggerURL, nil
}

func cleanupAdsPowerProfiles(apiBase, prefix string) error {
	data, err := adsPowerGet(apiBase, "/api/v1/user/list", map[string]string{"page_size": "100"})
	if err != nil {
		return err
	}
	raw, _ := data["list"].([]any)
	var ids []string
	for _, item := range raw {
		profile, _ := item.(map[string]any)
		name, _ := profile["name"].(string)
		id, _ := profile["user_id"].(string)
		if id != "" && strings.HasPrefix(name, prefix) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return adsPowerDeleteProfiles(apiBase, ids)
}

func deleteAdsPowerProfile(apiBase, profileID string) error {
	return adsPowerDeleteProfiles(apiBase, []string{profileID})
}

func adsPowerDeleteProfiles(apiBase string, ids []string) error {
	_, err := adsPowerPost(apiBase, "/api/v1/user/delete", map[string]any{"user_ids": ids})
	return err
}

func adsPowerProxyConfig(proxy string) map[string]string {
	if strings.TrimSpace(proxy) == "" {
		return map[string]string{"proxy_soft": "no_proxy"}
	}
	parsed, err := url.Parse(proxy)
	if err == nil && parsed.Hostname() != "" {
		cfg := map[string]string{
			"proxy_soft": "other",
			"proxy_type": strings.TrimSuffix(parsed.Scheme, "://"),
			"proxy_host": parsed.Hostname(),
			"proxy_port": parsed.Port(),
		}
		if cfg["proxy_type"] == "" {
			cfg["proxy_type"] = "http"
		}
		if parsed.User != nil {
			cfg["proxy_user"] = parsed.User.Username()
			if password, ok := parsed.User.Password(); ok {
				cfg["proxy_password"] = password
			}
		}
		return cfg
	}
	parts := strings.Split(proxy, ":")
	cfg := map[string]string{"proxy_soft": "other", "proxy_type": "http"}
	if len(parts) >= 2 {
		cfg["proxy_host"] = parts[0]
		cfg["proxy_port"] = parts[1]
	}
	if len(parts) >= 4 {
		cfg["proxy_user"] = parts[2]
		cfg["proxy_password"] = parts[3]
	}
	return cfg
}

func adsPowerGet(apiBase, path string, params map[string]string) (map[string]any, error) {
	u, _ := url.Parse(apiBase + path)
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeAdsPowerResponse(resp.Body)
}

func adsPowerPost(apiBase, path string, payload any) (map[string]any, error) {
	body, _ := json.Marshal(payload)
	resp, err := http.Post(apiBase+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeAdsPowerResponse(resp.Body)
}

func decodeAdsPowerResponse(reader io.Reader) (map[string]any, error) {
	body, _ := io.ReadAll(reader)
	var result struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 AdsPower 响应失败: %w, body=%s", err, string(body))
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("AdsPower API 失败: %s", firstNonEmpty(result.Msg, string(body)))
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	return result.Data, nil
}
