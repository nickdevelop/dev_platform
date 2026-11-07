package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type globSearchArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

func handleGlobSearch(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args globSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid glob_search arguments: %w", err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "", errors.New("glob_search: pattern is required")
	}

	searchRoot := "."
	if args.Path != "" {
		searchRoot = args.Path
	}

	targetRoot, err := SafeResolvePath(workDir, searchRoot)
	if err != nil {
		return "", err
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 100
	} else if limit > 500 {
		limit = 500
	}

	cleanWorkDir, _ := filepath.Abs(filepath.Clean(workDir))
	var matches []string

	// Compile glob pattern into a regex for multi-level matching
	regexPattern := globToRegex(args.Pattern)
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern %q: %w", args.Pattern, err)
	}

	err = filepath.WalkDir(targetRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip .git directory entirely
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(cleanWorkDir, p)
		if err != nil {
			return nil
		}
		// Normalize to forward slashes
		slashRel := filepath.ToSlash(rel)

		if re.MatchString(slashRel) || re.MatchString(d.Name()) {
			matches = append(matches, slashRel)
			if len(matches) >= limit {
				return io.EOF
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("glob search: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No files found matching %q in %s", args.Pattern, searchRoot), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching file(s) for %q:\n", len(matches), args.Pattern))
	for _, m := range matches {
		sb.WriteString(fmt.Sprintf("  %s\n", m))
	}
	if len(matches) >= limit {
		sb.WriteString(fmt.Sprintf("\n[... truncated at %d matches. Refine your pattern or increase limit ...]\n", limit))
	}

	return sb.String(), nil
}

type grepSearchArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Include string `json:"include,omitempty"` // e.g. "*.go"
	Limit   int    `json:"limit,omitempty"`
}

func handleGrepSearch(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args grepSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid grep_search arguments: %w", err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "", errors.New("grep_search: pattern is required")
	}

	searchRoot := "."
	if args.Path != "" {
		searchRoot = args.Path
	}

	targetRoot, err := SafeResolvePath(workDir, searchRoot)
	if err != nil {
		return "", err
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 100
	} else if limit > 500 {
		limit = 500
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		// Fallback to literal quote
		re = regexp.MustCompile(regexp.QuoteMeta(args.Pattern))
	}

	var includeRe *regexp.Regexp
	if args.Include != "" {
		includeRe, _ = regexp.Compile(globToRegex(args.Include))
	}

	cleanWorkDir, _ := filepath.Abs(filepath.Clean(workDir))
	var matchLines []string

	err = filepath.WalkDir(targetRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		if includeRe != nil && !includeRe.MatchString(d.Name()) {
			return nil
		}

		rel, _ := filepath.Rel(cleanWorkDir, p)
		slashRel := filepath.ToSlash(rel)

		file, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer file.Close()

		// Read first 512 bytes to check binary
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		if isBinaryMime("application/octet-stream", buf[:n]) {
			return nil
		}
		_, _ = file.Seek(0, 0)

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				trimmed := strings.TrimRight(line, "\r\n")
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				matchLines = append(matchLines, fmt.Sprintf("%s:%d: %s", slashRel, lineNum, trimmed))
				if len(matchLines) >= limit {
					return io.EOF
				}
			}
		}

		return nil
	})

	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("grep search: %w", err)
	}

	if len(matchLines) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchRoot), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d match(es) for %q:\n", len(matchLines), args.Pattern))
	for _, ml := range matchLines {
		sb.WriteString(ml + "\n")
	}
	if len(matchLines) >= limit {
		sb.WriteString(fmt.Sprintf("\n[... truncated at %d matches ...]\n", limit))
	}

	return sb.String(), nil
}

// globToRegex translates a glob pattern (*, **, ?) to an anchored regexp string.
// Specifically, "**/" matches zero or more directory segments.
func globToRegex(glob string) string {
	var sb strings.Builder
	sb.WriteString("^")

	chars := []rune(glob)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]
		if ch == '*' {
			if i+1 < len(chars) && chars[i+1] == '*' {
				// We have "**"
				if i+2 < len(chars) && chars[i+2] == '/' {
					// "**/" matches zero or more directories, e.g. "foo/bar/" or ""
					sb.WriteString("(.*/)?")
					i += 2
					continue
				}
				sb.WriteString(".*")
				i++
			} else {
				sb.WriteString("[^/]*")
			}
		} else if ch == '?' {
			sb.WriteString("[^/]")
		} else if strings.ContainsRune(".+()|[]{}^$\\-", ch) {
			sb.WriteRune('\\')
			sb.WriteRune(ch)
		} else {
			sb.WriteRune(ch)
		}
	}
	sb.WriteString("$")
	return sb.String()
}
