package settings

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"devplatform/internal/agents"
)

func TestAgentDefaultsAreEnabled(t *testing.T) {
	svc := NewService(openTestDB(t))

	got, err := svc.Agents()
	if err != nil {
		t.Fatalf("Agents: %v", err)
	}
	for _, backend := range []agents.BackendID{agents.BackendPi, agents.BackendOpenCodeV2} {
		setting := got[backend]
		if !setting.Enabled || setting.ManualPath != "" {
			t.Errorf("%s default = %+v, want enabled with no manual path", backend, setting)
		}
	}
}

func TestSetAgentEnabledRoundTrips(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	if _, err := svc.SetAgentEnabled(agents.BackendPi, false); err != nil {
		t.Fatalf("SetAgentEnabled: %v", err)
	}
	if NewService(db).Enabled(agents.BackendPi) {
		t.Error("pi should be disabled after the write")
	}
	if !NewService(db).Enabled(agents.BackendOpenCodeV2) {
		t.Error("opencode should remain enabled")
	}
}

func TestResolvePathPrefersManualThenLookPath(t *testing.T) {
	dir := t.TempDir()
	piPath := filepath.Join(dir, "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake pi: %v", err)
	}
	t.Setenv("PATH", dir)

	svc := NewService(openTestDB(t))

	path, source := svc.ResolvePath(agents.BackendPi)
	if path != piPath || source != "auto" {
		t.Errorf("ResolvePath = %q, %q; want %q, auto", path, source, piPath)
	}

	manual := filepath.Join(dir, "custom-pi")
	if _, err := svc.SetAgentPath(agents.BackendPi, manual); err != nil {
		t.Fatalf("SetAgentPath: %v", err)
	}
	path, source = svc.ResolvePath(agents.BackendPi)
	if path != manual || source != "manual" {
		t.Errorf("ResolvePath with manual = %q, %q; want %q, manual", path, source, manual)
	}
	if svc.Executable(agents.BackendPi) != manual {
		t.Errorf("Executable = %q, want %q", svc.Executable(agents.BackendPi), manual)
	}

	// Nothing on PATH and no manual path.
	path, source = svc.ResolvePath(agents.BackendOpenCodeV2)
	if path != "" || source != "none" {
		t.Errorf("ResolvePath(opencode) = %q, %q; want empty, none", path, source)
	}
}

func TestRecordDetectionStoresVersion(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	fixed := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	setting, err := svc.RecordDetection(agents.BackendPi, "1.1.0")
	if err != nil {
		t.Fatalf("RecordDetection: %v", err)
	}
	if setting.Version != "1.1.0" || setting.CheckedAt != fixed.Format(time.RFC3339) {
		t.Errorf("setting = %+v, want version 1.1.0 at %s", setting, fixed.Format(time.RFC3339))
	}

	reloaded, err := NewService(db).Agents()
	if err != nil {
		t.Fatalf("reload Agents: %v", err)
	}
	if reloaded[agents.BackendPi].Version != "1.1.0" {
		t.Errorf("persisted version = %q, want 1.1.0", reloaded[agents.BackendPi].Version)
	}
}
