# 交接：类型化轮次 / 交互推送 / 会话级 Stop（与 claudian 对齐）

> 分支：`refactor/typed-turn-interaction`（基线 `master`，`700d2ac`）
> 状态：代码、单元测试、文档同步与全量验证均已完成（§3 全部勾销）；待提交。
> 恢复：`git checkout refactor/typed-turn-interaction && git status --short`。

## 1. 目标（用户已确认 1–4 项，第 5 项明确不做）

1. 轮次状态类型化：`turn.go` 不再匹配后端原生字符串，只消费契约 turn 事件。
2. 守卫补齐：禁止会话层出现后端原生轮次事件名。
3. 交互推送：权限/表单走归一化流，去掉前端轮询分支与 raw 订阅。
4. 会话级 Stop：`Stop` 只停本会话，不连带实例与兄弟会话。
5. 不做：恢复槽位（providerState）与分叉/回退。

## 2. 已完成

### 2.1 契约（`internal/core/agents/events.go`）

- 新增 `EventTurn`（:45–:49）、`EventInteraction`（:50–:53），`Valid`（:63）、`NormalizedEvent` 字段（:332–:333）、`Validate` 映射与校验（:396–:421）。
- 新增 `TurnPayload`（:110，`ID/Phase/Outcome`）与 `Validate`（:118）；`InteractionPayload`（:165，`ID/Kind/Phase` + requested 时 exactly-one 载荷）与 `Validate`（:176）。
- `LifecycleEvent` 注释已改：session 级 state 仅作信息展示，轮次边界永走 `EventTurn`。

### 2.2 会话层（`internal/app/sessions/`）

- `turn.go`：`turnFromEvent` 只认 `EventTurn`（started→running，ended→idle）。
- `service.go:762`：`Stop` 会话级——运行中则经适配器 `Interrupt`，再标记本会话 stopped；不再调 `instances.Stop`，已删除 `markSiblingsDisconnected`。注意：门控用的是 `Interrupt` 能力，不是 `Stop`。
- `service_interactions.go:183`：`writeInteractionSnapshot`；`service.go:809` 的 `StreamSession` 在 replay 后、live 队列前写 `event: interactions` 快照帧。
- 守卫 `dependency_guard_test.go:72`：`TestSessionsDoesNotNameNativeTurnEvents`（9 个原生名禁令）。
- 测试：`turn_test.go` 重写为 turn 事件驱动 + “error 单独不改 turn”；`service_test.go` 的 Stop 改为三条新语义测试；`stream_test.go` 新增快照有/无两条。`go test ./internal/app/sessions/` 通过。

### 2.3 opencode 适配器（`internal/providers/opencode/adapter.go`）

- `instance` 新增 `turns` / `turnSeq`；`turnStarted` / `turnEnded`（孤立 end 返回空，无操作）/ `interactionEvent` / `resolvedInteraction`。
- 映射：`session.execution.started/succeeded/interrupted`→turn；`failed`→先 turn 结束（failed）后 error；`permission.asked/replied`、`form.created/replied/cancelled`→interaction（无 id 的 resolved 记 `unknown`）。执行类事件已移出通用 lifecycle 分支。
- `adapter_test.go` 更新 `TestAdapterSubscribeMapsNativeEvents` 并新增 `TestAdapterSubscribeMapsTurnsAndInteractions`。`go test ./internal/providers/opencode/` 通过。

### 2.4 pi 适配器（`internal/providers/pi/`）

- `event_parser.go`：parser 持 turn 状态（+`turnMu`，`beginTurn/endTurn/markAbort/nextTurnID`）；`agent_start` 开 turn，`agent_end/agent_settled` 闭 turn（第二个到达者无操作）；`agent_*` 移出 `lifecycleTypes`。解析器仅用于实时流，历史回放走独立 `normalizeHistoryRecord`，故状态安全。
- `adapter.go`：`instance.nextTurnID`（实例内唯一）；CLI 模式 `runProcess` 中断分支发 turn-cancelled，异常退出先发 turn-failed 再发 `process_exit`；RPC 解析器接入同一发号器。
- `rpc_child.go`：`handleExtensionUI` 发 requested，`ReplyPermission` 后发 resolved，`kill` 时 drain pending 并逐个发 resolved；`observe` 改消费 `EventTurn`；`abort` 先 `markAbort`（空闲时无效）。
- `permissions.go`：`ReplyPermission` 成功后发 resolved。`rpc_extension_ui.go`：`clear` 改为 `drain`（返回 id 供 settled 上报）。
- 测试：谓词批量替换为 turn 断言；`sleepingFakePi` 补 `agent_start`；裸解析测试补 `agent_start`；`TestRPCPermissions` 断言 requested/resolved；新增 `TestParserTurnBoundaries`、`TestRPCChildExitEndsOpenTurnAsFailed`。`go test ./internal/providers/pi/` 通过。

### 2.5 前端（`web/src/`）

- `api/sessions.ts`：`NormalizedEventType` 加 `turn/interaction`；新增 `TurnPayload`、`InteractionPayload`、`InteractionSnapshot`；`NormalizedEvent` 加 `turn/interaction` 字段。
- `stores/sessions.ts`：`applyLifecycle` 删轮次字符串判断；新增 `applyTurn` / `applyInteraction` / `applyInteractionSnapshot` 委托；`stopSession` 改会话级（不再标记兄弟 disconnected）。
- `features/chat/execution/SessionEventStream.ts`：注册 `turn` / `interaction` / 快照 `interactions` 三类帧；删除按 `execution_failed/process_exit` 置 idle 的兜底；`StreamStore` 加三个方法。
- `stores/interactions.ts`：重写为纯流驱动（`applySnapshot` 替换、`applyInteraction` 幂等折叠、保留 reply + expired 处理）；删除 raw 订阅/轮询/`loadApprovals`。
- `features/chat/interactions/useSessionInteractions.ts`：删轮询与 `isRunning` 选项；`SessionChatPanel.vue`：删 `loadApprovals/subscribeRaw/startApprovalPoll/unsubscribeRaw` 调用，open 只走 `openSession`（快照随流到达）。
- `api/interactions.ts` 的 `rawEventsPath` 与后端 raw 端点**保留**（调试通道），前端交互不再使用。
- 测试：`interactions.spec.ts` 重写；`session-chat-panel.spec.ts` 的 `FakeEventSource` 支持 `dispatch`，审批三测试改为流驱动；`sessions-store.spec.ts`（turn 结束/失败、error 不改 turn、中断事件改 cancelled）、`workbench.spec.ts`（stop 只影响目标）。`npx tsc -p tsconfig.typecheck.json` 零错误；`npx vitest run` 34 文件 / 396 项通过。

## 3. 未完成项的执行结果（已完成）

### 3.1 `docs/api/openapi.yaml`（已完成）

- `/projects/{id}/sessions/{sessionId}/stop`：改为会话级描述；门控 `interrupt`；409 为 `unsupported_capability`；502 为中断失败且会话保持原状。
- `/projects/{id}/sessions/{sessionId}/events`：补充 `turn` / `interaction` 语义与 `interactions` 快照帧（回放之后、实时之前，无 id，仅 tool confirmation 后端写入）；200 响应描述同步 `resync` / `interactions` 两类无 id 帧。
- `AgentCapabilities`：`stop` 门控实例停止；`interrupt` 门控会话 interrupt 与 stop。
- `AgentSession.turn`：来源改为会话自身的 `turn` 事件。
- `AgentSessionHistory`：历史只含 transcript，`turn` / `interaction` 仅实时流出现。
- YAML 解析通过（临时 Go 程序校验，已删除）。

### 3.2 架构与兼容文档（已完成）

- `docs/architecture/backend-boundaries.md`：契约机制新增 turn / interaction / 快照帧；禁止事项新增原生名守卫与会话级 stop 语义；剩余耦合表新增 `rawEvents` 调试通道偏差。
- `docs/agent-compatibility.md`：`stop` 行说明门控；新增“Turns, stop and interaction events”小节（两后端事件来源矩阵与规则）；修正过期的 opencode `toolConfirmation` 为 ✅（代码已声明 `ToolConfirmation: true`）。
- `docs/architecture/claudian-refactor-plan.md`：第 6 节新增本次落地与偏差记录。

### 3.3 验证（已完成）

1. `go vet ./...`：通过。
2. `go test ./...`：全部 ok。
3. `make verify`：退出码 0；前端 34 文件 / 396 项通过。
4. `make e2e`：两次全量运行，失败集合均为基线 7 项（layout / plantuml / session-history×2 / sessions / agent-service）。第一次另有 `agent-failures` 失败，第二次通过，单独运行 `agent-failures` 与 `sessions` 均通过，判定为共享服务器下并行时序 flake。`agent-service` 停止共享 opencode 服务是其诱因之一。
5. `web/e2e/fixtures/bin/pi` 的 `agent_start` / `agent_end` 与新的 turn 语义兼容，未改动。

### 3.4 提交

- 待提交：29 个代码/测试文件 + 文档 4 个 + 本交接文档。

## 4. 关键语义约定（勿改错）

- 孤立 end（无 open turn）= 无操作；`agent_end` + `agent_settled` 只闭合一次。
- 失败顺序：先 turn-end（failed），后 error；error 单独不改 turn。
- `markAbort` 仅在有 open run 时生效；空闲 abort 不污染下一轮。
- 会话 `Stop` 门控 `Interrupt`；实例停止仍走 `instances.Stop`（门控 `Stop`）。
- 快照帧名 `interactions`（复数），事件名 `interaction`（单数）；resolved 只带 id。
- `useSessionInteractions.ts` 的悬垂 tool_call 回退逻辑保留：有 `toolConfirmation` 的后端不渲染悬垂控制。
