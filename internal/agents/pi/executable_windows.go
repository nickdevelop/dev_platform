//go:build windows

package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devplatform/internal/platform"
)

// This file resolves the pi executable from the npm global layout. Unlike
// opencode, pi ships no native binary: its manifest points bin.pi at a JS
// bundle (dist/bundle/cli.js in 1.2.0), so the platform runs it with node:
//
//	node <prefix>\node_modules\@earendil-works\pi-coding-agent\<bin.pi> args...
//
// The entry is read from the installed package.json, never hardcoded, so a
// layout change in a future pi release only moves data. Resolution order:
//
//  1. The directory of pi.cmd on PATH is the npm prefix; no npm subprocess
//     is needed for this step.
//  2. <prefix>\node_modules\@earendil-works\pi-coding-agent\package.json
//     bin.pi, verified to exist, run with node from PATH.
//  3. `npm root -g` as a fallback prefix source (its parent directory).
//  4. A native pi binary on PATH when npm is out of the picture.
//  5. The pi.cmd shim itself, run via cmd.exe with fixed args only.
//
// A manual executable override runs as given (cmd-wrapped for .cmd/.bat),
// exactly like the pre-platform behavior.
func init() {
	Descriptor.ResolveExecutable = func(_ string) (string, error) {
		_, display, err := resolvePiBinary()
		return display, err
	}
}

// probePath resolves the executable for pre-flight checks: the JS entry, a
// native binary, or the shim — whichever the launcher would Stat.
func (a *Adapter) probePath() (string, error) {
	if manual := a.manualPath(); manual != "" {
		return manual, nil
	}
	_, display, err := resolvePiBinary()
	return display, err
}

// backendCommand builds a one-shot process to run (version, model list).
func (a *Adapter) backendCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if manual := a.manualPath(); manual != "" {
		return manualCommand(ctx, manual, args), nil
	}
	argv, _, err := resolvePiBinary()
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, argv[0], append(argv[1:], args...)...), nil
}

// resolveSpec rewrites a launch spec for Windows: the node prefix (or the
// cmd wrapper for shims and manual .cmd paths) becomes the executable.
func (a *Adapter) resolveSpec(spec processSpec) (processSpec, error) {
	if manual := a.manualPath(); manual != "" {
		if isCmdShim(manual) {
			spec.executable = "cmd"
			spec.args = append([]string{"/C", manual}, spec.args...)
		}
		return spec, nil
	}
	argv, _, err := resolvePiBinary()
	if err != nil {
		return spec, err
	}
	spec.executable = argv[0]
	spec.args = append(argv[1:], spec.args...)
	return spec, nil
}

// manualPath returns the operator-configured executable override, if any.
func (a *Adapter) manualPath() string {
	if a.resolveExecutable != nil {
		return a.resolveExecutable()
	}
	return ""
}

// piBinary is the runnable form: argv runs it, display is the path the
// settings page and pre-flight checks report.
type piBinary struct {
	argv    []string
	display string
}

// resolvePiBinary implements the resolution order above.
func resolvePiBinary() ([]string, string, error) {
	for _, prefix := range candidatePrefixes() {
		pkgDir := filepath.Join(prefix, "node_modules", "@earendil-works", "pi-coding-agent")
		entry, err := packageBinEntry(pkgDir, "pi")
		if err != nil {
			continue
		}
		node, err := platform.New().Executables().Resolve("node")
		if err != nil {
			return nil, "", fmt.Errorf("pi: node not on PATH: %w", err)
		}
		return []string{node, entry}, entry, nil
	}
	// No npm package: a native binary still works, then the shim via cmd.
	if bin, err := platform.New().Executables().Resolve("pi"); err == nil {
		if !isCmdShim(bin) {
			return []string{bin}, bin, nil
		}
		return []string{"cmd", "/C", bin}, bin, nil
	}
	return nil, "", fmt.Errorf("pi: executable not found on PATH and no npm package installed")
}

// candidatePrefixes returns the npm prefixes to probe: the shim directory,
// then the parent of `npm root -g`, deduplicated.
func candidatePrefixes() []string {
	var out []string
	if prefix, err := npmPrefixFor("pi.cmd"); err == nil {
		out = append(out, prefix)
	}
	if root, err := npmRootGlobal(); err == nil {
		if alt := filepath.Dir(root); alt != "" && !containsFold(out, alt) {
			out = append(out, alt)
		}
	}
	return out
}

// npmPrefixFor returns the npm global prefix derived from the directory of
// the shim for binary on PATH.
func npmPrefixFor(shim string) (string, error) {
	shimPath, err := platform.New().Executables().Resolve(shim)
	if err != nil {
		return "", fmt.Errorf("pi: npm shim %q not on PATH: %w", shim, err)
	}
	return filepath.Dir(shimPath), nil
}

// packageBinEntry reads <pkgDir>\package.json bin[name] and resolves it to
// an existing file, so manifest layout changes do not need code changes.
func packageBinEntry(pkgDir, name string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(pkgDir, "package.json"))
	if err != nil {
		return "", err
	}
	var manifest struct {
		Bin map[string]string `json:"bin"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("pi: parse package manifest: %w", err)
	}
	target, ok := manifest.Bin[name]
	if !ok || target == "" {
		return "", fmt.Errorf("pi: package manifest has no bin %q", name)
	}
	entry := filepath.Join(pkgDir, filepath.FromSlash(target))
	if info, err := os.Stat(entry); err != nil || info.IsDir() {
		return "", fmt.Errorf("pi: package entry %q missing", entry)
	}
	return entry, nil
}

// npmRootGlobal runs `npm root -g` as a fallback prefix source.
func npmRootGlobal() (string, error) {
	out, err := exec.Command("npm", "root", "-g").Output()
	if err != nil {
		return "", fmt.Errorf("pi: npm root -g: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("pi: npm root -g returned empty")
	}
	return root, nil
}

// manualCommand builds a process for an operator-configured override,
// cmd-wrapping .cmd/.bat targets like every other Windows launch.
func manualCommand(ctx context.Context, manual string, args []string) *exec.Cmd {
	if isCmdShim(manual) {
		return exec.CommandContext(ctx, "cmd", append([]string{"/C", manual}, args...)...)
	}
	return exec.CommandContext(ctx, manual, args...)
}

// isCmdShim reports whether path needs the cmd.exe wrapper.
func isCmdShim(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".cmd" || ext == ".bat"
}

func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}
