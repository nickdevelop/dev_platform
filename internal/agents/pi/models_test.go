package pi

import (
	"reflect"
	"testing"

	"devplatform/internal/agents"
)

// TestParseModelListHandlesCRLF verifies Windows CRLF output parses exactly
// like LF output.
func TestParseModelListHandlesCRLF(t *testing.T) {
	lf := "provider           model                            context  max-out\n" +
		"occ-command-code   deepseek/deepseek-v4.1-flash     1M       32K\n"
	crlf := "provider           model                            context  max-out\r\n" +
		"occ-command-code   deepseek/deepseek-v4.1-flash     1M       32K\r\n"

	want := []agents.ModelInfo{
		{ID: "occ-command-code/deepseek/deepseek-v4.1-flash", Name: "deepseek/deepseek-v4.1-flash", Provider: "occ-command-code"},
	}
	if got := parseModelList(lf); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModelList(LF) = %+v, want %+v", got, want)
	}
	if got := parseModelList(crlf); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModelList(CRLF) = %+v, want %+v", got, want)
	}
}
