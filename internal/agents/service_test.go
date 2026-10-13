package agents

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// TestServiceStatusJSONShape pins the wire contract of the machine-level
// service status: the platform settings page and docs/api/openapi.yaml both
// read these keys, so a rename is an API break and must fail here.
func TestServiceStatusJSONShape(t *testing.T) {
	status := ServiceStatus{
		Backend:    BackendOpenCodeV2,
		Supported:  true,
		State:      ServiceRunning,
		Endpoint:   "http://127.0.0.1:4096",
		PID:        4242,
		Version:    "v2.0.24",
		Executable: "/usr/local/bin/opencode",
		Detail:     "",
		CheckedAt:  time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{
		"backend", "supported", "state", "endpoint", "pid",
		"version", "executable", "detail", "checkedAt",
	}
	if len(fields) != len(want) {
		t.Fatalf("key count = %d (%v), want %d %v", len(fields), keysOf(fields), len(want), want)
	}
	for _, key := range want {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing key %q in %s", key, raw)
		}
	}
	if fields["state"] != "running" {
		t.Errorf("state = %v, want %q", fields["state"], "running")
	}
}

// TestServiceSentinelsDistinct guards against two service errors aliasing each
// other: the HTTP layer maps them to different codes and statuses.
func TestServiceSentinelsDistinct(t *testing.T) {
	pairs := [][2]error{
		{ErrServiceNotRunning, ErrServiceAlreadyRunning},
		{ErrServiceNotRunning, ErrServiceUnsupported},
		{ErrServiceAlreadyRunning, ErrServiceUnsupported},
	}
	for _, pair := range pairs {
		if errors.Is(pair[0], pair[1]) || errors.Is(pair[1], pair[0]) {
			t.Errorf("service sentinels alias each other: %v / %v", pair[0], pair[1])
		}
	}
}

func keysOf(fields map[string]any) []string {
	out := make([]string, 0, len(fields))
	for key := range fields {
		out = append(out, key)
	}
	return out
}
