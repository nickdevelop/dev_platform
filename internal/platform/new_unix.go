//go:build unix

package platform

import "runtime"

// New returns the unix Platform (shared by Linux and WSL2). Ports are
// still placeholders; phase 2 wires each real implementation here.
func New() Platform {
	return stubPlatform{name: runtime.GOOS}
}
