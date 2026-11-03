package platformtools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
)

func setupTestServices(t *testing.T) (*projects.Service, *prompts.Service) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Prompt{}, &database.Project{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	promptRepo := prompts.NewRepository(db)
	promptService := prompts.NewService(promptRepo)
	projectService := projects.NewService(db)

	return projectService, promptService
}

func TestExecutorDefinitions(t *testing.T) {
	projectSvc, promptSvc := setupTestServices(t)
	executor := NewExecutor(projectSvc, promptSvc)

	defs := executor.Definitions()
	if len(defs) != 7 {
		t.Fatalf("expected 7 tool definitions, got %d", len(defs))
	}

	toolNames := make(map[string]bool)
	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("expected tool type 'function', got %q", d.Type)
		}
		if d.Function.Name == "" || d.Function.Description == "" {
			t.Errorf("tool definition missing name or description: %+v", d.Function)
		}
		toolNames[d.Function.Name] = true
	}

	expectedTools := []string{
		"platform_list_projects",
		"platform_list_prompts",
		"platform_list_sessions",
		"platform_get_prompt",
		"platform_create_prompt",
		"platform_update_prompt",
		"platform_delete_prompt",
	}
	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("missing expected tool: %s", expected)
		}
	}
}

func TestExecutorProjectsAndPromptsLifecycle(t *testing.T) {
	ctx := context.Background()
	projectSvc, promptSvc := setupTestServices(t)
	executor := NewExecutor(projectSvc, promptSvc)

	// 1. 测试 platform_list_projects (初始化为空)
	projRes, err := executor.Execute(ctx, "platform_list_projects", "{}")
	if err != nil {
		t.Fatalf("list projects failed: %v", err)
	}
	if projRes != "[]" {
		t.Fatalf("expected empty projects json [], got %s", projRes)
	}

	// 在临时目录注册一个真实项目
	tempProjDir := filepath.Join(t.TempDir(), "demo-proj")
	if err := os.MkdirAll(tempProjDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.Register(tempProjDir); err != nil {
		t.Fatalf("register test project: %v", err)
	}

	// 再次查询项目
	projRes, err = executor.Execute(ctx, "platform_list_projects", `{"keyword":"demo"}`)
	if err != nil {
		t.Fatalf("list projects with keyword failed: %v", err)
	}
	if !strings.Contains(projRes, "demo-proj") {
		t.Fatalf("expected demo-proj in results, got %s", projRes)
	}

	// 2. 测试 platform_create_prompt
	createArgs := `{"name":"代码规范","description":"代码审查标准","content":"必须遵守严格分层和单测覆盖。"}`
	createRes, err := executor.Execute(ctx, "platform_create_prompt", createArgs)
	if err != nil {
		t.Fatalf("create prompt failed: %v", err)
	}
	var created struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(createRes), &created); err != nil {
		t.Fatalf("parse create prompt response: %v", err)
	}
	if created.ID == 0 || created.Name != "代码规范" {
		t.Fatalf("unexpected created prompt: %+v", created)
	}

	// 3. 测试 platform_list_prompts
	listRes, err := executor.Execute(ctx, "platform_list_prompts", `{"keyword":"代码"}`)
	if err != nil {
		t.Fatalf("list prompts failed: %v", err)
	}
	if !strings.Contains(listRes, "代码规范") {
		t.Fatalf("expected '代码规范' in list prompts response, got %s", listRes)
	}

	// 4. 测试 platform_get_prompt
	getArgs, _ := json.Marshal(map[string]any{"id": created.ID})
	getRes, err := executor.Execute(ctx, "platform_get_prompt", string(getArgs))
	if err != nil {
		t.Fatalf("get prompt failed: %v", err)
	}
	if !strings.Contains(getRes, "必须遵守严格分层和单测覆盖。") {
		t.Fatalf("expected prompt content in response, got %s", getRes)
	}

	// 5. 测试 platform_update_prompt
	newName := "Go与Vue代码规范"
	updateArgs, _ := json.Marshal(map[string]any{
		"id":   created.ID,
		"name": newName,
	})
	updateRes, err := executor.Execute(ctx, "platform_update_prompt", string(updateArgs))
	if err != nil {
		t.Fatalf("update prompt failed: %v", err)
	}
	if !strings.Contains(updateRes, newName) {
		t.Fatalf("expected updated name %q in response, got %s", newName, updateRes)
	}

	// 6. 测试 platform_delete_prompt
	deleteArgs, _ := json.Marshal(map[string]any{"id": created.ID})
	deleteRes, err := executor.Execute(ctx, "platform_delete_prompt", string(deleteArgs))
	if err != nil {
		t.Fatalf("delete prompt failed: %v", err)
	}
	if !strings.Contains(deleteRes, `"deleted":true`) {
		t.Fatalf("expected deleted:true, got %s", deleteRes)
	}

	// 验证删除后再次获取报错
	_, err = executor.Execute(ctx, "platform_get_prompt", string(getArgs))
	if err == nil {
		t.Fatalf("expected error getting deleted prompt, got nil")
	}

	// 7. 测试未知工具报错
	_, err = executor.Execute(ctx, "unknown_tool", "{}")
	if err == nil || !strings.Contains(err.Error(), "unknown platform tool") {
		t.Fatalf("expected unknown platform tool error, got %v", err)
	}
}
