package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TokenInfo 存储单个 token 的信息
type TokenInfo struct {
	Token         string    `json:"token"`
	Email         string    `json:"email"`
	UserID        string    `json:"user_id"`
	Valid         bool      `json:"valid"`
	LastChecked   time.Time `json:"last_checked"`
	UseCount      int64     `json:"use_count"`
	InvalidReason string    `json:"invalid_reason"`
}

type PublicTokenInfo struct {
	TokenPreview  string    `json:"token_preview"`
	Token         string    `json:"token,omitempty"`
	Email         string    `json:"email"`
	UserID        string    `json:"user_id"`
	Valid         bool      `json:"valid"`
	LastChecked   time.Time `json:"last_checked"`
	UseCount      int64     `json:"use_count"`
	InvalidReason string    `json:"invalid_reason"`
	Source        string    `json:"source"`
}

// TokenManager 管理所有用户 token
type TokenManager struct {
	mu              sync.RWMutex
	tokens          map[string]*TokenInfo // token -> TokenInfo
	invalidTokens   map[string]*TokenInfo // tokens_invalid.txt 里的 token
	validTokens     []string              // 有效 token 列表
	currentIndex    int                   // 轮询索引
	dataDir         string
	watcher         *fsnotify.Watcher
	checkInterval   time.Duration
	stopChan        chan struct{}
	multimodalCount int64 // 多模态请求计数
	totalCalls      int64 // 累计调用次数
	successCalls    int64 // 成功调用次数
}

var (
	tokenManager *TokenManager
	tokenOnce    sync.Once
)

// GetTokenManager 获取单例 TokenManager
func GetTokenManager() *TokenManager {
	tokenOnce.Do(func() {
		tokenManager = &TokenManager{
			tokens:        make(map[string]*TokenInfo),
			invalidTokens: make(map[string]*TokenInfo),
			validTokens:   make([]string, 0),
			dataDir:       "data",
			checkInterval: 5 * time.Minute, // 每5分钟检查一次
			stopChan:      make(chan struct{}),
		}
	})
	return tokenManager
}

// Start 启动 token 管理器
func (tm *TokenManager) Start() error {
	// 确保 data 目录存在
	if err := os.MkdirAll(tm.dataDir, 0755); err != nil {
		return fmt.Errorf("创建 data 目录失败: %v", err)
	}

	// 初始加载 token
	if err := tm.loadTokens(); err != nil {
		LogWarn("初始加载 token 失败: %v", err)
	}

	// 启动文件监听
	if err := tm.startWatcher(); err != nil {
		LogWarn("启动文件监听失败: %v", err)
	}

	// 启动定期验证
	go tm.startValidator()

	LogInfo("TokenManager 已启动，当前有效 token 数: %d", len(tm.validTokens))
	return nil
}

// Stop 停止 token 管理器
func (tm *TokenManager) Stop() {
	close(tm.stopChan)
	if tm.watcher != nil {
		tm.watcher.Close()
	}
}

// loadTokens 从 data 目录加载所有 token
func (tm *TokenManager) loadTokens() error {
	tokenFile := filepath.Join(tm.dataDir, "tokens.txt")

	file, err := os.Open(tokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			// 创建示例文件
			tm.createExampleTokenFile(tokenFile)
		} else {
			return err
		}
	}
	if file != nil {
		defer file.Close()
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 保留旧的统计数据
	oldTokens := tm.tokens
	oldInvalidTokens := tm.invalidTokens
	tm.tokens = make(map[string]*TokenInfo)
	tm.invalidTokens = make(map[string]*TokenInfo)
	tm.validTokens = make([]string, 0)
	activeSeen := make(map[string]bool)

	var scanErr error
	if file != nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			token := normalizeTokenLine(scanner.Text())
			if token == "" {
				continue
			}
			info := tm.reuseOrCreateTokenInfo(token, oldTokens, oldInvalidTokens)
			info.Valid = true
			info.InvalidReason = ""
			tm.tokens[token] = info
			tm.validTokens = append(tm.validTokens, token)
			activeSeen[token] = true
		}
		scanErr = scanner.Err()
	}

	for token, item := range readInvalidTokenFile(filepath.Join(tm.dataDir, "tokens_invalid.txt")) {
		if activeSeen[token] {
			continue
		}
		info := tm.reuseOrCreateTokenInfo(token, oldInvalidTokens, oldTokens)
		info.Valid = false
		if item.Reason != "" {
			info.InvalidReason = item.Reason
		} else if info.InvalidReason == "" {
			info.InvalidReason = "历史失效记录未保存原因"
		}
		tm.invalidTokens[token] = info
	}

	LogInfo("已加载 %d 个有效 token，%d 个失效 token", len(tm.validTokens), len(tm.invalidTokens))
	return scanErr
}

func normalizeTokenLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if strings.HasPrefix(line, "token=") {
		line = strings.TrimPrefix(line, "token=")
	}
	return strings.TrimSpace(line)
}

func (tm *TokenManager) reuseOrCreateTokenInfo(token string, sources ...map[string]*TokenInfo) *TokenInfo {
	for _, source := range sources {
		if info, exists := source[token]; exists {
			return info
		}
	}
	info := &TokenInfo{
		Token: token,
		Valid: true,
	}
	if payload, err := DecodeJWTPayload(token); err == nil && payload != nil {
		info.Email = payload.Email
		info.UserID = payload.ID
	}
	return info
}

// createExampleTokenFile 创建示例 token 文件
func (tm *TokenManager) createExampleTokenFile(path string) {
	content := `# 用户 Token 文件
# 每行一个 token，支持以下格式：
# 1. 直接写 token
# 2. token=xxx 格式
# 以 # 开头的行为注释

# 示例:
# eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.xxxxx
`
	os.WriteFile(path, []byte(content), 0644)
	LogInfo("已创建示例 token 文件: %s", path)
}

// startWatcher 启动文件变化监听
func (tm *TokenManager) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	tm.watcher = watcher

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					if strings.HasSuffix(event.Name, "tokens.txt") || strings.HasSuffix(event.Name, "tokens_invalid.txt") {
						LogInfo("检测到 token 文件变化，重新加载...")
						time.Sleep(100 * time.Millisecond) // 等待文件写入完成
						tm.loadTokens()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				LogError("文件监听错误: %v", err)
			case <-tm.stopChan:
				return
			}
		}
	}()

	return watcher.Add(tm.dataDir)
}

// startValidator 启动定期验证
func (tm *TokenManager) startValidator() {
	// 首次延迟验证
	time.Sleep(10 * time.Second)
	tm.validateAllTokens()

	ticker := time.NewTicker(tm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tm.validateAllTokens()
		case <-tm.stopChan:
			return
		}
	}
}

// validateAllTokens 验证所有 token
func (tm *TokenManager) validateAllTokens() {
	tm.mu.RLock()
	tokens := make([]string, 0, len(tm.tokens))
	for token := range tm.tokens {
		tokens = append(tokens, token)
	}
	tm.mu.RUnlock()

	LogInfo("开始验证 %d 个 token...", len(tokens))
	invalidCount := 0

	for _, token := range tokens {
		result := tm.validateToken(token)
		tm.mu.Lock()
		if info, exists := tm.tokens[token]; exists {
			info.Valid = result.Valid
			info.InvalidReason = result.Reason
			if result.Email != "" {
				info.Email = result.Email
			}
			if result.UserID != "" {
				info.UserID = result.UserID
			}
			info.LastChecked = time.Now()
			if !result.Valid {
				invalidCount++
			}
		}
		tm.mu.Unlock()
		time.Sleep(500 * time.Millisecond) // 避免请求过快
	}

	// 更新有效 token 列表
	tm.rebuildValidTokens()
	LogInfo("Token 验证完成，失效 %d 个，剩余有效 %d 个", invalidCount, len(tm.validTokens))

	// 自动删除失效 token
	if invalidCount > 0 {
		tm.removeInvalidTokens()
	}
}

type tokenValidationResult struct {
	Valid  bool
	Reason string
	Email  string
	UserID string
}

// validateToken 验证单个 token
func (tm *TokenManager) validateToken(token string) tokenValidationResult {
	req, err := http.NewRequest("GET", "https://chat.z.ai/api/v1/auths/", nil)
	if err != nil {
		return tokenValidationResult{Reason: "创建验证请求失败: " + err.Error()}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36 Edg/142.0.0.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DNT", "1")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://chat.z.ai/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Microsoft Edge";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Linux"`)
	req.Header.Set("sec-gpc", "1")
	req.AddCookie(&http.Cookie{Name: "token", Value: token})

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		LogDebug("Token 验证请求失败: %v", err)
		return tokenValidationResult{Reason: "验证请求失败: " + err.Error()}
	}
	defer resp.Body.Close()

	// 读取响应
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		LogDebug("Token 验证失败，状态码: %d", resp.StatusCode)
		reason := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if text := strings.TrimSpace(string(body)); text != "" {
			reason += ": " + truncateString(text, 300)
		}
		return tokenValidationResult{Reason: reason}
	}

	// 尝试解析响应获取新 token
	var authResp struct {
		Token string `json:"token"`
		Email string `json:"email"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal(body, &authResp); err == nil && authResp.Token != "" {
		return tokenValidationResult{
			Valid:  true,
			Email:  authResp.Email,
			UserID: authResp.ID,
		}
	}

	return tokenValidationResult{Valid: true}
}

// rebuildValidTokens 重建有效 token 列表
func (tm *TokenManager) rebuildValidTokens() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.rebuildValidTokensLocked()
}

func (tm *TokenManager) rebuildValidTokensLocked() {
	tm.validTokens = make([]string, 0)
	for token, info := range tm.tokens {
		if info.Valid {
			tm.validTokens = append(tm.validTokens, token)
		}
	}
}

// removeInvalidTokens 从文件中移除失效 token
func (tm *TokenManager) removeInvalidTokens() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 收集失效 token
	var invalidTokens []string
	for token, info := range tm.tokens {
		if !info.Valid {
			invalidTokens = append(invalidTokens, token)
			tm.invalidTokens[token] = info
			delete(tm.tokens, token)
		}
	}

	if len(invalidTokens) == 0 {
		return
	}

	tm.writeInvalidTokensLocked()
	tm.writeCurrentTokensLocked("自动更新", "# 失效 token 已移至 tokens_invalid.txt\n")
	LogInfo("已移除 %d 个失效 token 到 %s", len(invalidTokens), filepath.Join(tm.dataDir, "tokens_invalid.txt"))
}

// GetToken 获取一个有效 token（轮询）
func (tm *TokenManager) GetToken() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.validTokens) == 0 {
		return ""
	}

	token := tm.validTokens[tm.currentIndex%len(tm.validTokens)]
	tm.currentIndex++

	// 增加使用计数
	if info, exists := tm.tokens[token]; exists {
		info.UseCount++
	}

	return token
}

func (tm *TokenManager) ListTokens(includeRaw bool) []PublicTokenInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]PublicTokenInfo, 0, len(tm.tokens)+len(tm.invalidTokens))
	for token, info := range tm.tokens {
		out = append(out, publicTokenInfo(token, info, includeRaw, "active"))
	}
	for token, info := range tm.invalidTokens {
		out = append(out, publicTokenInfo(token, info, includeRaw, "invalid"))
	}
	return out
}

func publicTokenInfo(token string, info *TokenInfo, includeRaw bool, source string) PublicTokenInfo {
	item := PublicTokenInfo{
		TokenPreview:  previewToken(token),
		Email:         info.Email,
		UserID:        info.UserID,
		Valid:         info.Valid,
		LastChecked:   info.LastChecked,
		UseCount:      info.UseCount,
		InvalidReason: info.InvalidReason,
		Source:        source,
	}
	if includeRaw {
		item.Token = token
	}
	return item
}

func (tm *TokenManager) AddToken(token string) error {
	token = strings.TrimSpace(strings.TrimPrefix(token, "token="))
	if token == "" {
		return fmt.Errorf("token 为空")
	}
	tokenFile := filepath.Join(tm.dataDir, "tokens.txt")
	f, err := os.OpenFile(tokenFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(token + "\n"); err != nil {
		return err
	}
	tm.mu.Lock()
	delete(tm.invalidTokens, token)
	tm.writeInvalidTokensLocked()
	tm.mu.Unlock()
	return tm.loadTokens()
}

func (tm *TokenManager) DeleteToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token 为空")
	}
	tm.mu.Lock()
	delete(tm.tokens, token)
	delete(tm.invalidTokens, token)
	tm.writeInvalidTokensLocked()
	tm.mu.Unlock()
	tm.rebuildValidTokens()
	return tm.writeCurrentTokens()
}

func (tm *TokenManager) DeleteTokenByPreview(preview string) error {
	tm.mu.RLock()
	var token string
	for candidate := range tm.tokens {
		if previewToken(candidate) == preview {
			token = candidate
			break
		}
	}
	if token == "" {
		for candidate := range tm.invalidTokens {
			if previewToken(candidate) == preview {
				token = candidate
				break
			}
		}
	}
	tm.mu.RUnlock()
	if token == "" {
		return fmt.Errorf("未找到 token: %s", preview)
	}
	return tm.DeleteToken(token)
}

func (tm *TokenManager) RestoreToken(token string) error {
	token = strings.TrimSpace(strings.TrimPrefix(token, "token="))
	if token == "" {
		return fmt.Errorf("token 为空")
	}
	tm.mu.Lock()
	delete(tm.invalidTokens, token)
	tm.writeInvalidTokensLocked()
	tm.mu.Unlock()
	return tm.AddToken(token)
}

func (tm *TokenManager) RestoreTokenByPreview(preview string) error {
	tm.mu.RLock()
	var token string
	for candidate := range tm.invalidTokens {
		if previewToken(candidate) == preview {
			token = candidate
			break
		}
	}
	tm.mu.RUnlock()
	if token == "" {
		return fmt.Errorf("未找到 token: %s", preview)
	}
	return tm.RestoreToken(token)
}

func (tm *TokenManager) TestToken(token string) (PublicTokenInfo, error) {
	token = strings.TrimSpace(strings.TrimPrefix(token, "token="))
	if token == "" {
		return PublicTokenInfo{}, fmt.Errorf("token 为空")
	}
	tm.mu.RLock()
	_, active := tm.tokens[token]
	_, invalid := tm.invalidTokens[token]
	tm.mu.RUnlock()
	if !active && !invalid {
		return PublicTokenInfo{}, fmt.Errorf("未找到 token")
	}
	result := tm.validateToken(token)
	return tm.applyValidationResult(token, result), nil
}

func (tm *TokenManager) TestTokenByPreview(preview string) (PublicTokenInfo, error) {
	tm.mu.RLock()
	var token string
	for candidate := range tm.tokens {
		if previewToken(candidate) == preview {
			token = candidate
			break
		}
	}
	if token == "" {
		for candidate := range tm.invalidTokens {
			if previewToken(candidate) == preview {
				token = candidate
				break
			}
		}
	}
	tm.mu.RUnlock()
	if token == "" {
		return PublicTokenInfo{}, fmt.Errorf("未找到 token: %s", preview)
	}
	return tm.TestToken(token)
}

func (tm *TokenManager) applyValidationResult(token string, result tokenValidationResult) PublicTokenInfo {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	info, active := tm.tokens[token]
	source := "active"
	if !active {
		info = tm.invalidTokens[token]
		source = "invalid"
	}
	if info == nil {
		info = tm.reuseOrCreateTokenInfo(token)
	}
	info.Valid = result.Valid
	info.InvalidReason = result.Reason
	info.LastChecked = time.Now()
	if result.Email != "" {
		info.Email = result.Email
	}
	if result.UserID != "" {
		info.UserID = result.UserID
	}

	if result.Valid {
		delete(tm.invalidTokens, token)
		tm.tokens[token] = info
		source = "active"
	} else {
		delete(tm.tokens, token)
		tm.invalidTokens[token] = info
		source = "invalid"
	}
	tm.rebuildValidTokensLocked()
	tm.writeCurrentTokensLocked("单账号测试", "")
	tm.writeInvalidTokensLocked()
	return publicTokenInfo(token, info, true, source)
}

func (tm *TokenManager) ValidateNow() {
	tm.validateAllTokens()
}

func (tm *TokenManager) writeCurrentTokens() error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.writeCurrentTokensLocked("管理面板更新", "")
}

func (tm *TokenManager) writeCurrentTokensLocked(reason, extraComment string) error {
	lines := make([]string, 0, len(tm.tokens))
	for token, info := range tm.tokens {
		if info.Valid {
			lines = append(lines, token)
		}
	}
	content := fmt.Sprintf("# 用户 Token 文件（%s）\n", reason)
	content += fmt.Sprintf("# 更新时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	if extraComment != "" {
		content += extraComment + "\n"
	}
	content += strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return os.WriteFile(filepath.Join(tm.dataDir, "tokens.txt"), []byte(content), 0644)
}

func (tm *TokenManager) writeInvalidTokensLocked() {
	invalidFile := filepath.Join(tm.dataDir, "tokens_invalid.txt")
	var builder strings.Builder
	builder.WriteString("# 失效 Token 文件（自动更新）\n")
	builder.WriteString(fmt.Sprintf("# 更新时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	builder.WriteString("# 每条记录格式：# 失效于 <time> | 原因: <reason>\\n<token>\n\n")
	for token, info := range tm.invalidTokens {
		checkedAt := info.LastChecked
		if checkedAt.IsZero() {
			checkedAt = time.Now()
		}
		reason := strings.TrimSpace(info.InvalidReason)
		if reason == "" {
			reason = "未知"
		}
		builder.WriteString(fmt.Sprintf("# 失效于 %s | 原因: %s\n%s\n", checkedAt.Format("2006-01-02 15:04:05"), reason, token))
	}
	_ = os.WriteFile(invalidFile, []byte(builder.String()), 0644)
}

type invalidTokenFileItem struct {
	Reason string
}

func readInvalidTokenFile(path string) map[string]invalidTokenFileItem {
	out := make(map[string]invalidTokenFileItem)
	file, err := os.Open(path)
	if err != nil {
		return out
	}
	defer file.Close()

	lastReason := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if idx := strings.Index(line, "原因:"); idx >= 0 {
				lastReason = strings.TrimSpace(line[idx+len("原因:"):])
			}
			continue
		}
		token := normalizeTokenLine(line)
		if token == "" {
			continue
		}
		out[token] = invalidTokenFileItem{Reason: lastReason}
		lastReason = ""
	}
	return out
}

func truncateString(value string, maxLen int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

func previewToken(token string) string {
	if len(token) <= 18 {
		return token
	}
	return token[:10] + "..." + token[len(token)-8:]
}

// RecordCall 记录调用
func (tm *TokenManager) RecordCall(success bool, isMultimodal bool) {
	atomic.AddInt64(&tm.totalCalls, 1)
	if success {
		atomic.AddInt64(&tm.successCalls, 1)
	}
	if isMultimodal {
		atomic.AddInt64(&tm.multimodalCount, 1)
	}
}

// GetStats 获取统计数据
func (tm *TokenManager) GetStats() TokenManagerStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	total := atomic.LoadInt64(&tm.totalCalls)
	success := atomic.LoadInt64(&tm.successCalls)
	multimodal := atomic.LoadInt64(&tm.multimodalCount)

	var successRate float64
	if total > 0 {
		successRate = float64(success) / float64(total) * 100
	}

	return TokenManagerStats{
		ValidTokenCount:   len(tm.validTokens),
		TotalTokenCount:   len(tm.tokens),
		InvalidTokenCount: len(tm.invalidTokens),
		MultimodalCount:   multimodal,
		TotalCalls:        total,
		SuccessCalls:      success,
		SuccessRate:       successRate,
	}
}

// TokenManagerStats token 管理器统计数据
type TokenManagerStats struct {
	ValidTokenCount   int     `json:"valid_token_count"`
	TotalTokenCount   int     `json:"total_token_count"`
	InvalidTokenCount int     `json:"invalid_token_count"`
	MultimodalCount   int64   `json:"multimodal_count"`
	TotalCalls        int64   `json:"total_calls"`
	SuccessCalls      int64   `json:"success_calls"`
	SuccessRate       float64 `json:"success_rate"`
}

// GetClientIP 从请求中获取客户端 IP
func GetClientIP(r *http.Request) string {
	// 优先从 X-Forwarded-For 获取
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For 可能包含多个 IP，取第一个
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// 尝试 X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// 最后使用 RemoteAddr
	ip := r.RemoteAddr
	// 去除端口
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
