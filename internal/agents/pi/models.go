package pi

import (
	"strings"

	"devplatform/internal/agents"
)

// parseModelList reads the table printed by `pi --list-models`: a header row
// followed by one row per model, whose first two columns are provider and
// model. The model name may itself contain a slash, so it is taken as one
// field and the platform id is provider/model.
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
		models = append(models, agents.ModelInfo{
			ID:       provider + "/" + model,
			Name:     model,
			Provider: provider,
		})
	}
	return models
}
