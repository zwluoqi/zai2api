package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ModelStats 模型统计
type ModelStats struct {
	Requests  int64 `json:"requests"`
	InputTok  int64 `json:"input_tokens"`
	OutputTok int64 `json:"output_tokens"`
}

// Telemetry 遥测数据
type Telemetry struct {
	StartTime       time.Time
	TotalRequests   int64
	TotalInputTok   int64
	TotalOutputTok  int64
	minuteRequests  int64
	minuteInputTok  int64
	minuteOutputTok int64
	requestTimes    []time.Time
	modelStats      map[string]*ModelStats
	mu              sync.Mutex
}

var telemetry = &Telemetry{
	StartTime:    time.Now(),
	requestTimes: make([]time.Time, 0),
	modelStats:   make(map[string]*ModelStats),
}

type telemetryPersistedStats struct {
	TotalRequests  int64                  `json:"total_requests"`
	TotalInputTok  int64                  `json:"total_input_tokens"`
	TotalOutputTok int64                  `json:"total_output_tokens"`
	ModelStats     map[string]*ModelStats `json:"model_stats,omitempty"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

func LoadTelemetryStats() {
	path := filepath.Join("data", "telemetry_stats.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		LogWarn("读取 telemetry 统计失败: %v", err)
		return
	}
	var stats telemetryPersistedStats
	if err := json.Unmarshal(data, &stats); err != nil {
		LogWarn("解析 telemetry 统计失败: %v", err)
		return
	}
	atomic.StoreInt64(&telemetry.TotalRequests, stats.TotalRequests)
	atomic.StoreInt64(&telemetry.TotalInputTok, stats.TotalInputTok)
	atomic.StoreInt64(&telemetry.TotalOutputTok, stats.TotalOutputTok)
	telemetry.mu.Lock()
	if stats.ModelStats != nil {
		telemetry.modelStats = stats.ModelStats
	}
	telemetry.mu.Unlock()
	LogInfo("已加载 telemetry 持久化统计: requests=%d", stats.TotalRequests)
}

func RecordRequest(inputTokens, outputTokens int64, model string) {
	atomic.AddInt64(&telemetry.TotalRequests, 1)
	atomic.AddInt64(&telemetry.TotalInputTok, inputTokens)
	atomic.AddInt64(&telemetry.TotalOutputTok, outputTokens)
	atomic.AddInt64(&telemetry.minuteRequests, 1)
	atomic.AddInt64(&telemetry.minuteInputTok, inputTokens)
	atomic.AddInt64(&telemetry.minuteOutputTok, outputTokens)
	telemetry.mu.Lock()
	telemetry.requestTimes = append(telemetry.requestTimes, time.Now())
	// 模型维度统计
	if model != "" {
		if _, ok := telemetry.modelStats[model]; !ok {
			telemetry.modelStats[model] = &ModelStats{}
		}
		telemetry.modelStats[model].Requests++
		telemetry.modelStats[model].InputTok += inputTokens
		telemetry.modelStats[model].OutputTok += outputTokens
	}
	telemetry.mu.Unlock()
	saveTelemetryStats()
}

func saveTelemetryStats() {
	if err := os.MkdirAll("data", 0755); err != nil {
		LogWarn("创建 data 目录失败: %v", err)
		return
	}
	telemetry.mu.Lock()
	modelStatsCopy := make(map[string]*ModelStats, len(telemetry.modelStats))
	for model, stats := range telemetry.modelStats {
		modelStatsCopy[model] = &ModelStats{
			Requests:  stats.Requests,
			InputTok:  stats.InputTok,
			OutputTok: stats.OutputTok,
		}
	}
	telemetry.mu.Unlock()
	stats := telemetryPersistedStats{
		TotalRequests:  atomic.LoadInt64(&telemetry.TotalRequests),
		TotalInputTok:  atomic.LoadInt64(&telemetry.TotalInputTok),
		TotalOutputTok: atomic.LoadInt64(&telemetry.TotalOutputTok),
		ModelStats:     modelStatsCopy,
		UpdatedAt:      time.Now(),
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		LogWarn("序列化 telemetry 统计失败: %v", err)
		return
	}
	data = append(data, '\n')
	path := filepath.Join("data", "telemetry_stats.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		LogWarn("写入 telemetry 统计失败: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		LogWarn("保存 telemetry 统计失败: %v", err)
	}
}

func GetRPM() int {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	validTimes := make([]time.Time, 0)
	for _, t := range telemetry.requestTimes {
		if t.After(cutoff) {
			validTimes = append(validTimes, t)
		}
	}
	telemetry.requestTimes = validTimes
	return len(validTimes)
}

type TelemetryData struct {
	Uptime          string                 `json:"uptime"`
	TotalRequests   int64                  `json:"total_requests"`
	RPM             int                    `json:"rpm"`
	TotalInputTok   int64                  `json:"total_input_tokens"`
	TotalOutputTok  int64                  `json:"total_output_tokens"`
	AvgInputTok     int64                  `json:"avg_input_tokens"`
	AvgOutputTok    int64                  `json:"avg_output_tokens"`
	ValidTokens     int                    `json:"valid_tokens"`
	MultimodalCalls int64                  `json:"multimodal_calls"`
	TotalCalls      int64                  `json:"total_calls"`
	SuccessCalls    int64                  `json:"success_calls"`
	SuccessRate     float64                `json:"success_rate"`
	ModelStats      map[string]*ModelStats `json:"model_stats,omitempty"`
}

func GetTelemetryData() TelemetryData {
	totalReqs := atomic.LoadInt64(&telemetry.TotalRequests)
	totalIn := atomic.LoadInt64(&telemetry.TotalInputTok)
	totalOut := atomic.LoadInt64(&telemetry.TotalOutputTok)

	var avgIn, avgOut int64
	if totalReqs > 0 {
		avgIn = totalIn / totalReqs
		avgOut = totalOut / totalReqs
	}

	uptime := time.Since(telemetry.StartTime)
	uptimeStr := formatDuration(uptime)

	// 获取 token 管理器统计
	tmStats := GetTokenManager().GetStats()

	// 复制模型统计
	telemetry.mu.Lock()
	modelStatsCopy := make(map[string]*ModelStats)
	for k, v := range telemetry.modelStats {
		modelStatsCopy[k] = &ModelStats{
			Requests:  v.Requests,
			InputTok:  v.InputTok,
			OutputTok: v.OutputTok,
		}
	}
	telemetry.mu.Unlock()

	return TelemetryData{
		Uptime:          uptimeStr,
		TotalRequests:   totalReqs,
		RPM:             GetRPM(),
		TotalInputTok:   totalIn,
		TotalOutputTok:  totalOut,
		AvgInputTok:     avgIn,
		AvgOutputTok:    avgOut,
		ValidTokens:     tmStats.ValidTokenCount,
		MultimodalCalls: tmStats.MultimodalCount,
		TotalCalls:      tmStats.TotalCalls,
		SuccessCalls:    tmStats.SuccessCalls,
		SuccessRate:     tmStats.SuccessRate,
		ModelStats:      modelStatsCopy,
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return formatTime(days, "d") + formatTime(hours, "h") + formatTime(minutes, "m")
	}
	if hours > 0 {
		return formatTime(hours, "h") + formatTime(minutes, "m") + formatTime(seconds, "s")
	}
	if minutes > 0 {
		return formatTime(minutes, "m") + formatTime(seconds, "s")
	}
	return formatTime(seconds, "s")
}

func formatTime(value int, unit string) string {
	if value == 0 {
		return ""
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10)) + unit
}
