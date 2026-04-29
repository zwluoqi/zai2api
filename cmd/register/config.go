package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

type RegisterOptions struct {
	Count            int
	Provider         string
	Proxy            string
	CodeTimeout      time.Duration
	PollInterval     time.Duration
	OutlookFile      string
	OutlookUsedFile  string
	ICloudAPIBase    string
	ICloudAPIToken   string
	ICloudAutoDelete bool
	VisionBaseURL    string
	VisionAPIKey     string
	VisionModel      string
	SliderOffset     float64
	BrowserProvider  string
	AdsPowerAPI      string
	AdsPowerGroupID  string
	FingerprintLang  string
	FingerprintTZ    string
	FingerprintUA    string
}

type registerConfig struct {
	Run struct {
		Proxy proxyList `json:"proxy"`
		Count int       `json:"count"`
	} `json:"run"`
	Mail struct {
		Provider            string  `json:"provider"`
		OTPTimeoutSeconds   float64 `json:"otp_timeout_seconds"`
		PollIntervalSeconds float64 `json:"poll_interval_seconds"`
	} `json:"mail"`
	Outlook struct {
		AccountsFile string `json:"accounts_file"`
		UsedFile     string `json:"used_file"`
	} `json:"outlook"`
	ICloud struct {
		APIBase    string `json:"api_base"`
		APIToken   string `json:"api_token"`
		AutoDelete *bool  `json:"auto_delete"`
	} `json:"icloud"`
	Vision struct {
		BaseURL      string  `json:"base_url"`
		APIKey       string  `json:"api_key"`
		Model        string  `json:"model"`
		SliderOffset float64 `json:"slider_offset"`
	} `json:"vision"`
	Browser struct {
		Provider        string `json:"provider"`
		AdsPowerAPI     string `json:"adspower_api"`
		AdsPowerGroupID string `json:"adspower_group_id"`
		Language        string `json:"language"`
		Timezone        string `json:"timezone"`
		UA              string `json:"ua"`
	} `json:"browser"`
}

type proxyList []string

func (p *proxyList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*p = normalizeProxies(one)
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err == nil {
		*p = normalizeProxies(many)
		return nil
	}
	return nil
}

func LoadRegisterOptions() (*RegisterOptions, error) {
	configPath := flag.String("config", firstNonEmpty(os.Getenv("ZAI_REGISTER_CONFIG"), os.Getenv("DS_CONFIG"), "config.json"), "注册配置文件路径")
	countFlag := flag.Int("count", 0, "注册账号数量")
	providerFlag := flag.String("provider", "", "邮箱 provider: temp | outlook | icloud")
	proxyFlag := flag.String("proxy", "", "HTTP/HTTPS 代理，多个用逗号分隔时随机选一个")
	codeTimeoutFlag := flag.Float64("code-timeout", 0, "验证码等待超时秒数")
	pollIntervalFlag := flag.Float64("poll-interval", 0, "邮箱轮询间隔秒数")
	outlookFileFlag := flag.String("outlook-file", "", "Outlook 账号文件")
	outlookUsedFlag := flag.String("outlook-used", "", "Outlook 已使用账号记录文件")
	icloudAPIBaseFlag := flag.String("icloud-api-base", "", "iCloud GM Collection API 地址")
	icloudAPITokenFlag := flag.String("icloud-api-token", "", "iCloud GM Collection API token")
	visionBaseURLFlag := flag.String("vision-base-url", "", "OpenAI 兼容图片识别 API base URL")
	visionAPIKeyFlag := flag.String("vision-api-key", "", "OpenAI 兼容图片识别 API key")
	visionModelFlag := flag.String("vision-model", "", "图片识别模型")
	sliderOffsetFlag := flag.Float64("slider-offset", 0, "滑块距离补偿像素，可为负数")
	browserFlag := flag.String("browser", "", "浏览器类型: adspower | local")
	flag.Parse()

	cfg := loadRegisterConfig(*configPath)
	proxies := normalizeProxies(resolveString(*proxyFlag, firstNonEmpty(os.Getenv("ZAI_REGISTER_PROXY"), os.Getenv("DEEPSEEK_PROXY")), strings.Join(cfg.Run.Proxy, ","), ""))

	opts := &RegisterOptions{
		Count:            resolveInt(*countFlag, envInt("ZAI_REGISTER_COUNT"), cfg.Run.Count, 1),
		Provider:         strings.ToLower(resolveString(*providerFlag, firstNonEmpty(os.Getenv("ZAI_MAIL_PROVIDER"), os.Getenv("DS_MAIL_PROVIDER")), cfg.Mail.Provider, "temp")),
		CodeTimeout:      secondsDuration(resolveFloat(*codeTimeoutFlag, 0, cfg.Mail.OTPTimeoutSeconds, 180)),
		PollInterval:     secondsDuration(resolveFloat(*pollIntervalFlag, 0, cfg.Mail.PollIntervalSeconds, 3)),
		OutlookFile:      resolveString(*outlookFileFlag, firstNonEmpty(os.Getenv("ZAI_OUTLOOK_FILE"), os.Getenv("DS_OUTLOOK_FILE")), cfg.Outlook.AccountsFile, "outlook.txt"),
		OutlookUsedFile:  resolveString(*outlookUsedFlag, os.Getenv("ZAI_OUTLOOK_USED_FILE"), cfg.Outlook.UsedFile, "outlook_used.txt"),
		ICloudAPIBase:    resolveString(*icloudAPIBaseFlag, firstNonEmpty(os.Getenv("ZAI_ICLOUD_API_BASE"), os.Getenv("DS_ICLOUD_API_BASE")), cfg.ICloud.APIBase, ""),
		ICloudAPIToken:   resolveString(*icloudAPITokenFlag, firstNonEmpty(os.Getenv("ZAI_ICLOUD_API_TOKEN"), os.Getenv("DS_ICLOUD_API_TOKEN")), cfg.ICloud.APIToken, ""),
		ICloudAutoDelete: true,
		VisionBaseURL:    resolveString(*visionBaseURLFlag, os.Getenv("ZAI_VISION_BASE_URL"), cfg.Vision.BaseURL, "https://oneapi.gemiaude.com"),
		VisionAPIKey:     resolveString(*visionAPIKeyFlag, os.Getenv("ZAI_VISION_API_KEY"), cfg.Vision.APIKey, ""),
		VisionModel:      resolveString(*visionModelFlag, os.Getenv("ZAI_VISION_MODEL"), cfg.Vision.Model, "gemini-3-flash-preview"),
		SliderOffset:     resolveFloatAllowZero(*sliderOffsetFlag, envFloat("ZAI_SLIDER_OFFSET"), cfg.Vision.SliderOffset),
		BrowserProvider:  strings.ToLower(resolveString(*browserFlag, os.Getenv("ZAI_BROWSER"), cfg.Browser.Provider, "adspower")),
		AdsPowerAPI:      resolveString(os.Getenv("ADSPOWER_API_URL"), cfg.Browser.AdsPowerAPI, "http://127.0.0.1:50325"),
		AdsPowerGroupID:  resolveString(os.Getenv("ADSPOWER_GROUP_ID"), cfg.Browser.AdsPowerGroupID, "0"),
		FingerprintLang:  resolveString(os.Getenv("ZAI_FP_LANGUAGE"), cfg.Browser.Language, "ja-JP"),
		FingerprintTZ:    resolveString(os.Getenv("ZAI_FP_TIMEZONE"), cfg.Browser.Timezone, "Asia/Tokyo"),
		FingerprintUA:    resolveString(os.Getenv("ZAI_FP_UA"), cfg.Browser.UA, defaultFingerprintUA),
	}
	if cfg.ICloud.AutoDelete != nil {
		opts.ICloudAutoDelete = *cfg.ICloud.AutoDelete
	}
	if len(proxies) > 0 {
		opts.Proxy = proxies[rand.Intn(len(proxies))]
	}
	if opts.Count < 1 {
		return nil, fmt.Errorf("注册数量必须 >= 1")
	}
	switch opts.Provider {
	case "temp", "tempmail", "temporary":
		opts.Provider = "temp"
	case "outlook", "icloud":
	default:
		return nil, fmt.Errorf("不支持的邮箱 provider: %s", opts.Provider)
	}
	if opts.BrowserProvider != "adspower" && opts.BrowserProvider != "local" {
		return nil, fmt.Errorf("不支持的浏览器 provider: %s", opts.BrowserProvider)
	}
	return opts, nil
}

const defaultFingerprintUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"

func loadRegisterConfig(path string) registerConfig {
	var cfg registerConfig
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

func normalizeProxies(value any) []string {
	var raw []string
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		raw = strings.Split(v, ",")
	case []string:
		raw = v
	default:
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool)
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func resolveString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	return resolveString(values...)
}

func resolveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func resolveFloatAllowZero(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func resolveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func envInt(key string) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	var out int
	_, _ = fmt.Sscanf(value, "%d", &out)
	return out
}

func envFloat(key string) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	var out float64
	_, _ = fmt.Sscanf(value, "%f", &out)
	return out
}

func secondsDuration(seconds float64) time.Duration {
	if seconds <= 0 {
		seconds = 1
	}
	return time.Duration(seconds * float64(time.Second))
}
