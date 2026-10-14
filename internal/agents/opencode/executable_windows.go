//go:build windows

package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devplatform/internal/platform"
)

// This file resolves the opencode executable from the npm global layout
// instead of running the PATH shim. npm places two files in the global
// prefix for Git Bash and cmd respectively:
//
//	opencode      (extensionless sh script — never executable on Windows)
//	opencode.cmd  (cmd shim calling into node_modules)
//
// Running the shim through cmd.exe would drag every argument through cmd's
// quoting rules, so the platform locates the real binary underneath:
//
//  1. The directory of opencode.cmd on PATH is the npm prefix; no npm
//     subprocess is needed for this step.
//  2. <prefix>\node_modules\@opencode\cli\bin\opencode.exe, verified as a
//     native binary by its MZ header.
//  3. Any platform pack present (@opencode/cli-windows-x64 or -arm64), same
//     verification — no GOARCH checks in business code.
//  4. `npm root -g` as a fallback prefix source.
//  5. The opencode.cmd shim itself, run via cmd.exe with fixed args only.
//
// A native opencode.exe already on PATH (no npm involved) is honored when
// no shim exists at all.
func init() {
	Descriptor.ResolveExecutable = func(name string) (string, error) {
		return resolveOpencodePath(name)
	}
}

// probePath resolves the executable for pre-flight checks.
func (a *Adapter) probePath() (string, error) {
	if manual := a.manualPath(); manual != "" {
		return manual, nil
	}
	return resolveOpencodePath(a.execPath())
}

// backendCommand builds the process to run: the resolved .exe directly, or
// cmd.exe over the shim fallback. The fallback argv comes from the fixed
// platform call sites only, never from user input. Pass
// context.Background for launches that must outlive the start request.
func (a *Adapter) backendCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	executable := a.execPath()
	bin := executable
	if a.manualPath() == "" {
		resolved, err := resolveOpencodePath(executable)
		if err != nil {
			return nil, err
		}
		bin = resolved
	}
	if isCmdShim(bin) {
		return exec.CommandContext(ctx, "cmd", append([]string{"/C", bin}, args...)...), nil
	}
	return exec.CommandContext(ctx, bin, args...), nil
}

// manualPath returns the operator-configured executable override, if any.
func (a *Adapter) manualPath() string {
	if a.resolveExecutable != nil {
		return a.resolveExecutable()
	}
	return ""
}

// resolveOpencodePath implements the five-step npm layout resolution above.
func resolveOpencodePath(name string) (string, error) {
	if prefix, err := npmPrefixFor(name + ".cmd"); err == nil {
		if exe, ok := packageExe(prefix); ok {
			return exe, nil
		}
		if exe, ok := platformExe(prefix); ok {
			return exe, nil
		}
		// `npm root -g` reports the node_modules directory itself, so its
		// parent is the comparable prefix.
		if root, err := npmRootGlobal(); err == nil {
			if alt := filepath.Dir(root); !strings.EqualFold(alt, prefix) {
				if exe, ok := packageExe(alt); ok {
					return exe, nil
				}
				if exe, ok := platformExe(alt); ok {
					return exe, nil
				}
			}
		}
		return filepath.Join(prefix, name+".cmd"), nil
	}
	// No npm shim: a native binary on PATH still works.
	return platform.New().Executables().Resolve(name)
}

// npmPrefixFor returns the npm global prefix derived from the directory of
// the shim for binary on PATH. npm always places its shims in the prefix
// root, with packages under <prefix>\node_modules.
func npmPrefixFor(shim string) (string, error) {
	shimPath, err := platform.New().Executables().Resolve(shim)
	if err != nil {
		return "", fmt.Errorf("opencode: npm shim %q not on PATH: %w", shim, err)
	}
	return filepath.Dir(shimPath), nil
}

// packageExe returns the package's own bin/opencode.exe when it is a native
// binary.
func packageExe(prefix string) (string, bool) {
	candidate := filepath.Join(prefix, "node_modules", "@opencode", "cli", "bin", "opencode.exe")
	if isNativeExe(candidate) {
		return candidate, true
	}
	return "", false
}

// platformExe returns the platform optional dependency's bin/opencode.exe
// when present (@opencode/cli-windows-x64 or -arm64), verified native.
// Every present pack is probed: no architecture detection is needed.
func platformExe(prefix string) (string, bool) {
	pattern := filepath.Join(prefix, "node_modules", "@opencode", "cli-windows-*", "bin", "opencode.exe")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", false
	}
	for _, candidate := range matches {
		if isNativeExe(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// isNativeExe reports whether path exists and starts with the MZ header,
// i.e. it is a real PE binary rather than a JS placeholder left behind by
// an incomplete postinstall.
func isNativeExe(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var header [2]byte
	if _, err := file.Read(header[:]); err != nil {
		return false
	}
	return header[0] == 'M' && header[1] == 'Z'
}

// npmRootGlobal runs `npm root -g` as a fallback prefix source.
func npmRootGlobal() (string, error) {
	out, err := exec.Command("npm", "root", "-g").Output()
	if err != nil {
		return "", fmt.Errorf("opencode: npm root -g: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("opencode: npm root -g returned empty")
	}
	return root, nil
}

// isCmdShim reports whether path needs the cmd.exe wrapper.
func isCmdShim(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".cmd" || ext == ".bat"
}
