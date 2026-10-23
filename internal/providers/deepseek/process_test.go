package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDefaultStartupTimeoutAccommodatesDshBoot pins the production startup
// default. A real `dsh --profile web` needs 4.9-5.3s to print its launch URL
// (measured against dsh 0.2.1-rc.2), and a default cut below that made every
// backend call fail with "dsh startup timed out waiting for launch url" —
// surfaced to the operator as POST /agent-backends/{backend}/models/refresh
// answering 502 model_list_failed. Test harnesses that want a fast failure set
// DEEPSEEK_STARTUP_TIMEOUT; they must not shrink this default to get one.
func TestDefaultStartupTimeoutAccommodatesDshBoot(t *testing.T) {
	const measuredDshBoot = 5500 * time.Millisecond

	if defaultStartupTimeout < 2*measuredDshBoot {
		t.Errorf("defaultStartupTimeout = %s, want at least twice the measured dsh boot of %s",
			defaultStartupTimeout, measuredDshBoot)
	}
}

func TestResolveStartupTimeout(t *testing.T) {
	t.Setenv("DEEPSEEK_STARTUP_TIMEOUT", "")
	if got := resolveStartupTimeout(0); got != defaultStartupTimeout {
		t.Errorf("resolveStartupTimeout(0) = %s, want the default %s", got, defaultStartupTimeout)
	}
	if got := resolveStartupTimeout(7 * time.Second); got != 7*time.Second {
		t.Errorf("resolveStartupTimeout(7s) = %s, want the configured 7s", got)
	}

	t.Setenv("DEEPSEEK_STARTUP_TIMEOUT", "250ms")
	if got := resolveStartupTimeout(0); got != 250*time.Millisecond {
		t.Errorf("override resolveStartupTimeout(0) = %s, want 250ms", got)
	}
	if got := resolveStartupTimeout(7 * time.Second); got != 250*time.Millisecond {
		t.Errorf("override resolveStartupTimeout(7s) = %s, want 250ms", got)
	}

	t.Setenv("DEEPSEEK_STARTUP_TIMEOUT", "not-a-duration")
	if got := resolveStartupTimeout(0); got != defaultStartupTimeout {
		t.Errorf("invalid override resolveStartupTimeout(0) = %s, want the default %s", got, defaultStartupTimeout)
	}
}

// writeStubDsh writes an executable stand-in for dsh. The body is run with the
// launcher flags as arguments, so it can ignore them.
func writeStubDsh(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "dsh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write stub dsh: %v", err)
	}
	return path
}

// TestStartProcessAbortsWhenDshNeverAnnounces checks the startup deadline is
// actually enforced and reported: a child that prints no launch URL must fail
// fast with the startup-timeout error rather than hang the caller.
func TestStartProcessAbortsWhenDshNeverAnnounces(t *testing.T) {
	t.Setenv("DEEPSEEK_STARTUP_TIMEOUT", "")
	executable := writeStubDsh(t, "sleep 30")

	started := time.Now()
	_, err := StartProcess(context.Background(), executable, t.TempDir(), nil, 250*time.Millisecond)
	elapsed := time.Since(started)

	if err == nil || !strings.Contains(err.Error(), "startup timed out") {
		t.Fatalf("StartProcess error = %v, want a startup timeout", err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("StartProcess took %s to report the timeout, want a prompt failure", elapsed)
	}
}

// TestStartProcessReadsLaunchURLAfterSlowBoot walks the whole boot path against
// a stub dsh that only prints its launch URL after a delay: the URL, the token
// exchange and the mux event binding must all still complete.
func TestStartProcessReadsLaunchURLAfterSlowBoot(t *testing.T) {
	t.Setenv("DEEPSEEK_STARTUP_TIMEOUT", "")

	server := newStubDshServer(t)
	defer server.Close()

	executable := writeStubDsh(t, fmt.Sprintf(
		"sleep 1\necho \"dsh web: %s/?token=stub-token\"\nsleep 30\n", server.URL,
	))

	proc, err := StartProcess(context.Background(), executable, t.TempDir(), nil, 20*time.Second)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	defer func() { _ = proc.Shutdown(context.Background()) }()

	if proc.LaunchURL() == "" {
		t.Error("LaunchURL is empty, want the URL the stub printed")
	}
	if proc.Client() == nil || proc.Client().ClientID() != "stub-client" {
		t.Errorf("client id = %q, want the id the stub's $events stream reported", proc.Client().ClientID())
	}
}

// newStubDshServer answers the token exchange and the mux event stream the same
// way a live dsh host does, enough for StartProcess to finish its boot.
func newStubDshServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/connect":
			http.SetCookie(w, &http.Cookie{Name: "dsh-auth", Value: "stub"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/remote.mux":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("mux upgrade: %v", err)
				return
			}
			for {
				var msg wsClientMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "open" && msg.Endpoint == "$events" {
					if err := conn.WriteJSON(wsServerMessage{
						Type:     "item",
						StreamID: msg.StreamID,
						Value:    json.RawMessage(`{"type":"ready","clientId":"stub-client"}`),
					}); err != nil {
						return
					}
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}
