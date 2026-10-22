package http

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// openAPIDoc captures the parts of docs/api/openapi.yaml the guards assert on.
type openAPIDoc struct {
	Paths map[string]openAPIPathItem `yaml:"paths"`
}

// openAPIPathItem is one documented path. The parameters declared here apply to
// every operation under it, so the CSRF guard has to consider them too.
type openAPIPathItem struct {
	Parameters []openAPIParameter `yaml:"parameters"`
	Get        *openAPIOperation  `yaml:"get"`
	Post       *openAPIOperation  `yaml:"post"`
	Put        *openAPIOperation  `yaml:"put"`
	Patch      *openAPIOperation  `yaml:"patch"`
	Delete     *openAPIOperation  `yaml:"delete"`
}

// operation returns the documented operation for method, or nil when the method
// is not documented for this path.
func (item openAPIPathItem) operation(method string) *openAPIOperation {
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPut:
		return item.Put
	case http.MethodPatch:
		return item.Patch
	case http.MethodDelete:
		return item.Delete
	default:
		return nil
	}
}

// openAPIOperation carries the two ways an operation can require CSRF: a
// header parameter (X-CSRF-Token) or a security requirement naming the scheme.
type openAPIOperation struct {
	Parameters []openAPIParameter    `yaml:"parameters"`
	Security   []map[string][]string `yaml:"security"`
}

type openAPIParameter struct {
	Ref  string `yaml:"$ref"`
	Name string `yaml:"name"`
	In   string `yaml:"in"`
}

// csrfHeaderName mirrors auth.CSRFHeaderName; imported by value so the spec
// stays readable as plain OpenAPI.
const csrfHeaderName = "X-CSRF-Token"

// minDocumentedMutations is the floor the CSRF documentation guard must cover,
// so a broken parser cannot make the check pass vacuously.
const minDocumentedMutations = 40

// readOpenAPI loads and parses docs/api/openapi.yaml.
func readOpenAPI(t *testing.T) openAPIDoc {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	var doc openAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	return doc
}

// TestAllRoutesDocumentedInOpenAPI verifies that every API route registered
// on the Gin router has a corresponding path and method documented in docs/api/openapi.yaml.
func TestAllRoutesDocumentedInOpenAPI(t *testing.T) {
	doc := readOpenAPI(t)

	// The fixture mounts every service so the route table under test is the
	// production one, not a hand-written subset that can drift.
	router := newFullRouter(t)

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

		pathItem, exists := doc.Paths[openAPIFormatPath]
		if !exists {
			t.Errorf("undocumented route path in openapi.yaml: %s %s (from gin route %s)", route.Method, openAPIFormatPath, route.Path)
			continue
		}

		if pathItem.operation(route.Method) == nil {
			t.Errorf("undocumented method %s for path %s in openapi.yaml", route.Method, openAPIFormatPath)
		}
	}
}

// TestMutatingOperationsDocumentCSRF verifies that every documented
// state-changing operation declares the double-submit header, either as the
// X-CSRF-Token parameter or as a csrfToken security requirement.
//
// The transport attaches that header automatically, so the spec is the only
// place an API consumer (or an e2e test written against the spec) can learn that
// the header is required. A write endpoint documented without it is a trap.
func TestMutatingOperationsDocumentCSRF(t *testing.T) {
	doc := readOpenAPI(t)

	var offenders []string
	documented := 0
	for path, item := range doc.Paths {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			op := item.operation(method)
			if op == nil {
				continue
			}
			documented++
			if !declaresCSRF(item, op) {
				offenders = append(offenders, method+" "+path)
			}
		}
	}

	sort.Strings(offenders)
	for _, offender := range offenders {
		t.Errorf("%s does not declare the %s header in docs/api/openapi.yaml", offender, csrfHeaderName)
	}
	if documented < minDocumentedMutations {
		t.Fatalf("checked %d documented mutating operations, want at least %d", documented, minDocumentedMutations)
	}
}

// declaresCSRF reports whether the operation (or its path) requires the
// double-submit header.
func declaresCSRF(item openAPIPathItem, op *openAPIOperation) bool {
	for _, params := range [][]openAPIParameter{item.Parameters, op.Parameters} {
		for _, param := range params {
			if strings.HasSuffix(param.Ref, "/CSRFToken") {
				return true
			}
			if strings.EqualFold(param.Name, csrfHeaderName) && strings.EqualFold(param.In, "header") {
				return true
			}
		}
	}
	for _, requirement := range op.Security {
		if _, ok := requirement["csrfToken"]; ok {
			return true
		}
	}
	return false
}
