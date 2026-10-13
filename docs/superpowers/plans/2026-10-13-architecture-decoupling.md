# 架构解耦：同构层通用化与异构层特征化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除通用层（sessions、settings、diagnostics、HTTP、前端组件）对具体后端（opencode-v2、pi）的直接依赖，使"新增一个后端"只需新增适配器包、补一份描述符并完成组合根注册，不再修改通用层代码。

**Architecture:** Go/Gin/GORM 后端 + Vue 3/TS/Pinia 前端。三个机制：
1. `internal/agents` 承载通用扩展契约（权限回复、表单、原始事件、`Capabilities` 进入主接口），sessions 只依赖 `agents`。
2. `internal/agents` 新增 `BackendDescriptor` 与 `Catalog`，每个适配器包导出自己的描述符（显示名、二进制名、默认启用、测试版本范围、是否有机器级服务），取代 settings、diagnostics、handlers 中的手写表。
3. `/api/agent-backends` 返回描述符派生的展示元数据，前端据此渲染，组件中不再出现后端字面量。

**Tech Stack:** Go 1.24（`go test ./...`、`go vet ./...`）、Gin、GORM/SQLite、Vue 3 `<script setup>`、Pinia、Vitest（`cd web && npm test -- --run`）、`npm run typecheck`、Playwright e2e（`cd web && npm run test:e2e`）。

**Context:** 本计划对应评审结论 P1–P3，评审见本次对话。已核实的起点：
- `internal/sessions/service_opencode.go:13` 直接 import `internal/agents/opencode`，并以 `opencode.PermissionRequest`、`opencode.FormInfo`、`opencode.PermissionReplier`、`opencode.RawEventOpener`、`opencode.IsExpiredReplyError` 为类型和断言目标（类型定义位于 `internal/agents/opencode/client.go:385,416` 与 `permissions.go:25,34,58`）。
- `internal/sessions/service_opencode.go` 多处以 `agents.BackendOpenCodeV2` 做分支判断。
- `internal/sessions/handlers_opencode.go:16-17` 把通用能力 `prompt`、`interrupt` 挂在 `/opencode/` 前缀下。
- `internal/settings/agents.go:27-32` 手写默认值，`:174-180` 的 `backendBinary` 对未知后端默认返回 `pi`。
- `internal/settings/handlers_agents.go:18` 手写 `orderedBackends`。
- `internal/agents/diagnostics.go:168-169` 手写版本测试范围。
- `internal/agents/contracts.go` 的 `AgentAdapter` 不含 `Capabilities()`，`internal/sessions/service.go:1095-1104` 通过可选接口 `capabilityReporter` 取值，缺失时静默全 false。
- 前端字面量位置：`TaskChatPanel.vue`（112、135、156、237、262 行）、`SessionList.vue:29`、`ProjectSettingsPanel.vue:49,57`、`settings/AgentSettings.vue:23,36-37`、`resources/ResourceEditor.vue:124-135`、`BackendTag.vue:9,13`。

## Global Constraints

- **不新增数据库迁移，不改 `agent_instances` / `sessions` 表结构。**
- **HTTP 向后兼容优先**：新增通用路由后，旧 `/opencode/...prompt`、`/opencode/...interrupt` 保留至 Task 14 统一删除；删除前必须在同一提交内确认前端已全部迁移。
- 错误信封沿用 `{"error":"<code>"}`（`respondError`），前端映射在 `web/src/api/client.ts` 的 `errorMessages`。
- 用户可见文案不得出现"实例"二字（沿用既有决策 8）。
- 任何任务结束都要提交一次；提交前对应测试 + `go vet ./...`（后端）或 `npm run typecheck`（前端）必须干净。
- 不改变 opencode service 的启停语义：业务路径仍不启动、不停止 service（见 `docs/agent-compatibility.md` 的 *Machine-level service* 一节）。
- 每个阶段结束必须跑全量：`go test ./...`、`cd web && npm run typecheck && npm test -- --run`。e2e 在 Phase 4 与 Phase 5 各跑一次。

## Review Focus

一个没看过这份对话的维护者，最容易在这些地方栽：

1. **Capabilities 进入主接口会破坏所有测试假适配器的编译。** 必须先 `grep` 出所有实现 `AgentAdapter` 的测试替身并统一补齐，不能靠运行时兜底掩盖。
2. **移动权限/表单类型时别名要保持 JSON 形状不变**，否则 `/opencode/.../permission` 的线上响应字段会变。用 Task 2 的 JSON 形状测试锁定。
3. **gating 从"后端 ID 判断"改为"能力判断"后，能力缺失的后端必须得到同样的 `ErrUnsupportedCapability`**，不能因为 nil 扩展指针而 panic。
4. **描述符的 `DefaultEnabled`、`Binary` 默认值必须与现状逐字相同**，否则 `settings` 的默认启用语义与 PATH 解析会悄悄变化。
5. **前端 `BackendTag` 对未知后端不能再回退为 `opencode` 标签**，改为中性样式并显示后端原始 ID。
6. **旧浏览器缓存的前端版本仍会调用 `/opencode/.../prompt`**，删除旧路由前要确认部署顺序（先后端保留、前端迁移、再删）。

---

## Phase 0：基线与守卫

### Task 0: 记录基线并加入依赖守卫

**Files:**
- Create: `internal/sessions/dependency_guard_test.go`

**Interfaces:**
- Consumes: `go/parser`、`go/token`（标准库）
- Produces: 一个测试，断言 `internal/sessions` 下非测试 `.go` 文件不得 import `devplatform/internal/agents/opencode` 或 `devplatform/internal/agents/pi`。初始状态下该测试**应当失败**，用于标记待办；Task 6 完成后转绿。

- [ ] **Step 1: 记录基线** — 运行 `go test ./... 2>&1 | tail -20`、`cd web && npm test -- --run 2>&1 | tail -5`，把通过数量写入 PR 描述或本计划末尾的"基线记录"。
- [ ] **Step 2: 写守卫测试** — 用 `parser.ParseDir` 解析 `internal/sessions`，跳过 `_test.go`，断言 import 路径不含上述两个包。
- [ ] **Step 3: 确认当前失败** — `go test ./internal/sessions/ -run TestSessionsDoesNotImportBackendPackages -v` → FAIL，并列出违规文件（`service_opencode.go`）。
- [ ] **Step 4: 提交** — `git add internal/sessions/dependency_guard_test.go && git commit -m "test(sessions): guard against importing backend packages"`

---

## Phase 1：agents 包承载通用扩展契约

### Task 1: 把 PermissionRequest / FormInfo / 回复错误判定迁入 agents

**Files:**
- Create: `internal/agents/extensions.go`
- Create: `internal/agents/extensions_test.go`
- Modify: `internal/agents/opencode/client.go:385-430`（类型改为 `agents` 的别名，保留包内引用）
- Modify: `internal/agents/opencode/permissions.go`（`IsExpiredReplyError` 改为委托 `agents.IsExpiredReplyError`）

**Interfaces:**
- Produces（`agents` 包）：
  - `type PermissionRequest struct { ... }`（字段与 JSON 标签从 `opencode.PermissionRequest` 原样复制）
  - `type FormInfo struct { ... }`（同上）
  - `var ErrReplyExpired = errors.New("agents: reply target no longer pending")`
  - `func IsExpiredReplyError(err error) bool`
  - `type PermissionReplier interface { ... }`、`type RawEventOpener interface { ... }`（方法签名从 `opencode/permissions.go:25-37` 原样搬入，类型引用改为 `agents` 内类型）
- Consumes: 无新依赖。

- [ ] **Step 1: 写 JSON 形状测试** — `extensions_test.go` 断言 `PermissionRequest`、`FormInfo` 序列化后的 key 集合与迁移前逐字相同（先从旧类型抄出 key 列表写进测试）。
- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/ -run TestExtension -v` → 编译失败。
- [ ] **Step 3: 迁移类型** — 在 `agents` 定义上述类型；`opencode` 包内以类型别名 `type PermissionRequest = agents.PermissionRequest` 保留旧名，避免改动适配器内部调用点。
- [ ] **Step 4: 全绿** — `go test ./internal/agents/... -v`、`go vet ./...`。
- [ ] **Step 5: 提交** — `git commit -am "refactor(agents): own the permission and form contracts"`

### Task 2: Capabilities 进入 AgentAdapter 主接口

**Files:**
- Modify: `internal/agents/contracts.go:304-352`（在 `AgentAdapter` 中加入 `Capabilities() Capabilities`）
- Modify: `internal/sessions/service.go:1095-1104`（删除 `capabilityReporter` 可选接口与 fallback，直接调用 `adapter.Capabilities()`）
- Modify: 所有实现 `AgentAdapter` 的测试替身（先用 `grep -rn "func (.*) ListModels(" internal --include=*_test.go` 与 `contracts_test.go` 核对清单）
- Test: `internal/agents/contracts_test.go`

**Interfaces:**
- Produces: `AgentAdapter.Capabilities() Capabilities`。`Registry.Register` 不做额外校验（由编译期保证）。

- [ ] **Step 1: 列出替身** — 运行 grep，把所有需要补 `Capabilities()` 的测试类型记入任务备注。
- [ ] **Step 2: 写红测试** — `contracts_test.go` 增加一个编译期断言 `var _ AgentAdapter = (*fakeAdapter)(nil)` 的桩，并断言 `sessions` 的 `capabilities()` 不再有 fallback 分支（以行为测试代替：一个未声明能力的替身返回全零值）。
- [ ] **Step 3: 实现** — 接口加方法；替身统一补 `Capabilities()`；sessions 删除可选接口。
- [ ] **Step 4: 全绿** — `go test ./... -v`（至少 `./internal/agents/... ./internal/sessions/...`）。
- [ ] **Step 5: 提交** — `git commit -am "refactor(agents): capabilities are part of the adapter contract"`

---

## Phase 2：sessions 只依赖 agents，能力驱动门控

### Task 3: sessions 切换到 agents 的扩展类型

**Files:**
- Modify: `internal/sessions/service_opencode.go`（删除 `opencode` import；`PermissionRequest`、`FormInfo` 别名改指 `agents`；断言改为 `agents.PermissionReplier`、`agents.RawEventOpener`；`opencode.IsExpiredReplyError` 改为 `agents.IsExpiredReplyError`）

**Interfaces:**
- Consumes: Task 1 的 `agents` 扩展类型。

- [ ] **Step 1: 确认红** — `go test ./internal/sessions/ -run TestSessionsDoesNotImportBackendPackages -v` 仍 FAIL，违规点只剩 import 本身。
- [ ] **Step 2: 修改** — 按上面清单替换，文件内不再出现 `opencode.` 标识符。
- [ ] **Step 3: 全绿** — `go test ./internal/sessions/... -v`、`go vet ./...`。
- [ ] **Step 4: 提交** — `git commit -am "refactor(sessions): depend on agents extensions, not the opencode package"`

### Task 4: 以能力取代后端 ID 分支

**Files:**
- Modify: `internal/agents/contracts.go`（`Capabilities` 增加 `ToolConfirmation` 已有；新增 `RawEvents bool`，JSON `rawEvents`，注释说明"可读取原始事件流，供调试与专属视图使用"）
- Modify: `internal/sessions/service_opencode.go`（所有 `agents.BackendID(row.Backend) != agents.BackendOpenCodeV2` 与 `s.capabilities(agents.BackendOpenCodeV2)` 改为 `s.capabilities(backend).ToolConfirmation` / `.RawEvents` / `.Streaming`，适配器取值改为按 `row.Backend`）
- Modify: `internal/agents/opencode/adapter.go:194-`（`Capabilities()` 中声明 `RawEvents: true`，pi 为 false）
- Test: `internal/sessions/service_opencode_test.go`、`internal/agents/opencode/adapter_test.go`（`TestAdapterCapabilitiesDeclared` 增加 `RawEvents` 断言）

**Interfaces:**
- Produces: `Capabilities.RawEvents`。门控语义：能力为 false → 返回 `ErrUnsupportedCapability`（HTTP 409 映射沿用既有）；能力为 true 但适配器未实现扩展接口 → 同样返回 `ErrUnsupportedCapability`，不 panic。

- [ ] **Step 1: 写红测试** — 新增一个"声明了 ToolConfirmation 但未实现 PermissionReplier"的替身，断言权限回复返回 `ErrUnsupportedCapability`；新增 pi 会话调用 raw-events 返回 `ErrUnsupportedCapability`。
- [ ] **Step 2: 确认失败** → `go test ./internal/sessions/ -run 'TestPermissionReply|TestRawEvents' -v`。
- [ ] **Step 3: 实现** — 替换分支；断言失败时走 `ErrUnsupportedCapability`。
- [ ] **Step 4: 全绿** — `go test ./internal/sessions/... ./internal/agents/... -v`。
- [ ] **Step 5: 提交** — `git commit -am "refactor(sessions): gate opencode-only operations by declared capability"`

### Task 5: 通用能力上移为通用路由

**Files:**
- Modify: `internal/sessions/handlers_opencode.go:16-17`（删除 `/opencode/.../prompt` 与 `/opencode/.../interrupt` 的注册，**本任务保留旧注册为别名**，同一 handler 挂到通用路径）
- Modify: `internal/sessions/handlers.go`（新增 `POST /projects/:id/sessions/:sessionId/prompt` 与 `POST /projects/:id/sessions/:sessionId/interrupt`，复用 `handleOpencodePrompt`、`handleInterrupt` 的逻辑，改名为 `handlePrompt`）
- Test: `internal/sessions/service_test.go`（或新增 `handlers_test.go`）覆盖两条路径返回一致的 JSON。

**Interfaces:**
- Produces: 通用路由 `POST /api/projects/{id}/sessions/{sessionId}/prompt`、`POST /api/projects/{id}/sessions/{sessionId}/interrupt`。
- 旧 `/opencode/...` 两条路由暂留，由 Task 14 删除。

- [ ] **Step 1: 写红测试** — 通用路径返回 200 且 body 与旧路径一致；pi 会话调通用 interrupt 时按 Capability 决定是否 409。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现** — 抽出 handler 名称、注册两套路由。
- [ ] **Step 4: 全绿** — `go test ./internal/sessions/... ./internal/http/... -v`。
- [ ] **Step 5: 提交** — `git commit -am "feat(sessions): generic prompt and interrupt routes"`

---

## Phase 3：后端描述符取代手写表

### Task 6: BackendDescriptor 与 Catalog

**Files:**
- Create: `internal/agents/descriptor.go`
- Create: `internal/agents/descriptor_test.go`

**Interfaces:**
- Produces:
  ```go
  type BackendDescriptor struct {
      ID               BackendID
      Label            string   // 展示名，如 "OpenCode"、"Pi Agent"
      Binary           string   // 默认可执行名，如 "opencode"、"pi"
      DefaultEnabled   bool
      Order            int      // 展示顺序，升序
      Tested           string   // 测试版本，如 "2.0.24"
      Minimum, Maximum string   // 支持范围，闭开区间 [Minimum, Maximum)
      ServiceSupported bool     // 是否有机器级服务（ServiceController）
  }
  type Catalog struct{ ... }
  func NewCatalog(descriptors ...BackendDescriptor) (*Catalog, error) // 拒绝无效 ID、重复 ID、空 Binary
  func (c *Catalog) Get(id BackendID) (BackendDescriptor, bool)
  func (c *Catalog) All() []BackendDescriptor // 按 Order 排序
  ```
- `SupportedVersionFor` 在 Task 9 改为基于 Catalog，本任务先保留旧函数。

- [ ] **Step 1: 写红测试** — 覆盖：重复 ID 报错、空 Binary 报错、无效 ID 报错、`All()` 按 Order 排序、`Get` 未知返回 false。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 全绿** — `go test ./internal/agents/ -v`。
- [ ] **Step 5: 提交** — `git commit -m "feat(agents): backend descriptor and catalog"`

### Task 7: 适配器包导出描述符

**Files:**
- Create: `internal/agents/opencode/descriptor.go`、`internal/agents/pi/descriptor.go`
- Test: `internal/agents/opencode/adapter_test.go`、`internal/agents/pi/adapter_test.go`

**Interfaces:**
- Produces: `var Descriptor = agents.BackendDescriptor{...}`，值从现有代码逐字取得：
  - opencode：`ID=BackendOpenCodeV2`、`Label="OpenCode"`、`Binary="opencode"`、`DefaultEnabled=true`、`Order=1`、`Tested="2.0.24"`、`Minimum="2.0.0"`、`Maximum="3.0.0"`、`ServiceSupported=true`
  - pi：`ID=BackendPi`、`Label="Pi Agent"`、`Binary="pi"`、`DefaultEnabled=true`、`Order=2`、`Tested="1.1.0"`、`Minimum="1.0.0"`、`Maximum="2.0.0"`、`ServiceSupported=false`

- [ ] **Step 1: 写测试** — 断言 `Descriptor.Tested` 等字段与 `agents/diagnostics.go:168-169` 原表相同，断言 `ServiceSupported` 与适配器是否实现 `ServiceController` 一致（用 `var _ agents.ServiceController = (*Adapter)(nil)` 对 opencode 成立，pi 不成立）。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 全绿** — `go test ./internal/agents/... -v`。
- [ ] **Step 5: 提交** — `git commit -m "feat(agents): adapters export their descriptors"`

### Task 8: 组合根装配 Catalog 并做启动校验

**Files:**
- Modify: `cmd/platform/main.go:244-260`（构造 `agents.NewCatalog(opencode.Descriptor, pi.Descriptor)`；对注册表中每个 backend 校验存在描述符，缺失则启动失败）
- Modify: `cmd/platform/main.go:248-250`、`:253-259` 的注册调用处理不变
- Modify: `internal/settings/service.go`、`internal/settings/agents.go`（`NewService` 接收 `*agents.Catalog`，或新增 `WithCatalog` option，与 `platformEnabled` 风格一致）
- Test: `cmd/platform/main_test.go` 或 `internal/agents/registry_test.go` 增加"注册了无描述符的后端 → 装配失败"用例

**Interfaces:**
- Produces: 装配函数 `func buildCatalog(registry *agents.Registry, descriptors ...agents.BackendDescriptor) (*agents.Catalog, error)`，放在 `cmd/platform`，便于测试。

- [ ] **Step 1: 写红测试** — 无描述符注册时 `buildCatalog` 返回错误。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现** — 装配并把 Catalog 注入 settings。
- [ ] **Step 4: 全绿** — `go test ./cmd/platform/... ./internal/settings/... -v`。
- [ ] **Step 5: 提交** — `git commit -am "feat(platform): assemble the backend catalog at the composition root"`

### Task 9: settings 与 diagnostics 改读 Catalog

**Files:**
- Modify: `internal/settings/agents.go:27-32`（`defaultAgentSettings` 由 Catalog 生成，`Enabled` 取 `DefaultEnabled`）
- Modify: `internal/settings/agents.go:174-180`（`backendBinary` 改为 `Catalog.Get(id).Binary`；未知后端返回 `error`，调用方映射为 `error` 状态，**不再默认 pi**）
- Modify: `internal/settings/handlers_agents.go:18,51-54`（删除 `orderedBackends`，`handleListAgents` 遍历 `Catalog.All()`；`backendParam` 校验改为 `Catalog.Get`）
- Modify: `internal/agents/diagnostics.go:168-169,188-` （`SupportedVersionFor` 基于 Catalog；保留函数签名，内部改为查描述符；若 Catalog 无此后端则 `ok=false`）
- Test: `internal/settings/agents_test.go`、`internal/agents/diagnostics_test.go`

**Interfaces:**
- Consumes: Task 6、8 的 Catalog。
- 注意：`settings` 与 `agents/diagnostics` 的 API 形状保持不变（JSON 字段一个不改），前端无需同步。

- [ ] **Step 1: 写红测试** — `backendBinary` 对未知后端返回错误；`handleListAgents` 返回顺序与旧 `orderedBackends` 一致（opencode 在前）；默认启用状态与旧值一致（两个都 true）。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 全绿** — `go test ./internal/settings/... ./internal/agents/... -v`。
- [ ] **Step 5: 提交** — `git commit -am "refactor(settings): read backend defaults and binaries from the catalog"`

---

## Phase 4：API 元数据与前端去字面量

### Task 10: `/api/agent-backends` 返回展示元数据

**Files:**
- Modify: `internal/sessions/handlers.go:19-20,99-` （`backendListResponse` 每项增加 `label`、`order`、`serviceSupported`，数据来自 Catalog；`Capabilities` 与 `enabled` 保持原样）
- Modify: `internal/sessions/service.go`（`BackendCapabilities` 增加 `Label`、`ServiceSupported`；sessions 需要 Catalog 时通过 `WithCatalog` option 注入）
- Test: `internal/sessions/handlers_test.go`（或既有 `service_test.go:1030`、`:1055` 所在用例）断言新字段存在且 opencode 的 `label="OpenCode"`

- [ ] **Step 1: 写红测试**。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 全绿** — `go test ./internal/sessions/... -v`。
- [ ] **Step 5: 提交** — `git commit -am "feat(sessions): backend list carries display metadata"`

### Task 11: 前端后端元数据 store

**Files:**
- Create: `web/src/stores/backends.ts`
- Modify: `web/src/api/agents.ts:160-162`（`AgentBackendList` 类型增加 `label`、`order`、`serviceSupported`）
- Test: `web/src/tests/backends-store.spec.ts`

**Interfaces:**
- Produces: Pinia store `useBackendsStore()`，暴露 `backends`（按 order 排序）、`label(id)`（未知 id 返回原始 id）、`byId(id)`、`supportsService(id)`、`load()`。
- 任何组件需要后端名称、是否有服务，都通过此 store。

- [ ] **Step 1: 写红测试** — 未知 id 的 label 回退为原始 id；`supportsService('pi') === false`；`load` 失败时保留空列表而非抛错。
- [ ] **Step 2: 确认失败** — `cd web && npm test -- --run backends-store`。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 全绿** — `cd web && npm run typecheck && npm test -- --run`。
- [ ] **Step 5: 提交** — `git commit -am "feat(web): backend metadata store"`

### Task 12: 替换组件中的后端字面量

**Files:**
- Modify: `web/src/components/BackendTag.vue:9,13`（标签文字来自 store；未知后端使用中性 class `tag.unknown`，**不再回退为 opencode**）
- Modify: `web/src/components/SessionList.vue:25-29`（"服务无法连接"提示仅当 `backends.supportsService(session.backend)` 为真时显示）
- Modify: `web/src/components/TaskChatPanel.vue`（112、135、156、237、262 行：`session.backend === 'opencode-v2'` 改为 `session.capabilities.toolConfirmation` 或 `capabilities.rawEvents`，服务提示改用 store）
- Modify: `web/src/components/ProjectSettingsPanel.vue:49,57`（服务状态区块遍历 `backends.backends.filter(b => b.serviceSupported)`，不再写死 `'opencode-v2'`）
- Modify: `web/src/components/settings/AgentSettings.vue:23,36-37`（`serviceBackends` 与 `label` 映射删除，改由 store 提供）
- Modify: `web/src/components/resources/ResourceEditor.vue:124-135`（后端复选框由 store 循环生成，`data-test` 改为 `backend-${id}`；e2e 中既有选择器需同步，搜索 `backend-opencode-v2` 更新）
- Test: 对应 `web/src/tests/*.spec.ts`（`task-chat-panel.spec.ts`、`project-settings-panel.spec.ts`、`agent-settings.spec.ts`、`resources.spec.ts`），新增 `BackendTag` 未知后端用例。

- [ ] **Step 1: 写红测试** — 未知后端的 `BackendTag` 不含 `opencode` 文字；`ResourceEditor` 对 store 中的每个后端渲染一个复选框。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现** — 逐文件替换。
- [ ] **Step 4: 全绿** — `cd web && npm run typecheck && npm test -- --run`。
- [ ] **Step 5: 提交** — `git commit -am "refactor(web): components read backend traits from the store"`

### Task 13: 任务 chat 与 raw-events 的前端门控

**Files:**
- Modify: `web/src/components/TaskChatPanel.vue`（已在 Task 12 处理字面量；本任务负责 `opencodeRaw` store 的调用条件改为 `capabilities.rawEvents`）
- Modify: `web/src/stores/opencodeRaw.ts`（调用入口接收 `capabilities` 检查，不再硬编码后端）
- Test: `web/src/tests/opencode-raw.spec.ts`、`task-chat-panel.spec.ts`

- [ ] **Step 1: 写红测试** — `rawEvents=false` 时不订阅 raw 流。
- [ ] **Step 2: 确认失败**。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 全绿** — `cd web && npm test -- --run`。
- [ ] **Step 5: 提交** — `git commit -am "refactor(web): raw event stream is gated by capability"`

### Task 14: 删除旧的 `/opencode` 通用动作别名

**Files:**
- Modify: `internal/sessions/handlers_opencode.go:16-17`（删除旧注册）
- Modify: `web/src/api/opencode.ts`（若仍引用旧 prompt/interrupt 路径，改为通用路径）
- Modify: `web/src/api/sessions.ts`（如需）
- Test: `internal/sessions/handlers_test.go` 增加"旧路径返回 404"断言

**前置条件**：Task 12、13 合入，且 `grep -rn "/opencode/sessions" web/src` 只剩 permission、form、raw-events 三类。

- [ ] **Step 1: 确认前置** — 运行 grep，结果与前置条件一致才继续。
- [ ] **Step 2: 写红测试** — 旧路径 404。
- [ ] **Step 3: 删除并修前端引用**。
- [ ] **Step 4: 全绿** — `go test ./... -v`、`cd web && npm run typecheck && npm test -- --run`。
- [ ] **Step 5: 提交** — `git commit -am "refactor(sessions): drop the legacy opencode prompt and interrupt aliases"`

---

## Phase 5：文档与验证

### Task 15: 文档同步

**Files:**
- Modify: `docs/agent-compatibility.md`（能力矩阵增加 `rawEvents` 行；说明 `Capabilities` 已在主契约；路由表更新）
- Create: `docs/architecture/backend-boundaries.md`（说明：通用层与异构层的边界、新增后端的步骤清单、禁止事项 —— 通用层不得 import 具体后端包、不得写后端 ID 字面量；与 Task 0 守卫测试对应）
- Modify: `docs/superpowers/plans/2026-10-13-architecture-decoupling.md` 末尾"基线记录"与"完成记录"

- [ ] **Step 1: 更新文档**。
- [ ] **Step 2: 人工核对** — 文档中的路由、字段名与代码逐一比对（`grep`）。
- [ ] **Step 3: 提交** — `git commit -am "docs: backend boundaries and capability matrix"`

### Task 16: 全量验证与 e2e

- [ ] **Step 1: 后端** — `go vet ./... && go test ./... -count=1`，全部通过。
- [ ] **Step 2: 前端** — `cd web && npm run typecheck && npm test -- --run`。
- [ ] **Step 3: e2e** — `cd web && npm run test:e2e`；重点关注 `agent-service.spec.ts`、`agent-settings`（若有）、`resources.spec.ts`、`task-history.spec.ts`、`sessions.spec.ts`。
- [ ] **Step 4: 守卫** — Task 0 的守卫测试转绿。
- [ ] **Step 5: 写完成记录** — 在本计划末尾记录基线对比、被修改的测试数量、e2e 结果。

---

## 不在本计划内（记录决策）

- **项目级后端开关（`AgentBackendConfig`）留在 `instances` 包。** 它与实例启动耦合紧密，迁往项目管理的收益不足以抵消改动风险。若后续需要，另立计划。
- **`Enabled()` 读取失败时开放（fail-open）保持不变。** 这是有意设计，已在 `settings/agents.go` 注释说明；本计划只保证 `backendBinary` 对未知后端显式报错。
- **Resource sync 的适配器仍在组合根手工装配**（`cmd/platform/main.go:270-274`）。它属于异构层的一部分，但不在本次评审范围；待 Catalog 稳定后再评估是否合并进描述符。

## 风险与回滚

- Phase 1（契约变更）影响面最大，若测试替身补齐困难，可先只做 Task 1，把 Task 2 推迟。
- Phase 4 的前端改动与后端路由删除分两次提交，任何一步出问题都可以只回退前端或后端其一。
- 每个阶段结束都可单独发布，不要求一次性完成。

## 基线记录

执行前（Task 0）：
- `go test ./...`：全部包通过（cmd/platform、agents、agents/instances、agents/opencode、agents/pi、auth、database、http、modelconfig、modelprobe、paths、projects、resources、resourcesync、resourcesync/adapters、runtime、secrets、sessions、settings）。
- 前端 `npm test -- --run`：309 个用例通过；`npm run typecheck` 干净。
- e2e：未运行。

## 完成记录

执行后（Task 16）：
- `go vet ./...` 干净；`go test ./... -count=1` 全部通过。
- 前端：`npm run typecheck` 干净；`npm test -- --run` 317 个用例通过（新增 8 个）；`npm run build` 成功。
- e2e：`npm run test:e2e` 17/17 通过。
- 守卫测试：`TestSessionsDoesNotImportBackendPackages`、`TestGenericPackagesNameNoBackend` 通过。

### 与计划的偏离

| 任务 | 计划 | 实际 | 原因 |
| --- | --- | --- | --- |
| Task 0 | 守卫初始为红 | 守卫提交时为红，Task 3 修复后转绿 | 按计划执行；中间提交的 `sessions` 测试包处于红色 |
| Task 5 | 新增通用 `/prompt` 路由 | 不新增路由，直接使用既有的 `POST .../messages` 与 `POST .../interrupt` | 通用路由已存在，`handleOpencodePrompt` 与 `handleSend` 逻辑相同；再加一个名字只会增加第三个同义入口 |
| Task 8 | `buildCatalog(registry, descriptors...)` | 组合根先用 `agents.NewCatalog` 构建 catalog，注册后调用 `checkBackendWiring` 双向校验 | settings 需要 catalog，而适配器注册又依赖 settings 提供的可执行路径，因此 catalog 必须先于注册表建立 |
| Task 9 | diagnostics 改读 catalog | 删除 `supportedVersions` 全局表；适配器直接使用自己的 `Descriptor.SupportedVersion()`；版本范围测试移到各适配器的 `descriptor_test.go` | agents 包不能 import 适配器包（会成环），所以范围断言只能放在适配器侧 |
| Task 11 | 新建 `web/src/stores/backends.ts` | 扩展既有的 `agents` store，增加 `backendFor`、`backendLabel`、`orderedBackends`、`serviceBackends` | 后端列表已由 `agents` store 持有；新建第二个 store 会形成两份数据源 |
| Task 12 | 标签保持原文案 | `BackendTag` 与会话列表改为 catalog 的 Label，原来的 `opencode`、`pi` 变为 `OpenCode`、`Pi Agent` | 这是一处用户可见的文案变化，属于统一名称的结果。资源页复选框仍显示原始 id，保持原样 |
| Task 13 | 原始事件订阅按 `rawEvents` 门控 | 审批列表按 `toolConfirmation` 加载，原始流按 `rawEvents` 订阅，两者分开判断 | 审批的实时更新依赖原始流，两个能力需要分别判断 |
| Task 3 | 未涉及 | 新增 `replyFailure` 纯函数，过期回复的包装由适配器完成 | 让"过期即 409"的映射可以被单测直接覆盖 |
| Task 14 | 前端迁移后删除 | 前端 `opencodePrompt` 已无调用方，删除；`openapi.yaml` 的对应路径同步删除 | 确认无引用后删除 |

### 遗留项（未在本计划内处理）

- `BackendID.Valid()` 仍是 `contracts.go` 中的一个 switch。新增后端时需要改这里，文档已把它列为唯一必须改动的契约位置。
- `internal/sessions/handlers_opencode.go` 文件名与若干方法名仍带 opencode 前缀，内部已不依赖 opencode 包。
- 资源同步的 `OpenCode{}`、`Pi{}` 适配器仍在组合根手工装配。
