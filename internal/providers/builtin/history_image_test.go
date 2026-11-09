package builtin

import (
	"encoding/json"
	"testing"

	"devplatform/internal/core/agents"
)

func TestBuiltinHistoryImagesUseRecordScopedIDs(t *testing.T) {
	payload, err := json.Marshal(userRecord{Images: []agentsImage{
		{ID: "img_1", Name: "chart.png", MediaType: "image/png", Data: "aGVsbG8="},
		{ID: "img_2", Name: "empty.png", MediaType: "image/png"},
	}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	images := builtinHistoryImages(TranscriptRecord{Seq: 7, Kind: TranscriptUser, Payload: payload})
	if len(images) != 1 {
		t.Fatalf("len(images) = %d, want 1 (empty bodies are skipped)", len(images))
	}
	if images[0].ID != "7:img_1" || images[0].Name != "chart.png" {
		t.Fatalf("image = %+v, want the record-scoped id", images[0])
	}
	if images[0].Size != 5 {
		t.Errorf("Size = %d, want 5", images[0].Size)
	}
}

var _ agents.HistoryImageReader = (*Adapter)(nil)
