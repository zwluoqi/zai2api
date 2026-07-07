package internal

import (
	"io"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

// z.ai 的 /api/v2/chat/completions 走 Aliyun ESA 反爬网关。浏览器请求会携带网关下发的
// acw_tc / cdn_sec_tc 等 cookie；缺少它们时请求会被拦截并返回 INTERNAL_ERROR、
// "客户端版本过旧" 等误导性错误。这里通过一次 warmup GET 获取并缓存这些 cookie。

const wafCookieTTL = 10 * time.Minute

var (
	wafCookieMu   sync.RWMutex
	wafCookieVal  string
	wafCookieTime time.Time
)

// GetWAFCookie 返回缓存的 WAF cookie 串（过期自动刷新，失败返回空串）。
func GetWAFCookie() string {
	wafCookieMu.RLock()
	if wafCookieVal != "" && time.Since(wafCookieTime) < wafCookieTTL {
		v := wafCookieVal
		wafCookieMu.RUnlock()
		return v
	}
	wafCookieMu.RUnlock()
	return refreshWAFCookie()
}

func refreshWAFCookie() string {
	client, err := TLSHTTPClient(15 * time.Second)
	if err != nil {
		return ""
	}
	req, err := fhttp.NewRequest("GET", "https://chat.z.ai/", nil)
	if err != nil {
		return ""
	}
	ApplyBrowserFetchHeaders(req.Header, false)
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	seen := map[string]bool{}
	var parts []string
	for _, c := range resp.Cookies() {
		if c.Name == "" || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		parts = append(parts, c.Name+"="+c.Value)
	}
	if len(parts) == 0 {
		return ""
	}
	v := strings.Join(parts, "; ")
	wafCookieMu.Lock()
	wafCookieVal = v
	wafCookieTime = time.Now()
	wafCookieMu.Unlock()
	return v
}
