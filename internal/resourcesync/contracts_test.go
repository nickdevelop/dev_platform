package resourcesync

import (
	"errors"
	"testing"

	"devplatform/internal/agents"
	"devplatform/internal/resources"
)

func validTarget() ResourceTarget {
	return ResourceTarget{
		ProjectID: 7,
		BackendID: agents.BackendPi,
		Path:      ".pi/prompts/commit-style.md",
	}
}

func validConflict() SyncConflict {
	return SyncConflict{
		Kind:       ConflictUnmanagedTarget,
		ResourceID: 3,
		Target:     validTarget(),
		Detail:     "target already exists and is not platform-managed",
	}
}

func validPreview() SyncPreview {
	return SyncPreview{
		ResourceID:    3,
		Kind:          resources.ResourcePrompt,
		Target:        validTarget(),
		State:         SyncStateApplied,
		ChangeSummary: "write new managed block",
	}
}

func TestContractsSyncStates(t *testing.T) {
	want := map[SyncState]string{
		SyncStatePending:     "pending",
		SyncStateApplied:     "applied",
		SyncStateConflict:    "conflict",
		SyncStateFailed:      "failed",
		SyncStateUnsupported: "unsupported",
	}
	for state, wire := range want {
		if string(state) != wire {
			t.Errorf("state %v = %q, want %q", state, string(state), wire)
		}
		if !state.Valid() {
			t.Errorf("state %q Valid() = false, want true", state)
		}
	}
	for _, state := range []SyncState{"", "ok", "Applied", "paused"} {
		if state.Valid() {
			t.Errorf("state %q Valid() = true, want false", state)
		}
	}
}

func TestContractsConflictKinds(t *testing.T) {
	want := map[ConflictKind]string{
		ConflictUnmanagedTarget: "unmanaged-target",
		ConflictExternalDrift:   "external-drift",
		ConflictTargetInvalid:   "target-invalid",
	}
	for kind, wire := range want {
		if string(kind) != wire {
			t.Errorf("kind %v = %q, want %q", kind, string(kind), wire)
		}
		if !kind.Valid() {
			t.Errorf("kind %q Valid() = false, want true", kind)
		}
	}
	for _, kind := range []ConflictKind{"", "unmanaged", "drift"} {
		if kind.Valid() {
			t.Errorf("kind %q Valid() = true, want false", kind)
		}
	}
}

func TestContractsResourceTargetValidateAccepts(t *testing.T) {
	backendTarget := validTarget()

	projectPrompt := ResourceTarget{
		ProjectID: 7,
		Path:      "AGENTS.md",
	}

	for name, target := range map[string]ResourceTarget{
		"backend target": backendTarget,
		"project prompt": projectPrompt,
	} {
		if err := target.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", name, err)
		}
	}
}

func TestContractsResourceTargetValidateRejects(t *testing.T) {
	mutate := map[string]func(*ResourceTarget){
		"zero project":   func(t *ResourceTarget) { t.ProjectID = 0 },
		"empty path":     func(t *ResourceTarget) { t.Path = "" },
		"absolute path":  func(t *ResourceTarget) { t.Path = "/etc/passwd" },
		"parent escape":  func(t *ResourceTarget) { t.Path = "../outside.md" },
		"nested escape":  func(t *ResourceTarget) { t.Path = "a/../../b.md" },
		"backslash":      func(t *ResourceTarget) { t.Path = `a\b.md` },
		"nul byte":       func(t *ResourceTarget) { t.Path = "a\x00b.md" },
		"unclean dot":    func(t *ResourceTarget) { t.Path = "./a.md" },
		"trailing slash": func(t *ResourceTarget) { t.Path = "a/" },
		"unknown backend": func(t *ResourceTarget) {
			t.BackendID = "claude"
		},
	}

	for name, fn := range mutate {
		target := validTarget()
		fn(&target)
		err := target.Validate()
		if err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
			continue
		}
		if !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("%s: Validate() = %v, want ErrInvalidTarget", name, err)
		}
	}
}

func TestContractsSyncConflictValidate(t *testing.T) {
	for _, kind := range []ConflictKind{ConflictUnmanagedTarget, ConflictExternalDrift, ConflictTargetInvalid} {
		conflict := validConflict()
		conflict.Kind = kind
		if err := conflict.Validate(); err != nil {
			t.Errorf("kind %q: Validate() = %v, want nil", kind, err)
		}
	}

	mutate := map[string]func(*SyncConflict){
		"zero resource": func(c *SyncConflict) { c.ResourceID = 0 },
		"unknown kind":  func(c *SyncConflict) { c.Kind = "drift" },
		"bad target":    func(c *SyncConflict) { c.Target.ProjectID = 0 },
	}
	for name, fn := range mutate {
		conflict := validConflict()
		fn(&conflict)
		err := conflict.Validate()
		if err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
			continue
		}
		if !errors.Is(err, ErrInvalidConflict) {
			t.Errorf("%s: Validate() = %v, want ErrInvalidConflict", name, err)
		}
	}
}

func TestContractsSyncPreviewValidate(t *testing.T) {
	applied := validPreview()
	if err := applied.Validate(); err != nil {
		t.Errorf("applied preview: Validate() = %v, want nil", err)
	}

	conflicted := validPreview()
	conflicted.State = SyncStateConflict
	conflict := validConflict()
	conflicted.Conflict = &conflict
	if err := conflicted.Validate(); err != nil {
		t.Errorf("conflict preview: Validate() = %v, want nil", err)
	}

	unsupported := validPreview()
	unsupported.State = SyncStateUnsupported
	unsupported.ChangeSummary = ""
	if err := unsupported.Validate(); err != nil {
		t.Errorf("unsupported preview: Validate() = %v, want nil", err)
	}

	mutate := map[string]func(*SyncPreview){
		"zero resource":            func(p *SyncPreview) { p.ResourceID = 0 },
		"unknown kind":             func(p *SyncPreview) { p.Kind = "tool" },
		"unknown state":            func(p *SyncPreview) { p.State = "ok" },
		"bad target":               func(p *SyncPreview) { p.Target.Path = "../x" },
		"missing summary":          func(p *SyncPreview) { p.ChangeSummary = "  " },
		"conflict state no detail": func(p *SyncPreview) { p.State = SyncStateConflict },
		"stray conflict pointer": func(p *SyncPreview) {
			conflict := validConflict()
			p.Conflict = &conflict
		},
	}
	for name, fn := range mutate {
		preview := validPreview()
		fn(&preview)
		err := preview.Validate()
		if err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
			continue
		}
		if !errors.Is(err, ErrInvalidPreview) {
			t.Errorf("%s: Validate() = %v, want ErrInvalidPreview", name, err)
		}
	}
}

func TestContractsBackendSupportSupports(t *testing.T) {
	support := BackendSupport{
		BackendID: agents.BackendPi,
		Kinds:     []resources.ResourceKind{resources.ResourcePrompt, resources.ResourceMCP},
	}
	if !support.Supports(resources.ResourcePrompt) {
		t.Error("Supports(prompt) = false, want true")
	}
	if !support.Supports(resources.ResourceMCP) {
		t.Error("Supports(mcp) = false, want true")
	}
	if support.Supports(resources.ResourceSkill) {
		t.Error("Supports(skill) = true, want false")
	}

	empty := BackendSupport{BackendID: agents.BackendPi}
	if empty.Supports(resources.ResourcePrompt) {
		t.Error("empty support declared prompt, want false")
	}
}
