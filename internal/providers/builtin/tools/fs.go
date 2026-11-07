package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"` // 1-based start line
	Limit  int    `json:"limit,omitempty"`  // max lines, default 2000
}

func handleReadFile(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args readFileArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return "", errors.New("read_file: path is required")
	}

	targetPath, err := SafeResolvePath(workDir, args.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", args.Path)
		}
		return "", fmt.Errorf("stat file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, use list_directory instead", args.Path)
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Check if binary
	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)
	_, _ = file.Seek(0, io.SeekStart)

	mime := http.DetectContentType(buffer[:n])
	if isBinaryMime(mime, buffer[:n]) {
		return fmt.Sprintf("[Binary file: %s, size: %d bytes, mime: %s]", args.Path, info.Size(), mime), nil
	}

	offset := args.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 2000
	} else if limit > 5000 {
		limit = 5000
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(file)
	// Support long lines (up to 1MB)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	linesRead := 0
	hasMore := false

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if linesRead >= limit {
			hasMore = true
			break
		}
		linesRead++
		sb.WriteString(fmt.Sprintf("%d: %s\n", lineNum, scanner.Text()))
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read file lines: %w", err)
	}

	if linesRead == 0 && lineNum < offset {
		return fmt.Sprintf("[File %s has only %d lines; requested offset was %d]", args.Path, lineNum, offset), nil
	}

	if hasMore {
		sb.WriteString(fmt.Sprintf("\n[... output truncated: read %d lines starting at line %d. Use offset=%d to read more ...]\n", limit, offset, lineNum))
	}

	return sb.String(), nil
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func handleWriteFile(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args writeFileArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid write_file arguments: %w", err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return "", errors.New("write_file: path is required")
	}

	targetPath, err := SafeResolvePath(workDir, args.Path)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create parent directory: %w", err)
	}

	if err := os.WriteFile(targetPath, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	lineCount := strings.Count(args.Content, "\n")
	if len(args.Content) > 0 && !strings.HasSuffix(args.Content, "\n") {
		lineCount++
	}

	return fmt.Sprintf("Successfully wrote %d bytes (%d lines) to %s", len(args.Content), lineCount, args.Path), nil
}

type editFileArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func handleEditFile(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args editFileArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid edit_file arguments: %w", err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return "", errors.New("edit_file: path is required")
	}
	if args.OldString == "" {
		return "", errors.New("edit_file: old_string is required")
	}
	if args.OldString == args.NewString {
		return "", errors.New("edit_file: old_string and new_string must differ")
	}

	targetPath, err := SafeResolvePath(workDir, args.Path)
	if err != nil {
		return "", err
	}

	contentBytes, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", args.Path)
		}
		return "", fmt.Errorf("read file for edit: %w", err)
	}
	content := string(contentBytes)

	// Step 1: exact match count
	occurrences := strings.Count(content, args.OldString)
	if occurrences == 0 {
		// Normalize line endings to test CRLF vs LF
		normalizedContent := strings.ReplaceAll(content, "\r\n", "\n")
		normalizedOld := strings.ReplaceAll(args.OldString, "\r\n", "\n")
		if strings.Contains(normalizedContent, normalizedOld) {
			// Content has line ending mismatch; convert old_string to match target
			if strings.Contains(content, "\r\n") {
				args.OldString = strings.ReplaceAll(normalizedOld, "\n", "\r\n")
				args.NewString = strings.ReplaceAll(strings.ReplaceAll(args.NewString, "\r\n", "\n"), "\n", "\r\n")
			} else {
				args.OldString = normalizedOld
				args.NewString = strings.ReplaceAll(args.NewString, "\r\n", "\n")
			}
			occurrences = strings.Count(content, args.OldString)
		}
	}

	if occurrences == 0 {
		return "", fmt.Errorf("edit_file failed: old_string not found in %s. Please verify the exact text including whitespace and line breaks", args.Path)
	}

	if occurrences > 1 && !args.ReplaceAll {
		return "", fmt.Errorf("edit_file failed: old_string matches %d occurrences in %s. Please provide more surrounding context to uniquely identify the location, or set replace_all: true", occurrences, args.Path)
	}

	var newContent string
	var replacementsMade int
	if args.ReplaceAll {
		newContent = strings.ReplaceAll(content, args.OldString, args.NewString)
		replacementsMade = occurrences
	} else {
		newContent = strings.Replace(content, args.OldString, args.NewString, 1)
		replacementsMade = 1
	}

	if err := os.WriteFile(targetPath, []byte(newContent), 0o644); err != nil {
		return "", fmt.Errorf("save edited file: %w", err)
	}

	return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", replacementsMade, args.Path), nil
}

type listDirArgs struct {
	Path string `json:"path,omitempty"`
}

type dirEntryItem struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

func handleListDirectory(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args listDirArgs
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	if args.Path == "" {
		args.Path = "."
	}

	targetPath, err := SafeResolvePath(workDir, args.Path)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return "", fmt.Errorf("list directory: %w", err)
	}

	var dirs []dirEntryItem
	var files []dirEntryItem

	for _, entry := range entries {
		name := entry.Name()
		// Always ignore .git directory
		if name == ".git" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if entry.IsDir() {
			dirs = append(dirs, dirEntryItem{
				Name:  name + "/",
				IsDir: true,
			})
		} else {
			files = append(files, dirEntryItem{
				Name:  name,
				IsDir: false,
				Size:  info.Size(),
			})
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Directory %s (%d entries):\n", args.Path, len(dirs)+len(files)))
	for _, d := range dirs {
		sb.WriteString(fmt.Sprintf("  [DIR]  %s\n", d.Name))
	}
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("  [FILE] %-30s (%d bytes)\n", f.Name, f.Size))
	}

	return sb.String(), nil
}

func isBinaryMime(mime string, sample []byte) bool {
	if strings.HasPrefix(mime, "text/") {
		return false
	}
	if strings.Contains(mime, "json") || strings.Contains(mime, "xml") || strings.Contains(mime, "javascript") {
		return false
	}
	// Check for null bytes in sample
	if bytes.IndexByte(sample, 0) != -1 {
		return true
	}
	return !utf8.Valid(sample)
}
