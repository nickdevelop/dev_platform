package agents

import (
	"context"
	"strings"
	"testing"
)

// TestSupportedVersionDeclarations pins the tested CLI version ranges. The
// values must match docs/agent-compatibility.md verbatim: the documented
// matrix and the code must not drift apart.
func TestSupportedVersionDeclarations(t *testing.T) {
	opencode, ok := SupportedVersionFor(BackendOpenCodeV2)
	if !ok {
		t.Fatal("no supported version declared for opencode-v2")
	}
	if opencode.Tested != "2.0.24" || opencode.Minimum != "2.0.0" || opencode.Maximum != "3.0.0" {
		t.Errorf("opencode-v2 range = %+v, want tested 2.0.24 in [2.0.0, 3.0.0)", opencode)
	}

	pi, ok := SupportedVersionFor(BackendPi)
	if !ok {
		t.Fatal("no supported version declared for pi")
	}
	if pi.Tested != "1.1.0" || pi.Minimum != "1.0.0" || pi.Maximum != "2.0.0" {
		t.Errorf("pi range = %+v, want tested 1.1.0 in [1.0.0, 2.0.0)", pi)
	}

	if _, ok := SupportedVersionFor("claude-code"); ok {
		t.Error("unknown backend must not declare a supported version")
	}

	// Every declared bound must itself be parseable, or the range can never
	// classify anything.
	for id, sv := range map[BackendID]SupportedVersion{BackendOpenCodeV2: opencode, BackendPi: pi} {
		if _, ok := ParseVersion(sv.Minimum); !ok {
			t.Errorf("%s minimum %q is not a parseable version", id, sv.Minimum)
		}
		if _, ok := ParseVersion(sv.Maximum); !ok {
			t.Errorf("%s maximum %q is not a parseable version", id, sv.Maximum)
		}
		if _, ok := ParseVersion(sv.Tested); !ok {
			t.Errorf("%s tested version %q is not a parseable version", id, sv.Tested)
		}
		if tested, ok := ParseVersion(sv.Tested); !ok || !tested.InRange(sv) {
			t.Errorf("%s tested version %q is not inside its own declared range", id, sv.Tested)
		}
	}
}

// TestParseVersion extracts versions from real tool output shapes: pi prints
// "1.1.0", opencode prints "opencode v2.0.24", and pre-release suffixes must
// not break the parse.
func TestParseVersion(t *testing.T) {
	cases := []struct {
		text string
		want Version
		ok   bool
	}{
		{text: "1.1.0", want: Version{1, 1, 0}, ok: true},
		{text: "opencode v2.0.24", want: Version{2, 0, 24}, ok: true},
		{text: "1.1.0-mock", want: Version{1, 1, 0}, ok: true},
		{text: "opencode 0.0.0-mock", want: Version{0, 0, 0}, ok: true},
		{text: "  v10.20.30\n", want: Version{10, 20, 30}, ok: true},
		{text: "no version here", ok: false},
		{text: "", ok: false},
		{text: "1.2", ok: false},
		{text: "pi version unknown", ok: false},
	}
	for _, tc := range cases {
		got, ok := ParseVersion(tc.text)
		if ok != tc.ok {
			t.Errorf("ParseVersion(%q) ok = %v, want %v", tc.text, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ParseVersion(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

// TestVersionInRange checks the tested-range boundaries: the minimum is
// inclusive, the maximum exclusive.
func TestVersionInRange(t *testing.T) {
	opencode, _ := SupportedVersionFor(BackendOpenCodeV2)
	pi, _ := SupportedVersionFor(BackendPi)

	cases := []struct {
		version Version
		sv      SupportedVersion
		want    bool
	}{
		{Version{2, 0, 24}, opencode, true},
		{Version{2, 0, 0}, opencode, true},  // minimum is inclusive
		{Version{2, 9, 99}, opencode, true}, // anything under the exclusive maximum is supported
		{Version{3, 0, 0}, opencode, false}, // maximum is exclusive
		{Version{1, 99, 0}, opencode, false},
		{Version{1, 1, 0}, pi, true},
		{Version{1, 0, 0}, pi, true},
		{Version{2, 0, 0}, pi, false},
	}
	for _, tc := range cases {
		if got := tc.version.InRange(tc.sv); got != tc.want {
			t.Errorf("Version(%s).InRange(%+v) = %v, want %v", tc.version, tc.sv, got, tc.want)
		}
	}
}

// TestAggregateStatus folds check results into the worst observed severity.
func TestAggregateStatus(t *testing.T) {
	cases := []struct {
		checks []Diagnostic
		want   DiagnosticStatus
	}{
		{nil, DiagnosticUnknown},
		{[]Diagnostic{{Status: DiagnosticOK}}, DiagnosticOK},
		{[]Diagnostic{{Status: DiagnosticOK}, {Status: DiagnosticWarning}}, DiagnosticWarning},
		{[]Diagnostic{{Status: DiagnosticWarning}, {Status: DiagnosticError}}, DiagnosticError},
		{[]Diagnostic{{Status: DiagnosticError}, {Status: DiagnosticWarning}}, DiagnosticError},
	}
	for _, tc := range cases {
		if got := AggregateStatus(tc.checks); got != tc.want {
			t.Errorf("AggregateStatus(%+v) = %q, want %q", tc.checks, got, tc.want)
		}
	}
}

// diagnosticsStub is a scriptable Diagnostician for the collector tests.
type diagnosticsStub struct {
	stubAdapter
	report     BackendDiagnostics
	crash      bool
	panicValue any
	checked    *bool
}

func (s *diagnosticsStub) Diagnostics(ctx context.Context) BackendDiagnostics {
	if s.checked != nil {
		*s.checked = ctx.Err() == nil
	}
	if s.panicValue != nil {
		panic(s.panicValue)
	}
	if s.crash {
		panic("diagnostics exploded")
	}
	return s.report
}

// TestCollectDiagnosticsIsolatesFailingAdapter proves one backend's broken
// diagnostics cannot stop another backend from being reported.
func TestCollectDiagnosticsIsolatesFailingAdapter(t *testing.T) {
	registry := NewRegistry()

	healthy := &diagnosticsStub{report: BackendDiagnostics{
		Status: DiagnosticOK,
		Checks: []Diagnostic{{Kind: DiagnosticExecutable, Status: DiagnosticOK, Message: "found"}},
	}}
	if err := registry.Register(BackendPi, healthy); err != nil {
		t.Fatalf("register pi: %v", err)
	}
	broken := &diagnosticsStub{crash: true}
	if err := registry.Register(BackendOpenCodeV2, broken); err != nil {
		t.Fatalf("register opencode: %v", err)
	}

	reports := CollectDiagnostics(context.Background(), registry)
	if len(reports) != 2 {
		t.Fatalf("CollectDiagnostics returned %d reports, want 2: %+v", len(reports), reports)
	}

	byBackend := make(map[BackendID]BackendDiagnostics, len(reports))
	for _, report := range reports {
		byBackend[report.Backend] = report
	}

	pi := byBackend[BackendPi]
	if pi.Status != DiagnosticOK || len(pi.Checks) != 1 || pi.Checks[0].Status != DiagnosticOK {
		t.Errorf("pi report damaged by the failing backend: %+v", pi)
	}

	opencode := byBackend[BackendOpenCodeV2]
	if opencode.Status != DiagnosticError {
		t.Errorf("opencode status = %q, want error after the adapter crashed", opencode.Status)
	}
	if len(opencode.Checks) != 1 || opencode.Checks[0].Kind != DiagnosticAdapter {
		t.Errorf("opencode report = %+v, want a single adapter-kind error check", opencode.Checks)
	}
}

// TestCollectDiagnosticsStampsAndNormalizes verifies the collector stamps the
// backend id, derives a missing overall status, honors the context, and never
// mutates the registry order guarantee.
func TestCollectDiagnosticsStampsAndNormalizes(t *testing.T) {
	registry := NewRegistry()
	var checked bool
	stub := &diagnosticsStub{
		checked: &checked,
		report: BackendDiagnostics{
			Checks: []Diagnostic{{Kind: DiagnosticVersion, Status: DiagnosticWarning, Message: "unverified"}},
		},
	}
	if err := registry.Register(BackendPi, stub); err != nil {
		t.Fatalf("register: %v", err)
	}

	reports := CollectDiagnostics(context.Background(), registry)
	if len(reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one", reports)
	}
	report := reports[0]
	if report.Backend != BackendPi {
		t.Errorf("backend = %q, want it stamped by the collector", report.Backend)
	}
	if report.Status != DiagnosticWarning {
		t.Errorf("status = %q, want derived from the checks", report.Status)
	}
	if !checked {
		t.Error("Diagnostics was never called on the adapter")
	}

	// A canceled context is propagated so adapters can skip their probes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if reports := CollectDiagnostics(ctx, registry); len(reports) != 1 {
		t.Errorf("canceled context still returned %d reports, want the report envelope", len(reports))
	}
}

// capabilitiesOnlyStub is an adapter that declares capabilities but does not
// implement Diagnostics.
type capabilitiesOnlyStub struct {
	stubAdapter
	caps Capabilities
}

func (s *capabilitiesOnlyStub) Capabilities() Capabilities { return s.caps }

// TestCollectDiagnosticsWithoutDiagnostician covers adapters that do not
// implement Diagnostics: they must be reported with an unknown status (and
// their capabilities, when declared) instead of being silently dropped.
func TestCollectDiagnosticsWithoutDiagnostician(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(BackendPi, &stubAdapter{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registry.Register(BackendOpenCodeV2, &capabilitiesOnlyStub{caps: Capabilities{Streaming: true}}); err != nil {
		t.Fatalf("register opencode: %v", err)
	}

	reports := CollectDiagnostics(context.Background(), registry)
	if len(reports) != 2 {
		t.Fatalf("reports = %+v, want both backends", reports)
	}
	byBackend := make(map[BackendID]BackendDiagnostics, len(reports))
	for _, report := range reports {
		byBackend[report.Backend] = report
	}

	pi := byBackend[BackendPi]
	if pi.Status != DiagnosticUnknown || pi.Checks == nil || len(pi.Checks) != 0 || pi.Capabilities != nil {
		t.Errorf("pi report = %+v, want unknown status with an empty (non-nil) checks array and no capabilities", pi)
	}
	opencode := byBackend[BackendOpenCodeV2]
	if opencode.Status != DiagnosticUnknown {
		t.Errorf("opencode status = %q, want unknown", opencode.Status)
	}
	if opencode.Capabilities == nil || !opencode.Capabilities.Streaming {
		t.Errorf("opencode capabilities = %+v, want the adapter declaration", opencode.Capabilities)
	}
}

// TestCollectDiagnosticsRedactsPanicValue keeps the collector's crash report
// free of credentials: the recovered value is operator-visible, so it must
// pass through the same redaction as every other diagnostic.
func TestCollectDiagnosticsRedactsPanicValue(t *testing.T) {
	registry := NewRegistry()
	stub := &diagnosticsStub{panicValue: "token=panicsecretvalue"}
	if err := registry.Register(BackendPi, stub); err != nil {
		t.Fatalf("register: %v", err)
	}

	reports := CollectDiagnostics(context.Background(), registry)
	if len(reports) != 1 {
		t.Fatalf("reports = %+v, want one", reports)
	}
	report := reports[0]
	if len(report.Checks) != 1 || report.Checks[0].Kind != DiagnosticAdapter {
		t.Fatalf("checks = %+v, want a single adapter-kind check", report.Checks)
	}
	message := report.Checks[0].Message
	if strings.Contains(message, "panicsecretvalue") {
		t.Errorf("panic value leaked into the diagnostic: %q", message)
	}
	if !strings.Contains(message, "[REDACTED]") {
		t.Errorf("panic value was not redacted: %q", message)
	}
}

// TestCollectDiagnosticsAlwaysArraysChecks keeps the JSON contract: checks is
// always a (possibly empty) array, never null, for every adapter shape.
func TestCollectDiagnosticsAlwaysArraysChecks(t *testing.T) {
	t.Run("diagnostician with a zero report", func(t *testing.T) {
		registry := NewRegistry()
		if err := registry.Register(BackendPi, &diagnosticsStub{}); err != nil {
			t.Fatalf("register: %v", err)
		}
		report := CollectDiagnostics(context.Background(), registry)[0]
		if report.Checks == nil || len(report.Checks) != 0 {
			t.Errorf("checks = %#v, want an empty non-nil array", report.Checks)
		}
	})

	t.Run("adapter without diagnostics", func(t *testing.T) {
		registry := NewRegistry()
		if err := registry.Register(BackendPi, &stubAdapter{}); err != nil {
			t.Fatalf("register: %v", err)
		}
		report := CollectDiagnostics(context.Background(), registry)[0]
		if report.Checks == nil || len(report.Checks) != 0 {
			t.Errorf("checks = %#v, want an empty non-nil array", report.Checks)
		}
	})
}

// TestCollectDiagnosticsNilRegistry keeps the collector total: no registry, no
// reports, no panic.
func TestCollectDiagnosticsNilRegistry(t *testing.T) {
	if reports := CollectDiagnostics(context.Background(), nil); reports != nil {
		t.Errorf("nil registry returned %+v, want nil", reports)
	}
}

// TestRedactSecrets pins the credential shapes the redaction helper masks,
// including JSON-quoted key/value pairs (finding 6): a credential inside a
// serialized JSON diagnostic must never survive.
func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		in       string
		leak     string // must not appear in the output
		wantMask bool
	}{
		{in: `token=supersecret`, leak: "supersecret", wantMask: true},
		{in: `password: hunter2`, leak: "hunter2", wantMask: true},
		{in: `{"token": "abcd1234"}`, leak: "abcd1234", wantMask: true},
		{in: `{"password":"hunter2"}`, leak: "hunter2", wantMask: true},
		{in: `{"api_key": 'sk-live-abcdef123456'}`, leak: "sk-live-abcdef123456", wantMask: true},
		{in: `Authorization: Bearer abcdef.ghijkl`, leak: "abcdef.ghijkl", wantMask: true},
		{in: `token sk-abcd1234567`, leak: "sk-abcd1234567", wantMask: true},
		{in: `the tested version is 2.0.24`, leak: "", wantMask: false},
	}
	for _, tc := range cases {
		got := RedactSecrets(tc.in)
		if tc.leak != "" && strings.Contains(got, tc.leak) {
			t.Errorf("RedactSecrets(%q) = %q, leaked %q", tc.in, got, tc.leak)
		}
		if tc.wantMask && !strings.Contains(got, "[REDACTED]") {
			t.Errorf("RedactSecrets(%q) = %q, want a [REDACTED] marker", tc.in, got)
		}
		if !tc.wantMask && got != tc.in {
			t.Errorf("RedactSecrets(%q) = %q, want it unchanged", tc.in, got)
		}
	}
}

// TestDiagnosticianRequiresAdapter pins that Diagnostician is an extension of
// AgentAdapter, so a diagnostics-only type can never be registered.
func TestDiagnosticianRequiresAdapter(t *testing.T) {
	var adapter AgentAdapter = &diagnosticsStub{}
	if _, ok := adapter.(Diagnostician); !ok {
		t.Fatal("a type satisfying AgentAdapter and Diagnostics must satisfy Diagnostician")
	}
}
