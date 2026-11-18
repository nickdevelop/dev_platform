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

func TestResolveReadPath(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Relative path within workspace
	target, err := ResolveReadPath(tempDir, "sub/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Clean(filepath.Join(tempDir, "sub/file.txt"))
	if target != expected {
		t.Errorf("got %q, want %q", target, expected)
	}

	// 2. Relative path escaping workspace allows read resolution without error
	targetEscaped, err := ResolveReadPath(tempDir, "../escaped.txt")
	if err != nil {
		t.Fatalf("unexpected error for reading parent path: %v", err)
	}
	expectedEscaped := filepath.Clean(filepath.Join(tempDir, "../escaped.txt"))
	if targetEscaped != expectedEscaped {
		t.Errorf("got %q, want %q", targetEscaped, expectedEscaped)
	}

	// 3. Absolute path outside workspace allowed
	externalDir := t.TempDir()
	extPath := filepath.Join(externalDir, "SKILL.md")
	targetExt, err := ResolveReadPath(tempDir, extPath)
	if err != nil {
		t.Fatalf("unexpected error for absolute external path: %v", err)
	}
	if targetExt != filepath.Clean(extPath) {
		t.Errorf("got %q, want %q", targetExt, extPath)
	}

	// 4. Home directory expansion (~)
	homeDir, homeErr := os.UserHomeDir()
	if homeErr == nil && homeDir != "" {
		targetTilde, err := ResolveReadPath(tempDir, "~/.agents/skills/plantuml/SKILL.md")
		if err != nil {
			t.Fatalf("unexpected error expanding ~: %v", err)
		}
		expectedTilde := filepath.Clean(filepath.Join(homeDir, ".agents/skills/plantuml/SKILL.md"))
		if targetTilde != expectedTilde {
			t.Errorf("got %q, want %q", targetTilde, expectedTilde)
		}
	}

	// 5. Empty path error
	if _, err := ResolveReadPath(tempDir, ""); err == nil {
		t.Errorf("expected error on empty path")
	}
}

func TestReadAndListOutsideWorkspaceAllowed(t *testing.T) {
	workDir := t.TempDir()
	externalDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()

	// Create an external skill file (e.g. simulating ~/.agents/skills/plantuml/SKILL.md)
	externalSkillDir := filepath.Join(externalDir, "plantuml")
	if err := os.MkdirAll(externalSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir externalSkillDir: %v", err)
	}
	skillContent := "# PlantUML Skill Instructions\nUse standard formatting."
	skillFilePath := filepath.Join(externalSkillDir, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(skillContent), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	// 1. read_file on external skill file MUST SUCCEED
	readArgs := `{"path": "` + filepath.ToSlash(skillFilePath) + `"}`
	readRes, err := registry.Execute(ctx, workDir, ToolReadFile, readArgs)
	if err != nil {
		t.Fatalf("expected read_file to succeed for external skill, got: %v", err)
	}
	if !strings.Contains(readRes, "PlantUML Skill Instructions") {
		t.Errorf("read_file output did not match expected skill content: %s", readRes)
	}

	// 2. list_directory on external skill directory MUST SUCCEED
	listArgs := `{"path": "` + filepath.ToSlash(externalSkillDir) + `"}`
	listRes, err := registry.Execute(ctx, workDir, ToolListDirectory, listArgs)
	if err != nil {
		t.Fatalf("expected list_directory to succeed for external skill dir, got: %v", err)
	}
	if !strings.Contains(listRes, "SKILL.md") {
		t.Errorf("list_directory output did not contain SKILL.md: %s", listRes)
	}

	// 3. write_file on non-temp external path MUST STILL BE BLOCKED by write boundary
	home, _ := os.UserHomeDir()
	nonTempExternal := filepath.Join(home, ".devplatform_non_temp_test", "forbidden.txt")
	writeArgs := `{"path": "` + filepath.ToSlash(nonTempExternal) + `", "content": "leak"}`
	_, err = registry.Execute(ctx, workDir, ToolWriteFile, writeArgs)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected write_file outside workspace and temp to be rejected, got: %v", err)
	}

	// 4. edit_file on non-temp external path MUST STILL BE BLOCKED by write boundary
	editArgs := `{"path": "` + filepath.ToSlash(nonTempExternal) + `", "old_string": "a", "new_string": "b"}`
	_, err = registry.Execute(ctx, workDir, ToolEditFile, editArgs)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected edit_file outside workspace and temp to be rejected, got: %v", err)
	}
}

func TestWriteAndEditInTempDirectoryAllowed(t *testing.T) {
	workDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()

	// 1. Target file in os.TempDir() with nested subdirectory
	tempSubDir := filepath.Join(os.TempDir(), "devplat_test_write_whitelist", "nested")
	defer os.RemoveAll(filepath.Join(os.TempDir(), "devplat_test_write_whitelist"))

	tempFile := filepath.Join(tempSubDir, "output.json")
	writeArgs := `{"path": "` + filepath.ToSlash(tempFile) + `", "content": "{\"status\": \"initial\"}\n"}`
	writeRes, err := registry.Execute(ctx, workDir, ToolWriteFile, writeArgs)
	if err != nil {
		t.Fatalf("expected write_file in temp dir to succeed, got: %v", err)
	}
	if !strings.Contains(writeRes, "Successfully wrote") {
		t.Errorf("write_file response unexpected: %s", writeRes)
	}

	// 2. edit_file on the temp file
	editArgs := `{"path": "` + filepath.ToSlash(tempFile) + `", "old_string": "initial", "new_string": "updated"}`
	editRes, err := registry.Execute(ctx, workDir, ToolEditFile, editArgs)
	if err != nil {
		t.Fatalf("expected edit_file in temp dir to succeed, got: %v", err)
	}
	if !strings.Contains(editRes, "Successfully replaced") {
		t.Errorf("edit_file response unexpected: %s", editRes)
	}

	// 3. Verify content
	readRes, err := registry.Execute(ctx, workDir, ToolReadFile, `{"path": "`+filepath.ToSlash(tempFile)+`"}`)
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if !strings.Contains(readRes, "updated") {
		t.Errorf("expected updated content in temp file: %s", readRes)
	}
}

func TestSafeResolveWritePath(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid workspace relative path
	p1, err := SafeResolveWritePath(tempDir, "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1 != filepath.Clean(filepath.Join(tempDir, "file.txt")) {
		t.Errorf("got %q, want %q", p1, filepath.Clean(filepath.Join(tempDir, "file.txt")))
	}

	// 2. Valid temp directory path
	p2, err := SafeResolveWritePath(tempDir, filepath.Join(os.TempDir(), "some_temp_file.txt"))
	if err != nil {
		t.Fatalf("unexpected error for temp dir: %v", err)
	}
	if p2 != filepath.Clean(filepath.Join(os.TempDir(), "some_temp_file.txt")) {
		t.Errorf("got %q, want %q", p2, filepath.Clean(filepath.Join(os.TempDir(), "some_temp_file.txt")))
	}

	// 3. Traversal outside workspace to a non-temp path
	home, _ := os.UserHomeDir()
	nonTempAbs := filepath.Join(home, ".some_random_secret")
	_, err = SafeResolveWritePath(tempDir, nonTempAbs)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected error for non-temp write, got: %v", err)
	}

	// 4. Traversal escaping temp directory
	escapedTemp := filepath.Join(os.TempDir(), "..", "..", "etc", "passwd")
	_, err = SafeResolveWritePath(tempDir, escapedTemp)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected error for escaped temp path, got: %v", err)
	}

	// 5. Empty workspace directory error
	if _, err := SafeResolveWritePath("", "file.txt"); err == nil {
		t.Errorf("expected error on empty workspace directory")
	}
}
