package opencode

import "devplatform/internal/core/agents"

// Descriptor is the opencode backend's static description: the facts the
// platform's settings, diagnostics and UI present for it.
var Descriptor = agents.BackendDescriptor{
	ID:               agents.BackendOpenCodeV2,
	Label:            "OpenCode",
	Binary:           "opencode",
	DefaultEnabled:   true,
	Order:            1,
	Tested:           "2.0.24",
	Minimum:          "2.0.0",
	Maximum:          "3.0.0",
	ServiceSupported: true,
	Color:            "#10B981",
}
