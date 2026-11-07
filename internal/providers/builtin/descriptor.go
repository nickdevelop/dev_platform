package builtin

import (
	"errors"

	"devplatform/internal/core/agents"
)

// Descriptor describes the embedded agent backend to the platform catalog.
// It is the backend-neutral presentation the settings, diagnostics and UI
// read; there is no external binary to resolve.
var Descriptor = agents.BackendDescriptor{
	ID:              agents.BackendBuiltin,
	Label:           "平台内置 Agent",
	ShortName:       "BI",
	Embedded:        true,
	DefaultEnabled:  true,
	Order:           900,
	Tested:          "",
	OwnsSessionData: true,
	PermissionModes: []string{PermissionModeConfirm, PermissionModeAuto},
	Tone:            "primary",
	Color:           "#4a8ad6",
}

// Permission modes. The platform setting binds one at configure time; the
// adapter re-reads it before every turn through the injected permission-mode
// provider.
const (
	// PermissionModeConfirm asks the user before high-risk tools run. It is
	// the default and matches every other backend's on-platform approval.
	PermissionModeConfirm = "confirm"
	// PermissionModeAuto approves every tool call without a card.
	PermissionModeAuto = "auto"
)

// ErrNoModels reports that the model resolver delivered no usable models. The
// adapter refuses to run a turn without one; the message tells the operator
// what to fix.
var ErrNoModels = errors.New("builtin: no model configured; add an OpenAI-compatible model service in the platform settings")
