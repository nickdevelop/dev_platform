package agents

import (
	"encoding/json"
	"strings"
)

// FileDiff is the backend-agnostic edit summary the workbench renders
// desktop-style: a unified patch plus its line counts. opencode already
// produces it as metadata.files[0]; pi's details.{patch,diff} are
// normalized into the same shape so one frontend component serves both.
type FileDiff struct {
	File      string `json:"file"`
	Patch     string `json:"patch,omitempty"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status,omitempty"`
}

// Changed reports whether the diff carries visible line changes, mirroring
// opencode desktop's changedFileDiff gate (additions>0||deletions>0).
func (d FileDiff) Changed() bool {
	return d.Additions > 0 || d.Deletions > 0
}

// CountPatchLines counts unified-patch +/- lines, skipping the ---/+++
// file headers. Only headers before the first hunk are skipped, so hunk
// content lines that happen to start with ---/+++ still count.
func CountPatchLines(patch string) (additions, deletions int) {
	seenHunk := false
	for line := range strings.Lines(patch) {
		if strings.HasPrefix(line, "@@") {
			seenHunk = true
		}
		if !seenHunk && (strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---")) {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
		} else if strings.HasPrefix(line, "-") {
			deletions++
		}
	}
	return additions, deletions
}

// NormalizeFileDiff builds a FileDiff from a file name, an optional unified
// patch and optional precomputed counts (opencode). When counts are negative
// they are derived from the patch; when the patch is empty the diff is
// unchanged and Changed() is false.
func NormalizeFileDiff(file, patch string, additions, deletions int, status string) FileDiff {
	if additions < 0 || deletions < 0 {
		additions, deletions = CountPatchLines(patch)
	}
	if status == "" {
		status = "modified"
	}
	return FileDiff{File: file, Patch: patch, Additions: additions, Deletions: deletions, Status: status}
}

// ToolResultMetadata is the typed companion of ToolResultPayload.Output. It
// carries edit summaries; unknown backends/tools leave it absent.
type ToolResultMetadata struct {
	Files []FileDiff `json:"files,omitempty"`
}

// MarshalToolResultMetadata encodes files into the bounded metadata body, or
// returns nil when there is nothing to carry.
func MarshalToolResultMetadata(files []FileDiff) json.RawMessage {
	kept := files[:0]
	for _, f := range files {
		if f.File == "" {
			continue
		}
		kept = append(kept, f)
	}
	if len(kept) == 0 {
		return nil
	}
	raw, err := json.Marshal(ToolResultMetadata{Files: kept})
	if err != nil {
		return nil
	}
	if len(raw) > MaxRawEventBytes {
		return nil
	}
	return raw
}

// ParseToolResultMetadata decodes the metadata body, returning nil on any
// shape error so callers fall back to plain text output.
func ParseToolResultMetadata(raw json.RawMessage) *ToolResultMetadata {
	if len(raw) == 0 {
		return nil
	}
	var meta ToolResultMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	return &meta
}

// FirstChangedFile returns the first changed file diff, or nil when none
// qualifies for the desktop-style badge.
func (m *ToolResultMetadata) FirstChangedFile() *FileDiff {
	if m == nil {
		return nil
	}
	for i := range m.Files {
		if m.Files[i].Changed() {
			return &m.Files[i]
		}
	}
	return nil
}
