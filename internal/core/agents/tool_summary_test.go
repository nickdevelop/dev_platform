package agents

import (
	"encoding/json"
	"strings"
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

func TestSummarizeToolResult(t *testing.T) {
	tests := []struct {
		desc      string
		name      string
		input     string
		rawOutput string
		isError   bool
		wantSub   string
	}{
		{
			desc:    "read success",
			name:    "read_file",
			input:   `{"path": "web/src/App.vue"}`,
			wantSub: "读取了 web/src/App.vue 文件",
		},
		{
			desc:      "read error",
			name:      "read",
			input:     `{"path": "missing.txt"}`,
			rawOutput: "file not found\nmore error info",
			isError:   true,
			wantSub:   "读取 missing.txt 文件失败: file not found",
		},
		{
			desc:    "edit success",
			name:    "edit",
			input:   `{"path": "main.go", "oldString": "a", "newString": "b"}`,
			wantSub: "编辑了 main.go 文件",
		},
		{
			desc:    "write success",
			name:    "write_file",
			input:   `{"path": "README.md", "content": "hello"}`,
			wantSub: "写入了 README.md 文件",
		},
		{
			desc:      "edit error",
			name:      "edit",
			input:     `{"path": "main.go"}`,
			rawOutput: "permission denied",
			isError:   true,
			wantSub:   "编辑 main.go 文件失败: permission denied",
		},
		{
			desc:    "bash command success",
			name:    "bash",
			input:   `{"command": "npm run build"}`,
			wantSub: "执行了 npm run build 命令",
		},
		{
			desc:      "shell command error",
			name:      "shell",
			input:     `{"command": "cat foo"}`,
			rawOutput: "cat: foo: No such file or directory",
			isError:   true,
			wantSub:   "执行 cat foo 命令失败: cat: foo: No such file or directory",
		},
		{
			desc:    "search success",
			name:    "grep",
			input:   `{"pattern": "TODO"}`,
			wantSub: "检索了 TODO",
		},
		{
			desc:    "webfetch success",
			name:    "webfetch",
			input:   `{"url": "https://example.com"}`,
			wantSub: "访问了 https://example.com",
		},
		{
			desc:    "websearch success",
			name:    "websearch",
			input:   `{"query": "golang 1.24 release"}`,
			wantSub: "搜索了 golang 1.24 release",
		},
		{
			desc:      "question answer",
			name:      "question",
			input:     `{"question": "Proceed?"}`,
			rawOutput: "Yes, continue",
			wantSub:   "已回答: Yes, continue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := SummarizeToolResult(tt.name, []byte(tt.input), tt.rawOutput, tt.isError)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("SummarizeToolResult() = %q, want substring %q", got, tt.wantSub)
			}
		})
	}
}

func TestPruneToolInput(t *testing.T) {
	largeContent := strings.Repeat("A", 1000)
	input := []byte(`{"path": "test.txt", "content": "` + largeContent + `"}`)
	pruned := PruneToolInput("write", input)

	if len(pruned) >= len(input) {
		t.Errorf("PruneToolInput() length = %d, want smaller than %d", len(pruned), len(input))
	}

	var parsed map[string]any
	if err := json.Unmarshal(pruned, &parsed); err != nil {
		t.Fatalf("unmarshal pruned failed: %v", err)
	}
	if parsed["path"] != "test.txt" {
		t.Errorf("expected path to be retained, got %v", parsed["path"])
	}
	contentStr, ok := parsed["content"].(string)
	if !ok || !strings.Contains(contentStr, "[内容已省略]") {
		t.Errorf("expected content to be pruned with truncation mark, got %v", parsed["content"])
	}

	// Question input should NOT be pruned
	questionInput := []byte(`{"questions": [{"question": "Which option?", "options": ["A", "B"]}]}`)
	prunedQ := PruneToolInput("question", questionInput)
	if string(prunedQ) != string(questionInput) {
		t.Errorf("expected question input to stay untouched, got %s", string(prunedQ))
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
