package pi

import (
	"strconv"
	"strings"

	"devplatform/internal/core/agents"
)

// isMultimodalModel reports whether a model is known to accept image inputs.
func isMultimodalModel(modelID string) bool {
	id := strings.ToLower(modelID)
	indicators := []string{
		"gpt-4o", "gpt-4-turbo", "gpt-4-vision",
		"claude-3", "claude-3.5", "claude-3-5", "claude-3.7", "claude-3-7",
		"gemini", "vision", "vl", "pixtral", "llava", "omni",
	}
	for _, ind := range indicators {
		if strings.Contains(id, ind) {
			return true
		}
	}
	return false
}

// parseModelList reads the table printed by `pi --list-models`: a header row
// followed by one row per model, whose first two columns are provider and
// model, and whose third column is the context window (e.g. 200K, 1M). The
// model name may itself contain a slash, so it is taken as one field and the
// platform id is provider/model. A missing or unparseable context column
// leaves ContextLimit unset rather than reporting a wrong limit.
func parseModelList(output string) []agents.ModelInfo {
	models := make([]agents.ModelInfo, 0)
	for _, line := range strings.Split(output, "\n") {
		// Agent CLIs on Windows emit CRLF; strip the carriage return so
		// parsing matches the LF path exactly (as the JSONL event parser
		// and the opencode stream reader already do).
		fields := strings.Fields(strings.TrimSuffix(line, "\r"))
		if len(fields) < 2 || fields[0] == "provider" {
			continue
		}
		provider, model := fields[0], fields[1]
		modelID := provider + "/" + model
		entry := agents.ModelInfo{
			ID:             modelID,
			Name:           model,
			Provider:       provider,
			SupportsImages: isMultimodalModel(modelID),
		}
		if len(fields) >= 3 {
			if limit, ok := parseTokenCount(fields[2]); ok {
				entry.ContextLimit = &limit
			}
		}
		models = append(models, entry)
	}
	return models
}

// parseTokenCount reads pi's human-readable counts ("200K", "1.5M", "800").
// It reports false when the value is not a non-negative count.
func parseTokenCount(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	mult := 1
	digits := raw
	switch last := raw[len(raw)-1]; last {
	case 'K', 'k':
		mult = 1000
		digits = raw[:len(raw)-1]
	case 'M', 'm':
		mult = 1000000
		digits = raw[:len(raw)-1]
	}
	value, err := strconv.ParseFloat(digits, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return int(value * float64(mult)), true
}
