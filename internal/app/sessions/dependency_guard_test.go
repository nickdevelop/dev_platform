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
		"devplatform/internal/providers/opencode",
		"devplatform/internal/providers/pi",
		"devplatform/internal/providers/deepseek",
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
					t.Errorf("%s imports backend package %s; the generic session layer must depend on internal/core/agents only", name, path)
				}
			}
		}
	}
}

// TestGenericPackagesNameNoBackend keeps the generic layers free of backend
// identifiers: they learn about a backend from the catalog and the registry,
// never from a literal. The composition root is the one place that names them.
func TestGenericPackagesNameNoBackend(t *testing.T) {
	forbidden := []string{"BackendOpenCodeV2", "BackendPi", "BackendDeepSeek", `"opencode-v2"`, `"pi"`, `"deepseek"`}
	for _, dir := range []string{".", "../settings", "../../transport/http"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			data, err := os.ReadFile(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			for _, literal := range forbidden {
				if strings.Contains(string(data), literal) {
					t.Errorf("%s/%s names backend %s; take it from the catalog or the registry", dir, name, literal)
				}
			}
		}
	}
}

// TestSessionsDoesNotNameNativeTurnEvents keeps turn tracking on the typed
// contract. Backend-native lifecycle names (opencode's execution_* records, pi's
// agent_* records) are the adapters' business: they map them to EventTurn, and
// the generic session layer must never match on them again.
func TestSessionsDoesNotNameNativeTurnEvents(t *testing.T) {
	forbidden := []string{
		"execution_started", "execution_succeeded", "execution_interrupted", "execution_failed",
		"agent_start", "agent_end", "agent_settled", "turn_start", "turn_end",
	}
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
				t.Errorf("%s names native turn event %q; map it to agents.EventTurn in the adapter", name, literal)
			}
		}
	}
}
