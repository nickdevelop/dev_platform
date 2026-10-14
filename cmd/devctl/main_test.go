package main

import (
	"os"
	"reflect"
	"testing"
)

// TestBuildStepsAssembly pins the recipe: with missing dependencies the npm
// install comes first, then the frontend and server builds with the
// configured directories.
func TestBuildStepsAssembly(t *testing.T) {
	steps := buildSteps("web/dist", "bin/platform", true)
	want := []step{
		{name: "install frontend dependencies", dir: "web", argv: []string{"npm", "ci"}},
		{name: "build frontend -> web/dist", dir: "web", argv: []string{"npm", "run", "build"}},
		{name: "build server -> bin/platform", dir: ".", argv: []string{"go", "build", "-o", "bin/platform", "./cmd/platform"}},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("buildSteps(install) = %+v, want %+v", steps, want)
	}

	steps = buildSteps("custom/dist", "bin/platform", false)
	if len(steps) != 2 || steps[0].argv[0] != "npm" {
		t.Fatalf("buildSteps(no install) = %+v, want frontend and server builds only", steps)
	}
	if steps[0].dir != "web" || steps[1].dir != "." {
		t.Fatalf("buildSteps dirs = %q, %q; want web and repo root", steps[0].dir, steps[1].dir)
	}
}

// TestRunCommandAssembly pins the foreground launch: the flag plumbing and
// the data-dir passthrough.
func TestRunCommandAssembly(t *testing.T) {
	cmd := runCommand("/repo", "127.0.0.1:18080", "web/dist", "")
	want := []string{"go", "run", "./cmd/platform", "-listen", "127.0.0.1:18080", "-web-dir", "web/dist"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("runCommand args = %q, want %q", cmd.Args, want)
	}
	if cmd.Dir != "/repo" {
		t.Fatalf("runCommand dir = %q, want the repository root", cmd.Dir)
	}

	cmd = runCommand("/repo", "0.0.0.0:18080", "web/dist", "/data")
	joined := ""
	for _, arg := range cmd.Args {
		joined += " " + arg
	}
	for _, want := range []string{"-data-dir", "/data"} {
		found := false
		for _, arg := range cmd.Args {
			if arg == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("runCommand args = %q, want %q for the data dir", joined, want)
		}
	}
}

// TestRunCommandIsForeground pins that run never backgrounds: standard
// stdio flows straight through on every platform, so Ctrl+C (or closing
// the terminal window on Windows) reaches the platform process.
func TestRunCommandIsForeground(t *testing.T) {
	cmd := runCommand("/repo", "0.0.0.0:18080", "web/dist", "")
	if cmd.Stdin != os.Stdin || cmd.Stdout != os.Stdout || cmd.Stderr != os.Stderr {
		t.Fatal("runCommand does not wire standard stdio straight through; run must stay a foreground process")
	}
}
