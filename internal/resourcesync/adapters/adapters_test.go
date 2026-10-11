package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"devplatform/internal/agents"
	"devplatform/internal/resources"
	"devplatform/internal/resourcesync"
)

type fakeSecrets struct {
	values map[string]string
	err    error
}

func (f fakeSecrets) Get(_ context.Context, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.values[id]
	if !ok {
		return "", errors.New("secrets: not found")
	}
	return value, nil
}

func definition(kind resources.ResourceKind, name, content string) resources.ResourceDefinition {
	now := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	return resources.ResourceDefinition{
		ID:        1,
		Kind:      kind,
		Name:      name,
		Content:   content,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestAdaptersSupports(t *testing.T) {
	project := ProjectPrompts{}
	if !project.Supports(resources.ResourcePrompt) {
		t.Error("ProjectPrompts does not support prompts")
	}
	if project.Supports(resources.ResourceSkill) || project.Supports(resources.ResourceMCP) {
		t.Error("ProjectPrompts declared a non-prompt kind")
	}

	for _, adapter := range []resourcesync.ResourceSyncAdapter{OpenCode{}, Pi{}} {
		for _, kind := range []resources.ResourceKind{resources.ResourcePrompt, resources.ResourceSkill, resources.ResourceMCP} {
			if !adapter.Supports(kind) {
				t.Errorf("%s does not support %s", adapter.BackendID(), kind)
			}
		}
		if adapter.Supports("tool") {
			t.Errorf("%s declared an unknown kind", adapter.BackendID())
		}
	}

	if (OpenCode{}).BackendID() != agents.BackendOpenCodeV2 {
		t.Errorf("OpenCode backend = %q", (OpenCode{}).BackendID())
	}
	if (Pi{}).BackendID() != agents.BackendPi {
		t.Errorf("Pi backend = %q", (Pi{}).BackendID())
	}
	if (ProjectPrompts{}).BackendID() != "" {
		t.Errorf("ProjectPrompts backend = %q, want empty", (ProjectPrompts{}).BackendID())
	}
}

func TestProjectPromptsRender(t *testing.T) {
	patch, err := ProjectPrompts{}.Render(context.Background(),
		definition(resources.ResourcePrompt, "house-rules", "Always run tests."),
		resourcesync.RenderContext{ProjectID: 7})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if patch.Target.Path != ".devplatform/prompts/house-rules.md" {
		t.Fatalf("target = %q", patch.Target.Path)
	}
	if patch.Target.BackendID != "" {
		t.Fatalf("project target backend = %q, want empty", patch.Target.BackendID)
	}
	content := string(patch.Content)
	if !strings.Contains(content, "# house-rules") || !strings.Contains(content, "Always run tests.") {
		t.Fatalf("content = %q, want a markdown prompt", content)
	}
}

func TestOpenCodeRenderFormats(t *testing.T) {
	ctx := context.Background()

	prompt, err := OpenCode{}.Render(ctx, definition(resources.ResourcePrompt, "style", "Use tabs."), resourcesync.RenderContext{ProjectID: 7})
	if err != nil {
		t.Fatalf("Render prompt: %v", err)
	}
	if prompt.Target.Path != ".opencode/prompts/style.md" || prompt.Target.BackendID != agents.BackendOpenCodeV2 {
		t.Fatalf("prompt target = %+v", prompt.Target)
	}

	mcp := definition(resources.ResourceMCP, "server", `{"command":"npx","args":["-y","server"]}`)
	mcp.SecretRef = "sec_1"
	patch, err := OpenCode{}.Render(ctx, mcp, resourcesync.RenderContext{
		ProjectID: 7,
		Secrets:   fakeSecrets{values: map[string]string{"sec_1": "topsecret"}},
	})
	if err != nil {
		t.Fatalf("Render mcp: %v", err)
	}
	if patch.Target.Path != ".opencode/mcp/server.json" {
		t.Fatalf("mcp target = %q", patch.Target.Path)
	}
	var doc map[string]any
	if err := json.Unmarshal(patch.Content, &doc); err != nil {
		t.Fatalf("decode opencode mcp: %v", err)
	}
	if doc["name"] != "server" {
		t.Fatalf("mcp name = %v", doc["name"])
	}
	credentials, ok := doc["credentials"].(map[string]any)
	if !ok || credentials["token"] != "topsecret" {
		t.Fatalf("opencode credentials = %v, want credentials.token", doc["credentials"])
	}
}

func TestPiRenderFormats(t *testing.T) {
	mcp := definition(resources.ResourceMCP, "server", `{"command":"npx"}`)
	mcp.SecretRef = "sec_1"
	patch, err := Pi{}.Render(context.Background(), mcp, resourcesync.RenderContext{
		ProjectID: 7,
		Secrets:   fakeSecrets{values: map[string]string{"sec_1": "topsecret"}},
	})
	if err != nil {
		t.Fatalf("Render mcp: %v", err)
	}
	if patch.Target.Path != ".pi/mcp/server.json" || patch.Target.BackendID != agents.BackendPi {
		t.Fatalf("pi mcp target = %+v", patch.Target)
	}
	var doc map[string]any
	if err := json.Unmarshal(patch.Content, &doc); err != nil {
		t.Fatalf("decode pi mcp: %v", err)
	}
	if doc["serverName"] != "server" {
		t.Fatalf("pi serverName = %v", doc["serverName"])
	}
	env, ok := doc["env"].(map[string]any)
	if !ok || env["MCP_TOKEN"] != "topsecret" {
		t.Fatalf("pi env = %v, want env.MCP_TOKEN", doc["env"])
	}
}

func TestAdaptersSecretResolutionWithoutLeak(t *testing.T) {
	mcp := definition(resources.ResourceMCP, "server", `{"command":"npx"}`)
	mcp.SecretRef = "sec_1"

	// A missing value is reported by reference only.
	_, err := Pi{}.Render(context.Background(), mcp, resourcesync.RenderContext{
		ProjectID: 7,
		Secrets:   fakeSecrets{err: errors.New("store unavailable")},
	})
	if err == nil {
		t.Fatal("Render with a failing store = nil, want error")
	}
	if !strings.Contains(err.Error(), "sec_1") {
		t.Fatalf("error does not name the reference: %v", err)
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("error leaked a value: %v", err)
	}

	// No store configured is refused rather than embedding an empty secret.
	_, err = Pi{}.Render(context.Background(), mcp, resourcesync.RenderContext{ProjectID: 7})
	if err == nil {
		t.Fatal("Render without a store = nil, want error")
	}
}
