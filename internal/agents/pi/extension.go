package pi

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// approvalExtensionSource is the bundled pi approval extension. pi's core does
// not request tool approval, so RPC-mode tool confirmation only exists when an
// extension asks for it; the platform ships one so the capability is real out
// of the box.
//
//go:embed extension/approval.ts
var approvalExtensionSource []byte

// ApprovalExtensionName is the file name the bundled approval extension is
// written under.
const ApprovalExtensionName = "pi-platform-approval.ts"

// EnsureApprovalExtension writes the bundled approval extension into dir and
// returns its path, so it can be passed to pi as `--extension`. It is
// idempotent: an already up-to-date file is left in place.
func EnsureApprovalExtension(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("pi: extension directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("pi: create extension directory %q: %w", dir, err)
	}
	path := filepath.Join(dir, ApprovalExtensionName)
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, approvalExtensionSource) {
		return path, nil
	}
	if err := os.WriteFile(path, approvalExtensionSource, 0o644); err != nil {
		return "", fmt.Errorf("pi: write approval extension %q: %w", path, err)
	}
	return path, nil
}
