package agents

import (
	"encoding/json"
	"testing"
)

func TestClassifyTool(t *testing.T) {
	tests := []struct {
		name string
		want ToolAction
	}{
		{"read", ActionRead},
		{"read_file", ActionRead},
		{"readFile", ActionRead},
		{"edit", ActionEdit},
		{"write", ActionEdit},
		{"write_file", ActionEdit},
		{"bash", ActionCommand},
		{"sh", ActionCommand},
		{"shell", ActionCommand},
		{"exec", ActionCommand},
		{"grep", ActionSearch},
		{"glob", ActionSearch},
		{"search", ActionSearch},
		{"webfetch", ActionFetch},
		{"websearch", ActionFetch},
		{"question", ActionQuestion},
		{"ask_user", ActionQuestion},
		{"custom_tool", ActionOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyTool(tt.name); got != tt.want {
				t.Errorf("ClassifyTool(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestNormalizeQuestionToolInput(t *testing.T) {
	// DeepSeek style: single question with multi_select and string options
	dshSingle := []byte(`{
		"header": "Database",
		"question": "Which database would you prefer?",
		"options": ["SQLite", "PostgreSQL"],
		"multi_select": true
	}`)

	normalized := NormalizeQuestionToolInput(dshSingle)
	var parsed StandardQuestionPayload
	if err := json.Unmarshal(normalized, &parsed); err != nil {
		t.Fatalf("unmarshal normalized failed: %v", err)
	}

	if len(parsed.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(parsed.Questions))
	}
	q := parsed.Questions[0]
	if q.Header != "Database" || q.Question != "Which database would you prefer?" || !q.Multiple {
		t.Errorf("question item mismatch: %+v", q)
	}
	if len(q.Options) != 2 || q.Options[0].Label != "SQLite" || q.Options[1].Label != "PostgreSQL" {
		t.Errorf("options mismatch: %+v", q.Options)
	}

	// DeepSeek questions array with detail
	dshArray := []byte(`{
		"questions": [
			{
				"id": "q1",
				"header": "Framework",
				"question": "Pick framework",
				"detail": "Frontend framework selection",
				"options": [
					{"label": "Vue", "description": "Vue 3"},
					{"label": "React", "description": "React 19"}
				],
				"multi_select": false
			}
		]
	}`)
	normalizedArr := NormalizeQuestionToolInput(dshArray)
	var parsedArr StandardQuestionPayload
	if err := json.Unmarshal(normalizedArr, &parsedArr); err != nil {
		t.Fatalf("unmarshal array failed: %v", err)
	}
	if len(parsedArr.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(parsedArr.Questions))
	}
	item := parsedArr.Questions[0]
	if item.Header != "Framework" || item.Question != "Pick framework" || item.Multiple {
		t.Errorf("array item mismatch: %+v", item)
	}
	if len(item.Options) != 2 || item.Options[0].Label != "Vue" || item.Options[0].Description != "Vue 3" {
		t.Errorf("array options mismatch: %+v", item.Options)
	}
}
