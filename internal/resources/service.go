package resources

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/agents"
	"devplatform/internal/database"
	"devplatform/internal/secrets"
)

// Sentinel errors reported by the resource service. Callers test for them
// with errors.Is; the handlers map them to HTTP status and error codes.
var (
	// ErrResourceNotFound reports that no resource record with the given id
	// exists.
	ErrResourceNotFound = errors.New("resources: resource not found")
	// ErrProjectNotFound reports that no project record with the given id
	// exists.
	ErrProjectNotFound = errors.New("resources: project not found")
	// ErrSessionNotFound reports that no session of the given project has the
	// given id.
	ErrSessionNotFound = errors.New("resources: session not found")
	// ErrNotPrompt reports an attempt to select a non-prompt resource as a
	// session prompt.
	ErrNotPrompt = errors.New("resources: resource is not a prompt")
	// ErrSecretStoreUnavailable reports a plaintext credential supplied when no
	// secret store is configured; the value is refused rather than dropped.
	ErrSecretStoreUnavailable = errors.New("resources: secret store is not configured")
	// ErrResourceConflict reports a resource name already used by another
	// resource of the same kind; the two would collide on one managed target.
	ErrResourceConflict = errors.New("resources: resource name already exists")
)

// CreateInput carries the client-settable fields of a resource definition.
// It is validated before the platform assigns an id and timestamps.
type CreateInput struct {
	// Kind names the resource class.
	Kind ResourceKind
	// Name is the human-facing, single-segment resource name.
	Name string
	// Description is an optional note.
	Description string
	// Content is the definition body.
	Content string
	// Enabled reports whether the resource is active.
	Enabled bool
	// PromptScope applies to prompt definitions only.
	PromptScope PromptScope
	// SecretRef is the opaque credential reference for an MCP definition; it
	// is never the credential value.
	SecretRef string
	// SecretValue is a write-only plaintext credential. When set for an MCP
	// definition it is stored in the secret store and only the reference is
	// persisted; it is never returned.
	SecretValue string
	// Backends lists the backends the resource applies to (empty is
	// unrestricted).
	Backends []agents.BackendID
}

// Validate reports whether the input can form a valid definition.
func (in CreateInput) Validate() error {
	return validateFields(in.Kind, in.Name, in.PromptScope, in.SecretRef, in.Backends)
}

// Service manages canonical resource definitions, their project associations
// and their selection as session prompts. Credentials are referenced only by
// an opaque id; the service never reads or returns a plaintext secret.
type Service struct {
	repo    *repository
	secrets secrets.SecretStore
}

// Option configures a Service.
type Option func(*Service)

// WithSecretStore configures the store used for MCP credential values. When
// unset, a caller that supplies a plaintext value is refused rather than
// having the value silently dropped.
func WithSecretStore(store secrets.SecretStore) Option {
	return func(s *Service) {
		if store != nil {
			s.secrets = store
		}
	}
}

// NewService builds the resource service over the platform database.
func NewService(db *gorm.DB, opts ...Option) *Service {
	s := &Service{repo: &repository{db: db}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// resolveSecret returns the opaque reference to persist and, when a plaintext
// value is supplied, stores it. An empty value leaves providedRef unchanged,
// so a reference can be set without a value and a value can be rotated
// without changing the reference.
func (s *Service) resolveSecret(kind ResourceKind, providedRef, value string) (string, error) {
	if kind != ResourceMCP || value == "" {
		return providedRef, nil
	}
	if s.secrets == nil {
		return "", ErrSecretStoreUnavailable
	}
	ref := providedRef
	if ref == "" {
		generated, err := secrets.NewSecretID()
		if err != nil {
			return "", err
		}
		ref = generated
	}
	if err := s.secrets.Set(context.Background(), ref, value); err != nil {
		return "", err
	}
	return ref, nil
}

// Create validates and persists a new resource definition.
func (s *Service) Create(in CreateInput) (ResourceDefinition, error) {
	if err := in.Validate(); err != nil {
		return ResourceDefinition{}, err
	}
	secretRef, err := s.resolveSecret(in.Kind, in.SecretRef, in.SecretValue)
	if err != nil {
		return ResourceDefinition{}, err
	}
	now := time.Now().UTC()
	row := database.Resource{
		Kind:        string(in.Kind),
		Name:        in.Name,
		Description: in.Description,
		Content:     in.Content,
		Enabled:     in.Enabled,
		PromptScope: string(in.PromptScope),
		SecretRef:   secretRef,
		Backends:    joinBackends(in.Backends),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.create(&row); err != nil {
		return ResourceDefinition{}, err
	}
	return definitionFromModel(row), nil
}

// Get returns the resource with the given id.
func (s *Service) Get(id uint) (ResourceDefinition, error) {
	row, err := s.repo.get(id)
	if err != nil {
		return ResourceDefinition{}, err
	}
	return definitionFromModel(row), nil
}

// List returns resources in creation order, optionally restricted to one
// kind. An empty kind lists every resource.
func (s *Service) List(kind ResourceKind) ([]ResourceDefinition, error) {
	if kind != "" && !kind.Valid() {
		return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalidResource, kind)
	}
	rows, err := s.repo.list(string(kind))
	if err != nil {
		return nil, err
	}
	return definitionsFromModels(rows), nil
}

// Update replaces the mutable fields of an existing resource. The resource
// kind is immutable: a resource cannot silently change class after projects
// or sessions reference it.
func (s *Service) Update(id uint, in CreateInput) (ResourceDefinition, error) {
	row, err := s.repo.get(id)
	if err != nil {
		return ResourceDefinition{}, err
	}
	if err := in.Validate(); err != nil {
		return ResourceDefinition{}, err
	}
	if ResourceKind(row.Kind) != in.Kind {
		return ResourceDefinition{}, fmt.Errorf("%w: kind %q cannot change to %q",
			ErrInvalidResource, row.Kind, in.Kind)
	}
	row.Name = in.Name
	row.Description = in.Description
	row.Content = in.Content
	row.Enabled = in.Enabled
	row.PromptScope = string(in.PromptScope)
	providedRef := in.SecretRef
	if providedRef == "" {
		providedRef = row.SecretRef
	}
	secretRef, err := s.resolveSecret(in.Kind, providedRef, in.SecretValue)
	if err != nil {
		return ResourceDefinition{}, err
	}
	row.SecretRef = secretRef
	row.Backends = joinBackends(in.Backends)
	row.UpdatedAt = time.Now().UTC()
	if err := s.repo.update(&row); err != nil {
		return ResourceDefinition{}, err
	}
	return definitionFromModel(row), nil
}

// Delete removes a resource, every association that references it, and the
// credential value it referenced.
func (s *Service) Delete(id uint) error {
	row, err := s.repo.get(id)
	if err != nil {
		return err
	}
	if row.SecretRef != "" && s.secrets != nil {
		if err := s.secrets.Delete(context.Background(), row.SecretRef); err != nil && !errors.Is(err, secrets.ErrSecretNotFound) {
			return err
		}
	}
	return s.repo.delete(id)
}

// AssociateProject links a resource to a project. Both must exist;
// associating an existing pair is a no-op.
func (s *Service) AssociateProject(projectID, resourceID uint) error {
	if err := s.requireProject(projectID); err != nil {
		return err
	}
	if _, err := s.repo.get(resourceID); err != nil {
		return err
	}
	return s.repo.associate(projectID, resourceID)
}

// DisassociateProject removes a resource/project link; a missing link is a
// no-op.
func (s *Service) DisassociateProject(projectID, resourceID uint) error {
	if err := s.requireProject(projectID); err != nil {
		return err
	}
	if _, err := s.repo.get(resourceID); err != nil {
		return err
	}
	return s.repo.disassociate(projectID, resourceID)
}

// ListProjectResources returns the resources associated with one project.
func (s *Service) ListProjectResources(projectID uint) ([]ResourceDefinition, error) {
	if err := s.requireProject(projectID); err != nil {
		return nil, err
	}
	rows, err := s.repo.listProjectResources(projectID)
	if err != nil {
		return nil, err
	}
	return definitionsFromModels(rows), nil
}

// SelectSessionPrompt records a prompt as selected for one session of a
// project. The resource must be a prompt; both project and session must
// exist, and the session must belong to the project.
func (s *Service) SelectSessionPrompt(projectID uint, sessionID string, resourceID uint) error {
	if err := s.requireProjectSession(projectID, sessionID); err != nil {
		return err
	}
	row, err := s.repo.get(resourceID)
	if err != nil {
		return err
	}
	if ResourceKind(row.Kind) != ResourcePrompt {
		return fmt.Errorf("%w: resource %d is a %s", ErrNotPrompt, resourceID, row.Kind)
	}
	return s.repo.selectSessionPrompt(sessionID, resourceID)
}

// RemoveSessionPrompt clears a prompt selection; a missing selection is a
// no-op.
func (s *Service) RemoveSessionPrompt(projectID uint, sessionID string, resourceID uint) error {
	if err := s.requireProjectSession(projectID, sessionID); err != nil {
		return err
	}
	return s.repo.removeSessionPrompt(sessionID, resourceID)
}

// ListSessionPrompts returns the prompts selected for one session.
func (s *Service) ListSessionPrompts(projectID uint, sessionID string) ([]ResourceDefinition, error) {
	if err := s.requireProjectSession(projectID, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.repo.listSessionPrompts(sessionID)
	if err != nil {
		return nil, err
	}
	return definitionsFromModels(rows), nil
}

func (s *Service) requireProject(projectID uint) error {
	exists, err := s.repo.projectExists(projectID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %d", ErrProjectNotFound, projectID)
	}
	return nil
}

func (s *Service) requireProjectSession(projectID uint, sessionID string) error {
	if err := s.requireProject(projectID); err != nil {
		return err
	}
	exists, err := s.repo.sessionExistsInProject(projectID, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrSessionNotFound, sessionID)
	}
	return nil
}

func definitionsFromModels(rows []database.Resource) []ResourceDefinition {
	definitions := make([]ResourceDefinition, 0, len(rows))
	for _, row := range rows {
		definitions = append(definitions, definitionFromModel(row))
	}
	return definitions
}
