package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"devplatform/internal/app/database"
	"devplatform/internal/core/platform"
	"golang.org/x/crypto/ssh"
)

// Session represents an active terminal connection (local PTY or remote SSH).
type Session interface {
	io.ReadWriteCloser
	ID() string
	ServerID() uint
	ServerName() string
	Resize(rows, cols uint16) error
}

// LocalSession wraps an interactive local PTY shell.
type LocalSession struct {
	id         string
	serverID   uint
	serverName string
	ptySession platform.PTYSession
	closed     bool
	mu         sync.Mutex
}

func (s *LocalSession) ID() string         { return s.id }
func (s *LocalSession) ServerID() uint     { return s.serverID }
func (s *LocalSession) ServerName() string { return s.serverName }

func (s *LocalSession) Read(p []byte) (int, error) {
	return s.ptySession.Read(p)
}

func (s *LocalSession) Write(p []byte) (int, error) {
	return s.ptySession.Write(p)
}

func (s *LocalSession) Resize(rows, cols uint16) error {
	return s.ptySession.Resize(rows, cols)
}

func (s *LocalSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.ptySession.Close()
}

// SSHSession wraps a remote SSH interactive shell.
type SSHSession struct {
	id         string
	serverID   uint
	serverName string
	client     *ssh.Client
	session    *ssh.Session
	stdinPipe  io.WriteCloser
	reader     io.Reader
	closed     bool
	mu         sync.Mutex
}

func (s *SSHSession) ID() string         { return s.id }
func (s *SSHSession) ServerID() uint     { return s.serverID }
func (s *SSHSession) ServerName() string { return s.serverName }

func (s *SSHSession) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *SSHSession) Write(p []byte) (int, error) {
	return s.stdinPipe.Write(p)
}

func (s *SSHSession) Resize(rows, cols uint16) error {
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	return s.session.WindowChange(int(rows), int(cols))
}

func (s *SSHSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var errs []error
	if s.stdinPipe != nil {
		_ = s.stdinPipe.Close()
	}
	if s.session != nil {
		if err := s.session.Close(); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, err)
		}
	}
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// SessionManager manages active interactive terminal sessions.
type SessionManager struct {
	platform platform.Platform
	sessions map[string]Session
	mu       sync.RWMutex
}

// NewSessionManager creates a SessionManager.
func NewSessionManager(p platform.Platform) *SessionManager {
	return &SessionManager{
		platform: p,
		sessions: make(map[string]Session),
	}
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateLocalSession launches a local shell in a pty.
func (m *SessionManager) CreateLocalSession(rows, cols uint16) (Session, error) {
	if !m.platform.PTY().Supported() {
		return nil, platform.ErrUnsupported
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		if _, err := os.Stat("/bin/bash"); err == nil {
			shell = "/bin/bash"
		} else {
			shell = "/bin/sh"
		}
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptySession, err := m.platform.PTY().Start(cmd, rows, cols)
	if err != nil {
		return nil, fmt.Errorf("start local pty: %w", err)
	}

	sess := &LocalSession{
		id:         generateSessionID(),
		serverID:   0,
		serverName: "本机",
		ptySession: ptySession,
	}

	m.mu.Lock()
	m.sessions[sess.id] = sess
	m.mu.Unlock()

	return sess, nil
}

// CreateSSHSession connects to a remote server and opens an interactive shell.
func (m *SessionManager) CreateSSHSession(server *database.TerminalServer, secret string, rows, cols uint16) (Session, error) {
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}

	client, err := dialSSH(server.Host, server.Port, server.Username, server.AuthType, secret, 10*time.Second)
	if err != nil {
		return nil, err
	}

	sshSess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("create ssh session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sshSess.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		_ = sshSess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("request ssh pty: %w", err)
	}

	stdinPipe, err := sshSess.StdinPipe()
	if err != nil {
		_ = sshSess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("ssh stdin pipe: %w", err)
	}

	stdoutPipe, err := sshSess.StdoutPipe()
	if err != nil {
		_ = sshSess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("ssh stdout pipe: %w", err)
	}

	stderrPipe, err := sshSess.StderrPipe()
	if err != nil {
		_ = sshSess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("ssh stderr pipe: %w", err)
	}

	if err := sshSess.Shell(); err != nil {
		_ = sshSess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("start ssh shell: %w", err)
	}

	reader := io.MultiReader(stdoutPipe, stderrPipe)

	sess := &SSHSession{
		id:         generateSessionID(),
		serverID:   server.ID,
		serverName: server.Name,
		client:     client,
		session:    sshSess,
		stdinPipe:  stdinPipe,
		reader:     reader,
	}

	m.mu.Lock()
	m.sessions[sess.id] = sess
	m.mu.Unlock()

	return sess, nil
}

// GetSession returns the active session by ID.
func (m *SessionManager) GetSession(id string) (Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

// CloseSession closes and removes an active session.
func (m *SessionManager) CloseSession(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()

	if !ok || sess == nil {
		return nil
	}
	return sess.Close()
}

// CloseAll closes all active sessions.
func (m *SessionManager) CloseAll() {
	m.mu.Lock()
	active := make([]Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		active = append(active, s)
	}
	m.sessions = make(map[string]Session)
	m.mu.Unlock()

	for _, s := range active {
		_ = s.Close()
	}
}

// dialSSH connects to an SSH server with timeout.
func dialSSH(host string, port int, username, authType, secret string, timeout time.Duration) (*ssh.Client, error) {
	if port <= 0 {
		port = 22
	}

	var authMethods []ssh.AuthMethod
	switch authType {
	case "key":
		signer, err := ssh.ParsePrivateKey([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("invalid ssh private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	case "password":
		authMethods = append(authMethods, ssh.Password(secret))
	default:
		return nil, fmt.Errorf("unsupported auth type %q", authType)
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", addr, err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// TestSSHConnection verifies that an SSH host is reachable and credentials are valid.
func TestSSHConnection(ctx context.Context, host string, port int, username, authType, secret string) error {
	client, err := dialSSH(host, port, username, authType, secret, 5*time.Second)
	if err != nil {
		return err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open test session: %w", err)
	}
	defer sess.Close()

	return nil
}
