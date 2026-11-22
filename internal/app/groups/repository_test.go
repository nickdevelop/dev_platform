package groups

import (
	"testing"
	"time"

	"devplatform/internal/app/database"
	"devplatform/internal/app/tasks"
)

func TestFlowRunsIsolationAndCascade(t *testing.T) {
	db := setupTestDB(t)
	groupRepo := NewRepository(db)
	taskRepo := tasks.NewRepository(db)

	now := time.Now().UTC()

	// 1. Create a regular task flow run (task_id = 1, group_id = 0)
	regularRun := database.FlowRun{
		ID:                10,
		TaskID:            1,
		GroupID:           0,
		ProjectID:         100,
		WorktreeID:        0,
		Status:            "completed",
		OrchestratorModel: "gpt-4",
		Prompt:            "regular task run",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(&regularRun).Error; err != nil {
		t.Fatalf("create regular flow run: %v", err)
	}

	// 2. Create a group task flow run with the same task_id = 1 (group_id = 5, worktree_id = 2)
	groupRun := database.FlowRun{
		ID:                20,
		TaskID:            1,
		GroupID:           5,
		ProjectID:         100,
		WorktreeID:        2,
		Status:            "running",
		OrchestratorModel: "claude-3",
		Prompt:            "group task run",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(&groupRun).Error; err != nil {
		t.Fatalf("create group flow run: %v", err)
	}

	// Also create a flow message for group run 20
	groupMsg := database.FlowMessage{
		ID:         200,
		RunID:      20,
		SenderType: "orchestrator",
		Content:    "hello from group orchestrator",
		CreatedAt:  now,
	}
	if err := db.Create(&groupMsg).Error; err != nil {
		t.Fatalf("create flow message: %v", err)
	}

	// 3. Verify groups.Repository.ListFlowRuns(1) returns ONLY the group run
	groupRuns, err := groupRepo.ListFlowRuns(1)
	if err != nil {
		t.Fatalf("groupRepo.ListFlowRuns: %v", err)
	}
	if len(groupRuns) != 1 {
		t.Fatalf("groupRepo.ListFlowRuns count = %d, want 1", len(groupRuns))
	}
	if groupRuns[0].ID != 20 || groupRuns[0].GroupID != 5 || groupRuns[0].WorktreeID != 2 {
		t.Fatalf("groupRepo.ListFlowRuns unexpected run: %+v", groupRuns[0])
	}

	// 4. Verify tasks.Repository.ListFlowRuns(1) returns ONLY the regular run
	regularRuns, err := taskRepo.ListFlowRuns(1)
	if err != nil {
		t.Fatalf("taskRepo.ListFlowRuns: %v", err)
	}
	if len(regularRuns) != 1 {
		t.Fatalf("taskRepo.ListFlowRuns count = %d, want 1", len(regularRuns))
	}
	if regularRuns[0].ID != 10 || regularRuns[0].GroupID != 0 {
		t.Fatalf("taskRepo.ListFlowRuns unexpected run: %+v", regularRuns[0])
	}

	// 5. Test DeleteGroupTask cascades flow runs and messages
	// First insert a GroupTask row with ID = 1 so DeleteGroupTask succeeds
	gt := database.GroupTask{
		ID:          1,
		GroupID:     5,
		Title:       "Test Group Task",
		ProjectID:   100,
		WorktreeID:  2,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(&gt).Error; err != nil {
		t.Fatalf("create group task: %v", err)
	}

	if err := groupRepo.DeleteGroupTask(1); err != nil {
		t.Fatalf("DeleteGroupTask: %v", err)
	}

	// Verify group run 20 and msg 200 are deleted, but regular run 10 remains
	var count int64
	db.Model(&database.FlowRun{}).Where("id = ?", 20).Count(&count)
	if count != 0 {
		t.Errorf("group run 20 still exists after DeleteGroupTask")
	}
	db.Model(&database.FlowMessage{}).Where("id = ?", 200).Count(&count)
	if count != 0 {
		t.Errorf("group msg 200 still exists after DeleteGroupTask")
	}
	db.Model(&database.FlowRun{}).Where("id = ?", 10).Count(&count)
	if count != 1 {
		t.Errorf("regular run 10 should not be deleted by DeleteGroupTask")
	}
}
