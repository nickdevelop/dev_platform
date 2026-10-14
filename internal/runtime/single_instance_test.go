package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/platform"
	"devplatform/internal/runtime"
)

// TestAcquireLockRejectsSecondProcessOnSameDataDir verifies the core
// single-instance contract: one process per data directory. A second lock on
// the same data directory must fail with a clear error, a lock on a different
// data directory must succeed concurrently, and a released lock must be
// reacquirable.
func TestAcquireLockRejectsSecondProcessOnSameDataDir(t *testing.T) {
	dataDir := t.TempDir()

	first, err := runtime.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("AcquireLock(first): %v", err)
	}
	t.Cleanup(func() { _ = first.Release() })

	second, err := runtime.AcquireLock(dataDir)
	if err == nil {
		_ = second.Release()
		t.Fatal("AcquireLock(second) on the same data directory succeeded, want rejection")
	}
	// The error must tell the operator what is wrong and with which data
	// directory, so a confusing startup failure is diagnosable.
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error %q does not explain that the data directory is already in use", err)
	}
	if !strings.Contains(err.Error(), dataDir) {
		t.Errorf("error %q does not name the locked data directory %q", err, dataDir)
	}

	// A different data directory is independent and may run concurrently.
	other, err := runtime.AcquireLock(filepath.Join(dataDir, "..", "other-data"))
	if err != nil {
		t.Errorf("AcquireLock(other data directory): %v", err)
	} else {
		_ = other.Release()
	}
}

// TestAcquireLockIsReacquirableAfterRelease verifies that releasing the lock
// lets the next process take it, and that releasing twice is safe.
func TestAcquireLockIsReacquirableAfterRelease(t *testing.T) {
	dataDir := t.TempDir()

	lock, err := runtime.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Errorf("second Release: %v", err)
	}

	again, err := runtime.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("AcquireLock after release: %v", err)
	}
	_ = again.Release()
}

// TestAcquireLockRestrictsDataDirAndLockPermissions verifies the lock file
// and its data directory are owner-only on Unix, so a local user cannot read
// platform state (the directory also holds the session database).
func TestAcquireLockRestrictsDataDirAndLockPermissions(t *testing.T) {
	// Skip by capability, not by GOOS. Perms is wired, so the port is the
	// source of truth on every platform.
	if !platform.New().Perms().Enforced() {
		t.Skip("private permissions are not enforced on this platform")
	}

	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	lock, err := runtime.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	dirInfo, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("stat data directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("data directory mode = %o, want 700", got)
	}

	lockInfo, err := os.Stat(filepath.Join(dataDir, "platform.lock"))
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	if got := lockInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("lock file mode = %o, want 600", got)
	}
}

// TestAcquireLockCreatesMissingDataDir verifies the lock bootstraps the data
// directory like database.Open does, so startup order does not matter.
func TestAcquireLockCreatesMissingDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "not-created-yet")

	lock, err := runtime.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("AcquireLock on missing data directory: %v", err)
	}
	defer lock.Release()

	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		t.Errorf("data directory was not created: info=%v err=%v", info, err)
	}
}
