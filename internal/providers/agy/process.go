package agy

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// processSpec specifies an agy process invocation.
type processSpec struct {
	executable string
	args       []string
	dir        string
	env        []string
}

// buildChildArgs returns the CLI arguments for an agy stream-json session child.
func buildChildArgs(sessionID, model, permissionMode, dir string) []string {
	args := []string{"--input-format", "stream-json", "--output-format", "stream-json"}
	if sessionID != "" {
		args = append(args, "--conversation", sessionID)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if permissionMode == PermissionModeAuto {
		args = append(args, "--dangerously-skip-permissions")
	}
	if dir != "" {
		cleanDir := filepath.Clean(dir)
		settingsJSON, err := json.Marshal(map[string]any{
			"trustedWorkspaces": []string{cleanDir},
		})
		if err == nil {
			args = append(args, "--settings", string(settingsJSON))
		}
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

	rawWS, exists := settingsMap["trustedWorkspaces"]
	var workspaces []string
	if exists {
		if list, ok := rawWS.([]any); ok {
			for _, item := range list {
				if s, ok := item.(string); ok {
					workspaces = append(workspaces, filepath.Clean(s))
				}
			}
		}
	}

	alreadyTrusted := false
	for _, ws := range workspaces {
		if ws == cleanDir {
			alreadyTrusted = true
			break
		}
	}

	if !alreadyTrusted {
		workspaces = append(workspaces, cleanDir)
		settingsMap["trustedWorkspaces"] = workspaces
		_ = os.MkdirAll(settingsDir, 0755)
		if out, err := json.MarshalIndent(settingsMap, "", "  "); err == nil {
			_ = os.WriteFile(settingsPath, out, 0644)
		}
	}
	return nil
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

// findAgyTranscriptFile locates the transcript.jsonl file for a conversation.
func findAgyTranscriptFile(baseDir, sessionID string) string {
	return filepath.Join(baseDir, sessionID, ".system_generated", "logs", "transcript.jsonl")
}

// runVersionCommand runs `<executable> --version` to extract version string.
func runVersionCommand(ctx context.Context, executable string) (string, error) {
	cmd := exec.CommandContext(ctx, executable, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
