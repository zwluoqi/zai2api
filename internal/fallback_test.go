package internal

import (
	"os"
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
