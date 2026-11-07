package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type gitStatusArgs struct {
	Path string `json:"path,omitempty"`
}

func handleGitStatus(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args gitStatusArgs
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Verify git is inside a work tree
	checkCmd := exec.CommandContext(execCtx, "git", "rev-parse", "--is-inside-work-tree")
	checkCmd.Dir = workDir
	if out, err := checkCmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return fmt.Sprintf("Workspace %s is not a Git repository", workDir), nil
	}

	cmdArgs := []string{"status", "--porcelain"}
	if args.Path != "" {
		targetPath, err := SafeResolvePath(workDir, args.Path)
		if err != nil {
			return "", err
		}
		cmdArgs = append(cmdArgs, targetPath)
	}

	cmd := exec.CommandContext(execCtx, "git", cmdArgs...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git status error: %w (output: %s)", err, string(out))
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return "Working tree clean, no staged or untracked changes.", nil
	}

	return fmt.Sprintf("Git status:\n%s", output), nil
}

type gitDiffArgs struct {
	Path   string `json:"path,omitempty"`
	Staged bool   `json:"staged,omitempty"`
}

func handleGitDiff(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args gitDiffArgs
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}

	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmdArgs := []string{"diff"}
	if args.Staged {
		cmdArgs = append(cmdArgs, "--cached")
	}

	if args.Path != "" {
		targetPath, err := SafeResolvePath(workDir, args.Path)
		if err != nil {
			return "", err
		}
		cmdArgs = append(cmdArgs, "--", targetPath)
	}

	cmd := exec.CommandContext(execCtx, "git", cmdArgs...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff error: %w (output: %s)", err, string(out))
	}

	output := string(out)
	if strings.TrimSpace(output) == "" {
		if args.Staged {
			return "No staged changes found in git diff --cached.", nil
		}
		return "No unstaged changes found in git diff.", nil
	}

	// Truncate if diff is very long (e.g. > 100KB)
	const maxDiffLen = 100 * 1024
	if len(output) > maxDiffLen {
		output = output[:maxDiffLen] + "\n\n[... git diff truncated at 100KB ...]\n"
	}

	return output, nil
}
