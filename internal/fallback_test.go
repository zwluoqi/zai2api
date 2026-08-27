package internal

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	Cfg = &Config{LogLevel: "error"}
	InitLogger()
	os.Exit(m.Run())
}

func TestBuildModelChain(t *testing.T) {
	initBuiltinMappings()
	Cfg = &Config{ModelFallbacks: []string{"GLM-5-Turbo"}}

	// 请求模型在前，备用模型在后
	chain := buildModelChain("GLM-4.6")
	if len(chain) != 2 || chain[0] != "GLM-4.6" || chain[1] != "GLM-5-Turbo" {
		t.Fatalf("chain = %v, want [GLM-4.6 GLM-5-Turbo]", chain)
	}

	// 请求模型与备用模型相同 -> 去重，只剩一个
	chain = buildModelChain("GLM-5-Turbo")
	if len(chain) != 1 || chain[0] != "GLM-5-Turbo" {
		t.Fatalf("chain = %v, want [GLM-5-Turbo]", chain)
	}

	// 无效备用模型被跳过
	Cfg = &Config{ModelFallbacks: []string{"NoSuchModel", "GLM-5-Turbo"}}
	chain = buildModelChain("GLM-4.5")
	if len(chain) != 2 || chain[1] != "GLM-5-Turbo" {
		t.Fatalf("chain = %v, want [GLM-4.5 GLM-5-Turbo]", chain)
	}
}

func TestGetActiveCaptchaProxies(t *testing.T) {
	Cfg = &Config{
		CaptchaBrowserProxy: "http://env:1,http://env:2",
		CaptchaProxyPool:    []ProxyEntry{{URL: "http://pool:9", Enabled: true}, {URL: "http://off:9", Enabled: false}},
	}
	if got := GetActiveCaptchaProxies(); len(got) != 2 || got[0] != "http://env:1" {
		t.Fatalf("pool off: %v", got)
	}
	Cfg.CaptchaProxyPoolEnabled = true
	if got := GetActiveCaptchaProxies(); len(got) != 1 || got[0] != "http://pool:9" {
		t.Fatalf("pool on: %v", got)
	}
	Cfg.CaptchaProxyPoolEnabled = false
}

func TestParseCaptchaProxyList(t *testing.T) {
	got := parseCaptchaProxyList("http://a:1, http://b:2;\nhttp://a:1")
	if len(got) != 2 || got[0] != "http://a:1" || got[1] != "http://b:2" {
		t.Fatalf("got %v", got)
	}
	if len(parseCaptchaProxyList("")) != 0 {
		t.Fatal("empty should be empty")
	}
}

func TestRedactProxyURL(t *testing.T) {
	got := redactProxyURL("http://user:secret@1.2.3.4:8080")
	if strings.Contains(got, "secret") || !strings.Contains(got, "user") || !strings.Contains(got, "1.2.3.4:8080") {
		t.Fatalf("got %q", got)
	}
}

func TestConfigureCaptchaProxy(t *testing.T) {
	// 无账号密码：只返回 server，无扩展目录
	server, extDir, err := configureCaptchaProxy("http://1.2.3.4:8080")
	if err != nil || server != "http://1.2.3.4:8080" || extDir != "" {
		t.Fatalf("got server=%q extDir=%q err=%v", server, extDir, err)
	}
	// 含账号密码：返回 server + 鉴权扩展目录
	server, extDir, err = configureCaptchaProxy("http://u:p@1.2.3.4:8080")
	if err != nil || server != "http://1.2.3.4:8080" || extDir == "" {
		t.Fatalf("got server=%q extDir=%q err=%v", server, extDir, err)
	}
	os.RemoveAll(extDir)
	// 非法地址报错
	if _, _, err := configureCaptchaProxy("not-a-url"); err == nil {
		t.Fatal("expected error for invalid proxy")
	}
}

func TestShouldFallbackModel(t *testing.T) {
	cases := []struct {
		code, msg string
		want      bool
	}{
		{"MODEL_CONCURRENCY_LIMIT", "当前模型使用人数较多", true},
		{"", "Model not available for current user level", true},
		{"INTERNAL_ERROR", "Oops, something went wrong", false},
		{"FRONTEND_CAPTCHA_REQUIRED", "Captcha verification failed", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := shouldFallbackModel(c.code, c.msg); got != c.want {
			t.Errorf("shouldFallbackModel(%q, %q) = %v, want %v", c.code, c.msg, got, c.want)
		}
	}
}
