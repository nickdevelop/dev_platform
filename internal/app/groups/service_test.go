package groups

import (
	"context"
	"testing"

	"devplatform/internal/app/database"
	"devplatform/internal/app/tasks"
	"devplatform/internal/core/agents"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

type fakeFlowController struct {
	startedRuns []tasks.FlowRun
	userMsgs    []tasks.FlowMessage
	cancelled   []uint
}

func (f *fakeFlowController) StartGroupRun(ctx context.Context, groupID uint, taskID uint, triggerType string, initialPrompt string) (tasks.FlowRun, error) {
	run := tasks.FlowRun{
		ID:          uint(len(f.startedRuns) + 1),
		TaskID:      taskID,
		GroupID:     groupID,
		Status:      tasks.FlowRunRunning,
		Prompt:      initialPrompt,
		TriggerType: triggerType,
	}
	f.startedRuns = append(f.startedRuns, run)
	return run, nil
}

func (f *fakeFlowController) PostUserMessage(ctx context.Context, runID uint, content string) (tasks.FlowMessage, error) {
	msg := tasks.FlowMessage{
		ID:         uint(len(f.userMsgs) + 1),
		RunID:      runID,
		SenderType: tasks.SenderUser,
		Content:    content,
		Status:     tasks.FlowMsgComplete,
	}
	f.userMsgs = append(f.userMsgs, msg)
	return msg, nil
}

func (f *fakeFlowController) CancelRun(runID uint) error {
	f.cancelled = append(f.cancelled, runID)
	return nil
}

func (f *fakeFlowController) SubscribeEvents(runID uint) (<-chan tasks.FlowEvent, func()) {
	ch := make(chan tasks.FlowEvent)
	return ch, func() { close(ch) }
}

func TestGroupAndTaskLifecycle(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)
	fakeFlow := &fakeFlowController{}
	svc.SetFlowController(fakeFlow)
	ctx := context.Background()

	// 1. Seed two roles in agent_roles table
	role1 := database.AgentRole{
		Name:         "架构师",
		Description:  "负责总体架构与拆解",
		Backend:      "opencode",
		Model:        "claude-3-5-sonnet",
		SystemPrompt: "你是一个资深架构师",
		Icon:         "arch",
		Color:        "#3b82f6",
	}
	role2 := database.AgentRole{
		Name:         "前端工程师",
		Description:  "负责 UI 与交互实现",
		Backend:      "pi",
		Model:        "gpt-4o",
		SystemPrompt: "你是一个前端专家",
		Icon:         "frontend",
		Color:        "#10b981",
	}
	if err := db.Create(&role1).Error; err != nil {
		t.Fatalf("seed role1: %v", err)
	}
	if err := db.Create(&role2).Error; err != nil {
		t.Fatalf("seed role2: %v", err)
	}

	// 2. Create Group with initial member roles
	created, err := svc.CreateGroup(ctx, CreateGroupRequest{
		Name:               "研发核心组",
		Description:        "负责系统核心研发与重构",
		OrchestratorModel:  "gpt-4o",
		OrchestratorPrompt: "请作为中控大脑协调架构师与前端工程师完成工作",
		Icon:               "team",
		Color:              "#6366f1",
		RoleIDs:            []uint{role1.ID, role2.ID},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if created.ID == 0 || created.Name != "研发核心组" {
		t.Fatalf("unexpected group: %+v", created)
	}
	if len(created.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(created.Members))
	}
	if created.Members[0].Name != "架构师" || created.Members[0].Backend != agents.BackendID("opencode") {
		t.Fatalf("unexpected member 0: %+v", created.Members[0])
	}

	// 3. List Groups
	list, err := svc.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(list) != 1 || list[0].MemberCount != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}

	// 4. Update Group
	newName := "研发突击队"
	updated, err := svc.UpdateGroup(ctx, created.ID, UpdateGroupRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if updated.Name != "研发突击队" {
		t.Fatalf("expected updated name, got %s", updated.Name)
	}

	// 5. Create Group Task with Project and WorktreeID
	task, err := svc.CreateGroupTask(ctx, created.ID, CreateGroupTaskRequest{
		Title:       "构建工作树支持模块",
		Description: "在独立工作树分支下进行开发",
		ProjectID:   1,
		WorktreeID:  101, // specific git worktree
		Prompt:      "实现多Agent在工作树下的目录隔离协同",
	})
	if err != nil {
		t.Fatalf("CreateGroupTask: %v", err)
	}
	if task.ID == 0 || task.WorktreeID != 101 || task.GroupID != created.ID {
		t.Fatalf("unexpected task: %+v", task)
	}

	// 6. Test GroupProvider implementation for flow.Engine
	provider := NewProvider(repo)
	taskInfo, groupInfo, flowRoles, err := provider.GetGroupTask(task.ID)
	if err != nil {
		t.Fatalf("provider.GetGroupTask: %v", err)
	}
	if taskInfo.WorktreeID != 101 || taskInfo.ProjectID != 1 {
		t.Fatalf("unexpected taskInfo: %+v", taskInfo)
	}
	if groupInfo.OrchestratorModel != "gpt-4o" || groupInfo.OrchestratorPrompt == "" {
		t.Fatalf("unexpected groupInfo: %+v", groupInfo)
	}
	if len(flowRoles) != 2 || flowRoles[0].Name != "架构师" {
		t.Fatalf("unexpected flowRoles: %+v", flowRoles)
	}

	// 7. Trigger Group Task Run
	run, err := svc.TriggerGroupTask(ctx, created.ID, task.ID, "开始执行工作树任务")
	if err != nil {
		t.Fatalf("TriggerGroupTask: %v", err)
	}
	if run.GroupID != created.ID || run.TaskID != task.ID {
		t.Fatalf("unexpected run: %+v", run)
	}
	if len(fakeFlow.startedRuns) != 1 {
		t.Fatalf("expected flow to start 1 run")
	}

	// 8. Delete Task and Group
	if err := svc.DeleteGroupTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteGroupTask: %v", err)
	}
	if err := svc.DeleteGroup(ctx, created.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	afterDelete, _ := svc.ListGroups(ctx)
	if len(afterDelete) != 0 {
		t.Fatalf("expected 0 groups after delete")
	}
}

func TestCreateCollabSessionAndContext(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)
	fakeFlow := &fakeFlowController{}
	svc.SetFlowController(fakeFlow)
	ctx := context.Background()

	// 1. Seed role and create group
	role := database.AgentRole{
		Name:         "全栈工程师",
		Backend:      "builtin",
		Model:        "gpt-4o",
		SystemPrompt: "全栈开发",
	}
	_ = db.Create(&role).Error

	grp, err := svc.CreateGroup(ctx, CreateGroupRequest{
		Name:              "研发先锋队",
		OrchestratorModel: "gpt-4o",
		RoleIDs:           []uint{role.ID},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// 2. Create Collab Session
	res, err := svc.CreateCollabSession(ctx, grp.ID, CreateCollabSessionRequest{
		ProjectID:  10,
		WorktreeID: 2,
		Title:      "实现协同会话融入项目",
		Prompt:     "请专家团队协助完成需求改造",
	})
	if err != nil {
		t.Fatalf("CreateCollabSession: %v", err)
	}
	if res.Session.Backend != "group-flow" {
		t.Fatalf("unexpected backend: %s", res.Session.Backend)
	}
	if res.Session.WorktreeID != 2 || res.Session.ProjectID != 10 {
		t.Fatalf("unexpected session project/worktree: %+v", res.Session)
	}
	if len(fakeFlow.startedRuns) != 1 {
		t.Fatalf("expected 1 started run automatically, got %d", len(fakeFlow.startedRuns))
	}

	// 3. GetCollabSessionContext
	collabCtx, err := svc.GetCollabSessionContext(ctx, res.Session.ID)
	if err != nil {
		t.Fatalf("GetCollabSessionContext: %v", err)
	}
	if collabCtx.Group.ID != grp.ID {
		t.Fatalf("unexpected group in context: %+v", collabCtx.Group)
	}
	if collabCtx.Task.ID != res.Task.ID {
		t.Fatalf("unexpected task in context: %+v", collabCtx.Task)
	}

	// 4. Verify tasks.Repository.GetTaskSessionIDs includes flow_messages session_id
	tasksRepo := tasks.NewRepository(db)
	// seed a flow message with session_id
	flowRun := database.FlowRun{
		TaskID:    res.Task.ID,
		GroupID:   grp.ID,
		ProjectID: 10,
		Status:    "running",
	}
	_ = db.Create(&flowRun).Error
	childSessionID := "child-agent-session-999"
	flowMsg := database.FlowMessage{
		RunID:      flowRun.ID,
		SenderType: "agent",
		SenderName: "全栈工程师",
		Content:    "已启动子会话",
		SessionID:  childSessionID,
	}
	_ = db.Create(&flowMsg).Error

	taskSessionIDs, err := tasksRepo.GetTaskSessionIDs(10)
	if err != nil {
		t.Fatalf("GetTaskSessionIDs: %v", err)
	}
	found := false
	for _, id := range taskSessionIDs {
		if id == childSessionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected childSessionID %s to be collected in taskSessionIDs, got %+v", childSessionID, taskSessionIDs)
	}
}
