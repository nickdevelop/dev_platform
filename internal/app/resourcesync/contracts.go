// Package resourcesync defines the contracts for translating canonical
// resource definitions into backend-specific managed-file patches and for
// tracking the ownership, preview, conflict and recovery state of the files
// the platform maintains.
//
// The package is deliberately implementation-free: it carries the target,
// preview, conflict and backend-support vocabulary shared by the sync
// coordinator, the per-backend adapters and the HTTP handlers. Filesystem
// writes and agent-specific formats live in the implementation files.
package resourcesync

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/resources"
)

// Sentinel errors reported by the sync contract validation helpers.
var (
	// ErrInvalidTarget reports a synchronization target that names no
	// project, an unknown backend, or an unsafe relative path.
	ErrInvalidTarget = errors.New("resourcesync: invalid target")
	// ErrInvalidConflict reports a conflict record that violates the
	// contract.
	ErrInvalidConflict = errors.New("resourcesync: invalid conflict")
	// ErrInvalidPreview reports a sync preview that violates the contract.
	ErrInvalidPreview = errors.New("resourcesync: invalid preview")
)

// SyncState is the synchronization status of one resource/target pair.
type SyncState string

const (
	// SyncStatePending marks a pair whose first apply has not happened yet.
	SyncStatePending SyncState = "pending"
	// SyncStateApplied marks a pair whose managed content is in sync.
	SyncStateApplied SyncState = "applied"
	// SyncStateConflict marks a pair whose sync paused pending user
	// resolution.
	SyncStateConflict SyncState = "conflict"
	// SyncStateFailed marks a pair whose last sync attempt failed.
	SyncStateFailed SyncState = "failed"
	// SyncStateUnsupported marks a pair the backend cannot represent.
	SyncStateUnsupported SyncState = "unsupported"
)

// Valid reports whether the state is one of the defined sync states.
func (s SyncState) Valid() bool {
	switch s {
	case SyncStatePending, SyncStateApplied, SyncStateConflict, SyncStateFailed, SyncStateUnsupported:
		return true
	default:
		return false
	}
}

// ConflictKind classifies why a synchronization paused.
type ConflictKind string

const (
	// ConflictUnmanagedTarget marks a target that already exists but is not
	// owned by the platform.
	ConflictUnmanagedTarget ConflictKind = "unmanaged-target"
	// ConflictExternalDrift marks a platform-managed target that was edited
	// outside the platform since the last apply.
	ConflictExternalDrift ConflictKind = "external-drift"
	// ConflictTargetInvalid marks a target that cannot be written safely.
	ConflictTargetInvalid ConflictKind = "target-invalid"
)

// Valid reports whether the kind is one of the defined conflict kinds.
func (c ConflictKind) Valid() bool {
	switch c {
	case ConflictUnmanagedTarget, ConflictExternalDrift, ConflictTargetInvalid:
		return true
	default:
		return false
	}
}

// ResourceTarget names where a resource is synchronized. An empty BackendID
// means the target is project-level (for example a project instruction file)
// rather than a specific agent's configuration.
type ResourceTarget struct {
	// ProjectID is the project the target belongs to.
	ProjectID uint `json:"projectId"`
	// BackendID is the agent backend whose configuration is targeted, or
	// empty for a project-level target.
	BackendID agents.BackendID `json:"backendId,omitempty"`
	// Path is the target path relative to the project directory. It is a
	// logical, slash-separated path that must stay inside the project.
	Path string `json:"path"`
}

// Validate reports whether the target names a project and an in-project
// relative path, with a known backend when one is set.
func (t ResourceTarget) Validate() error {
	if t.ProjectID == 0 {
		return fmt.Errorf("%w: project id is zero", ErrInvalidTarget)
	}
	if t.BackendID != "" && !t.BackendID.Valid() {
		return fmt.Errorf("%w: unknown backend %q", ErrInvalidTarget, t.BackendID)
	}
	return validateRelativePath(t.Path)
}

// validateRelativePath rejects empty, absolute, separator-ambiguous, unclean
// and traversal paths so a target can never escape its project directory.
func validateRelativePath(p string) error {
	if p == "" {
		return fmt.Errorf("%w: path is empty", ErrInvalidTarget)
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("%w: path contains a NUL byte", ErrInvalidTarget)
	}
	if strings.ContainsRune(p, '\\') {
		return fmt.Errorf("%w: path %q contains a backslash", ErrInvalidTarget, p)
	}
	if strings.HasPrefix(p, "/") || path.IsAbs(p) {
		return fmt.Errorf("%w: path %q is absolute", ErrInvalidTarget, p)
	}
	if path.Clean(p) != p {
		return fmt.Errorf("%w: path %q is not clean", ErrInvalidTarget, p)
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("%w: path %q contains an unsafe segment", ErrInvalidTarget, p)
		}
	}
	return nil
}

// SyncConflict records why a resource/target pair paused.
type SyncConflict struct {
	// Kind classifies the conflict.
	Kind ConflictKind `json:"kind"`
	// ResourceID is the resource whose sync paused.
	ResourceID uint `json:"resourceId"`
	// Target is the location that conflicted.
	Target ResourceTarget `json:"target"`
	// Detail is a human-facing explanation that must not contain secrets.
	Detail string `json:"detail,omitempty"`
}

// Validate reports whether the conflict record is well-formed.
func (c SyncConflict) Validate() error {
	if c.ResourceID == 0 {
		return fmt.Errorf("%w: resource id is zero", ErrInvalidConflict)
	}
	if !c.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidConflict, c.Kind)
	}
	if err := c.Target.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConflict, err)
	}
	return nil
}

// SyncPreview is the dry-run result for one resource/target pair: the state
// the apply would leave behind and a summary of the change the user must
// confirm before the first apply or a target change.
type SyncPreview struct {
	// ResourceID is the resource being previewed.
	ResourceID uint `json:"resourceId"`
	// Kind is the resource kind.
	Kind resources.ResourceKind `json:"kind"`
	// Target is the location the preview applies to.
	Target ResourceTarget `json:"target"`
	// State is the state an apply would produce, or the current blocking
	// state (conflict, unsupported).
	State SyncState `json:"state"`
	// ChangeSummary describes the change for the user; it may be empty only
	// when the state is unsupported.
	ChangeSummary string `json:"changeSummary"`
	// Conflict is set exactly when State is conflict.
	Conflict *SyncConflict `json:"conflict,omitempty"`
}

// Validate reports whether the preview is internally consistent.
func (p SyncPreview) Validate() error {
	if p.ResourceID == 0 {
		return fmt.Errorf("%w: resource id is zero", ErrInvalidPreview)
	}
	if !p.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidPreview, p.Kind)
	}
	if err := p.Target.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPreview, err)
	}
	if !p.State.Valid() {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidPreview, p.State)
	}
	if p.State != SyncStateUnsupported && strings.TrimSpace(p.ChangeSummary) == "" {
		return fmt.Errorf("%w: change summary is empty", ErrInvalidPreview)
	}
	if p.State == SyncStateConflict {
		if p.Conflict == nil {
			return fmt.Errorf("%w: conflict state without conflict detail", ErrInvalidPreview)
		}
		if err := p.Conflict.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidPreview, err)
		}
	} else if p.Conflict != nil {
		return fmt.Errorf("%w: conflict detail without conflict state", ErrInvalidPreview)
	}
	return nil
}

// BackendSupport declares which resource kinds one agent backend can
// represent. A kind the backend does not declare must be reported
// unavailable rather than silently ignored.
type BackendSupport struct {
	// BackendID is the backend the declaration describes.
	BackendID agents.BackendID `json:"backendId"`
	// Kinds are the resource kinds the backend supports.
	Kinds []resources.ResourceKind `json:"kinds"`
}

// Supports reports whether the backend declared support for kind.
func (b BackendSupport) Supports(kind resources.ResourceKind) bool {
	for _, supported := range b.Kinds {
		if supported == kind {
			return true
		}
	}
	return false
}
