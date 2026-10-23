package sessions

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenericLayersDoNotImportBackendPackages keeps all generic app and transport
// layers backend-neutral: they may depend on the agents contract, never on a concrete
// backend package in internal/providers.
func TestGenericLayersDoNotImportBackendPackages(t *testing.T) {
	forbidden := []string{
		"devplatform/internal/providers/opencode",
		"devplatform/internal/providers/pi",
		"devplatform/internal/providers/deepseek",
	}

	roots := []string{"..", "../../transport"}
	fset := token.NewFileSet()

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imp := range file.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				for _, bad := range forbidden {
					if impPath == bad {
						t.Errorf("%s imports backend package %s; generic layers must depend on internal/core/agents only", path, impPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

// TestGenericLayersNameNoBackend keeps all generic layers free of backend
// identifiers: they learn about a backend from the catalog and the registry,
// never from a literal. The composition root is the one place that names them.
func TestGenericLayersNameNoBackend(t *testing.T) {
	forbidden := []string{"BackendOpenCodeV2", "BackendPi", "BackendDeepSeek", `"opencode-v2"`, `"pi"`, `"deepseek"`}
	roots := []string{"..", "../../transport"}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, literal := range forbidden {
				if strings.Contains(string(data), literal) {
					t.Errorf("%s names backend %s; take it from the catalog or the registry", path, literal)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
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
