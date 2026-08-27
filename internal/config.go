package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

const DefaultAPIEndpoint = "https://chat.z.ai/api/v2/chat/completions"

type Config struct {
	// Server
	Port       string
	ConfigPath string

	// API Configuration
	APIEndpoint  string
	APIEndpoints []string
	AuthTokens   []string // 支持多个 API Key（逗号分隔）
	BackupTokens []string // 支持多个 Backup Token（用于多模态，逗号分隔）

	// Feature Configuration
	DebugLogging bool
	ToolSupport  bool
	RetryCount   int
	// ModelFallbacks 模型降级链：当请求模型返回容量满/账号无权限时，依次降级到这些备用模型
	ModelFallbacks []string
	SkipAuthToken  bool
	ScanLimit      int
	LogLevel       string
	SpoofClientIP  bool

	// 匿名 token 池（无 TokenManager / BACKUP_TOKEN 时启用；已配置上游 token 时不使用池）
	AnonymousPoolSize               int
	AnonymousTokenTTLSeconds        int
	AnonymousRefreshIntervalSeconds int
	AnonymousFetchMaxRetries        int

	// Display
	Note []string // 多行备注，在 / 显示

	AdminToken string

	// CaptchaVerifyParam 阿里云人机验证 token (base64 JSON)，z.ai v2 chat completions 强制要求。
	// 手动兜底：非空时直接使用（一般仅用于临时抓包验证）；留空则走下面的自动生成池。
	CaptchaVerifyParam string

	// Captcha 自动生成（无头浏览器复刻阿里云无痕验证，见 captcha.go）
	CaptchaAutoGen           bool
	CaptchaHeadless          bool
	CaptchaBrowserBin        string
	CaptchaBrowserProxy      string // 环境变量兜底：代理池关闭时仅 captcha 浏览器使用
	CaptchaProxyPoolEnabled  bool   // 管理页可热更新：开启后整条链路直连 chat.z.ai 并走代理池
	CaptchaProxyPool         []ProxyEntry
	CaptchaPoolSize          int
	CaptchaGenTimeoutSeconds int
	CaptchaSceneID           string
	CaptchaPrefix            string
	CaptchaRegion            string
}

var Cfg *Config
var apiEndpointState struct {
	sync.Mutex
	next int
}

type runtimeFileConfig struct {
	API struct {
		Endpoint  string   `json:"endpoint"`
		Endpoints []string `json:"endpoints"`
	} `json:"api"`
	// 可在管理面板「系统设置」中编辑并持久化到 config.json 的运行时设置
	RetryCount              *int            `json:"retry_count"`
	ModelFallbacks          []string        `json:"model_fallbacks"`
	CaptchaProxyPoolEnabled *bool           `json:"captcha_proxy_pool_enabled"`
	CaptchaProxyPool        json.RawMessage `json:"captcha_proxy_pool"`
}

type ProxyEntry struct {
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	ExitIP    string `json:"exit_ip,omitempty"`
	Region    string `json:"region,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Success   int64  `json:"success,omitempty"`
	Fail      int64  `json:"fail,omitempty"`
	LastCheck int64  `json:"last_check,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// settingsMu 保护可在运行时（管理面板）热更新的设置字段：RetryCount、ModelFallbacks
var settingsMu sync.RWMutex

func getEnvString(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val == "true" || val == "1" || val == "yes"
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return defaultVal
}

// getEnvStringSlice 解析逗号分隔的字符串为切片
func getEnvStringSlice(key string) []string {
	val := os.Getenv(key)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parseStringList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t'
	})
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func normalizeAPIEndpoints(values ...string) []string {
	result := collectAPIEndpoints(values...)
	if len(result) == 0 {
		result = []string{DefaultAPIEndpoint}
	}
	return result
}

func collectAPIEndpoints(values ...string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		for _, endpoint := range parseStringList(value) {
			endpoint = strings.TrimRight(endpoint, "/")
			if endpoint == "" || seen[endpoint] {
				continue
			}
			seen[endpoint] = true
			result = append(result, endpoint)
		}
	}
	return result
}

func ValidateAPIEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint scheme 必须是 http 或 https")
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint 必须包含 host")
	}
	return nil
}

// parseNoteLines 解析多行备注，支持 \n 换行和 | 分隔
func parseNoteLines(note string) []string {
	if note == "" {
		return nil
	}
	// 支持 \n 和 | 作为换行符
	note = strings.ReplaceAll(note, "\\n", "\n")
	note = strings.ReplaceAll(note, "|", "\n")
	lines := strings.Split(note, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func LoadConfig() {
	godotenv.Load()
	configPath := getEnvString("CONFIG_FILE", "config.json")
	fileCfg := loadRuntimeFileConfig(configPath)
	apiEndpoints := collectAPIEndpoints(strings.Join(fileCfg.API.Endpoints, ","), fileCfg.API.Endpoint)
	if len(apiEndpoints) == 0 {
		apiEndpoints = normalizeAPIEndpoints(getEnvString("API_ENDPOINTS", ""), getEnvString("API_ENDPOINT", DefaultAPIEndpoint))
	}

	Cfg = &Config{
		// Server
		Port:       getEnvString("PORT", "8000"),
		ConfigPath: configPath,

		// API Configuration
		APIEndpoint:  apiEndpoints[0],
		APIEndpoints: apiEndpoints,
		AuthTokens:   getEnvStringSlice("AUTH_TOKEN"),
		BackupTokens: getEnvStringSlice("BACKUP_TOKEN"),

		// Feature Configuration
		DebugLogging:   getEnvBool("DEBUG_LOGGING", false),
		ToolSupport:    getEnvBool("TOOL_SUPPORT", true),
		RetryCount:     getEnvInt("RETRY_COUNT", 0),
		ModelFallbacks: parseStringList(getEnvString("MODEL_FALLBACKS", "GLM-5-Turbo")),
		SkipAuthToken:  getEnvBool("SKIP_AUTH_TOKEN", false),
		ScanLimit:      getEnvInt("SCAN_LIMIT", 200000),
		LogLevel:       getEnvString("LOG_LEVEL", "info"),
		SpoofClientIP:  getEnvBool("SPOOF_CLIENT_IP", false),

		AnonymousPoolSize:               getEnvInt("ANONYMOUS_POOL_SIZE", 4),
		AnonymousTokenTTLSeconds:        getEnvInt("ANONYMOUS_TOKEN_TTL_SECONDS", 1200),
		AnonymousRefreshIntervalSeconds: getEnvInt("ANONYMOUS_REFRESH_INTERVAL_SECONDS", 90),
		AnonymousFetchMaxRetries:        getEnvInt("ANONYMOUS_FETCH_MAX_RETRIES", 3),

		// Display
		Note: parseNoteLines(getEnvString("NOTE", "")),

		AdminToken: getEnvString("ADMIN_TOKEN", ""),

		CaptchaVerifyParam: getEnvString("CAPTCHA_VERIFY_PARAM", ""),

		CaptchaAutoGen:           getEnvBool("CAPTCHA_AUTO_GEN", false),
		CaptchaHeadless:          getEnvBool("CAPTCHA_HEADLESS", true),
		CaptchaBrowserBin:        getEnvString("CAPTCHA_BROWSER_BIN", ""),
		CaptchaBrowserProxy:      getEnvString("CAPTCHA_BROWSER_PROXY", ""),
		CaptchaProxyPoolEnabled:  false,
		CaptchaProxyPool:         nil,
		CaptchaPoolSize:          getEnvInt("CAPTCHA_POOL_SIZE", 4),
		CaptchaGenTimeoutSeconds: getEnvInt("CAPTCHA_GEN_TIMEOUT_SECONDS", 20),
		CaptchaSceneID:           getEnvString("CAPTCHA_SCENE_ID", ""),
		CaptchaPrefix:            getEnvString("CAPTCHA_PREFIX", ""),
		CaptchaRegion:            getEnvString("CAPTCHA_REGION", ""),
	}

	// config.json 中的运行时设置优先于 env 默认（由管理面板「系统设置」写入）
	if fileCfg.RetryCount != nil {
		Cfg.RetryCount = *fileCfg.RetryCount
	}
	if fileCfg.ModelFallbacks != nil {
		Cfg.ModelFallbacks = fileCfg.ModelFallbacks
	}
	if fileCfg.CaptchaProxyPoolEnabled != nil {
		Cfg.CaptchaProxyPoolEnabled = *fileCfg.CaptchaProxyPoolEnabled
	}
	if len(fileCfg.CaptchaProxyPool) > 0 {
		Cfg.CaptchaProxyPool = decodeProxyPoolJSON(fileCfg.CaptchaProxyPool)
	}
}

// GetRetryCount 返回当前 token 重试次数（线程安全，可被管理面板热更新）。
func GetRetryCount() int {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if Cfg == nil {
		return 0
	}
	if Cfg.RetryCount < 0 {
		return 0
	}
	return Cfg.RetryCount
}

// GetModelFallbacks 返回当前模型降级链的副本（线程安全，可被管理面板热更新）。
func GetModelFallbacks() []string {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if Cfg == nil {
		return nil
	}
	return append([]string(nil), Cfg.ModelFallbacks...)
}

func GetCaptchaProxyPoolEnabled() bool {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return Cfg != nil && Cfg.CaptchaProxyPoolEnabled
}

func GetCaptchaProxyEntries() []ProxyEntry {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if Cfg == nil {
		return nil
	}
	return append([]ProxyEntry(nil), Cfg.CaptchaProxyPool...)
}

func enabledProxyURLsLocked() []string {
	if Cfg == nil {
		return nil
	}
	out := make([]string, 0, len(Cfg.CaptchaProxyPool))
	for _, e := range Cfg.CaptchaProxyPool {
		if e.Enabled && e.URL != "" {
			out = append(out, e.URL)
		}
	}
	return out
}

// GetActiveCaptchaProxies 当前用于出站的代理列表。
// 代理池开启时只用已启用的条目；关闭时回退到环境变量 CAPTCHA_BROWSER_PROXY。
func GetActiveCaptchaProxies() []string {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if Cfg != nil && Cfg.CaptchaProxyPoolEnabled {
		return enabledProxyURLsLocked()
	}
	if Cfg == nil {
		return nil
	}
	return parseCaptchaProxyList(Cfg.CaptchaBrowserProxy)
}

func decodeProxyPoolJSON(raw json.RawMessage) []ProxyEntry {
	if len(raw) == 0 {
		return nil
	}
	var entries []ProxyEntry
	if err := json.Unmarshal(raw, &entries); err == nil && (len(entries) == 0 || entries[0].URL != "") {
		out := make([]ProxyEntry, 0, len(entries))
		seen := map[string]bool{}
		for _, e := range entries {
			e.URL = strings.TrimSpace(e.URL)
			if e.URL == "" || seen[e.URL] {
				continue
			}
			seen[e.URL] = true
			out = append(out, e)
		}
		return out
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		out := make([]ProxyEntry, 0, len(strs))
		for _, u := range normalizeProxyList(strs) {
			out = append(out, ProxyEntry{URL: u, Enabled: true})
		}
		return out
	}
	return nil
}

type RuntimeSettingsPatch struct {
	RetryCount              *int
	ModelFallbacks          *[]string
	CaptchaProxyPoolEnabled *bool
}

// UpdateRuntimeSettings 热更新可编辑的运行时设置并持久化到 config.json。
// 指针为 nil 表示不改动对应项。
func UpdateRuntimeSettings(p RuntimeSettingsPatch) error {
	settingsMu.Lock()
	if p.RetryCount != nil {
		rc := *p.RetryCount
		if rc < 0 {
			rc = 0
		}
		Cfg.RetryCount = rc
	}
	if p.ModelFallbacks != nil {
		Cfg.ModelFallbacks = *p.ModelFallbacks
	}
	if p.CaptchaProxyPoolEnabled != nil {
		Cfg.CaptchaProxyPoolEnabled = *p.CaptchaProxyPoolEnabled
	}
	retryCount := Cfg.RetryCount
	fallbacks := append([]string(nil), Cfg.ModelFallbacks...)
	poolOn := Cfg.CaptchaProxyPoolEnabled
	pool := append([]ProxyEntry(nil), Cfg.CaptchaProxyPool...)
	path := Cfg.ConfigPath
	settingsMu.Unlock()
	LogInfo("运行时设置已更新: retry_count=%d fallbacks=%d proxy_pool=%v proxies=%d", retryCount, len(fallbacks), poolOn, len(pool))
	return writeConfigSettings(path, retryCount, fallbacks, poolOn, pool)
}

func persistProxyPoolLocked() error {
	if Cfg == nil {
		return nil
	}
	return writeConfigSettings(Cfg.ConfigPath, Cfg.RetryCount, append([]string(nil), Cfg.ModelFallbacks...), Cfg.CaptchaProxyPoolEnabled, append([]ProxyEntry(nil), Cfg.CaptchaProxyPool...))
}

func writeConfigSettings(path string, retryCount int, fallbacks []string, poolOn bool, pool []ProxyEntry) error {
	if path == "" {
		return nil
	}
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	root["retry_count"] = retryCount
	if fallbacks == nil {
		fallbacks = []string{}
	}
	root["model_fallbacks"] = fallbacks
	root["captcha_proxy_pool_enabled"] = poolOn
	if pool == nil {
		pool = []ProxyEntry{}
	}
	root["captcha_proxy_pool"] = pool
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

type CaptchaSlot struct {
	Param string
	Proxy string
}

// GetCaptchaSlot 取一个 captcha token，以及（代理池开启时）与之绑定的出口代理。
func GetCaptchaSlot() CaptchaSlot {
	slot := CaptchaSlot{}
	if Cfg != nil && Cfg.CaptchaVerifyParam != "" {
		slot.Param = Cfg.CaptchaVerifyParam
	} else {
		slot = getCaptchaFromPool()
	}
	if GetCaptchaProxyPoolEnabled() && slot.Proxy == "" {
		if pool := GetActiveCaptchaProxies(); len(pool) > 0 {
			slot.Proxy = pool[0]
		}
	}
	if !GetCaptchaProxyPoolEnabled() {
		slot.Proxy = ""
	}
	return slot
}

func loadRuntimeFileConfig(path string) runtimeFileConfig {
	var cfg runtimeFileConfig
	if path == "" {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func GetAPIEndpoint() string {
	if Cfg == nil {
		return DefaultAPIEndpoint
	}
	apiEndpointState.Lock()
	defer apiEndpointState.Unlock()
	endpoints := Cfg.APIEndpoints
	if len(endpoints) == 0 {
		return Cfg.APIEndpoint
	}
	endpoint := endpoints[apiEndpointState.next%len(endpoints)]
	apiEndpointState.next++
	return endpoint
}

func GetAPIEndpoints() []string {
	if Cfg == nil {
		return []string{DefaultAPIEndpoint}
	}
	apiEndpointState.Lock()
	defer apiEndpointState.Unlock()
	endpoints := Cfg.APIEndpoints
	if len(endpoints) == 0 {
		endpoints = []string{Cfg.APIEndpoint}
	}
	out := make([]string, len(endpoints))
	copy(out, endpoints)
	return out
}

func SetAPIEndpoints(endpoints []string) error {
	normalized := normalizeAPIEndpoints(strings.Join(endpoints, ","))
	if len(normalized) == 0 {
		return fmt.Errorf("至少需要一个 endpoint")
	}
	for _, endpoint := range normalized {
		if err := ValidateAPIEndpoint(endpoint); err != nil {
			return fmt.Errorf("无效 endpoint %q: %w", endpoint, err)
		}
	}
	path := "config.json"
	if Cfg != nil && Cfg.ConfigPath != "" {
		path = Cfg.ConfigPath
	}
	if err := writeConfigAPIEndpoints(path, normalized); err != nil {
		return err
	}
	apiEndpointState.Lock()
	defer apiEndpointState.Unlock()
	Cfg.APIEndpoints = normalized
	Cfg.APIEndpoint = normalized[0]
	apiEndpointState.next = 0
	return nil
}

func AddAPIEndpoint(endpoint string) error {
	if Cfg == nil {
		return fmt.Errorf("配置未初始化")
	}
	endpoints := GetAPIEndpoints()
	endpoints = append(endpoints, endpoint)
	return SetAPIEndpoints(endpoints)
}

func DeleteAPIEndpoint(endpoint string) error {
	if Cfg == nil {
		return fmt.Errorf("配置未初始化")
	}
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	var endpoints []string
	current := GetAPIEndpoints()
	for _, existing := range current {
		if existing != endpoint {
			endpoints = append(endpoints, existing)
		}
	}
	if len(endpoints) == len(current) {
		return fmt.Errorf("endpoint 不存在")
	}
	if len(endpoints) == 0 {
		return fmt.Errorf("至少保留一个 endpoint")
	}
	return SetAPIEndpoints(endpoints)
}

func writeConfigAPIEndpoints(path string, endpoints []string) error {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	api, _ := root["api"].(map[string]any)
	if api == nil {
		api = map[string]any{}
	}
	api["endpoint"] = endpoints[0]
	api["endpoints"] = endpoints
	root["api"] = api

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func ValidateAuthToken(token string) bool {
	if Cfg.SkipAuthToken {
		return true
	}
	if len(Cfg.AuthTokens) == 0 {
		LogWarn("AUTH_TOKEN not configured, rejecting all requests")
		return false
	}
	for _, t := range Cfg.AuthTokens {
		if t == token {
			return true
		}
	}
	return false
}

var backupTokenIndex int

func GetBackupToken() string {
	if len(Cfg.BackupTokens) == 0 {
		return ""
	}
	token := Cfg.BackupTokens[backupTokenIndex%len(Cfg.BackupTokens)]
	backupTokenIndex++
	return token
}
