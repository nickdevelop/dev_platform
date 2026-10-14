//go:build windows

package windows_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/platform"
)

// TestWindowsPermsDegraded pins the accepted degradation: nothing is
// enforced, directories are still created, and a missing file still fails
// like the unix Chmod would.
func TestWindowsPermsDegraded(t *testing.T) {
	perms := platform.New().Perms()
	if perms.Enforced() {
		t.Fatal("Enforced() = true, want false on Windows")
	}
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	if err := perms.PrivateDir(dir); err != nil {
		t.Fatalf("PrivateDir: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("PrivateDir did not create %q: %v", dir, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.json")
	if err := perms.PrivateFile(missing); err == nil {
		t.Error("PrivateFile(missing) = nil, want an error")
	}
	present := filepath.Join(t.TempDir(), "present.json")
	if err := os.WriteFile(present, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := perms.PrivateFile(present); err != nil {
		t.Errorf("PrivateFile(existing) = %v, want nil", err)
	}
}

// TestWindowsEnvironmentDescribeWarns pins both startup warnings: the
// standing permission-degradation notice and the out-of-profile data
// directory warning.
func TestWindowsEnvironmentDescribeWarns(t *testing.T) {
	env := platform.New().Environment()
	if env == nil {
		t.Fatal("Environment() is nil")
	}
	if err := env.Check(); err != nil {
		t.Fatalf("Check() = %v, want nil: location risks warn, never block", err)
	}

	profile := os.Getenv("LOCALAPPDATA")
	if profile == "" {
		t.Skip("LOCALAPPDATA is not defined on this machine")
	}
	quiet := strings.Join(env.Describe(profile), "\n")
	if !strings.Contains(quiet, "not enforced") {
		t.Errorf("Describe(profile) = %q, want the standing degradation notice", quiet)
	}
	if strings.Contains(quiet, "outside the user profile") {
		t.Errorf("Describe(profile) = %q, want no location warning inside the profile", quiet)
	}

	shared := `C:\Shared\platform-data`
	warns := strings.Join(env.Describe(shared), "\n")
	if !strings.Contains(warns, "outside the user profile") || !strings.Contains(warns, shared) {
		t.Errorf("Describe(shared) = %q, want the location warning naming the directory", warns)
	}
}
