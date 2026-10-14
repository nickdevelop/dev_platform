package opencode

import (
	"encoding/json"
	"testing"

	"devplatform/internal/agents"
)

func TestToolSuccessCarriesFileDiffMetadata(t *testing.T) {
	raw := `{"sessionID":"ses_1","assistantMessageID":"msg_1","id":"tool_1","content":[{"type":"text","text":"Edited foo.ts (1 replacement)"}],"metadata":{"files":[{"file":"foo.ts","patch":"--- a/foo.ts\n+++ b/foo.ts\n@@ -1 +1 @@\n-a\n+b\n","additions":1,"deletions":1,"status":"modified"}]}}`
	data := decodeEventData(json.RawMessage(raw))
	meta := boundedMetadata(data.Metadata)
	if meta == nil {
		t.Fatal("metadata = nil, want file diff body")
	}
	parsed := agents.ParseToolResultMetadata(meta)
	first := parsed.FirstChangedFile()
	if first == nil || first.File != "foo.ts" || first.Additions != 1 || first.Deletions != 1 {
		t.Errorf("diff = %+v, want foo.ts 1/1", first)
	}
}

func TestBoundedMetadataDropsNonFileBodies(t *testing.T) {
	if boundedMetadata(json.RawMessage(`{"other":1}`)) != nil {
		t.Error("non-file metadata must be dropped")
	}
	if boundedMetadata(nil) != nil {
		t.Error("empty metadata must be nil")
	}
}
