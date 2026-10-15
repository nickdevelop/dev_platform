//go:build unix

package unix_test

import (
	"path/filepath"
	"testing"

	"devplatform/internal/core/platform"
)

// TestUnixDirsFollowXDG pins the XDG precedence at the port: explicit XDG
// variables win, otherwise the home-relative defaults apply.
func TestUnixDirsFollowXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))

	dirs := platform.New().Dirs()
	if got, _ := dirs.HomeDir(); got != home {
		t.Errorf("HomeDir() = %q, want $HOME %q", got, home)
	}
	if got, _ := dirs.ConfigDir(); got != filepath.Join(home, "xdg-config") {
		t.Errorf("ConfigDir() = %q, want $XDG_CONFIG_HOME", got)
	}
	if got, _ := dirs.DataDir(); got != filepath.Join(home, "xdg-data") {
		t.Errorf("DataDir() = %q, want $XDG_DATA_HOME", got)
	}
	if got, _ := dirs.StateDir(); got != filepath.Join(home, "xdg-state") {
		t.Errorf("StateDir() = %q, want $XDG_STATE_HOME", got)
	}
	if got := dirs.TempDir(); got == "" {
		t.Error("TempDir() is empty")
	}
}

// TestUnixDirsFallBackToHome pins the ~/. defaults when no XDG variable is
// set, and the error when no home exists either.
func TestUnixDirsFallBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	dirs := platform.New().Dirs()
	if got := mustDir(t, dirs.ConfigDir); got != filepath.Join(home, ".config") {
		t.Errorf("ConfigDir() = %q, want ~/.config", got)
	}
	if got := mustDir(t, dirs.DataDir); got != filepath.Join(home, ".local", "share") {
		t.Errorf("DataDir() = %q, want ~/.local/share", got)
	}
	if got := mustDir(t, dirs.StateDir); got != filepath.Join(home, ".local", "state") {
		t.Errorf("StateDir() = %q, want ~/.local/state", got)
	}

	t.Setenv("HOME", "")
	if _, err := dirs.ConfigDir(); err == nil {
		t.Error("ConfigDir() without HOME succeeded, want an error")
	}
	if _, err := dirs.DataDir(); err == nil {
		t.Error("DataDir() without HOME succeeded, want an error")
	}
	if _, err := dirs.StateDir(); err == nil {
		t.Error("StateDir() without HOME succeeded, want an error")
	}
	if _, err := dirs.HomeDir(); err == nil {
		t.Error("HomeDir() without HOME succeeded, want an error")
	}
}

func mustDir(t *testing.T, fn func() (string, error)) string {
	t.Helper()
	dir, err := fn()
	if err != nil {
		t.Fatalf("dir: %v", err)
	}
	return dir
}
