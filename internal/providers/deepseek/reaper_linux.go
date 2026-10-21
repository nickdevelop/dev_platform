//go:build linux

package deepseek

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"devplatform/internal/core/platform"
)

// reapOrphanDshProcesses scans /proc for lingering dsh processes whose parent
// has died (PPID == 1) and terminates them to prevent process leakage across platform restarts.
func reapOrphanDshProcesses() {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	selfPid := os.Getpid()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == selfPid {
			continue
		}

		// Check PPID in /proc/<pid>/stat
		statBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			continue
		}
		ppid := parsePPID(statBytes)
		if ppid != 1 {
			continue
		}

		// Check cmdline in /proc/<pid>/cmdline
		cmdlineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		if isDshWebProcess(cmdlineBytes) {
			_ = platform.New().Process().Terminate(pid)
		}
	}
}

func parsePPID(stat []byte) int {
	// Format: pid (comm) state ppid ...
	// The comm field may contain spaces and closing parentheses, so find the last ')'
	rParen := bytes.LastIndexByte(stat, ')')
	if rParen < 0 || rParen+2 >= len(stat) {
		return 0
	}
	fields := strings.Fields(string(stat[rParen+2:]))
	if len(fields) < 2 {
		return 0
	}
	// fields[0] is state, fields[1] is ppid
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return ppid
}

func isDshWebProcess(cmdline []byte) bool {
	// cmdline entries are separated by null bytes
	args := bytes.Split(cmdline, []byte{0})
	hasDsh := false
	hasProfileWeb := false

	for i, arg := range args {
		s := string(arg)
		if strings.Contains(s, "dsh") {
			hasDsh = true
		}
		if s == "--profile" && i+1 < len(args) && string(args[i+1]) == "web" {
			hasProfileWeb = true
		}
		if strings.Contains(s, "--profile=web") {
			hasProfileWeb = true
		}
	}

	return hasDsh && hasProfileWeb
}
