package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeResolvePath(t *testing.T) {
	tempDir := t.TempDir()

	// Subfile within workspace
	target, err := SafeResolvePath(tempDir, "sub/file.txt")
	if err != nil {
		t.Fatalf("unexpected error for valid path: %v", err)
	}
	expected := filepath.Clean(filepath.Join(tempDir, "sub/file.txt"))
	if target != expected {
		t.Errorf("got %q, want %q", target, expected)
	}

	// Traversal attempt
	_, err = SafeResolvePath(tempDir, "../escaped.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace boundary") {
		t.Errorf("expected ErrPathEscapesWorkspace, got %v", err)
	}

	// Deep traversal attempt
	_, err = SafeResolvePath(tempDir, "a/b/../../../../escaped.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace boundary") {
		t.Errorf("expected ErrPathEscapesWorkspace, got %v", err)
	}
}

func TestFileOperations(t *testing.T) {
	tempDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()

	// 1. write_file
	writeArgs := `{"path": "hello.txt", "content": "line 1\nline 2\nline 3\n"}`
	res, err := registry.Execute(ctx, tempDir, ToolWriteFile, writeArgs)
	if err != nil {
		t.Fatalf("write_file error: %v", err)
	}
	if !strings.Contains(res, "Successfully wrote") {
		t.Errorf("write_file response unexpected: %s", res)
	}

	// 2. read_file
	readArgs := `{"path": "hello.txt", "offset": 2, "limit": 2}`
	readRes, err := registry.Execute(ctx, tempDir, ToolReadFile, readArgs)
	if err != nil {
		t.Fatalf("read_file error: %v", err)
	}
	if !strings.Contains(readRes, "2: line 2") || !strings.Contains(readRes, "3: line 3") {
		t.Errorf("read_file content unexpected: %s", readRes)
	}

	// 3. edit_file unique replacement
	editArgs := `{"path": "hello.txt", "old_string": "line 2", "new_string": "line two modified"}`
	editRes, err := registry.Execute(ctx, tempDir, ToolEditFile, editArgs)
	if err != nil {
		t.Fatalf("edit_file error: %v", err)
	}
	if !strings.Contains(editRes, "Successfully replaced 1 occurrence") {
		t.Errorf("edit_file response unexpected: %s", editRes)
	}

	// Verify content after edit
	verifyRes, _ := registry.Execute(ctx, tempDir, ToolReadFile, `{"path": "hello.txt"}`)
	if !strings.Contains(verifyRes, "line two modified") {
		t.Errorf("file did not contain edited string: %s", verifyRes)
	}

	// 4. list_directory
	listRes, err := registry.Execute(ctx, tempDir, ToolListDirectory, `{}`)
	if err != nil {
		t.Fatalf("list_directory error: %v", err)
	}
	if !strings.Contains(listRes, "hello.txt") {
		t.Errorf("list_directory missing hello.txt: %s", listRes)
	}
}

func TestSearchOperations(t *testing.T) {
	tempDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()

	_ = os.MkdirAll(filepath.Join(tempDir, "pkg/api"), 0o755)
	_ = os.WriteFile(filepath.Join(tempDir, "pkg/api/client.go"), []byte("package api\nfunc Connect() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() {}\n"), 0o644)

	// glob_search
	globRes, err := registry.Execute(ctx, tempDir, ToolGlobSearch, `{"pattern": "**/*.go"}`)
	if err != nil {
		t.Fatalf("glob_search error: %v", err)
	}
	if !strings.Contains(globRes, "client.go") || !strings.Contains(globRes, "main.go") {
		t.Errorf("glob_search output unexpected: %s", globRes)
	}

	// grep_search
	grepRes, err := registry.Execute(ctx, tempDir, ToolGrepSearch, `{"pattern": "func Connect"}`)
	if err != nil {
		t.Fatalf("grep_search error: %v", err)
	}
	if !strings.Contains(grepRes, "client.go:2: func Connect() {}") {
		t.Errorf("grep_search output unexpected: %s", grepRes)
	}
}

func TestExecuteCommand(t *testing.T) {
	tempDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()

	// Normal command
	res, err := registry.Execute(ctx, tempDir, ToolExecuteCommand, `{"command": "echo test_output_123"}`)
	if err != nil {
		t.Fatalf("execute_command error: %v", err)
	}
	if !strings.Contains(res, "test_output_123") {
		t.Errorf("execute_command unexpected output: %s", res)
	}

	// Forbidden command
	_, err = registry.Execute(ctx, tempDir, ToolExecuteCommand, `{"command": "rm -rf /"}`)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected forbidden command error, got: %v", err)
	}
}

func TestGitToolsNonRepo(t *testing.T) {
	tempDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()

	// Should not crash when not inside a git repo
	statusRes, err := registry.Execute(ctx, tempDir, ToolGitStatus, `{}`)
	if err != nil {
		t.Fatalf("git_status error: %v", err)
	}
	if !strings.Contains(statusRes, "not a Git repository") {
		t.Errorf("git_status unexpected response: %s", statusRes)
	}
}
