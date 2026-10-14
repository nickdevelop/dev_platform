package pi

import (
	"encoding/json"
	"strings"

	"devplatform/internal/agents"
)

// editDetails mirrors pi's EditToolDetails: a display diff plus a standard
// unified patch. pi ships no line counts, so the patch is counted locally.
type editDetails struct {
	Diff  string `json:"diff"`
	Patch string `json:"patch"`
}

// editResultEnvelope mirrors pi's tool_execution_end result body: content
// blocks plus optional details. Unknown shapes are ignored.
type editResultEnvelope struct {
	Content json.RawMessage `json:"content"`
	Details *editDetails    `json:"details"`
}

// editArgs mirrors the subset of pi's edit input the workbench needs: the
// file path for the diff header.
type editArgs struct {
	Path     string `json:"path"`
	FilePath string `json:"file_path"`
	File     string `json:"file"`
}

func (a editArgs) file() string {
	for _, v := range []string{a.Path, a.FilePath, a.File} {
		if v != "" {
			return v
		}
	}
	return ""
}

// editFileDiff normalizes a pi edit result into the shared FileDiff metadata.
// It prefers the unified patch (which the workbench renders); the custom
// display diff only feeds the counts when no patch is present. It returns nil
// when the result carries no usable diff.
func editFileDiff(resultRaw, argsRaw json.RawMessage) json.RawMessage {
	if len(resultRaw) == 0 {
		return nil
	}
	var env editResultEnvelope
	if err := json.Unmarshal(resultRaw, &env); err != nil {
		return nil
	}
	if env.Details == nil {
		return nil
	}
	var args editArgs
	if len(argsRaw) > 0 {
		_ = json.Unmarshal(argsRaw, &args)
	}
	file := args.file()
	if env.Details.Patch != "" {
		additions, deletions := agents.CountPatchLines(env.Details.Patch)
		if additions == 0 && deletions == 0 {
			return nil
		}
		return agents.MarshalToolResultMetadata([]agents.FileDiff{
			{File: file, Patch: env.Details.Patch, Additions: additions, Deletions: deletions, Status: "modified"},
		})
	}
	if env.Details.Diff != "" {
		additions, deletions := countPiDisplayDiff(env.Details.Diff)
		if additions == 0 && deletions == 0 {
			return nil
		}
		return agents.MarshalToolResultMetadata([]agents.FileDiff{
			{File: file, Additions: additions, Deletions: deletions, Status: "modified"},
		})
	}
	return nil
}

// countPiDisplayDiff counts pi's custom display diff lines ("+12 ..."/"-12
// ..."), skipping context and separator lines.
func countPiDisplayDiff(diff string) (additions, deletions int) {
	for line := range strings.Lines(diff) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "...") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+") || strings.HasPrefix(trimmed, "+"):
			additions++
		case strings.HasPrefix(line, "-") || strings.HasPrefix(trimmed, "-"):
			deletions++
		}
	}
	return additions, deletions
}
