// Package platform is the OS port layer: every OS-dependent capability the
// business packages need is expressed as an interface here, with one
// implementation per platform under unix/ and windows/ plus a controllable
// fake/ for unit tests. Business packages must not import syscall,
// golang.org/x/sys/*, or branch on runtime.GOOS; see guard_test.go.
package platform

import (
	"errors"
	"os"
	"os/exec"
)

// Platform aggregates every OS capability. The composition root creates it
// once via New and injects it into the services.
type Platform interface {
	// Name reports runtime.GOOS. It is for diagnostics only; business
	// code must never branch on it.
	Name() string
	Dirs() Dirs
	Instance() Instance
	Process() Process
	Executables() Executables
	Perms() Perms
	Shutdown() Shutdown
	Terminal() Terminal
	Environment() Environment
}

// Dirs resolves platform directories and compares user-supplied paths.
type Dirs interface {
	// HomeDir is the current user's home directory.
	HomeDir() (string, error)
	// ConfigDir holds per-user configuration.
	// unix: $XDG_CONFIG_HOME or ~/.config; windows: %APPDATA%.
	ConfigDir() (string, error)
	// DataDir holds per-user data.
	// unix: $XDG_DATA_HOME or ~/.local/share; windows: %LOCALAPPDATA%.
	DataDir() (string, error)
	// StateDir holds per-user state such as the opencode service file.
	// unix: $XDG_STATE_HOME or ~/.local/state;
	// windows: $XDG_STATE_HOME or %USERPROFILE%\.local\state.
	StateDir() (string, error)
	// TempDir is the directory for temporary files.
	TempDir() string
	// CanonicalizeUserPath normalizes a user-supplied path. unix keeps the
	// C:\ -> /mnt/c translation; windows keeps drive letters and UNC paths.
	CanonicalizeUserPath(p string) (string, error)
	// SamePath compares paths for containment checks. unix is case
	// sensitive; windows is not.
	SamePath(a, b string) bool
}

// Lock is an exclusive guard on the data directory. Releasing happens on
// Release or when the process exits.
type Lock interface {
	Release() error
}

// Instance guards against two servers sharing one data directory.
type Instance interface {
	// Acquire takes an exclusive, non-blocking lock for dataDir.
	// unix: flock; windows: LockFileEx.
	Acquire(dataDir string) (Lock, error)
}

// Process manages child processes and probes foreign pids.
type Process interface {
	// Configure prepares a child command before Start.
	// unix: Setpgid; windows: CREATE_NEW_PROCESS_GROUP plus Job Object.
	Configure(cmd *exec.Cmd) error
	// Attach enrolls an already-started platform child into supervision so
	// nothing is left behind when the platform exits. Call it right after
	// cmd.Start; a failed attach must fail the launch. unix is a no-op;
	// windows assigns the child to the platform Job Object.
	Attach(proc *os.Process) error
	// Alive reports whether pid answers a signal, zombies included. Pair
	// with IsZombie to tell a live process from an unreaped one.
	Alive(pid int) bool
	// IsZombie reports whether pid is a zombie awaiting reaping.
	// windows always returns false.
	IsZombie(pid int) bool
	// Terminate ends pid and its children. A missing process is success.
	// unix: kill(-pgid, SIGKILL); windows: taskkill /T /F or Job Object.
	Terminate(pid int) error
}

// Executables locates commands and tests executability.
type Executables interface {
	// Resolve finds name on PATH and reports a lookup miss as an error,
	// like exec.LookPath. windows is PATHEXT-aware and ignores
	// extensionless sh shims.
	Resolve(name string) (string, error)
	// IsExecutable reports whether path is runnable. unix checks the 0o111
	// mode bits; windows checks the extension and file type.
	IsExecutable(path string) bool
}

// Perms applies private permissions to platform-owned files.
type Perms interface {
	// PrivateDir creates path restricted to the current user.
	// unix: 0700; windows: no ACL tightening, creates only.
	PrivateDir(path string) error
	// PrivateFile restricts an existing file to the current user.
	// unix: 0600; windows: no ACL tightening.
	PrivateFile(path string) error
	// Enforced reports whether private permissions take effect.
	// unix: true; windows: false (startup warns instead of failing).
	Enforced() bool
}

// Shutdown describes how the platform listens for termination.
type Shutdown interface {
	// Signals are the termination signals for signal.NotifyContext.
	// unix: SIGINT, SIGTERM; windows: os.Interrupt and console-close.
	Signals() []os.Signal
	// Interrupt asks a test helper process to stop. unix sends SIGTERM.
	Interrupt(p *os.Process) error
}

// Terminal abstracts interactive secret prompts.
type Terminal interface {
	IsInteractive() bool
	// ReadSecret reads a line without echoing. Non-interactive input
	// falls back to a plain stdin read.
	ReadSecret(prompt string) (string, error)
}

// Environment runs startup environment checks.
type Environment interface {
	// Describe returns diagnostic lines for the startup log, including
	// warnings that do not block startup. It takes the resolved data
	// directory so implementations can warn about hostile locations
	// (a DrvFs mount on unix, a shared directory on windows).
	Describe(dataDir string) []string
	// Check rejects unsupported environments, e.g. WSL1 on unix.
	Check() error
}

// ErrLockHeld reports that another process holds the single-instance lock.
// It is the canonical port value; each platform implementation holds its
// own copy and the New adapters below map it here, so business packages
// only depend on the port (the implementations cannot import this package
// without creating an import cycle).
var ErrLockHeld = errors.New("single-instance lock is held by another process")

// stubPlatform is the placeholder returned by New until each port has a
// real implementation. Ports are wired up task by task in phase 2 (unix)
// and phase 4 (windows).
type stubPlatform struct {
	name string
}

func (s stubPlatform) Name() string             { return s.name }
func (s stubPlatform) Dirs() Dirs               { return nil }
func (s stubPlatform) Instance() Instance       { return nil }
func (s stubPlatform) Process() Process         { return nil }
func (s stubPlatform) Executables() Executables { return nil }
func (s stubPlatform) Perms() Perms             { return nil }
func (s stubPlatform) Shutdown() Shutdown       { return nil }
func (s stubPlatform) Terminal() Terminal       { return nil }
func (s stubPlatform) Environment() Environment { return nil }
