package opencode

import "testing"

func TestHistoryImagesUseMessageScopedIDs(t *testing.T) {
	message := Message{
		ID:   "msg_abc",
		Type: "user",
		Text: "看图",
		Files: []HistoryFileAttachment{
			{Mime: "image/png", Data: "aGVsbG8="},
			{Mime: "text/plain", Data: "not-an-image"},
			{Mime: "image/jpeg", Data: "d29ybGQ="},
		},
	}

	images := historyImages(message)
	if len(images) != 2 {
		t.Fatalf("len(images) = %d, want 2", len(images))
	}
	if images[0].ID != "msg_abc:1" || images[1].ID != "msg_abc:3" {
		t.Fatalf("ids = %q, %q, want message-scoped file indexes", images[0].ID, images[1].ID)
	}
	if images[0].Size != 5 {
		t.Errorf("Size = %d, want 5", images[0].Size)
	}
}
