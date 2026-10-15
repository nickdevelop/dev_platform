//go:build unix && !linux

package unix

import "devplatform/internal/core/platform/policy"

// Trash always reports that the OS trash is unavailable: non-Linux unix
// systems have no supported trash integration. Deletion then requires the
// caller's explicit permanent-delete confirmation and never pretends to have
// used a trash. The New adapter maps the policy copy to the canonical port
// value.
type Trash struct{}

// Move always reports that the OS trash is unavailable.
func (Trash) Move(string) error {
	return policy.ErrTrashUnavailable
}
