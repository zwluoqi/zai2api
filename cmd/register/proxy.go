package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func configureBrowserProxy(rawProxy string) (string, string, error) {
	proxyURL, err := url.Parse(rawProxy)
	if err != nil {
		return "", "", fmt.Errorf("代理地址格式错误: %w", err)
	}
	if proxyURL.Scheme == "" || proxyURL.Host == "" {
		return "", "", fmt.Errorf("代理地址需要包含协议和主机，例如 http://host:port")
	}
	server := proxyURL.Scheme + "://" + proxyURL.Host
	if proxyURL.User == nil {
		return server, "", nil
	}
	username := proxyURL.User.Username()
	password, _ := proxyURL.User.Password()
	extDir, err := os.MkdirTemp("", "zai-register-proxy-*")
	if err != nil {
		return "", "", err
	}
	manifest := `{
  "version": "1.0.0",
  "manifest_version": 2,
  "name": "zai-register-proxy-auth",
  "permissions": ["proxy", "tabs", "unlimitedStorage", "storage", "<all_urls>", "webRequest", "webRequestBlocking"],
  "background": {"scripts": ["background.js"]}
}`
	background := fmt.Sprintf(`
chrome.webRequest.onAuthRequired.addListener(
  function() {
    return {authCredentials: {username: %q, password: %q}};
  },
  {urls: ["<all_urls>"]},
  ["blocking"]
);
`, username, password)
	if err := os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		_ = os.RemoveAll(extDir)
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(extDir, "background.js"), []byte(strings.TrimSpace(background)), 0644); err != nil {
		_ = os.RemoveAll(extDir)
		return "", "", err
	}
	return server, extDir, nil
}
