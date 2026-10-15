//go:build windows

package windows_test

import (
	"os"
	"testing"

	"devplatform/internal/core/platform"
)

// TestWindowsReadSecretFallsBackToStdin verifies piped input works where no
// console exists: the secret is read as-is without echo management.
func TestWindowsReadSecretFallsBackToStdin(t *testing.T) {
	if platform.New().Terminal().IsInteractive() {
		t.Skip("test needs non-interactive stdin")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	go func() {
		_, _ = w.Write([]byte("harness-secret\r\n"))
		_ = w.Close()
	}()

	got, err := platform.New().Terminal().ReadSecret("password: ")
	if err != nil {
		t.Fatalf("ReadSecret: %v", err)
	}
	if got != "harness-secret" {
		t.Fatalf("ReadSecret() = %q, want the piped line without CRLF", got)
	}
}
