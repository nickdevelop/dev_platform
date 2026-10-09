// This file defines the platform's agent diagnostics vocabulary: what a
// pre-flight check reports, how CLI versions are classified against a
// backend's tested range, and how the registry is walked so every registered
// backend gets a report — including a backend whose adapter is broken.
//
// The types here are JSON-stable and shared by the adapters (which run the
// actual checks), the /api/agent-backends/diagnostics endpoint and the
// workbench UI. Like the rest of the package, nothing here talks to an agent
// process or a database: the Collector only calls adapter methods and never
// lets one backend's failure prevent another's report.
package agents

import (
	"context"
	"fmt"
)

// DiagnosticKind classifies what a diagnostic inspected.
type DiagnosticKind string

const (
	// DiagnosticExecutable reports whether the backend's binary is present
	// and usable.
	DiagnosticExecutable DiagnosticKind = "executable"
	// DiagnosticVersion reports the backend CLI version (the CLI binary also
	// serves a server backend's API) against the tested range.
	DiagnosticVersion DiagnosticKind = "version"
	// DiagnosticAPI reports the reachability of a server-based backend's API.
	// CLI backends have no API and simply omit this check.
	DiagnosticAPI DiagnosticKind = "api"
	// DiagnosticEventStream reports event-stream health: a backend that sends
	// payloads the adapter cannot decode is diagnosed at runtime through
	// error events carrying this kind's sibling code (malformed_event), not
	// by pretending the stream is healthy.
	DiagnosticEventStream DiagnosticKind = "event_stream"
	// DiagnosticAdapter reports that the adapter itself failed to produce
	// diagnostics (for example it crashed); the platform never hides a
	// broken backend behind an empty report.
	DiagnosticAdapter DiagnosticKind = "adapter"
)

// DiagnosticStatus is the severity of one diagnostic result. Warnings are
// used whenever the backend may still work (an unverified or unsupported
// version); errors mean the backend cannot currently be used at all.
type DiagnosticStatus string

const (
	// DiagnosticOK means the check passed.
	DiagnosticOK DiagnosticStatus = "ok"
	// DiagnosticWarning means the check found a risk the operator should see,
	// but the backend may still work.
	DiagnosticWarning DiagnosticStatus = "warning"
	// DiagnosticError means the backend is unusable right now.
	DiagnosticError DiagnosticStatus = "error"
	// DiagnosticUnknown means the backend produced no checks at all.
	DiagnosticUnknown DiagnosticStatus = "unknown"
)

// Diagnostic is the result of one check. Message is written for the operator:
// it names what failed and how to fix it, and it is already redacted —
// adapters must never include credentials in diagnostics.
type Diagnostic struct {
	// Kind names what was inspected.
	Kind DiagnosticKind `json:"kind"`
	// Status is the severity of the result.
	Status DiagnosticStatus `json:"status"`
	// Message is the actionable, redacted summary shown in the UI.
	Message string `json:"message"`
	// Detail optionally carries the raw (bounded) check output.
	Detail string `json:"detail,omitempty"`
}

// BackendDiagnostics is the startup diagnostics report for one backend: the
// overall status (the worst check), the detected and tested versions, the
// individual checks, and the backend's capability declaration so the UI can
// render the compatibility matrix next to the live status.
type BackendDiagnostics struct {
	// Backend names the agent backend this report describes.
	Backend BackendID `json:"backend"`
	// Status is the overall status: the worst of the checks, or unknown when
	// the adapter produced no checks.
	Status DiagnosticStatus `json:"status"`
	// Version is the CLI version the probe detected, when parseable.
	Version string `json:"version,omitempty"`
	// TestedVersion is the version the adapter was verified against.
	TestedVersion string `json:"testedVersion,omitempty"`
	// Checks are the individual results, in a stable order.
	Checks []Diagnostic `json:"checks"`
	// Capabilities repeats the backend's declaration; nil when the adapter
	// does not publish one.
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

// AggregateStatus folds check results into the overall status: the worst
// observed severity (error over warning over ok), unknown when there are no
// checks.
func AggregateStatus(checks []Diagnostic) DiagnosticStatus {
	status := DiagnosticUnknown
	for _, check := range checks {
		switch check.Status {
		case DiagnosticError:
			return DiagnosticError
		case DiagnosticWarning:
			status = DiagnosticWarning
		case DiagnosticOK:
			if status != DiagnosticWarning {
				status = DiagnosticOK
			}
		}
	}
	return status
}

// SupportedVersion declares the CLI version range one backend is tested
// against. Minimum is inclusive, Maximum exclusive: [2.0.0, 3.0.0) accepts
// 2.x. The values are pinned by tests and must match
// docs/agent-compatibility.md.
type SupportedVersion struct {
	// Tested is the exact version the adapter was verified against.
	Tested string
	// Minimum is the inclusive lower bound of the tested range.
	Minimum string
	// Maximum is the exclusive upper bound of the tested range.
	Maximum string
}

// supportedVersions is the compatibility matrix's version axis, declared once
// so adapters, docs and tests cannot drift apart.
var supportedVersions = map[BackendID]SupportedVersion{
	BackendOpenCodeV2: {Tested: "2.0.24", Minimum: "2.0.0", Maximum: "3.0.0"},
	BackendPi:         {Tested: "1.1.0", Minimum: "1.0.0", Maximum: "2.0.0"},
}

// SupportedVersionFor returns the tested version range declared for id.
func SupportedVersionFor(id BackendID) (SupportedVersion, bool) {
	supported, ok := supportedVersions[id]
	return supported, ok
}

// Version is a parsed MAJOR.MINOR.PATCH version. Pre-release suffixes
// ("1.1.0-mock") and build metadata are ignored for comparison, so a packaged
// backport still classifies by its numbers.
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion extracts the first MAJOR.MINOR.PATCH triple from tool output
// such as "opencode v2.0.24" or "1.1.0-mock". It reports false when the text
// contains no such triple; it never fails on extra text around it.
func ParseVersion(text string) (Version, bool) {
	for i := 0; i < len(text); i++ {
		if !isDigit(text[i]) {
			continue
		}
		// A digit that continues a previous number is not a new start.
		if i > 0 && isDigit(text[i-1]) {
			continue
		}
		if version, ok := parseVersionAt(text, i); ok {
			return version, true
		}
	}
	return Version{}, false
}

// parseVersionAt parses exactly three dot-separated numbers starting at the
// digit at index start.
func parseVersionAt(text string, start int) (Version, bool) {
	var parts [3]int
	pos := start
	for field := 0; field < 3; field++ {
		value, digits := 0, 0
		for pos < len(text) && isDigit(text[pos]) {
			value = value*10 + int(text[pos]-'0')
			if value < 0 {
				return Version{}, false // overflow guard
			}
			pos++
			digits++
		}
		if digits == 0 {
			return Version{}, false
		}
		parts[field] = value
		if field < 2 {
			if pos >= len(text) || text[pos] != '.' {
				return Version{}, false
			}
			pos++
		}
	}
	return Version{Major: parts[0], Minor: parts[1], Patch: parts[2]}, true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// String renders the version in MAJOR.MINOR.PATCH form.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less reports whether v sorts strictly before other.
func (v Version) Less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// InRange reports whether v lies inside the supported range: at or above
// Minimum and strictly below Maximum.
func (v Version) InRange(supported SupportedVersion) bool {
	minimum, okMin := ParseVersion(supported.Minimum)
	maximum, okMax := ParseVersion(supported.Maximum)
	if !okMin || !okMax {
		return false
	}
	if v.Less(minimum) {
		return false
	}
	// Maximum is exclusive: v == maximum is outside the range.
	return v.Less(maximum)
}

// Diagnostician is the optional adapter extension for startup diagnostics.
// The frozen AgentAdapter interface stays unchanged; adapters that can report
// pre-flight checks implement this extra method and the platform asserts it,
// exactly like the capability declaration.
type Diagnostician interface {
	AgentAdapter
	// Diagnostics runs the backend's pre-flight checks — executable presence,
	// CLI/API version support, API reachability — and returns the report. It
	// must never start an instance, must honor ctx, and must keep every
	// message actionable and free of credentials.
	Diagnostics(ctx context.Context) BackendDiagnostics
}

// capabilitySource is the optional adapter method publishing a capability
// declaration; the same shape the sessions layer asserts.
type capabilitySource interface {
	Capabilities() Capabilities
}

// CollectDiagnostics runs the diagnostics of every registered backend and
// returns one report per backend in the registry's (sorted) order. A backend
// that fails to produce a report — including a panicking adapter — is
// returned as an error report so the failure stays visible, and never
// prevents the other backends' reports.
func CollectDiagnostics(ctx context.Context, registry *Registry) []BackendDiagnostics {
	if registry == nil {
		return nil
	}
	ids := registry.Backends()
	reports := make([]BackendDiagnostics, 0, len(ids))
	for _, id := range ids {
		adapter, ok := registry.Get(id)
		if !ok {
			continue
		}
		reports = append(reports, collectBackendDiagnostics(ctx, id, adapter))
	}
	return reports
}

// collectBackendDiagnostics gathers one backend's report, recovering from an
// adapter crash so the registry walk always completes.
func collectBackendDiagnostics(ctx context.Context, id BackendID, adapter AgentAdapter) (report BackendDiagnostics) {
	defer func() {
		if recovered := recover(); recovered != nil {
			report = BackendDiagnostics{
				Backend: id,
				Status:  DiagnosticError,
				Checks: []Diagnostic{{
					Kind:    DiagnosticAdapter,
					Status:  DiagnosticError,
					Message: fmt.Sprintf("%s adapter crashed while collecting diagnostics: %v; the other backends are unaffected", id, recovered),
				}},
			}
		}
	}()

	if diagnostician, ok := adapter.(Diagnostician); ok {
		report = diagnostician.Diagnostics(ctx)
		if report.Backend == "" {
			report.Backend = id
		}
		if report.Status == "" {
			report.Status = AggregateStatus(report.Checks)
		}
		if report.Capabilities == nil {
			if source, ok := adapter.(capabilitySource); ok {
				capabilities := source.Capabilities()
				report.Capabilities = &capabilities
			}
		}
		return report
	}

	// An adapter without diagnostics support is reported honestly as unknown
	// instead of being silently dropped.
	report = BackendDiagnostics{Backend: id, Status: DiagnosticUnknown}
	if source, ok := adapter.(capabilitySource); ok {
		capabilities := source.Capabilities()
		report.Capabilities = &capabilities
	}
	return report
}
