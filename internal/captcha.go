package internal

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// captcha.go 用无头浏览器复刻 z.ai 前端的阿里云验证码 2.0「无痕验证」流程，
// 自动产出 chat completions 所需的 captcha_verify_param（base64 JSON）。
//
// z.ai 前端（prod-fe bundle）的关键配置：
//   SDK        https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js
//   region     sgp        prefix  no8xfe
//   SceneId    didk33e0（chat.z.ai）
//   mode       popup      element #chat-captcha-element  button #chat-captcha-trigger
// 无痕场景下点击触发器即静默验证，success 回调直接返回 captcha_verify_param 字符串。
//
// token 基本一次性：每次 chat 请求消费一个，因此这里维护一个预热池（带缓冲的 channel），
// 后台单 goroutine 持有一个浏览器页面串行补充。

const (
	captchaSDKURL       = "https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js"
	captchaOriginURL    = "https://chat.z.ai/"
	captchaElementID    = "chat-captcha-element"
	captchaTriggerID    = "chat-captcha-trigger"
	captchaDefaultScene = "didk33e0"
	captchaDefaultPfx   = "no8xfe"
	captchaDefaultRegn  = "sgp"
)

type captchaGenerator struct {
	tokens chan string

	mu          sync.Mutex
	browser     *rod.Browser
	proxyExtDir string // 代理鉴权扩展临时目录（浏览器销毁时清理）

	scene   string
	prefix  string
	region  string
	timeout time.Duration

	startOnce sync.Once
	started   bool
}

var captchaGen = &captchaGenerator{}

// StartCaptchaGenerator 在启用自动生成时启动预热池；未启用则空操作。
func StartCaptchaGenerator() {
	if Cfg == nil || !Cfg.CaptchaAutoGen {
		return
	}
	captchaGen.startOnce.Do(func() {
		size := Cfg.CaptchaPoolSize
		if size < 1 {
			size = 1
		}
		if size > 16 {
			size = 16
		}
		captchaGen.tokens = make(chan string, size)
		captchaGen.scene = firstNonEmpty(Cfg.CaptchaSceneID, captchaDefaultScene)
		captchaGen.prefix = firstNonEmpty(Cfg.CaptchaPrefix, captchaDefaultPfx)
		captchaGen.region = firstNonEmpty(Cfg.CaptchaRegion, captchaDefaultRegn)
		captchaGen.timeout = time.Duration(Cfg.CaptchaGenTimeoutSeconds) * time.Second
		if captchaGen.timeout < 5*time.Second {
			captchaGen.timeout = 20 * time.Second
		}
		captchaGen.started = true
		go captchaGen.loop()
		LogInfo("captcha 生成器已启动: scene=%s prefix=%s region=%s pool=%d headless=%v",
			captchaGen.scene, captchaGen.prefix, captchaGen.region, size, Cfg.CaptchaHeadless)
	})
}

// getCaptchaFromPool 供 GetCaptchaVerifyParam 调用：非阻塞地尽量取一个预热 token，
// 池暂空时短暂等待一个新鲜 token；仍取不到则返回空（调用方决定是否照发）。
func getCaptchaFromPool() string {
	if captchaGen == nil || !captchaGen.started {
		return ""
	}
	select {
	case tok := <-captchaGen.tokens:
		return tok
	case <-time.After(captchaGen.timeout + 5*time.Second):
		LogError("[captcha] 池取用超时，本次请求不携带 captcha_verify_param")
		return ""
	}
}

func (g *captchaGenerator) loop() {
	for {
		tok, err := g.generateWithRecovery()
		if err != nil {
			LogError("[captcha] 生成失败: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		if tok == "" {
			time.Sleep(1 * time.Second)
			continue
		}
		LogDebug("[captcha] token 已生成，等待入池")
		g.tokens <- tok // channel 满时阻塞，天然限流为「消费即补充」
	}
}

// generateWithRecovery 确保浏览器就绪后生成一个 token。
// 每个 token 用一个全新页面（同页面重复验证不可靠：SDK 一个实例只完成一次验证循环），
// 浏览器进程常驻复用；浏览器异常时销毁并在下一轮重建。
func (g *captchaGenerator) generateWithRecovery() (string, error) {
	browser, err := g.ensureBrowser()
	if err != nil {
		return "", fmt.Errorf("准备浏览器: %w", err)
	}
	tok, err := g.generateOnce(browser)
	if err != nil {
		g.teardown()
		return "", err
	}
	return tok, nil
}

func (g *captchaGenerator) ensureBrowser() (*rod.Browser, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browser != nil {
		return g.browser, nil
	}

	bin := Cfg.CaptchaBrowserBin
	if bin == "" {
		if p, ok := launcher.LookPath(); ok {
			bin = p
		} else {
			return nil, fmt.Errorf("未找到浏览器，请设置 CAPTCHA_BROWSER_BIN")
		}
	}
	l := launcher.New().Bin(bin).
		Set("no-sandbox", "true").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-dev-shm-usage", "true").
		Set("disable-gpu", "true").
		Set("window-size", "1920,1080").
		Set("lang", "zh-CN")
	if Cfg.CaptchaHeadless {
		// Chrome 新无头比旧 --headless 更接近真浏览器；Docker 也走这条，不依赖 xvfb。
		l = l.Headless(true).Set("headless", "new")
	} else {
		l = l.Headless(false)
	}

	// 走代理（建议住宅 IP）：token 风险分在生成时按设备指纹+IP 打分，
	// 机房 IP 会被阿里云判高风险导致 verify_failed。
	if proxy := Cfg.CaptchaBrowserProxy; proxy != "" {
		server, extDir, err := configureCaptchaProxy(proxy)
		if err != nil {
			return nil, fmt.Errorf("配置 captcha 代理: %w", err)
		}
		if server != "" {
			l = l.Set("proxy-server", server)
		}
		if extDir != "" {
			l = l.Set("disable-extensions-except", extDir).Set("load-extension", extDir)
			g.proxyExtDir = extDir
		}
		LogInfo("captcha 生成器使用代理: %s", server)
	}

	u, err := l.Launch()
	if err != nil {
		if g.proxyExtDir != "" {
			_ = os.RemoveAll(g.proxyExtDir)
			g.proxyExtDir = ""
		}
		return nil, fmt.Errorf("启动浏览器: %w", err)
	}

	var browser *rod.Browser
	if err := rod.Try(func() {
		browser = rod.New().ControlURL(u).MustConnect()
	}); err != nil {
		if browser != nil {
			_ = browser.Close()
		}
		return nil, fmt.Errorf("连接浏览器: %w", err)
	}

	g.browser = browser
	return browser, nil
}

// generateOnce 在一个全新页面上完成一次无痕验证，返回 captcha_verify_param 后关闭页面。
func (g *captchaGenerator) generateOnce(browser *rod.Browser) (string, error) {
	var page *rod.Page
	if err := rod.Try(func() {
		page = stealth.MustPage(browser)
		// 必须在 Navigate 之前注入：阿里云无痕验证在首屏就采集指纹。
		_, _ = page.EvalOnNewDocument(captchaStealthJS)
		_ = proto.EmulationSetUserAgentOverride{
			UserAgent:      strings.ReplaceAll(page.MustEval(`() => navigator.userAgent`).String(), "HeadlessChrome", "Chrome"),
			AcceptLanguage: "zh-CN,zh;q=0.9,en;q=0.8",
		}.Call(page)
		page.MustSetViewport(1920, 1080, 1, false)
		page.Timeout(30 * time.Second).MustNavigate(captchaOriginURL).MustWaitLoad()
	}); err != nil {
		if page != nil {
			_ = page.Close()
		}
		return "", fmt.Errorf("打开页面: %w", err)
	}
	defer page.Close()

	res, err := page.Timeout(g.timeout).Eval(g.setupJS())
	if err != nil {
		return "", fmt.Errorf("初始化 SDK: %w", err)
	}
	if s := res.Value.String(); s != "ready" {
		return "", fmt.Errorf("SDK 未就绪: %s", s)
	}

	res, err = page.Timeout(g.timeout).Eval(`() => window.__getCaptchaToken()`)
	if err != nil {
		return "", fmt.Errorf("取 token: %w", err)
	}
	tok := res.Value.String()
	if tok == "" {
		return "", fmt.Errorf("空 token")
	}
	return tok, nil
}

func (g *captchaGenerator) teardown() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browser != nil {
		_ = g.browser.Close()
	}
	g.browser = nil
	if g.proxyExtDir != "" {
		_ = os.RemoveAll(g.proxyExtDir)
		g.proxyExtDir = ""
	}
}

// configureCaptchaProxy 解析代理地址，返回 proxy-server 值及（含账号密码时）鉴权扩展目录。
func configureCaptchaProxy(rawProxy string) (string, string, error) {
	proxyURL, err := url.Parse(rawProxy)
	if err != nil {
		return "", "", fmt.Errorf("代理地址格式错误: %w", err)
	}
	if proxyURL.Scheme == "" || proxyURL.Host == "" {
		return "", "", fmt.Errorf("代理地址需包含协议和主机，例如 http://host:port")
	}
	server := proxyURL.Scheme + "://" + proxyURL.Host
	if proxyURL.User == nil {
		return server, "", nil
	}
	username := proxyURL.User.Username()
	password, _ := proxyURL.User.Password()
	extDir, err := os.MkdirTemp("", "zai-captcha-proxy-*")
	if err != nil {
		return "", "", err
	}
	manifest := `{
  "version": "1.0.0",
  "manifest_version": 2,
  "name": "zai-captcha-proxy-auth",
  "permissions": ["proxy", "tabs", "unlimitedStorage", "storage", "<all_urls>", "webRequest", "webRequestBlocking"],
  "background": {"scripts": ["background.js"]}
}`
	background := fmt.Sprintf(`
chrome.webRequest.onAuthRequired.addListener(
  function() { return {authCredentials: {username: %q, password: %q}}; },
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

// setupJS 一次性注入：加载 SDK、初始化无痕验证、暴露 window.__getCaptchaToken()。
// 返回 "ready" 表示 getInstance 已回调，可开始取 token。
func (g *captchaGenerator) setupJS() string {
	return fmt.Sprintf(`() => new Promise((resolveSetup) => {
  const SDK = %q, SCENE = %q, REGION = %q, PREFIX = %q;
  const ELEM = %q, TRIG = %q;
  window.__cap = { pending: null, ready: false };
  window.AliyunCaptchaConfig = { region: REGION, prefix: PREFIX };
  const ensure = (id) => {
    let el = document.getElementById(id);
    if (!el) { el = document.createElement("div"); el.id = id; el.style.display = "none"; document.body.appendChild(el); }
    return el;
  };
  ensure(ELEM); ensure(TRIG);
  const settle = (val, err) => {
    const p = window.__cap.pending; window.__cap.pending = null;
    if (!p) return;
    if (err) p.reject(new Error(err)); else p.resolve(val);
    try { window.__capInst && window.__capInst.refresh && window.__capInst.refresh(); } catch (e) {}
  };
  const loadSDK = () => new Promise((res, rej) => {
    if (window.initAliyunCaptcha) return res();
    const s = document.createElement("script");
    s.src = SDK; s.onload = () => res(); s.onerror = () => rej(new Error("sdk load failed"));
    document.head.appendChild(s);
  });
  loadSDK().then(() => {
    window.initAliyunCaptcha({
      SceneId: SCENE, mode: "popup", element: "#" + ELEM, button: "#" + TRIG,
      language: "cn", timeout: 10000, delayBeforeSuccess: false,
      success: (e) => settle(typeof e === "string" ? e : JSON.stringify(e)),
      fail: (e) => settle(null, "fail:" + (typeof e === "string" ? e : JSON.stringify(e || ""))),
      onError: (e) => settle(null, "error:" + (typeof e === "string" ? e : JSON.stringify(e || ""))),
      onClose: () => {},
      getInstance: (inst) => { window.__capInst = inst; window.__cap.ready = true; resolveSetup("ready"); },
    });
  }).catch((e) => resolveSetup("initerr:" + String(e)));
  window.__getCaptchaToken = () => new Promise((resolve, reject) => {
    if (!window.__cap || !window.__cap.ready) { reject(new Error("not ready")); return; }
    window.__cap.pending = { resolve, reject };
    const t = document.getElementById(TRIG);
    if (!t) { reject(new Error("no trigger")); return; }
    t.click();
    setTimeout(() => {
      if (window.__cap.pending) { window.__cap.pending = null; reject(new Error("token-timeout")); }
    }, 12000);
  });
  setTimeout(() => resolveSetup("setup-timeout"), 14000);
})`, captchaSDKURL, g.scene, g.region, g.prefix, captchaElementID, captchaTriggerID)
}

// captchaStealthJS 在文档创建前补一层指纹补丁（go-rod/stealth 之外）。
// 无头 Chrome 默认带 HeadlessChrome UA / webdriver，阿里云会打成 verify_failed。
const captchaStealthJS = `() => {
  try { Object.defineProperty(navigator, 'webdriver', { get: () => undefined }); } catch (e) {}
  try {
    const ua = navigator.userAgent.replace('HeadlessChrome', 'Chrome');
    Object.defineProperty(navigator, 'userAgent', { get: () => ua });
  } catch (e) {}
  try {
    if (!window.chrome) window.chrome = { runtime: {} };
  } catch (e) {}
  try {
    Object.defineProperty(navigator, 'languages', { get: () => ['zh-CN', 'zh', 'en'] });
    Object.defineProperty(navigator, 'language', { get: () => 'zh-CN' });
  } catch (e) {}
}`

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
