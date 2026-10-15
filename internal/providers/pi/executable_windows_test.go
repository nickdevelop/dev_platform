//go:build windows

package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixturePrefix builds a fake npm global prefix with a space in the path:
// <tmp>/Some User/npm/pi.cmd, the package manifest pointing bin.pi at a JS
// bundle, the bundle itself, and a fake node.exe standing in for Node.js.
func fixturePrefix(t *testing.T) string {
	t.Helper()
	prefix := filepath.Join(t.TempDir(), "Some User", "npm")
	pkgDir := filepath.Join(prefix, "node_modules", "@earendil-works", "pi-coding-agent")
	binDir := filepath.Join(pkgDir, "dist", "bundle")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	shim := "@echo off\r\nnode \"%~dp0\\node_modules\\@earendil-works\\pi-coding-agent\\dist\\bundle\\cli.js\" %*\r\n"
	if err := os.WriteFile(filepath.Join(prefix, "pi.cmd"), []byte(shim), 0o644); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	manifest := `{"name":"@earendil-works/pi-coding-agent","version":"1.2.0","bin":{"pi":"dist/bundle/cli.js"}}`
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "cli.js"), []byte("console.log(1);\n"), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	// A stand-in node.exe: resolution only needs the file to exist with an
	// executable extension, never to run it.
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	raw, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "node.exe"), raw, 0o644); err != nil {
		t.Fatalf("write node: %v", err)
	}
	t.Setenv("PATH", prefix)
	return prefix
}

// TestResolvePiRuntimeFromManifest verifies the manifest-driven step: the
// entry comes from package.json bin.pi (never hardcoded), run with node.
func TestResolvePiRuntimeFromManifest(t *testing.T) {
	prefix := fixturePrefix(t)
	wantNode := filepath.Join(prefix, "node.exe")
	wantEntry := filepath.Join(prefix, "node_modules", "@earendil-works", "pi-coding-agent", "dist", "bundle", "cli.js")

	argv, display, err := resolvePiBinary()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(argv) != 2 || argv[0] != wantNode || argv[1] != wantEntry {
		t.Fatalf("argv = %q, want [%q %q]", argv, wantNode, wantEntry)
	}
	if display != wantEntry {
		t.Fatalf("display = %q, want entry %q", display, wantEntry)
	}
}

// TestResolvePiSpecPrefixesNode verifies a launch spec becomes node + entry.
func TestResolvePiSpecPrefixesNode(t *testing.T) {
	fixturePrefix(t)
	adapter := &Adapter{}
	spec, err := adapter.resolveSpec(processSpec{executable: "pi", args: []string{"--mode", "json"}, dir: t.TempDir()})
	if err != nil {
		t.Fatalf("resolveSpec: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(spec.executable), "node.exe") {
		t.Fatalf("executable = %q, want node", spec.executable)
	}
	if len(spec.args) < 1 || !strings.HasSuffix(spec.args[0], "cli.js") {
		t.Fatalf("args = %q, want the JS entry first", spec.args)
	}
	if spec.args[len(spec.args)-2] != "--mode" || spec.args[len(spec.args)-1] != "json" {
		t.Fatalf("args = %q, want original args preserved", spec.args)
	}
}

// TestResolvePiFallsBackToShim verifies the last resort: with no package on
// disk and no npm to ask, the shim runs via cmd.exe.
func TestResolvePiFallsBackToShim(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "Some User", "npm")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatalf("mkdir prefix: %v", err)
	}
	shim := filepath.Join(prefix, "pi.cmd")
	if err := os.WriteFile(shim, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	t.Setenv("PATH", prefix)

	argv, display, err := resolvePiBinary()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(argv) != 3 || argv[0] != "cmd" || argv[1] != "/C" || argv[2] != shim {
		t.Fatalf("argv = %q, want [cmd /C %q]", argv, shim)
	}
	if display != shim {
		t.Fatalf("display = %q, want shim %q", display, shim)
	}
}

// TestDescriptorResolverRegistered pins the settings-page wiring: the
// descriptor carries the npm resolver on Windows.
func TestDescriptorResolverRegistered(t *testing.T) {
	if Descriptor.ResolveExecutable == nil {
		t.Fatal("Descriptor.ResolveExecutable is nil on Windows")
	}
}
