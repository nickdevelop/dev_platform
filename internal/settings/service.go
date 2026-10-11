// Package settings owns platform-wide configuration that is shared by the
// single operator: the appearance preferences applied by the web UI and the
// platform-level agent configuration. Values live in the platform `settings`
// table as JSON blobs, so the schema is unchanged as the configuration grows.
package settings

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/database"
)

// appearanceKey is the settings-table key holding the appearance JSON.
const appearanceKey = "appearance.config"

// ErrInvalidAppearance reports an appearance value outside the allowed set.
var ErrInvalidAppearance = errors.New("settings: invalid appearance")

// Appearance is the platform's shared visual configuration. It is JSON-stable
// and mirrors the frontend's Appearance DTO.
type Appearance struct {
	Theme      string `json:"theme"`
	Font       string `json:"font"`
	CustomFont string `json:"customFont"`
	FontSize   int    `json:"fontSize"`
	Density    string `json:"density"`
}

// DefaultAppearance is the configuration used before the operator saves one.
func DefaultAppearance() Appearance {
	return Appearance{Theme: "light", Font: "system", CustomFont: "", FontSize: 14, Density: "standard"}
}

// Allowed values. The frontend's presets mirror these lists.
var (
	validThemes    = map[string]bool{"light": true, "dark": true, "warm": true, "cool": true}
	validFonts     = map[string]bool{"system": true, "sans": true, "serif": true, "mono": true, "custom": true}
	validDensities = map[string]bool{"compact": true, "standard": true, "relaxed": true}
)

// customFontPattern bounds a Google Fonts family name to the characters such a
// name actually uses, so it can never smuggle markup into the fonts URL.
var customFontPattern = regexp.MustCompile(`^[A-Za-z0-9 &+',.\-]{1,100}$`)

// Validate reports whether the appearance value is one the platform accepts.
func (a Appearance) Validate() error {
	if !validThemes[a.Theme] {
		return fmt.Errorf("%w: theme %q", ErrInvalidAppearance, a.Theme)
	}
	if !validFonts[a.Font] {
		return fmt.Errorf("%w: font %q", ErrInvalidAppearance, a.Font)
	}
	if a.Font == "custom" && !customFontPattern.MatchString(a.CustomFont) {
		return fmt.Errorf("%w: custom font %q", ErrInvalidAppearance, a.CustomFont)
	}
	if a.FontSize < 12 || a.FontSize > 20 {
		return fmt.Errorf("%w: font size %d", ErrInvalidAppearance, a.FontSize)
	}
	if !validDensities[a.Density] {
		return fmt.Errorf("%w: density %q", ErrInvalidAppearance, a.Density)
	}
	return nil
}

// Service reads and writes platform settings over the platform database.
type Service struct {
	db *gorm.DB
	mu sync.Mutex
	// now is the clock used for detection timestamps; tests override it.
	now func() time.Time
}

// NewService builds the settings service over the platform database.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Appearance returns the saved appearance, or the default when none is saved.
func (s *Service) Appearance() (Appearance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAppearance()
}

func (s *Service) loadAppearance() (Appearance, error) {
	var row database.Setting
	err := s.db.Where("key = ?", appearanceKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultAppearance(), nil
	}
	if err != nil {
		return Appearance{}, fmt.Errorf("settings: load appearance: %w", err)
	}

	// Decode over the defaults so a stored blob missing a field keeps the
	// default rather than the zero value.
	appearance := DefaultAppearance()
	if err := jsonUnmarshal(row.Value, &appearance); err != nil {
		return Appearance{}, fmt.Errorf("settings: decode appearance: %w", err)
	}
	return appearance, nil
}

// SetAppearance validates and persists the appearance, returning the stored
// value. An invalid value is rejected without changing what is stored.
func (s *Service) SetAppearance(appearance Appearance) (Appearance, error) {
	if err := appearance.Validate(); err != nil {
		return Appearance{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := jsonMarshal(appearance)
	if err != nil {
		return Appearance{}, fmt.Errorf("settings: encode appearance: %w", err)
	}
	row := database.Setting{Key: appearanceKey, Value: data}
	if err := s.db.Save(&row).Error; err != nil {
		return Appearance{}, fmt.Errorf("settings: save appearance: %w", err)
	}
	return appearance, nil
}
