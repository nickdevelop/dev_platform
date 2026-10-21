//go:build !linux

package deepseek

// reapOrphanDshProcesses is a no-op on non-Linux platforms where /proc scanning is not available.
func reapOrphanDshProcesses() {
}
