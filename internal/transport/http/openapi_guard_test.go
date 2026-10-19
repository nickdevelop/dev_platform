package http

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"devplatform/internal/app/agentservices"
	"devplatform/internal/app/auth"
	"devplatform/internal/app/instances"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/settings"
	"devplatform/internal/app/tasks"
	"devplatform/internal/app/terminal"
	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform/fake"
)

// openAPIDoc captures the top-level paths mapping from openapi.yaml.
type openAPIDoc struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

// TestAllRoutesDocumentedInOpenAPI verifies that every API route registered
// on the Gin router has a corresponding path and method documented in docs/api/openapi.yaml.
func TestAllRoutesDocumentedInOpenAPI(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	openAPIPath := filepath.Join(repoRoot, "docs", "api", "openapi.yaml")

	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	var doc openAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	db := openTestDB(t)
	registry := agents.NewRegistry()
	projSvc := projects.NewService(db)
	instSvc := instances.NewService(db, registry, projSvc)
	authSvc := auth.NewService(db)

	catalog, _ := agents.NewCatalog(
		agents.BackendDescriptor{ID: agents.BackendOpenCodeV2, Binary: "opencode", Order: 1},
		agents.BackendDescriptor{ID: agents.BackendPi, Binary: "pi", Order: 2},
	)
	settingsSvc := settings.NewService(db, catalog)
	tasksRepo := tasks.NewRepository(db)
	tasksSvc := tasks.NewService(tasksRepo, nil)

	router := NewRouter(Dependencies{
		Auth:          authSvc,
		Projects:      projSvc,
		Settings:      settingsSvc,
		Models:        modelconfig.NewService(db, nil),
		Instances:     instSvc,
		AgentServices: agentservices.NewService(nil),
		Sessions:      sessions.NewService(db, registry, instSvc),
		Tasks:         tasksSvc,
		Terminal:      terminal.NewService(terminal.NewRepository(db), nil, fake.New()),
	})

	paramRegex := regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

	for _, route := range router.Routes() {
		// Skip SPA static handler
		if strings.Contains(route.Path, "/*") {
			continue
		}

		// Strip /api prefix
		path := route.Path
		if strings.HasPrefix(path, "/api") {
			path = strings.TrimPrefix(path, "/api")
		}

		// Convert /projects/:id to /projects/{id}
		openAPIFormatPath := paramRegex.ReplaceAllString(path, "{$1}")
		method := strings.ToLower(route.Method)

		pathItem, exists := doc.Paths[openAPIFormatPath]
		if !exists {
			t.Errorf("undocumented route path in openapi.yaml: %s %s (from gin route %s)", route.Method, openAPIFormatPath, route.Path)
			continue
		}

		if _, methodExists := pathItem[method]; !methodExists {
			t.Errorf("undocumented method %s for path %s in openapi.yaml", route.Method, openAPIFormatPath)
		}
	}
}
