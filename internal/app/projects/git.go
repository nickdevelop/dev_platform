package projects

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitCommitItem represents one commit in the git repository.
type GitCommitItem struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Date      string `json:"date"`
	Timestamp int64  `json:"timestamp"`
}

var safeCommitRefRegex = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$|^HEAD(~[0-9]+)?(\^[0-9]*)?$`)

// isValidCommitRef checks whether a git ref is safe and well-formed.
func isValidCommitRef(ref string) bool {
	return safeCommitRefRegex.MatchString(ref)
}

// gitTimeout bounds every git probe so a hung filesystem cannot hold an API
// worker. Branch display is best-effort UI data, never worth waiting for.
const gitTimeout = 5 * time.Second

// GitInfo is the JSON-stable git state of an instance's working directory.
// IsGit is false when the directory is not inside a git work tree or git is
// unavailable; every other field is empty then. Branch is the short branch
// name (or the short commit when HEAD is detached); Dirty reports uncommitted
// changes; Root is the work-tree top level.
type GitInfo struct {
	IsGit  bool   `json:"isGit"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
	Root   string `json:"root,omitempty"`
}

// GitInfo returns the git state of a project's directory. The directory always
// comes from the platform record, never from caller input. A missing project is
// ErrProjectNotFound; any git-level failure degrades to IsGit false (see
// probeGitDir), so branch display never blocks the page that shows it.
func (s *Service) GitInfo(ctx context.Context, projectID uint) (GitInfo, error) {
	row, err := s.load(projectID)
	if err != nil {
		return GitInfo{}, err
	}
	return probeGitDir(ctx, row.Path), nil
}

// GitCommits returns the recent commits of a project's git repository.
// If the directory is not inside a git work tree, git is unavailable, or the repository
// has no commits yet, it returns an empty slice without an error.
func (s *Service) GitCommits(ctx context.Context, projectID uint, limit int) ([]GitCommitItem, error) {
	row, err := s.load(projectID)
	if err != nil {
		return nil, err
	}
	if !s.isGitDir(ctx, row.Path) {
		return []GitCommitItem{}, nil
	}

	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}

	// Format: <hash>\x00<shortHash>\x00<authorName>\x00<unixTimestamp>\x00<subject>\n
	out, err := gitOutputLimit(ctx, row.Path, 1<<20, "log", "-n", strconv.Itoa(limit), "--pretty=format:%H%x00%h%x00%an%x00%at%x00%s")
	if err != nil || strings.TrimSpace(out) == "" {
		return []GitCommitItem{}, nil
	}

	var commits []GitCommitItem
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 5 {
			continue
		}

		hash := parts[0]
		shortHash := parts[1]
		author := parts[2]
		ts, _ := strconv.ParseInt(parts[3], 10, 64)
		subject := parts[4]

		dateStr := ""
		if ts > 0 {
			dateStr = time.Unix(ts, 0).Format(time.RFC3339)
		}

		commits = append(commits, GitCommitItem{
			Hash:      hash,
			ShortHash: shortHash,
			Message:   subject,
			Author:    author,
			Date:      dateStr,
			Timestamp: ts,
		})
	}

	return commits, nil
}

// probeGitDir inspects directory with the git CLI. Any failure — missing git,
// missing directory, not a work tree, timeout — degrades to IsGit false
// rather than an error: branch display must never break opening a session.
func probeGitDir(ctx context.Context, directory string) GitInfo {
	if directory == "" {
		return GitInfo{}
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	root, err := gitOutput(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return GitInfo{}
	}
	info := GitInfo{IsGit: true, Root: root}

	branch, err := gitOutput(ctx, directory, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "" || branch == "HEAD" {
		// Unborn branches (no commits yet) and detached HEADs do not
		// resolve through abbrev-ref: symbolic-ref names the former, and a
		// short commit identifies the latter.
		if symbolic, serr := gitOutput(ctx, directory, "symbolic-ref", "--short", "HEAD"); serr == nil && symbolic != "" {
			branch = symbolic
			err = nil
		} else if short, serr := gitOutput(ctx, directory, "rev-parse", "--short", "HEAD"); serr == nil && short != "" {
			branch = short
			err = nil
		}
	}
	if err != nil || branch == "" || branch == "HEAD" {
		return info
	}
	info.Branch = branch

	// Porcelain v1 with --untracked-files=no keeps the probe cheap on huge
	// checkouts: any listed entry already means dirty. Output is capped so a
	// pathological work tree cannot inflate API memory.
	if out, err := gitOutputLimit(ctx, directory, 4096, "status", "--porcelain", "--untracked-files=no"); err == nil && strings.TrimSpace(out) != "" {
		info.Dirty = true
	}
	return info
}

// gitOutput runs one git command and returns its trimmed stdout.
func gitOutput(ctx context.Context, directory string, args ...string) (string, error) {
	out, err := gitOutputLimit(ctx, directory, 1<<20, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// gitOutputLimit runs git -C directory with args and returns up to limit
// bytes of stdout. Stderr is discarded: probe failures are silent by design.
func gitOutputLimit(ctx context.Context, directory string, limit int, args ...string) (string, error) {
	full := append([]string{"-C", directory}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: limit}
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// limitedWriter caps what it keeps so a chatty command cannot grow memory
// without bound; excess bytes are counted and dropped.
type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}
