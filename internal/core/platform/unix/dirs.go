//go:build unix

package unix

import (
	"os"
	"path/filepath"
)

// Dirs resolves unix directories from the XDG base directory specification
// with ~/. fallbacks. Home and Config delegate to the standard library so
// edge cases (empty variables, missing HOME) behave exactly as before.
type Dirs struct{}

// HomeDir is the current user's home directory ($HOME).
func (Dirs) HomeDir() (string, error) {
	return os.UserHomeDir()
}

// ConfigDir is $XDG_CONFIG_HOME or ~/.config.
func (Dirs) ConfigDir() (string, error) {
	return os.UserConfigDir()
}

// DataDir is $XDG_DATA_HOME or ~/.local/share.
func (Dirs) DataDir() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

// StateDir is $XDG_STATE_HOME or ~/.local/state.
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
