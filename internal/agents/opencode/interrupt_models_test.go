package opencode

import (
	"context"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"devplatform/internal/agents"
)

func TestAdapterListModelsMapsProviderModels(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/model": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":[{"id":"m1","modelID":"model","providerID":"prov","name":"Model","variants":[{"id":"fast"}]},{"id":"m2","modelID":"plain","providerID":"prov","name":"Plain"}]}`))
		},
	}))
	instance := startTestInstance(t, adapter)

	got, err := adapter.ListModels(context.Background(), instance.ID)
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
	if _, err := adapter.ListModels(context.Background(), "missing"); err == nil {
		t.Error("ListModels(unknown instance) = nil, want error")
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
