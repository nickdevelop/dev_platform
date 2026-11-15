package notes

import (
	"errors"
	"time"
)

var (
	ErrNoteNotFound     = errors.New("note not found")
	ErrInvalidNoteTitle = errors.New("note title is required")
)

// Note represents one user note.
type Note struct {
	ID        uint      `json:"id"`
	ProjectID uint      `json:"projectId"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateNoteRequest is the payload for creating a note.
type CreateNoteRequest struct {
	ProjectID uint   `json:"projectId"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}

// UpdateNoteRequest is the payload for updating a note.
type UpdateNoteRequest struct {
	ProjectID *uint   `json:"projectId"`
	Title     *string `json:"title"`
	Content   *string `json:"content"`
}
