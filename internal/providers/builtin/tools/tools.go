package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// ErrPathEscapesWorkspace reports an attempt to access a file outside the workspace.
	ErrPathEscapesWorkspace = errors.New("tools: path escapes workspace boundary")
	// ErrToolNotFound reports an unregistered tool.
	ErrToolNotFound = errors.New("tools: tool not found")
)

// FunctionSchema specifies the function tool name, description and parameters schema.
type FunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolDefinition describes an OpenAI-compatible function tool.
type ToolDefinition struct {
	Type     string         `json:"type"` // always "function"
	Function FunctionSchema `json:"function"`
}

// ToolHandler is the execution signature for a registered workspace tool.
type ToolHandler func(ctx context.Context, workDir string, argsJSON string) (string, error)

// Registry manages the set of available builtin coding tools.
type Registry struct {
	definitions []ToolDefinition
	handlers    map[string]ToolHandler
}

// NewRegistry creates a registry populated with the standard builtin coding toolset.
func NewRegistry() *Registry {
	r := &Registry{
		definitions: make([]ToolDefinition, 0),
		handlers:    make(map[string]ToolHandler),
	}
	r.registerAll()
	return r
}

// Definitions returns the list of OpenAI function tool definitions.
func (r *Registry) Definitions() []ToolDefinition {
	out := make([]ToolDefinition, len(r.definitions))
	copy(out, r.definitions)
	return out
}

// Execute runs the named tool within the specified workspace directory.
func (r *Registry) Execute(ctx context.Context, workDir string, name string, argsJSON string) (string, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrToolNotFound, name)
	}
	return handler(ctx, workDir, argsJSON)
}

// Register adds a tool definition and its handler.
func (r *Registry) Register(def ToolDefinition, handler ToolHandler) {
	r.definitions = append(r.definitions, def)
	r.handlers[def.Function.Name] = handler
}

// SafeResolvePath verifies that the requested relative or absolute path is strictly within
// the workspace directory, resolving symlinks where safe and returning the clean absolute path.
func SafeResolvePath(workDir, relOrAbsPath string) (string, error) {
	if workDir == "" {
		return "", errors.New("empty workspace directory")
	}

	cleanWorkDir, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	var targetPath string
	if filepath.IsAbs(relOrAbsPath) {
		targetPath = filepath.Clean(relOrAbsPath)
	} else {
		targetPath = filepath.Join(cleanWorkDir, relOrAbsPath)
	}
	targetPath = filepath.Clean(targetPath)

	// Check if target path starts with cleanWorkDir
	rel, err := filepath.Rel(cleanWorkDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q is outside of %q", ErrPathEscapesWorkspace, targetPath, cleanWorkDir)
	}

	// If the file or parent directory exists and is a symlink, verify the real target
	if evalPath, err := filepath.EvalSymlinks(targetPath); err == nil {
		evalRel, evalErr := filepath.Rel(cleanWorkDir, evalPath)
		if evalErr != nil || evalRel == ".." || strings.HasPrefix(evalRel, ".."+string(filepath.Separator)) {
			// Real target is outside workspace
			return "", fmt.Errorf("%w: symlink target %q is outside workspace", ErrPathEscapesWorkspace, evalPath)
		}
	}

	return targetPath, nil
}

// SystemTempDirs returns the canonicalized system temporary directories permitted for writing.
func SystemTempDirs() []string {
	var dirs []string
	seen := make(map[string]bool)

	addDir := func(d string) {
		if strings.TrimSpace(d) == "" {
			return
		}
		clean, err := filepath.Abs(filepath.Clean(d))
		if err != nil {
			return
		}
		if !seen[clean] {
			seen[clean] = true
			dirs = append(dirs, clean)
		}
		// Also include symlink-resolved target (e.g. /tmp -> /private/tmp on macOS)
		if eval, err := filepath.EvalSymlinks(clean); err == nil && !seen[eval] {
			seen[eval] = true
			dirs = append(dirs, eval)
		}
	}

	addDir(os.TempDir())
	if runtime.GOOS != "windows" {
		addDir("/tmp")
		addDir("/var/tmp")
	}

	return dirs
}

// isPathInside reports whether target is equal to or a subpath of root.
func isPathInside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SafeResolveWritePath verifies that the requested path is within the workspace directory
// or an allowed temporary directory (e.g., os.TempDir(), /tmp), preventing symlink escapes.
func SafeResolveWritePath(workDir, relOrAbsPath string, extraAllowedDirs ...string) (string, error) {
	if workDir == "" {
		return "", errors.New("empty workspace directory")
	}

	cleanWorkDir, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	var targetPath string
	if filepath.IsAbs(relOrAbsPath) {
		targetPath = filepath.Clean(relOrAbsPath)
	} else {
		targetPath = filepath.Join(cleanWorkDir, relOrAbsPath)
	}
	targetPath = filepath.Clean(targetPath)

	// Collect all allowed roots: workspace + system temp dirs + extra allowed dirs
	allowedRoots := []string{cleanWorkDir}
	for _, td := range SystemTempDirs() {
		allowedRoots = append(allowedRoots, td)
	}
	for _, ed := range extraAllowedDirs {
		if strings.TrimSpace(ed) == "" {
			continue
		}
		if cleanEd, err := filepath.Abs(filepath.Clean(ed)); err == nil {
			allowedRoots = append(allowedRoots, cleanEd)
			if evalEd, err := filepath.EvalSymlinks(cleanEd); err == nil {
				allowedRoots = append(allowedRoots, evalEd)
			}
		}
	}

	// 1. Check lexical containment
	matched := false
	for _, root := range allowedRoots {
		if isPathInside(root, targetPath) {
			matched = true
			break
		}
	}
	if !matched {
		return "", fmt.Errorf("%w: %q is outside of allowed write boundaries", ErrPathEscapesWorkspace, targetPath)
	}

	// 2. Symlink escape check: probe existing path or ancestor
	probePath := targetPath
	for probePath != "" && probePath != "." && probePath != string(filepath.Separator) {
		if _, err := os.Lstat(probePath); err == nil {
			break
		}
		parent := filepath.Dir(probePath)
		if parent == probePath {
			break
		}
		probePath = parent
	}

	if evalPath, err := filepath.EvalSymlinks(probePath); err == nil {
		relSub, relErr := filepath.Rel(probePath, targetPath)
		realTarget := targetPath
		if relErr == nil {
			realTarget = filepath.Join(evalPath, relSub)
		} else {
			realTarget = evalPath
		}
		realMatched := false
		for _, root := range allowedRoots {
			if isPathInside(root, realTarget) {
				realMatched = true
				break
			}
		}
		if !realMatched {
			return "", fmt.Errorf("%w: symlink target %q escapes allowed write boundaries", ErrPathEscapesWorkspace, realTarget)
		}
	}

	return targetPath, nil
}

// ResolveReadPath resolves a relative or absolute path for read-only tools (e.g. read_file, list_directory).
// Unlike SafeResolvePath which enforces workspace sandboxing for write/edit operations, ResolveReadPath
// allows reading files across the host filesystem (e.g., user-level ~/.agents/skills or platform data skills)
// and expands user home directory references (~).
func ResolveReadPath(workDir, relOrAbsPath string) (string, error) {
	cleanPath := strings.TrimSpace(relOrAbsPath)
	if cleanPath == "" {
		return "", errors.New("empty path")
	}

	// Expand ~ or ~/... to user's home directory if applicable
	if cleanPath == "~" || strings.HasPrefix(cleanPath, "~/") || strings.HasPrefix(cleanPath, "~\\") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			cleanPath = filepath.Join(home, cleanPath[1:])
		}
	}

	if filepath.IsAbs(cleanPath) {
		return filepath.Clean(cleanPath), nil
	}

	if workDir == "" {
		return "", errors.New("empty workspace directory")
	}
	cleanWorkDir, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	return filepath.Clean(filepath.Join(cleanWorkDir, cleanPath)), nil
}

// FormatError formats a tool error into a clear, structured JSON error response.
func FormatError(err error) string {
	resp, _ := json.Marshal(map[string]string{
		"error": err.Error(),
	})
	return string(resp)
}

// FileExists reports whether a path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
