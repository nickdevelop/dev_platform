//go:build windows

package windows

import "devplatform/internal/core/platform/policy"

// Trash always reports that the OS trash is unavailable on Windows.
// Deletion then requires the caller's explicit permanent-delete
// confirmation and never pretends to have used a trash.
type Trash struct{}

// Move always reports that the OS trash is unavailable. The New adapter maps
// the policy copy to the canonical port value.
func (Trash) Move(string) error {
	return policy.ErrTrashUnavailable
}
