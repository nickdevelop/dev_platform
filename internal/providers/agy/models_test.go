package agy

import (
	"testing"
)

func TestParseModelsOutput(t *testing.T) {
	raw := `⠋ Fetching available models...⠙ Fetching available models...
gemini-3.8-flash-high     Gemini 3.8 Flash (High)
gemini-3.8-flash-medium   Gemini 3.8 Flash (Medium)
claude-sonnet-4-6         Claude Sonnet 4.6 (Thinking)
gpt-oss-120b-medium       GPT-OSS 120B (Medium)
`

	models := ParseModelsOutput(raw)
	if len(models) != 4 {
		t.Fatalf("expected 4 models, got %d", len(models))
	}

	if models[0].ID != "gemini-3.8-flash-high" || models[0].Name != "Gemini 3.8 Flash (High)" || models[0].Provider != "google" {
		t.Errorf("unexpected model[0]: %+v", models[0])
	}
	if !models[0].SupportsImages {
		t.Errorf("expected gemini to support images")
	}

	if models[2].ID != "claude-sonnet-4-6" || models[2].Provider != "anthropic" {
		t.Errorf("unexpected model[2]: %+v", models[2])
	}

	if models[3].ID != "gpt-oss-120b-medium" || models[3].Provider != "openai" {
		t.Errorf("unexpected model[3]: %+v", models[3])
	}
}
