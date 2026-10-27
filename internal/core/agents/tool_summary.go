package agents

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ToolAction classifies the tool operation type.
type ToolAction string

const (
	ActionRead     ToolAction = "read"
	ActionEdit     ToolAction = "edit"
	ActionCommand  ToolAction = "command"
	ActionSearch   ToolAction = "search"
	ActionFetch    ToolAction = "fetch"
	ActionQuestion ToolAction = "question"
	ActionSubagent ToolAction = "subagent"
	ActionOther    ToolAction = "other"
)

// ClassifyTool categorizes a tool by its name.
func ClassifyTool(name string) ToolAction {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "task", "subagent", "delegate", "spawn":
		return ActionSubagent
	case "read", "read_file", "readfile", "view_file", "view", "cat", "fetch_file", "open_file":
		return ActionRead
	case "edit", "write", "write_file", "writefile", "edit_file", "editfile", "patch", "apply_patch", "append_file", "str_replace_editor":
		return ActionEdit
	case "bash", "sh", "shell", "exec", "command", "run_command", "terminal", "run_shell":
		return ActionCommand
	case "grep", "glob", "search", "find", "file_search", "rg":
		return ActionSearch
	case "webfetch", "fetch", "websearch", "web_search", "curl", "http":
		return ActionFetch
	case "question", "ask", "ask_user", "askuserquestion", "ask_user_question", "ask_question", "ask_followup_question", "inquire":
		return ActionQuestion
	default:
		if strings.Contains(lower, "ask_user") || strings.Contains(lower, "question") {
			return ActionQuestion
		}
		if strings.HasPrefix(lower, "read") {
			return ActionRead
		}
		if strings.HasPrefix(lower, "edit") || strings.HasPrefix(lower, "write") {
			return ActionEdit
		}
		if strings.Contains(lower, "shell") || strings.Contains(lower, "bash") || strings.Contains(lower, "exec") {
			return ActionCommand
		}
		if strings.Contains(lower, "search") || strings.Contains(lower, "find") {
			return ActionSearch
		}
		return ActionOther
	}
}

// ToolArgumentKeys in priority order to extract target argument.
var toolArgumentKeys = []string{
	"path", "filePath", "file_path", "file",
	"command", "cmd", "script",
	"pattern", "query",
	"url",
	"description", "prompt", "name",
}

// ExtractToolArgument extracts a concise string representation of the target argument.
func ExtractToolArgument(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	var raw any
	if err := json.Unmarshal(input, &raw); err != nil {
		str := strings.TrimSpace(string(input))
		return truncateSummaryString(str, 80)
	}
	record, ok := raw.(map[string]any)
	if !ok {
		if str, isStr := raw.(string); isStr {
			return truncateSummaryString(strings.TrimSpace(str), 80)
		}
		return ""
	}

	for _, key := range toolArgumentKeys {
		if val, exists := record[key]; exists {
			if strVal, isStr := val.(string); isStr && strings.TrimSpace(strVal) != "" {
				return truncateSummaryString(strings.TrimSpace(strVal), 80)
			}
		}
	}

	// Fallback to the first non-empty string value
	for _, val := range record {
		if strVal, isStr := val.(string); isStr && strings.TrimSpace(strVal) != "" {
			return truncateSummaryString(strings.TrimSpace(strVal), 80)
		}
	}
	return ""
}

// SummarizeToolResult produces a human-friendly Chinese summary of a tool invocation.
func SummarizeToolResult(name string, input []byte, rawOutput string, isError bool) string {
	action := ClassifyTool(name)
	arg := ExtractToolArgument(input)
	nameLower := strings.ToLower(strings.TrimSpace(name))

	if isError {
		errMsg := cleanErrorMessage(rawOutput)
		switch action {
		case ActionRead:
			if arg != "" {
				return fmt.Sprintf("读取 %s 文件失败%s", arg, errMsg)
			}
			return fmt.Sprintf("读取文件失败%s", errMsg)
		case ActionEdit:
			verb := "编辑"
			if strings.Contains(nameLower, "write") {
				verb = "写入"
			}
			if arg != "" {
				return fmt.Sprintf("%s %s 文件失败%s", verb, arg, errMsg)
			}
			return fmt.Sprintf("%s文件失败%s", verb, errMsg)
		case ActionCommand:
			if arg != "" {
				return fmt.Sprintf("执行 %s 命令失败%s", arg, errMsg)
			}
			return fmt.Sprintf("执行命令失败%s", errMsg)
		case ActionSearch:
			if arg != "" {
				return fmt.Sprintf("检索 %s 失败%s", arg, errMsg)
			}
			return fmt.Sprintf("检索失败%s", errMsg)
		case ActionFetch:
			if arg != "" {
				return fmt.Sprintf("请求 %s 失败%s", arg, errMsg)
			}
			return fmt.Sprintf("请求失败%s", errMsg)
		case ActionQuestion:
			return fmt.Sprintf("问答处理失败%s", errMsg)
		case ActionSubagent:
			if arg != "" {
				return fmt.Sprintf("子任务 %s 执行失败%s", arg, errMsg)
			}
			return fmt.Sprintf("子任务执行失败%s", errMsg)
		default:
			toolDisplayName := name
			if toolDisplayName == "" {
				toolDisplayName = "工具"
			}
			if arg != "" {
				return fmt.Sprintf("执行 %s (%s) 失败%s", toolDisplayName, arg, errMsg)
			}
			return fmt.Sprintf("执行 %s 失败%s", toolDisplayName, errMsg)
		}
	}

	// Success cases
	switch action {
	case ActionRead:
		if arg != "" {
			return fmt.Sprintf("读取了 %s 文件", arg)
		}
		return "读取了文件"
	case ActionEdit:
		verb := "编辑"
		if strings.Contains(nameLower, "write") {
			verb = "写入"
		}
		if arg != "" {
			return fmt.Sprintf("%s了 %s 文件", verb, arg)
		}
		return fmt.Sprintf("%s了文件", verb)
	case ActionCommand:
		if arg != "" {
			return fmt.Sprintf("执行了 %s 命令", arg)
		}
		return "执行了命令"
	case ActionSearch:
		if arg != "" {
			return fmt.Sprintf("检索了 %s", arg)
		}
		return "执行了检索"
	case ActionFetch:
		if strings.Contains(nameLower, "search") {
			if arg != "" {
				return fmt.Sprintf("搜索了 %s", arg)
			}
			return "执行了网络搜索"
		}
		if arg != "" {
			return fmt.Sprintf("访问了 %s", arg)
		}
		return "完成了网络请求"
	case ActionQuestion:
		ans := strings.TrimSpace(rawOutput)
		if ans != "" {
			return fmt.Sprintf("已回答: %s", truncateSummaryString(ans, 8192))
		}
		return "问答交互完成"
	case ActionSubagent:
		if strings.Contains(rawOutput, "<subagent") || strings.Contains(rawOutput, "sessionID=") {
			return truncateSummaryString(rawOutput, 240)
		}
		if arg != "" {
			return fmt.Sprintf("执行了子任务 %s", arg)
		}
		return "子任务执行完成"
	default:
		toolDisplayName := name
		if toolDisplayName == "" {
			toolDisplayName = "工具"
		}
		if arg != "" {
			return fmt.Sprintf("执行了 %s (%s)", toolDisplayName, arg)
		}
		return fmt.Sprintf("执行了 %s 工具", toolDisplayName)
	}
}

// PruneToolInput trims excessively large fields (e.g. file content in write/edit tools)
// to avoid sending hundreds of kilobytes over SSE when only metadata is needed.
func PruneToolInput(name string, input []byte) []byte {
	if len(input) == 0 {
		return input
	}
	action := ClassifyTool(name)
	if action == ActionQuestion || action == ActionSubagent {
		// Keep question and subagent payloads intact so the frontend can render them.
		return input
	}

	var parsed map[string]any
	if err := json.Unmarshal(input, &parsed); err != nil {
		if len(input) > 512 {
			return []byte(`{"_truncated":true}`)
		}
		return input
	}

	pruned := false
	largeKeys := []string{"content", "text", "oldString", "newString", "old_string", "new_string", "file_text"}
	for _, key := range largeKeys {
		if val, exists := parsed[key]; exists {
			if strVal, ok := val.(string); ok && len(strVal) > 120 {
				parsed[key] = truncateSummaryString(strVal, 60) + " [内容已省略]"
				pruned = true
			}
		}
	}

	if !pruned {
		return input
	}

	encoded, err := json.Marshal(parsed)
	if err != nil {
		return input
	}
	return encoded
}

func truncateSummaryString(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

func cleanErrorMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	firstLine := strings.Split(raw, "\n")[0]
	firstLine = strings.TrimSpace(firstLine)
	if firstLine == "" {
		return ""
	}
	return ": " + truncateSummaryString(firstLine, 80)
}

// StandardToolQuestion is the canonical tool name for human interaction/questioning.
const StandardToolQuestion = "question"

// StandardQuestionOption represents a normalized option for question tools.
type StandardQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// StandardQuestionItem represents a single normalized question item.
type StandardQuestionItem struct {
	ID       string                   `json:"id,omitempty"`
	Header   string                   `json:"header,omitempty"`
	Question string                   `json:"question"`
	Options  []StandardQuestionOption `json:"options,omitempty"`
	Multiple bool                     `json:"multiple,omitempty"`
}

// StandardQuestionPayload represents the canonical payload for question tools.
type StandardQuestionPayload struct {
	Questions []StandardQuestionItem `json:"questions"`
}

// NormalizeQuestionToolInput translates heterogeneous question tool payloads
// (e.g. from DeepSeek's ask_user_question or OpenCode's question) into a canonical shape.
func NormalizeQuestionToolInput(input []byte) []byte {
	if len(input) == 0 {
		return input
	}
	var raw any
	if err := json.Unmarshal(input, &raw); err != nil {
		return input
	}
	// Handle nested JSON string encoding (e.g. arguments serialized as string)
	if strVal, isStr := raw.(string); isStr {
		var inner any
		if err := json.Unmarshal([]byte(strVal), &inner); err == nil {
			raw = inner
		}
	}
	record, ok := raw.(map[string]any)
	if !ok {
		return input
	}

	var items []StandardQuestionItem

	// Check if it has a "questions" array
	if rawQuestions, hasQuestions := record["questions"]; hasQuestions {
		if qList, ok := rawQuestions.([]any); ok {
			for _, qItem := range qList {
				if itemMap, ok := qItem.(map[string]any); ok {
					item := parseSingleQuestionItem(itemMap)
					if item.Question != "" {
						items = append(items, item)
					}
				} else if strVal, ok := qItem.(string); ok && strings.TrimSpace(strVal) != "" {
					items = append(items, StandardQuestionItem{Question: strings.TrimSpace(strVal)})
				}
			}
		}
	} else {
		// Single question object
		item := parseSingleQuestionItem(record)
		if item.Question != "" {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return input
	}

	canonical := StandardQuestionPayload{Questions: items}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return input
	}
	return encoded
}

func parseSingleQuestionItem(m map[string]any) StandardQuestionItem {
	var item StandardQuestionItem

	// ID
	if idVal, ok := m["id"].(string); ok {
		item.ID = strings.TrimSpace(idVal)
	}

	// Header
	if h, ok := m["header"].(string); ok {
		item.Header = strings.TrimSpace(h)
	}

	// Question text (question / prompt / detail / text)
	if q, ok := m["question"].(string); ok && strings.TrimSpace(q) != "" {
		item.Question = strings.TrimSpace(q)
	} else if p, ok := m["prompt"].(string); ok && strings.TrimSpace(p) != "" {
		item.Question = strings.TrimSpace(p)
	} else if t, ok := m["text"].(string); ok && strings.TrimSpace(t) != "" {
		item.Question = strings.TrimSpace(t)
	}

	// If header is empty and question has detail, use detail as header or vice versa
	if item.Header == "" {
		if d, ok := m["detail"].(string); ok && strings.TrimSpace(d) != "" && item.Question != strings.TrimSpace(d) {
			item.Header = strings.TrimSpace(d)
		}
	}

	// Multiple flag (multiple / multi_select / multiSelect)
	if mul, ok := m["multiple"].(bool); ok {
		item.Multiple = mul
	} else if mulSnake, ok := m["multi_select"].(bool); ok {
		item.Multiple = mulSnake
	} else if mulCamel, ok := m["multiSelect"].(bool); ok {
		item.Multiple = mulCamel
	}

	// Options
	if rawOptions, ok := m["options"].([]any); ok {
		for _, rawOpt := range rawOptions {
			if optMap, ok := rawOpt.(map[string]any); ok {
				label := ""
				if l, ok := optMap["label"].(string); ok {
					label = strings.TrimSpace(l)
				} else if v, ok := optMap["value"].(string); ok {
					label = strings.TrimSpace(v)
				}
				if label == "" {
					continue
				}
				desc := ""
				if d, ok := optMap["description"].(string); ok {
					desc = strings.TrimSpace(d)
				}
				item.Options = append(item.Options, StandardQuestionOption{
					Label:       label,
					Description: desc,
				})
			} else if optStr, ok := rawOpt.(string); ok && strings.TrimSpace(optStr) != "" {
				item.Options = append(item.Options, StandardQuestionOption{
					Label: strings.TrimSpace(optStr),
				})
			}
		}
	}

	return item
}
