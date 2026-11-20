//go:build windows

package windows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// DiscoveredShell describes a shell candidate found on Windows.
type DiscoveredShell struct {
	ID            string
	Name          string
	Path          string
	Args          []string
	IsRecommended bool
}

// ScanShells discovers installed shells on Windows, prioritizing Git Bash.
func ScanShells() []DiscoveredShell {
	var list []DiscoveredShell
	seen := make(map[string]bool)

	add := func(id, name, path string, args []string, recommended bool) {
		clean := filepath.Clean(path)
		lower := strings.ToLower(clean)
		if seen[lower] {
			return
		}
		if fi, err := os.Stat(clean); err == nil && !fi.IsDir() {
			seen[lower] = true
			list = append(list, DiscoveredShell{
				ID:            id,
				Name:          name,
				Path:          clean,
				Args:          args,
				IsRecommended: recommended,
			})
		}
	}

	// 1. Git Bash candidates
	// Registry check: HKLM & HKCU
	gitBashPaths := findGitBashFromRegistry()
	for _, p := range gitBashPaths {
		add("git-bash", "Git Bash", p, []string{"--login", "-i"}, true)
	}

	// Common installation locations for Git
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	if programFilesX86 == "" {
		programFilesX86 = `C:\Program Files (x86)`
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	programData := os.Getenv("ProgramData")

	gitCandidates := []string{
		filepath.Join(programFiles, "Git", "bin", "bash.exe"),
		filepath.Join(programFilesX86, "Git", "bin", "bash.exe"),
	}
	if localAppData != "" {
		gitCandidates = append(gitCandidates, filepath.Join(localAppData, "Programs", "Git", "bin", "bash.exe"))
	}
	if programData != "" {
		gitCandidates = append(gitCandidates, filepath.Join(programData, "Git", "bin", "bash.exe"))
	}
	for _, p := range gitCandidates {
		add("git-bash", "Git Bash", p, []string{"--login", "-i"}, true)
	}

	// PATH check for git.exe -> infer bash.exe
	if gitPath, err := exec.LookPath("git.exe"); err == nil {
		// e.g. C:\Program Files\Git\cmd\git.exe -> C:\Program Files\Git\bin\bash.exe
		dir := filepath.Dir(gitPath)
		parent := filepath.Dir(dir)
		inferred := filepath.Join(parent, "bin", "bash.exe")
		add("git-bash", "Git Bash", inferred, []string{"--login", "-i"}, true)
	}
	// PATH check for bash.exe directly
	if bashPath, err := exec.LookPath("bash.exe"); err == nil {
		add("git-bash", "Git Bash", bashPath, []string{"--login", "-i"}, true)
	}

	// 2. PowerShell 7 (pwsh)
	if pwshPath, err := exec.LookPath("pwsh.exe"); err == nil {
		add("pwsh", "PowerShell 7", pwshPath, []string{"-NoLogo"}, len(list) == 0)
	}
	add("pwsh", "PowerShell 7", filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"), []string{"-NoLogo"}, len(list) == 0)

	// 3. Windows PowerShell (powershell.exe)
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	winPS := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	add("powershell", "Windows PowerShell", winPS, []string{"-NoLogo"}, len(list) == 0)
	if psPath, err := exec.LookPath("powershell.exe"); err == nil {
		add("powershell", "Windows PowerShell", psPath, []string{"-NoLogo"}, len(list) == 0)
	}

	// 4. Command Prompt (cmd.exe)
	cmdPath := filepath.Join(systemRoot, "System32", "cmd.exe")
	add("cmd", "Command Prompt", cmdPath, nil, false)

	// 5. WSL (wsl.exe)
	wslPath := filepath.Join(systemRoot, "System32", "wsl.exe")
	add("wsl", "WSL", wslPath, nil, false)

	// Ensure at least one is recommended if list is not empty
	hasRec := false
	for _, s := range list {
		if s.IsRecommended {
			hasRec = true
			break
		}
	}
	if !hasRec && len(list) > 0 {
		list[0].IsRecommended = true
	}

	return list
}

func findGitBashFromRegistry() []string {
	var paths []string
	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\GitForWindows`},
		{registry.CURRENT_USER, `SOFTWARE\GitForWindows`},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Git_is1`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Git_is1`},
	}
	for _, k := range keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		for _, valName := range []string{"InstallPath", "InstallLocation"} {
			if val, _, err := key.GetStringValue(valName); err == nil && val != "" {
				bashPath := filepath.Join(val, "bin", "bash.exe")
				paths = append(paths, bashPath)
			}
		}
		_ = key.Close()
	}
	return paths
}
