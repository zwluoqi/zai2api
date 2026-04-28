package internal

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function,omitempty"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}
type ToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

var (
	toolCallFencePattern     = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\}|\\[.*?\\])\\s*```")
	functionCallPattern      = regexp.MustCompile(`(?s)调用函数\s*[：:]\s*([\w\-\.]+)\s*(?:参数|arguments)[：:]\s*(\{.*?\})`)
	functionInvokePattern    = regexp.MustCompile(`(?s)\b([\w\-\.]+)\s*\(\s*(\{.*?\})\s*\)`)
	toolTaggedPayloadPattern = regexp.MustCompile(`(?is)<(?:tool_call|function_call)\)?\s*>(.*?)</(?:tool_call|function_call)\)?\s*>`)
	callIDCounter            int64
)

func GenerateToolPrompt(tools []Tool, toolChoice interface{}) string {
	if len(tools) == 0 {
		return ""
	}
	var toolDefs []string
	var toolNames []string
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}

		fn := tool.Function
		toolNames = append(toolNames, fn.Name)
		toolInfo := fmt.Sprintf("### %s\n%s", fn.Name, fn.Description)
		if len(fn.Parameters) > 0 {
			var params struct {
				Type       string                 `json:"type"`
				Properties map[string]interface{} `json:"properties"`
				Required   []string               `json:"required"`
			}
			if err := json.Unmarshal(fn.Parameters, &params); err == nil && len(params.Properties) > 0 {
				requiredSet := make(map[string]bool)
				for _, r := range params.Required {
					requiredSet[r] = true
				}

				toolInfo += "\n**参数:**"
				for name, details := range params.Properties {
					detailMap, _ := details.(map[string]interface{})
					paramType, _ := detailMap["type"].(string)
					paramDesc, _ := detailMap["description"].(string)
					enumVals, hasEnum := detailMap["enum"].([]interface{})

					reqMark := ""
					if requiredSet[name] {
						reqMark = " (必填)"
					}

					paramLine := fmt.Sprintf("\n- **%s** (%s%s): %s", name, paramType, reqMark, paramDesc)
					if hasEnum && len(enumVals) > 0 {
						var enumStrs []string
						for _, e := range enumVals {
							enumStrs = append(enumStrs, fmt.Sprintf("`%v`", e))
						}
						paramLine += fmt.Sprintf(" [可选值: %s]", strings.Join(enumStrs, ", "))
					}
					toolInfo += paramLine
				}
			}
		}
		toolDefs = append(toolDefs, toolInfo)
	}

	if len(toolDefs) == 0 {
		return ""
	}

	instructions := getToolChoiceInstructions(toolChoice, toolNames)
	return "\n\n# 可用工具\n" + strings.Join(toolDefs, "\n\n") + "\n\n" + instructions
}

func getToolChoiceInstructions(toolChoice interface{}, toolNames []string) string {
	allowedTools := strings.Join(toolNames, ", ")
	baseInstructions := fmt.Sprintf(`# 工具调用格式
你当前请求中唯一允许使用的函数工具是：%s
这些工具由调用方显式提供，必须视为真实可用。
不要声称“没有这个工具”，不要改用你自己的内置工具，也不要改写函数名。

当需要调用工具时，优先输出以下 OpenAI 兼容 JSON：
`+"```json"+`
{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"函数名","arguments":"{\"参数名\":\"参数值\"}"}}]}
`+"```"+`

也兼容以下次选格式，但只有在你无法稳定输出首选格式时才使用：
`+"```json"+`
{"name":"函数名","arguments":{"参数名":"参数值"}}
`+"```"+`
或：
`+"```xml"+`
<tool_call>{"name":"函数名","arguments":{"参数名":"参数值"}}</tool_call>
`+"```"+`

**重要规则：**
1. 首选输出必须是纯 JSON，不要添加解释文字
2. tool_calls[].function.arguments 必须是 JSON 字符串，不是对象
3. 只能调用上面列出的函数名，不能改名，不能替换成别的工具
4. 如果用户要求使用工具或 tool_choice 有要求，你必须先调用工具，不能先解释为什么不能调用
5. 即使信息不完整，也要先依据已有上下文构造最合理的参数发起调用
6. 可以同时调用多个工具，但只有在任务确实需要时才这么做
7. 如果已经收到工具结果，必须直接根据结果回答，不能重复调用工具

# 工具结果处理
当你看到以 "[已执行工具调用]" 开头的助手消息和以 "[工具返回结果]" 开头的用户消息时，说明工具已经被调用并返回了结果。
**此时你必须直接使用工具返回的数据来回答用户，绝对不要再次调用工具。**`, allowedTools)

	switch tc := toolChoice.(type) {
	case string:
		if tc == "auto" {
			return baseInstructions + "\n8. 根据用户需求自行判断是否需要调用工具。"
		} else if tc == "required" {
			return baseInstructions + "\n8. **必须**调用至少一个工具来响应用户请求。"
		}
	case map[string]interface{}:
		if tc["type"] == "function" {
			if fn, ok := tc["function"].(map[string]interface{}); ok {
				if name, ok := fn["name"].(string); ok {
					return baseInstructions + fmt.Sprintf("\n8. **必须**调用 `%s` 工具来响应用户请求。", name)
				}
			}
		}
	}

	return baseInstructions + "\n8. 根据用户需求自行判断是否需要调用工具。"
}

func ProcessMessagesWithTools(messages []Message, tools []Tool, toolChoice interface{}) []Message {
	if !Cfg.ToolSupport || len(tools) == 0 {
		LogDebug("[Tools] Tool support disabled or no tools provided")
		return messages
	}
	if tc, ok := toolChoice.(string); ok && tc == "none" {
		LogDebug("[Tools] Tool choice is 'none', skipping tool processing")
		return messages
	}

	toolPrompt := GenerateToolPrompt(tools, toolChoice)
	if toolPrompt == "" {
		LogDebug("[Tools] Generated empty tool prompt")
		return messages
	}
	LogDebug("[Tools] Injecting tool prompt for %d tools", len(tools))

	processed := make([]Message, len(messages))
	copy(processed, messages)
	for i, msg := range processed {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			processed[i] = convertAssistantToolCallMessage(msg)
		} else if msg.Role == "tool" {
			processed[i] = convertToolMessage(msg)
		}
	}

	hasSystem := false
	for i, msg := range processed {
		if msg.Role == "system" {
			hasSystem = true
			processed[i].Content = appendTextToContent(msg.Content, toolPrompt)
			break
		}
	}
	if !hasSystem {
		systemMsg := Message{
			Role:    "system",
			Content: "你是一个智能助手，能够帮助用户完成各种任务。" + toolPrompt,
		}
		processed = append([]Message{systemMsg}, processed...)
	}

	return processed
}
func convertAssistantToolCallMessage(msg Message) Message {
	content, _ := msg.ParseContent()
	var sb strings.Builder
	if content != "" {
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}
	sb.WriteString("[已执行工具调用]\n")
	for _, tc := range msg.ToolCalls {
		sb.WriteString(fmt.Sprintf("- 调用了 %s，参数: %s (call_id: %s)\n", tc.Function.Name, tc.Function.Arguments, tc.ID))
	}
	return Message{
		Role:    "assistant",
		Content: sb.String(),
	}
}

func convertToolMessage(msg Message) Message {
	content, _ := msg.ParseContent()
	var resultText string
	if msg.ToolCallID != "" {
		resultText = fmt.Sprintf("[工具返回结果] (call_id: %s)\n以下是工具返回的数据，请直接使用这些数据回答用户：\n%s", msg.ToolCallID, content)
	} else {
		resultText = fmt.Sprintf("[工具返回结果]\n以下是工具返回的数据，请直接使用这些数据回答用户：\n%s", content)
	}
	return Message{
		Role:    "user",
		Content: resultText,
	}
}

func appendTextToContent(content interface{}, suffix string) interface{} {
	switch c := content.(type) {
	case string:
		return c + suffix
	case []interface{}:
		result := make([]interface{}, len(c))
		copy(result, c)
		lastTextIdx := -1
		for i, item := range result {
			if part, ok := item.(map[string]interface{}); ok {
				if partType, _ := part["type"].(string); partType == "text" {
					lastTextIdx = i
				}
			}
		}

		if lastTextIdx >= 0 {
			if part, ok := result[lastTextIdx].(map[string]interface{}); ok {
				newPart := make(map[string]interface{})
				for k, v := range part {
					newPart[k] = v
				}
				if text, ok := newPart["text"].(string); ok {
					newPart["text"] = text + suffix
				}
				result[lastTextIdx] = newPart
			}
		} else {
			result = append(result, map[string]interface{}{
				"type": "text",
				"text": suffix,
			})
		}
		return result
	default:
		return content
	}
}
func findMatchingBrace(text string, start int) int {
	if start >= len(text) || text[start] != '{' {
		return -1
	}
	braceCount := 1
	inString := false
	escapeNext := false
	j := start + 1
	for j < len(text) && braceCount > 0 {
		ch := text[j]
		if escapeNext {
			escapeNext = false
			j++
			continue
		}
		switch ch {
		case '\\':
			if inString {
				escapeNext = true
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				braceCount++
			}
		case '}':
			if !inString {
				braceCount--
			}
		}
		j++
	}
	if braceCount != 0 {
		return -1
	}
	return j
}

func normalizeArguments(args interface{}) string {
	switch v := args.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return "{}"
		}
		var check json.RawMessage
		if json.Unmarshal([]byte(v), &check) == nil {
			return v
		}
		fixed := strings.ReplaceAll(v, "'", "\"")
		if json.Unmarshal([]byte(fixed), &check) == nil {
			return fixed
		}
		return v
	case map[string]interface{}:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	case []interface{}:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	case nil:
		return "{}"
	}
	return "{}"
}

func validateAndNormalizeCalls(calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	var valid []ToolCall
	for _, call := range calls {
		if call.Function.Name == "" {
			LogDebug("[Tools] Skipping tool call with empty function name")
			continue
		}
		if call.ID == "" {
			call.ID = generateCallID()
		}
		if call.Type == "" {
			call.Type = "function"
		}
		call.Function.Arguments = normalizeArguments(call.Function.Arguments)
		valid = append(valid, call)
	}
	if len(valid) == 0 {
		return nil
	}
	return valid
}

func parseNamedFunctionObject(jsonStr string) []ToolCall {
	var raw struct {
		ID        string      `json:"id"`
		Type      string      `json:"type"`
		Name      string      `json:"name"`
		Arguments interface{} `json:"arguments"`
		Tool      string      `json:"tool"`
		Args      interface{} `json:"args"`
		Input     interface{} `json:"input"`
		Function  *struct {
			Name      string      `json:"name"`
			Arguments interface{} `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil
	}

	name := raw.Name
	args := raw.Arguments
	if raw.Function != nil {
		if name == "" {
			name = raw.Function.Name
		}
		if args == nil {
			args = raw.Function.Arguments
		}
	}
	if name == "" && raw.Tool != "" {
		name = raw.Tool
	}
	if args == nil && raw.Args != nil {
		args = raw.Args
	}
	if args == nil && raw.Input != nil {
		args = raw.Input
	}
	if name == "" {
		return nil
	}
	return []ToolCall{{
		ID:   raw.ID,
		Type: raw.Type,
		Function: ToolCallFunction{
			Name:      name,
			Arguments: normalizeArguments(args),
		},
	}}
}

func parseTaggedToolPayload(text string) []ToolCall {
	matches := toolTaggedPayloadPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		payload := strings.TrimSpace(match[1])
		if calls := parseToolCallsJSON(payload); calls != nil {
			return calls
		}
		if calls := parseNamedFunctionObject(payload); calls != nil {
			return calls
		}
	}
	return nil
}

func parseFunctionInvocation(text string) []ToolCall {
	matches := functionInvokePattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		name := strings.TrimSpace(match[1])
		args := strings.TrimSpace(match[2])
		var check json.RawMessage
		if name != "" && json.Unmarshal([]byte(args), &check) == nil {
			return []ToolCall{{
				Type: "function",
				Function: ToolCallFunction{
					Name:      name,
					Arguments: args,
				},
			}}
		}
	}
	return nil
}

// ExtractToolInvocations 从响应文本中提取工具调用
func ExtractToolInvocations(text string) []ToolCall {
	if text == "" {
		return nil
	}

	scanText := text
	if len(scanText) > Cfg.ScanLimit {
		scanText = scanText[:Cfg.ScanLimit]
	}

	matches := toolCallFencePattern.FindAllStringSubmatch(scanText, -1)
	for _, match := range matches {
		if len(match) > 1 {
			if calls := parseToolCallsJSON(match[1]); calls != nil {
				LogDebug("[ExtractToolInvocations] Found %d tool calls in JSON fence", len(calls))
				return validateAndNormalizeCalls(calls)
			}
			if calls := parseNamedFunctionObject(match[1]); calls != nil {
				LogDebug("[ExtractToolInvocations] Found named function object in JSON fence")
				return validateAndNormalizeCalls(calls)
			}
		}
	}

	if calls := parseTaggedToolPayload(scanText); calls != nil {
		LogDebug("[ExtractToolInvocations] Found tagged tool payload")
		return validateAndNormalizeCalls(calls)
	}

	if calls := extractInlineToolCalls(scanText); calls != nil {
		LogDebug("[ExtractToolInvocations] Found %d tool calls inline", len(calls))
		return validateAndNormalizeCalls(calls)
	}

	if calls := extractSingleFunctionCall(scanText); calls != nil {
		LogDebug("[ExtractToolInvocations] Found single function call")
		return validateAndNormalizeCalls(calls)
	}

	if calls := parseFunctionInvocation(scanText); calls != nil {
		LogDebug("[ExtractToolInvocations] Found function invocation pattern")
		return validateAndNormalizeCalls(calls)
	}

	if match := functionCallPattern.FindStringSubmatch(scanText); len(match) > 2 {
		funcName := strings.TrimSpace(match[1])
		argsStr := strings.TrimSpace(match[2])
		var check json.RawMessage
		if json.Unmarshal([]byte(argsStr), &check) == nil {
			LogDebug("[ExtractToolInvocations] Found natural language function call: %s", funcName)
			return validateAndNormalizeCalls([]ToolCall{{
				Type: "function",
				Function: ToolCallFunction{
					Name:      funcName,
					Arguments: argsStr,
				},
			}})
		}
	}

	return nil
}

func extractSingleFunctionCall(text string) []ToolCall {
	searchStart := 0
	for {
		idx := strings.Index(text[searchStart:], `"name"`)
		if idx == -1 {
			break
		}
		idx += searchStart

		braceStart := -1
		for k := idx - 1; k >= 0; k-- {
			ch := text[k]
			if ch == '{' {
				braceStart = k
				break
			}
			if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
				break
			}
		}
		if braceStart == -1 {
			searchStart = idx + 1
			continue
		}

		end := findMatchingBrace(text, braceStart)
		if end == -1 {
			searchStart = idx + 1
			continue
		}

		jsonStr := text[braceStart:end]
		if calls := parseNamedFunctionObject(jsonStr); calls != nil {
			return calls
		}
		searchStart = idx + 1
	}
	return nil
}
func parseToolCallsJSON(jsonStr string) []ToolCall {
	var data struct {
		ToolCalls []struct {
			ID        string      `json:"id"`
			Type      string      `json:"type"`
			Name      string      `json:"name"`
			Arguments interface{} `json:"arguments"`
			Function  interface{} `json:"function"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil
	}
	if len(data.ToolCalls) == 0 {
		return nil
	}
	var calls []ToolCall
	for _, tc := range data.ToolCalls {
		call := ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
		}
		if fn, ok := tc.Function.(map[string]interface{}); ok {
			call.Function.Name, _ = fn["name"].(string)
			if args, ok := fn["arguments"]; ok {
				call.Function.Arguments = normalizeArguments(args)
			}
		}
		if call.Function.Name == "" {
			call.Function.Name = tc.Name
		}
		if call.Function.Arguments == "" {
			if tc.Arguments != nil {
				call.Function.Arguments = normalizeArguments(tc.Arguments)
			} else {
				call.Function.Arguments = "{}"
			}
		}
		calls = append(calls, call)
	}
	return calls
}

func extractInlineToolCalls(text string) []ToolCall {
	if !strings.Contains(text, `"tool_calls"`) {
		return nil
	}
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		end := findMatchingBrace(text, i)
		if end == -1 {
			continue
		}
		jsonStr := text[i:end]
		if strings.Contains(jsonStr, `"tool_calls"`) {
			if calls := parseToolCallsJSON(jsonStr); calls != nil {
				return calls
			}
		}
		i = end - 1
	}
	return nil
}

func isToolPayload(jsonStr string) bool {
	return parseToolCallsJSON(jsonStr) != nil || parseNamedFunctionObject(jsonStr) != nil
}

func RemoveToolJSONContent(text string) string {
	result := toolCallFencePattern.ReplaceAllStringFunc(text, func(match string) string {
		submatch := toolCallFencePattern.FindStringSubmatch(match)
		if len(submatch) > 1 && isToolPayload(strings.TrimSpace(submatch[1])) {
			return ""
		}
		return match
	})
	result = toolTaggedPayloadPattern.ReplaceAllString(result, "")
	result = removeInlineToolCallJSON(result)
	result = removeInlineSingleFunctionCallJSON(result)
	return strings.TrimSpace(result)
}

func removeInlineSingleFunctionCallJSON(text string) string {
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		end := findMatchingBrace(text, i)
		if end == -1 {
			continue
		}
		jsonStr := text[i:end]
		if parseNamedFunctionObject(jsonStr) != nil {
			return strings.TrimSpace(text[:i] + text[end:])
		}
		i = end - 1
	}
	return text
}
func removeInlineToolCallJSON(text string) string {
	if !strings.Contains(text, `"tool_calls"`) {
		return text
	}
	var result strings.Builder
	result.Grow(len(text))
	i := 0
	for i < len(text) {
		if text[i] != '{' {
			result.WriteByte(text[i])
			i++
			continue
		}
		end := findMatchingBrace(text, i)
		if end == -1 {
			result.WriteByte(text[i])
			i++
			continue
		}
		jsonStr := text[i:end]
		if strings.Contains(jsonStr, `"tool_calls"`) {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &data); err == nil {
				if _, ok := data["tool_calls"]; ok {
					i = end
					continue
				}
			}
		}
		result.WriteByte(text[i])
		i++
	}
	return result.String()
}

func generateCallID() string {
	seq := atomic.AddInt64(&callIDCounter, 1)
	return fmt.Sprintf("call_%d_%d", time.Now().UnixMilli(), seq)
}
