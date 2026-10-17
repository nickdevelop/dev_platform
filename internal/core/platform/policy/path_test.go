package policy

import (
	"testing"
)

func TestErrInvalidPath(t *testing.T) {
	if ErrInvalidPath == nil {
		t.Fatal("ErrInvalidPath must not be nil")
	}
	if got := ErrInvalidPath.Error(); got != "paths: invalid path" {
		t.Fatalf("ErrInvalidPath.Error() = %q, want %q", got, "paths: invalid path")
	}
}
