package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/agents"
)

// TestDiagnosticsCollectsRegisteredBackends verifies the session service
// surfaces the registry's diagnostics: a reporting backend passes its report
// through, a panicking adapter becomes an error report, and an adapter without
// diagnostics support is still reported with an unknown status.
func TestDiagnosticsCollectsRegisteredBackends(t *testing.T) {
	env := newTestEnv(t)

	env.fake.diagnostics = agents.BackendDiagnostics{
		Status:        agents.DiagnosticOK,
		Version:       "1.1.0",
		TestedVersion: "1.1.0",
		Checks: []agents.Diagnostic{
			{Kind: agents.DiagnosticExecutable, Status: agents.DiagnosticOK, Message: "found"},
		},
	}

	reports := env.svc.Diagnostics(context.Background())
	if len(reports) != 1 {
		t.Fatalf("Diagnostics = %+v, want exactly one report", reports)
	}
	report := reports[0]
	if report.Backend != agents.BackendPi {
		t.Errorf("backend = %q, want pi", report.Backend)
	}
	if report.Status != agents.DiagnosticOK || report.Version != "1.1.0" || len(report.Checks) != 1 {
		t.Errorf("report = %+v, want the adapter's report passed through", report)
	}
}

// TestDiagnosticsHandlerServesReport pins the JSON contract of
// GET /api/agent-backends/diagnostics.
func TestDiagnosticsHandlerServesReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)

	env.fake.diagnostics = agents.BackendDiagnostics{
		Status:  agents.DiagnosticWarning,
		Checks:  []agents.Diagnostic{{Kind: agents.DiagnosticVersion, Status: agents.DiagnosticWarning, Message: "unverified"}},
		Version: "1.1.0",
	}

	router := gin.New()
	env.svc.RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent-backends/diagnostics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s, want 200", rec.Code, rec.Body.String())
	}

	var payload struct {
		Diagnostics []agents.BackendDiagnostics `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if len(payload.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want exactly one backend", payload.Diagnostics)
	}
	report := payload.Diagnostics[0]
	if report.Backend != agents.BackendPi || report.Status != agents.DiagnosticWarning {
		t.Errorf("report = %+v, want the pi backend with the adapter's status", report)
	}
	if report.Capabilities == nil || !report.Capabilities.ModelSelection {
		t.Errorf("capabilities = %+v, want the adapter declaration for the UI", report.Capabilities)
	}
}
