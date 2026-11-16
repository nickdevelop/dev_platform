package agy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/providers/agy/acp"
)

func TestAdapterCapabilitiesDeclared(t *testing.T) {
	adapter := New(Options{})
	caps := adapter.Capabilities()

	if !caps.Streaming {
		t.Error("Streaming must be true")
	}
	if caps.SessionDiscovery {
		t.Error("SessionDiscovery must be false")
	}
	if !caps.SessionContinue {
		t.Error("SessionContinue must be true")
	}
	if !caps.Stop {
		t.Error("Stop must be true")
	}
	if !caps.ModelSelection {
		t.Error("ModelSelection must be true")
	}
	if !caps.ToolCallEvents {
		t.Error("ToolCallEvents must be true")
	}
	if !caps.ToolConfirmation {
		t.Error("ToolConfirmation must be true")
	}
	if !caps.Interrupt {
		t.Error("Interrupt must be true")
	}
	if !caps.SessionHistory {
		t.Error("SessionHistory must be true")
	}
}

func TestAdapterLifecycle(t *testing.T) {
	ctx := context.Background()
	seq := 0
	adapter := New(Options{
		NewID: func() (string, error) {
			seq++
			return "test-id-1", nil
		},
		Now: func() time.Time {
			return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
		},
	})

	// Start instance
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{
		Directory: "/mock/workspace",
	})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if inst.ID != "test-id-1" || inst.Backend != agents.BackendAgy || inst.Status != agents.InstanceRunning {
		t.Fatalf("unexpected instance: %+v", inst)
	}

	// Create session
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{
		Title: "New AGY Session",
		Model: "gemini-3.8-flash-high",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ref.Backend != agents.BackendAgy || ref.InstanceID != inst.ID || ref.Title != "New AGY Session" {
		t.Fatalf("unexpected session ref: %+v", ref)
	}

	// Continue session
	contRef, err := adapter.ContinueSession(ctx, *ref)
	if err != nil {
		t.Fatalf("ContinueSession: %v", err)
	}
	if contRef.SessionID != ref.SessionID || contRef.Title != "New AGY Session" {
		t.Fatalf("unexpected continued ref: %+v", contRef)
	}

	// Subscribe
	subCh, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if subCh == nil {
		t.Fatal("expected non-nil subscriber channel")
	}

	// Stop instance
	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}

	// After stop, creating session on stopped instance should fail
	_, err = adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err == nil {
		t.Fatal("expected error creating session on stopped instance")
	}
}

func TestAgyChildProcessManagement(t *testing.T) {
	inst := newInstance("inst-test", t.TempDir(), time.Now)
	child := newAgyChild(inst, "sess-test", "model-test", PermissionModeConfirm, time.Minute, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Launch a child process using "cat" which waits for stdin
	err := child.start(ctx, "cat", nil, nil)
	if err == nil {
		if child.cmd == nil || child.cmd.Process == nil {
			t.Fatal("expected non-nil cmd.Process")
		}
		if err := child.close(); err != nil {
			t.Errorf("close child failed: %v", err)
		}
	}
}

func TestAgyPermissionReplier(t *testing.T) {
	inst := newInstance("inst-perm", t.TempDir(), time.Now)
	child := newAgyChild(inst, "sess-perm", "model-test", PermissionModeConfirm, time.Minute, time.Now)
	inst.setChild("sess-perm", child)

	adapter := New(Options{})
	adapter.instances["inst-perm"] = inst

	ref := agents.SessionRef{
		InstanceID: "inst-perm",
		SessionID:  "sess-perm",
		Backend:    agents.BackendAgy,
	}

	// Check initially empty permissions
	perms, err := adapter.ListPermissions(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(perms) != 0 {
		t.Fatalf("expected 0 permissions, got %d", len(perms))
	}

	// Simulate incoming server request
	go func() {
		params := acp.RequestPermissionParams{
			SessionID: "sess-perm",
			ToolCall: acp.ToolCallUpdate{
				ToolCallID: "tc-1",
				Title:      "Run Command",
			},
			Options: []acp.PermissionOption{
				{OptionID: "opt-allow", Name: "Allow", Kind: "allow_once"},
				{OptionID: "opt-reject", Name: "Reject", Kind: "reject_once"},
			},
		}
		b, _ := json.Marshal(params)
		_, _ = child.handleRequest(context.Background(), "req-42", acp.MethodRequestPermission, b)
	}()

	// Wait for pending permission
	time.Sleep(50 * time.Millisecond)

	perms, err = adapter.ListPermissions(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(perms) != 1 || perms[0].ID != "req-42" {
		t.Fatalf("expected 1 pending permission for req-42, got %+v", perms)
	}

	// Reply permission
	err = adapter.ReplyPermission(context.Background(), ref, "req-42", "once", "")
	if err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}

	// Verify it resolved
	perms, err = adapter.ListPermissions(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(perms) != 0 {
		t.Fatalf("expected 0 permissions after resolution, got %d", len(perms))
	}
}

func TestAdapterDetectTurn(t *testing.T) {
	ctx := context.Background()
	adapter := New(Options{
		NewID: func() (string, error) { return "inst-detect", nil },
	})

	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{
		Directory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}

	ref := agents.SessionRef{
		InstanceID: inst.ID,
		Backend:    agents.BackendAgy,
		SessionID:  "sess-detect-1",
	}

	// 1. Before child exists -> false
	running, err := adapter.DetectTurn(ctx, ref)
	if err != nil {
		t.Fatalf("DetectTurn without child: %v", err)
	}
	if running {
		t.Errorf("DetectTurn without child = true, want false")
	}

	// 2. Add an idle child
	child := newAgyChild(adapter.instances[inst.ID], "sess-detect-1", "gemini-3.8-flash", PermissionModeAuto, time.Minute, nil)
	adapter.instances[inst.ID].setChild("sess-detect-1", child)

	running, err = adapter.DetectTurn(ctx, ref)
	if err != nil {
		t.Fatalf("DetectTurn with idle child: %v", err)
	}
	if running {
		t.Errorf("DetectTurn with idle child = true, want false")
	}

	// 3. Mark child turn running
	child.mu.Lock()
	child.turnRunning = true
	child.mu.Unlock()

	running, err = adapter.DetectTurn(ctx, ref)
	if err != nil {
		t.Fatalf("DetectTurn with running child: %v", err)
	}
	if !running {
		t.Errorf("DetectTurn with running child = false, want true")
	}

	// 4. Validation errors
	invalidRef := ref
	invalidRef.Backend = "unknown"
	if _, err := adapter.DetectTurn(ctx, invalidRef); err == nil {
		t.Errorf("expected error for mismatched backend")
	}
}
