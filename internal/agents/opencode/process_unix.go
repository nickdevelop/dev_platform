//go:build unix

package opencode

import (
	"fmt"
	"os"
	"strings"
)

// processIsZombie reports whether the pid names a process that has exited but
// was never reaped by its parent. Such a pid answers signal(0) — so the
// registration looks alive — while its listener is closed and nothing will ever
// answer on it again. A stopped service whose parent is a shell that moved on is
// exactly this, and reporting it as "running but unreachable" would tell the
// operator to debug a service that is simply gone.
//
// Task 2.3 moves this behind the Process.IsZombie port and deletes this file.
func processIsZombie(pid int) bool {
	if pid <= 0 {
		return false
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	// The comm field is parenthesised and may itself contain spaces or ')';
	// the state field follows the last ')'.
	stat := string(raw)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 || end+2 >= len(stat) {
		return false
	}
	return stat[end+2] == 'Z'
}
