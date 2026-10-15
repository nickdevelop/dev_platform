package resourcesync

import (
	"crypto/sha256"
	"encoding/hex"

	"devplatform/internal/app/resources"
)

// ManagedPatch is the platform-managed content the sync coordinator wants to
// write to one target. It carries the resource identity and change summary so
// the manager can build a contract-valid preview on its own.
type ManagedPatch struct {
	// ResourceID is the resource the patch was rendered from.
	ResourceID uint
	// Kind is the resource kind.
	Kind resources.ResourceKind
	// Target is the location the patch applies to.
	Target ResourceTarget
	// Content is the full platform-managed content for the target.
	Content []byte
	// Summary is a human-facing change description.
	Summary string
	// ResourceVersion identifies the resource definition version rendered.
	ResourceVersion string
}

// hashContent returns the stable hex digest used for drift detection.
func hashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
