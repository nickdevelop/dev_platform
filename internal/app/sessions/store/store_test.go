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
	byNative, err := s.GetByNative(1, "pi", "native")
	if err != nil || byNative.ID != "p1" {
		t.Fatalf("GetByNative = (%+v, %v), want the created row", byNative, err)
	}
	rows, err := s.List(1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("List = (%d rows, %v), want 1", len(rows), err)
	}

	if err := s.UpdateStatus("p1", "running"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	updated, err := s.GetByID("p1")
	if err != nil || updated.Status != "running" {
		t.Fatalf("GetByID after UpdateStatus = (%+v, %v), want running", updated, err)
	}

	touchTime := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	if err := s.TouchUpdatedAt("p1", touchTime); err != nil {
		t.Fatalf("TouchUpdatedAt: %v", err)
	}
	touched, err := s.GetByID("p1")
	if err != nil || !touched.UpdatedAt.Equal(touchTime) {
		t.Fatalf("GetByID after TouchUpdatedAt = (%+v, %v), want %v", touched, err, touchTime)
	}

	if err := s.Delete("p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.GetByID("p1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID after delete = %v, want ErrNotFound", err)
	}
}

func TestStoreListOrderingAndPreservedTimestamps(t *testing.T) {
	s := openStore(t)
	t1 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)

	// Create 3 sessions with different updated_at timestamps
	s1 := database.AgentSession{
		ID: "sess-1", ProjectID: 10, Backend: "pi", InstanceID: "inst-1",
		NativeSessionID: "n-1", Title: "Old Session", Status: "idle",
		CreatedAt: t1, UpdatedAt: t1,
	}
	s2 := database.AgentSession{
		ID: "sess-2", ProjectID: 10, Backend: "pi", InstanceID: "inst-1",
		NativeSessionID: "n-2", Title: "Middle Session", Status: "idle",
		CreatedAt: t2, UpdatedAt: t2,
	}
	s3 := database.AgentSession{
		ID: "sess-3", ProjectID: 10, Backend: "pi", InstanceID: "inst-1",
		NativeSessionID: "n-3", Title: "Newest Session", Status: "idle",
		CreatedAt: t3, UpdatedAt: t3,
	}
	for _, row := range []database.AgentSession{s1, s2, s3} {
		if err := s.Create(row); err != nil {
			t.Fatalf("Create(%s): %v", row.ID, err)
		}
		// Pin timestamps explicitly since GORM might auto-assign now()
		if err := s.TouchUpdatedAt(row.ID, row.UpdatedAt); err != nil {
			t.Fatalf("TouchUpdatedAt(%s): %v", row.ID, err)
		}
	}

	// List should return newest updated_at first: sess-3, sess-2, sess-1
	list, err := s.List(10)
	if err != nil {
		t.Fatalf("List(10): %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d items, want 3", len(list))
	}
	if list[0].ID != "sess-3" || list[1].ID != "sess-2" || list[2].ID != "sess-1" {
		t.Errorf("List order = [%s, %s, %s], want [sess-3, sess-2, sess-1]", list[0].ID, list[1].ID, list[2].ID)
	}

	// Verify UpdateInstanceID does NOT bump updated_at
	if err := s.UpdateInstanceID("sess-1", "inst-rebound"); err != nil {
		t.Fatalf("UpdateInstanceID: %v", err)
	}
	s1Reloaded, err := s.GetByID("sess-1")
	if err != nil {
		t.Fatalf("GetByID(sess-1): %v", err)
	}
	if s1Reloaded.InstanceID != "inst-rebound" {
		t.Errorf("InstanceID = %q, want inst-rebound", s1Reloaded.InstanceID)
	}
	if !s1Reloaded.UpdatedAt.Equal(t1) {
		t.Errorf("UpdatedAt = %v, want preserved %v", s1Reloaded.UpdatedAt, t1)
	}

	// Verify UpdateModel does NOT bump updated_at
	if err := s.UpdateModel("sess-1", "claude-3-7"); err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}
	s1Reloaded2, err := s.GetByID("sess-1")
	if err != nil {
		t.Fatalf("GetByID(sess-1): %v", err)
	}
	if s1Reloaded2.Model != "claude-3-7" {
		t.Errorf("Model = %q, want claude-3-7", s1Reloaded2.Model)
	}
	if !s1Reloaded2.UpdatedAt.Equal(t1) {
		t.Errorf("UpdatedAt = %v, want preserved %v", s1Reloaded2.UpdatedAt, t1)
	}
}
