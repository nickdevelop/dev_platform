// Package fake is the deterministic Platform test double. Every port is
// a struct whose zero value does nothing and returns nil; New options
// inject directories, process tables, lookup results, errors and secrets.
package fake

import (
	"os"
	"os/exec"

	"devplatform/internal/platform"
)

// Platform is a controllable platform.Platform.
type Platform struct {
	dirs        Dirs
	instance    Instance
	process     Process
	executables Executables
	trash       Trash
	perms       Perms
	shutdown    Shutdown
	terminal    Terminal
	environment Environment
}

// Option configures a fake Platform.
type Option func(*Platform)

// New returns a fake Platform with no behavior until options inject it.
func New(opts ...Option) *Platform {
	p := &Platform{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Platform) Dirs() platform.Dirs               { return &p.dirs }
func (p *Platform) Instance() platform.Instance       { return &p.instance }
func (p *Platform) Process() platform.Process         { return &p.process }
func (p *Platform) Executables() platform.Executables { return &p.executables }
func (p *Platform) Trash() platform.Trash             { return &p.trash }
func (p *Platform) Perms() platform.Perms             { return &p.perms }
func (p *Platform) Shutdown() platform.Shutdown       { return &p.shutdown }
func (p *Platform) Terminal() platform.Terminal       { return &p.terminal }
func (p *Platform) Environment() platform.Environment {
	return &p.environment
}

func (p *Platform) Name() string { return "fake" }

// WithHomeDir injects the home directory.
func WithHomeDir(dir string) Option {
	return func(p *Platform) { p.dirs.Home = dir }
}

// WithConfigDir injects the config directory.
func WithConfigDir(dir string) Option {
	return func(p *Platform) { p.dirs.Config = dir }
}

// WithDataDir injects the data directory.
func WithDataDir(dir string) Option {
	return func(p *Platform) { p.dirs.Data = dir }
}

// WithStateDir injects the state directory.
func WithStateDir(dir string) Option {
	return func(p *Platform) { p.dirs.State = dir }
}

// WithAlive injects the process liveness table.
func WithAlive(table map[int]bool) Option {
	return func(p *Platform) { p.process.AliveTable = table }
}

// WithExecutables injects the executable lookup table.
func WithExecutables(table map[string]string) Option {
	return func(p *Platform) { p.executables.Paths = table }
}

// WithTrashErr injects the trash error.
func WithTrashErr(err error) Option {
	return func(p *Platform) { p.trash.Err = err }
}

// WithPermsEnforced injects the Perms.Enforced value.
func WithPermsEnforced(enforced bool) Option {
	return func(p *Platform) { p.perms.EnforcedValue = enforced }
}

// WithSecret injects the secret ReadSecret returns.
func WithSecret(secret string) Option {
	return func(p *Platform) { p.terminal.Secret = secret }
}

// WithInteractive injects the Terminal.IsInteractive value.
func WithInteractive(interactive bool) Option {
	return func(p *Platform) { p.terminal.Interactive = interactive }
}

// Dirs fakes directory resolution and path policy.
type Dirs struct {
	Home   string
	Config string
	Data   string
	State  string
	Temp   string
	// Canonicalize overrides CanonicalizeUserPath; nil means identity.
	Canonicalize func(p string) (string, error)
	// Same overrides SamePath; nil means ==.
	Same func(a, b string) bool
}

func (d *Dirs) HomeDir() (string, error)   { return d.Home, nil }
func (d *Dirs) ConfigDir() (string, error) { return d.Config, nil }
func (d *Dirs) DataDir() (string, error)   { return d.Data, nil }
func (d *Dirs) StateDir() (string, error)  { return d.State, nil }
func (d *Dirs) TempDir() string            { return d.Temp }

func (d *Dirs) CanonicalizeUserPath(p string) (string, error) {
	if d.Canonicalize != nil {
		return d.Canonicalize(p)
	}
	return p, nil
}

func (d *Dirs) SamePath(a, b string) bool {
	if d.Same != nil {
		return d.Same(a, b)
	}
	return a == b
}

// Lock is the fake Instance lock.
type Lock struct {
	Released bool
}

// Release marks the lock released.
func (l *Lock) Release() error {
	l.Released = true
	return nil
}

// Instance fakes the single-instance lock.
type Instance struct {
	// AcquireErr makes Acquire fail.
	AcquireErr error
	// Acquired records the data directories Acquire was called with.
	Acquired []string
}

func (in *Instance) Acquire(dataDir string) (platform.Lock, error) {
	if in.AcquireErr != nil {
		return nil, in.AcquireErr
	}
	in.Acquired = append(in.Acquired, dataDir)
	return &Lock{}, nil
}

// Process fakes child configuration, liveness and termination.
type Process struct {
	// AliveTable maps pid to liveness; missing pids are not alive.
	AliveTable map[int]bool
	// ZombieTable maps pid to zombie state; missing pids are not zombies.
	ZombieTable map[int]bool
	// ConfigureErr makes Configure fail.
	ConfigureErr error
	// TerminateErr makes Terminate fail.
	TerminateErr error
	// Configured records commands passed to Configure.
	Configured []*exec.Cmd
	// Terminated records pids passed to Terminate.
	Terminated []int
}

func (pr *Process) Configure(cmd *exec.Cmd) error {
	pr.Configured = append(pr.Configured, cmd)
	return pr.ConfigureErr
}

func (pr *Process) Alive(pid int) bool { return pr.AliveTable[pid] }

func (pr *Process) IsZombie(pid int) bool { return pr.ZombieTable[pid] }

func (pr *Process) Terminate(pid int) error {
	pr.Terminated = append(pr.Terminated, pid)
	return pr.TerminateErr
}

// Executables fakes command lookup.
type Executables struct {
	// Paths maps command names to resolved paths.
	Paths map[string]string
	// ResolveErr makes Resolve fail.
	ResolveErr error
	// IsExecutableFunc overrides IsExecutable; nil means false.
	IsExecutableFunc func(path string) bool
}

func (e *Executables) Resolve(name string) (string, error) {
	if e.ResolveErr != nil {
		return "", e.ResolveErr
	}
	return e.Paths[name], nil
}

func (e *Executables) IsExecutable(path string) bool {
	if e.IsExecutableFunc != nil {
		return e.IsExecutableFunc(path)
	}
	return false
}

// Trash fakes the OS trash.
type Trash struct {
	// Err makes Move fail.
	Err error
	// Moved records paths passed to Move.
	Moved []string
}

func (t *Trash) Move(path string) error {
	t.Moved = append(t.Moved, path)
	return t.Err
}

// Perms fakes private permissions.
type Perms struct {
	// EnforcedValue is reported by Enforced.
	EnforcedValue bool
}

func (p *Perms) PrivateDir(path string) error  { return nil }
func (p *Perms) PrivateFile(path string) error { return nil }
func (p *Perms) Enforced() bool                { return p.EnforcedValue }

// Shutdown fakes termination signals.
type Shutdown struct {
	// SignalsValue is reported by Signals.
	SignalsValue []os.Signal
	// InterruptErr makes Interrupt fail.
	InterruptErr error
	// Interrupted records processes passed to Interrupt.
	Interrupted []*os.Process
}

func (s *Shutdown) Signals() []os.Signal { return s.SignalsValue }

func (s *Shutdown) Interrupt(p *os.Process) error {
	s.Interrupted = append(s.Interrupted, p)
	return s.InterruptErr
}

// Terminal fakes interactive secret prompts.
type Terminal struct {
	// Interactive is reported by IsInteractive.
	Interactive bool
	// Secret is returned by ReadSecret.
	Secret string
	// SecretErr makes ReadSecret fail.
	SecretErr error
}

func (t *Terminal) IsInteractive() bool { return t.Interactive }

func (t *Terminal) ReadSecret(prompt string) (string, error) {
	return t.Secret, t.SecretErr
}

// Environment fakes startup checks.
type Environment struct {
	// DescribeLines are reported by Describe.
	DescribeLines []string
	// CheckErr makes Check fail.
	CheckErr error
}

func (e *Environment) Describe() []string { return e.DescribeLines }
func (e *Environment) Check() error       { return e.CheckErr }
