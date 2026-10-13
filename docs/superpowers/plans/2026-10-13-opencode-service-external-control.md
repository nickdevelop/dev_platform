# opencode service 外部化 + Agent 启停 + 项目设置 tab 化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让平台把 opencode service 当成只可观察、只能由人在平台设置里手动启停的外部依赖，并把项目设置搬进工作区 tab。

**Architecture:** Go/Gin/GORM 后端（`internal/*`）+ Vue 3/TS/Pinia 前端（`web/src`）。后端新增一个跨后端的"机器级服务"契约（`internal/agents/service.go`），opencode 适配器实现它；业务路径（实例启动、关停）与 service 进程彻底解耦；HTTP 新增 `/api/agent-services/:backend`。前端把 tab 模型泛化为两类（任务 / 项目设置），并在 Agent 设置页给出 service 状态与启停。

**Tech Stack:** Go 1.24（`go test ./...`、`go vet ./...`）、Gin、GORM/SQLite、Vue 3 `<script setup>`、Pinia、Vitest（`cd web && npm test -- --run`）、Playwright e2e（`cd web && npm run test:e2e`）。

**Spec:** `docs/superpowers/specs/2026-10-13-opencode-service-external-control-design.md`（本计划按该设计实现，编号"决策 N"指其 §4 决策表）

## Global Constraints

- 用户可见文案**绝不出现"实例"二字**（决策 8）。
- 统一文案逐字使用：`opencode service 未启动，请到平台设置 → Agent 检测并启动。` 与 `无法连接 opencode service，请到平台设置 → Agent 检测并启动。`
- 业务路径任何情况下不启动、不停止、不重启 opencode service（决策 1、3）；唯一启停入口是平台设置 → Agent（决策 4）。
- 业务路径的 service 检查只读 `service.json` + `kill(pid,0)`，**不发 HTTP 请求**（spec §5.3）。
- 不新增数据库迁移；不改 `agent_instances` 表。
- API 错误沿用既有信封 `{"error":"<code>"}`（`internal/*/handlers.go` 的 `respondError`），前端在 `web/src/api/client.ts` 的 `errorMessages` 里映射中文。
- pi 后端不加 service 控件；平台关停仍回收 pi 子进程。
- 每个任务结尾提交一次；提交前该任务的测试与 `go vet ./...`（后端任务）或 `npm run typecheck`（前端任务）必须干净。

## Review Focus

一个没看过这份对话的维护者，最容易在这些输入上栽，且猜错会伤人：

1. **`service.json` 缺失 / JSON 坏 / url 非法 / pid 已死** → 一律判为 `stopped` 并给出可操作文案，绝不 500、绝不"顺手起一个"。
2. **接管来的 service（桌面端或手工起的 pid）** → 业务路径永不 kill；只有设置页的停止会按 pid 杀，杀之前有二次确认。
3. **平台 SIGTERM / `./dev.sh restart`** → opencode service 必须存活，pi 子进程必须仍被回收，且关停不得因为"opencode 实例已不在适配器内存中"而报错。
4. **老浏览器的 `dev-platform-open-tabs`（v1，无 `kind`）** → 刷新后已打开的任务 tab 必须还在（迁移一次到 v2）。
5. **pi 项目/任务** → 不出现 service 区块，opencode service 停了 pi 任务照常可用。
6. **单次网络抖动 / 探活超时** → 不得把会话打成"无法连接"（保留连续两次探测失败才广播失败的既有去抖）。

---

## Task 1: agents 包的机器级服务契约

**Files:**
- Create: `internal/agents/service.go`
- Test: `internal/agents/service_test.go`

**Interfaces:**
- Consumes: `agents.BackendID`（既有）
- Produces:
  - `type ServiceState string` + 常量 `ServiceRunning = "running"`、`ServiceStopped = "stopped"`、`ServiceUnreachable = "unreachable"`
  - `type ServiceStatus struct { Backend BackendID; Supported bool; State ServiceState; Endpoint string; PID int; Version string; Executable string; Detail string; CheckedAt time.Time }`（JSON：`backend,supported,state,endpoint,pid,version,executable,detail,checkedAt`）
  - `type ServiceController interface { DetectService(ctx context.Context) (ServiceStatus, error); ServiceAvailable() error; StartService(ctx context.Context) (ServiceStatus, error); StopService(ctx context.Context) (ServiceStatus, error) }`
  - `var ErrServiceNotRunning, ErrServiceAlreadyRunning, ErrServiceUnsupported error`

- [ ] **Step 1: 写失败测试** — `internal/agents/service_test.go`：`TestServiceStatusJSONShape` 断言序列化后的 key 集合与上面 JSON 名单逐字相同；`TestServiceSentinelsDistinct` 断言三个 sentinel `errors.Is` 互不相干。
- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/ -run 'TestServiceStatusJSONShape|TestServiceSentinelsDistinct' -v` → 编译失败（未定义）。
- [ ] **Step 3: 实现 `internal/agents/service.go`** — 包注释说明"有机器级常驻服务的后端（opencode）实现的可选契约；CLI 型后端（pi）不实现"。`ServiceAvailable() error` 的注释必须写明：便宜检查，只读注册文件与 pid 存活，不发网络请求，不可用时返回 `ErrServiceNotRunning`。
- [ ] **Step 4: 运行确认通过** — 同上命令 → PASS。
- [ ] **Step 5: 提交** — `git add internal/agents/service.go internal/agents/service_test.go && git commit -m "feat(agents): machine-level service contract"`

## Task 2: opencode 业务路径摘除自动启动/停止

**Files:**
- Modify: `internal/agents/opencode/sidecar.go`（`ensureShared`、`watchShared`、新增 `detachShared`）
- Modify: `internal/agents/opencode/adapter.go:495-514`（`StopInstance`）
- Test: `internal/agents/opencode/sidecar_test.go`、`discovery_test.go`、`adapter_test.go`

**Interfaces:**
- Consumes: `agents.ErrServiceNotRunning`（Task 1）
- Produces:
  - `func (a *Adapter) detachShared()` — 清 `a.shared`/`a.sharedStop`，**不 kill 进程**
  - `ensureShared` 在接管失败时返回包装了 `agents.ErrServiceNotRunning` 的错误，且**不再调用** `a.launch`
  - `StopInstance` 不再调用 `teardownShared`

- [ ] **Step 1: 改测试到红** — 在 `discovery_test.go` 旁加 `TestStartInstanceDoesNotLaunchServiceWhenNoRegistration`（注册文件不存在 + `adapter.launch` 换成记录器）：`StartInstance` 返回 `errors.Is(err, agents.ErrServiceNotRunning)` 且 launcher 调用次数为 0。加 `TestStartInstanceTreatsBadRegistrationAsStopped`（JSON 坏的、url 缺 scheme 的、pid 已死的注册文件各一例，均返回 `ErrServiceNotRunning` 不 panic，Review Focus 1）。在 `sidecar_test.go` 加 `TestStopLastInstanceLeavesServiceProcessAlone`：stop 全部实例后 stop hook 调用次数为 0；并确认已有“单次探活失败不得广播失败”的用例存在，缺则补 `TestWatchSharedRequiresTwoConsecutiveFailures`（Review Focus 6）。
- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/opencode/ -run 'TestStartInstanceDoesNotLaunchService|TestStopLastInstance' -v` → FAIL。
- [ ] **Step 3: 实现** — 删 `ensureShared` 的 launch 分支（保留接管逻辑与"已有 shared 且探活通过则复用"）；新增 `detachShared`；`watchShared` 两条失败分支把 `teardownShared()` 换成 `detachShared()`，`failAllInstances` 调用保持；`StopInstance` 删除 `remaining == 0 → teardownShared`。
- [ ] **Step 4: 修既有断言** — 现有测试中凡是通过 `StartInstance` 触发 launch 的（`adapter_test.go:90`、`diagnostics_test.go:253`、`sidecar_test.go:22`），改成先写入合法注册文件或预置 `a.shared`（用测试内已有的 helper），launch 断言留给 Task 3 的 `StartService`。
- [ ] **Step 5: 全绿** — `go test ./internal/agents/opencode/ -v` → PASS；`go vet ./...` 干净。
- [ ] **Step 6: 提交** — `git commit -am "fix(opencode): never launch or kill the service from business paths"`

## Task 3: opencode 的 DetectService / ServiceAvailable / Start / Stop

**Files:**
- Create: `internal/agents/opencode/service.go`
- Test: `internal/agents/opencode/service_test.go`
- Modify: `internal/agents/opencode/sidecar.go`（`launchSharedServer` 的工作目录参数化）

**Interfaces:**
- Consumes: `a.serviceFile()`、`discoverExistingService`、`launchSharedServer`、`waitReady`、`teardownShared`、`failAllInstances`、`a.execPath()`（均既有）
- Produces（`*Adapter` 方法，满足 `agents.ServiceController`）：
  - `DetectService(ctx) (agents.ServiceStatus, error)` — 全量探测：注册文件 → pid 存活 → `GET /api/info`
  - `ServiceAvailable() error` — 便宜探测，无 HTTP
  - `StartService(ctx) (agents.ServiceStatus, error)` — 已 running 则 `agents.ErrServiceAlreadyRunning`；成功用 `a.launch`（`launchSharedServer`）并把 `a.shared` 指向新 service
  - `StopService(ctx) (agents.ServiceStatus, error)` — 按注册文件 pid kill 进程组（外部进程同样处理），再 `detachShared()` + `failAllInstances("opencode service stopped from platform settings")`；未运行时幂等成功
  - 编译期断言：`var _ agents.ServiceController = (*Adapter)(nil)`

- [ ] **Step 1: 写失败测试** — `service_test.go`，逐条：`TestDetectServiceStates`（子测试 missing-file→stopped、dead-pid→stopped、活 pid+API 正常→running 且填 Endpoint/PID/Version、活 pid+API 失败→unreachable 且 Detail 非空且已脱敏）、`TestServiceAvailableCheap`（断言用假 client 也不会被调用；实现上用一个 `http.DefaultTransport` 计数 transport 或直接断言 `DiscoverExistingService` 未被调用）、`TestStartServiceRefusesWhenRunning`、`TestStartServicePublishesSharedServer`（launcher 注入 fake，断言 `sharedEndpoint()` 变化）、`TestStopServiceKillsExternalPid`（注册文件写一个由测试自己 fork 的短命进程 pid，见 Step 3 注）、`TestStopServiceIdempotentWhenStopped`。
- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/opencode/ -run 'TestDetectService|TestServiceAvailable|TestStartService|TestStopService' -v` → FAIL。
- [ ] **Step 3: 实现 `service.go`** — 状态判定复用 `discovery.go` 的解析（把 `discoverExistingService` 里的"读文件+校验"抽成 `readRegistration() (serviceRegistration, bool)` 供两处复用，保持既有接管语义与注释不变）。`StopService` 的 kill 走既有 `killProcessGroup`（对 `syscall.Kill(-pid, SIGTERM/SIGKILL)` 语义已由 `process_unix.go` 提供），pid 来自注册文件而非平台内存。平台**不写不删** `service.json`（决策 4 + spec §5.2），`unreachable` 时尽力 kill 并在 `Detail` 说明。
- [ ] **Step 4: 运行确认通过** — 同 Step 2 → PASS。
- [ ] **Step 5: 提交** — `git commit -am "feat(opencode): manual service detect/start/stop"`

## Task 4: instances 服务门面与 `/api/agent-services`

**Files:**
- Modify: `internal/agents/instances/service.go`（新增 4 个方法）
- Modify: `internal/agents/instances/handlers.go`（路由 + 错误映射）
- Test: `internal/agents/instances/service_test.go`、`internal/agents/instances/handlers_test.go`（或新建 `service_control_test.go`）
- Modify: `docs/api/openapi.yaml`

**Interfaces:**
- Consumes: `registry.Get(backend)`、`agents.ServiceController`（Task 1）
- Produces:
  - `func (s *Service) DetectService(ctx context.Context, backend agents.BackendID) (agents.ServiceStatus, error)` — 未注册后端 → `ErrUnknownBackend`；适配器未实现 `ServiceController` → 返回 `ServiceStatus{Backend: backend, Supported: false}` + nil error
  - `func (s *Service) StartService(ctx context.Context, backend agents.BackendID) (agents.ServiceStatus, error)`、`StopService(...)` — 不支持时 `agents.ErrServiceUnsupported`
  - `func (s *Service) ServiceAvailable(backend agents.BackendID) error` — 便宜闸门，不支持时 nil
  - HTTP：`GET /agent-services/:backend`、`POST /agent-services/:backend/start`、`POST /agent-services/:backend/stop`（响应体 `{"status": <ServiceStatus>}`）
  - 错误码：`service_not_running`(409)、`service_already_running`(409)、`service_unsupported`(400)、`unknown_backend`(400)

- [ ] **Step 1: 写失败测试** — 后端服务层：`TestDetectServiceUnsupportedBackend`（用既有 fake adapter）返回 `Supported:false` 且 err==nil；`TestStartServiceUnsupported` 返回 `agents.ErrServiceUnsupported`；`TestServiceAvailableGatePassesForNonServiceBackend`。HTTP：`TestAgentServiceRoutes`（GET running、POST start 409 `service_already_running`、POST stop 需 CSRF、未知后端 400）。
- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/instances/ -run 'TestDetectService|TestStartService|TestServiceAvailable|TestAgentServiceRoutes' -v` → FAIL。
- [ ] **Step 3: 实现** — 门面方法用 `s.registry.Get` + `if c, ok := adapter.(agents.ServiceController); ok`；路由注册在 `RegisterRoutes` 内（`backendParam` 风格的本地解析函数，非法 id → `unknown_backend`）；`respondServiceError` 补三个 case。
- [ ] **Step 4: 写 OpenAPI** — `docs/api/openapi.yaml`：新增 `AgentServiceStatus` schema（字段与 Task 1 JSON 名单一致）与三个 path，错误信封引用既有 `Error` 组件，补 `service_not_running`/`service_already_running`/`service_unsupported` 枚举值。
- [ ] **Step 5: 全绿** — `go test ./internal/agents/instances/ -v`；`go vet ./...`。
- [ ] **Step 6: 提交** — `git commit -am "feat(api): agent service status and manual start/stop routes"`

## Task 5: sessions 层拒绝未启动的 service

**Files:**
- Modify: `internal/sessions/service.go:115-132`（`InstanceService` 接口）、`:341-359`（`instanceForBackend`）
- Modify: `internal/sessions/handlers.go:57-85`（错误映射）
- Modify: `internal/agents/opencode/adapter.go:376-384`（诊断文案）
- Modify: `docs/agent-compatibility.md`（opencode 列：service 生命周期为外部依赖）
- Test: `internal/sessions/service_test.go`、`internal/sessions/handlers_test.go`（或 `service_test.go` 内）

**Interfaces:**
- Consumes: `instances.Service.ServiceAvailable`（Task 4）
- Produces:
  - `sessions.ErrServiceNotRunning = errors.New("sessions: agent service is not running")`（包装 `agents.ErrServiceNotRunning` 后由 handler 映射为 409 `service_not_running`）
  - `InstanceService` 新成员 `ServiceAvailable(backend agents.BackendID) error`

- [ ] **Step 1: 写失败测试** — 用真实 `instances.NewService` + 实现 `ServiceController` 的 fake adapter（`ServiceAvailable` 返回 `agents.ErrServiceNotRunning`）：`TestCreateRefusedWhenServiceNotRunning` / `TestDiscoverProjectRefusedWhenServiceNotRunning` / `TestContinueRebindRefusedWhenServiceNotRunning` 三个都断言 `errors.Is(err, ErrServiceNotRunning)` **且 `agent_instances` 表行数为 0**（Review Focus 5）；`TestContinueSucceedsAfterServiceStarted` 覆盖 spec §7.1（人工启动后重连可恢复 active）；HTTP 层 `TestSessionCreateServiceNotRunning` 断言 409 + `{"error":"service_not_running"}`。
- [ ] **Step 2: 运行确认失败** — `go test ./internal/sessions/ -run 'ServiceNotRunning|ServiceStarted' -v` → FAIL（接口编译失败也算红）。
- [ ] **Step 3: 实现** — `instanceForBackend` 在 `found` 为 false、要 `Start` 之前调 `s.instances.ServiceAvailable(backend)`，非 nil 就返回包装错误；`Continue` 的重绑分支同一路径（它已调 `instanceForBackend`，无需重复）；`respondServiceError` 补 `case errors.Is(err, ErrServiceNotRunning): respondError(c, 409, "service_not_running")`。诊断文案改为逐字：`opencode service 不可达（%s）；请到平台设置 → Agent 检测并启动`。
- [ ] **Step 4: 全绿** — `go test ./internal/sessions/ -v`；`go test ./...` 全绿（既有 pi/fake adapter 因未实现 `ServiceController` 而不受影响，无需改测试；如有 fake 需要补 `ServiceAvailable`，就地加 `func (f *fakeAdapter) ServiceAvailable(agents.BackendID) error { return nil }`）。
- [ ] **Step 5: 提交** — `git commit -am "feat(sessions): refuse work when the agent service is not running"`

## Task 6: e2e 夹具自持 service

**Files:**
- Modify: `web/e2e/fixtures/bin/opencode`
- Modify: `web/scripts/run-e2e.mjs`
- Test: `web/e2e/*.spec.ts`（按需修断言）

**Produces:** mock 支持 `--service`：启动时把 `{id,version,url,pid,password}` 写到 `$XDG_STATE_HOME/opencode/service.json`（真实 opencode 契约），密码由 mock 自己生成并写进文件；无 `--service` 时行为不变。`run-e2e.mjs` 在启动平台服务器**之前**先起 mock（`XDG_STATE_HOME` 指向本次 e2e 的隔离目录），并把该 `XDG_STATE_HOME` 传给平台进程。

- [ ] **Step 1: 改 mock** — 加 `--service` 分支：解析实际监听端口（`server.address().port`）后写注册文件；`/api/info` 返回与文件一致的 `pid`/`version`；进程退出时删除自己写的文件。
- [ ] **Step 2: 改 run-e2e** — 用现有 `start()` 先起 `opencode serve --service --hostname 127.0.0.1 --port 0`（env 含 `XDG_STATE_HOME`、`OPENCODE_MOCK_STORE_DIR`），`waitFor` mock 的 `/api/info` 就绪需要密码 → 改为轮询注册文件存在且能 `fetch(url+/api/info)`（basic auth 用文件里的 password）。平台进程 env 加同一个 `XDG_STATE_HOME`。
- [ ] **Step 3: 跑受影响 spec** — `cd web && npm run test:e2e -- e2e/sessions.spec.ts e2e/task-history.spec.ts e2e/agent-failures.spec.ts` → 全 PASS（`agent-failures` 若原本依赖"平台自己拉起 mock"，改为在 spec 内先写坏注册文件模拟不可达）。
- [ ] **Step 4: 加 e2e 覆盖** — 新建 `web/e2e/agent-service.spec.ts`：登录 → 平台设置 → Agent → 断言 opencode 显示"运行中" → 停止（二次确认）→ 断言"未启动" → 打开某项目新建任务 → 断言出现统一文案与跳转链接 → 回到 Agent 页点启动 → 断言"运行中"。
- [ ] **Step 5: 提交** — `git add -A && git commit -m "test(e2e): mock opencode registers itself as the machine service"`

## Task 7: 前端 API 与 store

**Files:**
- Modify: `web/src/api/agents.ts`
- Modify: `web/src/api/client.ts`（`errorMessages`）
- Modify: `web/src/stores/agents.ts`
- Test: `web/src/tests/agents-service.spec.ts`（新建）

**Interfaces:**
- Consumes: 后端 `/api/agent-services/:backend`（Task 4）
- Produces:
  - `export type ServiceState = 'running' | 'stopped' | 'unreachable'`
  - `export interface AgentServiceStatus { backend: BackendId; supported: boolean; state: ServiceState; endpoint?: string; pid?: number; version?: string; executable?: string; detail?: string; checkedAt: string }`
  - `getServiceStatus(backend)` / `startAgentService(backend, csrfToken)` / `stopAgentService(backend, csrfToken)`
  - store：`services: Record<string, AgentServiceStatus | null>`、`serviceError: string`、`detectService(backend): Promise<AgentServiceStatus | null>`、`startService(backend)`、`stopService(backend)`（三者都刷新 `services[backend]` 并写 `serviceError`）
  - `errorMessages` 新增逐字：`service_not_running: 'opencode service 未启动，请到平台设置 → Agent 检测并启动。'`、`service_already_running: 'opencode service 已在运行。'`

- [ ] **Step 1: 写失败测试** — `agents-service.spec.ts`：stub fetch 后依次调用三个 action，断言请求路径/方法/CSRF 头，以及 `services['opencode-v2'].state`；断言 `describeApiError(new ApiError(409,'service_not_running'))` 等于上面逐字文案。
- [ ] **Step 2: 运行确认失败** — `cd web && npm test -- --run src/tests/agents-service.spec.ts` → FAIL。
- [ ] **Step 3: 实现 + 通过** — 同上命令 → PASS；`npm run typecheck` 干净。
- [ ] **Step 4: 提交** — `git add -A && git commit -m "feat(web): agent service api and store"`

## Task 8: Agent 设置页 service 区块与二次确认

**Files:**
- Modify: `web/src/components/settings/AgentSettings.vue`
- Modify: `web/src/tests/agent-settings.spec.ts`

**Produces:** 每个后端卡片下的 service 区块，`data-test` 依次为 `service-state-<backend>`、`service-endpoint`、`service-pid`、`service-checked-at`、`service-detect`、`service-start`、`service-stop`、`service-stop-confirm`、`service-error`。

- [ ] **Step 1: 写失败测试** — 断言：挂载时调用一次 `GET /agent-services/opencode-v2` 且**不调用** `GET /agent-services/pi`（Review Focus 5）；`state==='running'` 时 `service-start` 禁用、`service-stop` 可用；点 `service-stop` 先出现确认框且**未**发出 stop 请求（Review Focus 2），点 `service-stop-confirm` 后才发；`state==='stopped'` 时 `service-stop` 禁用；`checkedAt` 渲染成 `最后检测于`；`detail` 在 `unreachable` 时可见。
- [ ] **Step 2: 运行确认失败** — `cd web && npm test -- --run src/tests/agent-settings.spec.ts` → FAIL。
- [ ] **Step 3: 实现** — `onMounted` 里对 `supported !== false` 的后端各调一次 `detectService`；确认框用组件内 `confirmStop: string | null` 状态 + 一段正文「这会停止本机所有正在使用该 service 的客户端，包括桌面端 opencode。正在运行的任务会被中断。」，确认按钮文案逐字「确认停止」；不做轮询（决策 5）。
- [ ] **Step 4: 通过** — 同 Step 2 → PASS。
- [ ] **Step 5: 提交** — `git commit -am "feat(web): service status and manual controls in agent settings"`

## Task 9: openTabs 支持两类 tab

**Files:**
- Modify: `web/src/stores/openTabs.ts`
- Test: `web/src/tests/open-tabs.spec.ts`（新建）

**Produces:**

```ts
export type OpenTab =
  | { kind: 'task'; projectId: number; sessionId: string; title: string }
  | { kind: 'project-settings'; projectId: number }
export function tabKey(tab: OpenTab): string  // task → sessionId；settings → `settings:${projectId}`
```
actions：`openTab(projectId, sessionId, title?)`（只处理 task，签名不变）、`openProjectSettings(projectId)`（同项目幂等）、`closeTab(sessionId string)`（**保留现有签名**，关闭该 sessionId 的 task tab，供 `TaskChatPanel.vue:326` 等现有调用点使用）、`closeTabByKey(key string)`（关闭任一类 tab，`AppShell` 用这个）、保留 `updateTitle`/`closeAllTabs`；storage key 为 `dev-platform-open-tabs-v2`。

- [ ] **Step 1: 写失败测试** — `open-tabs.spec.ts`：`openProjectSettings(3)` 两次只有一条；task 与 settings 可共存且顺序为打开顺序；`tabKey` 两类不同；**迁移测试**：`localStorage['dev-platform-open-tabs']` 写一条 v1 任务 tab → 新 store 读取后它出现在 `tabs` 且带 `kind:'task'`，且写入 v2 key（Review Focus 4）。
- [ ] **Step 2: 运行确认失败** — `cd web && npm test -- --run src/tests/open-tabs.spec.ts` → FAIL。
- [ ] **Step 3: 实现并通过** — 同 Step 2 → PASS。
- [ ] **Step 4: 提交** — `git commit -am "feat(web): tab model supports project-settings tabs"`

## Task 10: 路由、tab 条与项目树入口接线

**Files:**
- Modify: `web/src/router.ts`（新增 `projects/:projectId/settings` → `project-settings`；`projects/:projectId` 改为 `redirect: { name: 'home' }`，决策 13）
- Modify: `web/src/components/TaskTabs.vue`（props 换成 `tabs: OpenTab[]` + `activeKey: string | null`；渲染 `tabKey`，settings tab 用 ⚙ 前缀与 `项目名 · 设置` 标签）
- Modify: `web/src/components/AppShell.vue`（`syncTabWithRoute` 处理两类路由；`selectTab`/`closeTab` 走 `tabKey`；新增 `openSettings(id)`；`onSubmitted` 用 `tabKey` 关该项目的两类 tab；移除对已删路由 `project` 的导航）
- Modify: `web/src/components/ProjectTree.vue`（`.project-settings` 按钮 `emit('open-settings', project.id)`，新增 emit；点项目名的展开/收起逻辑不动）
- Test: `web/src/tests/app-shell.spec.ts`、`web/src/tests/project-tree.spec.ts`、`web/src/tests/workbench.spec.ts`

**Produces:** 路由名 `project-settings`（params `projectId`）；ProjectTree emit `open-settings: [projectId: number]`。

- [ ] **Step 1: 写失败测试** — `project-tree.spec.ts`：点 `.project-settings` emit `open-settings` 且 **不** emit `select-project`；点 `.project-toggle` 仍只切展开态、不 emit 导航（决策 7）。`app-shell.spec.ts`：路由到 `project-settings` 后 tab 条出现 `data-test="open-tab"` 且 `data-tab-key="settings:<id>"`、`aria-selected` 为真；已存在同项目 settings tab 时不产生第二个；关闭活动 settings tab 后回落到最后一个 tab，无 tab 时到 `home`；混排顺序为打开顺序。
- [ ] **Step 2: 运行确认失败** — `cd web && npm test -- --run src/tests/project-tree.spec.ts src/tests/app-shell.spec.ts` → FAIL。
- [ ] **Step 3: 实现** — `syncTabWithRoute`：`route.name === 'project-settings'` → `openProjectSettings`；否则保留现有 task 分支；`selectTab` 按 kind push；`closeTab` 只对 task 调 `sessions.unsubscribe`。
- [ ] **Step 4: 通过 + 修连带** — 上面两个 spec + `workbench.spec.ts`、`project-settings-panel.spec.ts` 全 PASS；`npm run typecheck` 干净。
- [ ] **Step 5: 提交** — `git commit -am "feat(web): open project settings as a workbench tab"`

## Task 11: 项目设置面板去实例 + service 归属提示

**Files:**
- Modify: `web/src/components/ProjectSettingsPanel.vue`（删实例区块与 `startInstance`/`stopInstance`/`loadInstances` 调用；新增 service 提示行）
- Modify: `web/src/components/BackendDiagnostics.vue`（删 `instances` prop、实例列表区块与 `stop` emit）
- Test: `web/src/tests/project-settings-panel.spec.ts`、`web/src/tests/diagnostics.spec.ts`

- [ ] **Step 1: 写失败测试** — `project-settings-panel.spec.ts`：面板 HTML 中不含 `实例`、不含 `data-test="instance-list"` / `stop-instance`（决策 8）；opencode service 为 `stopped` 时出现 `data-test="project-service-down"` 且内含到 `settings-agents` 的链接；挂载只调一次 `GET /agent-services/opencode-v2`。`diagnostics.spec.ts`：不再渲染实例列表。
- [ ] **Step 2: 运行确认失败** — `cd web && npm test -- --run src/tests/project-settings-panel.spec.ts src/tests/diagnostics.spec.ts` → FAIL。
- [ ] **Step 3: 实现并通过** — 同 Step 2 → PASS；提示行文案用统一口径，`data-test="project-service-down"`。
- [ ] **Step 4: 提交** — `git commit -am "refactor(web): drop instances from project settings, show service status"`

## Task 12: 任务面板与新建任务的不可用文案 + 重新连接

**Files:**
- Modify: `web/src/components/TaskChatPanel.vue:89-91,375-378`（`unavailable` 的文案与按钮）
- Modify: `web/src/components/NewTaskPanel.vue`（提交后 409 `service_not_running` 就地显示）
- Modify: `web/src/components/SessionList.vue:19-23`（opencode 的 `disconnected` 标签为「服务无法连接」）
- Test: `web/src/tests/task-chat-panel.spec.ts`、`web/src/tests/new-task-panel.spec.ts`

- [ ] **Step 1: 写失败测试** — chat 面板：`status==='disconnected'` 且 backend 为 `opencode-v2` 时 `data-test="session-unavailable"` 文案含「无法连接 opencode service」且**不含**「实例」，按钮文案「重新连接」且 `data-test="session-reconnect"`，点击后发出 `POST .../sessions/<id>/continue`（决策 14）；pi 会话在 `disconnected` 时仍是原文案（Review Focus 5）。新建任务：continue/create 返回 409 时显示统一文案 + 链接。SessionList：opencode disconnected 显示「服务无法连接」。
- [ ] **Step 2: 运行确认失败** — `cd web && npm test -- --run src/tests/task-chat-panel.spec.ts src/tests/new-task-panel.spec.ts` → FAIL。
- [ ] **Step 3: 实现** — `unavailable` 拆成 `serviceDown`（opencode + disconnected）与原有 unavailable 两种文案；重新连接复用既有 `open()`（内部就是 continue），保持"失败就地换文案"；与既有 SSE `reconnect`（`streamStatus==='error'`）互不干扰、`data-test` 不同名。
- [ ] **Step 4: 通过** — 同 Step 2 → PASS；`npm run typecheck` 干净。
- [ ] **Step 5: 提交** — `git commit -am "feat(web): reconnect affordance when the opencode service is down"`

## Task 13: 全量验证与手工冒烟

**Files:**
- Modify: `docs/agent-compatibility.md`（若 Task 5 未覆盖：service 生命周期一行）、`docs/superpowers/specs/...-design.md`（如实现与设计有偏差，回写）

- [ ] **Step 1: 全量检查** — `make verify` → vet + Go 测试 + 前端测试 + 前端构建全绿。
- [ ] **Step 2: e2e** — `cd web && npm run test:e2e` → 全绿（含新 `agent-service.spec.ts`）。
- [ ] **Step 3: 手工冒烟（决策 3 / Review Focus 3）** — `./dev.sh restart --build` → 打开 opencode 任务 → 从平台设置停止 service → 确认任务显示「无法连接 opencode service」+ 重新连接 → 平台启动 service 后点重新连接确认恢复 → `./dev.sh restart` → `pgrep -af "opencode serve"` 确认进程仍在、平台不再报错、打开任务仍可继续。把观察结果写进本任务的提交信息。
- [ ] **Step 4: 提交** — `git add -A && git commit -m "docs: record manual verification for external opencode service control"`
