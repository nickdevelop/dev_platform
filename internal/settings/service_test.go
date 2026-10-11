package settings

import (
	"errors"
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
	svc := NewService(openTestDB(t))

	got, err := svc.Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	want := Appearance{Theme: "light", Font: "system", CustomFont: "", FontSize: 14, Density: "standard"}
	if got != want {
		t.Errorf("Appearance() = %+v, want %+v", got, want)
	}
}

func TestSetAppearanceRoundTrips(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	input := Appearance{Theme: "dark", Font: "custom", CustomFont: "Noto Sans SC", FontSize: 18, Density: "compact"}
	saved, err := svc.SetAppearance(input)
	if err != nil {
		t.Fatalf("SetAppearance: %v", err)
	}
	if saved != input {
		t.Errorf("SetAppearance returned %+v, want %+v", saved, input)
	}

	got, err := NewService(db).Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	if got != input {
		t.Errorf("reloaded Appearance = %+v, want %+v", got, input)
	}
}

func TestSetAppearanceRejectsInvalidValues(t *testing.T) {
	svc := NewService(openTestDB(t))
	valid := DefaultAppearance()

	cases := []struct {
		name   string
		mutate func(*Appearance)
	}{
		{"theme", func(a *Appearance) { a.Theme = "neon" }},
		{"font", func(a *Appearance) { a.Font = "comic" }},
		{"density", func(a *Appearance) { a.Density = "huge" }},
		{"font size low", func(a *Appearance) { a.FontSize = 11 }},
		{"font size high", func(a *Appearance) { a.FontSize = 21 }},
		{"custom font empty", func(a *Appearance) { a.Font = "custom"; a.CustomFont = "" }},
		{"custom font illegal", func(a *Appearance) { a.Font = "custom"; a.CustomFont = "bad<script>" }},
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
