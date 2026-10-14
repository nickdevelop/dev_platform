//go:build unix

package unix_test

import (
	"errors"
	"strings"
	"testing"

	"devplatform/internal/platform"
	"devplatform/internal/platform/unix"
)

const (
	nativeLinuxVersion = "Linux version 6.8.0-52-generic (buildd@lcy02-amd64-007) (gcc-13) #60-Ubuntu SMP"
	wsl2Version        = "Linux version 5.15.146.1-microsoft-standard-WSL2 (root@aaa) (gcc-11) #1 SMP"
	wsl1Version        = "Linux version 4.4.0-19041-Microsoft (Microsoft@Microsoft.com) (gcc-5.4) #1-Microsoft"
)

func versionReader(version string, err error) func() (string, error) {
	return func() (string, error) { return version, err }
}

// TestEnvironmentCheckRejectsWSL1 verifies the four version inputs: native
// Linux and WSL2 start, WSL1 is refused with an upgrade hint, and an
// unreadable /proc/version does not block startup.
func TestEnvironmentCheckRejectsWSL1(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version string
		readErr error
		wantErr bool
	}{
		{"native linux", nativeLinuxVersion, nil, false},
		{"WSL2", wsl2Version, nil, false},
		{"WSL1", wsl1Version, nil, true},
		{"unreadable version", "", errors.New("no proc"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := unix.Environment{VersionReader: versionReader(tt.version, tt.readErr)}
			err := env.Check()
			if tt.wantErr && err == nil {
				t.Fatal("Check() = nil, want WSL1 refusal")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Check() = %v, want nil", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "WSL2") {
				t.Errorf("Check() = %v, want an upgrade hint", err)
			}
		})
	}
}

// TestEnvironmentDescribeWarnsOnDrvFsDataDir verifies the data-directory
// warnings: a /mnt/<drive> location warns without blocking, a native
// location stays quiet, and WSL2 is reported.
func TestEnvironmentDescribeWarnsOnDrvFsDataDir(t *testing.T) {
	native := unix.Environment{VersionReader: versionReader(nativeLinuxVersion, nil)}
	if got := native.Describe(t.TempDir()); len(got) != 0 {
		t.Errorf("Describe(native dir) = %q, want quiet", got)
	}

	wsl2 := unix.Environment{VersionReader: versionReader(wsl2Version, nil)}
	drvfs := "/mnt/c/Users/someone/.config/devplatform"
	lines := wsl2.Describe(drvfs)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "WSL2") {
		t.Errorf("Describe(WSL2) = %q, want the WSL2 marker", lines)
	}
	if !strings.Contains(joined, "/mnt/") || !strings.Contains(joined, drvfs) {
		t.Errorf("Describe(%q) = %q, want the DrvFs warning", drvfs, lines)
	}

	// Near-misses stay quiet: /mnt itself, /mnt/data (no drive letter),
	// and a native path merely containing the letters.
	for _, dir := range []string{"/mnt", "/mnt/data", "/home/user/mnt/c"} {
		if got := native.Describe(dir); len(got) != 0 {
			t.Errorf("Describe(%q) = %q, want quiet", dir, got)
		}
	}
	// The drive root itself already counts as DrvFs.
	if got := native.Describe("/mnt/c"); len(got) == 0 {
		t.Error("Describe(/mnt/c) is quiet, want the DrvFs warning")
	}
}

// TestEnvironmentPortIsWired verifies the port exposes working checks, not
// the nil placeholder.
func TestEnvironmentPortIsWired(t *testing.T) {
	env := platform.New().Environment()
	if env == nil {
		t.Fatal("Environment() is nil")
	}
	// The test machine runs the real /proc/version: whatever it says must
	// not block the suite itself.
	if err := env.Check(); err != nil {
		t.Fatalf("Check() on the test machine = %v", err)
	}
	_ = env.Describe(t.TempDir())
}
