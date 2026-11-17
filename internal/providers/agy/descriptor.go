package agy

import "devplatform/internal/core/agents"

// Descriptor is the agy backend's static description: the facts the
// platform's settings, diagnostics and UI present for it.
var Descriptor = agents.BackendDescriptor{
	ID:               agents.BackendAgy,
	Label:            "Antigravity",
	Icon:             iconSVG,
	ShortName:        "AGY",
	Binary:           "agy",
	DefaultEnabled:   true,
	Order:            4,
	Tested:           "1.5.6",
	Minimum:          "1.0.0",
	Maximum:          "2.0.0",
	ServiceSupported: false,
	Color:            "#4285F4",
	Tone:             "info",
	PermissionModes:  []string{PermissionModeConfirm, PermissionModeAuto},
	SupportsUsage:    true,
}

// Permission modes for agy.
const (
	PermissionModeConfirm = "confirm"
	PermissionModeAuto    = "auto"
)
