package sessions

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestSessionsDoesNotImportBackendPackages keeps the generic session layer
// backend-neutral: it may depend on the agents contract, never on a concrete
// backend package.
func TestSessionsDoesNotImportBackendPackages(t *testing.T) {
	forbidden := []string{
		"devplatform/internal/agents/opencode",
		"devplatform/internal/agents/pi",
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
				if path == bad {
					t.Errorf("%s imports backend package %s; the generic session layer must depend on internal/agents only", name, path)
				}
			}
		}
	}
}
