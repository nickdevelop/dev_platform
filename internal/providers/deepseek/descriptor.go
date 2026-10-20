package deepseek

import "devplatform/internal/core/agents"

// Descriptor is the deepseek backend's static description: the facts the
// platform's settings, diagnostics and UI present for it.
var Descriptor = agents.BackendDescriptor{
	ID:               agents.BackendDeepSeek,
	Label:            "DeepSeek",
	Binary:           "dsh",
	DefaultEnabled:   true,
	Order:            3,
	Tested:           "0.2.1-rc.2",
	Minimum:          "0.2.0",
	Maximum:          "1.0.0",
	ServiceSupported: false,
	Tone:             "info",
}
