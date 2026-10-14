//go:build windows

package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixturePrefix builds a fake npm global prefix with a space in the path:
// <tmp>/Some User/npm/opencode.cmd plus the package tree underneath.
func fixturePrefix(t *testing.T) string {
	t.Helper()
	prefix := filepath.Join(t.TempDir(), "Some User", "npm")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatalf("mkdir prefix: %v", err)
	}
	shim := "@echo off\r\nnode \"%~dp0\\node_modules\\@opencode\\cli\\bin\\opencode.exe\" %*\r\n"
	if err := os.WriteFile(filepath.Join(prefix, "opencode.cmd"), []byte(shim), 0o644); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	t.Setenv("PATH", prefix)
	return prefix
}

// materializeExe copies the test binary (a real PE with an MZ header) to
// path, standing in for an installed native opencode.exe.
func materializeExe(t *testing.T, path string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	raw, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write exe: %v", err)
	}
}

// TestResolveOpencodeExeFromPackage verifies the primary step: the package
// bin/opencode.exe under the shim prefix wins.
func TestResolveOpencodeExeFromPackage(t *testing.T) {
	prefix := fixturePrefix(t)
	want := filepath.Join(prefix, "node_modules", "@opencode", "cli", "bin", "opencode.exe")
	materializeExe(t, want)

	got, err := resolveOpencodePath("opencode")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != want {
		t.Fatalf("resolve = %q, want %q", got, want)
	}
}

// TestResolveOpencodeExeFromPlatformPack verifies the fallback to the
// platform optional dependency when the package binary is not native
// (e.g. a JS placeholder left by an incomplete postinstall).
func TestResolveOpencodeExeFromPlatformPack(t *testing.T) {
	prefix := fixturePrefix(t)
	if err := os.MkdirAll(filepath.Join(prefix, "node_modules", "@opencode", "cli", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	placeholder := filepath.Join(prefix, "node_modules", "@opencode", "cli", "bin", "opencode.exe")
	if err := os.WriteFile(placeholder, []byte("#!/usr/bin/env node\n"), 0o644); err != nil {
		t.Fatalf("write placeholder: %v", err)
	}
	want := filepath.Join(prefix, "node_modules", "@opencode", "cli-windows-x64", "bin", "opencode.exe")
	materializeExe(t, want)

	got, err := resolveOpencodePath("opencode")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != want {
		t.Fatalf("resolve = %q, want platform pack %q", got, want)
	}
}

// TestResolveOpencodeFallsBackToShim verifies the last resort: with no
// package on disk and no npm to ask, the shim path itself is returned for
// the cmd.exe wrapper.
func TestResolveOpencodeFallsBackToShim(t *testing.T) {
	prefix := fixturePrefix(t)
	want := filepath.Join(prefix, "opencode.cmd")

	got, err := resolveOpencodePath("opencode")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != want {
		t.Fatalf("resolve = %q, want shim %q", got, want)
	}
	if !strings.HasSuffix(strings.ToLower(got), ".cmd") {
		t.Fatalf("resolve = %q, want a .cmd shim for the cmd.exe wrapper", got)
	}
}

// TestDescriptorResolverRegistered pins the settings-page wiring: the
// descriptor carries the npm resolver on Windows.
func TestDescriptorResolverRegistered(t *testing.T) {
	if Descriptor.ResolveExecutable == nil {
		t.Fatal("Descriptor.ResolveExecutable is nil on Windows")
	}
}
