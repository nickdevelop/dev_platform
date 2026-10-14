// Package secrets stores MCP credential values in a platform-private file,
// separate from the database and from project content. Only an opaque
// reference ever reaches the API, the UI, the logs or the resources table.
//
// Threat model: the secret file is plaintext at rest with owner-only
// permissions (0600 on Linux/WSL, 0700 for its directory). This does not
// protect against a process running as the same user or with higher
// privileges, nor against a backup that includes the file. It does protect
// against other local users and against accidental disclosure through the
// API, logs or diagnostics.
package secrets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"devplatform/internal/platform"
)

const (
	// FileName is the name of the secret file inside the platform data
	// directory.
	FileName = "secrets.json"
	// FileMode is the owner-only permission the secret file carries. The
	// enforcement lives behind the platform Perms port; this constant
	// documents the unix value.
	FileMode os.FileMode = 0o600
	// dirMode is the owner-only permission the secret file's directory
	// carries. See FileMode.
	dirMode os.FileMode = 0o700
)

var (
	// ErrSecretNotFound reports a reference with no stored value.
	ErrSecretNotFound = errors.New("secrets: secret not found")
	// ErrInvalidSecretID reports a reference that is not a safe opaque id.
	ErrInvalidSecretID = errors.New("secrets: invalid secret id")
	// ErrEmptySecretValue reports an empty credential value, which is never
	// stored: an empty write would destroy a working credential.
	ErrEmptySecretValue = errors.New("secrets: secret value is empty")
)

// secretIDPattern bounds an opaque reference to a safe, path-free token.
var secretIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Store is a file-backed secret value store. Values are addressed by an
// opaque id; a missing file is an empty store, and every write atomically
// replaces the file without widening its permissions.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore builds a secret store rooted at the platform data directory. The
// file is created lazily on the first write.
func NewStore(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("secrets: data directory is required")
	}
	return &Store{path: filepath.Join(dataDir, FileName)}, nil
}

// Path is the location of the secret file; exposed for permission checks.
func (s *Store) Path() string { return s.path }

// Set stores value under id, replacing any previous value (rotation).
func (s *Store) Set(ctx context.Context, id, value string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if value == "" {
		return ErrEmptySecretValue
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	values, err := s.load()
	if err != nil {
		return err
	}
	values[id] = value
	return s.save(values)
}

// Get returns the value stored under id.
func (s *Store) Get(ctx context.Context, id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	values, err := s.load()
	if err != nil {
		return "", err
	}
	value, ok := values[id]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, id)
	}
	return value, nil
}

// Delete removes the value stored under id.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	values, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := values[id]; !ok {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, id)
	}
	delete(values, id)
	return s.save(values)
}

// Exists reports whether a value is stored under id without reading it.
func (s *Store) Exists(ctx context.Context, id string) (bool, error) {
	if err := validateID(id); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	values, err := s.load()
	if err != nil {
		return false, err
	}
	_, ok := values[id]
	return ok, nil
}

// load reads the id→value map. A missing file is an empty map; a symlinked
// file is refused rather than followed, so a symlink planted at the secret
// path cannot redirect a read to another file.
func (s *Store) load() (map[string]string, error) {
	info, err := os.Lstat(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("secrets: inspect secret file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("secrets: secret file %s is a symlink; refusing to follow it", s.path)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("secrets: read secret file: %w", err)
	}
	values := map[string]string{}
	if len(data) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("secrets: parse secret file: %w", err)
	}
	return values, nil
}

// save encodes the map into the secret file atomically: a temp file in the
// same directory is written with owner-only permissions and then renamed over
// the target, so a crash never leaves a partial file and the rename replaces
// the directory entry rather than following a symlink.
func (s *Store) save(values map[string]string) error {
	dir := filepath.Dir(s.path)
	if err := platform.New().Perms().PrivateDir(dir); err != nil {
		return fmt.Errorf("secrets: create data directory: %w", err)
	}

	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("secrets: encode secrets: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("secrets: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if err := platform.New().Perms().PrivateFile(tmpName); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secrets: set permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secrets: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secrets: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("secrets: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("secrets: install secret file: %w", err)
	}
	return nil
}

// NewSecretID returns a fresh opaque reference. It carries no meaning and is
// safe to expose in the API.
func NewSecretID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("secrets: generate id: %w", err)
	}
	return "sec_" + hex.EncodeToString(buf[:]), nil
}

// validateID rejects references that are not safe, path-free tokens.
func validateID(id string) error {
	if !secretIDPattern.MatchString(id) {
		return fmt.Errorf("%w: must be 1-128 characters of [A-Za-z0-9_-]", ErrInvalidSecretID)
	}
	return nil
}
