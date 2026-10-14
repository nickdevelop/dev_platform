package fake_test

import (
	"errors"
	"testing"

	"devplatform/internal/platform"
	"devplatform/internal/platform/fake"
)

// TestFakeSatisfiesPlatform pins the test double to the port surface.
func TestFakeSatisfiesPlatform(t *testing.T) {
	var _ platform.Platform = fake.New()
}

// TestZeroValueIsNoOp checks the untouched fake does nothing and returns nil.
func TestZeroValueIsNoOp(t *testing.T) {
	p := fake.New()
	if got, _ := p.Dirs().HomeDir(); got != "" {
		t.Fatalf("HomeDir() = %q, want empty", got)
	}
	if got := p.Dirs().TempDir(); got != "" {
		t.Fatalf("TempDir() = %q, want empty", got)
	}
	if p.Process().Alive(1234) {
		t.Fatal("Alive() = true, want false")
	}
	if p.Process().IsZombie(1234) {
		t.Fatal("IsZombie() = true, want false")
	}
	if _, err := p.Executables().Resolve("opencode"); err == nil {
		t.Fatal("Resolve() on a missing name = nil error, want a lookup miss")
	}
	if p.Executables().IsExecutable("/bin/sh") {
		t.Fatal("IsExecutable() = true, want false")
	}
	if err := p.Trash().Move("/tmp/x"); err != nil {
		t.Fatalf("Move() error = %v, want nil", err)
	}
	if p.Perms().Enforced() {
		t.Fatal("Enforced() = true, want false")
	}
	if err := p.Environment().Check(); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if _, err := p.Terminal().ReadSecret("password: "); err != nil {
		t.Fatalf("ReadSecret() error = %v, want nil", err)
	}
	if _, err := p.Instance().Acquire(t.TempDir()); err != nil {
		t.Fatalf("Acquire() error = %v, want nil", err)
	}
}

// TestInjectedValuesReturned checks every New option is returned as-is.
func TestInjectedValuesReturned(t *testing.T) {
	trashErr := errors.New("trash unavailable")
	p := fake.New(
		fake.WithHomeDir("/home/user"),
		fake.WithConfigDir("/home/user/.config"),
		fake.WithDataDir("/home/user/.local/share"),
		fake.WithStateDir("/home/user/.local/state"),
		fake.WithAlive(map[int]bool{4242: true}),
		fake.WithExecutables(map[string]string{"opencode": "/usr/bin/opencode"}),
		fake.WithTrashErr(trashErr),
		fake.WithPermsEnforced(true),
		fake.WithInteractive(true),
		fake.WithSecret("s3cret"),
	)

	if got, _ := p.Dirs().HomeDir(); got != "/home/user" {
		t.Fatalf("HomeDir() = %q", got)
	}
	if got, _ := p.Dirs().ConfigDir(); got != "/home/user/.config" {
		t.Fatalf("ConfigDir() = %q", got)
	}
	if got, _ := p.Dirs().DataDir(); got != "/home/user/.local/share" {
		t.Fatalf("DataDir() = %q", got)
	}
	if got, _ := p.Dirs().StateDir(); got != "/home/user/.local/state" {
		t.Fatalf("StateDir() = %q", got)
	}
	if !p.Process().Alive(4242) {
		t.Fatal("Alive(4242) = false, want true")
	}
	if p.Process().Alive(9999) {
		t.Fatal("Alive(9999) = true, want false")
	}
	if got, _ := p.Executables().Resolve("opencode"); got != "/usr/bin/opencode" {
		t.Fatalf("Resolve() = %q", got)
	}
	if err := p.Trash().Move("/tmp/x"); !errors.Is(err, trashErr) {
		t.Fatalf("Move() error = %v, want trashErr", err)
	}
	if !p.Perms().Enforced() {
		t.Fatal("Enforced() = false, want true")
	}
	if !p.Terminal().IsInteractive() {
		t.Fatal("IsInteractive() = false, want true")
	}
	if got, _ := p.Terminal().ReadSecret("password: "); got != "s3cret" {
		t.Fatalf("ReadSecret() = %q", got)
	}
}
