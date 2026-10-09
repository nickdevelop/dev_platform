//go:build !linux

package projects

// systemTrash is the OS trash implementation on platforms other than Linux.
// The platform targets Linux and WSL; no other platform has a supported trash
// integration here, so this mover honestly reports that the trash is
// unavailable. Deletion then requires the caller's explicit permanent-delete
// confirmation and never pretends to have used a trash.
type systemTrash struct{}

// Move always reports that the OS trash is unavailable.
func (systemTrash) Move(string) error {
	return ErrTrashUnavailable
}
