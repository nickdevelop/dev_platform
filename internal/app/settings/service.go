// Package settings owns platform-wide configuration that is shared by the
// single operator: the appearance preferences applied by the web UI and the
// platform-level agent configuration. Values live in the platform `settings`
// table as JSON blobs, so the schema is unchanged as the configuration grows.
package settings

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform"
)

// appearanceKey is the settings-table key holding the appearance JSON.
const appearanceKey = "appearance.config"

// quickAssistantKey is the settings-table key holding the quick-assistant
// feature toggle. It is kept apart from the visual appearance config so a
// feature switch never rewrites theme preferences.
const quickAssistantKey = "quick-assistant.config"

// ErrInvalidAppearance reports an appearance value outside the allowed set.
var ErrInvalidAppearance = errors.New("settings: invalid appearance")

// Appearance is the platform's shared visual configuration. It is JSON-stable
// and mirrors the frontend's Appearance DTO. The type of face is not part of
// it: the UI bundles its own fonts (web/src/fonts) and always uses them.
type Appearance struct {
	Theme             string `json:"theme"`
	FontSize          int    `json:"fontSize"`
	Density           string `json:"density"`
	PlantumlMode      string `json:"plantumlMode"`
	PlantumlServerURL string `json:"plantumlServerUrl"`
}

// DefaultAppearance is the configuration used before the operator saves one.
func DefaultAppearance() Appearance {
	return Appearance{
		Theme:             "light",
		FontSize:          14,
		Density:           "standard",
		PlantumlMode:      "local",
		PlantumlServerURL: "https://www.plantuml.com/plantuml",
	}
}

// QuickAssistant is the platform-wide gate for the floating quick-assistant
// surface. It defaults to off so a fresh install does not show an unexplained
// floating control.
type QuickAssistant struct {
	Enabled bool `json:"enabled"`
}

// DefaultQuickAssistant is the configuration used before the operator saves one.
func DefaultQuickAssistant() QuickAssistant {
	return QuickAssistant{Enabled: false}
}

// Allowed values. The frontend's presets mirror these lists.
var (
	validThemes         = map[string]bool{"light": true, "dark": true, "warm": true, "cool": true}
	validDensities      = map[string]bool{"compact": true, "standard": true, "relaxed": true}
	validPlantumlModes  = map[string]bool{"local": true, "online": true}
)

// Validate reports whether the appearance value is one the platform accepts.
func (a *Appearance) Validate() error {
	if !validThemes[a.Theme] {
		return fmt.Errorf("%w: theme %q", ErrInvalidAppearance, a.Theme)
	}
	if a.FontSize < 12 || a.FontSize > 20 {
		return fmt.Errorf("%w: font size %d", ErrInvalidAppearance, a.FontSize)
	}
	if !validDensities[a.Density] {
		return fmt.Errorf("%w: density %q", ErrInvalidAppearance, a.Density)
	}
	if a.PlantumlMode == "" {
		a.PlantumlMode = "local"
	}
	if !validPlantumlModes[a.PlantumlMode] {
		return fmt.Errorf("%w: plantuml mode %q", ErrInvalidAppearance, a.PlantumlMode)
	}
	if a.PlantumlServerURL == "" {
		a.PlantumlServerURL = "https://www.plantuml.com/plantuml"
	}
	return nil
}

// Service reads and writes platform settings over the platform database.
type Service struct {
	repo Repository
	mu sync.Mutex
	// now is the clock used for detection timestamps; tests override it.
	now func() time.Time
	// catalog describes every backend the platform knows: its default
	// enablement, executable name and presentation order.
	catalog *agents.Catalog
	// platform carries the OS port layer; tests inject the fake.
	platform platform.Platform
}

// NewService builds the settings service over the platform database and the
// backend catalog.
func NewService(db *gorm.DB, catalog *agents.Catalog) *Service {
	return NewServiceWithRepository(NewGORMRepository(db), catalog)
}

// NewServiceWithRepository builds the settings service over a custom repository.
func NewServiceWithRepository(repo Repository, catalog *agents.Catalog) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }, catalog: catalog, platform: platform.New()}
}

// Appearance returns the saved appearance, or the default when none is saved.
func (s *Service) Appearance() (Appearance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAppearance()
}

func (s *Service) loadAppearance() (Appearance, error) {
	val, found, err := s.repo.Get(appearanceKey)
	if err != nil {
		return Appearance{}, err
	}
	if !found {
		return DefaultAppearance(), nil
	}

	appearance := DefaultAppearance()
	if err := jsonUnmarshal(val, &appearance); err != nil {
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
	if err := s.repo.Set(appearanceKey, data); err != nil {
		return Appearance{}, err
	}
	return appearance, nil
}

// QuickAssistant returns the saved quick-assistant toggle, or the default
// (disabled) when none is saved.
func (s *Service) QuickAssistant() (QuickAssistant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadQuickAssistant()
}

func (s *Service) loadQuickAssistant() (QuickAssistant, error) {
	val, found, err := s.repo.Get(quickAssistantKey)
	if err != nil {
		return QuickAssistant{}, err
	}
	if !found {
		return DefaultQuickAssistant(), nil
	}
	config := DefaultQuickAssistant()
	if err := jsonUnmarshal(val, &config); err != nil {
		return QuickAssistant{}, fmt.Errorf("settings: decode quick assistant: %w", err)
	}
	return config, nil
}

// SetQuickAssistant persists the feature toggle and returns the stored value.
func (s *Service) SetQuickAssistant(config QuickAssistant) (QuickAssistant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := jsonMarshal(config)
	if err != nil {
		return QuickAssistant{}, fmt.Errorf("settings: encode quick assistant: %w", err)
	}
	if err := s.repo.Set(quickAssistantKey, data); err != nil {
		return QuickAssistant{}, err
	}
	return config, nil
}
