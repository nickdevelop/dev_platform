package deepseek

import (
	"context"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
)

// RawModelCatalog represents the payload returned by session/modelCatalog.
type RawModelCatalog struct {
	Default struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"default"`
	Groups []RawModelGroup `json:"groups"`
}

// RawModelGroup is one provider group in the catalog.
type RawModelGroup struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Models []RawModelEntry `json:"models"`
}

// RawModelEntry is one model entry in a group.
type RawModelEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ContextWindow int    `json:"contextWindow"`
	Reasoning     *struct {
		Efforts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"efforts"`
	} `json:"reasoning,omitempty"`
}

// DecodeModelCatalog parses a RawModelCatalog into a slice of platform ModelInfo.
func DecodeModelCatalog(catalog *RawModelCatalog) []agents.ModelInfo {
	if catalog == nil {
		return nil
	}
	var out []agents.ModelInfo
	for _, group := range catalog.Groups {
		for _, m := range group.Models {
			provider := group.ID
			if provider == "" {
				provider = "deepseek"
			}

			// Format model ID as "provider/model" if not already prefixed
			modelID := m.ID
			if !strings.HasPrefix(modelID, provider+"/") {
				modelID = fmt.Sprintf("%s/%s", provider, m.ID)
			}

			name := m.Name
			if name == "" {
				name = m.ID
			}

			var variants []string
			if m.Reasoning != nil {
				for _, effort := range m.Reasoning.Efforts {
					if effort.ID != "" {
						variants = append(variants, effort.ID)
					}
				}
			}

			var contextLimit *int
			if m.ContextWindow > 0 {
				cw := m.ContextWindow
				contextLimit = &cw
			} else if cw := resolveDefaultContextWindow(provider, m.ID); cw > 0 {
				contextLimit = &cw
			}

			out = append(out, agents.ModelInfo{
				ID:             modelID,
				Name:           name,
				Provider:       provider,
				Variants:       variants,
				ContextLimit:   contextLimit,
				SupportsImages: true,
			})
		}
	}
	return out
}

// resolveDefaultContextWindow derives a sensible context window limit in tokens
// when dsh's session/modelCatalog does not advertise one.
func resolveDefaultContextWindow(provider, modelID string) int {
	lower := strings.ToLower(modelID)
	switch {
	case strings.Contains(lower, "1m") || strings.Contains(lower, "vision"):
		return 1000000
	case strings.Contains(lower, "32k"):
		return 32000
	case strings.Contains(lower, "64k"):
		return 64000
	case strings.Contains(lower, "200k") || strings.Contains(lower, "256k"):
		return 200000
	default:
		// DeepSeek standard default context window (128K tokens)
		return 128000
	}
}

// FetchModels queries session/modelCatalog from the given client.
func FetchModels(ctx context.Context, client *Client) ([]agents.ModelInfo, error) {
	var catalog RawModelCatalog
	if err := client.Call(ctx, "session/modelCatalog", map[string]any{}, &catalog); err != nil {
		return nil, fmt.Errorf("fetch model catalog: %w", err)
	}
	return DecodeModelCatalog(&catalog), nil
}
