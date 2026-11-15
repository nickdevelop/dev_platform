package agy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// buildChildArgs returns the CLI arguments for an agy ACP stdio session child.
func buildChildArgs(permissionMode, dir string) []string {
	args := []string{"acp", "--stdio"}
	if permissionMode == PermissionModeAuto {
		args = append(args, "--dangerously-skip-permissions")
	}
	if dir != "" {
		cleanDir := filepath.Clean(dir)
		args = append(args, "--add-dir", cleanDir)
	}
	return args
}

// EnsureWorkspaceTrusted automatically adds directory to ~/.gemini/antigravity-cli/settings.json.
func EnsureWorkspaceTrusted(dir string) error {
	if dir == "" {
		return nil
	}
	cleanDir := filepath.Clean(dir)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settingsDir := filepath.Join(home, ".gemini", "antigravity-cli")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	var settingsMap map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(data, &settingsMap)
	}
	if settingsMap == nil {
		settingsMap = make(map[string]any)
	}

	trusted, _ := settingsMap["trustedWorkspaces"].([]any)
	for _, entry := range trusted {
		if s, ok := entry.(string); ok && filepath.Clean(s) == cleanDir {
			return nil
		}
	}

	trusted = append(trusted, cleanDir)
	settingsMap["trustedWorkspaces"] = trusted

	data, err := json.MarshalIndent(settingsMap, "", "  ")
	if err != nil {
		return err
	}

	_ = os.MkdirAll(settingsDir, 0700)
	return os.WriteFile(settingsPath, data, 0600)
}

// resolveExecutable looks up the binary on PATH or returns the specified executable.
func resolveExecutable(custom string) string {
	if custom != "" {
		return custom
	}
	if path, err := exec.LookPath("agy"); err == nil {
		return path
	}
	return "agy"
}
