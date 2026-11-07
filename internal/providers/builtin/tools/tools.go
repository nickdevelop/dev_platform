package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
