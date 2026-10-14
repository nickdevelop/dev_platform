//go:build !unix

package opencode

// processIsZombie cannot tell on non-unix platforms: they have no zombie pid
// that keeps a listener closed, so a live pid is treated as live.
//
// Transitional: Task 2.3 moves the zombie check behind the Process.IsZombie
// port (used via discovery.go) and deletes this file with process_unix.go.
func processIsZombie(pid int) bool { return false }
