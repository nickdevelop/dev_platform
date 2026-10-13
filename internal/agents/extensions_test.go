package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestPermissionRequestJSONShape(t *testing.T) {
	got := jsonKeys(t, PermissionRequest{ID: "p", SessionID: "s", Action: "bash", Resources: []string{"ls"}, Message: "m"})
	want := []string{"action", "id", "message", "resources", "sessionID"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PermissionRequest keys = %v, want %v", got, want)
	}
	omitted := jsonKeys(t, PermissionRequest{ID: "p", SessionID: "s", Action: "bash"})
	if want := []string{"action", "id", "resources", "sessionID"}; !reflect.DeepEqual(omitted, want) {
		t.Fatalf("PermissionRequest keys without message = %v, want %v", omitted, want)
	}
}

func TestFormInfoJSONShape(t *testing.T) {
	got := jsonKeys(t, FormInfo{ID: "f", SessionID: "s", Title: "t", Fields: json.RawMessage(`[]`)})
	want := []string{"fields", "id", "sessionID", "title"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FormInfo keys = %v, want %v", got, want)
	}
	if omitted := jsonKeys(t, FormInfo{ID: "f", SessionID: "s", Title: "t"}); !reflect.DeepEqual(omitted, []string{"id", "sessionID", "title"}) {
		t.Fatalf("FormInfo keys without fields = %v", omitted)
	}
}

func TestErrReplyExpiredIsWrappable(t *testing.T) {
	wrapped := fmt.Errorf("%w: backend said gone", ErrReplyExpired)
	if !errors.Is(wrapped, ErrReplyExpired) {
		t.Fatal("wrapped ErrReplyExpired must satisfy errors.Is")
	}
}
