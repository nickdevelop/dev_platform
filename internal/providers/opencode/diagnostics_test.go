package opencode

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/core/agents"
)

// versionStub returns a VersionProbe seam returning the given output or error.
func versionStub(output string, err error) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return output, err }
}

// writeVersionScript writes an executable that prints output, for the subtests
// that exercise the real `--version` command path.
func writeVersionScript(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \""+output+"\"\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}

// TestAdapterDiagnosticsMissingExecutable reports an actionable error and no
// further checks: without the binary there is nothing left to probe.
func TestAdapterDiagnosticsMissingExecutable(t *testing.T) {
	adapter := New(Options{Executable: "definitely-not-a-real-opencode-binary-xyz"})

	report := adapter.Diagnostics(context.Background())

	if report.Backend != agents.BackendOpenCodeV2 {
		t.Fatalf("backend = %q, want opencode-v2", report.Backend)
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

// TestAdapterDiagnosticsHealthy pins the all-clear report: executable found,
// tested CLI version accepted, no running instances to probe.
func TestAdapterDiagnosticsHealthy(t *testing.T) {
	adapter := New(Options{
		Executable:   writeVersionScript(t, "unused"),
		VersionProbe: versionStub("opencode v2.0.24", nil),
	})

	report := adapter.Diagnostics(context.Background())

	if report.Status != agents.DiagnosticOK {
		t.Fatalf("status = %q, want ok (checks: %+v)", report.Status, report.Checks)
	}
	if report.Version != "2.0.24" {
		t.Errorf("version = %q, want the parsed CLI version", report.Version)
	}
	if report.TestedVersion != "2.0.24" {
		t.Errorf("tested version = %q, want the declared tested version", report.TestedVersion)
	}
	if len(report.Checks) != 3 {
		t.Fatalf("checks = %+v, want executable, version and api checks", report.Checks)
	}
	if report.Checks[0].Kind != agents.DiagnosticExecutable || report.Checks[0].Status != agents.DiagnosticOK {
		t.Errorf("executable check = %+v, want ok", report.Checks[0])
	}
	version := report.Checks[1]
	if version.Kind != agents.DiagnosticVersion || version.Status != agents.DiagnosticOK {
		t.Errorf("version check = %+v, want ok", version)
	}
	api := report.Checks[2]
	if api.Kind != agents.DiagnosticAPI || api.Status != agents.DiagnosticOK {
		t.Errorf("api check = %+v, want ok", api)
	}
	if report.Capabilities == nil || !report.Capabilities.ToolCallEvents || !report.Capabilities.ToolConfirmation {
		t.Errorf("capabilities = %+v, want the adapter declaration", report.Capabilities)
	}
}

// TestAdapterDiagnosticsUnsupportedVersion warns when the CLI version is
// outside the tested range, including the range in the message.
func TestAdapterDiagnosticsUnsupportedVersion(t *testing.T) {
	adapter := New(Options{
		Executable:   writeVersionScript(t, "unused"),
		VersionProbe: versionStub("opencode v9.9.9", nil),
	})

	report := adapter.Diagnostics(context.Background())

	if report.Status != agents.DiagnosticWarning {
		t.Fatalf("status = %q, want warning", report.Status)
	}
	if report.Version != "9.9.9" {
		t.Errorf("version = %q, want 9.9.9", report.Version)
	}
	version := report.Checks[1]
	if version.Kind != agents.DiagnosticVersion || version.Status != agents.DiagnosticWarning {
		t.Fatalf("version check = %+v, want a warning", version)
	}
	for _, want := range []string{"9.9.9", "2.0.0", "3.0.0", "2.0.24"} {
		if !strings.Contains(version.Message, want) {
			t.Errorf("message %q must mention %q", version.Message, want)
		}
	}
}

// TestAdapterDiagnosticsUndeterminableVersion warns instead of failing when
// the version cannot be established.
func TestAdapterDiagnosticsUndeterminableVersion(t *testing.T) {
	t.Run("unparseable output", func(t *testing.T) {
		adapter := New(Options{
			Executable:   writeVersionScript(t, "unused"),
			VersionProbe: versionStub("mystery build", nil),
		})
		report := adapter.Diagnostics(context.Background())
		if report.Status != agents.DiagnosticWarning || report.Checks[1].Kind != agents.DiagnosticVersion {
			t.Fatalf("report = %+v, want a version warning", report)
		}
	})

	t.Run("probe error", func(t *testing.T) {
		adapter := New(Options{
			Executable:   writeVersionScript(t, "unused"),
			VersionProbe: versionStub("", errors.New("exit status 1: boom")),
		})
		report := adapter.Diagnostics(context.Background())
		if report.Status != agents.DiagnosticWarning {
			t.Fatalf("status = %q, want warning", report.Status)
		}
		if !strings.Contains(report.Checks[1].Message, "exit status 1") {
			t.Errorf("message %q should carry the redacted probe error", report.Checks[1].Message)
		}
	})
}

// TestAdapterDiagnosticsRedactsUnparseableVersionOutput proves the
// unparseable-version branch never leaks raw CLI output that happens to
// contain a credential: the version probe errored in a way the parser could
// not classify, but its text is still operator-visible.
func TestAdapterDiagnosticsRedactsUnparseableVersionOutput(t *testing.T) {
	adapter := New(Options{
		Executable:   writeVersionScript(t, "unused"),
		VersionProbe: versionStub("token=supersecretvalue", nil),
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
// the adapter runs `<executable> --version` and parses the output.
func TestAdapterDiagnosticsRunsRealVersionCommand(t *testing.T) {
	adapter := New(Options{Executable: writeVersionScript(t, "opencode v2.0.24")})

	report := adapter.Diagnostics(context.Background())

	if report.Version != "2.0.24" {
		t.Fatalf("version = %q, want 2.0.24 parsed from the command output", report.Version)
	}
	if report.Checks[1].Status != agents.DiagnosticOK {
		t.Fatalf("version check = %+v, want ok", report.Checks[1])
	}
}

// TestAdapterDiagnosticsAPIProbesRunningInstances checks the API diagnostic
// against a live (test) server: reachable, version-mismatched and disconnected
// servers are each classified correctly.
func TestAdapterDiagnosticsAPIProbesRunningInstances(t *testing.T) {
	t.Run("reachable api", func(t *testing.T) {
		adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
			"GET /api/info": func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"version":"2.0.24","pid":7}`)
			},
		}))
		adapter.versionProbe = versionStub("opencode v2.0.24", nil)
		startTestInstance(t, adapter)

		report := adapter.Diagnostics(context.Background())

		api := report.Checks[2]
		if api.Kind != agents.DiagnosticAPI || api.Status != agents.DiagnosticOK {
			t.Fatalf("api check = %+v, want ok", api)
		}
		if !strings.Contains(api.Message, "1 running") {
			t.Errorf("message %q should count the probed instance", api.Message)
		}
	})

	t.Run("api version outside the tested range", func(t *testing.T) {
		adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
			"GET /api/info": func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"version":"0.0.0-mock","pid":7}`)
			},
		}))
		adapter.versionProbe = versionStub("opencode v2.0.24", nil)
		startTestInstance(t, adapter)

		report := adapter.Diagnostics(context.Background())

		api := report.Checks[2]
		if api.Status != agents.DiagnosticWarning {
			t.Fatalf("api check = %+v, want a warning for the unsupported server version", api)
		}
		if !strings.Contains(api.Message, "0.0.0") {
			t.Errorf("message %q should name the offending server version", api.Message)
		}
	})

	t.Run("disconnected api", func(t *testing.T) {
		adapter, _, server := newAdapterWithServer(t, infoHandler("2.0.24"))
		startTestInstance(t, adapter)
		server.Close() // the service the instance adopted is now unreachable

		report := adapter.Diagnostics(context.Background())

		if report.Status != agents.DiagnosticError {
			t.Fatalf("status = %q, want error for a disconnected server (%+v)", report.Status, report.Checks)
		}
		api := report.Checks[2]
		if api.Kind != agents.DiagnosticAPI || api.Status != agents.DiagnosticError {
			t.Fatalf("api check = %+v, want an error", api)
		}
		if !strings.Contains(api.Message, "inst-1") {
			t.Errorf("message %q must name the instance", api.Message)
		}
		// The platform never restarts the service itself, so the only remedy the
		// message may suggest is the one place an operator can act.
		if !strings.Contains(api.Message, "平台设置") || !strings.Contains(api.Message, "检测并启动") {
			t.Errorf("message %q must point at platform settings → Agent", api.Message)
		}
	})

	t.Run("unparseable api version is a warning, not silently ok", func(t *testing.T) {
		adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
			"GET /api/info": func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"version":"build-unknown","pid":7}`)
			},
		}))
		adapter.versionProbe = versionStub("opencode v2.0.24", nil)
		startTestInstance(t, adapter)

		report := adapter.Diagnostics(context.Background())

		api := report.Checks[2]
		if api.Kind != agents.DiagnosticAPI || api.Status != agents.DiagnosticWarning {
			t.Fatalf("api check = %+v, want a warning for an unparseable server version", api)
		}
		if !strings.Contains(api.Message, "unparseable") {
			t.Errorf("message %q should call out the unparseable version", api.Message)
		}
	})
}
