package opencode

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"devplatform/internal/agents"
)

func TestAdapterListModelsMapsProviderModels(t *testing.T) {
	var requested string
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/model": func(w http.ResponseWriter, r *http.Request) {
			requested = r.URL.Query().Get("directory")
			w.Write([]byte(`{"data":[{"id":"m1","modelID":"model","providerID":"prov","name":"Model","variants":[{"id":"fast"}]},{"id":"m2","modelID":"plain","providerID":"prov","name":"Plain"}]}`))
		},
	}))

	// No instance was started: the shared service answers for the directory, so
	// the workbench can offer a model before it has an instance.
	got, err := adapter.ListModels(context.Background(), "/work/app")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []agents.ModelInfo{
		{ID: "prov/model", Name: "Model", Provider: "prov", Variants: []string{"fast"}},
		{ID: "prov/plain", Name: "Plain", Provider: "prov"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListModels() = %+v, want %+v", got, want)
	}
	if requested != "/work/app" {
		t.Errorf("server was asked about directory %q, want %q", requested, "/work/app")
	}
}

// TestAdapterListModelsWithoutService checks the refusal when no machine-wide
// service is registered: the platform reports it instead of starting one.
func TestAdapterListModelsWithoutService(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	adapter := New(Options{Executable: executable, ServiceFile: filepath.Join(t.TempDir(), "service.json")})
	if _, err := adapter.ListModels(context.Background(), "/work/app"); !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Errorf("ListModels without a service = %v, want ErrServiceNotRunning", err)
	}
}

func TestAdapterInterruptCallsSessionInterrupt(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session/ses_1/interrupt": func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			paths = append(paths, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		},
	}))
	instance := startTestInstance(t, adapter)

	ref := agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendOpenCodeV2, SessionID: "ses_1"}
	if err := adapter.Interrupt(context.Background(), ref); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(paths, []string{"/api/session/ses_1/interrupt"}) {
		t.Errorf("interrupt requests = %v", paths)
	}
	wrong := agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendPi, SessionID: "ses_1"}
	if err := adapter.Interrupt(context.Background(), wrong); err == nil {
		t.Error("Interrupt(pi backend ref) = nil, want error")
	}
}
