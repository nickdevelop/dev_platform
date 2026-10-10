# 任务对话与新建任务重设计 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 对话页按原型 B 重排；模型由后端按 agent 提供；「运行中」表示当前回合处理中；新建任务只选 agent；停止只中断当前回合。

**Architecture:** 后端在 adapter 契约中新增 `ListModels` 与 `Interrupt`，并在能力声明中新增 `interrupt`。会话服务为每个运行中的实例维护一个后台订阅，依据生命周期事件推导每个会话的 `turn`（idle/running），并在会话 DTO 中返回。新建会话由后端选用或启动实例。前端对话页、新建任务页与任务树据此重写，能力门控保持不变。

**Tech Stack:** Go（gin、gorm）、Vue 3 + Pinia + Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-10-10-task-chat-redesign-design.md`

## Global Constraints

- `POST /api/projects/{id}/sessions` 请求体为 `{ backend, title? }`，不再接受 `instanceId`。
- 会话 DTO 新增 `turn`，取值仅 `"idle" | "running"`；回合状态不持久化。
- `Capabilities.interrupt` 为 bool；pi 与 opencode 均声明为 true。
- 模型标识格式：`provider/model`，有变体时为 `provider/model#variant`，与 `SendMessageRequest.Model` 一致。
- 所有错误文案沿用 `describeApiError`；不新增未在 `errorMessages` 中定义的错误码。
- 修改 `docs/api/openapi.yaml` 与代码同批提交。

## Review Focus

1. 回合中刷新页面：任务仍显示「运行中」，且停止按钮可用（由会话 DTO 的 `turn` 恢复，不依赖本地状态）。
2. 对空闲任务执行中断：幂等成功，不显示错误，状态保持空闲。
3. 自动启动实例失败：不创建会话，前端显示错误，不留下孤立会话行。
4. 模型列表加载失败：模型选择显示失败提示且发送被禁用，不回退为手填。
5. 在任务 A 运行中切换到任务 B：B 的输入区与运行状态不受 A 的事件影响；A 的回合状态仍在树中正确显示。
6. pi 中断当前回合：不在对话中产生 `process_exit` 错误条目。

---

## Backend

### Task 1: 契约与能力声明

**Files:**
- Modify: `internal/agents/contracts.go`（`Capabilities`、`AgentAdapter`，新增 `ModelInfo`）
- Modify: `internal/sessions/service_test.go`、`internal/agents/instances/service_test.go`、`internal/http/router_test.go`、`internal/agents/contracts_test.go`（补齐 fake adapter 新方法）
- Test: `internal/agents/contracts_test.go`

**Interfaces:**
- Produces:
  - `Capabilities.Interrupt bool`（JSON `interrupt`）。
  - `type ModelInfo struct { ID string \`json:"id"\`; Name string \`json:"name"\`; Provider string \`json:"provider"\`; Variants []string \`json:"variants,omitempty"\` }`
  - `AgentAdapter.ListModels(ctx context.Context, instanceID string) ([]ModelInfo, error)`
  - `AgentAdapter.Interrupt(ctx context.Context, ref SessionRef) error`

- [ ] **Step 1: 写失败测试**：断言 `Capabilities` 的 JSON 包含 `"interrupt"` 字段，且零值为 false。
- [ ] **Step 2: 运行确认失败**：`cd /data/projects/dev_platform && go test ./internal/agents/ -run Capabilities -v`，期望编译失败（字段不存在）。
- [ ] **Step 3: 实现**：在 `contracts.go` 中按上述签名新增字段与方法、`ModelInfo`；为各测试 fake adapter 添加 `ListModels`（返回 `nil, nil`）与 `Interrupt`（返回 `nil`）以保证编译。
- [ ] **Step 4: 运行确认通过**：`go test ./internal/agents/... ./internal/sessions/... ./internal/http/...`，期望全部 PASS（pi/opencode 尚未实现，暂由编译保证）。
- [ ] **Step 5: 提交**：`git commit -am "feat: add model listing and interrupt to adapter contract"`

### Task 2: pi 适配器的模型列表与中断

**Files:**
- Modify: `internal/agents/pi/adapter.go`（`Capabilities`、`ListModels`、`Interrupt`、`instance.procs`）
- Modify: `internal/agents/pi/process.go`（`process` 记录 `sessionID` 与 `interrupted` 标记）
- Create: `internal/agents/pi/models.go`（`parseModelList`）
- Test: `internal/agents/pi/models_test.go`、`internal/agents/pi/adapter_test.go`（补充）

**Interfaces:**
- Consumes: `agents.ModelInfo`、`agents.AgentAdapter` 新方法。
- Produces: `func parseModelList(output string) []agents.ModelInfo`（解析表头后的每行，`ID = provider + "/" + model`，`Provider = provider`，`Name = model`）。

- [ ] **Step 1: 写失败测试**：`parseModelList` 对样例输出（表头 `provider model context max-out thinking images`，含 `occ-command-code  deepseek/deepseek-v4.1-flash ...`）返回 `ID="occ-command-code/deepseek/deepseek-v4.1-flash"`；空输出返回空切片。
- [ ] **Step 2: 写失败测试**：`Interrupt` 只终止目标会话的进程：用 fake pi 可执行文件（参照现有 adapter_test 的 fake 方式）启动两个会话的提示进程，中断其中一个后另一个仍在运行。
- [ ] **Step 3: 写失败测试**：`ListModels` 通过 fake 可执行文件的 `--list-models` 参数获取输出并解析。
- [ ] **Step 4: 运行确认失败**：`go test ./internal/agents/pi/ -run "ModelList|Interrupt|ListModels" -v`。
- [ ] **Step 5: 实现**：
  - `Capabilities()` 中 `Interrupt: true`。
  - `ListModels`：`exec.CommandContext(ctx-with-5s-timeout, a.executable, "--list-models")`，参数以数组传入，输出交给 `parseModelList`。
  - `process` 增加 `sessionID string` 字段，`attachProcess` 时写入。
  - `Interrupt`：找到实例中 `sessionID` 匹配的进程，标记 `interrupted = true` 后 `kill()`；`runProcess` 中若 `interrupted` 则不发布 `process_exit` 错误，而改为发布 `EventLifecycle{State: "agent_settled", Detail: "aborted"}`，使回合状态归为 idle。
  - 会话不存在于实例中 → 返回 nil（幂等）。
- [ ] **Step 6: 运行确认通过**：同 Step 4，期望 PASS；另运行 `go test ./internal/agents/pi/...` 全量。
- [ ] **Step 7: 提交**：`git commit -am "feat: list pi models and interrupt a session turn"`

### Task 3: opencode 适配器的模型列表、中断与生命周期映射

**Files:**
- Modify: `internal/agents/opencode/client.go`（`ListModels`、`Interrupt`）
- Modify: `internal/agents/opencode/adapter.go`（`Capabilities`、`ListModels`、`Interrupt`、`normalize` 中的生命周期列表）
- Test: `internal/agents/opencode/client_test.go`、`internal/agents/opencode/adapter_test.go`（补充）

**Interfaces:**
- Consumes: `agents.ModelInfo`、`Model.Info`（`providerID`、`id`、`variants`、`name`）。
- Produces:
  - `func (c *Client) ListModels(ctx context.Context, directory string) ([]ModelInfo, error)`，调用 `GET /api/model`，与 `ListSessions` 相同的 directory 查询参数。
  - `func (c *Client) Interrupt(ctx context.Context, sessionID string) error`，调用 `POST /api/session/{id}/interrupt`。

- [ ] **Step 1: 写失败测试**：`client_test.go` 中以 httptest 服务器断言 `ListModels` 请求路径为 `/api/model`，并把响应映射为 `ModelInfo`（有变体时 `Variants` 非空）；`Interrupt` 请求为 `POST /api/session/ses_1/interrupt`。
- [ ] **Step 2: 写失败测试**：`normalize` 将 `session.execution.failed` 映射为 `EventLifecycle{State: "execution_failed"}`（当前未映射，测试应先失败）；`session.execution.started`、`succeeded`、`interrupted` 的 state 分别为 `execution_started`、`execution_succeeded`、`execution_interrupted`。
- [ ] **Step 3: 运行确认失败**：`go test ./internal/agents/opencode/ -run "ListModels|Interrupt|Execution" -v`。
- [ ] **Step 4: 实现**：
  - `Capabilities()` 中 `Interrupt: true`。
  - 适配器 `ListModels` 通过实例的 client 调用，返回的 `ID` 为 `providerID/id`；`Interrupt` 调用 client 的 `Interrupt`。
  - `normalize` 的生命周期 case 列表中加入 `"session.execution.failed"`。
- [ ] **Step 5: 运行确认通过**：同 Step 3，期望 PASS；另运行 `go test ./internal/agents/opencode/...`。
- [ ] **Step 6: 提交**：`git commit -am "feat: list opencode models and interrupt a session turn"`

### Task 4: 会话回合状态（turn）

**Files:**
- Modify: `internal/sessions/service.go`（`Session.Turn`、`Service.turns`、`ensureTracker`、`trackTurns`）
- Test: `internal/sessions/service_test.go`

**Interfaces:**
- Consumes: `InstanceService.Subscribe(ctx, instanceID) (<-chan agents.NormalizedEvent, error)`；`agents.NormalizedEvent.Lifecycle.State`、`SessionID`。
- Produces:
  - `type TurnState string`，常量 `TurnIdle = "idle"`、`TurnRunning = "running"`。
  - `Session.Turn TurnState \`json:"turn"\``。
  - `func (s *Service) turnOf(sessionID string) TurnState`（无记录时为 idle）。
  - `func (s *Service) ensureTracker(ctx context.Context, instanceID string)`：每个实例仅一个后台订阅；订阅通道关闭（实例停止）后移除记录。

State 映射（仅此表中的 state 改变回合）：

| state | turn |
| --- | --- |
| `execution_started`（opencode）、`agent_start`（pi） | running |
| `execution_succeeded`、`execution_failed`、`execution_interrupted`（opencode）、`agent_settled`、`agent_end`（pi） | idle |

- [ ] **Step 1: 写失败测试**：用 `fakeAdapter.emit` 为会话发送 `execution_started`，`sessions` 的 `List` 返回的该会话 `Turn == "running"`；随后发送 `execution_succeeded`，返回 `idle`；其他会话不受影响。
- [ ] **Step 2: 写失败测试**：未经任何事件的新会话 `Turn == "idle"`。
- [ ] **Step 3: 运行确认失败**：`go test ./internal/sessions/ -run Turn -v`。
- [ ] **Step 4: 实现**：`Create`、`Send`、`Continue` 成功后调用 `ensureTracker(context.Background(), instanceID)`；`trackTurns` 读取通道并按上表更新 `turns` map（以 `sync.Mutex` 保护）；`sessionFromModel` 填充 `Turn`。
- [ ] **Step 5: 运行确认通过**：同 Step 3，期望 PASS；另运行 `go test ./internal/sessions/...`。
- [ ] **Step 6: 提交**：`git commit -am "feat: track per-session turn state from lifecycle events"`

### Task 5: 按 backend 新建会话（自动选用或启动实例）

**Files:**
- Modify: `internal/sessions/service.go`（`Create` 签名）
- Modify: `internal/sessions/handlers.go`（`createSessionRequest`、`handleCreate`）
- Test: `internal/sessions/service_test.go`、`internal/sessions/handlers_test.go`（如不存在则在 `service_test.go` 中以 `httptest` 覆盖）

**Interfaces:**
- Consumes: `InstanceService.List(projectID)`、`InstanceService.Start(ctx, projectID, backend)`（需在 `InstanceService` 接口中新增 `Start`；`*instances.Service` 已实现）。
- Produces: `func (s *Service) Create(ctx context.Context, projectID uint, backend agents.BackendID, title string) (Session, error)`；请求体 `createSessionRequest { Backend agents.BackendID \`json:"backend"\`; Title string \`json:"title,omitempty"\` }`。

选择规则：
1. `backend` 不合法 → 400 `invalid_request`。
2. 在 `List(projectID)` 中选取该 backend、状态为 running 的实例，取最近创建的一个。
3. 若无，调用 `Start(ctx, projectID, backend)`；`ErrBackendDisabled` → 409 `unsupported_capability`（沿用现有映射）；启动失败 → 502 `agent_operation_failed`，且不创建会话。
4. 调用适配器 `CreateSession`，不携带模型。

- [ ] **Step 1: 写失败测试**：存在运行中实例时复用该实例（`startCount` 不增加）；无实例时启动一次；启动失败时不写入会话索引表；请求体含 `instanceId` 时被忽略并因缺少 `backend` 返回 400。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/sessions/ -run Create -v`。
- [ ] **Step 3: 实现**：按上述签名修改；`handleCreate` 仅校验 `backend`；`InstanceService` 接口新增 `Start`。
- [ ] **Step 4: 运行确认通过**：同 Step 2，期望 PASS；另运行 `go test ./internal/sessions/... ./internal/http/...`。
- [ ] **Step 5: 提交**：`git commit -am "feat: create sessions by backend, reusing or starting an instance"`

### Task 6: 模型列表与中断接口

**Files:**
- Modify: `internal/sessions/service.go`（`ListModels`、`Interrupt`）
- Modify: `internal/sessions/handlers.go`（`handleModels`、`handleInterrupt`）
- Modify: `internal/sessions/handlers.go` 路由注册（`GET /projects/:id/instances/:instanceId/models`、`POST /projects/:id/sessions/:sessionId/interrupt`）
- Test: `internal/sessions/service_test.go`、`internal/sessions/handlers_test.go`

**Interfaces:**
- Produces:
  - `func (s *Service) ListModels(ctx context.Context, projectID uint, instanceID string) ([]agents.ModelInfo, error)`：实例不属于项目 → `instances.ErrInstanceNotFound`（沿用现有映射 404 `instance_not_found`）；`ModelSelection` 为 false → `ErrUnsupportedCapability`。
  - `func (s *Service) Interrupt(ctx context.Context, projectID uint, sessionID string) (Session, error)`：`Interrupt` 能力为 false → `ErrUnsupportedCapability`；当前 turn 为 idle 时直接返回会话（幂等，不调用适配器）；否则调用适配器 `Interrupt`，失败 → `ErrBackendFailed`。
  - 响应：`GET models` → `{ "models": ModelInfo[] }`；`POST interrupt` → 会话 DTO，状态码 200。

- [ ] **Step 1: 写失败测试**：`ListModels` 返回适配器结果；不支持模型选择时返回 409；`Interrupt` 对 idle 会话不调用适配器（fake 记录调用次数为 0）；对 running 会话调用一次。
- [ ] **Step 2: 运行确认失败**：`go test ./internal/sessions/ -run "ListModels|Interrupt" -v`。
- [ ] **Step 3: 实现**：如上；`fakeAdapter` 的 `Interrupt` 记录调用次数，供测试断言。
- [ ] **Step 4: 运行确认通过**：同 Step 2；另运行 `go test ./internal/sessions/... ./internal/http/...`。
- [ ] **Step 5: 提交**：`git commit -am "feat: expose model listing and turn interrupt endpoints"`

### Task 7: API 契约文档

**Files:**
- Modify: `docs/api/openapi.yaml`（`Capabilities` 增加 `interrupt`；会话 DTO 增加 `turn`；`POST /projects/{id}/sessions` 请求体改为 `{backend, title}`；新增 `GET /projects/{id}/instances/{instanceId}/models`、`POST /projects/{id}/sessions/{sessionId}/interrupt`；新增 `ModelInfo` schema）
- Test: `go test ./internal/http/...`（若存在 openapi 一致性测试）；否则以 `grep` 核对字段名与代码一致。

- [ ] **Step 1: 修改 openapi.yaml**，字段名与 Task 1–6 的 JSON 标签完全一致。
- [ ] **Step 2: 校验**：`grep -n "interrupt\|turn\|ModelInfo" docs/api/openapi.yaml` 与 `grep -rn 'json:"turn"\|json:"interrupt"' internal` 逐项对照。
- [ ] **Step 3: 提交**：`git commit -am "docs: update openapi for model listing, turn and interrupt"`

---

## Frontend

### Task 8: API 客户端与类型

**Files:**
- Modify: `web/src/api/sessions.ts`（`CreateSessionBody { backend; title? }`；`AgentSession.turn`；`Capabilities.interrupt`；`interruptProjectSession`）
- Modify: `web/src/api/agents.ts`（`ModelInfo`；`listInstanceModels(projectId, instanceId)`）
- Test: `web/src/tests/api-contracts.spec.ts`（新建）

**Interfaces:**
- Produces:
  - `export type TurnState = 'idle' | 'running'`
  - `export interface ModelInfo { id: string; name: string; provider: string; variants?: string[] }`
  - `listInstanceModels(projectId: number, instanceId: string): Promise<{ models: ModelInfo[] }>`
  - `interruptProjectSession(projectId: number, sessionId: string, csrfToken: string): Promise<AgentSession>`
  - `createProjectSession(projectId, body: CreateSessionBody, csrfToken)`：`CreateSessionBody = { backend: BackendId; title?: string }`

- [ ] **Step 1: 写失败测试**：以 stub fetch 断言 `createProjectSession` 请求体为 `{backend, title}` 且不含 `instanceId`；`listInstanceModels` 请求路径正确。
- [ ] **Step 2: 运行确认失败**：`cd web && npx vitest run src/tests/api-contracts.spec.ts`。
- [ ] **Step 3: 实现**类型与三个函数。
- [ ] **Step 4: 运行确认通过**：同 Step 2；`npm run typecheck`。
- [ ] **Step 5: 提交**：`git commit -am "feat: frontend API for models, turn and interrupt"`

### Task 9: sessions store 与回合刷新

**Files:**
- Modify: `web/src/stores/sessions.ts`（`createSession(projectId, body)` 新签名；`interrupt(projectId, sessionId)`；`refreshSessions(projectId)`）
- Test: `web/src/tests/sessions-store.spec.ts`（补充）

**Interfaces:**
- Produces:
  - `interrupt(projectId: number, sessionId: string): Promise<AgentSession | null>`：成功后用返回的会话 DTO 替换列表中对应项。
  - `refreshSessions(projectId: number): Promise<void>`：静默刷新列表（不清空、不置 loading），仅接受最新请求结果（沿用 `listRequest` 机制）。

- [ ] **Step 1: 写失败测试**：`interrupt` 成功后列表中该会话的 `turn` 为 idle；`interrupt` 对空闲会话的响应不产生 `error`。
- [ ] **Step 2: 写失败测试**：`refreshSessions` 不清空列表，并在网络失败时保留旧列表且不设置 `error`。
- [ ] **Step 3: 运行确认失败**：`npx vitest run src/tests/sessions-store.spec.ts`。
- [ ] **Step 4: 实现**。
- [ ] **Step 5: 运行确认通过**：同 Step 3；`npm run typecheck`。
- [ ] **Step 6: 提交**：`git commit -am "feat: sessions store interrupt and silent refresh"`

### Task 10: 任务树状态与运行中轮询

**Files:**
- Modify: `web/src/components/ProjectTree.vue`（状态标签由 `turn` 决定；`disconnected` 显示「断开」；展开项目存在 running 任务时每 2 秒静默刷新一次）
- Test: `web/src/tests/project-tree.spec.ts`（补充）

**Interfaces:**
- Consumes: `refreshSessions`（Task 9）、`AgentSession.turn`（Task 8）。
- 说明：轮询是为满足规格中「树中准确显示运行中」的要求，因为非打开任务的回合事件不会进入当前 SSE 流。组件卸载时清除定时器。

- [ ] **Step 1: 写失败测试**：`turn: 'running'` 显示「运行中」；`turn: 'idle'` 不显示状态文字；`status: 'disconnected'` 显示「断开」；存在 running 任务时定时器触发一次刷新（使用 `vi.useFakeTimers`）。
- [ ] **Step 2: 运行确认失败**：`npx vitest run src/tests/project-tree.spec.ts`。
- [ ] **Step 3: 实现**：删除 `stopped` 的状态标签映射；添加 `watch` 与 `setInterval`。
- [ ] **Step 4: 运行确认通过**：同 Step 2。
- [ ] **Step 5: 提交**：`git commit -am "feat: task tree shows turn state and refreshes while running"`

### Task 11: 对话页重排（TaskChatPanel）

**Files:**
- Modify: `web/src/components/TaskChatPanel.vue`（头部、消息区、输入卡片；模型选择；停止按钮；删除「继续」「重命名」）
- Create: `web/src/components/ModelSelect.vue`（输入卡片左下的模型下拉；props `{ instanceId; modelSelection: boolean; modelValue }`；通过 `listInstanceModels` 加载；加载失败显示「模型列表加载失败」并通过 emit 通知父组件禁用发送）
- Test: `web/src/tests/task-chat-panel.spec.ts`（重写相关断言）、`web/src/tests/model-select.spec.ts`（新建）

**Interfaces:**
- Consumes: `listInstanceModels`（Task 8）、`interruptProjectSession`（Task 8）、`store.interrupt`（Task 9）、`AgentSession.turn`。
- Produces: `ModelSelect` emits `update:modelValue(id: string)`、`status(ok: boolean)`。
- 布局（对照原型 B）：头部 `h2` 标题、`BackendTag`、实例与目录 chips、回合状态（仅 running 显示「运行中」）；消息区；输入卡片含 `textarea`，底栏左为 `ModelSelect`（仅 `modelSelection` 为 true 时渲染），右为发送按钮。
- 发送按钮：`turn === 'idle'` 时为「发送」；`turn === 'running'` 时为「停止」并调用 `interrupt`。
- 停止按钮不再使用 `stop` 能力；`interrupt` 能力为 false 时不渲染停止按钮（所有已知 agent 均为 true）。
- 删除：「重命名」「继续」按钮及其调用；工具确认仍由 `toolConfirmation` 门控。

- [ ] **Step 1: 写失败测试（TaskChatPanel）**：头部不含「重命名」「继续」；`turn: 'running'` 时存在「停止」按钮，点击调用 interrupt；`turn: 'idle'` 时为「发送」。
- [ ] **Step 2: 写失败测试（ModelSelect）**：加载成功后渲染模型选项，默认选第一项并 emit；加载失败显示「模型列表加载失败」且 emit `status(false)`；`modelSelection` 为 false 时不渲染。
- [ ] **Step 3: 写失败测试（Review Focus 4 与 6）**：模型加载失败时发送按钮禁用；pi 中断后消息区不出现 `process_exit`（由 store 的事件处理保证，测试 store 的 `handleEvent` 忽略 `agent_settled` 的 aborted 详情）。
- [ ] **Step 4: 运行确认失败**：`npx vitest run src/tests/task-chat-panel.spec.ts src/tests/model-select.spec.ts`。
- [ ] **Step 5: 实现**：按布局重写 `TaskChatPanel` 模板；保留 `activateSession` 与 `unsubscribeAll` 的生命周期逻辑（Review Focus 5 由既有隔离测试覆盖）。
- [ ] **Step 6: 运行确认通过**：同 Step 4；`npm run typecheck`。
- [ ] **Step 7: 提交**：`git commit -am "feat: task chat panel per prototype with model select and stop"`

### Task 12: 新建任务页与实例中的「发现原生会话」

**Files:**
- Modify: `web/src/components/NewTaskPanel.vue`（仅 agent 选择与可选标题；提交 `{ backend, title }`）
- Modify: `web/src/components/ProjectSettingsPanel.vue`（实例列表的每个运行中实例旁增加「发现原生会话」入口，调用 `sessions.discoverNativeSessions`）
- Test: `web/src/tests/new-task-panel.spec.ts`（重写）、`web/src/tests/project-settings-panel.spec.ts`（补充）

**Interfaces:**
- Consumes: `createProjectSession`（Task 8）、`GET /api/agent-backends`（`agents.backends`）、`agents.capabilitiesFor`。
- 说明：discover 的入口从新建任务页移到项目设置，因为新建任务页已不再选择实例。这是为落地设计而做出的位置调整，需在规格审阅中确认（已在 spec 第 4.2 节描述，实施时以 spec 为准）。

- [ ] **Step 1: 写失败测试**：NewTaskPanel 只渲染 agent 选择与标题输入；提交载荷为 `{ backend: 'pi', title }`，不含 `instanceId`、`model`。
- [ ] **Step 2: 写失败测试**：未启用的 backend 在选择中禁用并标注「已在项目中禁用」（依据 `backendConfigs`）。
- [ ] **Step 3: 写失败测试（Review Focus 3）**：创建失败（502）时显示错误、不跳转、会话列表不新增条目。
- [ ] **Step 4: 运行确认失败**：`npx vitest run src/tests/new-task-panel.spec.ts src/tests/project-settings-panel.spec.ts`。
- [ ] **Step 5: 实现**。
- [ ] **Step 6: 运行确认通过**：同 Step 4。
- [ ] **Step 7: 提交**：`git commit -am "feat: new task selects only an agent; discover moves to project settings"`

### Task 13: 端到端与全量验证

**Files:**
- Modify: `web/e2e/sessions.spec.ts`、`web/e2e/agent-failures.spec.ts`（`createTask` 改为选择 agent；断言「运行中」「停止」按钮；删除实例下拉与模型输入的断言）
- Modify: `web/e2e/projects.spec.ts`（如依赖旧新建任务流程则同步）
- Modify: `web/src/tests/workbench.spec.ts`（删除已迁移的旧 discover 与实例下拉断言）

- [ ] **Step 1: 更新 e2e**：`createTask(page, backendName, title)` 通过 `#backend-select` 选择；发送后断言树中出现「运行中」；点击「停止」后断言「空闲」。
- [ ] **Step 2: 运行单元测试**：`cd web && npx vitest run`，期望全部 PASS。
- [ ] **Step 3: 运行后端测试**：`cd /data/projects/dev_platform && go test ./...`，期望全部 PASS。
- [ ] **Step 4: 类型检查与构建**：`cd web && npm run typecheck && npm run build`，期望无错误。
- [ ] **Step 5: 端到端**：`cd web && npm run test:e2e`，期望全部 PASS。
- [ ] **Step 6: 视觉核对**：与原型 B 截图比对布局（参照 Task 8 的截图流程，截图后删除临时用例）。
- [ ] **Step 7: 提交**：`git commit -am "test: update e2e and workbench tests for the task chat redesign"`
