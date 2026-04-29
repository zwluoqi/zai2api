package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VerificationMailbox interface {
	Address() string
	WaitToken(timeout, interval time.Duration) (string, error)
	Cleanup() error
}

func NewVerificationMailbox(opts *RegisterOptions) (VerificationMailbox, error) {
	switch opts.Provider {
	case "temp":
		client := NewHTTPClient(opts.Proxy)
		email, err := client.GetTempEmail()
		if err != nil {
			return nil, fmt.Errorf("获取临时邮箱失败: %w", err)
		}
		return &tempMailbox{client: client, email: email}, nil
	case "outlook":
		pool := NewOutlookAccountPool(opts.OutlookFile, opts.OutlookUsedFile)
		account, err := pool.TakeHealthy(opts.Proxy, 20)
		if err != nil {
			return nil, err
		}
		return NewOutlookMailbox(account, opts.Proxy), nil
	case "icloud":
		client := NewICloudClient(opts.ICloudAPIBase, opts.ICloudAPIToken, opts.ICloudAutoDelete, opts.Proxy)
		alias, err := client.CreateAlias()
		if err != nil {
			return nil, err
		}
		return &icloudMailbox{client: client, alias: alias, startedAt: time.Now().Add(-2 * time.Minute)}, nil
	default:
		return nil, fmt.Errorf("不支持的邮箱 provider: %s", opts.Provider)
	}
}

type tempMailbox struct {
	client *HTTPClient
	email  string
}

func (m *tempMailbox) Address() string { return m.email }

func (m *tempMailbox) WaitToken(timeout, interval time.Duration) (string, error) {
	return m.client.CheckEmail(m.email)
}

func (m *tempMailbox) Cleanup() error { return nil }

type OutlookAccount struct {
	Email        string
	Password     string
	ClientID     string
	RefreshToken string
}

type OutlookAccountPool struct {
	accountsFile string
	usedFile     string
	mu           sync.Mutex
	accounts     []OutlookAccount
}

func NewOutlookAccountPool(accountsFile, usedFile string) *OutlookAccountPool {
	p := &OutlookAccountPool{accountsFile: accountsFile, usedFile: usedFile}
	p.accounts = p.loadAccounts()
	return p
}

func (p *OutlookAccountPool) TakeHealthy(proxy string, maxSkips int) (*OutlookAccount, error) {
	for skipped := 0; skipped < maxSkips; skipped++ {
		account, ok := p.take()
		if !ok {
			return nil, fmt.Errorf("Outlook 账号池为空: %s", p.accountsFile)
		}
		fmt.Printf("[Outlook] 验证账号 %s ...\n", account.Email)
		if err := account.Verify(proxy); err != nil {
			fmt.Printf("[Outlook] 跳过坏号 %s: %v\n", account.Email, err)
			appendLine("outlook_bad.txt", fmt.Sprintf(`{"email":%q,"reason":%q,"ts":%d}`, account.Email, err.Error(), time.Now().Unix()))
			continue
		}
		return account, nil
	}
	return nil, fmt.Errorf("连续 %d 个 Outlook 账号验证失败，请检查账号文件或代理", maxSkips)
}

func (p *OutlookAccountPool) take() (*OutlookAccount, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.accounts) == 0 {
		return nil, false
	}
	account := p.accounts[0]
	p.accounts = p.accounts[1:]
	appendLine(p.usedFile, strings.ToLower(account.Email))
	return &account, true
}

func (p *OutlookAccountPool) loadAccounts() []OutlookAccount {
	used := loadUsedEmails(p.usedFile)
	items, err := parseOutlookAccounts(p.accountsFile)
	if err != nil {
		fmt.Printf("[Outlook] 读取账号文件失败: %v\n", err)
		return nil
	}
	out := make([]OutlookAccount, 0, len(items))
	for _, account := range items {
		if used[strings.ToLower(account.Email)] {
			continue
		}
		out = append(out, account)
	}
	fmt.Printf("[Outlook] 加载 %d 个可用账号，跳过已使用 %d 个\n", len(out), len(items)-len(out))
	return out
}

func parseOutlookAccounts(path string) ([]OutlookAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		var raw []map[string]string
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		out := make([]OutlookAccount, 0, len(raw))
		for _, item := range raw {
			account := OutlookAccount{
				Email:        firstNonEmpty(item["emailAddr"], item["email"]),
				Password:     item["password"],
				ClientID:     firstNonEmpty(item["clientId"], item["client_id"]),
				RefreshToken: firstNonEmpty(item["refreshToken"], item["refresh_token"]),
			}
			if account.Email != "" && account.ClientID != "" && account.RefreshToken != "" {
				out = append(out, account)
			}
		}
		return out, nil
	}
	var out []OutlookAccount
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "----")
		if len(parts) < 4 {
			continue
		}
		out = append(out, OutlookAccount{
			Email:        strings.TrimSpace(parts[0]),
			Password:     strings.TrimSpace(parts[1]),
			ClientID:     strings.TrimSpace(parts[2]),
			RefreshToken: strings.TrimSpace(parts[3]),
		})
	}
	return out, scanner.Err()
}

func loadUsedEmails(path string) map[string]bool {
	used := make(map[string]bool)
	data, err := os.ReadFile(path)
	if err != nil {
		return used
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		email := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if email != "" {
			used[email] = true
		}
	}
	return used
}

func appendLine(path, line string) {
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if filepath.Dir(path) == "." {
		_ = os.MkdirAll(".", 0755)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}

const (
	outlookIMAPHost = "outlook.office365.com:993"
	outlookTokenURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
)

func (a *OutlookAccount) Verify(proxy string) error {
	accessToken, newRefreshToken, err := refreshOutlookAccessToken(a.ClientID, a.RefreshToken, proxy)
	if err != nil {
		return fmt.Errorf("oauth_refresh: %w", err)
	}
	a.RefreshToken = newRefreshToken
	conn, err := newIMAPConn(outlookIMAPHost)
	if err != nil {
		return fmt.Errorf("imap_connect: %w", err)
	}
	defer conn.Close()
	if err := conn.AuthXOAUTH2(a.Email, accessToken); err != nil {
		return fmt.Errorf("imap_auth: %w", err)
	}
	if err := conn.Select("INBOX"); err != nil {
		return fmt.Errorf("imap_select: %w", err)
	}
	return nil
}

type outlookMailbox struct {
	account     *OutlookAccount
	proxy       string
	accessToken string
	seen        map[string]bool
	startedAt   time.Time
}

func NewOutlookMailbox(account *OutlookAccount, proxy string) *outlookMailbox {
	return &outlookMailbox{
		account:   account,
		proxy:     proxy,
		seen:      make(map[string]bool),
		startedAt: time.Now().Add(-2 * time.Minute),
	}
}

func (m *outlookMailbox) Address() string { return m.account.Email }

func (m *outlookMailbox) WaitToken(timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		token, err := m.pollOnce()
		if err != nil {
			fmt.Printf("[Outlook] 轮询异常: %v\n", err)
			m.accessToken = ""
		}
		if token != "" {
			return token, nil
		}
		fmt.Println("  等待 Outlook 验证邮件...")
		time.Sleep(interval)
	}
	return "", fmt.Errorf("等待 Outlook 验证邮件超时")
}

func (m *outlookMailbox) Cleanup() error { return nil }

func (m *outlookMailbox) ensureToken() (string, error) {
	if m.accessToken != "" {
		return m.accessToken, nil
	}
	accessToken, newRefreshToken, err := refreshOutlookAccessToken(m.account.ClientID, m.account.RefreshToken, m.proxy)
	if err != nil {
		return "", err
	}
	m.accessToken = accessToken
	m.account.RefreshToken = newRefreshToken
	return accessToken, nil
}

func (m *outlookMailbox) pollOnce() (string, error) {
	accessToken, err := m.ensureToken()
	if err != nil {
		return "", err
	}
	conn, err := newIMAPConn(outlookIMAPHost)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := conn.AuthXOAUTH2(m.account.Email, accessToken); err != nil {
		return "", err
	}
	for _, folder := range []string{"INBOX", "Junk"} {
		if err := conn.Select(folder); err != nil {
			continue
		}
		ids, err := conn.SearchAll()
		if err != nil {
			continue
		}
		sort.Ints(ids)
		for i := len(ids) - 1; i >= 0 && i >= len(ids)-10; i-- {
			key := fmt.Sprintf("%s:%d", folder, ids[i])
			if m.seen[key] {
				continue
			}
			m.seen[key] = true
			msg, err := conn.FetchRFC822(ids[i])
			if err != nil || msg == nil {
				continue
			}
			if !msg.Date.IsZero() && msg.Date.Before(m.startedAt) {
				continue
			}
			if !isRelevantZAIMail(msg.Text) {
				continue
			}
			if token := extractTokenFromEmail(msg.Text); token != "" {
				return token, nil
			}
		}
	}
	return "", nil
}

func refreshOutlookAccessToken(clientID, refreshToken, proxy string) (string, string, error) {
	accessToken, newRefreshToken, err := refreshOutlookAccessTokenOnce(clientID, refreshToken, proxy)
	if err == nil || proxy == "" {
		return accessToken, newRefreshToken, err
	}
	fmt.Printf("[Outlook] OAuth 代理请求失败，改为直连重试: %v\n", err)
	return refreshOutlookAccessTokenOnce(clientID, refreshToken, "")
}

func refreshOutlookAccessTokenOnce(clientID, refreshToken, proxy string) (string, string, error) {
	client := NewHTTPClient(proxy).client
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	req, err := http.NewRequest("POST", outlookTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("解析 OAuth 响应失败: %w", err)
	}
	if result.AccessToken == "" {
		if result.Description != "" {
			return "", "", fmt.Errorf("%s: %s", result.Error, result.Description)
		}
		return "", "", fmt.Errorf("OAuth 响应缺少 access_token: %s", string(body))
	}
	if result.RefreshToken == "" {
		result.RefreshToken = refreshToken
	}
	return result.AccessToken, result.RefreshToken, nil
}

type imapConn struct {
	conn   *tls.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	nextID int
}

func newIMAPConn(addr string) (*imapConn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	tlsConn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: "outlook.office365.com"})
	if err != nil {
		return nil, err
	}
	c := &imapConn{
		conn:   tlsConn,
		reader: bufio.NewReader(tlsConn),
		writer: bufio.NewWriter(tlsConn),
	}
	if _, err := c.readLine(); err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	return c, nil
}

func (c *imapConn) Close() error {
	_, _ = c.command("LOGOUT")
	return c.conn.Close()
}

func (c *imapConn) nextTag() string {
	c.nextID++
	return fmt.Sprintf("A%04d", c.nextID)
}

func (c *imapConn) readLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

func (c *imapConn) command(format string, args ...any) ([]string, error) {
	tag := c.nextTag()
	if _, err := fmt.Fprintf(c.writer, "%s %s\r\n", tag, fmt.Sprintf(format, args...)); err != nil {
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return nil, err
	}
	var lines []string
	for {
		line, err := c.readLine()
		if err != nil {
			return lines, err
		}
		lines = append(lines, line)
		if strings.HasPrefix(line, tag+" ") {
			if strings.Contains(line, " OK") {
				return lines, nil
			}
			return lines, fmt.Errorf("%s", line)
		}
	}
}

func (c *imapConn) AuthXOAUTH2(emailAddr, accessToken string) error {
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", emailAddr, accessToken)))
	_, err := c.command("AUTHENTICATE XOAUTH2 %s", auth)
	return err
}

func (c *imapConn) Select(folder string) error {
	_, err := c.command("SELECT %q", folder)
	return err
}

func (c *imapConn) SearchAll() ([]int, error) {
	lines, err := c.command("SEARCH ALL")
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, line := range lines {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}
		for _, part := range strings.Fields(strings.TrimPrefix(line, "* SEARCH")) {
			id, _ := strconv.Atoi(part)
			if id > 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

var literalRe = regexp.MustCompile(`\{(\d+)\}$`)

func (c *imapConn) FetchRFC822(id int) (*parsedMail, error) {
	tag := c.nextTag()
	if _, err := fmt.Fprintf(c.writer, "%s FETCH %d (RFC822)\r\n", tag, id); err != nil {
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return nil, err
	}
	var raw []byte
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		if matches := literalRe.FindStringSubmatch(line); len(matches) == 2 {
			size, _ := strconv.Atoi(matches[1])
			raw = make([]byte, size)
			if _, err := io.ReadFull(c.reader, raw); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(line, tag+" ") {
			if !strings.Contains(line, " OK") {
				return nil, fmt.Errorf("%s", line)
			}
			if len(raw) == 0 {
				return nil, nil
			}
			return parseRawMail(raw)
		}
	}
}

type parsedMail struct {
	Text string
	Date time.Time
}

func parseRawMail(raw []byte) (*parsedMail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	subject, _ := (&mime.WordDecoder{}).DecodeHeader(msg.Header.Get("Subject"))
	from, _ := (&mime.WordDecoder{}).DecodeHeader(msg.Header.Get("From"))
	date, _ := mail.ParseDate(msg.Header.Get("Date"))
	body, _ := io.ReadAll(msg.Body)
	text := decodeMIMEBody(msg.Header, body)
	return &parsedMail{
		Text: fmt.Sprintf("From: %s\nSubject: %s\n\n%s", from, subject, text),
		Date: date,
	}, nil
}

func decodeMIMEBody(header mail.Header, body []byte) string {
	contentType := header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		var plain, html string
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			partBody, _ := io.ReadAll(part)
			text := decodeBytes(part.Header.Get("Content-Transfer-Encoding"), partBody)
			partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			switch partType {
			case "text/plain":
				if plain == "" {
					plain = text
				}
			case "text/html":
				if html == "" {
					html = text
				}
			}
		}
		return firstNonEmpty(plain, html)
	}
	return decodeBytes(header.Get("Content-Transfer-Encoding"), body)
}

func decodeBytes(transferEncoding string, body []byte) string {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if err == nil {
			body = decoded
		}
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		if err == nil {
			body = decoded
		}
	}
	return string(body)
}

func isRelevantZAIMail(body string) bool {
	body = strings.ToLower(body)
	return strings.Contains(body, "z.ai") || strings.Contains(body, "chat.z.ai") || strings.Contains(body, "verify")
}

type ICloudAlias struct {
	AliasEmail  string
	AppleID     string
	AnonymousID string
}

type ICloudClient struct {
	apiBase    string
	apiToken   string
	autoDelete bool
	httpClient *HTTPClient
}

func NewICloudClient(apiBase, apiToken string, autoDelete bool, proxy string) *ICloudClient {
	return &ICloudClient{
		apiBase:    strings.TrimRight(apiBase, "/"),
		apiToken:   firstNonEmpty(apiToken, "icloud_extral_api4538"),
		autoDelete: autoDelete,
		httpClient: NewHTTPClient(proxy),
	}
}

func (c *ICloudClient) headers(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
}

func (c *ICloudClient) CreateAlias() (*ICloudAlias, error) {
	if c.apiBase == "" {
		return nil, fmt.Errorf("icloud.api_base 未配置")
	}
	payload := strings.NewReader(`{"channel":"zai"}`)
	req, err := http.NewRequest("POST", c.apiBase+"/api/external/icloud/alias/create", payload)
	if err != nil {
		return nil, err
	}
	c.headers(req)
	resp, err := c.httpClient.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success     bool   `json:"success"`
		AliasEmail  string `json:"alias_email"`
		AppleID     string `json:"apple_id"`
		AnonymousID string `json:"anonymous_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if !result.Success || result.AliasEmail == "" {
		return nil, fmt.Errorf("iCloud alias 创建失败: %s", string(body))
	}
	return &ICloudAlias{AliasEmail: result.AliasEmail, AppleID: result.AppleID, AnonymousID: result.AnonymousID}, nil
}

func (c *ICloudClient) ListEmails(alias *ICloudAlias) []map[string]any {
	payload := fmt.Sprintf(`{"apple_id":%q,"alias_email":%q,"folders":["INBOX","Junk"],"top":20,"skip":0}`, alias.AppleID, alias.AliasEmail)
	req, err := http.NewRequest("POST", c.apiBase+"/api/external/icloud/alias/emails", strings.NewReader(payload))
	if err != nil {
		return nil
	}
	c.headers(req)
	resp, err := c.httpClient.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool             `json:"success"`
		Value   []map[string]any `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.Success {
		return nil
	}
	return result.Value
}

func (c *ICloudClient) DeleteAlias(alias *ICloudAlias) error {
	if !c.autoDelete || alias == nil {
		return nil
	}
	payload := fmt.Sprintf(`{"apple_id":%q,"alias_email":%q}`, alias.AppleID, alias.AliasEmail)
	req, err := http.NewRequest("POST", c.apiBase+"/api/external/icloud/alias/delete", strings.NewReader(payload))
	if err != nil {
		return err
	}
	c.headers(req)
	resp, err := c.httpClient.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type icloudMailbox struct {
	client    *ICloudClient
	alias     *ICloudAlias
	seen      map[string]bool
	startedAt time.Time
}

func (m *icloudMailbox) Address() string { return m.alias.AliasEmail }

func (m *icloudMailbox) WaitToken(timeout, interval time.Duration) (string, error) {
	if m.seen == nil {
		m.seen = make(map[string]bool)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, msg := range m.client.ListEmails(m.alias) {
			id := fmt.Sprint(msg["id"])
			if id != "" && m.seen[id] {
				continue
			}
			if id != "" {
				m.seen[id] = true
			}
			content := fmt.Sprintf("From: %v\nSubject: %v\n\n%v\n%v\n%v",
				msg["from"], msg["subject"], msg["bodyPreview"], msg["body"], msg["content"])
			if !isRelevantZAIMail(content) {
				continue
			}
			if token := extractTokenFromEmail(content); token != "" {
				return token, nil
			}
		}
		fmt.Println("  等待 iCloud 验证邮件...")
		time.Sleep(interval)
	}
	return "", fmt.Errorf("等待 iCloud 验证邮件超时")
}

func (m *icloudMailbox) Cleanup() error {
	return m.client.DeleteAlias(m.alias)
}
