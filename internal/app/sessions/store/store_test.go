package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"devplatform/internal/app/database"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return New(db)
}

func TestStoreRoundTripsAnIndexRow(t *testing.T) {
	s := openStore(t)
	row := database.AgentSession{
		ID: "p1", ProjectID: 1, Backend: "pi", InstanceID: "inst",
		NativeSessionID: "native", Title: "t", Status: "idle",
	}
	if err := s.Create(row); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byID, err := s.GetByID("p1")
	if err != nil || byID.NativeSessionID != "native" {
		t.Fatalf("GetByID = (%+v, %v), want the created row", byID, err)
	}
	byNative, err := s.GetByNative("inst", "native")
	if err != nil || byNative.ID != "p1" {
		t.Fatalf("GetByNative = (%+v, %v), want the created row", byNative, err)
	}
	rows, err := s.List(1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("List = (%d rows, %v), want 1", len(rows), err)
	}

	if err := s.UpdateStatus("p1", "running", time.Now().UTC()); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := s.Delete("p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.GetByID("p1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID after delete = %v, want ErrNotFound", err)
	}
}
