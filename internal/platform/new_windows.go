//go:build windows

package platform

import "runtime"

// New returns the windows Platform placeholder. Phase 4 wires each real
// implementation here. It exists now only so the package compiles on
// windows and the port surface stays identical across platforms.
func New() Platform {
	return stubPlatform{name: runtime.GOOS}
}
