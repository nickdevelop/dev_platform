package pi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"devplatform/internal/core/agents"
)

// formOption mirrors the standard option schema rendered by the workbench
// (web/src/features/chat/interactions/types.ts).
type formOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// formField mirrors the standard field schema rendered by QuestionQueue.
type formField struct {
	Key         string       `json:"key"`
	Type        string       `json:"type"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Required    bool         `json:"required,omitempty"`
	Options     []formOption `json:"options,omitempty"`
	Custom      bool         `json:"custom,omitempty"`
}

// piPendingForm tracks an active questionnaire form within an RPC child session.
type piPendingForm struct {
	mu sync.Mutex

	info      agents.FormInfo
	items     []agents.StandardQuestionItem
	submitted bool
	answers   map[int]string // question index -> formatted answer string for pi extension

	currentWaitIndex int
	pendingRequestID string
	pendingMethod    string

	// For standalone select/input dialogs not attached to a tool questionnaire
	standalone bool
}

// buildQuestionnaireForm constructs a standard FormInfo from a question tool's items.
func buildQuestionnaireForm(sessionID, formID string, items []agents.StandardQuestionItem) (agents.FormInfo, error) {
	fields := make([]formField, 0, len(items))
	title := "问题"

	for index, item := range items {
		if index == 0 {
			if h := strings.TrimSpace(item.Header); h != "" {
				title = h
			} else if q := strings.TrimSpace(item.Question); q != "" {
				title = q
			}
		}

		key := strings.TrimSpace(item.ID)
		if key == "" {
			key = fmt.Sprintf("q%d", index)
		}

		field := formField{
			Key:      key,
			Type:     "string",
			Title:    strings.TrimSpace(item.Header),
			Required: true,
		}

		if field.Title == "" {
			field.Title = strings.TrimSpace(item.Question)
		} else {
			field.Description = strings.TrimSpace(item.Question)
		}
		if field.Title == "" {
			field.Title = key
		}

		if len(item.Options) > 0 {
			field.Options = make([]formOption, 0, len(item.Options))
			for _, opt := range item.Options {
				field.Options = append(field.Options, formOption{
					Value:       opt.Label,
					Label:       opt.Label,
					Description: opt.Description,
				})
			}
			if item.Multiple {
				field.Type = "multiselect"
			} else {
				field.Custom = true
			}
		}

		fields = append(fields, field)
	}

	encoded, err := json.Marshal(fields)
	if err != nil {
		return agents.FormInfo{}, fmt.Errorf("pi: encode form fields: %w", err)
	}

	return agents.FormInfo{
		ID:        formID,
		SessionID: sessionID,
		Title:     title,
		Fields:    encoded,
	}, nil
}

// formatQuestionAnswer translates the user's form answer into the string representation
// expected by pi's ask_user_question plugin (in rpc-fallback mode).
//
// For single-select (select dialog):
//   - If the user selected an option label, returns the label.
//   - If the user entered custom text, returns the custom text.
//
// For multi-select (input dialog):
//   - Pi expects "1,3" or "1,3; custom answer" (1-based indices into options).
//   - If the user selected options, maps them to 1-based indices comma-separated.
//   - Any non-option custom text is appended after a semicolon.
func formatQuestionAnswer(item agents.StandardQuestionItem, rawAnswer any) string {
	values := extractAnswerValues(rawAnswer)
	if len(values) == 0 {
		return ""
	}

	if !item.Multiple {
		// Single select or custom text: return the first non-empty value verbatim
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}

	// Multi-select
	optionIndexMap := make(map[string]int, len(item.Options))
	for idx, opt := range item.Options {
		optionIndexMap[opt.Label] = idx + 1
	}

	var selectedIndices []string
	var customTexts []string

	for _, val := range values {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		if idx, ok := optionIndexMap[val]; ok {
			selectedIndices = append(selectedIndices, strconv.Itoa(idx))
		} else {
			customTexts = append(customTexts, val)
		}
	}

	var parts []string
	if len(selectedIndices) > 0 {
		parts = append(parts, strings.Join(selectedIndices, ","))
	}
	if len(customTexts) > 0 {
		custom := strings.Join(customTexts, "; ")
		if len(parts) > 0 {
			parts = append(parts, custom)
			return strings.Join(parts, "; ")
		}
		return custom
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// extractAnswerValues flattens string/slice representations from the frontend into []string.
func extractAnswerValues(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []string:
		return typed
	case []any:
		res := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				res = append(res, strings.TrimSpace(s))
			} else if item != nil {
				res = append(res, fmt.Sprint(item))
			}
		}
		return res
	default:
		s := fmt.Sprint(typed)
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

// piFormManager tracks active question forms for an RPC child.
type piFormManager struct {
	mu    sync.Mutex
	forms map[string]*piPendingForm
}

func newPiFormManager() *piFormManager {
	return &piFormManager{
		forms: make(map[string]*piPendingForm),
	}
}

// addQuestionnaireForm registers a form created from a tool call.
func (m *piFormManager) addQuestionnaireForm(info agents.FormInfo, items []agents.StandardQuestionItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forms[info.ID] = &piPendingForm{
		info:    info,
		items:   items,
		answers: make(map[int]string),
	}
}

// addStandaloneForm registers a form created from a standalone select/input dialog.
func (m *piFormManager) addStandaloneForm(info agents.FormInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forms[info.ID] = &piPendingForm{
		info:             info,
		standalone:       true,
		pendingRequestID: info.ID,
	}
}

// list returns all pending forms that have not yet been submitted.
func (m *piFormManager) list(sessionID string) []agents.FormInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]agents.FormInfo, 0, len(m.forms))
	for _, form := range m.forms {
		form.mu.Lock()
		submitted := form.submitted
		info := form.info
		form.mu.Unlock()
		if !submitted {
			info.SessionID = sessionID
			list = append(list, info)
		}
	}
	return list
}

// get returns the pending form by formID.
func (m *piFormManager) get(formID string) *piPendingForm {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.forms[formID]
}

// remove deletes a form by formID.
func (m *piFormManager) remove(formID string) *piPendingForm {
	m.mu.Lock()
	defer m.mu.Unlock()
	form := m.forms[formID]
	delete(m.forms, formID)
	return form
}

// findActiveQuestionnaireForm returns the first questionnaire form that is active
// (either awaiting user input or currently replaying answers to pi).
func (m *piFormManager) findActiveQuestionnaireForm() *piPendingForm {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.forms {
		if !f.standalone {
			return f
		}
	}
	return nil
}

// drain removes all forms and returns their IDs.
func (m *piFormManager) drain() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.forms))
	for id := range m.forms {
		ids = append(ids, id)
	}
	m.forms = make(map[string]*piPendingForm)
	return ids
}
