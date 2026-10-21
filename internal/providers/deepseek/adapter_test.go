package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func newMockDSHServer(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{
				Name:  "dsh-auth",
				Value: "test-auth-cookie",
			})
			w.WriteHeader(http.StatusFound)
			return
		}

		if r.URL.Path == "/api/remote.mux" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			for {
				var msg wsClientMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "open" && msg.Endpoint == "$events" {
					_ = conn.WriteJSON(wsServerMessage{
						Type:     "item",
						StreamID: msg.StreamID,
						Value:    json.RawMessage(`{"type": "ready", "clientId": "client-test-1"}`),
					})
				}
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			method := strings.TrimPrefix(r.URL.Path, "/api/")
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			var val any

			switch method {
			case "session/list":
				val = map[string]any{
					"items": []any{
						map[string]any{
							"sessionId": "ses-proj1",
							"cwd":       "/data/projects/project1",
							"blank":     false,
							"running":   true,
							"projections": map[string]any{
								"values": map[string]any{
									"title": "Project 1 Session",
								},
							},
						},
						map[string]any{
							"sessionId": "ses-proj2",
							"cwd":       "/data/projects/project2",
							"blank":     false,
							"running":   false,
						},
						map[string]any{
							"sessionId": "ses-blank",
							"cwd":       "/data/projects/project1",
							"blank":     true,
						},
					},
				}
			case "session/create":
				val = map[string]any{
					"sessionId":   "ses-created-123",
					"agentPreset": "standard",
				}
			case "session/selectModel", "session/rename", "session/cancel", "$events/result":
				val = map[string]any{"ok": true}
			case "session/prompt":
				val = map[string]any{"accepted": true}
			case "session/projections":
				val = map[string]any{
					"asOfSeq": 20,
					"values": map[string]any{
						"title": "Active Session",
					},
				}
			case "session/page":
				val = map[string]any{
					"records": []any{
						map[string]any{
							"type": "event",
							"event": map[string]any{
								"seq":  10,
								"type": "user/message",
								"data": map[string]any{
									"content": "hello from user",
								},
							},
						},
						map[string]any{
							"type": "event",
							"event": map[string]any{
								"seq":  15,
								"type": "tool/call",
								"data": map[string]any{
									"callId":    "call-read-1",
									"name":      "read",
									"arguments": map[string]any{"path": "test.txt"},
								},
							},
						},
					},
					"hasMore": false,
				}
			case "session/modelCatalog":
				val = map[string]any{
					"groups": []any{
						map[string]any{
							"id":   "deepseek",
							"name": "DeepSeek",
							"models": []any{
								map[string]any{
									"id":            "deepseek-chat",
									"name":          "DeepSeek V3",
									"contextWindow": 128000,
								},
							},
						},
					},
				}
			case "commands/list":
				val = []any{
					map[string]any{
						"name":        "compact",
						"description": "Compact history",
					},
				}
			case "skills/list":
				val = map[string]any{
					"skills": []any{
						map[string]any{
							"name":        "websearch",
							"description": "Search the web",
						},
					},
				}
			default:
				val = map[string]any{"ok": true}
			}

			respBytes, _ := json.Marshal(rpcResponse{
				Type:  "server-response",
				RPCID: req.RPCID,
				Result: rpcResult{
					OK:    true,
					Value: mustJSON(val),
				},
			})
			w.Write(respBytes)
			return
		}

		http.NotFound(w, r)
	}))

	client := NewClient(server.URL, "dsh-auth=test-auth-cookie")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.ConnectMux(ctx); err != nil {
		t.Fatalf("failed to connect mock mux: %v", err)
	}

	return server, client
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestAdapterCapabilitiesDeclared(t *testing.T) {
	adapter := New(Options{})
	caps := adapter.Capabilities()

	if !caps.Streaming {
		t.Error("Streaming must be true")
	}
	if !caps.SessionDiscovery {
		t.Error("SessionDiscovery must be true")
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
	if caps.RawEvents {
		t.Error("RawEvents must be false")
	}
}

func TestAdapterProbe(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "dsh")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	adapter := New(Options{
		Executable: fakeBinary,
	})

	if err := adapter.Probe(context.Background()); err != nil {
		t.Fatalf("Probe() with valid executable failed: %v", err)
	}

	missingAdapter := New(Options{
		Executable: filepath.Join(tmpDir, "nonexistent"),
	})
	if err := missingAdapter.Probe(context.Background()); err == nil {
		t.Fatal("Probe() with missing executable must fail")
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := adapter.Probe(canceledCtx); err == nil {
		t.Fatal("Probe() with canceled context must fail")
	}
}

func TestAdapterDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "dsh")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	adapter := New(Options{
		Executable: fakeBinary,
		VersionProbe: func(ctx context.Context) (string, error) {
			return "dsh 0.2.1-rc.2", nil
		},
	})

	report := adapter.Diagnostics(context.Background())
	if report.Status != agents.DiagnosticOK {
		t.Fatalf("Diagnostics status = %s, want OK. Checks: %+v", report.Status, report.Checks)
	}
	if report.Version != "0.2.1" {
		t.Errorf("version = %s, want 0.2.1", report.Version)
	}

	// Out of range version
	outOfRangeAdapter := New(Options{
		Executable: fakeBinary,
		VersionProbe: func(ctx context.Context) (string, error) {
			return "dsh 0.1.0", nil
		},
	})
	reportOut := outOfRangeAdapter.Diagnostics(context.Background())
	if reportOut.Status != agents.DiagnosticWarning {
		t.Errorf("Diagnostics status = %s, want Warning for out-of-range version", reportOut.Status)
	}
}

func TestAdapterLifecycleAndSessions(t *testing.T) {
	server, client := newMockDSHServer(t)
	defer server.Close()
	defer client.Close()

	adapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			return &Process{
				client:   client,
				exitDone: make(chan struct{}),
			}, nil
		},
	})

	ctx := context.Background()

	// 1. StartInstance
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{
		Directory: "/data/projects/project1",
	})
	if err != nil {
		t.Fatalf("StartInstance failed: %v", err)
	}
	if inst.ID == "" || inst.Backend != agents.BackendDeepSeek || inst.Directory != "/data/projects/project1" {
		t.Fatalf("unexpected instance: %+v", inst)
	}

	// 2. ListSessions (should only return sessions matching /data/projects/project1)
	sessions, err := adapter.ListSessions(ctx, inst.ID)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("ListSessions returned %d sessions, want 1", len(sessions))
	}
	if sessions[0].SessionID != "ses-proj1" || sessions[0].Title != "Project 1 Session" {
		t.Errorf("session = %+v, want ses-proj1 with title 'Project 1 Session'", sessions[0])
	}

	// 3. CreateSession
	created, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{
		Title: "New Feature",
		Model: "deepseek/deepseek-chat",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if created.SessionID != "ses-created-123" {
		t.Errorf("created SessionID = %s, want ses-created-123", created.SessionID)
	}

	// 4. ContinueSession
	ref, err := adapter.ContinueSession(ctx, *created)
	if err != nil {
		t.Fatalf("ContinueSession failed: %v", err)
	}
	if ref.Title != "Active Session" {
		t.Errorf("ref.Title = %s, want 'Active Session'", ref.Title)
	}

	// 5. SendMessage
	err = adapter.SendMessage(ctx, *created, agents.SendMessageRequest{
		Text: "write hello world",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// 6. Interrupt
	err = adapter.Interrupt(ctx, *created)
	if err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	// 7. DetectTurn
	running, err := adapter.DetectTurn(ctx, agents.SessionRef{
		InstanceID: inst.ID,
		Backend:    agents.BackendDeepSeek,
		SessionID:  "ses-proj1",
	})
	if err != nil {
		t.Fatalf("DetectTurn failed: %v", err)
	}
	if !running {
		t.Error("DetectTurn must report true for ses-proj1")
	}

	// 8. StopInstance
	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Fatalf("StopInstance failed: %v", err)
	}
}

func TestAdapterHistoryReader(t *testing.T) {
	server, client := newMockDSHServer(t)
	defer server.Close()
	defer client.Close()

	adapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			return &Process{
				client:   client,
				exitDone: make(chan struct{}),
			}, nil
		},
	})

	res, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{
		Directory: "/data/projects/project1",
		SessionID: "ses-proj1",
	})
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}

	if len(res.Events) != 2 {
		t.Fatalf("ListHistory returned %d events, want 2", len(res.Events))
	}
	if res.Events[0].Type != agents.EventMessage || res.Events[0].Message.Text != "hello from user" {
		t.Errorf("event 0 mismatch: %+v", res.Events[0])
	}
	if res.Events[1].Type != agents.EventToolCall || res.Events[1].ToolCall.CallID != "call-read-1" {
		t.Errorf("event 1 mismatch: %+v", res.Events[1])
	}
	if res.Events[0].Sequence != 1 || res.Events[1].Sequence != 2 {
		t.Errorf("sequences = %d, %d, want 1, 2", res.Events[0].Sequence, res.Events[1].Sequence)
	}
	if res.Events[0].InstanceID != "" || res.Events[0].SessionID != "" {
		t.Errorf("InstanceID and SessionID must be empty in HistoryResult")
	}
}

func TestAdapterPermissionsAndCommands(t *testing.T) {
	server, client := newMockDSHServer(t)
	defer server.Close()
	defer client.Close()

	adapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			return &Process{
				client:   client,
				exitDone: make(chan struct{}),
			}, nil
		},
	})

	ref := agents.SessionRef{
		InstanceID: "inst-1",
		Backend:    agents.BackendDeepSeek,
		SessionID:  "ses-1",
	}

	// 1. Permission replier
	adapter.permissions.AddPermission(agents.PermissionRequest{
		ID:        "perm-1",
		SessionID: "ses-1",
		Action:    "run bash",
		Resources: []string{"bash"},
	})

	perms, err := adapter.ListPermissions(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListPermissions failed: %v", err)
	}
	if len(perms) != 1 || perms[0].ID != "perm-1" {
		t.Fatalf("perms = %+v, want perm-1", perms)
	}

	err = adapter.ReplyPermission(context.Background(), ref, "perm-1", "allow", "")
	if err != nil {
		t.Fatalf("ReplyPermission failed: %v", err)
	}

	// Replying expired or removed permission must return ErrReplyExpired
	err = adapter.ReplyPermission(context.Background(), ref, "perm-1", "allow", "")
	if err != agents.ErrReplyExpired {
		t.Fatalf("expected ErrReplyExpired, got %v", err)
	}

	// 2. Command discoverer
	commands, err := adapter.ListCommands(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListCommands failed: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("ListCommands returned %d items, want 2 (1 slash command, 1 skill)", len(commands))
	}
	if commands[0].Name != "compact" || commands[0].Kind != agents.CommandKindSlash {
		t.Errorf("command 0 = %+v, want compact (command)", commands[0])
	}
	if commands[1].Name != "websearch" || commands[1].Kind != agents.CommandKindSkill {
		t.Errorf("command 1 = %+v, want websearch (skill)", commands[1])
	}

	// 3. ListModels
	models, err := adapter.ListModels(context.Background(), "/data/projects/project1")
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 1 || models[0].ID != "deepseek/deepseek-chat" {
		t.Errorf("models = %+v, want deepseek/deepseek-chat", models)
	}
}

func TestAdapterAutoAdmitAndSafeSubscriptionClose(t *testing.T) {
	server, client := newMockDSHServer(t)
	defer server.Close()
	defer client.Close()

	adapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			return &Process{
				client:   client,
				exitDone: make(chan struct{}),
			}, nil
		},
	})

	ctx := context.Background()
	// An instance ID that was not created in memory (e.g. surviving platform restart)
	restartedInstanceID := "inst-dsh-restarted-999"

	// 1. CreateSession must auto-admit the instance and succeed rather than failing with ErrInvalidInstance
	created, err := adapter.CreateSession(ctx, restartedInstanceID, agents.CreateSessionOptions{
		Title: "Surviving Session",
		Model: "deepseek/deepseek-chat",
	})
	if err != nil {
		t.Fatalf("CreateSession on surviving instance failed: %v", err)
	}
	if created.SessionID != "ses-created-123" {
		t.Errorf("created SessionID = %s, want ses-created-123", created.SessionID)
	}

	// 2. Subscribe on surviving instance must also auto-admit and establish channel
	subCtx, cancelSub := context.WithCancel(ctx)
	eventsCh, err := adapter.Subscribe(subCtx, restartedInstanceID)
	if err != nil {
		t.Fatalf("Subscribe on surviving instance failed: %v", err)
	}
	if eventsCh == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// 3. StopInstance and cancelSub concurrently must NOT cause double-close panic
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = adapter.StopInstance(ctx, restartedInstanceID)
		cancelSub()
	}()
	<-done

	// Verify channel is closed properly and drained
	for range eventsCh {
	}
}

func TestSharedHostDefaultIdleTimeout(t *testing.T) {
	host := NewSharedHost(SharedHostOptions{})
	if host.idleTimeout != 30*time.Minute {
		t.Errorf("host.idleTimeout = %v, want 30m0s", host.idleTimeout)
	}
}

func TestAdapterSessionModelTrackingAndHistory(t *testing.T) {
	server, client := newMockDSHServer(t)
	defer server.Close()
	defer client.Close()

	adapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			return &Process{
				client:   client,
				exitDone: make(chan struct{}),
			}, nil
		},
	})

	ctx := context.Background()
	instID := "inst-dsh-test-model"

	// 1. CreateSession sets the session model
	created, err := adapter.CreateSession(ctx, instID, agents.CreateSessionOptions{
		Model: "deepseek/deepseek-chat",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	model := adapter.sessionModel(created.SessionID)
	if model == nil {
		t.Fatalf("sessionModel(%q) = nil, want deepseek/deepseek-chat", created.SessionID)
	}
	if model.ID != "deepseek/deepseek-chat" || model.Provider != "deepseek" {
		t.Errorf("sessionModel = %+v, want ID: deepseek/deepseek-chat, Provider: deepseek", model)
	}

	// 2. SendMessage with new model updates the session model
	err = adapter.SendMessage(ctx, *created, agents.SendMessageRequest{
		Text:  "switch to reasoner",
		Model: "deepseek/deepseek-reasoner",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	updatedModel := adapter.sessionModel(created.SessionID)
	if updatedModel == nil || updatedModel.ID != "deepseek/deepseek-reasoner" {
		t.Errorf("updated sessionModel = %+v, want deepseek/deepseek-reasoner", updatedModel)
	}

	// 3. ListHistory stamps model from request/context onto assistant messages
	histServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		method := r.URL.Path[len("/api/"):]

		var val any
		switch method {
		case "session/projections":
			val = map[string]any{
				"asOfSeq": 10,
				"values":  map[string]any{},
			}
		case "session/page":
			val = map[string]any{
				"records": []any{
					map[string]any{
						"type": "event",
						"event": map[string]any{
							"seq":  1,
							"type": "request/context",
							"data": map[string]any{
								"provider":      "deepseek",
								"model":         "deepseek-chat",
								"contextWindow": 128000,
							},
						},
					},
					map[string]any{
						"type": "event",
						"event": map[string]any{
							"seq":  2,
							"type": "assistant/message",
							"data": map[string]any{
								"message": map[string]any{
									"role":    "assistant",
									"content": "Hello world from deepseek",
								},
							},
						},
					},
				},
				"hasMore": false,
			}
		}
		respBytes, _ := json.Marshal(rpcResponse{
			Type:  "server-response",
			RPCID: req.RPCID,
			Result: rpcResult{
				OK:    true,
				Value: mustJSON(val),
			},
		})
		w.Write(respBytes)
	}))
	defer histServer.Close()

	histClient := NewClient(histServer.URL, "auth=test")
	defer histClient.Close()

	histAdapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			return &Process{
				client:   histClient,
				exitDone: make(chan struct{}),
			}, nil
		},
	})

	histRes, err := histAdapter.ListHistory(ctx, agents.ListHistoryRequest{
		Directory: "/data/projects/project1",
		SessionID: "ses-test-hist-model",
	})
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}

	foundAssistant := false
	for _, ev := range histRes.Events {
		if ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "assistant" {
			foundAssistant = true
			if ev.Message.Model == nil {
				t.Fatalf("assistant message Model is nil in ListHistory")
			}
			if ev.Message.Model.ID != "deepseek/deepseek-chat" || ev.Message.Model.Provider != "deepseek" {
				t.Errorf("assistant message Model = %+v, want deepseek/deepseek-chat", ev.Message.Model)
			}
		}
	}
	if !foundAssistant {
		t.Error("assistant message not found in ListHistory result")
	}
}

func TestAdapterClose(t *testing.T) {
	exitDone := make(chan struct{})
	adapter := New(Options{
		ProcessLauncher: func(ctx context.Context) (*Process, error) {
			proc := &Process{
				exitDone: exitDone,
			}
			go func() {
				time.Sleep(20 * time.Millisecond)
				close(exitDone)
			}()
			return proc, nil
		},
	})

	// Acquire to start process
	_, release, err := adapter.host.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	release()

	// Verify Close releases the host
	if err := adapter.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	adapter.mu.Lock()
	stopped := adapter.host.proc == nil
	adapter.mu.Unlock()

	if !stopped {
		t.Error("host.proc must be nil after Close")
	}
}
