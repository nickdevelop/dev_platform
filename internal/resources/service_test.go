package resources

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/agents"
	"devplatform/internal/database"
)

// openTestDB opens and migrates a throwaway platform database.
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

func seedProject(t *testing.T, db *gorm.DB) database.Project {
	t.Helper()
	row := database.Project{Name: "demo", Path: "/data/projects/demo"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	return row
}

func seedSession(t *testing.T, db *gorm.DB, projectID uint) database.AgentSession {
	t.Helper()
	row := database.AgentSession{
		ID:              "ses_1",
		ProjectID:       projectID,
		Backend:         "pi",
		InstanceID:      "inst_1",
		NativeSessionID: "native_1",
		Status:          "idle",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return row
}

func skillInput() CreateInput {
	return CreateInput{
		Kind:        ResourceSkill,
		Name:        "commit-style",
		Description: "how the team writes commits",
		Content:     "Use the imperative mood.",
		Enabled:     true,
		Backends:    []agents.BackendID{agents.BackendPi},
	}
}

func TestServiceCreateAndGet(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	created, err := svc.Create(skillInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("Create returned a zero id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("Create left timestamps zero: %+v", created)
	}
	if len(created.Backends) != 1 || created.Backends[0] != agents.BackendPi {
		t.Fatalf("Create backends = %v, want [pi]", created.Backends)
	}

	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "commit-style" || got.Content != "Use the imperative mood." || !got.Enabled {
		t.Fatalf("Get returned %+v, want the created definition", got)
	}
}

func TestServiceCreateRejectsInvalid(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	mutate := map[string]func(*CreateInput){
		"empty name":      func(in *CreateInput) { in.Name = "" },
		"traversal name":  func(in *CreateInput) { in.Name = "../evil" },
		"slash name":      func(in *CreateInput) { in.Name = "a/b" },
		"unknown kind":    func(in *CreateInput) { in.Kind = "tool" },
		"prompt no scope": func(in *CreateInput) { in.Kind = ResourcePrompt },
		"scope on skill":  func(in *CreateInput) { in.PromptScope = PromptScopeProject },
		"secret on skill": func(in *CreateInput) { in.SecretRef = "sec_1" },
		"unknown backend": func(in *CreateInput) { in.Backends = []agents.BackendID{"claude"} },
		"invalid scope":   func(in *CreateInput) { in.Kind = ResourcePrompt; in.PromptScope = "global" },
	}
	for name, fn := range mutate {
		in := skillInput()
		fn(&in)
		if _, err := svc.Create(in); err == nil {
			t.Errorf("%s: Create() = nil error, want failure", name)
		} else if !errors.Is(err, ErrInvalidResource) {
			t.Errorf("%s: Create() = %v, want ErrInvalidResource", name, err)
		}
	}

	if _, err := svc.Create(skillInput()); err != nil {
		t.Fatalf("sanity create failed: %v", err)
	}
	var count int64
	if err := db.Model(&database.Resource{}).Count(&count).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if count != 1 {
		t.Fatalf("invalid creates persisted %d rows, want 1", count)
	}
}

func TestServiceUpdate(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	created, err := svc.Create(skillInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	update := skillInput()
	update.Name = "commit-style-v2"
	update.Content = "Wrap the body at 72 columns."
	update.Enabled = false
	updated, err := svc.Update(created.ID, update)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "commit-style-v2" || updated.Content != "Wrap the body at 72 columns." || updated.Enabled {
		t.Fatalf("Update returned %+v", updated)
	}

	bad := skillInput()
	bad.Name = "a/b"
	if _, err := svc.Update(created.ID, bad); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("Update with a bad name = %v, want ErrInvalidResource", err)
	}
	if _, err := svc.Update(9999, skillInput()); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("Update unknown = %v, want ErrResourceNotFound", err)
	}
}

func TestServiceListFiltersByKind(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	if _, err := svc.Create(skillInput()); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	prompt := skillInput()
	prompt.Kind = ResourcePrompt
	prompt.Name = "house-rules"
	prompt.PromptScope = PromptScopeProject
	if _, err := svc.Create(prompt); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	all, err := svc.List("")
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List(all) = %d, want 2", len(all))
	}

	prompts, err := svc.List(ResourcePrompt)
	if err != nil {
		t.Fatalf("List(prompt): %v", err)
	}
	if len(prompts) != 1 || prompts[0].Kind != ResourcePrompt {
		t.Fatalf("List(prompt) = %+v", prompts)
	}

	if _, err := svc.List("tool"); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("List(unknown) = %v, want ErrInvalidResource", err)
	}
}

func TestServiceDelete(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	created, err := svc.Create(skillInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(created.ID); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("Get after delete = %v, want ErrResourceNotFound", err)
	}
	if err := svc.Delete(created.ID); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("Delete twice = %v, want ErrResourceNotFound", err)
	}
}

func TestServiceProjectAssociation(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	project := seedProject(t, db)

	skill, err := svc.Create(skillInput())
	if err != nil {
		t.Fatalf("Create skill: %v", err)
	}
	prompt := skillInput()
	prompt.Kind = ResourcePrompt
	prompt.Name = "house-rules"
	prompt.PromptScope = PromptScopeProject
	houseRules, err := svc.Create(prompt)
	if err != nil {
		t.Fatalf("Create prompt: %v", err)
	}

	if err := svc.AssociateProject(project.ID, skill.ID); err != nil {
		t.Fatalf("Associate skill: %v", err)
	}
	if err := svc.AssociateProject(project.ID, houseRules.ID); err != nil {
		t.Fatalf("Associate prompt: %v", err)
	}
	// Associating twice is idempotent, not an error and not a duplicate row.
	if err := svc.AssociateProject(project.ID, skill.ID); err != nil {
		t.Fatalf("Associate twice: %v", err)
	}

	associated, err := svc.ListProjectResources(project.ID)
	if err != nil {
		t.Fatalf("ListProjectResources: %v", err)
	}
	if len(associated) != 2 {
		t.Fatalf("ListProjectResources = %d, want 2", len(associated))
	}

	if err := svc.DisassociateProject(project.ID, skill.ID); err != nil {
		t.Fatalf("Disassociate: %v", err)
	}
	associated, err = svc.ListProjectResources(project.ID)
	if err != nil {
		t.Fatalf("ListProjectResources after remove: %v", err)
	}
	if len(associated) != 1 || associated[0].ID != houseRules.ID {
		t.Fatalf("after disassociate = %+v, want only the prompt", associated)
	}

	// Disassociating again is idempotent.
	if err := svc.DisassociateProject(project.ID, skill.ID); err != nil {
		t.Fatalf("Disassociate twice: %v", err)
	}

	if err := svc.AssociateProject(9999, skill.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("Associate unknown project = %v, want ErrProjectNotFound", err)
	}
	if err := svc.AssociateProject(project.ID, 9999); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("Associate unknown resource = %v, want ErrResourceNotFound", err)
	}

	// Deleting a resource removes its associations.
	if err := svc.Delete(skill.ID); err != nil {
		t.Fatalf("Delete skill: %v", err)
	}
	rows, err := svc.ListProjectResources(project.ID)
	if err != nil {
		t.Fatalf("ListProjectResources after delete: %v", err)
	}
	for _, row := range rows {
		if row.ID == skill.ID {
			t.Fatal("deleted resource still associated")
		}
	}
}

func TestServiceSessionPromptSelection(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	project := seedProject(t, db)
	session := seedSession(t, db, project.ID)

	projectPrompt := skillInput()
	projectPrompt.Kind = ResourcePrompt
	projectPrompt.Name = "house-rules"
	projectPrompt.PromptScope = PromptScopeProject
	houseRules, err := svc.Create(projectPrompt)
	if err != nil {
		t.Fatalf("Create project prompt: %v", err)
	}

	sessionPrompt := skillInput()
	sessionPrompt.Kind = ResourcePrompt
	sessionPrompt.Name = "review-checklist"
	sessionPrompt.PromptScope = PromptScopeSession
	review, err := svc.Create(sessionPrompt)
	if err != nil {
		t.Fatalf("Create session prompt: %v", err)
	}

	if err := svc.SelectSessionPrompt(project.ID, session.ID, review.ID); err != nil {
		t.Fatalf("SelectSessionPrompt: %v", err)
	}
	selected, err := svc.ListSessionPrompts(project.ID, session.ID)
	if err != nil {
		t.Fatalf("ListSessionPrompts: %v", err)
	}
	if len(selected) != 1 || selected[0].ID != review.ID {
		t.Fatalf("ListSessionPrompts = %+v, want the selected prompt", selected)
	}

	// A project-scoped prompt is also selectable in a session.
	if err := svc.SelectSessionPrompt(project.ID, session.ID, houseRules.ID); err != nil {
		t.Fatalf("Select project prompt: %v", err)
	}

	// A skill is not a prompt and cannot be selected.
	skill, err := svc.Create(skillInput())
	if err != nil {
		t.Fatalf("Create skill: %v", err)
	}
	if err := svc.SelectSessionPrompt(project.ID, session.ID, skill.ID); !errors.Is(err, ErrNotPrompt) {
		t.Fatalf("Select skill = %v, want ErrNotPrompt", err)
	}

	if err := svc.SelectSessionPrompt(9999, session.ID, review.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("Select in unknown project = %v, want ErrProjectNotFound", err)
	}
	if err := svc.SelectSessionPrompt(project.ID, "missing", review.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Select in unknown session = %v, want ErrSessionNotFound", err)
	}

	if err := svc.RemoveSessionPrompt(project.ID, session.ID, review.ID); err != nil {
		t.Fatalf("RemoveSessionPrompt: %v", err)
	}
	selected, err = svc.ListSessionPrompts(project.ID, session.ID)
	if err != nil {
		t.Fatalf("ListSessionPrompts after remove: %v", err)
	}
	for _, row := range selected {
		if row.ID == review.ID {
			t.Fatal("removed prompt still selected")
		}
	}
}

func TestServiceNeverReturnsPlaintextSecret(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	in := skillInput()
	in.Kind = ResourceMCP
	in.Name = "filesystem-server"
	in.SecretRef = "sec_opaque_1"
	created, err := svc.Create(in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"secretRef":"sec_opaque_1"`) {
		t.Fatalf("response missing opaque secretRef: %s", data)
	}
	for _, forbidden := range []string{`"secret":`, `"value":`, `"token":`, `"password":`} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, data)
		}
	}
}

// TestServiceTimestampsUseUTC guards the persisted timestamps against a
// non-UTC process clock, which would make ordering by time unstable across
// deployments.
func TestServiceTimestampsUseUTC(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	created, err := svc.Create(skillInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt location = %v, want UTC", created.CreatedAt.Location())
	}
}

// TestServiceRoutesRegisterWithoutConflict builds the same route tree the
// platform router uses, so a :id/:sessionId conflict panics here rather than
// at boot.
func TestServiceRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTestDB(t)
	engine := gin.New()
	NewService(db).RegisterRoutes(engine.Group("/api"))
}
