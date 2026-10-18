package prompts

import (
	"strings"
	"time"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(req CreatePromptRequest) (Prompt, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Prompt{}, ErrInvalidPromptName
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Prompt{}, ErrInvalidPromptContent
	}
	now := time.Now().UTC()
	return s.repo.Create(Prompt{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Content:     req.Content,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func (s *Service) Get(id uint) (Prompt, error) {
	return s.repo.Get(id)
}

func (s *Service) List(keyword string) ([]Prompt, error) {
	return s.repo.List(keyword)
}

func (s *Service) Update(id uint, req UpdatePromptRequest) (Prompt, error) {
	existing, err := s.repo.Get(id)
	if err != nil {
		return Prompt{}, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return Prompt{}, ErrInvalidPromptName
		}
		existing.Name = name
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Content != nil {
		content := strings.TrimSpace(*req.Content)
		if content == "" {
			return Prompt{}, ErrInvalidPromptContent
		}
		existing.Content = *req.Content
	}
	existing.UpdatedAt = time.Now().UTC()
	return s.repo.Update(existing)
}

func (s *Service) Delete(id uint) error {
	return s.repo.Delete(id)
}
