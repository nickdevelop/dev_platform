package resources

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// validDefinition returns a ResourceDefinition that passes validation, for
// tests that start from a known-good record and mutate one field at a time.
func validDefinition() ResourceDefinition {
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	return ResourceDefinition{
		ID:          1,
		Kind:        ResourceSkill,
		Name:        "commit-style",
		Description: "how the team writes commits",
		Content:     "Use the imperative mood in the subject line.",
		Enabled:     true,
		Backends:    []agents.BackendID{agents.BackendPi},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestContractsResourceKinds(t *testing.T) {
	want := map[ResourceKind]string{
		ResourceSkill:  "skill",
		ResourceMCP:    "mcp",
		ResourcePrompt: "prompt",
	}
	for kind, wire := range want {
		if string(kind) != wire {
			t.Errorf("kind %v = %q, want %q", kind, string(kind), wire)
		}
		if !kind.Valid() {
			t.Errorf("kind %q Valid() = false, want true", kind)
		}
	}
	for _, kind := range []ResourceKind{"", "Skills", "tool", "prompts"} {
		if kind.Valid() {
			t.Errorf("kind %q Valid() = true, want false", kind)
		}
	}
}

func TestContractsPromptScopes(t *testing.T) {
	if PromptScopeProject != "project" || PromptScopeSession != "session" {
		t.Fatalf("prompt scopes = %q/%q, want project/session", PromptScopeProject, PromptScopeSession)
	}
	for _, scope := range []PromptScope{PromptScopeProject, PromptScopeSession} {
		if !scope.Valid() {
			t.Errorf("scope %q Valid() = false, want true", scope)
		}
	}
	for _, scope := range []PromptScope{"", "global", "Project"} {
		if scope.Valid() {
			t.Errorf("scope %q Valid() = true, want false", scope)
		}
	}
}

func TestContractsResourceDefinitionValidateAccepts(t *testing.T) {
	skill := validDefinition()

	mcp := validDefinition()
	mcp.Kind = ResourceMCP
	mcp.Name = "filesystem-server"
	mcp.SecretRef = "sec_opaque_1"

	publicMCP := validDefinition()
	publicMCP.Kind = ResourceMCP
	publicMCP.Name = "public-server"
	publicMCP.SecretRef = "" // a server that needs no credential is valid

	projectPrompt := validDefinition()
	projectPrompt.Kind = ResourcePrompt
	projectPrompt.Name = "house-rules"
	projectPrompt.PromptScope = PromptScopeProject

	sessionPrompt := validDefinition()
	sessionPrompt.Kind = ResourcePrompt
	sessionPrompt.Name = "review-checklist"
	sessionPrompt.PromptScope = PromptScopeSession

	unicodeName := validDefinition()
	unicodeName.Name = "提交规范"

	for name, def := range map[string]ResourceDefinition{
		"skill":          skill,
		"mcp":            mcp,
		"public mcp":     publicMCP,
		"project prompt": projectPrompt,
		"session prompt": sessionPrompt,
		"unicode name":   unicodeName,
	} {
		if err := def.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", name, err)
		}
	}
}

func TestContractsResourceDefinitionValidateRejects(t *testing.T) {
	mutate := map[string]func(*ResourceDefinition){
		"zero id":                func(d *ResourceDefinition) { d.ID = 0 },
		"unknown kind":           func(d *ResourceDefinition) { d.Kind = "tool" },
		"empty name":             func(d *ResourceDefinition) { d.Name = "" },
		"blank name":             func(d *ResourceDefinition) { d.Name = "   " },
		"dot name":               func(d *ResourceDefinition) { d.Name = "." },
		"dotdot name":            func(d *ResourceDefinition) { d.Name = ".." },
		"slash name":             func(d *ResourceDefinition) { d.Name = "a/b" },
		"backslash name":         func(d *ResourceDefinition) { d.Name = `a\b` },
		"nul name":               func(d *ResourceDefinition) { d.Name = "a\x00b" },
		"scope on skill":         func(d *ResourceDefinition) { d.PromptScope = PromptScopeProject },
		"prompt without scope":   func(d *ResourceDefinition) { d.Kind = ResourcePrompt },
		"prompt bad scope":       func(d *ResourceDefinition) { d.Kind = ResourcePrompt; d.PromptScope = "global" },
		"secret on non-mcp":      func(d *ResourceDefinition) { d.SecretRef = "sec_1" },
		"unknown backend":        func(d *ResourceDefinition) { d.Backends = []agents.BackendID{"claude"} },
		"zero created timestamp": func(d *ResourceDefinition) { d.CreatedAt = time.Time{} },
		"zero updated timestamp": func(d *ResourceDefinition) { d.UpdatedAt = time.Time{} },
	}

	for name, fn := range mutate {
		def := validDefinition()
		fn(&def)
		err := def.Validate()
		if err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
			continue
		}
		if !errors.Is(err, ErrInvalidResource) {
			t.Errorf("%s: Validate() = %v, want ErrInvalidResource", name, err)
		}
	}
}

func TestContractsResourceDefinitionJSONOmitsRawSecret(t *testing.T) {
	def := validDefinition()
	def.Kind = ResourceMCP
	def.Name = "filesystem-server"
	def.SecretRef = "sec_opaque_1"

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got := string(decoded["secretRef"]); got != `"sec_opaque_1"` {
		t.Errorf("secretRef = %s, want opaque reference %q", got, "sec_opaque_1")
	}

	// The definition must carry only the opaque reference. A raw value field
	// (or any alias) would let the API leak the credential.
	for _, forbidden := range []string{"secret", "secretValue", "value", "token", "password", "credentials"} {
		if _, ok := decoded[forbidden]; ok {
			t.Errorf("JSON must not contain %q, got: %s", forbidden, data)
		}
	}

	if strings.Contains(string(data), "plaintext") {
		t.Errorf("JSON must not contain secret material: %s", data)
	}
}
