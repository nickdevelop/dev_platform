# 平台设置中心技术设计

## 1. 背景与目标

平台需要一个统一的设置中心，覆盖外观、模型、Agent 三块配置，并移除旧的“授权工作区根目录”约束，使项目可在任意目录创建。本设计把 `docs/platform-settings-requirements.md` 的功能需求落成可实施的技术方案，并把需求中所有“暂未指定/待定”的事项定死，避免实施阶段再确认。

平台现有架构：Go + Gin + GORM/SQLite 后端（`internal/*`），Vue 3 + TypeScript + Pinia 前端（`web/src`），OpenAPI 契约在 `docs/api/openapi.yaml`，数据库使用显式版本化迁移（`internal/database/migrations.go`，当前最高版本 12）。

## 2. 范围

### 2.1 纳入

1. 设置中心信息架构与导航。
2. 外观设置：主题配色、字体、字号、界面密度，全平台共享、即时生效。
3. 模型管理：模型服务 CRUD、模型 CRUD、从服务拉取模型列表、模型连通性测试、API Key 安全存储。
4. Agent 管理：平台级启用/停用、存在性与访问路径检测、版本显示、手动路径兜底。
5. 工作区管理调整：移除授权根目录列表与其网页管理，取消项目路径的授权目录约束。

### 2.2 不纳入（与需求一致）

- 安全设置（网页改密码）——暂缓。
- 独立平台诊断——不做；Agent 检测归入 Agent 管理。
- Agent 如何读取/消费平台模型配置——不实现，模型管理仅负责“定义 + 测试”。
- 多用户、权限、跨机同步。

## 3. 决策记录（把不确定事项定死）

> 需求 §2.3 明确“暂未指定”的三项在此定稿；其余为实施所需的技术决策。全部作为本设计的一部分，实施阶段不再单独确认。

### 3.1 外观预设（定稿）

**主题配色：4 套预设**，每套定义完整色板，不只是强调色。前端用 CSS 变量承载，`data-theme` 切换。默认 `light`。

| key | 名称 | 背景 | 面板 | 次级面板 | 正文 | 次要文字 | 边框 | 强调边框 | 主色 | 主色前景 | 危险色 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `light` | 浅色（默认） | `#f4f5f7` | `#ffffff` | `#f8fafc` | `#1f2328` | `#64748b` | `#e3e6eb` | `#d0d5dd` | `#2563eb` | `#ffffff` | `#b91c1c` |
| `dark` | 深色 | `#0f1115` | `#171a21` | `#1e222b` | `#e6e8ef` | `#9aa3b2` | `#2a2f3a` | `#3a4150` | `#4f8cff` | `#0b0d11` | `#f87171` |
| `warm` | 暖阳 | `#f6f1e7` | `#fffaf0` | `#f1e8d8` | `#3a3226` | `#7a6f5c` | `#e4d9c5` | `#d3c4a8` | `#b45309` | `#ffffff` | `#b91c1c` |
| `cool` | 冷灰 | `#eef1f5` | `#f8fafc` | `#e6ebf2` | `#1e293b` | `#5b6b82` | `#d5dde8` | `#c2ccda` | `#0f766e` | `#ffffff` | `#b91c1c` |

色板具体色值属于“我的实现选择”，可在评审时调整；调整只改 `web/src/theme/presets.ts` 与 `web/src/style.css` 两处数据，不影响结构。

**字体：4 套预设 + 自定义 Google Fonts**，`data-font` + `--font-family`。

| key | 名称 | 字体栈 |
|---|---|---|
| `system` | 系统默认（默认） | `-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif` |
| `sans` | 无衬线 | `'Noto Sans SC', 'PingFang SC', 'Microsoft YaHei', 'Segoe UI', sans-serif` |
| `serif` | 衬线 | `'Noto Serif SC', 'Songti SC', Georgia, 'Times New Roman', serif` |
| `mono` | 等宽 | `'JetBrains Mono', 'Fira Code', 'SFMono-Regular', Consolas, monospace` |
| `custom` | 自定义 | 用户输入 Google Fonts family，运行时注入 `<link>`，失败回退 `system` |

自定义字体约束：family 名非空、≤100 字符、字符集 `[A-Za-z0-9 &+'’,.\-]`；URL 用 `https://fonts.googleapis.com/css2?family=<family with + for space>&display=swap`。Google Fonts 不可达时浏览器回退字体栈，不阻塞页面。

**字号：12–20 px，步进 1，默认 14 px。** 通过 `--font-size-base` 作用于 `html` 根字号，组件尺寸改用 `rem` 从而联动。

**界面密度：三档**，只控制间距与控件高度，不改字号。

| key | 名称 | `--space-unit` | `--control-height` | `--radius` |
|---|---|---|---|---|
| `compact` | 紧凑 | `4px` | `28px` | `5px` |
| `standard` | 标准（默认） | `6px` | `34px` | `6px` |
| `relaxed` | 宽松 | `8px` | `40px` | `8px` |

### 3.2 模型管理决策（定稿）

- 服务与模型用独立数据表（`model_services`、`model_defs`）规范化存储，外键级联由服务层事务实现。
- **API Key 用现有平台私有凭据库 `internal/secrets`（`secrets.json`，`0600`）存储**，数据库只存不透明 `secret_ref`。API/UI/日志永不返回 Key 明文；列表只返回 `hasKey`。这复用 MCP 凭据的既有安全模型。
- 编辑服务时 `apiKey` 字段：空串 = 清除已保存 Key；非空 = 替换。与需求 §3.1 完全一致；UI 在输入框显式提示“留空将清除已保存的 Key”。
- **拉取模型列表与模型测试都由后端代理发起**，请求携带“表单当前配置”（未保存也可）：`baseUrl` + `apiKey` 或 `secretRef` +（测试时）`modelId`。测试不写库、不隐式保存。
- 接口地址只允许 `http`/`https`；请求超时 15s；响应体上限 1 MiB；错误信息脱敏（复用 `agents.RedactSecrets`）。
- 兼容 OpenAI 约定：`GET {baseUrl}/models`、`POST {baseUrl}/chat/completions`。`baseUrl` 由用户填写完整前缀（含 `/v1` 时需自行包含），后端仅去掉尾部 `/` 后拼接。
- 测试请求体：`{"model": modelId, "messages":[{"role":"user","content":"ping"}], "max_tokens": 1}`；HTTP 2xx 即成功。
- `usable = service.enabled && model.enabled`；停用服务后其模型 `usable=false`。由于 Agent 消费模型明确不在范围内，本期“可用性”只体现在 API/UI，不接入任务创建。

### 3.3 Agent 管理决策（定稿）

- 平台级 Agent 配置存 `settings` 表 `agents.config`：每个后端 `{enabled, manualPath, version, checkedAt}`。
- **路径解析规则**：`manualPath` 为空 → 每次实时 `exec.LookPath("opencode"/"pi")`；非空 → 使用 `manualPath`。GET 返回实时解析出的 `path` 与 `pathSource`（`auto`/`manual`/`none`）、`available`、上次检测的 `version`。
- **手动路径只在自动检测失败时允许保存**；自动检测成功后 `PUT .../path` 返回 `409 auto_detected`，UI 也不展示编辑入口（满足需求 §4.1）。
- **检测操作**：`POST /api/agents/{backend}/detect` 复用适配器 `Diagnostics`（用解析出的路径），成功持久化 `version`、`checkedAt`，返回检测报告。这满足“检测成功显示版本”，并让 Agent 检测集中在 Agent 管理。
- 适配器可执行文件路径在运行时解析：为 `pi`/`opencode` 的 `Options` 增加 `ResolveExecutable func() string`，适配器所有用到二进制路径处改为调用该方法；未配置时回退默认名。已在运行的实例不受路径变更影响。
- **平台启用状态是新建任务的门禁**，必须在两个入口都判定，因为 `sessions.Create` 会复用已在运行的实例、绕过 `instances.Start`：
  - `instances.Service` 注入 `PlatformEnabled func(agents.BackendID) bool`，`Start` 时若平台停用则拒绝（`ErrAgentDisabled`，HTTP `agent_disabled`）。
  - `sessions.Service` 同样注入 `PlatformEnabled`，在 `Create` 与 `DiscoverProject` 开头判定，停用返回 `ErrAgentDisabled`，handler 映射 `agent_disabled`。停用不停止已有实例、不使已有任务失效。
- 生效规则：`平台启用 AND 项目启用`。`GET /api/agent-backends` 与 `/api/agent-backends/diagnostics` 由 `internal/sessions/handlers.go` 提供，在其 `AgentBackend` DTO 增加 `enabled` 字段（反映平台启用），项目端与新建任务端据此过滤。
- 现有独立诊断面板 `BackendDiagnostics.vue` 从项目设置中移除，检测统一放到 Agent 管理；`/api/agent-backends/diagnostics` 端点保留（Agent 管理复用），不新建“平台诊断”功能。

### 3.4 工作区授权移除（定稿）

- 删除 `internal/workspaces` 包（`Service`、root 路由、`ResolveAuthorizedPath`、`path_policy` 的授权部分），新增 `internal/paths` 只做“规范化 + 存在性/目录校验”，不再做根目录包含校验。
- 项目 `resolve` 改为 `paths.Resolve`，`ErrOutsideRoots` 相关映射移除；`ErrPathChanged` 校验保留（防止已登记路径被符号链接替换）。
- 删除数据库表 `workspace_roots`（迁移 v13 `drop_workspace_roots`）；删除 `WorkspaceRoot` 模型、`/api/workspaces/roots` 三个路由、`WorkspaceRootsDialog.vue` 及其入口。磁盘目录永不被删除。
- 项目创建/登记接受任意绝对路径，仍受进程操作系统文件权限约束。
- 安全边界说明：路径仍做绝对化、`..` 词法归一、符号链接解析、NUL/UNC/盘符相对路径拒绝；失去的是“必须位于白名单根目录”这一层约束，这是需求的明确取舍。

### 3.5 设置中心信息架构（定稿）

- 左侧大类栏底部的“设置”按钮从“打开授权根目录弹窗”改为路由跳转到 `/settings/appearance`。
- 路由（`web/src/router.ts`）在受认证的 shell 下新增：
  - `/settings` → `SettingsView.vue`（左侧二级导航：外观 / 模型 / Agent），默认重定向 `appearance`；
  - `/settings/appearance`、`/settings/models`、`/settings/agents`。
- 进入设置区时隐藏项目侧栏（`meta.settings = true`），让设置区占满右侧面板。
- 设置页全部为平台级（单人共享），保存后即时应用；其他已打开页面刷新后生效（不做实时推送）。

## 4. 数据模型与迁移

新增模型（`internal/database/models.go`）：

```go
// model_services 表
type ModelService struct {
    ID        uint   `gorm:"primaryKey"`
    Name      string `gorm:"not null"`
    BaseURL   string `gorm:"not null"`
    SecretRef string // 空表示无 Key
    Enabled   bool   `gorm:"not null;default:true"`
    CreatedAt, UpdatedAt time.Time
}

// model_defs 表
type ModelDef struct {
    ID            uint   `gorm:"primaryKey"`
    ServiceID     uint   `gorm:"not null;index;uniqueIndex:idx_model_service_wire"`
    ModelID       string `gorm:"not null;uniqueIndex:idx_model_service_wire"` // 服务内唯一
    Name          string `gorm:"not null"`
    SupportsText  bool
    SupportsImage bool
    SupportsTools bool
    ContextTokens int
    OutputTokens  int
    Enabled       bool   `gorm:"not null;default:true"`
    CreatedAt, UpdatedAt time.Time
}
```

迁移步骤追加（不修改 ≤12 的既有步骤）：

- v13 `drop_workspace_roots`：`tx.Migrator().DropTable(&WorkspaceRoot{})`。
- v14 `create_model_services`。
- v15 `create_model_defs`。
- v16 `create_model_defs_service_index`（`idx_model_service_wire`，仿 v12 的显式建索引）。

外观与 Agent 配置复用既有 `settings` 表，不新增表。`Setting` 模型不变。

## 5. 后端 API（新增/变更）

所有路由挂在 `auth.RequireSession` 保护组下，CSRF 由全局中间件处理。

### 5.1 外观

- `GET /api/settings/appearance` → `Appearance`
- `PUT /api/settings/appearance` → `Appearance`

```json
{ "theme": "light", "font": "system", "customFont": "", "fontSize": 14, "density": "standard" }
```

校验：`theme∈{light,dark,warm,cool}`、`font∈{system,sans,serif,mono,custom}`、`font=custom` 时 `customFont` 合法非空、`fontSize∈[12,20]`、`density∈{compact,standard,relaxed}`。非法返回 `400 invalid_appearance`。

### 5.2 模型

- `GET /api/model-services` → `{ services: [ServiceDTO] }`；`ServiceDTO={id,name,baseUrl,hasKey,secretRef,enabled,models:[ModelDTO]}`。
- `POST /api/model-services` `{name,baseUrl,apiKey?,enabled}` → 201 `ServiceDTO`
- `PUT /api/model-services/:id` `{name,baseUrl,apiKey,enabled}`（`apiKey` 空串清除）→ `ServiceDTO`
- `DELETE /api/model-services/:id` → 204（级联删除模型并删除凭据）
- `POST /api/model-services/:id/models` `{modelId,name,supportsText,supportsImage,supportsTools,contextTokens,outputTokens,enabled}` → 201 `ModelDTO`
- `PUT /api/models/:id` 同上字段（`modelId` 可改）→ `ModelDTO`
- `DELETE /api/models/:id` → 204
- `POST /api/model-services/fetch-models` `{baseUrl,apiKey?,secretRef?}` → `{models:[{id,name?}]}`
- `POST /api/models/test` `{baseUrl,apiKey?,secretRef?,modelId}` → `{ok:true,modelId,latencyMs}`

`ModelDTO={id,serviceId,modelId,name,supportsText,supportsImage,supportsTools,contextTokens,outputTokens,enabled,usable}`。错误码：`invalid_request`、`invalid_service`、`invalid_model`、`service_not_found`、`model_not_found`、`model_conflict`、`upstream_unreachable`、`upstream_error`、`server_error`。

### 5.3 Agent

- `GET /api/agents` → `{ agents: [AgentDTO] }`
- `PUT /api/agents/:backend` `{enabled}` → `AgentDTO`
- `PUT /api/agents/:backend/path` `{path}` → `AgentDTO`（自动检测成功时 409 `auto_detected`；否则 `invalid_path`）
- `POST /api/agents/:backend/detect` → `AgentDetectionDTO`（含 `status`、`path`、`version?`、`checks`）
- `GET /api/agent-backends` → 每个后端增加 `enabled`（平台启用）字段

`AgentDTO={backend,enabled,path,pathSource,available,version?,checkedAt?}`；`pathSource∈{auto,manual,none}`。

### 5.4 移除

- `GET/POST /api/workspaces/roots`、`DELETE /api/workspaces/roots/:id` 删除。

## 6. 后端服务与接口

### 6.1 `internal/paths`（新）

```go
var ErrInvalidPath = errors.New("paths: invalid path")
// Resolve 把输入规范化为可操作的绝对规范路径；不存在的尾部保留，最近的已存在祖先做符号链接解析。
func Resolve(input string) (string, error)
```

从 `internal/workspaces/path_policy.go` 迁移 `normalizeInput`/`canonicalize`/`translateWindowsPath` 及对应用例；删除 `ResolveAuthorizedPath`、`ResolvedPath`、`ErrOutsideRoots`。

### 6.2 `internal/settings`（新）

```go
type Appearance struct {
    Theme      string `json:"theme"`
    Font       string `json:"font"`
    CustomFont string `json:"customFont"`
    FontSize   int    `json:"fontSize"`
    Density    string `json:"density"`
}
type AgentSetting struct {
    Enabled    bool   `json:"enabled"`
    ManualPath string `json:"manualPath"`
    Version    string `json:"version"`
    CheckedAt  string `json:"checkedAt"`
}

func NewService(db *gorm.DB) *Service
func (s *Service) Appearance() (Appearance, error)          // 未配置返回默认值
func (s *Service) SetAppearance(a Appearance) (Appearance, error) // 校验并持久化
func (s *Service) Agents() (map[agents.BackendID]AgentSetting, error)
func (s *Service) SetAgentEnabled(backend agents.BackendID, enabled bool) (AgentSetting, error)
func (s *Service) SetAgentPath(backend agents.BackendID, manualPath string) (AgentSetting, error)
func (s *Service) RecordDetection(backend agents.BackendID, version string) (AgentSetting, error)
func (s *Service) Executable(backend agents.BackendID) string // manualPath 或 ""
func (s *Service) Enabled(backend agents.BackendID) bool
func (s *Service) ResolvePath(backend agents.BackendID) (path string, source string) // LookPath 或 manual
```

实现要点：读写 `settings` 表 JSON 值；`defaultAppearance` 提供 `light/system//14/standard`；并发读写用互斥锁；`Executable`/`Enabled` 走内存缓存并随写更新，供适配器闭包高频调用（也可每次查表，量小可接受——采用每次查表 + 短时缓存，见实施计划）。

### 6.3 `internal/modelconfig`（新）

```go
type ServiceDTO / ModelDTO / Service / Model ...
func NewService(db *gorm.DB, store SecretStore) *Service
func (s *Service) List() ([]ServiceDTO, error)
func (s *Service) Create(input ServiceInput) (ServiceDTO, error)
func (s *Service) Update(id uint, input ServiceInput) (ServiceDTO, error)
func (s *Service) Delete(id uint) error
func (s *Service) AddModel(serviceID uint, input ModelInput) (ModelDTO, error)
func (s *Service) UpdateModel(id uint, input ModelInput) (ModelDTO, error)
func (s *Service) DeleteModel(id uint) error
func (s *Service) ResolveKey(serviceID uint) (string, error) // 内部使用
// SecretStore 接口：Set/Get/Delete/Exists（*secrets.Store 实现）
```

### 6.4 `internal/modelprobe`（新）

```go
func FetchModels(ctx context.Context, baseURL, apiKey string) ([]RemoteModel, error)
func TestModel(ctx context.Context, baseURL, apiKey, modelID string) (TestResult, error)
```

`baseURL` 规范化 + `http(s)` 校验、超时、响应上限、脱敏错误。被模型 handler 调用；请求层解析 `apiKey` / `secretRef`（`secretRef` 经 `secrets.Store` 取值）。

### 6.5 现有改动

- `internal/projects`：`resolve`/`resolveExistingDirectory` 改用 `paths.Resolve`；删除 `authorizedRoots`；错误映射去掉 `ErrOutsideRoots`。
- `internal/agents/instances`：注入 `PlatformEnabled func(agents.BackendID) bool`；新增 `ErrAgentDisabled`；`Start` 先判平台启用再判项目启用；handler 映射 `agent_disabled`；错误映射改用 `paths.ErrInvalidPath`。
- `internal/sessions`：`Service` 注入 `PlatformEnabled func(agents.BackendID) bool`；新增 `ErrAgentDisabled`；`Create`、`DiscoverProject` 开头判定；handler 映射 `agent_disabled`；`AgentBackend` DTO 增加 `enabled`。
- `internal/agents/pi`、`internal/agents/opencode`：`Options` 增加 `ResolveExecutable func() string`，实例内 `execPath()`；所有 `a.executable` 使用点改为 `a.execPath()`（Probe、version、ListModels、process/launch）。
- `internal/agents/registry.go`：不新增热更新能力；适配器通过 `ResolveExecutable` 闭包读取当前路径即可，运行中实例不受影响。
- `internal/http/router.go`：`Dependencies` 增加 `Settings *settings.Service`、`Models *modelconfig.Service`；移除 `Workspaces`；注册新路由。
- `cmd/platform/main.go`：构造 `settings.NewService(db)`；用它给适配器传 `ResolveExecutable`；构造 `modelconfig.NewService(db, secretStore)`；`instances.NewService(..., settings.Enabled)`；移除 `workspaces.NewService`。

## 7. 前端架构

### 7.1 主题令牌与迁移

- `web/src/style.css` 定义：`:root` 默认令牌、`[data-theme="light|dark|warm|cool"]` 色板、`[data-density="..."]` 尺寸、`html { font-family: var(--font-family); font-size: var(--font-size-base) }`、`body` 基础色。
- `web/src/theme/presets.ts`：主题/字体/密度元数据（key、标签、色值或字体栈），供设置页渲染与校验。
- `web/src/theme/apply.ts`：`applyAppearance(config)` 设置 `document.documentElement` 的 `dataset.theme`/`dataset.density`、`--font-family`、`--font-size-base`，并注入/移除 Google Fonts `<link id="platform-google-font">`。
- 组件迁移：把所有硬编码颜色/字号/间距替换为令牌（`var(--color-*)`、`rem`、`var(--space-unit)`、`var(--control-height)`、`var(--radius)`）。这是外观需求的主要工作量，按组件分组在计划中拆任务。
- 防闪烁：`main.ts` 挂载前用 `localStorage['platform.appearance']` 缓存即时应用，登录后从服务端拉取覆盖并回写缓存。

### 7.2 新增 API/Store/视图

- `web/src/api/settings.ts`：外观与 Agent 的调用与 DTO。
- `web/src/api/models.ts`：模型服务/模型/拉取/测试的调用与 DTO。
- `web/src/stores/settings.ts`：`appearance` 状态 + `loadAppearance/saveAppearance`（保存后 `applyAppearance`）；`agents` 状态 + `loadAgents/setEnabled/setPath/detect`。
- `web/src/stores/models.ts`：服务/模型列表与 CRUD、拉取候选、测试。
- 视图：`SettingsView.vue`（二级导航 + `RouterView`）、`AppearanceSettings.vue`、`ModelSettings.vue`、`AgentSettings.vue`。
- 组件：外观的分区控件、模型的服务/模型编辑表单（含“获取模型”勾选、测试按钮）、Agent 卡片（启用开关、路径、检测、版本）。
- 删除 `WorkspaceRootsDialog.vue`；`CategoryRail.vue` 设置按钮改 `RouterLink`；`AppShell.vue` 去掉 roots 弹窗、按 `meta.settings` 隐藏项目栏；`ProjectSettingsPanel.vue` 移除 `BackendDiagnostics` 面板。

### 7.3 错误码文案

在 `web/src/api/client.ts` 的 `errorMessages` 增加：`invalid_appearance`、`agent_disabled`、`auto_detected`、`invalid_service`、`invalid_model`、`service_not_found`、`model_not_found`、`model_conflict`、`upstream_unreachable`、`upstream_error`；移除 `root_already_registered`、`invalid_root`、`root_not_found`、`path_not_authorized`（保留 `invalid_path`/`directory_missing` 等仍在用的）。

## 8. 安全与可靠性

- API Key 只进 `secrets.Store`，DB 存 `secret_ref`；列表仅 `hasKey`；日志/错误经 `RedactSecrets`。
- 模型探测限制 scheme、超时、响应体大小；`baseUrl` 不做私网拦截（本机单用户需访问局域网模型服务）。
- 路径移除白名单后仍保留规范化与符号链接防护；`ErrPathChanged` 防止登记路径被替换。
- 迁移 v13 删除 `workspace_roots` 只删授权列表，不触碰磁盘目录。
- 前端保存外观失败时保留原配置并提示，不静默切换。

## 9. 验收与测试策略

- `make verify`：`go vet ./...`、`go test ./...`、前端 Vitest、`npm run build`。
- 后端单元测试：`paths`、`settings`、`modelconfig`、`modelprobe`（用 `httptest`）、projects 无根目录路径、instances 平台停用门禁、适配器 `ResolveExecutable`。
- 前端 Vitest：主题应用、外观校验、模型表单、Agent 卡片、错误码文案。
- 契约：更新 `docs/api/openapi.yaml`（新增外观/模型/Agent 路径，移除 workspaces/roots、`agent-backends` 增字段）。
- E2E：登录后进入设置中心切主题/字体/密度并断言根属性；新增模型服务并测试；Agent 检测与启停；移除根目录相关用例。

## 10. 影响文件清单（初始）

- 后端新增：`internal/paths/`、`internal/settings/`、`internal/modelconfig/`、`internal/modelprobe/`。
- 后端修改：`internal/database/{models,migrations}.go`、`internal/projects/{service,handlers}.go`、`internal/agents/instances/{service,handlers}.go`、`internal/agents/{pi,opencode}/adapter.go`、`internal/agents/registry.go`、`internal/http/router.go`、`cmd/platform/main.go`。
- 后端删除：`internal/workspaces/`。
- 前端新增：`theme/{presets,apply}.ts`、`api/{settings,models}.ts`、`stores/{settings,models}.ts`、`views/` 下 4 个设置视图（或 `components/settings/`）。
- 前端修改：`style.css`、`main.ts`、`router.ts`、`components/{AppShell,CategoryRail,ProjectSettingsPanel}.vue`、`api/client.ts`、各组件样式令牌化。
- 前端删除：`components/WorkspaceRootsDialog.vue`。
- 文档：`docs/api/openapi.yaml`、`docs/operations.md`（如涉及）、本设计对应计划。

## 11. 剩余假设（已定，实施不再确认）

- 主题色值/字体栈如上，属可调数据。
- 模型本期不与 Agent 打通（需求 §3.4）。
- 平台级 Agent 启停与项目级启停取交集。
- `baseUrl` 需用户填完整前缀（含 `/v1`）。
