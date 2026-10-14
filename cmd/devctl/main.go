// Command devctl is the cross-platform development tool for the AI
// development platform. It runs wherever Go runs (Linux, WSL, Windows)
// with no bash or make dependency:
//
//	devctl build   Build the frontend (web/dist) and the server (bin/platform)
//	devctl run     Run the server in the foreground from source
//	devctl help    Show this help
//
// Background process management (start/stop/restart/status/logs) stays in
// dev.sh on Linux/WSL. devctl never stops anything: it builds and runs in
// the foreground only, so it can never touch the machine-wide opencode
// service either — only the platform's own process.
//
// Environment overrides (same names as dev.sh):
//
//	DEV_LISTEN_ADDR   Listen address            (default 0.0.0.0:18080)
//	DEV_DATA_DIR      Platform data directory   (default: the platform's own)
//	DEV_WEB_DIR       Built web assets dir      (default web/dist)
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// step is one external command in a recipe. Steps run in order with the
// user's environment; a failing step aborts the recipe.
type step struct {
	name string
	dir  string
	argv []string
}

// buildSteps assembles the build recipe: frontend dependencies (when
// missing), the frontend build, then the Go server. It shells out to npm
// and go directly so Windows needs neither make nor bash.
func buildSteps(webDir, binary string, installDeps bool) []step {
	var steps []step
	if installDeps {
		steps = append(steps, step{name: "install frontend dependencies", dir: "web", argv: []string{"npm", "ci"}})
	}
	steps = append(steps,
		step{name: "build frontend -> " + webDir, dir: "web", argv: []string{"npm", "run", "build"}},
		step{name: "build server -> " + binary, dir: ".", argv: []string{"go", "build", "-o", binary, "./cmd/platform"}},
	)
	return steps
}

// runCommand builds the foreground server process: standard stdio straight
// through, so Ctrl+C (or closing the terminal window) reaches the platform
// and triggers its normal shutdown. The process is never backgrounded on
// any platform.
func runCommand(root, listen, webDir, dataDir string) *exec.Cmd {
	argv := []string{"run", "./cmd/platform", "-listen", listen, "-web-dir", webDir}
	if dataDir != "" {
		argv = append(argv, "-data-dir", dataDir)
	}
	cmd := exec.Command("go", argv...)
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// repoRoot walks up from the working directory to the repository root,
// recognized by go.mod and the web directory.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if stat, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !stat.IsDir() {
			if info, err := os.Stat(filepath.Join(dir, "web")); err == nil && info.IsDir() {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("devctl: no repository root (go.mod + web/) above the working directory")
		}
		dir = parent
	}
}

func getenv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// runBuild executes the build recipe from the repository root.
func runBuild(webDir, binary string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	_, statErr := os.Stat(filepath.Join(root, "web", "node_modules"))
	steps := buildSteps(webDir, binary, os.IsNotExist(statErr))
	for _, s := range steps {
		fmt.Printf("[devctl] %s\n", s.name)
		cmd := exec.Command(s.argv[0], s.argv[1:]...)
		cmd.Dir = filepath.Join(root, s.dir)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("devctl: %s: %w", s.name, err)
		}
	}
	fmt.Println("[devctl] build complete")
	return nil
}

// runRun launches the server in the foreground from the repository root.
func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	listen := fs.String("listen", getenv("DEV_LISTEN_ADDR", "0.0.0.0:18080"), "HTTP listen address host:port")
	webDir := fs.String("web-dir", getenv("DEV_WEB_DIR", "web/dist"), "directory of built web UI assets to serve")
	dataDir := fs.String("data-dir", getenv("DEV_DATA_DIR", ""), "platform data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("devctl run takes only flags; see devctl help")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	fmt.Printf("[devctl] running server on %s (foreground; Ctrl+C stops it)\n", *listen)
	return runCommand(root, *listen, *webDir, *dataDir).Run()
}

func usage() {
	fmt.Print(`devctl — cross-platform development tool (Linux, WSL, Windows)

Usage:
  devctl build             Build the frontend (web/dist) and server (bin/platform)
  devctl run [flags]       Run the server in the foreground from source
  devctl help              Show this help

run flags:
  -listen <host:port>      HTTP listen address (default $DEV_LISTEN_ADDR, else 0.0.0.0:18080)
  -web-dir <dir>           Built web assets dir (default $DEV_WEB_DIR, else web/dist)
  -data-dir <dir>          Platform data directory (default $DEV_DATA_DIR, else the platform's own)

Background management (start/stop/restart/status/logs) stays in dev.sh and
works on Linux/WSL only. devctl never stops any process — foreground only —
so it cannot disturb the machine-wide opencode service.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "build":
		if len(os.Args) > 2 {
			err = errors.New("devctl build takes no arguments; see devctl help")
		} else {
			err = runBuild(getenv("DEV_WEB_DIR", "web/dist"), "bin/platform")
		}
	case "run":
		err = runRun(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		err = fmt.Errorf("devctl: unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "devctl: %v\n", err)
		os.Exit(1)
	}
}
