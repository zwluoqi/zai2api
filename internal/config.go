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
	DebugLogging  bool
	ToolSupport   bool
	RetryCount    int
	SkipAuthToken bool
	ScanLimit     int
	LogLevel      string
	SpoofClientIP bool

	// 匿名 token 池（无 TokenManager / BACKUP_TOKEN 时启用；已配置上游 token 时不使用池）
	AnonymousPoolSize               int
	AnonymousTokenTTLSeconds        int
	AnonymousRefreshIntervalSeconds int
	AnonymousFetchMaxRetries        int

	// Display
	Note []string // 多行备注，在 / 显示

	AdminToken string
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
}

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
		DebugLogging:  getEnvBool("DEBUG_LOGGING", false),
		ToolSupport:   getEnvBool("TOOL_SUPPORT", true),
		RetryCount:    getEnvInt("RETRY_COUNT", 5),
		SkipAuthToken: getEnvBool("SKIP_AUTH_TOKEN", false),
		ScanLimit:     getEnvInt("SCAN_LIMIT", 200000),
		LogLevel:      getEnvString("LOG_LEVEL", "info"),
		SpoofClientIP: getEnvBool("SPOOF_CLIENT_IP", false),

		AnonymousPoolSize:               getEnvInt("ANONYMOUS_POOL_SIZE", 4),
		AnonymousTokenTTLSeconds:        getEnvInt("ANONYMOUS_TOKEN_TTL_SECONDS", 1200),
		AnonymousRefreshIntervalSeconds: getEnvInt("ANONYMOUS_REFRESH_INTERVAL_SECONDS", 90),
		AnonymousFetchMaxRetries:        getEnvInt("ANONYMOUS_FETCH_MAX_RETRIES", 3),

		// Display
		Note: parseNoteLines(getEnvString("NOTE", "")),

		AdminToken: getEnvString("ADMIN_TOKEN", ""),
	}
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
