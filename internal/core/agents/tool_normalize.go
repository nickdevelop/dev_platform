package agents

import (
	"encoding/json"
	"strings"
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
