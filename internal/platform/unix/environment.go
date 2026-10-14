//go:build unix

package unix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment runs the unix startup checks: WSL1 is refused outright, and a
// data directory on a DrvFs mount produces a warning.
type Environment struct {
	// VersionReader returns the contents of /proc/version. It is nil in
	// production (the real file is read) and injected in tests.
	VersionReader func() (string, error)
}

// Check rejects WSL1: its emulated kernel breaks flock and mode-bit
// semantics the platform relies on. A missing /proc/version cannot prove
// WSL1, so it does not block startup.
func (e Environment) Check() error {
	version, err := e.procVersion()
	if err != nil {
		return nil
	}
	if isWSL1(version) {
		return errors.New("platform: WSL1 is not supported; upgrade to WSL2 and run the Linux build there")
	}
	return nil
}

// Describe returns startup diagnostics: a WSL2 marker when detected and a
// warning when the data directory sits on a DrvFs mount, where flock and
// permission bits are unreliable.
func (e Environment) Describe(dataDir string) []string {
	var lines []string
	if version, err := e.procVersion(); err == nil && isWSL2(version) {
		lines = append(lines, "platform: running under WSL2")
	}
	if isDrvFsDataDir(dataDir) {
		lines = append(lines, fmt.Sprintf(
			"platform: data directory %s is on a DrvFs mount (/mnt/<drive>); flock and permission bits are unreliable there, prefer a directory in the Linux filesystem",
			dataDir))
	}
	return lines
}

func (e Environment) procVersion() (string, error) {
	if e.VersionReader != nil {
		return e.VersionReader()
	}
	raw, err := os.ReadFile("/proc/version")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// isWSL reports a Windows Subsystem for Linux kernel of either generation.
func isWSL(version string) bool {
	return strings.Contains(strings.ToLower(version), "microsoft")
}

// isWSL2 reports the WSL2 marker in the kernel release.
func isWSL2(version string) bool {
	return strings.Contains(strings.ToLower(version), "wsl2")
}

// isWSL1 reports a WSL kernel without the WSL2 marker.
func isWSL1(version string) bool {
	return isWSL(version) && !isWSL2(version)
}

// isDrvFsDataDir reports whether dir lives under a /mnt/<drive> mount.
func isDrvFsDataDir(dir string) bool {
	rest, ok := strings.CutPrefix(filepath.Clean(dir), "/mnt/")
	if !ok || len(rest) == 0 {
		return false
	}
	drive := rest[0]
	if !(drive >= 'a' && drive <= 'z' || drive >= 'A' && drive <= 'Z') {
		return false
	}
	return len(rest) == 1 || rest[1] == '/'
}
