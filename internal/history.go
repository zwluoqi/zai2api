package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	historyDir           = "data/history"
	HistoryStatusRunning = "running"
	HistoryStatusSuccess = "success"
	HistoryStatusFailed  = "failed"
)

var historyMu sync.Mutex

type HistoryMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type ChatHistoryRecord struct {
	ID            string           `json:"id"`
	Model         string           `json:"model"`
	UpstreamModel string           `json:"upstream_model,omitempty"`
	Stream        bool             `json:"stream"`
	Status        string           `json:"status"`
	RequestTime   time.Time        `json:"request_time"`
	UpdatedAt     time.Time        `json:"updated_at"`
	DurationMs    int64            `json:"duration_ms"`
	InputTokens   int64            `json:"input_tokens"`
	OutputTokens  int64            `json:"output_tokens"`
	TotalTokens   int64            `json:"total_tokens"`
	Error         string           `json:"error,omitempty"`
	Messages      []HistoryMessage `json:"messages"`
}

type ChatHistorySummary struct {
	ID            string    `json:"id"`
	Model         string    `json:"model"`
	UpstreamModel string    `json:"upstream_model,omitempty"`
	Stream        bool      `json:"stream"`
	Status        string    `json:"status"`
	RequestTime   time.Time `json:"request_time"`
	UpdatedAt     time.Time `json:"updated_at"`
	DurationMs    int64     `json:"duration_ms"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	TotalTokens   int64     `json:"total_tokens"`
	Error         string    `json:"error,omitempty"`
}

func StartChatHistory(model string, stream bool, messages []Message, inputTokens int64) *ChatHistoryRecord {
	now := time.Now()
	record := &ChatHistoryRecord{
		ID:          now.Format("20060102150405.000000000") + "-" + uuid.New().String(),
		Model:       model,
		Stream:      stream,
		Status:      HistoryStatusRunning,
		RequestTime: now,
		UpdatedAt:   now,
		InputTokens: inputTokens,
		TotalTokens: inputTokens,
		Messages:    historyMessagesFromRequest(messages),
	}
	if err := record.Save(); err != nil {
		LogWarn("Failed to save chat history start: %v", err)
	}
	return record
}

func (h *ChatHistoryRecord) FinishSuccess(upstreamModel string, outputTokens int64, assistant HistoryMessage) {
	if h == nil {
		return
	}
	h.finish(HistoryStatusSuccess, upstreamModel, outputTokens, "", assistant)
}

func (h *ChatHistoryRecord) FinishFailed(upstreamModel string, outputTokens int64, errMsg string, assistant *HistoryMessage) {
	if h == nil {
		return
	}
	msg := HistoryMessage{}
	if assistant != nil {
		msg = *assistant
	}
	h.finish(HistoryStatusFailed, upstreamModel, outputTokens, errMsg, msg)
}

func (h *ChatHistoryRecord) finish(status, upstreamModel string, outputTokens int64, errMsg string, assistant HistoryMessage) {
	now := time.Now()
	h.Status = status
	h.UpstreamModel = upstreamModel
	h.UpdatedAt = now
	h.DurationMs = now.Sub(h.RequestTime).Milliseconds()
	h.OutputTokens = outputTokens
	h.TotalTokens = h.InputTokens + h.OutputTokens
	h.Error = errMsg
	if assistant.Role != "" || assistant.Content != "" || assistant.ReasoningContent != "" || len(assistant.ToolCalls) > 0 {
		if assistant.Role == "" {
			assistant.Role = "assistant"
		}
		h.Messages = append(h.Messages, assistant)
	}
	if err := h.Save(); err != nil {
		LogWarn("Failed to save chat history finish: %v", err)
	}
}

func (h *ChatHistoryRecord) Save() error {
	if h == nil || h.ID == "" {
		return nil
	}
	historyMu.Lock()
	defer historyMu.Unlock()
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(historyDir, h.ID+".json"), data, 0644)
}

func ListChatHistoryPage(page, pageSize int) ([]ChatHistorySummary, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	entries, err := os.ReadDir(historyDir)
	if os.IsNotExist(err) {
		return []ChatHistorySummary{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	type historyFile struct {
		name    string
		modTime time.Time
	}
	files := make([]historyFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			LogWarn("Failed to stat chat history %s: %v", entry.Name(), err)
			continue
		}
		files = append(files, historyFile{name: entry.Name(), modTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		leftHasTime := len(files[i].name) > len("20060102150405.000000000-") && files[i].name[14] == '.'
		rightHasTime := len(files[j].name) > len("20060102150405.000000000-") && files[j].name[14] == '.'
		if leftHasTime && rightHasTime {
			return files[i].name > files[j].name
		}
		return files[i].modTime.After(files[j].modTime)
	})

	total := len(files)
	start := (page - 1) * pageSize
	if start >= total {
		return []ChatHistorySummary{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	summaries := make([]ChatHistorySummary, 0, end-start)
	for _, file := range files[start:end] {
		record, err := readChatHistoryFile(filepath.Join(historyDir, file.name))
		if err != nil {
			LogWarn("Failed to read chat history %s: %v", file.name, err)
			continue
		}
		summaries = append(summaries, ChatHistorySummary{
			ID:            record.ID,
			Model:         record.Model,
			UpstreamModel: record.UpstreamModel,
			Stream:        record.Stream,
			Status:        record.Status,
			RequestTime:   record.RequestTime,
			UpdatedAt:     record.UpdatedAt,
			DurationMs:    record.DurationMs,
			InputTokens:   record.InputTokens,
			OutputTokens:  record.OutputTokens,
			TotalTokens:   record.TotalTokens,
			Error:         record.Error,
		})
	}
	return summaries, total, nil
}

func GetChatHistory(id string) (*ChatHistoryRecord, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return nil, fmt.Errorf("无效历史记录 ID")
	}
	return readChatHistoryFile(filepath.Join(historyDir, id+".json"))
}

func readChatHistoryFile(path string) (*ChatHistoryRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record ChatHistoryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func historyMessagesFromRequest(messages []Message) []HistoryMessage {
	out := make([]HistoryMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, HistoryMessage{
			Role:       msg.Role,
			Content:    historyContentToString(msg.Content),
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}
	return out
}

func historyContentToString(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func historyAssistantFromResult(result UpstreamResult) *HistoryMessage {
	if result.Content == "" && result.ReasoningContent == "" && len(result.ToolCalls) == 0 {
		return nil
	}
	return &HistoryMessage{
		Role:             "assistant",
		Content:          result.Content,
		ReasoningContent: result.ReasoningContent,
		ToolCalls:        result.ToolCalls,
	}
}
