package pi

import (
	"encoding/json"
	"testing"

	"devplatform/internal/agents"
)

func TestEditFileDiffFromPatch(t *testing.T) {
	result := json.RawMessage(`{"content":[{"type":"text","text":"Successfully replaced 1 block(s) in foo.ts."}],"details":{"diff":"+1 x","patch":"--- a/foo.ts\n+++ b/foo.ts\n@@ -1 +1 @@\n-a\n+b\n"}}`)
	args := json.RawMessage(`{"path":"foo.ts","edits":[{"oldText":"a","newText":"b"}]}`)
	raw := editFileDiff(result, args)
	if raw == nil {
		t.Fatal("metadata = nil, want file diff")
	}
	meta := agents.ParseToolResultMetadata(raw)
	first := meta.FirstChangedFile()
	if first == nil || first.File != "foo.ts" || first.Additions != 1 || first.Deletions != 1 {
		t.Errorf("diff = %+v, want foo.ts 1/1", first)
	}
	if first.Patch == "" {
		t.Error("patch must be preserved for the diff body")
	}
}

func TestEditFileDiffNoDetails(t *testing.T) {
	raw := editFileDiff(json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`), nil)
	if raw != nil {
		t.Errorf("metadata = %s, want nil", raw)
	}
}

func TestEditFileDiffEmptyPatch(t *testing.T) {
	result := json.RawMessage(`{"content":[],"details":{"diff":"","patch":""}}`)
	if raw := editFileDiff(result, nil); raw != nil {
		t.Errorf("metadata = %s, want nil", raw)
	}
}
