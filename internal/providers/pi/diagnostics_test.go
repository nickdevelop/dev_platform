package pi

import (
	"context"
	"strings"
	"testing"

	"devplatform/internal/core/agents"
)

// TestAdapterDiagnosticsMissingExecutable reports an actionable error and
// stops there: without the pi binary there is nothing left to probe.
func TestAdapterDiagnosticsMissingExecutable(t *testing.T) {
	adapter := New(Options{Executable: "definitely-not-a-real-pi-binary-xyz"})

	report := adapter.Diagnostics(context.Background())

	if report.Backend != agents.BackendPi {
		t.Fatalf("backend = %q, want pi", report.Backend)
	}
	if report.Status != agents.DiagnosticError {
		t.Fatalf("status = %q, want error", report.Status)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("checks = %+v, want only the executable check", report.Checks)
	}
	check := report.Checks[0]
	if check.Kind != agents.DiagnosticExecutable || check.Status != agents.DiagnosticError {
		t.Errorf("check = %+v, want an executable error", check)
	}
	if !strings.Contains(check.Message, "install") {
		t.Errorf("message %q is not actionable: it must tell the operator how to fix the missing binary", check.Message)
	}
}

// TestAdapterDiagnosticsHealthy pins the all-clear report for the pi CLI:
// executable found, tested version accepted, capabilities declared. pi is a
// CLI backend, so the report has no api check.
func TestAdapterDiagnosticsHealthy(t *testing.T) {
	adapter := New(Options{
		Executable:   writeFakePi(t, t.TempDir(), "exit 0"),
		VersionProbe: func(context.Context) (string, error) { return "1.1.0", nil },
	})

	report := adapter.Diagnostics(context.Background())

	if report.Status != agents.DiagnosticOK {
		t.Fatalf("status = %q, want ok (checks: %+v)", report.Status, report.Checks)
	}
	if report.Version != "1.1.0" || report.TestedVersion != "1.1.0" {
		t.Errorf("version = %q, tested = %q, want both 1.1.0", report.Version, report.TestedVersion)
	}
	if len(report.Checks) != 2 {
		t.Fatalf("checks = %+v, want exactly executable and version (pi has no API server)", report.Checks)
	}
	for _, check := range report.Checks {
		if check.Status != agents.DiagnosticOK {
			t.Errorf("check %+v = %q, want ok", check.Kind, check.Status)
		}
	}
	if report.Capabilities == nil || !report.Capabilities.ToolCallEvents || report.Capabilities.ToolConfirmation {
		t.Errorf("capabilities = %+v, want the adapter declaration", report.Capabilities)
	}
}

// TestAdapterDiagnosticsUnsupportedVersion warns when the CLI version is
// outside the tested range, naming the offending version and the range.
func TestAdapterDiagnosticsUnsupportedVersion(t *testing.T) {
	adapter := New(Options{
		Executable:   writeFakePi(t, t.TempDir(), "exit 0"),
		VersionProbe: func(context.Context) (string, error) { return "2.4.0", nil },
	})

	report := adapter.Diagnostics(context.Background())

	if report.Status != agents.DiagnosticWarning {
		t.Fatalf("status = %q, want warning", report.Status)
	}
	if report.Version != "2.4.0" {
		t.Errorf("version = %q, want 2.4.0", report.Version)
	}
	version := report.Checks[1]
	if version.Kind != agents.DiagnosticVersion || version.Status != agents.DiagnosticWarning {
		t.Fatalf("version check = %+v, want a warning", version)
	}
	for _, want := range []string{"2.4.0", "1.0.0", "2.0.0", "1.1.0"} {
		if !strings.Contains(version.Message, want) {
			t.Errorf("message %q must mention %q", version.Message, want)
		}
	}
}

// TestAdapterDiagnosticsUndeterminableVersion warns instead of failing when
// the version cannot be established.
func TestAdapterDiagnosticsUndeterminableVersion(t *testing.T) {
	adapter := New(Options{
		Executable:   writeFakePi(t, t.TempDir(), "exit 0"),
		VersionProbe: func(context.Context) (string, error) { return "pi version mystery", nil },
	})

	report := adapter.Diagnostics(context.Background())

	if report.Status != agents.DiagnosticWarning {
		t.Fatalf("status = %q, want warning", report.Status)
	}
	if report.Checks[1].Kind != agents.DiagnosticVersion {
		t.Errorf("check kind = %q, want version", report.Checks[1].Kind)
	}
}

// TestAdapterDiagnosticsRedactsUnparseableVersionOutput proves the
// unparseable-version branch never leaks raw CLI output that happens to
// contain a credential: the version probe's text is operator-visible, so it
// must pass through redaction like every other diagnostic.
func TestAdapterDiagnosticsRedactsUnparseableVersionOutput(t *testing.T) {
	adapter := New(Options{
		Executable:   writeFakePi(t, t.TempDir(), "exit 0"),
		VersionProbe: func(context.Context) (string, error) { return "api_key=supersecretvalue", nil },
	})

	report := adapter.Diagnostics(context.Background())

	if report.Status != agents.DiagnosticWarning {
		t.Fatalf("status = %q, want warning", report.Status)
	}
	message := report.Checks[1].Message
	if strings.Contains(message, "supersecretvalue") {
		t.Errorf("raw CLI output leaked into the unparseable-version diagnostic: %q", message)
	}
	if !strings.Contains(message, "[REDACTED]") {
		t.Errorf("raw CLI output was not redacted: %q", message)
	}
}

// TestAdapterDiagnosticsRunsRealVersionCommand exercises the default probe:
// the adapter runs `pi --version` and parses the output.
func TestAdapterDiagnosticsRunsRealVersionCommand(t *testing.T) {
	adapter := New(Options{Executable: writeFakePi(t, t.TempDir(), `echo "1.1.0"`)})

	report := adapter.Diagnostics(context.Background())

	if report.Version != "1.1.0" {
		t.Fatalf("version = %q, want 1.1.0 parsed from the command output", report.Version)
	}
	if report.Checks[1].Status != agents.DiagnosticOK {
		t.Fatalf("version check = %+v, want ok", report.Checks[1])
	}
}
