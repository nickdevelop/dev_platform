// Package modelconfig owns the platform's model service and model definitions.
// A service is an OpenAI-compatible endpoint; its API key is never stored here
// but in the platform-private secret store, addressed by an opaque SecretRef.
// The package exposes JSON-stable DTOs so GORM models never reach the API.
package modelconfig

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/app/secrets"
)

// Service errors, mapped to HTTP statuses by the handlers.
var (
	ErrServiceNotFound  = errors.New("modelconfig: service not found")
	ErrModelNotFound    = errors.New("modelconfig: model not found")
	ErrInvalidService   = errors.New("modelconfig: invalid service")
	ErrInvalidModel     = errors.New("modelconfig: invalid model")
	ErrModelConflict    = errors.New("modelconfig: model already exists for this service")
	ErrModelUnavailable = errors.New("modelconfig: model unavailable")
)

// ResolvedModel is the server-side view a model consumer needs to make a
// completion request. APIKey is never serialized or exposed by the API: it is
// read from the platform secret store at call time and only handed to the
// in-process client.
type ResolvedModel struct {
	ID            uint
	ServiceID     uint
	ModelID       string
	Name          string
	BaseURL       string
	APIKey        string
	SupportsImage bool
	ContextTokens int
	OutputTokens  int
}

// SecretStore is the subset of the platform secret store modelconfig needs.
// *secrets.Store implements it; tests inject an in-memory fake.
type SecretStore interface {
	Set(ctx context.Context, id, value string) error
	Exists(ctx context.Context, id string) (bool, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (string, error)
}

// ServiceDTO is the JSON-stable API representation of a model service.
type ServiceDTO struct {
	ID        uint       `json:"id"`
	Name      string     `json:"name"`
	BaseURL   string     `json:"baseUrl"`
	HasKey    bool       `json:"hasKey"`
	SecretRef string     `json:"secretRef"`
	Enabled   bool       `json:"enabled"`
	Models    []ModelDTO `json:"models"`
}

// ModelDTO is the JSON-stable API representation of one model. Usable is the
// effective availability: the model and its service are both enabled.
type ModelDTO struct {
	ID            uint   `json:"id"`
	ServiceID     uint   `json:"serviceId"`
	ModelID       string `json:"modelId"`
	Name          string `json:"name"`
	SupportsText  bool   `json:"supportsText"`
	SupportsImage bool   `json:"supportsImage"`
	SupportsTools bool   `json:"supportsTools"`
	ContextTokens int    `json:"contextTokens"`
	OutputTokens  int    `json:"outputTokens"`
	Enabled       bool   `json:"enabled"`
	Usable        bool   `json:"usable"`
}

// ServiceInput is the caller-supplied service fields. On create APIKey is
// optional; on update an empty APIKey keeps the stored key and a non-empty one
// replaces it.
type ServiceInput struct {
	Name    string
	BaseURL string
	APIKey  string
	Enabled bool
}

// ModelInput is the caller-supplied model fields. ModelID is the wire id.
type ModelInput struct {
	ModelID       string
	Name          string
	SupportsText  bool
	SupportsImage bool
	SupportsTools bool
	ContextTokens int
	OutputTokens  int
	Enabled       bool
}

// Service manages model services and their models over the platform database
// and the platform secret store.
type Service struct {
	repo  Repository
	store SecretStore
}

// NewService builds the model config service.
func NewService(db *gorm.DB, store SecretStore) *Service {
	return NewServiceWithRepository(NewGORMRepository(db), store)
}

// NewServiceWithRepository builds the model config service over a custom repository.
func NewServiceWithRepository(repo Repository, store SecretStore) *Service {
	return &Service{repo: repo, store: store}
}

// List returns every service with its models, in creation order.
func (s *Service) List() ([]ServiceDTO, error) {
	services, err := s.repo.ListServices()
	if err != nil {
		return nil, err
	}
	models, err := s.repo.ListModels()
	if err != nil {
		return nil, err
	}

	byService := make(map[uint][]database.ModelDef, len(services))
	for _, model := range models {
		byService[model.ServiceID] = append(byService[model.ServiceID], model)
	}

	out := make([]ServiceDTO, 0, len(services))
	for _, service := range services {
		out = append(out, serviceDTO(service, byService[service.ID]))
	}
	return out, nil
}

// Create validates and inserts a new service, storing the API key when given.
func (s *Service) Create(input ServiceInput) (ServiceDTO, error) {
	name, baseURL, err := validateServiceInput(input)
	if err != nil {
		return ServiceDTO{}, err
	}

	row := database.ModelService{Name: name, BaseURL: baseURL, Enabled: input.Enabled}
	if input.APIKey != "" {
		ref, err := s.storeKey(input.APIKey)
		if err != nil {
			return ServiceDTO{}, err
		}
		row.SecretRef = ref
	}

	if err := s.repo.CreateService(&row); err != nil {
		return ServiceDTO{}, err
	}
	return serviceDTO(row, nil), nil
}

// Update replaces a service's fields and applies the API key rule: a non-empty
// key replaces the stored one, an empty key keeps it.
func (s *Service) Update(id uint, input ServiceInput) (ServiceDTO, error) {
	name, baseURL, err := validateServiceInput(input)
	if err != nil {
		return ServiceDTO{}, err
	}
	row, err := s.loadService(id)
	if err != nil {
		return ServiceDTO{}, err
	}

	// An unset key keeps the stored one (the edit form sends an empty key
	// whenever the user left the field blank), so the reference only changes
	// when a replacement key is supplied.
	oldRef := row.SecretRef
	newRef := oldRef
	if input.APIKey != "" {
		ref, err := s.storeKey(input.APIKey)
		if err != nil {
			return ServiceDTO{}, err
		}
		newRef = ref
	}

	row.Name, row.BaseURL, row.SecretRef, row.Enabled = name, baseURL, newRef, input.Enabled
	if err := s.repo.UpdateService(&row); err != nil {
		return ServiceDTO{}, err
	}
	// Only once the row no longer points at the old key is it safe to delete.
	if oldRef != "" && oldRef != newRef {
		_ = s.store.Delete(context.Background(), oldRef)
	}

	models, err := s.modelsFor(id)
	if err != nil {
		return ServiceDTO{}, err
	}
	return serviceDTO(row, models), nil
}

// Delete removes a service, all of its models, and its stored API key. The
// models are removed in one transaction with the service so a partial delete
// cannot leave orphaned models.
func (s *Service) Delete(id uint) error {
	row, err := s.loadService(id)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteService(id); err != nil {
		return err
	}
	if row.SecretRef != "" {
		_ = s.store.Delete(context.Background(), row.SecretRef)
	}
	return nil
}

// AddModel validates and inserts a model under an existing service.
func (s *Service) AddModel(serviceID uint, input ModelInput) (ModelDTO, error) {
	service, err := s.loadService(serviceID)
	if err != nil {
		return ModelDTO{}, err
	}
	if err := validateModelInput(input); err != nil {
		return ModelDTO{}, err
	}

	row := database.ModelDef{
		ServiceID:     serviceID,
		ModelID:       strings.TrimSpace(input.ModelID),
		Name:          strings.TrimSpace(input.Name),
		SupportsText:  input.SupportsText,
		SupportsImage: input.SupportsImage,
		SupportsTools: input.SupportsTools,
		ContextTokens: input.ContextTokens,
		OutputTokens:  input.OutputTokens,
		Enabled:       input.Enabled,
	}
	if err := s.repo.CreateModel(&row); err != nil {
		return ModelDTO{}, err
	}
	return modelDTO(service, row), nil
}

// UpdateModel replaces a model's fields, including its wire id.
func (s *Service) UpdateModel(id uint, input ModelInput) (ModelDTO, error) {
	if err := validateModelInput(input); err != nil {
		return ModelDTO{}, err
	}
	row, err := s.loadModel(id)
	if err != nil {
		return ModelDTO{}, err
	}
	service, err := s.loadService(row.ServiceID)
	if err != nil {
		return ModelDTO{}, err
	}

	row.ModelID = strings.TrimSpace(input.ModelID)
	row.Name = strings.TrimSpace(input.Name)
	row.SupportsText = input.SupportsText
	row.SupportsImage = input.SupportsImage
	row.SupportsTools = input.SupportsTools
	row.ContextTokens = input.ContextTokens
	row.OutputTokens = input.OutputTokens
	row.Enabled = input.Enabled
	if err := s.repo.UpdateModel(&row); err != nil {
		return ModelDTO{}, err
	}
	return modelDTO(service, row), nil
}

// DeleteModel removes one model.
func (s *Service) DeleteModel(id uint) error {
	return s.repo.DeleteModel(id)
}

// ResolveKey returns the plaintext API key for a service, or "" when the
// service needs no auth. It is used by the probe endpoints and never exposed.
func (s *Service) ResolveKey(serviceID uint) (string, error) {
	row, err := s.loadService(serviceID)
	if err != nil {
		return "", err
	}
	if row.SecretRef == "" {
		return "", nil
	}
	value, err := s.store.Get(context.Background(), row.SecretRef)
	if err != nil {
		return "", fmt.Errorf("modelconfig: resolve key: %w", err)
	}
	return value, nil
}

// ResolveModel returns everything the server needs to call an enabled model:
// its wire id, display name, service base URL and key. Disabled services,
// disabled models and models without text input capability are rejected with
// ErrModelUnavailable. The key is returned only for in-process use and must
// never be included in an API DTO or a log line.
func (s *Service) ResolveModel(id uint) (ResolvedModel, error) {
	model, err := s.loadModel(id)
	if err != nil {
		return ResolvedModel{}, err
	}
	service, err := s.loadService(model.ServiceID)
	if err != nil {
		return ResolvedModel{}, err
	}
	if !service.Enabled || !model.Enabled || !model.SupportsText {
		return ResolvedModel{}, fmt.Errorf("%w: model %d", ErrModelUnavailable, id)
	}
	if _, _, err := validateServiceInput(ServiceInput{Name: service.Name, BaseURL: service.BaseURL}); err != nil {
		return ResolvedModel{}, fmt.Errorf("%w: service %d: %v", ErrModelUnavailable, service.ID, err)
	}

	key := ""
	if service.SecretRef != "" {
		if s.store == nil {
			return ResolvedModel{}, fmt.Errorf("modelconfig: resolve key: secret store is not configured")
		}
		key, err = s.store.Get(context.Background(), service.SecretRef)
		if err != nil {
			return ResolvedModel{}, fmt.Errorf("modelconfig: resolve key: %w", err)
		}
	}

	return ResolvedModel{
		ID:            model.ID,
		ServiceID:     service.ID,
		ModelID:       model.ModelID,
		Name:          model.Name,
		BaseURL:       service.BaseURL,
		APIKey:        key,
		SupportsImage: model.SupportsImage,
		ContextTokens: model.ContextTokens,
		OutputTokens:  model.OutputTokens,
	}, nil
}

// storeKey generates a reference and stores the value.
func (s *Service) storeKey(value string) (string, error) {
	ref, err := secrets.NewSecretID()
	if err != nil {
		return "", fmt.Errorf("modelconfig: generate secret id: %w", err)
	}
	if err := s.store.Set(context.Background(), ref, value); err != nil {
		return "", fmt.Errorf("modelconfig: store key: %w", err)
	}
	return ref, nil
}

func (s *Service) loadService(id uint) (database.ModelService, error) {
	return s.repo.GetService(id)
}

func (s *Service) loadModel(id uint) (database.ModelDef, error) {
	return s.repo.GetModel(id)
}

func (s *Service) modelsFor(serviceID uint) ([]database.ModelDef, error) {
	return s.repo.ListModelsByService(serviceID)
}

// validateServiceInput trims and validates the name and base URL.
func validateServiceInput(input ServiceInput) (string, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", fmt.Errorf("%w: name is empty", ErrInvalidService)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", fmt.Errorf("%w: base url %q", ErrInvalidService, input.BaseURL)
	}
	return name, baseURL, nil
}

// validateModelInput validates the model id, name and token bounds.
func validateModelInput(input ModelInput) error {
	if strings.TrimSpace(input.ModelID) == "" {
		return fmt.Errorf("%w: model id is empty", ErrInvalidModel)
	}
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidModel)
	}
	if input.ContextTokens < 0 || input.OutputTokens < 0 {
		return fmt.Errorf("%w: token counts must not be negative", ErrInvalidModel)
	}
	return nil
}

func serviceDTO(row database.ModelService, models []database.ModelDef) ServiceDTO {
	out := ServiceDTO{
		ID:        row.ID,
		Name:      row.Name,
		BaseURL:   row.BaseURL,
		HasKey:    row.SecretRef != "",
		SecretRef: row.SecretRef,
		Enabled:   row.Enabled,
		Models:    make([]ModelDTO, 0, len(models)),
	}
	for _, model := range models {
		out.Models = append(out.Models, modelDTO(row, model))
	}
	return out
}

func modelDTO(service database.ModelService, model database.ModelDef) ModelDTO {
	return ModelDTO{
		ID:            model.ID,
		ServiceID:     model.ServiceID,
		ModelID:       model.ModelID,
		Name:          model.Name,
		SupportsText:  model.SupportsText,
		SupportsImage: model.SupportsImage,
		SupportsTools: model.SupportsTools,
		ContextTokens: model.ContextTokens,
		OutputTokens:  model.OutputTokens,
		Enabled:       model.Enabled,
		Usable:        service.Enabled && model.Enabled,
	}
}
