package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

func HandleAdminProxies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entries := GetCaptchaProxyEntries()
		enabled := 0
		healthy := 0
		for _, e := range entries {
			if e.Enabled {
				enabled++
			}
			if e.ExitIP != "" && e.LastError == "" {
				healthy++
			}
		}
		writeJSON(w, map[string]any{
			"enabled": GetCaptchaProxyPoolEnabled(),
			"items":   entries,
			"stats": map[string]int{
				"total":   len(entries),
				"active":  enabled,
				"healthy": healthy,
			},
		})
	case http.MethodPost:
		var req struct {
			URLs []string `json:"urls"`
			URL  string   `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		urls := append([]string{}, req.URLs...)
		if req.URL != "" {
			urls = append(urls, req.URL)
		}
		added, err := AddProxyEntries(urls)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "added": added})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func HandleAdminProxyDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := DeleteProxyEntry(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func HandleAdminProxyToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL     string `json:"url"`
		Enabled *bool  `json:"enabled"`
		Pool    *bool  `json:"pool"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Pool != nil {
		on := *req.Pool
		if err := UpdateRuntimeSettings(RuntimeSettingsPatch{CaptchaProxyPoolEnabled: &on}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "enabled": on})
		return
	}
	if req.Enabled == nil {
		http.Error(w, "missing enabled", http.StatusBadRequest)
		return
	}
	if err := SetProxyEntryEnabled(req.URL, *req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func HandleAdminProxyCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
		All bool   `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.All {
		n := CheckAllProxies()
		writeJSON(w, map[string]any{"ok": true, "checked": n})
		return
	}
	entry, err := CheckProxyEntry(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "item": entry})
}

func AddProxyEntries(urls []string) (int, error) {
	parsed := normalizeProxyList(urls)
	if len(parsed) == 0 {
		return 0, fmt.Errorf("没有有效的代理地址")
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	seen := map[string]bool{}
	for _, e := range Cfg.CaptchaProxyPool {
		seen[e.URL] = true
	}
	added := 0
	for _, u := range parsed {
		if seen[u] {
			continue
		}
		seen[u] = true
		Cfg.CaptchaProxyPool = append(Cfg.CaptchaProxyPool, ProxyEntry{URL: u, Enabled: true})
		added++
	}
	return added, persistProxyPoolLocked()
}

func DeleteProxyEntry(url string) error {
	url = strings.TrimSpace(url)
	settingsMu.Lock()
	defer settingsMu.Unlock()
	out := make([]ProxyEntry, 0, len(Cfg.CaptchaProxyPool))
	for _, e := range Cfg.CaptchaProxyPool {
		if e.URL != url {
			out = append(out, e)
		}
	}
	Cfg.CaptchaProxyPool = out
	return persistProxyPoolLocked()
}

func SetProxyEntryEnabled(url string, enabled bool) error {
	url = strings.TrimSpace(url)
	settingsMu.Lock()
	defer settingsMu.Unlock()
	found := false
	for i := range Cfg.CaptchaProxyPool {
		if Cfg.CaptchaProxyPool[i].URL == url {
			Cfg.CaptchaProxyPool[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("代理不存在")
	}
	return persistProxyPoolLocked()
}

func CheckProxyEntry(url string) (ProxyEntry, error) {
	url = strings.TrimSpace(url)
	result := probeProxy(url)
	settingsMu.Lock()
	defer settingsMu.Unlock()
	for i := range Cfg.CaptchaProxyPool {
		if Cfg.CaptchaProxyPool[i].URL != url {
			continue
		}
		applyProbe(&Cfg.CaptchaProxyPool[i], result)
		_ = persistProxyPoolLocked()
		return Cfg.CaptchaProxyPool[i], nil
	}
	return ProxyEntry{}, fmt.Errorf("代理不存在")
}

func CheckAllProxies() int {
	entries := GetCaptchaProxyEntries()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, e := range entries {
		e := e
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, _ = CheckProxyEntry(e.URL)
		}()
	}
	wg.Wait()
	return len(entries)
}

type proxyProbe struct {
	ok        bool
	ip        string
	region    string
	latencyMS int64
	err       string
}

func applyProbe(e *ProxyEntry, p proxyProbe) {
	e.LastCheck = time.Now().Unix()
	e.LatencyMS = p.latencyMS
	if p.ok {
		e.Success++
		e.ExitIP = p.ip
		e.Region = p.region
		e.LastError = ""
		return
	}
	e.Fail++
	e.LastError = p.err
}

func probeProxy(proxyURL string) proxyProbe {
	start := time.Now()
	client, err := TLSHTTPClientWithProxy(12*time.Second, proxyURL)
	if err != nil {
		return proxyProbe{err: err.Error(), latencyMS: time.Since(start).Milliseconds()}
	}
	req, err := fhttp.NewRequest("GET", "https://api.ipify.org", nil)
	if err != nil {
		return proxyProbe{err: err.Error()}
	}
	req.Header.Set("User-Agent", BrowserUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return proxyProbe{err: err.Error(), latencyMS: time.Since(start).Milliseconds()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
	ip := strings.TrimSpace(string(body))
	lat := time.Since(start).Milliseconds()
	if resp.StatusCode != http.StatusOK || ip == "" {
		return proxyProbe{err: fmt.Sprintf("HTTP %d", resp.StatusCode), latencyMS: lat}
	}
	region := lookupIPRegion(ip)
	return proxyProbe{ok: true, ip: ip, region: region, latencyMS: lat}
}

func lookupIPRegion(ip string) string {
	client, err := TLSHTTPClient(8 * time.Second)
	if err != nil {
		return ""
	}
	req, err := fhttp.NewRequest("GET", "http://ip-api.com/json/"+ip+"?fields=status,country,city", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var parsed struct {
		Status  string `json:"status"`
		Country string `json:"country"`
		City    string `json:"city"`
	}
	if json.NewDecoder(resp.Body).Decode(&parsed) != nil || parsed.Status != "success" {
		return ""
	}
	if parsed.City != "" && parsed.Country != "" {
		return parsed.Country + " / " + parsed.City
	}
	return parsed.Country
}
