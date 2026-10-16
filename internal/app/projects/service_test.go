package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/app/database"
	"devplatform/internal/app/paths"
)

// openTestDB opens and migrates a throwaway platform database. The GORM
// logger is discarded because these tests deliberately exercise expected-error
// paths whose warnings would otherwise pollute the output.
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

// fakeStopper stands in for the instance service. It records the projects it
// was asked to stop; err makes the stop fail, and onStop (when set) runs for
// each call, for example to simulate an instance started during the stop.
type fakeStopper struct {
	mu     sync.Mutex
	err    error
	onStop func(projectID uint)
	calls  []uint
}

func (f *fakeStopper) StopProjectInstances(_ context.Context, projectID uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, projectID)
	if f.onStop != nil {
		f.onStop(projectID)
	}
	return f.err
}

func (f *fakeStopper) stopCalls() []uint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint(nil), f.calls...)
}

// newTestService builds a project service over a throwaway database with a
// fake instance stopper, returning the service, database, a canonical working
// directory and the stopper.
func newTestService(t *testing.T) (*Service, *gorm.DB, string, *fakeStopper) {
	t.Helper()

	db := openTestDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	stopper := &fakeStopper{}
	svc := NewService(db)
	svc.SetInstanceStopper(stopper)
	return svc, db, root, stopper
}

// mustMkdir creates dir and fails the test on error.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
}

// projectCount returns the number of stored project records.
func projectCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var n int64
	if err := db.Model(&database.Project{}).Count(&n).Error; err != nil {
		t.Fatalf("count projects: %v", err)
	}
	return n
}

func TestRegisterExistingDirectory(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	dir := filepath.Join(root, "alpha")
	mustMkdir(t, dir)

	project, err := svc.Register(dir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if project.ID == 0 {
		t.Errorf("registered project has zero id")
	}
	if project.Name != "alpha" {
		t.Errorf("name = %q, want %q", project.Name, "alpha")
	}
	if project.Path != canonical {
		t.Errorf("path = %q, want canonical %q", project.Path, canonical)
	}
	if got := projectCount(t, db); got != 1 {
		t.Fatalf("project count = %d, want 1", got)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != project.ID {
		t.Errorf("list = %+v, want the registered project", list)
	}
}

func TestRegisterRejectsInvalidDirectories(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	filePath := filepath.Join(root, "a-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cases := []struct {
		name  string
		input string
		want  error
	}{
		{"missing", filepath.Join(root, "missing"), ErrDirectoryMissing},
		{"file", filePath, ErrNotDirectory},
		{"empty", "", paths.ErrInvalidPath},
		{"relative", "relative/path", paths.ErrInvalidPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Register(tc.input); !errors.Is(err, tc.want) {
				t.Errorf("Register(%q) error = %v, want %v", tc.input, err, tc.want)
			}
		})
	}
	if got := projectCount(t, db); got != 0 {
		t.Errorf("project count = %d, want 0 after rejected registrations", got)
	}
}

func TestRegisterRejectsDuplicatePath(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	dir := filepath.Join(root, "dup")
	mustMkdir(t, dir)
	if _, err := svc.Register(dir); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	if _, err := svc.Register(dir); !errors.Is(err, ErrProjectExists) {
		t.Fatalf("Register duplicate error = %v, want ErrProjectExists", err)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want 1", got)
	}
}

func TestCreateProject(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	project, err := svc.Create(root, "beta")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := filepath.Join(root, "beta")
	if project.Name != "beta" {
		t.Errorf("name = %q, want beta", project.Name)
	}
	if project.Path != want {
		t.Errorf("path = %q, want %q", project.Path, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("created directory: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", want)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want 1", got)
	}
}

func TestCreateRejectsInvalidNames(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "nul\x00byte"} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			if _, err := svc.Create(root, name); !errors.Is(err, ErrInvalidName) {
				t.Errorf("Create(%q) error = %v, want ErrInvalidName", name, err)
			}
		})
	}
	if got := projectCount(t, db); got != 0 {
		t.Errorf("project count = %d, want 0", got)
	}
}

func TestCreateRejectsCollisionAndMissingParent(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	mustMkdir(t, filepath.Join(root, "exists"))
	if _, err := svc.Create(root, "exists"); !errors.Is(err, ErrTargetExists) {
		t.Errorf("Create over existing target error = %v, want ErrTargetExists", err)
	}
	if _, err := svc.Create(filepath.Join(root, "nope", "deeper"), "child"); !errors.Is(err, ErrDirectoryMissing) {
		t.Errorf("Create with missing parent error = %v, want ErrDirectoryMissing", err)
	}
	if got := projectCount(t, db); got != 0 {
		t.Errorf("project count = %d, want 0", got)
	}
}

func TestRenameProject(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	project, err := svc.Create(root, "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	renamed, err := svc.Rename(project.ID, "new", "old")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	want := filepath.Join(root, "new")
	if renamed.ID != project.ID {
		t.Errorf("id changed from %d to %d", project.ID, renamed.ID)
	}
	if renamed.Name != "new" || renamed.Path != want {
		t.Errorf("renamed = %+v, want name new path %q", renamed, want)
	}
	if _, err := os.Stat(filepath.Join(root, "old")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old path still exists: %v", err)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Errorf("new path not a directory: err=%v", err)
	}

	// A mismatched typed confirmation must not mutate anything.
	if _, err := svc.Rename(project.ID, "other", "wrong"); !errors.Is(err, ErrConfirmationMismatch) {
		t.Errorf("Rename wrong confirmation error = %v, want ErrConfirmationMismatch", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("directory moved despite confirmation mismatch: %v", err)
	}

	// An occupied target is never overwritten.
	mustMkdir(t, filepath.Join(root, "taken"))
	if _, err := svc.Rename(project.ID, "taken", "new"); !errors.Is(err, ErrTargetExists) {
		t.Errorf("Rename collision error = %v, want ErrTargetExists", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("source renamed despite collision: %v", err)
	}

	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want 1", got)
	}
}

func TestMoveProject(t *testing.T) {
	svc, _, root, _ := newTestService(t)

	project, err := svc.Create(root, "src")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mustMkdir(t, filepath.Join(root, "nested"))

	dest := filepath.Join(root, "nested", "dst")
	moved, err := svc.Move(project.ID, dest, "src")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.ID != project.ID || moved.Name != "dst" || moved.Path != dest {
		t.Errorf("moved = %+v, want id %d name dst path %q", moved, project.ID, dest)
	}
	if _, err := os.Stat(filepath.Join(root, "src")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source still exists after move: %v", err)
	}
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		t.Errorf("destination not a directory: err=%v", err)
	}

	mustMkdir(t, filepath.Join(root, "collide"))
	if _, err := svc.Move(project.ID, filepath.Join(root, "collide"), "dst"); !errors.Is(err, ErrTargetExists) {
		t.Errorf("Move collision error = %v, want ErrTargetExists", err)
	}
	if _, err := svc.Move(project.ID, filepath.Join(root, "other"), "wrong"); !errors.Is(err, ErrConfirmationMismatch) {
		t.Errorf("Move wrong confirmation error = %v, want ErrConfirmationMismatch", err)
	}
	if _, err := svc.Move(project.ID, filepath.Join(root, "gone", "deep"), "dst"); !errors.Is(err, ErrDirectoryMissing) {
		t.Errorf("Move missing destination parent error = %v, want ErrDirectoryMissing", err)
	}
	if _, err := svc.Move(project.ID, filepath.Join(dest, "inner"), "dst"); !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("Move into own subtree error = %v, want ErrInvalidTarget", err)
	}
}

func TestUnregisterKeepsDirectory(t *testing.T) {
	svc, db, root, stopper := newTestService(t)

	project, err := svc.Create(root, "kept")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Unregister(context.Background(), project.ID); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if got := projectCount(t, db); got != 0 {
		t.Errorf("project count = %d, want 0", got)
	}
	if info, err := os.Stat(filepath.Join(root, "kept")); err != nil || !info.IsDir() {
		t.Errorf("directory was touched by unregister: err=%v", err)
	}
	if calls := stopper.stopCalls(); len(calls) != 1 || calls[0] != project.ID {
		t.Errorf("stop calls = %v, want [%d]", calls, project.ID)
	}
	if err := svc.Unregister(context.Background(), project.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("Unregister unknown id error = %v, want ErrProjectNotFound", err)
	}
}

func TestUnregisterCascadesPlatformRows(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	project, err := svc.Create(root, "cascade")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	other, err := svc.Create(root, "bystander")
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}
	seedProjectRows(t, db, project.ID, "stopped-inst")
	seedProjectRows(t, db, other.ID, "other-inst")

	if err := svc.Unregister(context.Background(), project.ID); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	for _, model := range []any{&database.AgentInstance{}, &database.AgentSession{}, &database.AgentBackendConfig{}} {
		var n int64
		if err := db.Model(model).Where("project_id = ?", project.ID).Count(&n).Error; err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if n != 0 {
			t.Errorf("%T rows for unregistered project = %d, want 0", model, n)
		}
	}
	// Rows of other projects are untouched.
	var instances int64
	db.Model(&database.AgentInstance{}).Where("project_id = ?", other.ID).Count(&instances)
	if instances != 1 {
		t.Errorf("other project instances = %d, want 1", instances)
	}
	var sessions int64
	db.Model(&database.AgentSession{}).Where("project_id = ?", other.ID).Count(&sessions)
	if sessions != 1 {
		t.Errorf("other project sessions = %d, want 1", sessions)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want 1", got)
	}
}

// seedProjectRows inserts one stopped instance, one session on it and one
// backend config for the project, standing in for the state the platform
// accumulates while a project is in use.
func seedProjectRows(t *testing.T, db *gorm.DB, projectID uint, instanceID string) {
	t.Helper()

	rows := []any{
		&database.AgentInstance{ID: instanceID, ProjectID: projectID, Backend: "pi", Directory: "/x", Status: "stopped"},
		&database.AgentSession{ID: instanceID + "-s", ProjectID: projectID, Backend: "pi", InstanceID: instanceID, NativeSessionID: instanceID + "-native", Status: "idle"},
		&database.AgentBackendConfig{ProjectID: projectID, Backend: "pi", Enabled: true},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
}

func TestUnregisterAbortsWhenInstanceStopFails(t *testing.T) {
	svc, db, root, stopper := newTestService(t)

	project, err := svc.Create(root, "busy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	seedProjectRows(t, db, project.ID, "busy-inst")
	stopper.err = errors.New("adapter refused to stop")

	err = svc.Unregister(context.Background(), project.ID)
	if !errors.Is(err, ErrInstanceStop) {
		t.Fatalf("Unregister error = %v, want ErrInstanceStop", err)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want the record preserved", got)
	}
	var instances int64
	db.Model(&database.AgentInstance{}).Where("project_id = ?", project.ID).Count(&instances)
	if instances != 1 {
		t.Errorf("instance rows = %d, want the rows preserved", instances)
	}
	if info, err := os.Stat(filepath.Join(root, "busy")); err != nil || !info.IsDir() {
		t.Errorf("directory touched by refused unregister: err=%v", err)
	}
}

func TestUnregisterRefusesInstanceStartedDuringStop(t *testing.T) {
	svc, db, root, stopper := newTestService(t)

	project, err := svc.Create(root, "racy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The stopper reports success, but an instance is running when the
	// transaction runs: the platform must not drop the project under it.
	stopper.onStop = func(projectID uint) {
		if err := db.Create(&database.AgentInstance{ID: "late", ProjectID: projectID, Backend: "pi", Directory: "/x", Status: "running"}).Error; err != nil {
			t.Errorf("seed late instance: %v", err)
		}
	}

	if err := svc.Unregister(context.Background(), project.ID); !errors.Is(err, ErrInstanceStop) {
		t.Fatalf("Unregister error = %v, want ErrInstanceStop", err)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want the record preserved", got)
	}
}

func TestOperationsFailWhenDirectoryMissing(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	project, err := svc.Create(root, "gone")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if _, err := svc.Rename(project.ID, "x", "gone"); !errors.Is(err, ErrDirectoryMissing) {
		t.Errorf("Rename missing directory error = %v, want ErrDirectoryMissing", err)
	}
	if _, err := svc.Move(project.ID, filepath.Join(root, "x"), "gone"); !errors.Is(err, ErrDirectoryMissing) {
		t.Errorf("Move missing directory error = %v, want ErrDirectoryMissing", err)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want record preserved", got)
	}
}

func TestConcurrentCreateTargetCollision(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	const workers = 16
	results := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = svc.Create(root, "race")
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for i, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrTargetExists):
		default:
			t.Fatalf("worker %d error = %v, want nil or ErrTargetExists", i, err)
		}
	}
	if successes != 1 {
		t.Errorf("successful creates = %d, want exactly 1", successes)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want exactly 1 record", got)
	}
	if info, err := os.Stat(filepath.Join(root, "race")); err != nil || !info.IsDir() {
		t.Errorf("target directory missing after race: err=%v", err)
	}
}

func TestMutationsRejectedWhenStoredPathChanged(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	project, err := svc.Create(root, "orphan")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Replacing the stored directory with a symlink to another location makes
	// the stored path no longer canonical: the project must be refused, not
	// mutated.
	if err := os.Rename(project.Path, project.Path+"-real"); err != nil {
		t.Fatalf("rename aside: %v", err)
	}
	if err := os.Symlink(project.Path+"-real", project.Path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"rename", func() error { _, err := svc.Rename(project.ID, "renamed", "orphan"); return err }},
		{"move", func() error { _, err := svc.Move(project.ID, filepath.Join(root, "moved"), "orphan"); return err }},
	}
	for _, tc := range cases {
		if err := tc.call(); !errors.Is(err, ErrPathChanged) {
			t.Errorf("%s under changed path error = %v, want ErrPathChanged", tc.name, err)
		}
	}

	if _, err := os.Lstat(filepath.Join(root, "orphan")); err != nil {
		t.Errorf("stored path removed despite refusal: %v", err)
	}
	if got := projectCount(t, db); got != 1 {
		t.Errorf("project count = %d, want the record preserved", got)
	}
}

func TestRelocateRollbackFailureSurfacesError(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	project, err := svc.Create(root, "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	target := filepath.Join(root, "new")
	// A conflicting record at the target makes the record update fail, while
	// the target directory itself stays absent so the rename proceeds.
	if err := db.Create(&database.Project{Name: "ghost", Path: target}).Error; err != nil {
		t.Fatalf("seed conflicting record: %v", err)
	}

	calls := 0
	svc.renamePath = func(oldpath, newpath string) error {
		calls++
		if calls == 1 {
			return os.Rename(oldpath, newpath)
		}
		return errors.New("injected rollback failure")
	}

	_, err = svc.Rename(project.ID, "new", "old")
	if !errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("Rename error = %v, want ErrRollbackFailed", err)
	}
	if !strings.Contains(err.Error(), target) || !strings.Contains(err.Error(), project.Path) {
		t.Errorf("error %q should identify both affected paths", err)
	}
}

func TestConcurrentMoveTargetCollision(t *testing.T) {
	svc, db, root, _ := newTestService(t)

	first, err := svc.Create(root, "first")
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := svc.Create(root, "second")
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	mustMkdir(t, filepath.Join(root, "destparent"))
	dest := filepath.Join(root, "destparent", "target")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i, id := range []uint{first.ID, second.ID} {
		wg.Add(1)
		go func(i int, id uint) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.Move(id, dest, map[uint]string{first.ID: "first", second.ID: "second"}[id])
		}(i, id)
	}
	close(start)
	wg.Wait()

	successes := 0
	for i, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrTargetExists):
		default:
			t.Fatalf("worker %d error = %v", i, err)
		}
	}
	if successes != 1 {
		t.Errorf("successful moves = %d, want exactly 1", successes)
	}
	if got := projectCount(t, db); got != 2 {
		t.Errorf("project count = %d, want 2 records preserved", got)
	}
}
