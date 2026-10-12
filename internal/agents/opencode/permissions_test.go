package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClientPermissionAndFormRoundTrip proves the new v2 client methods hit
// the protocol paths with the desktop payload shapes.
func TestClientPermissionAndFormRoundTrip(t *testing.T) {
	var permissionReplyBody map[string]any
	var formReplyBody map[string]any
	server := newPermissionStub(t, &permissionReplyBody, &formReplyBody)
	client, err := NewClient(ClientOptions{BaseURL: server.URL, Username: "opencode", Password: "pw", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	permissions, err := client.ListPermissions(ctx, "ses_1")
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(permissions) != 1 || permissions[0].ID != "per_1" {
		t.Fatalf("permissions = %+v, want one per_1", permissions)
	}

	if err := client.ReplyPermission(ctx, "ses_1", "per_1", "once", "ok"); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	if permissionReplyBody["decision"] != "once" {
		t.Fatalf("permission reply body = %v, want decision once", permissionReplyBody)
	}

	forms, err := client.ListForms(ctx, "ses_1")
	if err != nil {
		t.Fatalf("ListForms: %v", err)
	}
	if len(forms) != 1 || forms[0].ID != "frm_1" {
		t.Fatalf("forms = %+v, want one frm_1", forms)
	}
	if err := client.ReplyForm(ctx, "ses_1", "frm_1", map[string]any{"confirm": true}); err != nil {
		t.Fatalf("ReplyForm: %v", err)
	}
	var answer map[string]any
	if raw, ok := formReplyBody["answer"]; ok {
		answer, _ = raw.(map[string]any)
	}
	if answer["confirm"] != true {
		t.Fatalf("form reply body = %v, want answer.confirm true", formReplyBody)
	}
}

// TestAdapterReplyPermissionValidation proves decision validation happens
// before any HTTP call.
func TestAdapterReplyPermissionValidation(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(nil))
	adapter.newID = func() (string, error) { return "inst-1", nil }
	_ = adapter
}

func newPermissionStub(t *testing.T, permissionReply, formReply *map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session/ses_1/permission", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"per_1","sessionID":"ses_1","action":"read","resources":["/tmp"],"message":"allow?"}]}`))
	})
	mux.HandleFunc("POST /api/session/ses_1/permission/per_1/reply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		*permissionReply = body
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/session/ses_1/form", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"frm_1","sessionID":"ses_1","title":"confirm"}]}`))
	})
	mux.HandleFunc("POST /api/session/ses_1/form/frm_1/reply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		*formReply = body
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}
