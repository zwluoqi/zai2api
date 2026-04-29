package internal

import (
	"fmt"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// DefaultBrowserProfile 与 tls-client 内置 Chrome_133 一致（ClientHello、HTTP/2 SETTINGS/优先级/伪头序等），便于 JA3/JA4 与 HTTP/2 指纹对齐。
var DefaultBrowserProfile = profiles.Chrome_133

// BrowserUserAgent 必须与 DefaultBrowserProfile 同源，勿随机替换，否则与 TLS 指纹不一致。
const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

var (
	chromeGetHeaderOrder = []string{
		"host", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
		"upgrade-insecure-requests", "user-agent", "accept",
		"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
		"accept-encoding", "accept-language",
	}
	chromePostHeaderOrder = []string{
		"host", "content-length", "sec-ch-ua", "sec-ch-ua-mobile",
		"sec-ch-ua-platform", "content-type", "user-agent", "accept",
		"origin", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest",
		"referer", "accept-encoding", "accept-language",
	}
)

var (
	tlsClientMu sync.Mutex
	tlsBySec    = map[int]tls_client.HttpClient{}
)

// TLSHTTPClient 返回按超时（秒）复用的 tls-client 实例；所有出站 HTTPS 应通过此处以统一指纹。
func TLSHTTPClient(timeout time.Duration) (tls_client.HttpClient, error) {
	sec := int(timeout.Round(time.Second) / time.Second)
	if sec < 1 {
		sec = 1
	}
	tlsClientMu.Lock()
	defer tlsClientMu.Unlock()
	if c, ok := tlsBySec[sec]; ok {
		return c, nil
	}
	c, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(),
		tls_client.WithTimeoutSeconds(sec),
		tls_client.WithClientProfile(DefaultBrowserProfile),
		tls_client.WithRandomTLSExtensionOrder(),
	)
	if err != nil {
		return nil, fmt.Errorf("tls-client: %w", err)
	}
	tlsBySec[sec] = c
	return c, nil
}

// TLSHTTPClientWithProxy returns a non-shared tls-client using the same browser profile plus an optional proxy.
func TLSHTTPClientWithProxy(timeout time.Duration, proxyURL string) (tls_client.HttpClient, error) {
	sec := int(timeout.Round(time.Second) / time.Second)
	if sec < 1 {
		sec = 1
	}
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(sec),
		tls_client.WithClientProfile(DefaultBrowserProfile),
		tls_client.WithRandomTLSExtensionOrder(),
	}
	if proxyURL != "" {
		options = append(options, tls_client.WithProxyUrl(proxyURL))
	}
	c, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("tls-client: %w", err)
	}
	return c, nil
}

// ApplyBrowserFingerprintHeaders 设置与 Chrome 133 一致的 User-Agent 与 Sec-CH-UA（配合 TLSHTTPClient 的 TLS/H2 指纹）。
func ApplyBrowserFingerprintHeaders(h fhttp.Header) {
	h.Set("User-Agent", BrowserUserAgent)
	h.Set("sec-ch-ua", `"Google Chrome";v="133", "Chromium";v="133", "Not A(Brand";v="24"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua-platform", `"Windows"`)
}

// ApplyBrowserFetchHeaders adds the request headers browsers attach around fetch/XHR requests.
// It should be used together with TLSHTTPClient so TLS, HTTP/2, UA, and headers stay coherent.
func ApplyBrowserFetchHeaders(h fhttp.Header, isPost bool) {
	if isPost {
		h[fhttp.HeaderOrderKey] = chromePostHeaderOrder
	} else {
		h[fhttp.HeaderOrderKey] = chromeGetHeaderOrder
	}
	ApplyBrowserFingerprintHeaders(h)
	if h.Get("Accept") == "" {
		h.Set("Accept", "*/*")
	}
	if h.Get("Accept-Language") == "" {
		h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	}
	if h.Get("Sec-Fetch-Site") == "" {
		h.Set("Sec-Fetch-Site", "same-origin")
	}
	if h.Get("Sec-Fetch-Mode") == "" {
		h.Set("Sec-Fetch-Mode", "cors")
	}
	if h.Get("Sec-Fetch-Dest") == "" {
		h.Set("Sec-Fetch-Dest", "empty")
	}
	if h.Get("DNT") == "" {
		h.Set("DNT", "1")
	}
}
