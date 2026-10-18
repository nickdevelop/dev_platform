package pi

import (
	"reflect"
	"testing"

	"devplatform/internal/core/agents"
)

// TestParseModelListHandlesCRLF verifies Windows CRLF output parses exactly
// like LF output.
func TestParseModelListHandlesCRLF(t *testing.T) {
	lf := "provider           model                            context  max-out\n" +
		"occ-command-code   deepseek/deepseek-v4.1-flash     1M       32K\n"
	crlf := "provider           model                            context  max-out\r\n" +
		"occ-command-code   deepseek/deepseek-v4.1-flash     1M       32K\r\n"

	million := 1000000
	want := []agents.ModelInfo{
		{ID: "occ-command-code/deepseek/deepseek-v4.1-flash", Name: "deepseek/deepseek-v4.1-flash", Provider: "occ-command-code", ContextLimit: &million},
	}
	if got := parseModelList(lf); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModelList(LF) = %+v, want %+v", got, want)
	}
	if got := parseModelList(crlf); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModelList(CRLF) = %+v, want %+v", got, want)
	}
}

// TestParseTokenCount pins the human-readable context column decoding.
func TestParseTokenCount(t *testing.T) {
	cases := map[string]int{
		"200K": 200000,
		"32k":  32000,
		"1M":   1000000,
		"1.5M": 1500000,
		"800":  800,
	}
	for raw, want := range cases {
		got, ok := parseTokenCount(raw)
		if !ok || got != want {
			t.Errorf("parseTokenCount(%q) = %d, %v; want %d, true", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"", "abc", "-5K", "K"} {
		if _, ok := parseTokenCount(raw); ok {
			t.Errorf("parseTokenCount(%q) succeeded, want failure", raw)
		}
	}
}

// TestParseModelListLeavesUnknownContextUnset verifies a row without a
// parseable context column still yields the model with no limit rather than
// a wrong one.
func TestParseModelListLeavesUnknownContextUnset(t *testing.T) {
	got := parseModelList("provider model\nacme widget\n")
	if len(got) != 1 {
		t.Fatalf("parseModelList = %+v, want one model", got)
	}
	if got[0].ContextLimit != nil {
		t.Errorf("ContextLimit = %v, want nil", *got[0].ContextLimit)
	}
}

// TestParseAvailableModelsMultiPart verifies models whose native IDs already contain
// slashes (such as deepseek/deepseek-v4.1-flash under provider occ-commandcode-o) are
// properly qualified with their provider name, yielding a 3-part ID.
func TestParseAvailableModelsMultiPart(t *testing.T) {
	raw := []byte(`{
		"models": [
			{
				"id": "ankki-code-plus",
				"name": "Ankki Code Plus",
				"provider": "ankki",
				"contextWindow": 204800,
				"input": ["text"]
			},
			{
				"id": "deepseek/deepseek-v4.1-flash",
				"name": "DeepSeek V4.1 Flash",
				"provider": "occ-commandcode-o",
				"contextWindow": 1000000,
				"input": ["text", "image"]
			},
			{
				"id": "occ-commandcode-o/z-ai/glm-5.3-flash",
				"name": "GLM-5.3 Flash",
				"provider": "occ-commandcode-o",
				"contextWindow": 1000000,
				"input": ["text", "image"]
			}
		]
	}`)

	models, err := parseAvailableModels(raw)
	if err != nil {
		t.Fatalf("parseAvailableModels() error = %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("models count = %d, want 3", len(models))
	}

	limit204k := 204800
	limit1m := 1000000

	want := []agents.ModelInfo{
		{
			ID:             "ankki/ankki-code-plus",
			Name:           "Ankki Code Plus",
			Provider:       "ankki",
			ContextLimit:   &limit204k,
			SupportsImages: false,
		},
		{
			ID:             "occ-commandcode-o/deepseek/deepseek-v4.1-flash",
			Name:           "DeepSeek V4.1 Flash",
			Provider:       "occ-commandcode-o",
			ContextLimit:   &limit1m,
			SupportsImages: true,
		},
		{
			ID:             "occ-commandcode-o/z-ai/glm-5.3-flash",
			Name:           "GLM-5.3 Flash",
			Provider:       "occ-commandcode-o",
			ContextLimit:   &limit1m,
			SupportsImages: true,
		},
	}

	if !reflect.DeepEqual(models, want) {
		t.Errorf("parseAvailableModels() =\n%+v\nwant:\n%+v", models, want)
	}
}

// TestRPCModelIDQualifiesProvider verifies rpcModelID prefixes provider when needed.
func TestRPCModelIDQualifiesProvider(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: `{"id": "gpt-1", "provider": "stub"}`,
			want:  "stub/gpt-1",
		},
		{
			input: `{"id": "deepseek/deepseek-v4.1-flash", "provider": "occ-commandcode-o"}`,
			want:  "occ-commandcode-o/deepseek/deepseek-v4.1-flash",
		},
		{
			input: `{"id": "occ-commandcode-o/deepseek/deepseek-v4.1-flash", "provider": "occ-commandcode-o"}`,
			want:  "occ-commandcode-o/deepseek/deepseek-v4.1-flash",
		},
		{
			input: `{"id": "bare-model"}`,
			want:  "bare-model",
		},
	}
	for _, tc := range cases {
		got := rpcModelID([]byte(tc.input))
		if got != tc.want {
			t.Errorf("rpcModelID(%s) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
