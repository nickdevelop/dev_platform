// Package auth implements the platform's single shared-password access:
// Argon2id password storage, expiring cookie sessions, CSRF enforcement and
// bounded login throttling. It never logs passwords, session IDs or CSRF
// tokens.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// Argon2id parameters. Memory is expressed in KiB.
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// ErrPasswordAlreadySet is returned by SetPassword when a password has
// already been configured. First-run setup must not silently overwrite it.
var ErrPasswordAlreadySet = errors.New("auth: platform password is already configured")

// argon2Encoding is the unpadded base64 alphabet used by the standard
// PHC string format for Argon2 hashes.
var argon2Encoding = base64.RawStdEncoding

// HashPassword derives an Argon2id hash of password with a fresh random salt
// and returns it in the standard PHC string format
// "$argon2id$v=19$m=...,t=...,p=...$salt$hash". The plaintext is never
// stored or logged.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		argon2Encoding.EncodeToString(salt), argon2Encoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the Argon2id hash encoded
// in the PHC format. The digest comparison is constant-time. A malformed
// hash is reported as an error rather than as a mismatch.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("auth: malformed password hash")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("auth: malformed password hash version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("auth: unsupported argon2 version %d", version)
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("auth: malformed password hash parameters: %w", err)
	}
	if memory == 0 || time == 0 || threads == 0 {
		return false, errors.New("auth: malformed password hash parameters")
	}

	salt, err := argon2Encoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("auth: malformed password salt: %w", err)
	}
	want, err := argon2Encoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("auth: malformed password hash digest: %w", err)
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// PasswordConfigured reports whether a platform password has been set.
func PasswordConfigured(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&database.Setting{}).
		Where("key = ?", database.SettingKeyPasswordHash).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("auth: read password setting: %w", err)
	}
	return count > 0, nil
}

// SetPassword hashes password and stores it as the platform password. It
// refuses to overwrite an already-configured password so a first-run setup
// step cannot silently reset access.
func SetPassword(db *gorm.DB, password string) error {
	if password == "" {
		return errors.New("auth: password must not be empty")
	}

	configured, err := PasswordConfigured(db)
	if err != nil {
		return err
	}
	if configured {
		return ErrPasswordAlreadySet
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	setting := database.Setting{Key: database.SettingKeyPasswordHash, Value: hash}
	if err := db.Create(&setting).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrPasswordAlreadySet
		}
		return fmt.Errorf("auth: store password hash: %w", err)
	}
	return nil
}

// passwordHash loads the stored Argon2id hash. The boolean reports whether a
// password has been configured at all.
func passwordHash(db *gorm.DB) (string, bool, error) {
	var settings []database.Setting
	if err := db.Where("key = ?", database.SettingKeyPasswordHash).
		Limit(1).Find(&settings).Error; err != nil {
		return "", false, fmt.Errorf("auth: load password hash: %w", err)
	}
	if len(settings) == 0 {
		return "", false, nil
	}
	return settings[0].Value, true, nil
}
