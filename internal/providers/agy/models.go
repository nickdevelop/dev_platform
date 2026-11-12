package agy

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"

	"devplatform/internal/core/agents"
)

var (
	ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x1b]*\x1b\\|\x1b\][^\x07]*\x07`)
	spinnerRegex    = regexp.MustCompile(`^[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏\s]+Fetching available models\.{0,3}`)
)

// ListModels executes `agy models` and returns normalized ModelInfo records.
func (a *Adapter) ListModels(ctx context.Context, directory string) ([]agents.ModelInfo, error) {
	if a.listModelsOverride != nil {
		return a.listModelsOverride(ctx, directory)
	}

	execPath := a.execPath()
	cmd := exec.CommandContext(ctx, execPath, "models")
	if directory != "" {
		cmd.Dir = directory
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return ParseModelsOutput(stdout.String()), nil
}

// ParseModelsOutput cleans up raw stdout of `agy models` and parses ModelInfo items.
func ParseModelsOutput(raw string) []agents.ModelInfo {
	// Strip ANSI escapes
	clean := ansiEscapeRegex.ReplaceAllString(raw, "")

	lines := strings.Split(clean, "\n")
	var models []agents.ModelInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// If line has spinner text or help text, skip
		if strings.Contains(line, "Fetching available models") ||
			strings.HasPrefix(line, "Usage:") ||
			strings.HasPrefix(line, "Flags:") ||
			strings.HasPrefix(line, "List available models") ||
			strings.HasPrefix(line, "-h") ||
			strings.HasPrefix(line, "--help") {
			continue
		}

		// Line format is: "<model_id>     <display_name>"
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		modelID := parts[0]
		displayName := strings.TrimSpace(line[len(modelID):])
		if displayName == "" {
			displayName = modelID
		}

		provider := "agy"
		idLower := strings.ToLower(modelID)
		if strings.Contains(idLower, "gemini") {
			provider = "google"
		} else if strings.Contains(idLower, "claude") {
			provider = "anthropic"
		} else if strings.Contains(idLower, "gpt") {
			provider = "openai"
		}

		supportsImages := strings.Contains(idLower, "gemini") || strings.Contains(idLower, "claude")

		models = append(models, agents.ModelInfo{
			ID:             modelID,
			Name:           displayName,
			Provider:       provider,
			SupportsImages: supportsImages,
			Enabled:        true,
		})
	}

	return models
}
