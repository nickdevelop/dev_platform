# 平台设置中心 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付平台设置中心（外观、模型、Agent），并移除授权工作区根目录约束，使项目可在任意目录创建。

**Architecture:** 后端新增 `internal/paths`（无授权路径规范化）、`internal/settings`（外观 + Agent 平台级配置，存 `settings` 表）、`internal/modelconfig`（模型服务/模型 CRUD，凭据存 `secrets` 库）、`internal/modelprobe`（OpenAI 兼容的拉取与测试）；`instances`/`sessions` 增加平台启用门禁；`pi`/`opencode` 适配器运行时解析可执行路径。前端新增 `/settings` 区域与主题令牌系统，逐组件从硬编码样式迁移到 CSS 变量。

**Tech Stack:** Go 1.24 + Gin + GORM/SQLite；Vue 3 + TypeScript + Pinia + vue-router；Vitest + Playwright；OpenAPI 契约。

**Spec:** `docs/superpowers/specs/2026-10-11-platform-settings-design.md`

## Global Constraints

- 数据库只允许在 `internal/database/migrations.go` 追加版本化步骤，禁止修改 ≤12 的既有步骤，禁止 AutoMigrate。
- 新增 API 路由必须挂在 `http.Dependencies` 注入并在 `auth.RequireSession` 保护组下注册；CSRF 由现全局中间件处理。
- API 响应为 JSON 稳定 DTO，绝不直接序列化 GORM 模型；凭据/明文密钥永不出现在响应、日志、诊断中（复用 `agents.RedactSecrets`）。
- 模型与外观/Agent 的错误信息必须脱敏；Key 仅存 `internal/secrets`（`0600`）。
- 每个后端任务用 `go test ./...` 验证；每个前端任务用 `npm test -- --run`（在 `web/`）验证；涉及构建时用 `make web-build`。
- 所有新增用户可见文案为中文。
- 每完成一个任务提交一次，提交信息用 `feat:` / `refactor:` / `test:` 前缀。

## Review Focus

- **旧项目记录的路径**：移除授权白名单后，历史登记的 `projects.path` 仍必须能解析与操作；若组件被替换为符号链接，必须拒绝而非误操作（`ErrPathChanged`）。
- **停用 Agent 的边界**：停用后不得能新建任务（含复用已运行实例的会话创建），但已运行实例与已有会话必须继续可用。
- **API Key 语义**：编辑服务时提交空串必须清除旧 Key、提交新值必须替换且旧值被清理，绝不能因“未填”而静默删除用户期望保留的 Key——UI 必须显式提示。
- **模型探测输入**：`baseUrl` 为非法 scheme、超时、超长响应或非 JSON 时，接口必须给出可读错误且不崩溃、不记录密钥。
- **外观持久化失败**：服务端保存失败时，前端必须保留已经生效的旧配置并提示，不得出现“界面已切换但服务器未保存”的静默不一致。

---

# Part A — 移除工作区授权

### Task A1: `internal/paths` 规范化路径包

**Files:**
- Create: `internal/paths/paths.go`
- Create: `internal/paths/paths_test.go`（从 `internal/workspaces/path_policy_test.go` 迁移用例）

**Interfaces:**
- Produces:
  - `var ErrInvalidPath = errors.New("paths: invalid path")`
  - `func Resolve(input string) (string, error)`

- [ ] **Step 1: 写失败测试**。把 `internal/workspaces/path_policy_test.go` 中与规范化相关的用例（Windows 盘符翻译、相对路径拒绝、NUL、UNC/反斜杠拒绝、`..` 词法归一、已存在路径符号链接解析、不存在尾部保留、路径穿透文件拒绝）迁移为 `paths.Resolve` 的表驱动测试，删除所有“根目录包含/拒绝”的用例。至少包含：

```go
func TestResolveRejectsRelative(t *testing.T) {
    if _, err := Resolve("work/proj"); !errors.Is(err, ErrInvalidPath) {
        t.Fatalf("want ErrInvalidPath, got %v", err)
    }
}

func TestResolveTranslatesWindowsDrive(t *testing.T) {
    got, err := Resolve(`C:\work\proj`) // 目标不存在时返回 /mnt/c/work/proj
    if err != nil || got != "/mnt/c/work/proj" { t.Fatalf("got %q err %v", got, err) }
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./internal/paths/ -run TestResolve -v`，预期编译失败（包不存在）。
- [ ] **Step 3: 实现**。将 `internal/workspaces/path_policy.go` 的 `normalizeInput`、`translateWindowsPath`、`canonicalize` 原样迁入，去掉 `ResolvedPath`、`containingRoot`、`pathInsideRoot`、`ResolveAuthorizedPath`、`ErrOutsideRoots`；`Resolve` = `normalizeInput` + `canonicalize`。包注释说明不再做根目录授权，仅规范化。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/paths/ -v`。
- [ ] **Step 5: 提交**：`git add internal/paths && git commit -m "refactor: extract unauthorized path resolution into internal/paths"`。

### Task A2: 项目与实例切到 `paths`，删除 `workspaces` 包

**Files:**
- Modify: `internal/projects/service.go`、`internal/projects/handlers.go`
- Modify: `internal/agents/instances/handlers.go`
- Modify: `internal/http/router.go`、`internal/http/router_test.go`
- Modify: `cmd/platform/main.go`
- Delete: `internal/workspaces/`（整目录）

**Interfaces:**
- Consumes: `paths.Resolve`、`paths.ErrInvalidPath`
- Produces: `projects.Service.resolve` 返回 `(string, error)`；`workspaces` 包不再存在。

- [ ] **Step 1: 写失败测试**。在 `internal/projects/service_test.go` 增加：登记/创建路径位于任意目录（如 `t.TempDir()`）成功；相对路径失败为 `ErrInvalidPath` 语义（handler 返回 `invalid_target` 或 `not_a_directory`）。把原依赖 `WithRoots`/授权根的测试删除或改为直接登记。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/projects/ -run TestRegisterOutside -v`。
- [ ] **Step 3: 实现**：
  - `projects/service.go`：删除 `workspaces` import，改 `paths`；`resolve(input string) (string, error)` 直接 `paths.Resolve`；`resolveExistingDirectory` 返回 `(string, error)`；删除 `authorizedRoots`；`authorizeStoredPath` 改为比较 `paths.Resolve` 结果与存储值，不一致返回 `ErrPathChanged`；所有调用点 `resolved.Path` → 新返回的字符串。
  - `projects/handlers.go`：错误映射删除 `workspaces.ErrOutsideRoots`，`workspaces.ErrInvalidPath` → `paths.ErrInvalidPath`，`path_not_authorized` 改为 `invalid_target`（`ErrPathChanged` 仍映射 `invalid_target`）。
  - `instances/handlers.go`：删除 `workspaces` import，`workspaces.ErrInvalidPath`/`ErrOutsideRoots` → `paths.ErrInvalidPath`，删除 `project_not_authorized` 分支。
  - `http/router.go`：删除 `Workspaces` 字段、`workspaces` import 与注册块。**不要**在本任务新增 `Settings`/`Models` 字段（对应包尚未创建）；它们分别由 Task B1 / Task C4 添加。
  - `cmd/platform/main.go`：删除 `workspaces.NewService(db)` 与 `Workspaces:` 字段。
  - 删除 `internal/workspaces/` 整目录。
- [ ] **Step 4: 运行确认通过**：`go build ./... && go test ./internal/projects/ ./internal/agents/... ./internal/http/ -v`。
- [ ] **Step 5: 提交**：`git add -A && git commit -m "refactor: drop workspace-root authorization from projects and instances"`。

### Task A3: 迁移 v13 删除 `workspace_roots`，移除模型

**Files:**
- Modify: `internal/database/models.go`、`internal/database/migrations.go`、`internal/database/database_test.go`

**Interfaces:**
- Produces: 迁移序列最高版本 13；`database.WorkspaceRoot` 不再存在。

- [ ] **Step 1: 写失败测试**。在 `database_test.go` 增加：`Migrate` 后 `db.Migrator().HasTable("workspace_roots")` 为 false；重复 `Migrate` 幂等。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/database/ -run TestMigrateDropsWorkspaceRoots -v`。
- [ ] **Step 3: 实现**：在 `migrationSteps` 追加 `{version: 13, name: "drop_workspace_roots", run: func(tx){ if tx.Migrator().HasTable(&WorkspaceRoot{}) { return tx.Migrator().DropTable(&WorkspaceRoot{}) } ; return nil }}`；从 `models.go` 删除 `WorkspaceRoot`。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/database/ -v`。
- [ ] **Step 5: 提交**：`git add internal/database && git commit -m "feat: migrate away from workspace_roots table"`。

---

# Part B — 设置中心外壳与外观

### Task B1: `internal/settings` 外观配置

**Files:**
- Create: `internal/settings/service.go`、`internal/settings/appearance.go`、`internal/settings/handlers.go`
- Create: `internal/settings/service_test.go`、`internal/settings/handlers_test.go`
- Modify: `internal/http/router.go`（`Dependencies` 增加 `Settings *settings.Service` 并在 `protected` 组注册）
- Modify: `cmd/platform/main.go`（构造 `settings.NewService(db)` 并注入）

**Interfaces:**
- Produces:
  - `type Appearance struct{ Theme, Font, CustomFont string; FontSize int; Density string }`
  - `func DefaultAppearance() Appearance` → `{"light","system","",14,"standard"}`
  - `func NewService(db *gorm.DB) *Service`
  - `func (s *Service) Appearance() (Appearance, error)`
  - `func (s *Service) SetAppearance(a Appearance) (Appearance, error)`（校验失败返回 `ErrInvalidAppearance`）
  - `var ErrInvalidAppearance`
  - `func (s *Service) RegisterRoutes(r gin.IRouter)`（注册 `GET/PUT /settings/appearance`）
- Consumes: `database.Setting`（key `"appearance.config"`，value 为 JSON）。

- [ ] **Step 1: 写失败测试**：默认值、往返保存、非法 theme/font/fontSize/density/customFont 被拒；handler 返回 `400 invalid_appearance`。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/settings/ -run TestAppearance -v`。
- [ ] **Step 3: 实现**：`Appearance` 校验函数 `Validate()`；`Service` 读写 `settings` 表（`db.Where("key = ?", ...).First`，不存在返回默认值）；`RegisterRoutes` 用 `auth.RequireSession` 由 router 外层保证，本处只注册路由；错误响应体 `{"error":"invalid_appearance"}`。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/settings/ -v`。
- [ ] **Step 5: 提交**：`git add internal/settings && git commit -m "feat: add appearance settings backend"`。

### Task B2: 前端主题令牌、预设与套用

**Files:**
- Modify: `web/src/style.css`、`web/src/main.ts`
- Create: `web/src/theme/presets.ts`、`web/src/theme/apply.ts`
- Create: `web/src/tests/theme.spec.ts`

**Interfaces:**
- Produces:
  - `theme/themes`: `{ key, label, colors: Record<string,string> }[]`（light/dark/warm/cool，色值按设计 §3.1）
  - `theme/fonts`: `{ key, label, stack }[]`（system/sans/serif/mono）
  - `theme/densities`: `{ key, label, spaceUnit, controlHeight, radius }[]`
  - `func applyAppearance(a: Appearance): void`
  - `const CACHE_KEY = 'platform.appearance'`

- [ ] **Step 1: 写失败测试** `theme.spec.ts`：`applyAppearance` 后 `document.documentElement.dataset.theme === 'dark'`、`dataset.density`、`style.getPropertyValue('--font-size-base') === '16px'`；`font:'custom'` 时生成 `<link id="platform-google-font">`，切回预设时移除；Google Fonts family 中空格替换为 `+`。
- [ ] **Step 2: 运行确认失败**：`cd web && npm test -- --run src/tests/theme.spec.ts`。
- [ ] **Step 3: 实现**：
  - `style.css` 定义 `:root` 全套变量默认值、`[data-theme=...]` 色板、`[data-density=...]` 尺寸、`html{font-family:var(--font-family);font-size:var(--font-size-base)}`、`body{background:var(--color-bg);color:var(--color-text)}`。
  - `theme/presets.ts` 导出常量与 `resolveFontFamily`、`googleFontURL`。
  - `theme/apply.ts` 设置 data 属性与内联变量、注入/移除字体 link。
  - `main.ts` 挂载前读取 `localStorage[CACHE_KEY]` 并 `applyAppearance`（防闪烁）。
- [ ] **Step 4: 运行确认通过**：`cd web && npm test -- --run src/tests/theme.spec.ts && npm run typecheck`。
- [ ] **Step 5: 提交**：`git add web/src && git commit -m "feat: add appearance theme tokens and application"`。

### Task B3: 设置中心外壳与导航

**Files:**
- Create: `web/src/views/SettingsView.vue`
- Modify: `web/src/router.ts`、`web/src/components/CategoryRail.vue`、`web/src/components/AppShell.vue`、`web/src/tests/app-shell.spec.ts`
- Delete: `web/src/components/WorkspaceRootsDialog.vue`

**Interfaces:**
- Consumes: `applyAppearance`（后续 Task B4 使用）。
- Produces: 路由 `settings`、`settings-appearance`、`settings-models`、`settings-agents`；`SettingsView` 用 `<RouterLink>` 二级导航 + `<RouterView>`。

- [ ] **Step 1: 写失败测试**：`app-shell.spec.ts` 中删除“授权工作区弹窗”用例，新增：点击“设置”导航到 `/settings/appearance`；在设置路由时项目侧栏不渲染（`aside.sidebar` 不存在）；`SettingsView` 渲染三个二级入口。
- [ ] **Step 2: 运行确认失败**：`cd web && npm test -- --run src/tests/app-shell.spec.ts`。
- [ ] **Step 3: 实现**：
  - `router.ts`：在 AppShell children 增加 `{ path:'settings', name:'settings', component:SettingsView, meta:{settings:true}, children:[ {path:'',redirect:{name:'settings-appearance'}}, {path:'appearance',name:'settings-appearance',component:AppearanceSettings}, {path:'models',name:'settings-models',component:ModelSettings}, {path:'agents',name:'settings-agents',component:AgentSettings} ] }`。
  - 本任务先创建三个最小占位组件 `web/src/components/settings/{AppearanceSettings,ModelSettings,AgentSettings}.vue`（后续 B4/C6/D6 填充），保证可编译。
  - `CategoryRail.vue`：`设置` 改为 `<RouterLink :to="{name:'settings-appearance'}">`，移除 `open-roots` emit。
  - `AppShell.vue`：移除 `showRoots`、`WorkspaceRootsDialog`；当 `route.meta.settings === true` 时不渲染 `<aside class="sidebar">`，主区占满。
  - 删除 `WorkspaceRootsDialog.vue`。
- [ ] **Step 4: 运行确认通过**：`cd web && npm test -- --run src/tests/app-shell.spec.ts && npm run typecheck`。
- [ ] **Step 5: 提交**：`git add -A web/src && git commit -m "feat: add settings center shell and navigation"`。

### Task B4: 外观设置页、Store 与 API

**Files:**
- Create: `web/src/api/settings.ts`、`web/src/stores/settings.ts`
- Modify: `web/src/components/settings/AppearanceSettings.vue`
- Modify: `web/src/components/AppShell.vue`（`onMounted` 在认证后调用 `settings.loadAppearance()`，用服务端配置覆盖本地缓存）
- Create: `web/src/tests/appearance-settings.spec.ts`

**Interfaces:**
- Consumes: `GET/PUT /api/settings/appearance`（Task B1）、`applyAppearance`（Task B2）。
- Produces: `api/settings.ts`：`interface Appearance`、`getAppearance()`、`putAppearance(a, csrf)`；`stores/settings.ts`：state `appearance`，actions `loadAppearance()`、`saveAppearance(patch)`（保存成功后 `applyAppearance` 并更新 `localStorage`）。

- [ ] **Step 1: 写失败测试**：主题选择项渲染 4 个；切换主题调用 `PUT` 并 `applyAppearance`；保存失败时保留旧值并显示错误。
- [ ] **Step 2: 运行确认失败**：`cd web && npm test -- --run src/tests/appearance-settings.spec.ts`。
- [ ] **Step 3: 实现**：`AppearanceSettings.vue` 提供主题卡片、字体下拉、字号滑块/数字（12–20）、密度三档；加载与保存经 store；保存用 `useAuthStore().csrfToken`（遵循现有调用方式）。
- [ ] **Step 4: 运行确认通过**：`cd web && npm test -- --run src/tests/appearance-settings.spec.ts && npm run typecheck`。
- [ ] **Step 5: 提交**：`git add web/src && git commit -m "feat: add appearance settings UI"`。

### Task B5: 令牌迁移 — 外壳与认证

**Files:** Modify `web/src/App.vue`、`web/src/components/AppShell.vue`、`web/src/components/CategoryRail.vue`、`web/src/views/EmptyWorkspace.vue`、`web/src/views/LoginView.vue`

- [ ] **Step 1:** 把这些文件中所有硬编码颜色/字号/圆角/内边距替换为令牌（`var(--color-*)`、`rem`、`var(--space-unit)`、`var(--control-height)`、`var(--radius)`）。
- [ ] **Step 2:** `cd web && npm test -- --run && npm run build`；预期全部通过。
- [ ] **Step 3:** 人工核对四套主题下无残留硬编码色（`grep -nE '#[0-9a-fA-F]{3,6}' web/src/components/AppShell.vue web/src/components/CategoryRail.vue`）。
- [ ] **Step 4:** 提交 `git commit -am "refactor: tokenize shell and auth styles"`。

### Task B6: 令牌迁移 — 项目管理

**Files:** Modify `web/src/components/ProjectTree.vue`、`ProjectManageDialog.vue`、`ProjectDialog.vue`、`ProjectSettingsPanel.vue`

- [ ] **Step 1:** 按 Task B5 的方法替换令牌；移除 `ProjectSettingsPanel.vue` 中 `BackendDiagnostics` 面板相关模板/样式/import（检测归入 Agent 管理，见 Task D6）。
- [ ] **Step 2:** `cd web && npm test -- --run src/tests/project-tree.spec.ts src/tests/project-manage-dialog.spec.ts src/tests/project-settings-panel.spec.ts`。
- [ ] **Step 3:** 提交 `git commit -am "refactor: tokenize project management styles"`。

### Task B7: 令牌迁移 — 任务与会话

**Files:** Modify `web/src/components/NewTaskPanel.vue`、`TaskChatPanel.vue`、`SessionList.vue`、`MessageView.vue`、`ModelSelect.vue`、`BackendTag.vue`、`CapabilityControls.vue`

- [ ] **Step 1:** 替换令牌。
- [ ] **Step 2:** `cd web && npm test -- --run src/tests/task-chat-panel.spec.ts src/tests/new-task-panel.spec.ts src/tests/message-view.spec.ts src/tests/model-select.spec.ts`。
- [ ] **Step 3:** 提交 `git commit -am "refactor: tokenize task and session styles"`。

### Task B8: 令牌迁移 — 资源管理

**Files:** Modify `web/src/components/ResourcesSection.vue`、`web/src/components/resources/SecretField.vue`、`ResourceEditor.vue`、`SyncPreview.vue`、`ConflictDialog.vue`

- [ ] **Step 1:** 替换令牌。
- [ ] **Step 2:** `cd web && npm test -- --run src/tests/resources.spec.ts`。
- [ ] **Step 3:** 提交 `git commit -am "refactor: tokenize resource management styles"`。

---

# Part C — 模型管理

### Task C1: 模型数据表与迁移

**Files:**
- Modify: `internal/database/models.go`、`internal/database/migrations.go`、`internal/database/database_test.go`

**Interfaces:**
- Produces: `database.ModelService`、`database.ModelDef`（字段见设计 §4）；迁移 v14 create_model_services、v15 create_model_defs、v16 create_model_defs_service_index。

- [ ] **Step 1: 写失败测试**：迁移后两张表存在且 `(service_id, model_id)` 唯一索引存在。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/database/ -run TestMigrateModelTables -v`。
- [ ] **Step 3: 实现**：按设计 §4 增加模型与三个迁移步骤（索引仿 v12 用 `HasIndex/CreateIndex` 显式创建）。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/database/ -v`。
- [ ] **Step 5: 提交**：`git add internal/database && git commit -m "feat: add model service and model tables"`。

### Task C2: `internal/modelconfig` 服务与 CRUD

**Files:**
- Create: `internal/modelconfig/service.go`、`internal/modelconfig/models.go`
- Create: `internal/modelconfig/service_test.go`

**Interfaces:**
- Produces（见设计 §6.3）：
  - `type SecretStore interface{ Set(ctx, id, value) error; Exists(ctx, id) (bool, error); Delete(ctx, id) error; Get(ctx, id) (string, error) }`
  - `type ServiceDTO struct{ ID uint; Name, BaseURL string; HasKey bool; SecretRef string; Enabled bool; Models []ModelDTO }`
  - `type ModelDTO struct{ ID, ServiceID uint; ModelID, Name string; SupportsText, SupportsImage, SupportsTools bool; ContextTokens, OutputTokens int; Enabled, Usable bool }`
  - `func NewService(db *gorm.DB, store SecretStore) *Service`
  - `List/Create/Update/Delete/AddModel/UpdateModel/DeleteModel`（签名见设计 §6.3）
  - sentinels：`ErrServiceNotFound`、`ErrModelNotFound`、`ErrInvalidService`、`ErrInvalidModel`、`ErrModelConflict`
- Consumes: `database.ModelService/ModelDef`、`secrets.Store`。

- [ ] **Step 1: 写失败测试**：创建含/不含 Key 的服务（`hasKey` 正确、库里只有 `secret_ref`）；更新时空串删 Key、新值换 Key 且旧 ref 被 `Delete`；删除服务级联删模型并删 Key；同服务重复 `modelId` 冲突；`Usable = service.enabled && model.enabled`。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/modelconfig/ -run TestService -v`。
- [ ] **Step 3: 实现**：事务内 CRUD；Key 处理走 `SecretStore`（`secrets.NewSecretID` 由 handler 传入或服务内部生成——采用服务内部调用 `secrets.NewSecretID`）；baseUrl 校验 `http/https` 绝对地址，去尾 `/`。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/modelconfig/ -v`。
- [ ] **Step 5: 提交**：`git add internal/modelconfig && git commit -m "feat: add model service CRUD"`。

### Task C3: `internal/modelprobe` 拉取与测试

**Files:**
- Create: `internal/modelprobe/probe.go`、`internal/modelprobe/probe_test.go`

**Interfaces:**
- Produces:
  - `type RemoteModel struct{ ID, Name string }`
  - `type TestResult struct{ ModelID string; LatencyMs int64 }`
  - `func FetchModels(ctx context.Context, baseURL, apiKey string) ([]RemoteModel, error)`
  - `func TestModel(ctx context.Context, baseURL, apiKey, modelID string) (TestResult, error)`
  - `var ErrUpstreamUnreachable`、`var ErrUpstreamError`、`var ErrInvalidBaseURL`
- Consumes: `agents.RedactSecrets`（脱敏）。

- [ ] **Step 1: 写失败测试**（用 `httptest.Server`）：`GET /models` 返回 `{"data":[{"id":"m1"}]}` → 解析出 m1；`/chat/completions` 2xx → 成功；4xx/5xx → `ErrUpstreamError` 且错误文本不含 API Key；非法 `baseURL`（`ftp://x`、相对路径）→ `ErrInvalidBaseURL`；超时由 ctx 控制。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/modelprobe/ -run TestFetch -v`。
- [ ] **Step 3: 实现**：`http.Client{Timeout:15s}`；`io.LimitReader` 1 MiB；仅 `http`/`https`；拼接 `base + "/models"`、`base + "/chat/completions"`；错误经 `agents.RedactSecrets`。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/modelprobe/ -v`。
- [ ] **Step 5: 提交**：`git add internal/modelprobe && git commit -m "feat: add OpenAI-compatible model probe"`。

### Task C4: 模型 API 路由与 handler，接入 router/main

**Files:**
- Create: `internal/modelconfig/handlers.go`、`internal/modelconfig/handlers_test.go`
- Modify: `internal/http/router.go`、`cmd/platform/main.go`、`docs/api/openapi.yaml`

**Interfaces:**
- Consumes: `modelconfig.Service`、`modelprobe`、`secrets.Store`（解析 `secretRef`）。
- Produces: 设计 §5.2 全部路由的 `RegisterRoutes(r gin.IRouter)`；错误码见设计 §5.2。

- [ ] **Step 1: 写失败测试**：CRUD 往返、`fetch-models` 用 `apiKey` 与 `secretRef` 两种方式、`models/test` 成功与上游错误映射、非法请求 400。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/modelconfig/ -run TestHandler -v`。
- [ ] **Step 3: 实现**：
  - handler 解析 `apiKey`/`secretRef`：`apiKey` 非空优先，否则 `secretRef` 经 `secrets.Store.Get`（不存在 → 400 `invalid_request`）。
  - `http/router.go`：`Dependencies.Models` 非空时 `deps.Models.RegisterRoutes(protected)`。
  - `cmd/platform/main.go`：`modelConfig := modelconfig.NewService(db, secretStore)`；注入 `Models: modelConfig`。
  - `docs/api/openapi.yaml`：新增上述路径与 schema。
- [ ] **Step 4: 运行确认通过**：`go test ./... && go vet ./...`。
- [ ] **Step 5: 提交**：`git add -A && git commit -m "feat: expose model management API"`。

### Task C5: 模型前端 API 与 Store

**Files:**
- Create: `web/src/api/models.ts`、`web/src/stores/models.ts`
- Create: `web/src/tests/models-store.spec.ts`

**Interfaces:**
- Produces: `api/models.ts` 的 DTO 与 `listServices/upsertService/deleteService/addModel/updateModel/deleteModel/fetchModels/testModel`；`stores/models.ts` 状态与 action。
- Consumes: 设计 §5.2 路由。

- [ ] **Step 1: 写失败测试**：加载/创建/更新/删除调用正确路径；`fetchModels` 返回候选；`testModel` 成功/失败状态。
- [ ] **Step 2: 运行确认失败**：`cd web && npm test -- --run src/tests/models-store.spec.ts`。
- [ ] **Step 3: 实现** DTO 与 store（CSRF 取自 `useAuthStore`）。
- [ ] **Step 4: 运行确认通过**：`cd web && npm test -- --run src/tests/models-store.spec.ts && npm run typecheck`。
- [ ] **Step 5: 提交**：`git add web/src && git commit -m "feat: add model management client and store"`。

### Task C6: 模型设置页

**Files:**
- Modify: `web/src/components/settings/ModelSettings.vue`
- Create: `web/src/tests/model-settings.spec.ts`

**Interfaces:**
- Consumes: `stores/models.ts`、`api/client.ts` 错误码文案（Task D7 统一补）。

- [ ] **Step 1: 写失败测试**：服务列表渲染；新建服务表单；“获取模型”弹出候选并可勾选添加；编辑模型可改 `modelId` 与能力字段；测试按钮显示成功/失败。
- [ ] **Step 2: 运行确认失败**：`cd web && npm test -- --run src/tests/model-settings.spec.ts`。
- [ ] **Step 3: 实现**：服务卡片 + 内嵌模型表；“API Key（留空将清除已保存的 Key）”提示；停用/删除按钮带确认；`usable` 显示不可用态。
- [ ] **Step 4: 运行确认通过**：`cd web && npm test -- --run src/tests/model-settings.spec.ts && npm run build`。
- [ ] **Step 5: 提交**：`git add web/src && git commit -m "feat: add model settings UI"`。

---

# Part D — Agent 管理

### Task D1: `settings` Agent 平台级配置

**Files:**
- Create: `internal/settings/agents.go`、`internal/settings/agents_test.go`
- Modify: `internal/settings/service.go`（如需共享读写助手）

**Interfaces:**
- Produces（见设计 §6.2）：
  - `type AgentSetting struct{ Enabled bool; ManualPath, Version, CheckedAt string }`
  - `Agents() (map[agents.BackendID]AgentSetting, error)`
  - `SetAgentEnabled(backend, enabled) (AgentSetting, error)`
  - `SetAgentPath(backend, manualPath) (AgentSetting, error)`
  - `RecordDetection(backend, version) (AgentSetting, error)`
  - `Enabled(backend) bool`、`Executable(backend) string`、`ResolvePath(backend) (path, source string)`
- Consumes: `database.Setting`（key `"agents.config"`）、`agents.BackendID`。

- [ ] **Step 1: 写失败测试**：默认 `Enabled=true`、`ManualPath=""`；`SetAgentEnabled(false)` 往返；`ResolvePath` 无手动路径时用 `exec.LookPath`（测试用临时可执行文件注入 PATH）；有手动路径时返回 `manual`；`Executable` 返回手动路径或空。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/settings/ -run TestAgent -v`。
- [ ] **Step 3: 实现**：JSON 读写带互斥锁；`ResolvePath` 用 `exec.LookPath(backendBinary(backend))`（pi→`pi`、opencode→`opencode`）；`Enabled`/`Executable` 每次查表（规模极小）。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/settings/ -v`。
- [ ] **Step 5: 提交**：`git add internal/settings && git commit -m "feat: add platform-level agent settings"`。

### Task D2: 适配器运行时解析可执行路径

**Files:**
- Modify: `internal/agents/pi/adapter.go`、`internal/agents/pi/adapter_test.go`
- Modify: `internal/agents/opencode/adapter.go`、`internal/agents/opencode/adapter_test.go`

**Interfaces:**
- Produces: `Options.ResolveExecutable func() string`；内部方法 `execPath() string`；`New` 保留 `Executable` 兜底。

- [ ] **Step 1: 写失败测试**（两个包各一）：`New(Options{Executable:"fallback", ResolveExecutable: func() string { return "/custom/pi" }})` 后 `Probe`/version 使用 `/custom/pi`；resolver 返回空时回退 `fallback`（可用 `VersionProbe` 注入断言传入的可执行名）。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/agents/pi/ ./internal/agents/opencode/ -run TestResolveExecutable -v`。
- [ ] **Step 3: 实现**：新增字段与 `execPath()`；把所有 `a.executable` 引用改为 `a.execPath()`（Probe、runVersionCommand、ListModels、process spec/launch）。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/agents/pi/ ./internal/agents/opencode/ -v`。
- [ ] **Step 5: 提交**：`git add internal/agents && git commit -m "feat: resolve agent executables at runtime"`。

### Task D3: Agent 管理 API 与检测

**Files:**
- Create: `internal/settings/handlers_agents.go`、`internal/settings/handlers_agents_test.go`
- Modify: `internal/settings/service.go`（`RegisterRoutes` 注册 agent 路由）

**Interfaces:**
- Produces（见设计 §5.3）：`GET /api/agents`、`PUT /api/agents/:backend`、`PUT /api/agents/:backend/path`、`POST /api/agents/:backend/detect`。
- Consumes: `settings` agent 方法、`exec.LookPath`、`agents.ParseVersion`。

- [ ] **Step 1: 写失败测试**：GET 返回 path/pathSource/available/version；`PUT path` 在 `LookPath` 成功时 409 `auto_detected`，失败时接受存在的可执行文件、非法路径 400；detect 成功持久化 version（用临时脚本回显版本号），失败返回状态与错误。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/settings/ -run TestAgentHandler -v`。
- [ ] **Step 3: 实现**：`detect` 解析路径（`ResolvePath`）后 `exec.CommandContext(path, "--version").Output()`，`agents.ParseVersion` 解析，成功 `RecordDetection`；返回 `{backend,status,path,version,checks}`（checks 复用一个 executable/version 诊断结构，或直接给 `status`/`error`）。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/settings/ -v`。
- [ ] **Step 5: 提交**：`git add internal/settings && git commit -m "feat: expose agent management API with detection"`。

### Task D4: 平台启用门禁（instances + sessions）

**Files:**
- Modify: `internal/agents/instances/service.go`、`internal/agents/instances/handlers.go`、`internal/agents/instances/service_test.go`
- Modify: `internal/sessions/service.go`、`internal/sessions/handlers.go`、`internal/sessions/service_test.go`

**Interfaces:**
- Produces: `instances.WithPlatformEnabled(func(agents.BackendID) bool) Option`；`instances.ErrAgentDisabled`；`sessions.WithPlatformEnabled(...)`；`sessions.ErrAgentDisabled`。
- Consumes: `settings.Enabled`。

- [ ] **Step 1: 写失败测试**：平台停用时 `instances.Start` 返回 `ErrAgentDisabled`；`sessions.Create` 在已有运行实例时仍返回 `ErrAgentDisabled`；启用后恢复；停用不影响已有会话读取。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/agents/instances/ ./internal/sessions/ -run TestPlatformDisabled -v`。
- [ ] **Step 3: 实现**：两个包加 Option 与字段；`Start`/`Create`/`DiscoverProject` 开头判定；handler 映射 `agent_disabled`；`cmd/platform/main.go` 传入 `settings.Enabled`。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/agents/instances/ ./internal/sessions/ -v`。
- [ ] **Step 5: 提交**：`git add -A && git commit -m "feat: gate new tasks on platform agent enablement"`。

### Task D5: `/agent-backends` 暴露平台启用状态

**Files:**
- Modify: `internal/sessions/service.go`、`internal/sessions/handlers.go`、`internal/sessions/service_test.go`
- Modify: `web/src/api/agents.ts`

**Interfaces:**
- Produces: `BackendCapabilities`/`AgentBackend` 增加 `Enabled bool`；前端 `AgentBackend` 增加 `enabled: boolean`。

- [ ] **Step 1: 写失败测试**：`Backends()` 返回的每项 `Enabled` 与 `PlatformEnabled` 一致。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/sessions/ -run TestBackendsEnabled -v`。
- [ ] **Step 3: 实现**：DTO 增加字段并用注入的 `platformEnabled` 填充；更新 `web/src/api/agents.ts` 类型与 `docs/api/openapi.yaml`。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/sessions/ -v && cd web && npm run typecheck`。
- [ ] **Step 5: 提交**：`git add -A && git commit -m "feat: report platform agent enablement in backend list"`。

### Task D6: Agent 管理前端与导航接线

**Files:**
- Modify: `web/src/api/settings.ts`、`web/src/stores/settings.ts`、`web/src/components/settings/AgentSettings.vue`
- Modify: `web/src/components/NewTaskPanel.vue`（按平台启用过滤）
- Create: `web/src/tests/agent-settings.spec.ts`

**Interfaces:**
- Consumes: `/api/agents*`（Task D3）、`AgentBackend.enabled`（Task D5）。
- Produces: settings store 的 `agents`、`loadAgents`、`setEnabled`、`setPath`、`detect`；`AgentSettings.vue` 卡片（启用开关、路径只读/可编辑、检测按钮、版本）。

- [ ] **Step 1: 写失败测试**：自动检测成功时路径只读、无手动输入；自动失败时显示手动输入；检测后显示版本；停用开关调用 PUT；新建任务下拉隐藏平台停用的后端。
- [ ] **Step 2: 运行确认失败**：`cd web && npm test -- --run src/tests/agent-settings.spec.ts src/tests/new-task-panel.spec.ts`。
- [ ] **Step 3: 实现** store + 视图 + NewTaskPanel 过滤。
- [ ] **Step 4: 运行确认通过**：`cd web && npm test -- --run && npm run typecheck`。
- [ ] **Step 5: 提交**：`git add web/src && git commit -m "feat: add agent management UI"`。

### Task D7: 文案、契约与端到端验证

**Files:**
- Modify: `web/src/api/client.ts`、`docs/api/openapi.yaml`、`web/e2e/*.spec.ts`、`docs/operations.md`（如涉及）
- Modify: `web/src/tests/api-contracts.spec.ts`

**Interfaces:**
- Consumes: 前面所有任务。

- [ ] **Step 1:** `api/client.ts` 增加 `invalid_appearance`、`agent_disabled`、`auto_detected`、`invalid_service`、`invalid_model`、`service_not_found`、`model_not_found`、`model_conflict`、`upstream_unreachable`、`upstream_error`、`invalid_path`；删除仅在移除功能中使用的 `root_already_registered`、`invalid_root`、`root_not_found`、`path_not_authorized`。
- [ ] **Step 2:** 更新 `api-contracts.spec.ts` 与 OpenAPI：新增外观/模型/Agent 路径，移除 `/workspaces/roots*`，`AgentBackend` 增 `enabled`。
- [ ] **Step 3:** 更新/新增 E2E：进入设置中心切主题/字体/密度并断言 `documentElement` 属性；新增模型服务与测试；Agent 检测与启停；删除根目录相关用例。
- [ ] **Step 4:** 全量验证：`make verify`（`go vet`、`go test`、前端测试、前端构建）；`make e2e`。
- [ ] **Step 5:** 提交：`git add -A && git commit -m "test: cover settings center and update API contract"`。

---

## Self-Review

**Spec coverage:** 外观（B1/B2/B3/B4 + B5–B8 令牌迁移）↔ 需求 §2；模型（C1–C6）↔ §3；Agent（D1–D7）↔ §4；工作区移除（A1–A3）↔ §5；暂缓项未实现 ↔ §6。全部有任务。

**Type consistency:** `paths.Resolve`、`settings.Appearance/AgentSetting`、`modelconfig.ServiceDTO/ModelDTO`、`modelprobe.FetchModels/TestModel`、`WithPlatformEnabled`、`ErrAgentDisabled`、`AgentBackend.enabled` 在各任务间命名一致。

**Review Focus 对应测试：** 旧路径解析 → A2；停用边界 → D4；Key 语义 → C2/C6；探测输入 → C3；外观保存失败 → B4。

**Proportion:** 计划以接口、测试名与断言为主，未展开实现体；组件令牌迁移按组拆为可独立审查的任务。
