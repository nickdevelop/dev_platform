package notes

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

func (s *Service) Create(req CreateNoteRequest) (Note, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "无标题笔记"
	}
	now := time.Now().UTC()
	note := Note{
		ProjectID: req.ProjectID,
		Title:     title,
		Content:   req.Content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return s.repo.Create(note)
}

func (s *Service) Get(id uint) (Note, error) {
	return s.repo.Get(id)
}

func (s *Service) List(keyword string, projectID *uint) ([]Note, error) {
	return s.repo.List(keyword, projectID)
}

func (s *Service) Update(id uint, req UpdateNoteRequest) (Note, error) {
	existing, err := s.repo.Get(id)
	if err != nil {
		return Note{}, err
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return Note{}, ErrInvalidNoteTitle
		}
		existing.Title = title
	}
	if req.Content != nil {
		existing.Content = *req.Content
	}
	if req.ProjectID != nil {
		existing.ProjectID = *req.ProjectID
	}

	return s.repo.Update(existing)
}

func (s *Service) Delete(id uint) error {
	return s.repo.Delete(id)
}
