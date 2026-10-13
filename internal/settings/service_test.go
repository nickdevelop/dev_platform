package settings

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/database"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	db.Logger = logger.Discard
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestAppearanceDefaultsWhenUnset(t *testing.T) {
	svc := NewService(openTestDB(t), testCatalog(t))

	got, err := svc.Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	want := Appearance{Theme: "light", FontSize: 14, Density: "standard"}
	if got != want {
		t.Errorf("Appearance() = %+v, want %+v", got, want)
	}
}

func TestSetAppearanceRoundTrips(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, testCatalog(t))

	input := Appearance{Theme: "dark", FontSize: 18, Density: "compact"}
	saved, err := svc.SetAppearance(input)
	if err != nil {
		t.Fatalf("SetAppearance: %v", err)
	}
	if saved != input {
		t.Errorf("SetAppearance returned %+v, want %+v", saved, input)
	}

	got, err := NewService(db, testCatalog(t)).Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	if got != input {
		t.Errorf("reloaded Appearance = %+v, want %+v", got, input)
	}
}

func TestSetAppearanceRejectsInvalidValues(t *testing.T) {
	svc := NewService(openTestDB(t), testCatalog(t))
	valid := DefaultAppearance()

	cases := []struct {
		name   string
		mutate func(*Appearance)
	}{
		{"theme", func(a *Appearance) { a.Theme = "neon" }},
		{"density", func(a *Appearance) { a.Density = "huge" }},
		{"font size low", func(a *Appearance) { a.FontSize = 11 }},
		{"font size high", func(a *Appearance) { a.FontSize = 21 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.mutate(&input)
			if _, err := svc.SetAppearance(input); !errors.Is(err, ErrInvalidAppearance) {
				t.Errorf("SetAppearance(%+v) error = %v, want ErrInvalidAppearance", input, err)
			}
		})
	}

	// A rejected write must not have persisted anything.
	got, err := svc.Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	if got != valid {
		t.Errorf("stored appearance = %+v, want default %+v", got, valid)
	}
}

// The type of face is built into the UI, not operator-selectable, so the
// stored blob must stop carrying font fields.
func TestStoredAppearanceHasNoFontFields(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, testCatalog(t))

	if _, err := svc.SetAppearance(DefaultAppearance()); err != nil {
		t.Fatalf("SetAppearance: %v", err)
	}

	var row database.Setting
	if err := db.Where("key = ?", appearanceKey).First(&row).Error; err != nil {
		t.Fatalf("load stored row: %v", err)
	}
	for _, key := range []string{`"font"`, `"customFont"`} {
		if strings.Contains(row.Value, key) {
			t.Errorf("stored appearance %s contains %s", row.Value, key)
		}
	}
}

// Blobs saved while the font was still selectable carry font and customFont;
// loading one must ignore those keys instead of failing.
func TestAppearanceIgnoresStoredFontFields(t *testing.T) {
	db := openTestDB(t)
	stored := `{"theme":"dark","font":"mono","customFont":"Fira Code","fontSize":16,"density":"compact"}`
	if err := db.Save(&database.Setting{Key: appearanceKey, Value: stored}).Error; err != nil {
		t.Fatalf("store legacy appearance: %v", err)
	}

	got, err := NewService(db, testCatalog(t)).Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	want := Appearance{Theme: "dark", FontSize: 16, Density: "compact"}
	if got != want {
		t.Errorf("Appearance() = %+v, want %+v", got, want)
	}
}
