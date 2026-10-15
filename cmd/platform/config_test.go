package main

// Unit tests for the deployment configuration resolution: every setting is
// configurable by flag, by environment variable, and has a documented
// default, so operators can bind the service for LAN access and relocate
// the data directory without touching code.

import (
	"os"
	"path/filepath"
	"testing"

	"devplatform/internal/core/platform"
)

// writeIndex creates an index.html at path, marking a built web UI.
func writeIndex(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("<!doctype html><html></html>"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestResolveListenAddr verifies the precedence: flag > PLATFORM_LISTEN_ADDR
// > :8080. The default binds every interface so the platform is reachable
// from the LAN; loopback is opt-in via an explicit host.
func TestResolveListenAddr(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		envValue  string
		want      string
	}{
		{name: "flag wins", flagValue: "192.168.1.10:9000", envValue: "127.0.0.1:1", want: "192.168.1.10:9000"},
		{name: "env when no flag", flagValue: "", envValue: "127.0.0.1:18080", want: "127.0.0.1:18080"},
		{name: "empty env falls back to default", flagValue: "", envValue: "", want: ":8080"},
		{name: "port-only env binds all interfaces", flagValue: "", envValue: ":9000", want: ":9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLATFORM_LISTEN_ADDR", tt.envValue)
			if got := resolveListenAddr(tt.flagValue); got != tt.want {
				t.Errorf("resolveListenAddr(%q) = %q, want %q", tt.flagValue, got, tt.want)
			}
		})
	}
}

// TestResolveDataDir verifies the precedence: flag > PLATFORM_DATA_DIR >
// user config directory > working-directory fallback.
func TestResolveDataDir(t *testing.T) {
	flagDir := t.TempDir()

	t.Run("flag wins", func(t *testing.T) {
		t.Setenv("PLATFORM_DATA_DIR", "/somewhere/else")
		if got := resolveDataDir(flagDir); got != flagDir {
			t.Errorf("resolveDataDir(flag) = %q, want %q", got, flagDir)
		}
	})

	t.Run("env when no flag", func(t *testing.T) {
		t.Setenv("PLATFORM_DATA_DIR", "/somewhere/else")
		if got := resolveDataDir(""); got != "/somewhere/else" {
			t.Errorf("resolveDataDir(\"\") = %q, want the env value", got)
		}
	})

	t.Run("user config dir by default", func(t *testing.T) {
		t.Setenv("PLATFORM_DATA_DIR", "")
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)
		// Skip by capability, not by GOOS: the XDG assertion only runs
		// where the platform config dir follows XDG_CONFIG_HOME.
		if base, err := platform.New().Dirs().ConfigDir(); err != nil || base != configHome {
			t.Skip("platform config dir does not follow XDG_CONFIG_HOME here")
		}
		if got := resolveDataDir(""); got != filepath.Join(configHome, "devplatform") {
			t.Errorf("resolveDataDir(\"\") = %q, want <config dir>/devplatform", got)
		}
	})

	t.Run("working-directory fallback without a config dir", func(t *testing.T) {
		t.Setenv("PLATFORM_DATA_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		if got := resolveDataDir(""); got != filepath.Join(".", "devplatform-data") {
			t.Errorf("resolveDataDir(\"\") = %q, want ./devplatform-data", got)
		}
	})
}

// TestResolveWebDir verifies the precedence: flag > PLATFORM_WEB_DIR >
// ./web/dist when the UI has been built > static hosting disabled.
func TestResolveWebDir(t *testing.T) {
	flagDir := t.TempDir()
	writeIndex(t, filepath.Join(flagDir, "index.html"))

	t.Run("flag wins", func(t *testing.T) {
		t.Setenv("PLATFORM_WEB_DIR", "/somewhere/ui")
		if got := resolveWebDir(flagDir); got != flagDir {
			t.Errorf("resolveWebDir(flag) = %q, want %q", got, flagDir)
		}
	})

	t.Run("env when no flag", func(t *testing.T) {
		t.Setenv("PLATFORM_WEB_DIR", "/somewhere/ui")
		if got := resolveWebDir(""); got != "/somewhere/ui" {
			t.Errorf("resolveWebDir(\"\") = %q, want the env value", got)
		}
	})

	t.Run("repository build output when it exists", func(t *testing.T) {
		t.Setenv("PLATFORM_WEB_DIR", "")
		t.Chdir(t.TempDir())
		writeIndex(t, filepath.Join("web", "dist", "index.html"))
		if got := resolveWebDir(""); got != filepath.Join("web", "dist") {
			t.Errorf("resolveWebDir(\"\") = %q, want web/dist", got)
		}
	})

	t.Run("disabled when nothing is built", func(t *testing.T) {
		t.Setenv("PLATFORM_WEB_DIR", "")
		t.Chdir(t.TempDir())
		if got := resolveWebDir(""); got != "" {
			t.Errorf("resolveWebDir(\"\") = %q, want empty (static hosting disabled)", got)
		}
	})
}
