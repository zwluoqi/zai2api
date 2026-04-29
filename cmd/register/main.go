package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/png"
	"io"
	"math"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"zai-proxy/internal"
)

// TempMailProvider 临时邮箱服务
type TempMailProvider struct {
	Name        string
	GenerateURL string
	CheckURL    string
	Headers     map[string]string
}

var tempMailProviders = []TempMailProvider{
	{
		Name:        "chatgpt.org.uk",
		GenerateURL: "https://mail.chatgpt.org.uk/api/generate-email",
		CheckURL:    "https://mail.chatgpt.org.uk/api/emails?email=%s",
		Headers: map[string]string{
			"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
			"Referer":    "https://mail.chatgpt.org.uk",
		},
	},
}

// SliderTrack 滑块轨迹点
type SliderTrack struct {
	X    int   `json:"x"`
	Y    int   `json:"y"`
	Time int64 `json:"t"`
}

// GenerateSliderTrack 生成滑块轨迹
// 公式: y = 14.7585 * x^0.5190 - 3.9874
func GenerateSliderTrack(distance int) []SliderTrack {
	tracks := make([]SliderTrack, 0)
	startTime := time.Now().UnixMilli()

	// 初始点
	tracks = append(tracks, SliderTrack{X: 0, Y: 0, Time: 0})

	currentX := 0.0
	totalTime := int64(0)

	// 使用贝塞尔曲线模拟人手滑动
	steps := 30 + rand.Intn(20) // 30-50步

	for i := 1; i <= steps; i++ {
		progress := float64(i) / float64(steps)

		// 使用缓动函数模拟加速减速
		// easeOutQuad: 1 - (1 - t)^2
		easedProgress := 1 - math.Pow(1-progress, 2)

		targetX := float64(distance) * easedProgress

		// 计算Y偏移，使用给定公式: y = 14.7585 * x^0.5190 - 3.9874
		// 添加随机抖动
		baseY := 14.7585*math.Pow(targetX, 0.5190) - 3.9874
		yOffset := baseY*0.1 + float64(rand.Intn(5)-2)

		// 时间增量，模拟人类操作的不均匀性
		timeStep := int64(20 + rand.Intn(30)) // 20-50ms
		totalTime += timeStep

		currentX = targetX

		tracks = append(tracks, SliderTrack{
			X:    int(currentX),
			Y:    int(yOffset),
			Time: totalTime,
		})
	}

	// 确保最后一个点到达目标
	tracks = append(tracks, SliderTrack{
		X:    distance,
		Y:    rand.Intn(3) - 1,
		Time: totalTime + int64(50+rand.Intn(30)),
	})

	_ = startTime
	return tracks
}

// GenerateUsername 生成随机用户名
func GenerateUsername() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	length := 8 + rand.Intn(8) // 8-15字符
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// GeneratePassword 生成随机密码
func GeneratePassword() string {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := 12 + rand.Intn(6) // 12-17字符
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// HTTPClient 基于 internal 的 tls-client（与 Chrome 133 TLS/H2/UA 指纹一致）
type HTTPClient struct {
	client tls_client.HttpClient
}

func NewHTTPClient(proxy string) *HTTPClient {
	c, err := internal.TLSHTTPClientWithProxy(90*time.Second, proxy)
	if err != nil {
		panic("tls-client: " + err.Error())
	}
	return &HTTPClient{client: c}
}

func (c *HTTPClient) SetDefaultHeaders(req *fhttp.Request) {
	internal.ApplyBrowserFingerprintHeaders(req.Header)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("DNT", "1")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

// GetTempEmail 获取临时邮箱
func (c *HTTPClient) GetTempEmail() (string, error) {
	provider := tempMailProviders[0]

	req, err := fhttp.NewRequest("GET", provider.GenerateURL, nil)
	if err != nil {
		return "", err
	}

	for k, v := range provider.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取临时邮箱失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 尝试两种格式解析
	var result1 struct {
		Email string `json:"email"`
	}
	var result2 struct {
		Success bool `json:"success"`
		Data    struct {
			Email string `json:"email"`
		} `json:"data"`
	}

	// 先尝试 {data: {email: ...}} 格式
	if err := json.Unmarshal(body, &result2); err == nil && result2.Data.Email != "" {
		return result2.Data.Email, nil
	}

	// 再尝试 {email: ...} 格式
	if err := json.Unmarshal(body, &result1); err == nil && result1.Email != "" {
		return result1.Email, nil
	}

	return "", fmt.Errorf("获取邮箱为空, body: %s", string(body))
}

// CheckEmail 检查邮箱获取验证token
func (c *HTTPClient) CheckEmail(email string) (string, error) {
	provider := tempMailProviders[0]
	url := fmt.Sprintf(provider.CheckURL, email)

	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		req, err := fhttp.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}

		for k, v := range provider.Headers {
			req.Header.Set(k, v)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// 解析邮件列表 - 适配新API格式
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Emails []struct {
					Subject     string `json:"subject"`
					Content     string `json:"content"`
					HtmlContent string `json:"html_content"`
				} `json:"emails"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// 查找验证邮件
		for _, mail := range response.Data.Emails {
			if strings.Contains(strings.ToLower(mail.Subject), "verify") ||
				strings.Contains(mail.Subject, "验证") ||
				strings.Contains(mail.Subject, "z.ai") {
				// 从邮件内容提取token
				token := extractTokenFromEmail(mail.HtmlContent)
				if token == "" {
					token = extractTokenFromEmail(mail.Content)
				}
				if token != "" {
					return token, nil
				}
			}
		}

		fmt.Printf("  等待验证邮件... (%d/%d)\n", i+1, maxRetries)
		time.Sleep(3 * time.Second)
	}

	return "", fmt.Errorf("等待验证邮件超时")
}

// FinishSignup 通过HTTP请求完成注册
func (c *HTTPClient) FinishSignup(email, password, verifyToken string) (string, error) {
	username := strings.Split(email, "@")[0]
	if username == "" {
		username = email
	}
	// Step 1: 验证邮箱
	verifyData := fmt.Sprintf(`{"username":"%s","email":"%s","token":"%s"}`, username, email, verifyToken)
	req, _ := fhttp.NewRequest("POST", "https://chat.z.ai/api/v1/auths/verify_email", strings.NewReader(verifyData))
	req.Header.Set("Content-Type", "application/json")
	c.SetDefaultHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("验证邮箱请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 获取cookie中的token
	var tempToken string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "token" {
			tempToken = cookie.Value
			break
		}
	}

	// Step 2: 完成注册
	profileImage := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAYAAABw4pVUAAACtUlEQVR4AeyaPUplQRBGLy+bGaOZSWYXs41h3IEYiJF7UtyEP4kG7sBYTATBRAPxoYK/kaDQFvJ1V1e1HuGCdllf1TuHTi5vdrX+84knD4PZxE8qAghJpWOaEIKQZASSrcMNQUgyAsnW4YYgJBmBZOtwQ76EkGQfcqR1uCHJbCEEIckIJFuHG4KQZASSrcMNQUgyAsnW4YYgJBmBZOuMdEOSofNZByE+XOVUhMjofBoR4sNVTkWIjM6nESE+XOVUhMjofBoR4sNVTkWIjM6nESE+XOVUhMjofBoR4sNVTkWIjM6nMb2QhZWzaWH1otnzY+nIh2Sj1PRCGn3OYWIQkkzVUEKeLo+n273luudgLZmCt+uMJeRuPt2f7FQ9D6f7bwkk+2soIcnYuayDEBeseihCdHZWp1xDiIzOpxEhPlzlVITI6HwaEeLDVU4dSsjs99+qd1rfF3dlUL0ahxLSC0rkHIRE0i/MHkrI4/nhNN/4JT/XW/8KCHIdDSUkFzqfbRDiw1VONYXIqTTKBBAio/NpRIgPVzkVITI6n0aE+HCVUxEio/NpRIgPVzl1KCG1LxdfvnD37f+WDMy7cSgh3jAy5CMkg4VXOwQIeTX9A7/ON//ILxPfexF5s734gckx/5JeSAyWuKkIiWNfnIyQIpa4Q4TEsS9ORkgRS9whQuLYFycjpIgl7hAhceyLkxFSxBJ3+GmExCFsOxkhbXlWpyGkGmHbAIS05VmdhpBqhG0DENKWZ3UaQqoRtg1ASFue1WkIqUbYNgAhbXlWpyHERNi/iJD+zM2JCDHx9C8ipD9zcyJCTDz9iwjpz9yciBATT/8iQvozNycixMTTv4iQ/szNiQgx8fgUrVSEWHQCaggJgG6NRIhFJ6CGkADo1kiEWHQCaggJgG6NRIhFJ6CGkADo1shnAAAA//+Le9XMAAAABklEQVQDAJLb6FjT4DiyAAAAAElFTkSuQmCC"
	signupData := fmt.Sprintf(`{"username":"%s","email":"%s","token":"%s","password":"%s","profile_image_url":"%s","sso_redirect":null}`, username, email, verifyToken, password, profileImage)

	req2, _ := fhttp.NewRequest("POST", "https://chat.z.ai/api/v1/auths/finish_signup", strings.NewReader(signupData))
	req2.Header.Set("Content-Type", "application/json")
	if tempToken != "" {
		req2.Header.Set("Cookie", "token="+tempToken)
	}
	c.SetDefaultHeaders(req2)

	resp2, err := c.client.Do(req2)
	if err != nil {
		return "", fmt.Errorf("完成注册请求失败: %v", err)
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)

	// 解析响应获取token
	var result struct {
		Success bool `json:"success"`
		User    struct {
			Token string `json:"token"`
		} `json:"user"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v, body: %s", err, string(body))
	}

	if !result.Success || result.User.Token == "" {
		return "", fmt.Errorf("注册失败: %s", string(body))
	}

	return result.User.Token, nil
}

// extractTokenFromEmail 从邮件内容提取token
func extractTokenFromEmail(content string) string {
	// 处理HTML编码
	content = html.UnescapeString(content)
	content = strings.ReplaceAll(content, "&amp;", "&")

	// 查找 token= 参数 (UUID格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
	for _, pattern := range []string{
		`(?i)(?:[?&]token=|token=)([^"'<> \t\r\n&]+)`,
		`(?i)(?:[?&]token%3d|token%3d)([^"'<> \t\r\n&]+)`,
		`(?i)\b([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})\b`,
	} {
		matches := regexp.MustCompile(pattern).FindStringSubmatch(content)
		if len(matches) > 1 {
			value := strings.Trim(matches[1], `"'<> )]}`)
			if decoded, err := url.QueryUnescape(value); err == nil {
				return decoded
			}
			return value
		}
	}
	return ""
}

// BrowserRegister 使用rod浏览器自动化完成注册
type BrowserRegister struct {
	browser    *rod.Browser
	httpClient *HTTPClient
	mailbox    VerificationMailbox
	proxy      string
	timeout    time.Duration
	interval   time.Duration
	vision     VisionConfig
	options    *RegisterOptions
}

type VisionConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	SliderOffset float64
}

type CaptchaVisionResult struct {
	PieceLeft    float64 `json:"piece_left"`
	GapLeft      float64 `json:"gap_left"`
	DragDistance float64 `json:"drag_distance"`
}

type CaptchaMetrics struct {
	ImageCSSWidth    float64 `json:"image_css_width"`
	ImageCSSHeight   float64 `json:"image_css_height"`
	DevicePixelRatio float64 `json:"device_pixel_ratio"`
	ScreenshotWidth  int     `json:"screenshot_width"`
	ScreenshotHeight int     `json:"screenshot_height"`
	TrackCSSWidth    float64 `json:"track_css_width"`
	SliderCSSWidth   float64 `json:"slider_css_width"`
}

// Point 轨迹点
type Point struct {
	X, Y float64
}

func NewBrowserRegister(opts *RegisterOptions, mailbox VerificationMailbox, vision VisionConfig) *BrowserRegister {
	return &BrowserRegister{
		httpClient: NewHTTPClient(opts.Proxy),
		mailbox:    mailbox,
		proxy:      opts.Proxy,
		timeout:    opts.CodeTimeout,
		interval:   opts.PollInterval,
		vision:     vision,
		options:    opts,
	}
}

// 生成人类化的鼠标移动轨迹
// 公式: y = 14.7585 * x^0.5190 - 3.9874
func (br *BrowserRegister) generateHumanTrack(startX, startY, endX, endY float64) []Point {
	var movements []Point

	distance := endX - startX
	if distance <= 0 {
		return []Point{{X: endX, Y: endY}}
	}

	current := 0.0
	velocity := 0.0
	mid := distance * (0.58 + rand.Float64()*0.18)
	for current < distance {
		t := 0.014 + rand.Float64()*0.018
		acceleration := 0.0
		if current < mid {
			acceleration = 1900 + rand.Float64()*2600
		} else {
			acceleration = -(2300 + rand.Float64()*3200)
		}
		move := velocity*t + 0.5*acceleration*t*t
		velocity += acceleration * t
		if move < 0.35 {
			move = 0.35 + rand.Float64()*1.4
		}
		if current+move > distance {
			move = distance - current
		}
		current += move
		progress := current / distance
		yOffset := math.Sin(progress*math.Pi)*1.7 + float64(rand.Intn(9)-4)*0.32
		movements = append(movements, Point{
			X: startX + current,
			Y: startY + yOffset,
		})
	}

	if len(movements) == 0 || movements[len(movements)-1].X != endX {
		movements = append(movements, Point{X: endX, Y: endY})
	}
	return movements
}

// SlideSlider 使用Gemini识别缺口位置并滑动
func (br *BrowserRegister) SlideSlider(page *rod.Page) error {
	maxRetries := slideMaxRetries(10)

	for retry := 0; retry < maxRetries; retry++ {
		fmt.Printf("滑块验证尝试 %d/%d\n", retry+1, maxRetries)

		// 等待滑块加载
		slider, err := page.Timeout(5 * time.Second).Element("#aliyunCaptcha-sliding-slider")
		if err != nil || slider == nil {
			fmt.Println("未找到滑块，可能已验证成功")
			return nil
		}
		time.Sleep(500 * time.Millisecond)

		// 截取验证码图片 - 使用实际选择器
		imgEl, _ := page.Timeout(2 * time.Second).Element("div.puzzle, #aliyunCaptcha-img-box")

		var screenshot []byte
		if imgEl != nil {
			screenshot, err = imgEl.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
		}

		if screenshot == nil || err != nil {
			fmt.Println("截图失败，使用默认距离")
			// 使用默认距离直接滑动
			br.doSlideJS(page, 180+float64(rand.Intn(60)))
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		metrics := br.readCaptchaMetrics(page, screenshot)
		fmt.Printf("验证码尺寸: screenshot=%dx%d css=%.1fx%.1f dpr=%.2f\n",
			metrics.ScreenshotWidth, metrics.ScreenshotHeight,
			metrics.ImageCSSWidth, metrics.ImageCSSHeight, metrics.DevicePixelRatio)

		// 使用Gemini识别缺口位置
		distance, err := br.analyzeWithGemini(screenshot, metrics.ScreenshotWidth, metrics.ScreenshotHeight)
		if err != nil {
			fmt.Printf("Gemini识别失败: %v，使用默认距离\n", err)
			distance = 180 + float64(rand.Intn(60))
		}
		fmt.Printf("识别到拖动距离: %.0f screenshot px\n", distance)

		cssDistance := br.screenshotDistanceToCSS(distance, metrics)
		adjustedDistance := br.candidateSlideDistance(cssDistance, metrics, retry)
		fmt.Printf("换算拖动距离: %.1f CSS px，候选拖动: %.1f\n", cssDistance, adjustedDistance)
		br.saveCaptchaDebug(page, screenshot, retry+1, metrics, distance, cssDistance, adjustedDistance)
		if err := br.doSlideMouse(page, slider, adjustedDistance); err != nil {
			fmt.Printf("鼠标滑动失败: %v，改用JS滑动\n", err)
			br.doSlideJS(page, adjustedDistance)
		}

		time.Sleep(1500 * time.Millisecond)
		br.savePageDebug(page, fmt.Sprintf("after_attempt_%d", retry+1))

		// 检查是否成功
		_, err = page.Timeout(1 * time.Second).Element("#aliyunCaptcha-sliding-slider")
		if err != nil {
			fmt.Println("验证成功!")
			return nil
		}

		// 刷新重试
		refreshBtn, _ := page.Timeout(500 * time.Millisecond).Element("#aliyunCaptcha-img-refresh")
		if refreshBtn != nil {
			refreshBtn.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(1 * time.Second)
		}
	}

	// 自动失败，等待手动
	if !slideManualWaitEnabled() {
		return fmt.Errorf("自动滑块验证失败")
	}
	fmt.Println("\n=== 自动验证失败，请手动完成 ===")
	for i := 0; i < 60; i++ {
		time.Sleep(1 * time.Second)
		_, err := page.Timeout(500 * time.Millisecond).Element("#aliyunCaptcha-sliding-slider")
		if err != nil {
			fmt.Println("检测到验证成功!")
			return nil
		}
	}
	return fmt.Errorf("等待手动滑块验证超时")
}

func (br *BrowserRegister) readCaptchaMetrics(page *rod.Page, screenshot []byte) CaptchaMetrics {
	width, height := imageSize(screenshot)
	metrics := CaptchaMetrics{
		ScreenshotWidth:  width,
		ScreenshotHeight: height,
		DevicePixelRatio: 1,
	}
	result, err := page.Eval(`() => {
		const img = document.querySelector('div.puzzle, #aliyunCaptcha-img-box');
		const slider = document.querySelector('#aliyunCaptcha-sliding-slider');
		const rect = img ? img.getBoundingClientRect() : null;
		return {
			imageCSSWidth: rect ? rect.width : 0,
			imageCSSHeight: rect ? rect.height : 0,
			trackCSSWidth: slider && slider.parentElement ? slider.parentElement.getBoundingClientRect().width : 0,
			sliderCSSWidth: slider ? slider.getBoundingClientRect().width : 0,
			dpr: window.devicePixelRatio || 1
		};
	}`)
	if err == nil && !result.Value.Nil() {
		metrics.ImageCSSWidth = result.Value.Get("imageCSSWidth").Num()
		metrics.ImageCSSHeight = result.Value.Get("imageCSSHeight").Num()
		metrics.TrackCSSWidth = result.Value.Get("trackCSSWidth").Num()
		metrics.SliderCSSWidth = result.Value.Get("sliderCSSWidth").Num()
		if dpr := result.Value.Get("dpr").Num(); dpr > 0 {
			metrics.DevicePixelRatio = dpr
		}
	}
	return metrics
}

func imageSize(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func (br *BrowserRegister) screenshotDistanceToCSS(distance float64, metrics CaptchaMetrics) float64 {
	if metrics.ScreenshotWidth > 0 && metrics.ImageCSSWidth > 0 {
		scale := metrics.ImageCSSWidth / float64(metrics.ScreenshotWidth)
		return distance * scale
	}
	if metrics.DevicePixelRatio > 0 {
		return distance / metrics.DevicePixelRatio
	}
	return distance
}

func (br *BrowserRegister) candidateSlideDistance(cssDistance float64, metrics CaptchaMetrics, retry int) float64 {
	candidates := []float64{
		cssDistance + 10,
		cssDistance + 8,
		cssDistance + 12,
		cssDistance + 6,
		cssDistance + 14,
		cssDistance + 4,
		cssDistance + 16,
		cssDistance,
		cssDistance + 18,
		cssDistance + 2,
	}
	if retry >= len(candidates) {
		retry = len(candidates) - 1
	}
	return candidates[retry] + br.vision.SliderOffset
}

func (br *BrowserRegister) saveCaptchaDebug(page *rod.Page, screenshot []byte, attempt int, metrics CaptchaMetrics, modelDistance, cssDistance, adjustedDistance float64) {
	dir := filepath.Join("data", "register_debug")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	ts := time.Now().Format("20060102_150405")
	prefix := fmt.Sprintf("%s_attempt_%d", ts, attempt)
	_ = os.WriteFile(filepath.Join(dir, prefix+"_captcha.png"), screenshot, 0644)
	pageShot, err := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, prefix+"_page.png"), pageShot, 0644)
	}
	payload := map[string]any{
		"attempt":           attempt,
		"metrics":           metrics,
		"model_distance_px": modelDistance,
		"css_distance_px":   cssDistance,
		"adjusted_px":       adjustedDistance,
	}
	if data, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, prefix+"_meta.json"), data, 0644)
	}
}

func (br *BrowserRegister) savePageDebug(page *rod.Page, name string) {
	dir := filepath.Join("data", "register_debug")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	ts := time.Now().Format("20060102_150405")
	pageShot, err := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, ts+"_"+name+".png"), pageShot, 0644)
	}
}

func (br *BrowserRegister) doSlideMouse(page *rod.Page, slider *rod.Element, distance float64) error {
	result, err := page.Eval(`() => {
		const slider = document.querySelector('#aliyunCaptcha-sliding-slider');
		if (!slider) return null;
		const rect = slider.getBoundingClientRect();
		return {x: rect.left + rect.width / 2, y: rect.top + rect.height / 2};
	}`)
	if err != nil {
		return err
	}
	if result.Value.Nil() {
		return fmt.Errorf("滑块坐标为空")
	}
	startX := result.Value.Get("x").Num()
	startY := result.Value.Get("y").Num()
	fmt.Printf("鼠标滑动: %.0f 像素\n", distance)
	br.doSlide(page, startX, startY, distance)
	return nil
}
func (br *BrowserRegister) analyzeWithGemini(screenshot []byte, imageWidth, imageHeight int) (float64, error) {
	if br.vision.APIKey == "" {
		return 0, fmt.Errorf("未配置图片识别 API key，可设置 --vision-api-key 或 ZAI_VISION_API_KEY")
	}
	apiURL := normalizeChatCompletionsURL(br.vision.BaseURL)
	model := br.vision.Model

	// 转base64
	imgBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// OpenAI格式请求 - 提供完整信息让模型准确估算
	prompt := fmt.Sprintf(`这是一个滑块拼图验证码图片。
图片信息：
- 图片尺寸：%d x %d 像素
- 左侧是当前可拖动拼图块，右侧是同形状缺口
- 任务是估算滑块手柄/鼠标需要向右拖动多少像素，使当前拼图块完整嵌入右侧缺口
- 请分别估算当前拼图块主体左边缘 x 坐标、右侧缺口主体左边缘 x 坐标
- drag_distance 是最终鼠标/滑块手柄应该向右移动的像素距离，不是缺口坐标，也不是缺口中心点
- 注意左侧拼图块通常有 5-15 像素的视觉内缩，drag_distance 应让完整拼图块嵌入右侧卡槽中心
- 不要故意把拼图拖到卡槽右侧，只允许非常小的修正，避免超过卡槽
- 不要返回缺口中心点，不要返回图片宽度，不要返回已经拖动后的坐标

只返回 JSON，不要解释，不要 Markdown：
{"piece_left": 当前拼图块主体左边缘整数, "gap_left": 缺口主体左边缘整数, "drag_distance": 滑块手柄需要向右拖动的整数}`, imageWidth, imageHeight)

	requestBody, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + imgBase64}},
				},
			},
		},
		"max_tokens":  5000,
		"temperature": 0,
	})
	if err != nil {
		return 0, err
	}

	req, _ := fhttp.NewRequest("POST", apiURL, strings.NewReader(string(requestBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+br.vision.APIKey)
	internal.ApplyBrowserFingerprintHeaders(req.Header)

	resp, err := br.httpClient.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 解析OpenAI格式响应
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("解析响应失败: %v, body: %s", err, string(body))
	}

	if len(result.Choices) > 0 {
		text := strings.TrimSpace(result.Choices[0].Message.Content)
		fmt.Printf("Gemini返回: %s\n", text)
		maxDistance := float64(imageWidth) * 0.95
		if maxDistance <= 0 {
			maxDistance = 1000
		}
		distance := parseCaptchaDragDistance(text, 20, maxDistance)
		if distance > 0 {
			return distance, nil
		}
	}

	return 0, fmt.Errorf("无法解析Gemini响应: %s", string(body))
}

func parseCaptchaDragDistance(text string, min, max float64) float64 {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var vision CaptchaVisionResult
	if err := json.Unmarshal([]byte(cleaned), &vision); err == nil {
		if vision.DragDistance > min && vision.DragDistance < max {
			return vision.DragDistance
		}
		if vision.GapLeft > vision.PieceLeft {
			drag := vision.GapLeft - vision.PieceLeft
			if drag > min && drag < max {
				return drag
			}
		}
	}
	return extractCandidateDistance(text, min, max)
}

func normalizeChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/chat/completions"
	}
	return baseURL + "/v1/chat/completions"
}

func extractCandidateDistance(text string, min, max float64) float64 {
	matches := regexp.MustCompile(`\d+(?:\.\d+)?`).FindAllString(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		var value float64
		_, _ = fmt.Sscanf(matches[i], "%f", &value)
		if value > min && value < max {
			return value
		}
	}
	return 0
}

// doSlideJS 使用JS执行滑动
func (br *BrowserRegister) doSlideJS(page *rod.Page, distance float64) {
	fmt.Printf("JS滑动: %.0f 像素\n", distance)
	page.Eval(fmt.Sprintf(`() => {
		const slider = document.querySelector('#aliyunCaptcha-sliding-slider');
		if (!slider) return;
		
		const rect = slider.getBoundingClientRect();
		const startX = rect.left + rect.width / 2;
		const startY = rect.top + rect.height / 2;
		const endX = startX + %f;
		const pointerId = 1;
		const mk = (type, x, y, buttons = 1) => ({
			bubbles: true,
			cancelable: true,
			composed: true,
			view: window,
			clientX: x,
			clientY: y,
			screenX: x,
			screenY: y,
			button: 0,
			buttons,
			pointerId,
			pointerType: 'mouse',
			isPrimary: true
		});
		const fire = (target, type, x, y, buttons = 1) => {
			try {
				if (type.startsWith('pointer')) target.dispatchEvent(new PointerEvent(type, mk(type, x, y, buttons)));
				else target.dispatchEvent(new MouseEvent(type, mk(type, x, y, buttons)));
			} catch (e) {}
		};
		fire(slider, 'pointerover', startX, startY);
		fire(slider, 'mouseover', startX, startY);
		fire(slider, 'pointerenter', startX, startY);
		fire(slider, 'mouseenter', startX, startY);
		fire(slider, 'pointerdown', startX, startY);
		fire(slider, 'mousedown', startX, startY);
		try { slider.setPointerCapture(pointerId); } catch (e) {}

		let x = startX;
		let step = Math.max(3, Math.abs(endX - startX) / 35);
		const move = () => {
			x += step;
			if (x >= endX) x = endX;
			fire(document, 'pointermove', x, startY);
			fire(document, 'mousemove', x, startY);
			if (x < endX) {
				setTimeout(move, 15);
			} else {
				setTimeout(() => {
					fire(document, 'pointerup', x, startY, 0);
					fire(document, 'mouseup', x, startY, 0);
					try { slider.releasePointerCapture(pointerId); } catch (e) {}
				}, 50);
			}
		};
		setTimeout(move, 30);
	}`, distance))
}

// doSlide 执行一次滑动
func (br *BrowserRegister) doSlide(page *rod.Page, startX, startY, distance float64) {
	page.Mouse.MustMoveTo(startX-float64(18+rand.Intn(18)), startY+float64(rand.Intn(9)-4))
	time.Sleep(time.Duration(120+rand.Intn(100)) * time.Millisecond)
	page.Mouse.MustMoveTo(startX, startY)
	time.Sleep(time.Duration(120+rand.Intn(120)) * time.Millisecond)

	page.Mouse.MustDown(proto.InputMouseButtonLeft)
	time.Sleep(time.Duration(160+rand.Intn(170)) * time.Millisecond)

	// 人类化轨迹滑动
	endX := startX + distance
	overshoot := 1.2 + rand.Float64()*2.8
	if rand.Intn(3) == 0 {
		overshoot = 0
	}
	track := br.generateHumanTrack(startX, startY, endX+overshoot, startY)
	for i, point := range track {
		page.Mouse.MustMoveTo(point.X, point.Y)
		if i == len(track)/4 || i == len(track)*3/5 {
			time.Sleep(time.Duration(35+rand.Intn(45)) * time.Millisecond)
		}
		time.Sleep(time.Duration(7+rand.Intn(13)) * time.Millisecond)
	}

	if overshoot > 0 {
		time.Sleep(time.Duration(70+rand.Intn(70)) * time.Millisecond)
		corrections := 2 + rand.Intn(3)
		for i := 1; i <= corrections; i++ {
			progress := float64(i) / float64(corrections)
			x := endX + overshoot*(1-progress)
			y := startY + float64(rand.Intn(5)-2)*0.25
			page.Mouse.MustMoveTo(x, y)
			time.Sleep(time.Duration(25+rand.Intn(35)) * time.Millisecond)
		}
	}

	page.Mouse.MustMoveTo(endX, startY+float64(rand.Intn(3)-1)*0.4)
	if slideDebugEnabled() {
		br.saveBeforeReleaseDebug(page, startX, startY, endX, distance)
	}
	time.Sleep(time.Duration(240+rand.Intn(240)) * time.Millisecond)
	page.Mouse.MustUp(proto.InputMouseButtonLeft)
	if slideDebugEnabled() {
		time.Sleep(120 * time.Millisecond)
		br.saveSlideDebug(page, "after_release", startX, startY, endX, distance)
	}
}

func slideDebugEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZAI_SLIDE_DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

func slideManualWaitEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZAI_SLIDE_WAIT_MANUAL")))
	return v == "" || v == "1" || v == "true" || v == "yes"
}

func slideMaxRetries(defaultValue int) int {
	value := strings.TrimSpace(os.Getenv("ZAI_SLIDE_MAX_RETRIES"))
	if value == "" {
		return defaultValue
	}
	var out int
	_, _ = fmt.Sscanf(value, "%d", &out)
	if out <= 0 {
		return defaultValue
	}
	return out
}

func (br *BrowserRegister) saveBeforeReleaseDebug(page *rod.Page, startX, startY, endX, distance float64) {
	br.saveSlideDebug(page, "before_release", startX, startY, endX, distance)
}

func (br *BrowserRegister) saveSlideDebug(page *rod.Page, label string, startX, startY, endX, distance float64) {
	dir := filepath.Join("data", "register_debug")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	ts := time.Now().Format("20060102_150405_000")
	pageShot, err := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, ts+"_"+label+".png"), pageShot, 0644)
	}
	domInfo := map[string]any{}
	if result, err := page.Eval(`() => {
		const rectOf = (sel) => {
			const el = document.querySelector(sel);
			if (!el) return null;
			const r = el.getBoundingClientRect();
			return {left:r.left, top:r.top, width:r.width, height:r.height, right:r.right, bottom:r.bottom, transform:getComputedStyle(el).transform};
		};
		return {
			slider: rectOf('#aliyunCaptcha-sliding-slider'),
			image: rectOf('div.puzzle, #aliyunCaptcha-img-box'),
			button: rectOf('#aliyunCaptcha-sliding-slider'),
			text: (document.body && document.body.innerText || '').slice(0, 500)
		};
	}`); err == nil && !result.Value.Nil() {
		_ = json.Unmarshal([]byte(result.Value.JSON("", "")), &domInfo)
	}
	payload := map[string]any{
		"start_x":     startX,
		"start_y":     startY,
		"target_x":    endX,
		"distance_px": distance,
		"label":       label,
		"dom":         domInfo,
		"saved_at":    time.Now().Format(time.RFC3339),
	}
	if data, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, ts+"_"+label+"_meta.json"), data, 0644)
	}
}

// clickElement 安全点击元素
func (br *BrowserRegister) clickElement(page *rod.Page, selectors []string, desc string) bool {
	for _, sel := range selectors {
		el, err := page.Timeout(3 * time.Second).Element(sel)
		if err == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				fmt.Printf("  %s: 已点击 (%s)\n", desc, sel)
				return true
			}
		}
	}
	return false
}

// clickElementByText 通过文本匹配点击元素
func (br *BrowserRegister) clickElementByText(page *rod.Page, tag, text, desc string) bool {
	el, err := page.Timeout(5*time.Second).ElementR(tag, text)
	if err == nil && el != nil {
		if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
			fmt.Printf("  %s: 已点击\n", desc)
			return true
		}
	}
	fmt.Printf("  %s: 未找到\n", desc)
	return false
}

// inputText 安全输入文本
func (br *BrowserRegister) inputText(page *rod.Page, selectors []string, text, desc string) bool {
	for _, sel := range selectors {
		el, err := page.Timeout(2 * time.Second).Element(sel)
		if err == nil && el != nil {
			el.MustClick()
			time.Sleep(time.Duration(180+rand.Intn(180)) * time.Millisecond)
			el.MustSelectAllText().MustInput(text)
			fmt.Printf("  %s: 已输入\n", desc)
			return true
		}
	}
	fmt.Printf("  %s: 未找到输入框\n", desc)
	return false
}

func formStepDelay() {
	time.Sleep(time.Duration(900+rand.Intn(700)) * time.Millisecond)
}

func (br *BrowserRegister) waitCaptchaPopup(page *rod.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	selectors := []string{"#aliyunCaptcha-sliding-slider", "div.puzzle", "#aliyunCaptcha-img-box"}
	for time.Now().Before(deadline) {
		for _, sel := range selectors {
			el, err := page.Timeout(600 * time.Millisecond).Element(sel)
			if err == nil && el != nil {
				time.Sleep(time.Duration(800+rand.Intn(500)) * time.Millisecond)
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("等待滑块弹窗超时")
}

func (br *BrowserRegister) Register(email, password string) (string, error) {
	u, cleanup, err := br.launchBrowser()
	if err != nil {
		return "", fmt.Errorf("启动浏览器失败: %v", err)
	}
	defer cleanup()

	br.browser = rod.New().ControlURL(u).MustConnect()
	defer br.browser.MustClose()

	var page *rod.Page
	if err := rod.Try(func() {
		page = br.browser.MustPage("https://chat.z.ai/auth")
	}); err != nil {
		return "", fmt.Errorf("打开注册页失败: %v", err)
	}

	// 移除webdriver标记，规避自动化检测
	page.MustEval(`() => {
		Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
		Object.defineProperty(navigator, 'plugins', {get: () => [1, 2, 3, 4, 5]});
		Object.defineProperty(navigator, 'languages', {get: () => ['zh-CN', 'zh', 'en']});
		window.chrome = {runtime: {}};
	}`)

	br.clickElementByText(page, "button,span", "Email", "Continue with Email")
	formStepDelay()
	br.clickElementByText(page, "button,a", "Sign up", "Sign up")
	formStepDelay()
	displayName := strings.Split(email, "@")[0]
	if displayName == "" {
		displayName = GenerateUsername()
	}
	br.inputText(page, []string{"input[placeholder*='Name']", "input[name='name']"}, displayName, "Name")
	formStepDelay()
	br.inputText(page, []string{"input[type='email']", "input[name='email']"}, email, "Email")
	formStepDelay()
	br.inputText(page, []string{"input[type='password']", "input[name='password']"}, password, "Password")
	formStepDelay()
	// 点击验证按钮触发滑块弹窗
	if !br.clickElement(page, []string{"#aliyunCaptcha-captcha-text", "span[id*='captcha']"}, "验证按钮") {
		br.clickElementByText(page, "span,div", "verification", "验证按钮")
	}

	// 等待滑块弹窗完全加载
	fmt.Println("等待滑块弹窗加载...")
	if err := br.waitCaptchaPopup(page, 15*time.Second); err != nil {
		fmt.Printf("滑块弹窗未出现，重试点击验证按钮: %v\n", err)
		formStepDelay()
		if !br.clickElement(page, []string{"#aliyunCaptcha-captcha-text", "span[id*='captcha']"}, "验证按钮") {
			br.clickElementByText(page, "span,div", "verification", "验证按钮")
		}
		if err := br.waitCaptchaPopup(page, 15*time.Second); err != nil {
			br.savePageDebug(page, "captcha_popup_timeout")
			return "", err
		}
	}

	// 处理滑块验证
	if err := br.SlideSlider(page); err != nil {
		return "", err
	}

	// 滑块验证完成后再点击Create Account
	time.Sleep(500 * time.Millisecond)
	br.clickElementByText(page, "button", "Create", "Create Account")

	// 等待提交完成
	fmt.Println("等待提交完成...")
	time.Sleep(5 * time.Second)
	if pageErr := br.readPageError(page); pageErr != "" {
		return "", fmt.Errorf("提交注册后页面报错: %s", pageErr)
	}

	// 关闭浏览器
	br.browser.MustClose()

	// 等待验证邮件
	fmt.Println("\n等待验证邮件...")
	verifyToken, err := br.mailbox.WaitToken(br.timeout, br.interval)
	if err != nil {
		return "", fmt.Errorf("获取验证邮件失败: %v", err)
	}
	fmt.Printf("获取到验证token: %s\n", verifyToken)

	// 通过HTTP请求完成注册
	token, err := br.httpClient.FinishSignup(email, password, verifyToken)
	if err != nil {
		return "", fmt.Errorf("完成注册失败: %v", err)
	}

	fmt.Printf("注册成功! Token: %s...\n", token[:50])
	return token, nil
}

func (br *BrowserRegister) launchBrowser() (string, func(), error) {
	if br.options != nil && br.options.BrowserProvider == "adspower" {
		session, err := NewAdsPowerSession(br.options)
		if err != nil {
			return "", func() {}, err
		}
		fmt.Printf("使用 AdsPower 指纹浏览器: profile=%s port=%d\n", session.ProfileID, session.DebugPort)
		return session.ControlURL, session.Close, nil
	}

	path, found := launcher.LookPath()
	if !found {
		return "", func() {}, fmt.Errorf("未找到系统浏览器")
	}
	fmt.Printf("使用本地浏览器: %s\n", path)

	l := launcher.New().Bin(path).Headless(false).
		Set("no-sandbox", "true").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-infobars", "true").
		Set("excludeSwitches", "enable-automation").
		Set("useAutomationExtension", "false")
	var proxyExt string
	if br.proxy != "" {
		proxyServer, ext, err := configureBrowserProxy(br.proxy)
		if err != nil {
			return "", func() {}, err
		}
		proxyExt = ext
		if proxyServer != "" {
			l = l.Set("proxy-server", proxyServer)
		}
		if proxyExt != "" {
			l = l.Set("disable-extensions-except", proxyExt).Set("load-extension", proxyExt)
		}
		fmt.Printf("浏览器代理: %s\n", proxyServer)
	}
	u, err := l.Launch()
	cleanup := func() {
		if proxyExt != "" {
			_ = os.RemoveAll(proxyExt)
		}
	}
	return u, cleanup, err
}

func (br *BrowserRegister) readPageError(page *rod.Page) string {
	result, err := page.Eval(`() => {
		const text = (document.body && document.body.innerText || '').replace(/\s+/g, ' ').trim();
		const lower = text.toLowerCase();
		const needles = ['already', 'exists', 'invalid', 'failed', 'error', 'too many', '频繁', '已存在', '失败', '错误'];
		for (const n of needles) {
			const idx = lower.indexOf(n);
			if (idx >= 0) return text.slice(Math.max(0, idx - 80), idx + 160);
		}
		return '';
	}`)
	if err != nil || result.Value.Nil() {
		return ""
	}
	return strings.TrimSpace(result.Value.String())
}

// SaveToken 保存token到文件
func SaveToken(token string) error {
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	tokenFile := filepath.Join(dataDir, "tokens.txt")
	f, err := os.OpenFile(tokenFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(token + "\n")
	return err
}

type registrationResult struct {
	Email    string
	Password string
	Token    string
}

func registerOnce(opts *RegisterOptions, index, total int) (*registrationResult, error) {
	if total > 1 {
		fmt.Printf("\n=== 开始注册 %d/%d ===\n", index, total)
	}
	if opts.Proxy != "" {
		fmt.Printf("使用代理: %s\n", opts.Proxy)
	}

	mailbox, err := NewVerificationMailbox(opts)
	if err != nil {
		return nil, fmt.Errorf("准备邮箱失败: %w", err)
	}
	defer mailbox.Cleanup()

	email := mailbox.Address()
	password := GeneratePassword()
	br := NewBrowserRegister(opts, mailbox, VisionConfig{
		BaseURL:      opts.VisionBaseURL,
		APIKey:       opts.VisionAPIKey,
		Model:        opts.VisionModel,
		SliderOffset: opts.SliderOffset,
	})
	token, err := br.Register(email, password)
	if err != nil {
		return nil, err
	}
	return &registrationResult{
		Email:    email,
		Password: password,
		Token:    token,
	}, nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	opts, err := LoadRegisterOptions()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	successes := make([]*registrationResult, 0, opts.Count)
	failures := 0
	for i := 1; i <= opts.Count; i++ {
		result, err := registerOnce(opts, i, opts.Count)
		if err != nil {
			failures++
			fmt.Printf("注册失败 (%d/%d): %v\n", i, opts.Count, err)
			if opts.Count == 1 {
				os.Exit(1)
			}
			continue
		}

		// 保存token
		fmt.Println("\n保存token...")
		if err := SaveToken(result.Token); err != nil {
			fmt.Printf("保存token失败: %v\n", err)
		}

		successes = append(successes, result)
		fmt.Println("\n=== 注册成功 ===")
		fmt.Printf("邮箱: %s\n", result.Email)
		fmt.Printf("密码: %s\n", result.Password)
		fmt.Printf("Token: %s\n", result.Token)
		fmt.Println("\nToken已保存到 data/tokens.txt")
	}

	if opts.Count > 1 {
		fmt.Println("\n=== 批量注册完成 ===")
		fmt.Printf("目标: %d，成功: %d，失败: %d\n", opts.Count, len(successes), failures)
		for i, result := range successes {
			fmt.Printf("%d. %s / %s\n", i+1, result.Email, result.Password)
		}
	}
	if len(successes) == 0 {
		os.Exit(1)
	}
}
