//go:build unix && !linux

package unix

import "os/exec"

func setPdeathsig(cmd *exec.Cmd) {
	// Pdeathsig is a Linux-specific kernel feature.
}
