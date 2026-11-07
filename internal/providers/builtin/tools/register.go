package tools

import (
	"encoding/json"
)

// Tool Names constants.
const (
	ToolReadFile       = "read_file"
	ToolWriteFile      = "write_file"
	ToolEditFile       = "edit_file"
	ToolListDirectory  = "list_directory"
	ToolGlobSearch     = "glob_search"
	ToolGrepSearch     = "grep_search"
	ToolExecuteCommand = "execute_command"
	ToolGitStatus      = "git_status"
	ToolGitDiff        = "git_diff"
)

// registerAll registers the complete suite of builtin coding tools into the registry.
func (r *Registry) registerAll() {
	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolReadFile,
				Description: "Read text or view content of a file in the workspace. Supports pagination using offset and limit lines.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "Path to the file relative to the project workspace root or absolute path within workspace."
						},
						"offset": {
							"type": "integer",
							"description": "The 1-based line number to start reading from (optional, default 1)."
						},
						"limit": {
							"type": "integer",
							"description": "The maximum number of lines to read (optional, default 2000, max 5000)."
						}
					},
					"required": ["path"]
				}`),
			},
		},
		handleReadFile,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolWriteFile,
				Description: "Create a new file or completely overwrite an existing file with new content. Automatically creates parent directories.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "Path to the file to create or overwrite."
						},
						"content": {
							"type": "string",
							"description": "The full text content to write into the file."
						}
					},
					"required": ["path", "content"]
				}`),
			},
		},
		handleWriteFile,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolEditFile,
				Description: "Perform targeted precise search-and-replace edits inside an existing file. old_string must match uniquely in the file unless replace_all is true.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "Path to the file to edit."
						},
						"old_string": {
							"type": "string",
							"description": "Exact text to find and replace. Include surrounding lines for uniqueness if necessary."
						},
						"new_string": {
							"type": "string",
							"description": "Text to replace old_string with (must differ from old_string)."
						},
						"replace_all": {
							"type": "boolean",
							"description": "Whether to replace every occurrence of old_string. Defaults to false (exact single occurrence)."
						}
					},
					"required": ["path", "old_string", "new_string"]
				}`),
			},
		},
		handleEditFile,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolListDirectory,
				Description: "List files and subdirectories inside a directory in the workspace.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "Directory path to list (optional, defaults to workspace root '.')."
						}
					}
				}`),
			},
		},
		handleListDirectory,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolGlobSearch,
				Description: "Search file paths matching a glob pattern (e.g. '**/*.go', 'src/**/*.vue', '*.json') within the workspace.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"pattern": {
							"type": "string",
							"description": "Glob pattern to match file paths against."
						},
						"path": {
							"type": "string",
							"description": "Directory to search from (optional, defaults to '.')."
						},
						"limit": {
							"type": "integer",
							"description": "Maximum number of files to return (optional, default 100)."
						}
					},
					"required": ["pattern"]
				}`),
			},
		},
		handleGlobSearch,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolGrepSearch,
				Description: "Search text contents across workspace files using regular expressions or keywords, returning line previews.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"pattern": {
							"type": "string",
							"description": "Regular expression or literal text to search in file contents."
						},
						"path": {
							"type": "string",
							"description": "Directory to search within (optional, defaults to '.')."
						},
						"include": {
							"type": "string",
							"description": "Glob pattern to filter filenames (e.g. '*.go', '*.ts')."
						},
						"limit": {
							"type": "integer",
							"description": "Maximum matching lines to return (optional, default 100)."
						}
					},
					"required": ["pattern"]
				}`),
			},
		},
		handleGrepSearch,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolExecuteCommand,
				Description: "Execute a shell command within the project workspace directory and return the exit code, stdout, and stderr.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"command": {
							"type": "string",
							"description": "Shell command line to execute."
						},
						"timeout_ms": {
							"type": "integer",
							"description": "Execution timeout in milliseconds (optional, default 30000ms, max 120000ms)."
						}
					},
					"required": ["command"]
				}`),
			},
		},
		handleExecuteCommand,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolGitStatus,
				Description: "Get the current git status of the project workspace, showing modified, staged and untracked files.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "Optional subdirectory or file to limit git status to."
						}
					}
				}`),
			},
		},
		handleGitStatus,
	)

	r.Register(
		ToolDefinition{
			Type: "function",
			Function: FunctionSchema{
				Name:        ToolGitDiff,
				Description: "Inspect git diff of unstaged or staged code changes in the workspace.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "Optional file path to view diff for."
						},
						"staged": {
							"type": "boolean",
							"description": "Whether to view staged changes (--cached). Defaults to false."
						}
					}
				}`),
			},
		},
		handleGitDiff,
	)
}
