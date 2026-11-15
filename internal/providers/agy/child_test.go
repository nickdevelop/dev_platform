package agy

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A child that never prints init is killed and reaped when the start times
// out: its pid must not survive as a zombie of the platform.
func TestStartTimeoutReapsChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	executable := writeFakeAgy(t, `echo $$ > "`+pidFile+`"
sleep 30`)
	inst := newInstance("inst-reap", t.TempDir(), time.Now)
	child := newAgyChild(inst, "", "", PermissionModeConfirm, time.Minute, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := child.start(ctx, executable, nil, nil); err == nil {
		t.Fatal("start must fail when init never arrives")
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse pid %q: %v", raw, err)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("child %d survives the failed start (kill(0) = %v), want it reaped", pid, err)
	}
}

// TestAgyChildInitUnderTerminal starts the real agy the way the platform does
// and requires its init line. It needs a controlling terminal, so it runs only
// under script(1):
//
//	AGY_TTY_E2E=1 script -qfc "go test -run TestAgyChildInitUnderTerminal ./internal/providers/agy/" /dev/null
func TestAgyChildInitUnderTerminal(t *testing.T) {
	if os.Getenv("AGY_TTY_E2E") != "1" {
		t.Skip("set AGY_TTY_E2E=1 under a terminal (script -qfc) to run the real agy")
	}
	inst := newInstance("inst-tty", t.TempDir(), time.Now)
	child := newAgyChild(inst, "", "", PermissionModeConfirm, time.Minute, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := child.start(ctx, "agy", nil, nil); err != nil {
		t.Fatalf("start under a terminal: %v", err)
	}
	defer child.close()
	if child.sessionID == "" {
		t.Fatal("init must report a conversation id")
	}
}
