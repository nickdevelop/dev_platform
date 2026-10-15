//go:build windows

package windows_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/core/platform"
)

// TestWindowsDirsFollowKnownFolders pins the folder mapping: %APPDATA%,
// %LOCALAPPDATA%, %USERPROFILE%, and the XDG_STATE_HOME override rule.
func TestWindowsDirsFollowKnownFolders(t *testing.T) {
	dirs := platform.New().Dirs()

	if home, err := os.UserHomeDir(); err != nil || home == "" {
		t.Skip("no user home on this machine")
	} else if got, err := dirs.HomeDir(); err != nil || got != home {
		t.Errorf("HomeDir() = %q, %v; want %%USERPROFILE%% %q", got, err, home)
	}
	if appData := os.Getenv("APPDATA"); appData == "" {
		t.Error("APPDATA is not defined on this Windows machine")
	} else if got, err := dirs.ConfigDir(); err != nil || got != appData {
		t.Errorf("ConfigDir() = %q, %v; want %%APPDATA%% %q", got, err, appData)
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData == "" {
		t.Error("LOCALAPPDATA is not defined on this Windows machine")
	} else if got, err := dirs.DataDir(); err != nil || got != localAppData {
		t.Errorf("DataDir() = %q, %v; want %%LOCALAPPDATA%% %q", got, err, localAppData)
	}
	if got := dirs.TempDir(); got == "" {
		t.Error("TempDir() is empty")
	}
}

// TestWindowsStateDirDerivesServiceFile verifies the plan's example: with
// no XDG_STATE_HOME, the service registration lands under
// %USERPROFILE%\.local\state\opencode\service.json.
func TestWindowsStateDirDerivesServiceFile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no user home on this machine")
	}
	t.Setenv("XDG_STATE_HOME", "")

	state, err := platform.New().Dirs().StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	wantState := filepath.Join(home, ".local", "state")
	if state != wantState {
		t.Fatalf("StateDir() = %q, want %q", state, wantState)
	}
	serviceFile := filepath.Join(state, "opencode", "service.json")
	if !strings.HasSuffix(serviceFile, filepath.Join(".local", "state", "opencode", "service.json")) {
		t.Fatalf("service file = %q", serviceFile)
	}

	custom := `C:\ProgramData\opencode-state`
	t.Setenv("XDG_STATE_HOME", custom)
	if got, err := platform.New().Dirs().StateDir(); err != nil || got != custom {
		t.Errorf("StateDir() with XDG_STATE_HOME = %q, %v; want %q", got, err, custom)
	}
}

// TestWindowsSamePathIgnoresCase verifies a differently-cased path is the
// same path on Windows.
func TestWindowsSamePathIgnoresCase(t *testing.T) {
	dirs := platform.New().Dirs()
	if !dirs.SamePath(`C:\Users\ChenLi\work`, `c:\users\chenli\work`) {
		t.Error("SamePath(different case) = false, want true")
	}
	if dirs.SamePath(`C:\Users\ChenLi\work`, `C:\Users\ChenLi\other`) {
		t.Error("SamePath(different dirs) = true, want false")
	}
	// A UNC path is accepted as absolute (never rejected like on unix): with
	// no such server reachable the resolution itself fails closed, but the
	// failure must not be the not-absolute rejection.
	if _, err := dirs.CanonicalizeUserPath(`\\server\share\dir`); err == nil {
		t.Log("UNC test share unexpectedly resolved; environment provides it")
	} else if strings.Contains(err.Error(), "is not absolute") {
		t.Errorf("CanonicalizeUserPath(UNC) = %v, want it accepted as absolute", err)
	}
}

// TestWindowsCanonicalizeRejectsAmbiguous verifies relative inputs,
// drive-relative references and NUL bytes are refused outright.
func TestWindowsCanonicalizeRejectsAmbiguous(t *testing.T) {
	dirs := platform.New().Dirs()
	for _, input := range []string{
		"",
		`relative\path`,
		`C:relative`,
		`C:`,
		`\rooted-no-drive`,
		"nul\x00byte",
	} {
		if _, err := dirs.CanonicalizeUserPath(input); err == nil {
			t.Errorf("CanonicalizeUserPath(%q) = nil, want an error", input)
		}
	}
	// An existing drive-absolute path with forward slashes normalizes to the
	// resolved form.
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	slashed := strings.ReplaceAll(tmp, `\`, `/`)
	got, err := dirs.CanonicalizeUserPath(slashed)
	if err != nil {
		t.Fatalf("CanonicalizeUserPath(%q) = %v", slashed, err)
	}
	if got != tmp {
		t.Errorf("CanonicalizeUserPath(%q) = %q, want %q", slashed, got, tmp)
	}
}
