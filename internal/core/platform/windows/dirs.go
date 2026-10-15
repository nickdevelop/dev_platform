//go:build windows

package windows

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dirs resolves Windows directories from the known folders, with the same
// XDG_STATE_HOME override rule as unix so the opencode service registration
// lands in the same relative place on every platform.
type Dirs struct{}

// HomeDir is the current user's profile directory (%USERPROFILE%).
func (Dirs) HomeDir() (string, error) {
	return os.UserHomeDir()
}

// ConfigDir is the roaming application data directory (%APPDATA%).
func (Dirs) ConfigDir() (string, error) {
	return os.UserConfigDir()
}

// DataDir is the local application data directory (%LOCALAPPDATA%).
func (Dirs) DataDir() (string, error) {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("platform: %%LOCALAPPDATA%% is not defined")
}

// StateDir is $XDG_STATE_HOME when set, otherwise
// %USERPROFILE%\.local\state — the same rule as unix, so the service
// registration resolves to <state>\opencode\service.json everywhere.
func (Dirs) StateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

// TempDir is the directory for temporary files.
func (Dirs) TempDir() string {
	return os.TempDir()
}
