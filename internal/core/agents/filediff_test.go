package agents

import (
	"encoding/json"
	"testing"
)

func TestCountPatchLines(t *testing.T) {
	patch := "--- a/foo.ts\n+++ b/foo.ts\n@@ -1,2 +1,3 @@\n context\n-removed\n+added1\n+added2\n"
	additions, deletions := CountPatchLines(patch)
	if additions != 2 || deletions != 1 {
		t.Errorf("counts = %d/%d, want 2/1", additions, deletions)
	}
}

func TestCountPatchLinesSkipsHeaders(t *testing.T) {
	// Header-looking content inside hunks still counts; only the two file
	// header lines are skipped.
	patch := "--- a\n+++ b\n@@ -1 +1 @@\n--- not a header\n+++ not a header\n"
	additions, deletions := CountPatchLines(patch)
	if additions != 1 || deletions != 1 {
		t.Errorf("counts = %d/%d, want 1/1", additions, deletions)
	}
}

func TestNormalizeFileDiffDerivesCounts(t *testing.T) {
	d := NormalizeFileDiff("f.ts", "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\n", -1, -1, "")
	if d.Additions != 1 || d.Deletions != 1 || d.Status != "modified" {
		t.Errorf("diff = %+v, want 1/1/modified", d)
	}
}

func TestToolResultMetadataRoundTrip(t *testing.T) {
	raw := MarshalToolResultMetadata([]FileDiff{
		{File: "a.ts", Patch: "--- a\n+++ b\n", Additions: 2, Deletions: 1, Status: "modified"},
		{File: "", Additions: 9, Deletions: 9},
	})
	if raw == nil {
		t.Fatal("marshal = nil, want body")
	}
	meta := ParseToolResultMetadata(raw)
	if meta == nil || len(meta.Files) != 1 || meta.Files[0].File != "a.ts" {
		t.Fatalf("round trip = %s, want one file", raw)
	}
	if meta.FirstChangedFile() == nil {
		t.Error("FirstChangedFile = nil, want the changed file")
	}
}

func TestMarshalToolResultMetadataEmpty(t *testing.T) {
	if MarshalToolResultMetadata(nil) != nil {
		t.Error("empty marshal = non-nil, want nil")
	}
	if ParseToolResultMetadata(json.RawMessage(`{"files":"nope"}`)) != nil {
		t.Error("shape error must decode to nil so callers fall back to text")
	}
	if MarshalToolResultMetadata([]FileDiff{{}}) != nil {
		t.Error("nameless files must be dropped")
	}
}

func TestFirstChangedFileGate(t *testing.T) {
	meta := &ToolResultMetadata{Files: []FileDiff{{File: "a", Additions: 0, Deletions: 0}}}
	if meta.FirstChangedFile() != nil {
		t.Error("zero counts must not qualify for the badge")
	}
}
