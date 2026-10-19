package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"devplatform/internal/app/database"
	"devplatform/internal/app/secrets"
	"devplatform/internal/core/platform"
)

var (
	// ErrLocalImmutable reports an attempt to edit or delete the built-in local server.
	ErrLocalImmutable = errors.New("cannot modify or delete the local server")
	// ErrServerNotFound reports that a server ID does not exist.
	ErrServerNotFound = errors.New("server not found")
)

// Service provides terminal server operations and session orchestration.
type Service struct {
	repo           Repository
	secrets        *secrets.Store
	platform       platform.Platform
	sessionManager *SessionManager
}

// NewService creates a new terminal Service.
func NewService(repo Repository, secretStore *secrets.Store, p platform.Platform) *Service {
	return &Service{
		repo:           repo,
		secrets:        secretStore,
		platform:       p,
		sessionManager: NewSessionManager(p),
	}
}

// Status reports whether terminal hosting is supported on this platform.
func (s *Service) Status() StatusDTO {
	supported := s.platform.PTY().Supported()
	osName := s.platform.Name()
	msg := ""
	if !supported {
		msg = fmt.Sprintf("终端功能仅在 Linux 服务器环境中支持（当前宿主为 %s）", osName)
	}
	return StatusDTO{
		Supported: supported,
		OS:        osName,
		Message:   msg,
	}
}

// LocalServerDTO returns the standard representation for the host machine.
func LocalServerDTO() ServerDTO {
	return ServerDTO{
		ID:        0,
		Name:      "本机 (Localhost)",
		Host:      "localhost",
		Port:      0,
		Username:  "local",
		AuthType:  "local",
		IsLocal:   true,
		HasSecret: false,
	}
}

// ListServers returns the list of configured servers, starting with localhost.
func (s *Service) ListServers(ctx context.Context) ([]ServerDTO, error) {
	out := []ServerDTO{LocalServerDTO()}

	dbServers, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list terminal servers: %w", err)
	}

	for _, srv := range dbServers {
		out = append(out, toDTO(srv))
	}
	return out, nil
}

// GetServer returns a single server by ID.
func (s *Service) GetServer(ctx context.Context, id uint) (*ServerDTO, error) {
	if id == 0 {
		dto := LocalServerDTO()
		return &dto, nil
	}

	srv, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}
	dto := toDTO(*srv)
	return &dto, nil
}

// CreateServer registers a new remote third-party server.
func (s *Service) CreateServer(ctx context.Context, req CreateServerRequest) (*ServerDTO, error) {
	if req.Port <= 0 {
		req.Port = 22
	}

	secretID := generateSecretID()
	if err := s.secrets.Set(ctx, secretID, req.Secret); err != nil {
		return nil, fmt.Errorf("save server secret: %w", err)
	}

	srv := database.TerminalServer{
		Name:      req.Name,
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		AuthType:  req.AuthType,
		SecretRef: secretID,
		IsLocal:   false,
	}

	if err := s.repo.Create(ctx, &srv); err != nil {
		return nil, fmt.Errorf("create terminal server: %w", err)
	}

	dto := toDTO(srv)
	return &dto, nil
}

// UpdateServer edits an existing remote server configuration.
func (s *Service) UpdateServer(ctx context.Context, id uint, req UpdateServerRequest) (*ServerDTO, error) {
	if id == 0 {
		return nil, ErrLocalImmutable
	}

	srv, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}

	if req.Port <= 0 {
		req.Port = 22
	}

	srv.Name = req.Name
	srv.Host = req.Host
	srv.Port = req.Port
	srv.Username = req.Username
	srv.AuthType = req.AuthType

	if req.Secret != "" {
		if srv.SecretRef == "" {
			srv.SecretRef = generateSecretID()
		}
		if err := s.secrets.Set(ctx, srv.SecretRef, req.Secret); err != nil {
			return nil, fmt.Errorf("update server secret: %w", err)
		}
	}

	if err := s.repo.Update(ctx, srv); err != nil {
		return nil, fmt.Errorf("update terminal server: %w", err)
	}

	dto := toDTO(*srv)
	return &dto, nil
}

// DeleteServer deletes a remote server configuration and cleans up credentials.
func (s *Service) DeleteServer(ctx context.Context, id uint) error {
	if id == 0 {
		return ErrLocalImmutable
	}

	srv, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if srv == nil {
		return ErrServerNotFound
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete terminal server: %w", err)
	}

	return nil
}

// TestConnection tests SSH connectivity to a remote server.
func (s *Service) TestConnection(ctx context.Context, req TestConnectionRequest) error {
	secret := req.Secret
	host := req.Host
	port := req.Port
	username := req.Username
	authType := req.AuthType

	if req.ServerID != nil && *req.ServerID > 0 {
		srv, err := s.repo.Get(ctx, *req.ServerID)
		if err != nil {
			return err
		}
		if srv == nil {
			return ErrServerNotFound
		}
		host = srv.Host
		port = srv.Port
		username = srv.Username
		authType = srv.AuthType
		if secret == "" {
			storedSecret, err := s.secrets.Get(ctx, srv.SecretRef)
			if err != nil {
				return fmt.Errorf("read stored secret: %w", err)
			}
			secret = storedSecret
		}
	}

	if secret == "" {
		return errors.New("secret credential is required to test connection")
	}

	return TestSSHConnection(ctx, host, port, username, authType, secret)
}

// Connect creates and returns an interactive terminal session for serverID.
func (s *Service) Connect(ctx context.Context, serverID uint, rows, cols uint16) (Session, error) {
	if serverID == 0 {
		if !s.platform.PTY().Supported() {
			return nil, platform.ErrUnsupported
		}
		return s.sessionManager.CreateLocalSession(rows, cols)
	}

	srv, err := s.repo.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}

	secret, err := s.secrets.Get(ctx, srv.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("read server secret: %w", err)
	}

	return s.sessionManager.CreateSSHSession(srv, secret, rows, cols)
}

// CloseSession closes an active session.
func (s *Service) CloseSession(id string) error {
	return s.sessionManager.CloseSession(id)
}

// Close stops all active terminal sessions.
func (s *Service) Close() {
	s.sessionManager.CloseAll()
}

func toDTO(s database.TerminalServer) ServerDTO {
	return ServerDTO{
		ID:        s.ID,
		Name:      s.Name,
		Host:      s.Host,
		Port:      s.Port,
		Username:  s.Username,
		AuthType:  s.AuthType,
		IsLocal:   s.IsLocal,
		HasSecret: s.SecretRef != "",
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func generateSecretID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "term_" + hex.EncodeToString(b)
}
