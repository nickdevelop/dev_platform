package chat

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestChatStaysIndependentOfAgentLayers keeps the non-agent chat feature
// free of the project, instance, session and provider layers. Chat must call a
// model service directly, never turn into a special agent backend.
func TestChatStaysIndependentOfAgentLayers(t *testing.T) {
	forbidden := []string{
		"devplatform/internal/core/agents",
		"devplatform/internal/providers/",
		"devplatform/internal/app/sessions",
		"devplatform/internal/app/instances",
		"devplatform/internal/app/projects",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if path == bad || strings.HasPrefix(path, bad) {
					t.Errorf("%s imports %s; pure chat must not depend on the agent layers", name, path)
				}
			}
		}
	}
}

// TestChatNamesNoBackend keeps backend identifiers out of the generic chat
// domain. A future backend integration belongs in providers or composition,
// not here.
func TestChatNamesNoBackend(t *testing.T) {
	forbidden := []string{"BackendOpenCodeV2", "BackendPi", "BackendDeepSeek", `"opencode-v2"`, `"pi"`, `"deepseek"`}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, literal := range forbidden {
			if strings.Contains(string(data), literal) {
				t.Errorf("%s names backend %s; chat learns models from modelconfig only", name, literal)
			}
		}
	}
}
