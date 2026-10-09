package main

// Black-box tests for the deployment/runtime hardening contract of the
// platform binary. They build the real cmd/platform binary, run it as a
// subprocess against an isolated data directory, and assert:
//
//   - a second process on the same data directory fails to start with a
//     clear error (single-instance lock scoped to the data directory),
//   - the listen address is configurable (LAN bind config) and the log
//     reports the concrete address the server bound,
//   - passwords and session tokens never appear in captured server logs,
//   - SIGTERM triggers a graceful shutdown with exit status 0.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"devplatform/internal/auth"
	"devplatform/internal/database"
)

// listenLogPattern matches the "platform listening on <addr>" startup line.
var listenLogPattern = regexp.MustCompile(`platform listening on (\S+)`)

// lockedBuffer is a concurrency-safe capture of subprocess output.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// repoRoot returns the module root, two levels above cmd/platform.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// buildPlatformServer compiles the real platform binary into a temp dir.
func buildPlatformServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "platform")
	build := exec.Command("go", "build", "-o", bin, "./cmd/platform")
	build.Dir = repoRoot(t)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/platform: %v", err)
	}
	return bin
}

// prepareDataDir creates a platform data directory with the given password
// configured, mirroring what `platform set-password` produces.
func prepareDataDir(t *testing.T, password string) string {
	t.Helper()
	dataDir := t.TempDir()
	db, err := database.Open(dataDir)
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	if err := auth.SetPassword(db, password); err != nil {
		t.Fatalf("auth.SetPassword: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	return dataDir
}

// serverProcess is a running platform server subprocess under test.
type serverProcess struct {
	cmd    *exec.Cmd
	logs   *lockedBuffer
	waitCh chan error

	mu       sync.Mutex
	exited   bool
	stopOnce sync.Once
	waitErr  error
}

// stop terminates the server with SIGTERM (the graceful shutdown signal),
// waits for it to exit, and returns the resulting wait error. It is safe to
// call more than once and is also registered as the test cleanup.
func (p *serverProcess) stop() error {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		exited := p.exited
		p.mu.Unlock()
		if !exited {
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case p.waitErr = <-p.waitCh:
		case <-time.After(15 * time.Second):
			_ = p.cmd.Process.Kill()
			p.waitErr = <-p.waitCh
		}
	})
	return p.waitErr
}

// startServer runs the platform binary with the given data directory and
// listen address, waits until the startup log reports a concrete bound
// address and /api/health answers, and returns the process handle. The
// subprocess is stopped when the test finishes.
func startServer(t *testing.T, bin, dataDir, listenAddr string) *serverProcess {
	t.Helper()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"PLATFORM_DATA_DIR="+dataDir,
		"PLATFORM_LISTEN_ADDR="+listenAddr,
	)
	logs := &lockedBuffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start platform server: %v", err)
	}

	p := &serverProcess{cmd: cmd, logs: logs, waitCh: make(chan error, 1)}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.exited = true
		p.mu.Unlock()
		p.waitCh <- err
	}()
	t.Cleanup(func() { _ = p.stop() })

	// Wait for the startup log line reporting a concrete (non-zero) address,
	// then confirm the API actually answers on it.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if m := listenLogPattern.FindStringSubmatch(logs.String()); m != nil {
			if host, port, err := net.SplitHostPort(m[1]); err == nil && port != "0" && port != "" {
				resp, err := http.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						return p
					}
				}
			}
		}
		p.mu.Lock()
		exited := p.exited
		p.mu.Unlock()
		if exited {
			t.Fatalf("platform server exited before becoming ready:\n%s", logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("platform server never reported a concrete listen address; logs:\n%s", logs.String())
	return p
}

// newAuthedClient builds a cookie-jar client and returns it with a CSRF token.
func newAuthedClient(t *testing.T, base string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	resp, err := client.Get(base + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	resp.Body.Close()
	if body.CSRFToken == "" {
		t.Fatal("session endpoint returned an empty CSRF token")
	}
	return client, body.CSRFToken
}

// postLogin attempts a password login with the CSRF token and returns the
// response (for status and cookie inspection).
func postLogin(t *testing.T, client *http.Client, base, csrf, password string) *http.Response {
	t.Helper()
	payload := fmt.Sprintf(`{"password":%q}`, password)
	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	return resp
}

// TestSecondProcessOnSameDataDirFailsToStart is the end-to-end single-instance
// contract: while one platform process serves a data directory, a second
// process pointing at the same data directory must exit non-zero with a clear
// error naming the data directory.
func TestSecondProcessOnSameDataDirFailsToStart(t *testing.T) {
	bin := buildPlatformServer(t)
	password := fmt.Sprintf("e2e-secret-%d", time.Now().UnixNano())
	dataDir := prepareDataDir(t, password)

	startServer(t, bin, dataDir, "127.0.0.1:0")

	// The second process must fail on the lock. It may never bind anything,
	// so no listen address is needed; a context bounds a regression where
	// the second process would instead start serving.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	second := exec.CommandContext(ctx, bin)
	second.Env = append(os.Environ(), "PLATFORM_DATA_DIR="+dataDir)
	output, err := second.CombinedOutput()

	if ctx.Err() != nil {
		t.Fatalf("second process neither failed nor exited on its own; it was killed after the deadline\noutput:\n%s", output)
	}
	if err == nil {
		t.Fatalf("second process on the same data directory started successfully; want lock rejection\noutput:\n%s", output)
	}
	if !strings.Contains(string(output), "already in use") {
		t.Errorf("second-process error output %q does not explain that the data directory is already in use", string(output))
	}
	if !strings.Contains(string(output), dataDir) {
		t.Errorf("second-process error output %q does not name the data directory %q", string(output), dataDir)
	}
}

// TestServerBindsConfiguredLANAddress verifies the listen address comes from
// PLATFORM_LISTEN_ADDR and the startup log reports the concrete bound
// address, so operators (and tests) never depend on a specific port.
func TestServerBindsConfiguredLANAddress(t *testing.T) {
	bin := buildPlatformServer(t)
	dataDir := prepareDataDir(t, "unused-here")

	p := startServer(t, bin, dataDir, "127.0.0.1:0")
	bound := listenLogPattern.FindStringSubmatch(p.logs.String())[1]

	host, port, err := net.SplitHostPort(bound)
	if err != nil {
		t.Fatalf("reported address %q: %v", bound, err)
	}
	if port == "0" || port == "" {
		t.Errorf("server reported binding port 0 instead of the concrete port; logs:\n%s", p.logs.String())
	}
	if host != "127.0.0.1" {
		t.Errorf("bound host = %q, want the configured 127.0.0.1", host)
	}
}

// TestServerLogsNeverContainSecrets drives a failed and a successful login
// against the real server and asserts the attempted passwords and the issued
// session token never appear anywhere in the captured server output.
func TestServerLogsNeverContainSecrets(t *testing.T) {
	bin := buildPlatformServer(t)
	password := fmt.Sprintf("e2e-secret-%d", time.Now().UnixNano())
	wrongPassword := password + "-wrong"
	dataDir := prepareDataDir(t, password)

	p := startServer(t, bin, dataDir, "127.0.0.1:0")
	base := "http://" + listenLogPattern.FindStringSubmatch(p.logs.String())[1]

	client, csrf := newAuthedClient(t, base)

	// A failed login must not leak the attempted secret.
	bad := postLogin(t, client, base, csrf, wrongPassword)
	io.Copy(io.Discard, bad.Body)
	bad.Body.Close()
	if bad.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong-password login status = %d, want %d", bad.StatusCode, http.StatusUnauthorized)
	}

	// A successful login issues the session whose token must never be logged.
	good := postLogin(t, client, base, csrf, password)
	if good.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(good.Body)
		good.Body.Close()
		t.Fatalf("correct-password login status = %d, want %d: %s", good.StatusCode, http.StatusOK, body)
	}
	var sessionID string
	for _, cookie := range good.Cookies() {
		if cookie.Name == "platform_session" {
			sessionID = cookie.Value
		}
	}
	good.Body.Close()
	if sessionID == "" {
		t.Fatal("successful login did not set a platform_session cookie")
	}

	// Gracefully stop so every log line is flushed into the capture.
	if err := p.stop(); err != nil {
		t.Fatalf("server did not shut down cleanly: %v\nlogs:\n%s", err, p.logs.String())
	}

	captured := p.logs.String()
	for _, secret := range []string{password, wrongPassword, sessionID} {
		if strings.Contains(captured, secret) {
			t.Errorf("captured server logs contain the secret %q", secret)
		}
	}
}

// TestServerShutsDownGracefullyOnSigterm verifies SIGTERM ends the process
// with exit status 0 via the graceful shutdown path.
func TestServerShutsDownGracefullyOnSigterm(t *testing.T) {
	bin := buildPlatformServer(t)
	dataDir := prepareDataDir(t, "unused-here")

	p := startServer(t, bin, dataDir, "127.0.0.1:0")

	if err := p.stop(); err != nil {
		t.Fatalf("SIGTERM shutdown exit error = %v, want clean exit\nlogs:\n%s", err, p.logs.String())
	}
	if !strings.Contains(p.logs.String(), "shutting down") {
		t.Errorf("logs do not show the graceful shutdown path:\n%s", p.logs.String())
	}
}
