package roles

import (
	"strings"
	"time"

	"devplatform/internal/app/prompts"
	"devplatform/internal/core/agents"
)

// PromptReader decouples prompt retrieval from the concrete prompt service implementation.
type PromptReader interface {
	Get(id uint) (prompts.Prompt, error)
}

type Service struct {
	repo         *Repository
	promptReader PromptReader
}

func NewService(repo *Repository, promptReader PromptReader) *Service {
	return &Service{
		repo:         repo,
		promptReader: promptReader,
	}
}

func (s *Service) Create(req CreateRoleRequest) (Role, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Role{}, ErrInvalidRoleName
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return Role{}, ErrInvalidRoleModel
	}

	backend := req.Backend
	if strings.TrimSpace(string(backend)) == "" {
		backend = agents.BackendID("llm")
	}

	systemPrompt := req.SystemPrompt
	// If a prompt ID is referenced, fetch the content to ensure it's valid and synced
	if req.PromptID > 0 && s.promptReader != nil {
		if p, err := s.promptReader.Get(req.PromptID); err == nil {
			systemPrompt = p.Content
		}
	}

	now := time.Now().UTC()
	return s.repo.Create(Role{
		Name:         name,
		Description:  strings.TrimSpace(req.Description),
		Backend:      backend,
		Model:        model,
		PromptID:     req.PromptID,
		SystemPrompt: systemPrompt,
		Icon:         strings.TrimSpace(req.Icon),
		Color:        strings.TrimSpace(req.Color),
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func (s *Service) Get(id uint) (Role, error) {
	role, err := s.repo.Get(id)
	if err != nil {
		return Role{}, err
	}
	// Synchronize prompt content dynamically if prompt exists
	if role.PromptID > 0 && s.promptReader != nil {
		if p, err := s.promptReader.Get(role.PromptID); err == nil && strings.TrimSpace(p.Content) != "" {
			role.SystemPrompt = p.Content
		}
	}
	return role, nil
}

func (s *Service) List(keyword string) ([]Role, error) {
	roles, err := s.repo.List(keyword)
	if err != nil {
		return nil, err
	}
	// Enrich with referenced prompt contents
	if s.promptReader != nil {
		for i := range roles {
			if roles[i].PromptID > 0 {
				if p, err := s.promptReader.Get(roles[i].PromptID); err == nil && strings.TrimSpace(p.Content) != "" {
					roles[i].SystemPrompt = p.Content
				}
			}
		}
	}
	return roles, nil
}

func (s *Service) Update(id uint, req UpdateRoleRequest) (Role, error) {
	existing, err := s.repo.Get(id)
	if err != nil {
		return Role{}, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return Role{}, ErrInvalidRoleName
		}
		existing.Name = name
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Backend != nil {
		existing.Backend = *req.Backend
	}
	if req.Model != nil {
		model := strings.TrimSpace(*req.Model)
		if model == "" {
			return Role{}, ErrInvalidRoleModel
		}
		existing.Model = model
	}
	if req.PromptID != nil {
		existing.PromptID = *req.PromptID
		if *req.PromptID > 0 && s.promptReader != nil {
			if p, err := s.promptReader.Get(*req.PromptID); err == nil {
				existing.SystemPrompt = p.Content
			}
		}
	}
	if req.SystemPrompt != nil {
		// Only override if prompt_id is 0 or explicitly provided
		if req.PromptID == nil || *req.PromptID == 0 {
			existing.SystemPrompt = *req.SystemPrompt
		}
	}
	if req.Icon != nil {
		existing.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Color != nil {
		existing.Color = strings.TrimSpace(*req.Color)
	}

	existing.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(existing); err != nil {
		return Role{}, err
	}
	return existing, nil
}

func (s *Service) Delete(id uint) error {
	return s.repo.Delete(id)
}
