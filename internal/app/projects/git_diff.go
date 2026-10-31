package projects

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitFileDiffResponse holds the unified diff and change statistics of a file.
type GitFileDiffResponse struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath,omitempty"`
	Status    string `json:"status"` // "modified", "added", "deleted", "untracked", "renamed"
	Patch     string `json:"patch"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	IsBinary  bool   `json:"isBinary"`
}

// resolveProjectRelativePath validates that relPath stays safely inside the project workspace boundary.
func (s *Service) resolveProjectRelativePath(projectID uint, relPath string) (string, string, error) {
	if strings.ContainsRune(relPath, 0) {
		return "", "", ErrInvalidTarget
	}
	proj, err := s.repo.Get(projectID)
	if err != nil {
		return "", "", err
	}
	rootDir := proj.Path
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	cleanRel = strings.TrimPrefix(cleanRel, "/")
	if cleanRel == "" || cleanRel == "." {
		return "", "", ErrInvalidTarget
	}
	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.Contains(cleanRel, "/../") {
		return "", "", ErrPathEscaped
	}
	return rootDir, cleanRel, nil
}

// GitFileDiff returns the unified diff for a single file within the project,
// either for working tree changes (when commitRef is empty) or for a specific commit.
func (s *Service) GitFileDiff(ctx context.Context, projectID uint, relPath, commitRef string) (*GitFileDiffResponse, error) {
	rootDir, cleanRel, err := s.resolveProjectRelativePath(projectID, relPath)
	if err != nil {
		return nil, err
	}
	if !s.isGitDir(ctx, rootDir) {
		return nil, ErrInvalidTarget
	}

	cleanRef := strings.TrimSpace(commitRef)
	if cleanRef != "" {
		if !isValidCommitRef(cleanRef) {
			return nil, ErrInvalidTarget
		}
		return s.gitCommitFileDiff(ctx, rootDir, cleanRel, cleanRef)
	}

	return s.gitWorkingFileDiff(ctx, rootDir, cleanRel)
}

func (s *Service) gitWorkingFileDiff(ctx context.Context, rootDir, cleanRel string) (*GitFileDiffResponse, error) {
	fullPath := filepath.Join(rootDir, filepath.FromSlash(cleanRel))

	// Check porcelain status to see if it's untracked
	porcelain, _ := gitOutputLimit(ctx, rootDir, 16384, "status", "--porcelain", "--", cleanRel)
	porcelain = strings.TrimSpace(porcelain)
	isUntracked := strings.HasPrefix(porcelain, "??")

	var patch string
	var status string = "modified"

	if isUntracked {
		status = "untracked"
		// Produce diff against /dev/null for untracked new file
		// Note: git diff --no-index returns exit status 1 on diff, so gitOutputLimit returns error,
		// but limitedWriter captures the patch in stdout.
		patchOut, _ := gitOutputLimit(ctx, rootDir, 2<<20, "diff", "--no-index", "--", "/dev/null", fullPath)
		if patchOut != "" {
			patch = normalizeNoIndexPatch(patchOut, cleanRel, fullPath)
		} else if fi, statErr := os.Stat(fullPath); statErr == nil && !fi.IsDir() {
			// Fallback: manually generate patch if file is readable text
			patch = generateNewFilePatch(fullPath, cleanRel)
		}
	} else {
		// First try diffing against HEAD
		out, err := gitOutputLimit(ctx, rootDir, 2<<20, "diff", "-U3", "HEAD", "--", cleanRel)
		if err != nil || out == "" {
			// In empty repos without HEAD, try staged or plain working diff
			out, _ = gitOutputLimit(ctx, rootDir, 2<<20, "diff", "-U3", "--", cleanRel)
		}
		patch = out

		if strings.HasPrefix(porcelain, "A ") || strings.HasPrefix(porcelain, "A") {
			status = "added"
		} else if strings.HasPrefix(porcelain, "D ") || strings.HasPrefix(porcelain, " D") {
			status = "deleted"
		} else if strings.HasPrefix(porcelain, "R ") {
			status = "renamed"
		}
	}

	return parsePatchToDiffResponse(cleanRel, patch, status), nil
}

func (s *Service) gitCommitFileDiff(ctx context.Context, rootDir, cleanRel, commitRef string) (*GitFileDiffResponse, error) {
	// Use diff-tree with -p (patch) and --root (handles initial commit)
	out, err := gitOutputLimit(ctx, rootDir, 2<<20, "diff-tree", "-p", "-U3", "--root", "-r", commitRef, "--", cleanRel)
	if err != nil {
		return nil, err
	}

	status := "modified"
	if strings.Contains(out, "new file mode") {
		status = "added"
	} else if strings.Contains(out, "deleted file mode") {
		status = "deleted"
	}

	return parsePatchToDiffResponse(cleanRel, out, status), nil
}

func parsePatchToDiffResponse(cleanRel, patch, fallbackStatus string) *GitFileDiffResponse {
	var additions, deletions int
	var isBinary bool
	status := fallbackStatus

	if strings.Contains(patch, "Binary files") || strings.Contains(patch, "GIT binary patch") {
		isBinary = true
	}

	scanner := bufio.NewScanner(strings.NewReader(patch))
	for scanner.Scan() {
		line := scanner.Text()
		if fallbackStatus != "untracked" {
			if strings.HasPrefix(line, "new file mode") {
				status = "added"
			} else if strings.HasPrefix(line, "deleted file mode") {
				status = "deleted"
			}
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}

	return &GitFileDiffResponse{
		Path:      cleanRel,
		Status:    status,
		Patch:     patch,
		Additions: additions,
		Deletions: deletions,
		IsBinary:  isBinary,
	}
}

func normalizeNoIndexPatch(patch, cleanRel, fullPath string) string {
	res := strings.ReplaceAll(patch, "/dev/null", "a/"+cleanRel)
	res = strings.ReplaceAll(res, "b"+fullPath, "b/"+cleanRel)
	res = strings.ReplaceAll(res, fullPath, "b/"+cleanRel)
	return res
}

func generateNewFilePatch(fullPath, cleanRel string) string {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}
	content := string(data)
	if content == "" {
		return fmt.Sprintf("diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +0,0 @@\n", cleanRel, cleanRel, cleanRel)
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	lineCount := len(lines)

	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", cleanRel, cleanRel, cleanRel, lineCount)
	for _, l := range lines {
		sb.WriteString("+" + l + "\n")
	}
	return sb.String()
}
