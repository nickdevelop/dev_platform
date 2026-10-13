package pi

import "devplatform/internal/agents"

// Descriptor is the pi backend's static description: the facts the platform's
// settings, diagnostics and UI present for it.
var Descriptor = agents.BackendDescriptor{
	ID:               agents.BackendPi,
	Label:            "Pi Agent",
	Binary:           "pi",
	DefaultEnabled:   true,
	Order:            2,
	Tested:           "1.1.0",
	Minimum:          "1.0.0",
	Maximum:          "2.0.0",
	ServiceSupported: false,
}
