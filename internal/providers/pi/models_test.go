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
