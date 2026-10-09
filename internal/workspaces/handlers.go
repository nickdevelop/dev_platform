package workspaces

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"devplatform/internal/database"
)

var (
	// ErrRootExists reports that the workspace root path is already
	// registered.
	ErrRootExists = errors.New("workspaces: workspace root already registered")
	// ErrRootNotFound reports that no workspace root with the given identity
	// exists.
	ErrRootNotFound = errors.New("workspaces: workspace root not found")
	// ErrInvalidRoot reports a path that cannot be registered as a workspace
	// root because it does not exist or is not a directory.
	ErrInvalidRoot = errors.New("workspaces: invalid workspace root")
)

// Root is the JSON-stable API representation of an authorized workspace root.
// GORM models are never serialized directly.
type Root struct {
	ID        uint      `json:"id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service manages the operator-authorized workspace roots backed by the
// platform database.
type Service struct {
	db *gorm.DB
}

// NewService builds the workspace service over the platform database.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Roots lists the registered workspace roots in creation order.
func (s *Service) Roots() ([]Root, error) {
	var rows []database.WorkspaceRoot
	if err := s.db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("workspaces: list workspace roots: %w", err)
	}
	roots := make([]Root, 0, len(rows))
	for _, row := range rows {
		roots = append(roots, rootFromModel(row))
	}
	return roots, nil
}

// AddRoot validates and registers a new workspace root. The path must exist
// and be a directory; the canonical (symlink-resolved) path is stored so the
// database only ever holds real locations. Project operations can only use
// roots registered here.
func (s *Service) AddRoot(input string) (Root, error) {
	cleaned, err := normalizeInput(input)
	if err != nil {
		return Root{}, err
	}

	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return Root{}, fmt.Errorf("%w: path does not exist: %q", ErrInvalidRoot, input)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Root{}, fmt.Errorf("%w: cannot inspect %q: %v", ErrInvalidRoot, input, err)
	}
	if !info.IsDir() {
		return Root{}, fmt.Errorf("%w: not a directory: %q", ErrInvalidRoot, input)
	}

	row := database.WorkspaceRoot{Path: resolved}
	if err := s.db.Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Root{}, fmt.Errorf("%w: %q", ErrRootExists, resolved)
		}
		return Root{}, fmt.Errorf("workspaces: add workspace root: %w", err)
	}
	return rootFromModel(row), nil
}

// RemoveRoot unregisters the workspace root with the given id. Removing a
// root never touches the directory on disk.
func (s *Service) RemoveRoot(id uint) error {
	result := s.db.Delete(&database.WorkspaceRoot{}, id)
	if result.Error != nil {
		return fmt.Errorf("workspaces: remove workspace root: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRootNotFound
	}
	return nil
}

// rootFromModel maps the persistence model to the JSON-stable DTO.
func rootFromModel(row database.WorkspaceRoot) Root {
	return Root{ID: row.ID, Path: row.Path, CreatedAt: row.CreatedAt}
}

// RegisterRoutes registers the workspace-root management routes under r. The
// caller must protect r with session authentication (auth.RequireSession);
// CSRF enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/workspaces/roots", s.handleListRoots)
	r.POST("/workspaces/roots", s.handleAddRoot)
	r.DELETE("/workspaces/roots/:id", s.handleRemoveRoot)
}

// errorResponse is the JSON error envelope for workspace API failures.
type errorResponse struct {
	Error string `json:"error"`
}

// respondError stops the chain and writes a JSON error envelope.
func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// rootListResponse is the JSON body of GET /api/workspaces/roots.
type rootListResponse struct {
	Roots []Root `json:"roots"`
}

// addRootRequest is the JSON body of POST /api/workspaces/roots.
type addRootRequest struct {
	Path string `json:"path"`
}

// handleListRoots returns the registered workspace roots.
func (s *Service) handleListRoots(c *gin.Context) {
	roots, err := s.Roots()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, rootListResponse{Roots: roots})
}

// handleAddRoot registers a new workspace root.
func (s *Service) handleAddRoot(c *gin.Context) {
	var req addRootRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	root, err := s.AddRoot(req.Path)
	if err != nil {
		switch {
		case errors.Is(err, ErrRootExists):
			respondError(c, http.StatusConflict, "root_already_registered")
		case errors.Is(err, ErrInvalidRoot), errors.Is(err, ErrInvalidPath):
			respondError(c, http.StatusBadRequest, "invalid_root")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}
	c.JSON(http.StatusCreated, root)
}

// handleRemoveRoot unregisters a workspace root by id.
func (s *Service) handleRemoveRoot(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := s.RemoveRoot(uint(id)); err != nil {
		if errors.Is(err, ErrRootNotFound) {
			respondError(c, http.StatusNotFound, "root_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.Status(http.StatusNoContent)
}
