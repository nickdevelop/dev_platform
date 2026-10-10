# 交接文档：任务对话与新建任务重设计

日期：2026-10-10
状态：执行中，停在 Task 4。Task 1–3 已完成并提交；Task 4 代码已完成但引入回归，未提交。

## 1. 目标与依据

- 规格：`docs/superpowers/specs/2026-10-10-task-chat-redesign-design.md`（已确认）
- 计划：`docs/superpowers/plans/2026-10-10-task-chat-redesign.md`（13 个任务）
- 执行方式：Native（用户已选定），直接在 master 上实施，不使用工作树（用户此前明确选择）。
- 进度账本：`.superpowers/sdd/2026-10-10-task-chat-redesign/progress.md`（被 `.gitignore` 忽略，不随 git 传递；本文件第 5 节保留了账本全文）。

执行请使用 `superpowers:executing-plans`，从 Task 4 继续。

## 2. 当前 git 状态

- 已完成并提交的提交（从新到旧）：
  - `158bb5e` feat: list opencode models and interrupt a session turn（Task 3）
  - `ab65eb1` feat: list pi models and interrupt a session turn（Task 2）
  - `f442363` feat: add model listing and interrupt to adapter contract（Task 1）
  - `28e34cf` docs: add task chat redesign implementation plan
  - `382d14c` docs: add task chat redesign spec
- 工作区未提交（Task 4 进行中）：
  - 新增：`internal/sessions/turn.go`
  - 新增：`internal/sessions/turn_test.go`
  - 修改：`internal/sessions/service.go`
- 与本次工作无关的既有改动（请勿动）：`.gitignore` 修改、未跟踪的 `dev.sh`。

Task 4 的改动已在本交接中作为一个 WIP 提交保存，见第 7 节。

## 3. 已完成的任务（Task 1–3）

- **Task 1（`f442363`）**：在 `internal/agents/contracts.go` 增加 `Capabilities.Interrupt`、`ModelInfo` 结构，以及 `AgentAdapter` 的 `ListModels` / `Interrupt` 方法。为四处测试 fake（`internal/sessions/service_test.go`、`internal/agents/instances/service_test.go`、`internal/http/router_test.go`、`internal/agents/contracts_test.go`）补齐了新方法。
  裁决：契约新增方法会使 pi 与 opencode 适配器编译失败，因此在两者中加了临时桩（返回 “not implemented”），已在 Task 2、3 中替换。
- **Task 2（`ab65eb1`）**：pi 的 `ListModels`（执行 `pi --list-models`，`internal/agents/pi/models.go` 的 `parseModelList` 解析）与 `Interrupt`（终止目标会话的提示进程）。`internal/agents/pi/process.go` 的 `process` 增加 `sessionID` 与 `interrupted`。
  裁决：被中断的回合发布 `EventLifecycle{State:"agent_settled", Detail:"aborted"}`，不发布 `process_exit` 错误。
- **Task 3（`158bb5e`）**：opencode 的 `ListModels`（`GET /api/model`）与 `Interrupt`（`POST /api/session/{id}/interrupt`），实现于 `internal/agents/opencode/client.go` 与 `adapter.go`；`Capabilities` 声明 `Interrupt: true`。
  裁决：`ModelInfo.ID = providerID/modelID`；`session.execution.failed` 保持映射为 `EventError code=execution_failed`（已有测试覆盖），由 Task 4 的回合跟踪识别它。

## 4. Task 4（进行中）与当前的回归

**Task 4 目标**：会话 DTO 增加 `turn`（`idle`/`running`），由实例生命周期事件推导，不持久化。

**已写好的代码**（工作区未提交）：

- `internal/sessions/turn.go`
  - `type TurnState string`，常量 `TurnIdle="idle"`、`TurnRunning="running"`。
  - `turnFromEvent(ev)`：映射生命周期与错误事件到回合状态。
    - `execution_started`、`agent_start` → running
    - `execution_succeeded`、`execution_interrupted`、`agent_settled`、`agent_end` → idle
    - `EventError` code 为 `execution_failed` 或 `process_exit` → idle
  - `ensureTracker(instanceID)`：每个实例一个后台订阅，订阅 `s.instances.Subscribe`，用 `trackTurns` 消费事件。
  - `turnOf(instanceID, nativeSessionID)`。
- `internal/sessions/service.go`
  - `Session` 增加 `Turn TurnState`（JSON `turn`）。
  - `Service` 增加 `turnMu sync.Mutex`、`trackers map[string]bool`、`turns map[string]map[string]TurnState`，并在 `NewService` 初始化。
  - `Create`、`Send`、`Continue` 成功后调用 `ensureTracker(inst.ID)`；`List` 对运行中的实例调用 `ensureTracker`。
  - `sessionFromModel` 填充 `Turn`。
- `internal/sessions/turn_test.go`：两个用例，`go test ./internal/sessions/ -run Turn` 通过。

**回归**：`ensureTracker` 给实例事件流增加了一个长期订阅者。`internal/sessions/stream_test.go` 的就绪辅助函数 `waitForSubscriber` 只等待 fake 适配器出现一个订阅通道；由于 tracker 先订阅，它会在 `StreamSession` 的 SSE 订阅者注册之前就返回，于是测试在 SSE 订阅者就绪前就发送事件（或先停实例），导致：

- `TestStreamSessionFramesOrderedEvents`（`internal/sessions/stream_test.go:131`）：等待 3 个 SSE 帧超时。
- `TestStreamSessionSkipsMalformedAndPreservesUnknown`（`internal/sessions/stream_test.go:201`）：等待 1 个帧超时。
- `TestStreamSessionEndMarksSessionDisconnected`（`internal/sessions/stream_test.go:326`）：`StreamSession` 返回 `instance is not running: stopped`（SSE goroutine 尚未订阅时实例已被 Stop）。
- `go test -race` 下 `TestStreamSessionSlowClientDisconnectsWithoutStalling` 也会失败。

**已确认的根因**（做了两步验证）：

1. 把 `ensureTracker` 临时改为直接 `return`，`go test ./internal/sessions/ -run TestStreamSession` 全部恢复通过。
2. 在 `fakeAdapter.Subscribe` 加日志，确认整个流程只订阅上游一次（tracker 触发），SSE 订阅者随后加入同一 stream；在 `StreamSession` 的事件循环加日志，确认 SSE 订阅者一个事件都没收到。

结论：这是**测试就绪信号**的问题，不是 `StreamSession` 的生产缺陷。tracker 使 `waitForSubscriber` 失去了“SSE 订阅者已注册”的含义。

**推荐修法（按优先顺序）**：

1. **把回合跟踪下沉到实例服务**（更干净，推荐评估）。`instances.Service.runStream` 已经消费全部事件并做 `applyLifecycle`；在那里维护每会话 `turn`，并由 `sessions.Service` 通过 `InstanceService` 读取。这样不新增第二个订阅者，`stream_test` 无需改动。代价：需要让实例服务在实例运行期间保持上游订阅（目前是按需在 `Subscribe` 时创建），改动比当前方案大。
2. **保留 tracker，修正测试就绪信号**。为 `instances.Service` 增加一个仅测试可用的订阅者计数访问器（例如 `SubscriberCount(instanceID) int`），把 `stream_test` 的 `waitForSubscriber` 改为等待 SSE 订阅者注册（tracker 已占 1，等到 ≥2）。改动小，但把测试耦合到订阅者数量。
3. 其它：让 tracker 延迟到 `List`/`Get` 才订阅（会改变 turn 语义，且 `turn_test` 需先 `List`），不推荐。

无论选哪种，恢复后需重新运行 `go test ./internal/sessions/...` 与 `go test -race ./internal/sessions/...` 全绿，并保留 `turn_test.go` 的两个用例通过。

## 5. 账本全文（备份；原始文件在 `.superpowers/sdd/2026-10-10-task-chat-redesign/progress.md`）

```
# SDD ledger — plan: docs/superpowers/plans/2026-10-10-task-chat-redesign.md

Setup: Ruling: execute on master without a worktree, as in the previous plan, per the human partner's earlier explicit choice — cost if wrong: changes land on master directly.
Spec: docs/superpowers/specs/2026-10-10-task-chat-redesign-design.md (approved).

Pre-flight:
- T1 → T2, T3 implement them; T6 consumes the adapter methods: consistent.
- T4 Service.turns + ensureTracker uses InstanceService.Subscribe (exists) → T5, T6: consistent.
- T5 adds InstanceService.Start, consumes instances.Service.Start (exists): consistent.
- T5 plan says project-disabled backend → 409 unsupported_capability; the code's existing mapping is backend_disabled (409, message exists in web errorMessages). Ruling: use backend_disabled.
- T5 plan says auto-start failure → 502 agent_operation_failed; the existing mapping is instance_start_failed (502). Ruling: keep instance_start_failed.
- T8–T12 consume the JSON names and store APIs named in the plan: consistent.
Baseline: go test ./... green; web vitest 72/72 green.

Task 1: Ruling: AgentAdapter gained ListModels/Interrupt, so the real pi and opencode adapters would not compile — added temporary stubs returning "not implemented"; Tasks 2 and 3 replace them.
Task 1: complete (commits 28e34cf..f442363, tests: go test ./internal/agents/... ./internal/sessions/... ./internal/http/... → ok)
Task 2: Ruling: pi Interrupt publishes a lifecycle agent_settled with Detail "aborted" for the killed turn instead of an error event.
Task 2: Ruling: RED for Interrupt was verified by mutation.
Task 2: complete (commits f442363..ab65eb1, tests: go test ./internal/agents/pi/... → ok)
Task 3: Ruling: plan Step 2 said session.execution.failed must map to a lifecycle event; the code already maps it to EventError code execution_failed, covered by an existing test. Keep the error event; the turn tracker treats it as the end of a turn.
Task 3: Ruling: ModelInfo.ID for opencode is providerID/modelID.
Task 3: complete (commits ab65eb1..158bb5e, tests: go test ./internal/agents/opencode/... → ok)
Task 4: IN PROGRESS — turn.go/turn_test.go added, service.go modified; turn_test passes but 3 existing stream tests regress (see section 4). Not yet committed; handed off.
```

## 6. 剩余任务

后端（计划中）：

- **Task 5**：`Service.Create` 改为按 backend 创建（自动选用运行中实例或启动）；`InstanceService` 增加 `Start`；`handleCreate` 请求体改为 `{backend, title}`。错误码用 `backend_disabled`（409）与 `instance_start_failed`（502），见账本裁决。
- **Task 6**：新增 `Service.ListModels` / `Service.Interrupt` 与路由 `GET /projects/:id/instances/:instanceId/models`、`POST /projects/:id/sessions/:sessionId/interrupt`。
- **Task 7**：更新 `docs/api/openapi.yaml`（`interrupt`、`turn`、`ModelInfo`、新的 sessions 请求体、两个新路由）。

前端（计划中，尚未开始）：

- **Task 8**：`web/src/api/sessions.ts`（`CreateSessionBody { backend; title? }`、`AgentSession.turn`、`Capabilities.interrupt`、`interruptProjectSession`）与 `web/src/api/agents.ts`（`ModelInfo`、`listInstanceModels`）。
- **Task 9**：`web/src/stores/sessions.ts` 的 `interrupt` 与 `refreshSessions`。
- **Task 10**：`web/src/components/ProjectTree.vue` 按 `turn` 显示「运行中」，并有运行中任务时的静默刷新。
- **Task 11**：`web/src/components/TaskChatPanel.vue` 按原型 B 重排；新增 `web/src/components/ModelSelect.vue`；发送按钮在 running 时变为「停止」。
- **Task 12**：`web/src/components/NewTaskPanel.vue` 只选 agent；`web/src/components/ProjectSettingsPanel.vue` 增加「发现原生会话」入口。
- **Task 13**：更新 `web/e2e/*` 与 `web/src/tests/workbench.spec.ts`，跑全量验证（`npx vitest run`、`go test ./...`、`npm run typecheck`、`npm run build`、`npm run test:e2e`），并与原型 B 截图比对。

收尾：整分支最终审查（用最强模型做只读审查），Critical/Important 一次修复，Minor 记入账本；然后 `superpowers:finishing-a-development-branch`。

## 7. 恢复步骤

1. 读本文件第 4 节，决定 Task 4 的修法（推荐先用方案 1 评估，其次方案 2）。
2. 恢复 `.superpowers/sdd/2026-10-10-task-chat-redesign/progress.md`（内容见第 5 节），或按 `superpowers:executing-plans` 的 Setup 重新建立。
3. 若 WIP 已提交，从该提交继续；否则工作区已有 Task 4 的改动。
4. 修好 Task 4 的回归，`go test -race ./internal/sessions/...` 全绿后，提交并运行 `task-done` 记录 Task 4。
5. 按计划从 Task 5 继续。

## 8. 关键文件

- `internal/agents/contracts.go`：`Capabilities`、`ModelInfo`、`AgentAdapter`。
- `internal/agents/pi/adapter.go`、`internal/agents/pi/models.go`、`internal/agents/pi/process.go`、`internal/agents/pi/event_parser.go`。
- `internal/agents/opencode/adapter.go`、`internal/agents/opencode/client.go`。
- `internal/sessions/service.go`、`internal/sessions/handlers.go`、`internal/sessions/turn.go`、`internal/sessions/turn_test.go`、`internal/sessions/stream_test.go`。
- `internal/agents/instances/service.go`（`Subscribe`、`streamFor`、`broadcast`、`addSubscriber`、`runStream`）。
- `docs/api/openapi.yaml`。
- 前端：`web/src/api/sessions.ts`、`web/src/api/agents.ts`、`web/src/stores/sessions.ts`、`web/src/components/ProjectTree.vue`、`web/src/components/TaskChatPanel.vue`、`web/src/components/NewTaskPanel.vue`、`web/src/components/ProjectSettingsPanel.vue`、`web/src/tests/*`、`web/e2e/*`。
- 原型：`/tmp/opencode/prototypes/02-task-chat.html`。
