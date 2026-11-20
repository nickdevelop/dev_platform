package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devplatform/internal/app/database"
	"devplatform/internal/app/secrets"
	"devplatform/internal/core/platform"
)

const (
	localShellConfigKey = "terminal.local_shell"
	terminalConfigKey   = "terminal.config"
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
		msg = fmt.Sprintf("当前系统环境不支持本地终端（宿主为 %s）", osName)
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
func (s *Service) Connect(ctx context.Context, serverID uint, rows, cols uint16, cwd string) (Session, error) {
	if serverID == 0 {
		if !s.platform.PTY().Supported() {
			return nil, platform.ErrUnsupported
		}
		shellDTO, _ := s.GetEffectiveLocalShell(ctx)
		return s.sessionManager.CreateLocalSession(rows, cols, cwd, shellDTO.Path, shellDTO.Args)
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

// ScanShells returns all available interactive shells detected on the host.
func (s *Service) ScanShells() []platform.ShellInfo {
	return s.platform.PTY().ScanShells()
}

// GetEffectiveLocalShell returns the active shell path and args for local sessions.
// If the user has saved a valid configuration, it is used; otherwise it falls back to DefaultShell.
func (s *Service) GetEffectiveLocalShell(ctx context.Context) (LocalShellDTO, error) {
	defPath, defArgs := s.platform.PTY().DefaultShell()
	shells := s.platform.PTY().ScanShells()

	val, found, err := s.repo.GetSetting(ctx, localShellConfigKey)
	if err != nil {
		return LocalShellDTO{}, fmt.Errorf("load local shell setting: %w", err)
	}

	if found && strings.TrimSpace(val) != "" {
		var cfg LocalShellConfig
		if err := json.Unmarshal([]byte(val), &cfg); err == nil && strings.TrimSpace(cfg.Path) != "" {
			name := filepath.Base(cfg.Path)
			isRec := false
			for _, sh := range shells {
				if strings.EqualFold(sh.Path, cfg.Path) {
					name = sh.Name
					isRec = sh.IsRecommended
					break
				}
			}
			return LocalShellDTO{
				Path:          cfg.Path,
				Args:          cfg.Args,
				Name:          name,
				IsCustom:      true,
				IsRecommended: isRec,
			}, nil
		}
	}

	// Fallback to platform default
	name := filepath.Base(defPath)
	isRec := true
	for _, sh := range shells {
		if strings.EqualFold(sh.Path, defPath) {
			name = sh.Name
			isRec = sh.IsRecommended
			break
		}
	}
	return LocalShellDTO{
		Path:          defPath,
		Args:          defArgs,
		Name:          name,
		IsCustom:      false,
		IsRecommended: isRec,
	}, nil
}

// UpdateLocalShell saves user-configured shell path and arguments.
func (s *Service) UpdateLocalShell(ctx context.Context, req UpdateLocalShellRequest) (*LocalShellDTO, error) {
	cleanPath := strings.TrimSpace(req.Path)
	if err := s.ValidateShellPath(cleanPath); err != nil {
		return nil, err
	}

	cfg := LocalShellConfig{
		Path: cleanPath,
		Args: req.Args,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal local shell config: %w", err)
	}

	if err := s.repo.SetSetting(ctx, localShellConfigKey, string(raw)); err != nil {
		return nil, fmt.Errorf("save local shell config: %w", err)
	}

	dto, err := s.GetEffectiveLocalShell(ctx)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

// ResetLocalShell clears custom shell configuration and reverts to system default.
func (s *Service) ResetLocalShell(ctx context.Context) (*LocalShellDTO, error) {
	if err := s.repo.SetSetting(ctx, localShellConfigKey, ""); err != nil {
		return nil, fmt.Errorf("reset local shell config: %w", err)
	}
	dto, err := s.GetEffectiveLocalShell(ctx)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

// GetTerminalConfig returns the stored terminal preferences or defaults.
func (s *Service) GetTerminalConfig(ctx context.Context) (TerminalConfigDTO, error) {
	val, found, err := s.repo.GetSetting(ctx, terminalConfigKey)
	if err != nil {
		return TerminalConfigDTO{}, fmt.Errorf("load terminal config: %w", err)
	}
	if !found || strings.TrimSpace(val) == "" {
		return TerminalConfigDTO{FontFamily: ""}, nil
	}
	var cfg TerminalConfigDTO
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return TerminalConfigDTO{FontFamily: ""}, nil
	}
	return cfg, nil
}

// UpdateTerminalConfig updates user preferences for the terminal.
func (s *Service) UpdateTerminalConfig(ctx context.Context, req UpdateTerminalConfigRequest) (*TerminalConfigDTO, error) {
	cfg := TerminalConfigDTO{
		FontFamily: strings.TrimSpace(req.FontFamily),
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal terminal config: %w", err)
	}
	if err := s.repo.SetSetting(ctx, terminalConfigKey, string(raw)); err != nil {
		return nil, fmt.Errorf("save terminal config: %w", err)
	}
	return &cfg, nil
}

// ResetTerminalConfig resets terminal preferences to platform defaults.
func (s *Service) ResetTerminalConfig(ctx context.Context) (*TerminalConfigDTO, error) {
	if err := s.repo.SetSetting(ctx, terminalConfigKey, ""); err != nil {
		return nil, fmt.Errorf("reset terminal config: %w", err)
	}
	return &TerminalConfigDTO{FontFamily: ""}, nil
}

// ValidateShellPath checks if a given path exists and is executable.
func (s *Service) ValidateShellPath(path string) error {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return errors.New("shell 路径不能为空")
	}
	// Check if path exists directly
	fi, err := os.Stat(clean)
	if err == nil {
		if fi.IsDir() {
			return errors.New("指定路径为目录，不是可执行文件")
		}
		return nil
	}
	// Also attempt LookPath in case it's a binary name on PATH
	if lp, err := exec.LookPath(clean); err == nil {
		if fi, err := os.Stat(lp); err == nil && !fi.IsDir() {
			return nil
		}
	}
	return fmt.Errorf("未找到可执行文件：%s", clean)
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
