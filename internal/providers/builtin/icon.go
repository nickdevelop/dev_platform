package builtin

import _ "embed"

// iconSVG is the backend's logo, served by the platform's icon route.
//
//go:embed icon.svg
var iconSVG []byte
