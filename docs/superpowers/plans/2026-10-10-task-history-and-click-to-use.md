# 任务历史回放与「点击即用」实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 打开任务即可继续对话，并立即看到该会话的历史消息；平台重启后不再需要用户手动启动实例。

**Architecture:** adapter 契约新增一个与实例无关的可选历史读接口 `agents.HistoryReader`（能力位 `sessionHistory`），pi 读自己的 JSONL transcript、opencode 调 `GET /api/session/{id}/message`，两端都翻译成与实时流同一套 `NormalizedEvent`。会话服务把它盖章成平台信封并暴露 `GET …/sessions/{id}/history`；前端 store 用有序的 `openSession`（continue → history → subscribe）保证历史先于实时落地。`POST …/continue` 的语义改为「缺实例则自动启动并重绑会话」，从而消除「去项目设置手动启动」的死路。

**Tech Stack:** Go 1.27（gin、gorm）、Vue 3 + Pinia + Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-10-10-task-history-and-click-to-use-design.md`

## Global Constraints

- 历史权威在 agent 原生存储；平台只读、不复制、不建索引。
- 本批次**不改数据库表结构**：保留 `agent_instances` 与 `agent_sessions.instance_id`。
- `agents.ListHistoryRequest` 无实例入参；adapter 返回的历史事件 `instanceId` 为空且不调用 `Validate()`；`instanceId`/`sessionId` 由会话服务层盖章后再校验。
- `Capabilities.sessionHistory` 为 bool；pi 与 opencode 均声明 `true`。
- 单会话历史上限 `agents.MaxHistoryEvents = 5000`；超出时保留**最近** 5000 条并将响应 `truncated` 置 `true`。
- 回放事件带 `replayed: true`；跳过 pi 的 `system` 与 `thinking`、opencode 的 `reasoning`/`idle`/`model-switched`。
- 打开任务的时序固定为 `continue → loadHistory → subscribe`，由 store 的 `openSession` 保证，调用方不得自行拼顺序。
- pi 的历史文件解析复用 `sessionDirName` 与 `--session-dir` 覆盖语义，与 `ListSessions` 的读写一致。
- 模型/能力/错误文案沿用现有约定；所有新增错误文案走 `describeApiError`。
- `docs/api/openapi.yaml` 与 `docs/agent-compatibility.md` 与代码同批提交。

## Review Focus

1. 巨型 transcript（超过 5000 条事件、数 MB）：任务页仍能打开，只显示最近 5000 条并给出提示，不卡死、不 OOM。
2. 历史文件损坏、权限不足或末行 JSON 被截断：历史加载显示可重试的错误，实时流照常订阅，任务仍可发送。
3. 未安装 pi / opencode 时打开任务：给出具体原因（未安装/启动失败），不假装历史为空，不无限转圈。
4. 平台重启后打开旧任务：自动起实例且**不产生重复任务行**，历史与后续消息归属同一个平台会话 id。
5. 项目目录被移动或重命名（原生会话路径失效）：历史读取失败有明确文案，且不影响发送新消息。
6. opencode 服务端不可达：history 返回 502 而非 500，且错误文本不泄露服务端密码（沿用现有 redact）。

---

### Task 1: 契约与事件骨架

**Files:**
- Modify: `internal/agents/contracts.go`
- Modify: `internal/agents/events.go`
- Test: `internal/agents/contracts_test.go`

**Interfaces:**
- Consumes: 无。
- Produces:
  - `agents.ListHistoryRequest{ Directory string; SessionID string }`（json `directory`/`sessionId`）
  - `agents.ListHistoryRequest.Validate() error` → `agents.ErrInvalidHistoryRequest`
  - `agents.HistoryReader interface { ListHistory(ctx context.Context, request ListHistoryRequest) ([]NormalizedEvent, error) }`
  - `agents.Capabilities.SessionHistory bool`（json `sessionHistory`）
  - `agents.NormalizedEvent.Replayed bool`（json `replayed,omitempty`）
  - `agents.MaxHistoryEvents = 5000`（const）
  - `agents.ErrInvalidHistoryRequest`（sentinel）

- [ ] **Step 1: 写失败测试**

加到 `internal/agents/contracts_test.go`：

```go
func TestListHistoryRequestValidate(t *testing.T) {
    // 空 Directory → ErrInvalidHistoryRequest；空 SessionID → ErrInvalidHistoryRequest
    // 两者齐备 → nil
}

func TestCapabilitiesSessionHistoryDefaultsFalse(t *testing.T) {
    var c Capabilities
    if c.SessionHistory { t.Fatal("zero value must declare nothing") }
}

func TestNormalizedEventReplayedRoundTrip(t *testing.T) {
    // 构造 Replayed=true 的 message 事件，json.Marshal →
    // 断言含 `"replayed":true`；Replayed=false 时 marshal 结果不含 `replayed`
}

func TestHistoryReaderIsOptional(t *testing.T) {
    // 一个只实现 AgentAdapter 的 stub 必须能通过
    // `_, ok := any(stubAdapter{}).(HistoryReader)` 且 ok == false
}
```

- [ ] **Step 2: 运行并确认失败**

Run: `go test ./internal/agents/ -run 'History|SessionHistory|Replayed' -v`
Expected: FAIL（`ListHistoryRequest` 未定义等编译错误）

- [ ] **Step 3: 实现**

`internal/agents/contracts.go`：新增 `ErrInvalidHistoryRequest`（放进已有 `var (...)` 块）、`ListHistoryRequest` 及其 `Validate()`、`HistoryReader`、`Capabilities.SessionHistory`。`Validate` 复用 `SessionRef` 的报错风格，空字段用 `fmt.Errorf("%w: …")` 包裹。

`internal/agents/events.go`：`NormalizedEvent` 新增 `Replayed` 字段（放在 `Sequence` 之后，注释说明「历史回放事件为 true，实时流为 false」）；新增 `const MaxHistoryEvents = 5000`。

- [ ] **Step 4: 运行并确认通过**

Run: `go build ./... && go test ./internal/agents/ -v`
Expected: PASS（`HistoryReader` 是可选接口，因此 `stubAdapter`、`diagnosticsStub`、`fakeAdapter` 都无需改动）

- [ ] **Step 5: 提交**

```bash
git add internal/agents/contracts.go internal/agents/events.go internal/agents/contracts_test.go
git commit -m "feat(agents): 契约新增可选历史读接口与 sessionHistory 能力"
```

---

### Task 2: pi 历史读取

**Files:**
- Create: `internal/agents/pi/history.go`
- Create: `internal/agents/pi/testdata/transcript.jsonl`
- Test: `internal/agents/pi/history_test.go`

**Interfaces:**
- Consumes: Task 1 的 `agents.ListHistoryRequest`、`agents.NormalizedEvent`、`agents.MaxHistoryEvents`；已有 `sessionDirName(directory string) string`、`Adapter.sessionDir`。
- Produces: `func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) ([]agents.NormalizedEvent, error)`

**夹具要求**：`testdata/transcript.jsonl` 按实机 pi v3 的**结构**合成（不得提交真实对话原文），必须包含：`session` 头（`id` 与请求的 `SessionID` 一致、`cwd` 与请求目录一致）、一条 `system`、一条 `user`（`content:[{type:text}]`）、一条 `assistant`（`content:[{type:thinking},{type:text},{type:toolCall,id,name,arguments}]`）、一条 `toolResult`（消息级 `toolCallId`/`toolName`/`isError:true`，`content:[{type:text}]`）、一条 `model_change`。

- [ ] **Step 1: 写失败测试**

`internal/agents/pi/history_test.go`：

```go
func TestListHistoryMapsPiTranscript(t *testing.T) {
    // a := New(Options{Executable:"pi", SessionDir: t.TempDir()})
    // 把夹具写到 <SessionDir>/<sessionDirName(dir)>/<ts>_<sessionid>.jsonl
    // events, err := a.ListHistory(ctx, agents.ListHistoryRequest{Directory: dir, SessionID: id})
    // 断言顺序与内容：
    //   [0] message  role=user      text=夹具用户文本        Replayed=true
    //   [1] message  role=assistant text=夹具助手文本        Replayed=true
    //   [2] tool_call  callId=夹具 toolCall.id, name=夹具 name, input=夹具 arguments
    //   [3] tool_result callId=该 toolCallId, isError=true, output=夹具输出
    // 断言无 system/thinking 事件；断言每条 InstanceID=="" 且 Backend==agents.BackendPi
    // 断言 Sequence 为 1..N 连续
}

func TestListHistoryMissingSessionFile(t *testing.T) {
    // 目录存在但无匹配文件 → 返回空切片与 nil 错误（与 ListSessions 对不存在目录的处理一致）
}

func TestListHistoryTruncatesToMostRecent(t *testing.T) {
    // 用 >maxHistoryEvents 条记录生成夹具（循环写 message 行）
    // 断言 len(events)==agents.MaxHistoryEvents 且 events[0] 是「最近 N 条」的首条
}

func TestListHistoryRejectsEmptyRequest(t *testing.T) {
    // 空 Directory / 空 SessionID → 错误（经 agents.ListHistoryRequest.Validate）
}
```

- [ ] **Step 2: 运行并确认失败**

Run: `go test ./internal/agents/pi/ -run ListHistory -v`
Expected: FAIL（`a.ListHistory` 未定义）

- [ ] **Step 3: 实现 `internal/agents/pi/history.go`**

要点（签名与测试已定，以下是不由测试决定的选择）：
- 文件定位：`filepath.Join(a.sessionDir, sessionDirName(dir))` 下逐个读 `.jsonl` 头部，取头部 `id == request.SessionID` 的文件；`a.sessionDir == ""` 时返回错误。
- 用 `bufio.Scanner` 逐行 `json.Unmarshal`；**末行解析失败即停止**（写盘中的截断行），已解析的事件照常返回，不报错。
- 每行按 `type` 分派：`message` 取 `message.role` 与 `message.content`（`content` 可能是 string 或块数组，两种都要处理）；其他类型跳过。
- 归一化：`user`/`assistant` 的 `text` 块 → `EventMessage`；`assistant` 的 `toolCall` 块 → `EventToolCall`（`Input` 为该块的 `arguments`，用 `json.RawMessage`）；`toolResult` → `EventToolResult`（`CallID` 取消息级 `toolCallId`，`Output` 为其 `text` 块拼接，`IsError` 取消息级 `isError`）。
- `Timestamp` 取该行的 `timestamp`（ISO8601）；缺失时用零值并在截断前补 `time.Now().UTC()`（保持 `Validate` 可用）。统一 `Replayed: true`、`Backend: agents.BackendPi`、`Sequence` 从 1 递增。
- 截断：若事件数超过 `agents.MaxHistoryEvents`，保留最后 N 条并把 `Sequence` 重排为 `1..N`。

- [ ] **Step 4: 运行并确认通过**

Run: `go test ./internal/agents/pi/ -v`
Expected: PASS（含既有测试）

- [ ] **Step 5: 提交**

```bash
git add internal/agents/pi/history.go internal/agents/pi/history_test.go internal/agents/pi/testdata/transcript.jsonl
git commit -m "feat(pi): 从原生 JSONL 读取会话历史"
```

---

### Task 3: opencode 历史读取

**Files:**
- Modify: `internal/agents/opencode/client.go`
- Modify: `internal/agents/opencode/adapter.go`
- Create: `internal/agents/opencode/testdata/messages.json`
- Test: `internal/agents/opencode/client_test.go`, `internal/agents/opencode/adapter_test.go`

**Interfaces:**
- Consumes: Task 1 的 `agents.ListHistoryRequest`；已有 `Client.do`、`instance.client`、`a.instances`。
- Produces:
  - `Client.ListMessages(ctx context.Context, sessionID string) ([]Message, error)`
  - `opencode.Message`（`ID string`、`Type string`、`Text string`、`Content []ContentBlock`、`Time …`）
  - `opencode.ContentBlock`（`Type`、`Text`、`ID`、`Name`、`State *ToolState`）
  - `opencode.ToolState`（`Status string`、`Input json.RawMessage`、`Output string`）
  - `func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) ([]agents.NormalizedEvent, error)`

**夹具要求**：`testdata/messages.json` 按实机 `GET /api/session/{id}/message` 的**结构**合成，必须包含 `user`（顶层 `text`）、`assistant`（`content` 含 `text`/`reasoning`/`tool`，`tool.state` 含 `input`/`output`/`status`）、`idle`、`model-switched` 各至少一条。

- [ ] **Step 1: 写失败测试**

`client_test.go`：

```go
func TestClientListMessages(t *testing.T) {
    // httptest server 断言 method=GET 且 path == "/api/session/ses_1/message"
    // 返回夹具 JSON → 断言解析出的 Message 数量与 type 分布
}
```

`adapter_test.go`：

```go
func TestListHistoryMapsOpenCodeMessages(t *testing.T) {
    // 用已有 mock server 模式：注册 /api/session/ses_1/message 返回夹具
    // 先 StartInstance 让 adapter 有可用 client
    // events, err := a.ListHistory(ctx, agents.ListHistoryRequest{Directory: dir, SessionID: "ses_1"})
    // 断言：user 文本 → message(user)；assistant text → message(assistant)；
    //       tool 块 → 相邻的 tool_call(callId=tool.id, name, input=state.input)
    //       与 tool_result(callId 相同, output=state.output, isError=state.status!="completed")
    // 断言 reasoning/idle/model-switched 未产生事件
    // 断言 InstanceID=="" 且 Backend==agents.BackendOpenCodeV2，Replayed==true
}

func TestListHistoryWithoutRunningInstance(t *testing.T) {
    // 未 StartInstance → 返回错误，且错误文案含「start」/「not running」类可操作信息
}
```

- [ ] **Step 2: 运行并确认失败**

Run: `go test ./internal/agents/opencode/ -run ListHistory -v`
Expected: FAIL（`Client.ListMessages` / `Adapter.ListHistory` 未定义）

- [ ] **Step 3: 实现**

`client.go`：新增 `Message`/`ContentBlock`/`ToolState` 类型与 `ListMessages`，走既有 `do(ctx, http.MethodGet, "/api/session/"+sessionID+"/message", nil, nil, &payload)`，响应外层为 `{"data":[…]}`。

`adapter.go`：实现 `ListHistory`。选择 client 的规则（测试固定为「取最近创建的一个运行中实例」）：在 `a.instances` 中取一个 `running` 实例的 `client`；无则返回错误，文案提示调用方先启动后端服务。映射规则见 spec §4.1 表格；`state.status != "completed"` 视为 `IsError`。截断与 `Sequence` 重排同 Task 2。

- [ ] **Step 4: 运行并确认通过**

Run: `go test ./internal/agents/opencode/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/agents/opencode/client.go internal/agents/opencode/adapter.go internal/agents/opencode/client_test.go internal/agents/opencode/adapter_test.go internal/agents/opencode/testdata/messages.json
git commit -m "feat(opencode): 通过原生 message 接口读取会话历史"
```

---

### Task 4: 会话服务与历史 HTTP 接口

**Files:**
- Modify: `internal/sessions/service.go`
- Modify: `internal/sessions/handlers.go`
- Modify: `internal/sessions/service_test.go`（`fakeAdapter` 增加 `ListHistory`）
- Modify: `docs/api/openapi.yaml`

**Interfaces:**
- Consumes: Task 1 契约；Task 2/3 的 `ListHistory`；已有 `loadScoped`、`adapter(backend)`、`capabilities(backend)`、`respondServiceError`。
- Produces:
  - `sessions.History struct { Events []agents.NormalizedEvent `json:"events"`; Truncated bool `json:"truncated"` }`
  - `func (s *Service) History(ctx context.Context, projectID uint, sessionID string) (History, error)`
  - `func (s *Service) handleHistory(c *gin.Context)`，路由 `GET /projects/:id/sessions/:sessionId/history`
  - `fakeAdapter.ListHistory(ctx, req) ([]agents.NormalizedEvent, error)`（可脚本化，测试用）

- [ ] **Step 1: 写失败测试**

`service_test.go`：

```go
func TestHistoryStampsEnvelopeAndReturnsEvents(t *testing.T) {
    // fakeAdapter 配置返回两条事件（InstanceID 留空）
    // 调用 History → 断言每条 event.InstanceID == 会话行 instance_id、
    //   event.SessionID == 会话行的 native_session_id、Backend 正确、
    //   且每条都通过 Validate()
}

func TestHistoryUnsupportedCapability(t *testing.T) {
    // capabilities.SessionHistory=false 的 adapter → errors.Is(err, ErrUnsupportedCapability)
}

func TestHistoryBackendFailureIsReported(t *testing.T) {
    // fakeAdapter 返回 error → errors.Is(err, ErrBackendFailed)
}

func TestHistoryUnknownSessionIsNotFound(t *testing.T) {
    // 不存在的 sessionId → errors.Is(err, ErrSessionNotFound)
}

func TestHistoryTruncatedFlagPropagates(t *testing.T) {
    // fakeAdapter 返回恰好 MaxHistoryEvents 条 → 响应 Truncated 由 adapter 决定
    // （规定：Service 只透传，不二次截断）
}
```

HTTP 测试（沿用 `internal/sessions` 现有 gin 测试风格，放 `service_test.go` 或新增 `handlers_test.go`）：200 形状 `{"events":[...],"truncated":false}`；409 能力缺失；404 会话不存在；502 后端失败。

- [ ] **Step 2: 运行并确认失败**

Run: `go test ./internal/sessions/ -run History -v`
Expected: FAIL（`History` 未定义）

- [ ] **Step 3: 实现**

`service.go`：`History` 用 `loadScoped(projectID, sessionID)` 取行，校验 `capabilities(backend).SessionHistory`，取 adapter 并断言 `agents.HistoryReader`，用 `agents.ListHistoryRequest{Directory: inst.Directory, SessionID: row.NativeSessionID}` 调用（目录取自 `s.instances.Get(row.InstanceID).Directory`；第一步实例行从不删除，即使实例已停止/失败行仍带着目录，所以不需要回退路径）。对每条事件盖 `InstanceID=row.InstanceID`、`SessionID=row.NativeSessionID`、`Backend=row.Backend` 后调用 `Validate()`（失败即 `ErrBackendFailed`）。

`handlers.go`：注册路由，新增 `handleHistory`，错误映射沿用 `respondServiceError`（能力缺失→409、会话不存在→404、后端失败→502）。

`docs/api/openapi.yaml`：新增路径与 `AgentSessionHistory` schema（`events` 引用既有 `NormalizedEvent`、`truncated: boolean`），并给 `Capabilities` 增加 `sessionHistory`，给 `NormalizedEvent` 增加 `replayed`。同步更新 `internal/agents/contracts_test.go` 或 `docs` 测试中钉住 openapi 与代码一致性之处（若存在）。

- [ ] **Step 4: 运行并确认通过**

Run: `go test ./internal/sessions/ ./internal/agents/ ./internal/http/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/sessions/service.go internal/sessions/handlers.go internal/sessions/service_test.go docs/api/openapi.yaml
git commit -m "feat(sessions): 新增任务历史接口"
```

---

### Task 5: `continue` 自动确保后端并重绑会话

**Files:**
- Modify: `internal/sessions/service.go`（`Continue`）
- Modify: `internal/sessions/repository.go`（新增 `delete`）
- Modify: `internal/sessions/service_test.go`

**Interfaces:**
- Consumes: 已有 `instanceForBackend(ctx, projectID, backend)`、`upsertRef`/`indexRef`、`repo.update(row database.AgentSession) error`、`repo.getByNative(instanceID, nativeID) (database.AgentSession, error)`（未命中时返回 `ErrSessionNotFound`，用 `errors.Is` 分支）。
- Produces:
  - `Service.Continue` 的新语义（签名 `Continue(ctx context.Context, projectID uint, sessionID string) (Session, error)` 不变）
  - `func (r *repository) delete(id string) error`

- [ ] **Step 1: 写失败测试**

```go
func TestContinueStartsInstanceWhenNoneRunning(t *testing.T) {
    // 会话行指向已停止实例 → 调用 Continue
    // 断言：fakeAdapter.startCount() == 1；返回的 DTO instanceId 为新实例；
    //       DB 中该会话行的 instance_id 已更新
}

func TestContinueIsIdempotentWhenInstanceRuns(t *testing.T) {
    // 已运行 → startCount() == 0，返回同一 instanceId
}

func TestContinueRebindMergesDuplicateRowOnNewInstance(t *testing.T) {
    // 预置两行：用户打开的 A（旧实例）与同一 native id 在新实例上的 B
    // 调用 Continue(A) → 断言 A 的 platform id 不变、instance_id 变为新实例、
    //  B 行被删除、会话总数减一
}

func TestContinueRejectsOtherProject(t *testing.T) {
    // 用项目 2 调项目 1 的会话 → ErrSessionNotFound
}
```

- [ ] **Step 2: 运行并确认失败**

Run: `go test ./internal/sessions/ -run Continue -v`
Expected: FAIL（当前实现对已停止实例返回 `ErrInstanceNotRunning`）

- [ ] **Step 3: 实现**

改写 `Continue`：取行（`loadScoped`）→ `instanceRunnable` 不满足时调用 `instanceForBackend` 启动 → 按 spec §4.3 的合并规则重绑：`repo.getByNative(新实例ID, row.NativeSessionID)` 命中时保留当前行的平台 id 与其 `instance_id` 更新（`repo.update`），并 `repo.delete` 另一行；命中不到（`errors.Is(err, ErrSessionNotFound)`）时只做 `repo.update` 改 `instance_id`。`title`/`created_at` 取更完整者。合并判定抽成与 `upsertRef` 共用的私有辅助函数，避免规则漂移 → 调 adapter `ContinueSession` → 返回 DTO。

- [ ] **Step 4: 运行并确认通过**

Run: `go test ./internal/sessions/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/sessions/service.go internal/sessions/service_test.go
git commit -m "feat(sessions): continue 在实例缺失时自动启动并重绑会话"
```

---

### Task 6: 前端 API 与 store

**Files:**
- Modify: `web/src/api/sessions.ts`
- Modify: `web/src/stores/sessions.ts`
- Test: `web/src/tests/sessions-store.spec.ts`, `web/src/tests/api-contracts.spec.ts`

**Interfaces:**
- Consumes: Task 4 的 `GET …/history` 响应形状。
- Produces（store 状态与动作）：
  - state 新增 `historyLoaded: Record<string, boolean>`、`historyTruncated: Record<string, boolean>`、`historyError: Record<string, string>`
  - `ChatEntry.kind` 新增 `'separator'`
  - `loadHistory(projectId: number, sessionId: string): Promise<void>`
  - `openSession(projectId: number, sessionId: string): Promise<void>`（顺序：`continueSession` → `loadHistory` → `activateSession`）
  - `api/sessions.ts`：`SessionHistory { events: NormalizedEvent[]; truncated: boolean }`、`NormalizedEvent.replayed?: boolean`、`getSessionHistory(projectId, sessionId)`

- [ ] **Step 1: 写失败测试**

`sessions-store.spec.ts`：

```ts
it('openSession 按 continue → history → subscribe 顺序调用')
it('loadHistory 填充条目并在历史后插入分隔线')
it('loadHistory 重复调用不重复拉取（historyLoaded 守卫）')
it('loadHistory 失败时记录 historyError 且仍可订阅')
it('truncated=true 时记录 historyTruncated')
it('历史末尾的 assistant 消息不会被随后同内容的实时事件重复追加')
```

顺序测试用假 API 记录调用序列数组断言，例如期望 `['continue','history','subscribe']`。

`api-contracts.spec.ts`：`getSessionHistory` 的 path/method 断言。

- [ ] **Step 2: 运行并确认失败**

Run: `cd web && npx vitest run src/tests/sessions-store.spec.ts src/tests/api-contracts.spec.ts`
Expected: FAIL

- [ ] **Step 3: 实现**

`sessions.ts`（API）：新增类型与 `getSessionHistory`，`NormalizedEvent` 增加 `replayed?: boolean`。

`sessions.ts`（store）：`loadHistory` 走 `getSessionHistory`，把 `event => ChatEntry` 复用现有 `handleEvent` 的映射分支（抽出一个 `entryFromEvent(sessionId, event): ChatEntry` 私有函数供两处共用），追加后压入 `{kind:'separator', text:'以上为历史记录'}`；`openSession` 依次 `await continueSession` → `await loadHistory` → `activateSession`；实时去重规则仅在 `historyLoaded[sessionId]` 为真时生效。

- [ ] **Step 4: 运行并确认通过**

Run: `cd web && npx vitest run`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add web/src/api/sessions.ts web/src/stores/sessions.ts web/src/tests/sessions-store.spec.ts web/src/tests/api-contracts.spec.ts
git commit -m "feat(web): 会话历史加载与有序打开流程"
```

---

### Task 7: 对话页改造

**Files:**
- Modify: `web/src/components/TaskChatPanel.vue`
- Modify: `web/src/components/MessageView.vue`
- Test: `web/src/tests/task-chat-panel.spec.ts`

**Interfaces:**
- Consumes: Task 6 的 `openSession`、`historyError`、`historyTruncated`；`ChatEntry.kind === 'separator'`。
- Produces: 无新导出（组件行为）。

- [ ] **Step 1: 写失败测试**

```ts
it('挂载时调用 openSession 而非 activateSession')
it('头部不渲染实例 UUID chip')
it('渲染历史分隔线与 truncated 提示')
it('历史加载失败时显示重试按钮，点击后重新 loadHistory')
it('实例启动中显示准备状态，失败时显示原因')
it('interactEnabled 不再依赖 agents.instances')
```

- [ ] **Step 2: 运行并确认失败**

Run: `cd web && npx vitest run src/tests/task-chat-panel.spec.ts`
Expected: FAIL

- [ ] **Step 3: 实现**

`TaskChatPanel.vue`：挂载调 `openSession`；删除实例 chip 与「实例已停止，请在项目设置中启动」提示，替换为基于会话 `status` 的准备中/失败行（含原因与重试）；`MessageView.vue` 增加 `separator` 渲染分支（一条居中分隔文案）。

- [ ] **Step 4: 运行并确认通过**

Run: `cd web && npx vitest run`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add web/src/components/TaskChatPanel.vue web/src/components/MessageView.vue web/src/tests/task-chat-panel.spec.ts
git commit -m "feat(web): 任务页展示历史并去除实例阻塞"
```

---

### Task 8: 项目级扫描、移除实例面板、诊断页停止服务

**Files:**
- Modify: `internal/sessions/service.go`（新增 `DiscoverProject`）
- Modify: `internal/sessions/handlers.go`（新增路由）
- Modify: `docs/api/openapi.yaml`
- Modify: `web/src/api/sessions.ts`、`web/src/stores/sessions.ts`
- Modify: `web/src/components/ProjectSettingsPanel.vue`、`web/src/components/BackendDiagnostics.vue`
- Delete: `web/src/components/AgentInstanceList.vue`
- Test: `internal/sessions/service_test.go`, `web/src/tests/project-settings-panel.spec.ts`, `web/src/tests/diagnostics.spec.ts`

**Interfaces:**
- Produces:
  - `Service.DiscoverProject(ctx, projectID uint, backend agents.BackendID) ([]Session, error)`
  - `GET /projects/:id/sessions/discover?backend={backend}`
  - `web/src/api/sessions.ts`：`discoverProjectSessions(projectId, backend)`
  - `BackendDiagnostics.vue` 新增 props：`instances: AgentInstance[]`，新增 emit：`stop: [instanceId: string]`

- [ ] **Step 1: 写失败后端测试**

```go
func TestDiscoverProjectUsesRunnableInstance(t *testing.T) {
    // 已有运行实例 → 调用它的 ListSessions，合并入索引，不重复
}
func TestDiscoverProjectStartsInstanceWhenNeeded(t *testing.T) {
    // 无运行实例 → startCount()==1，并返回发现结果
}
func TestDiscoverProjectRejectsUnknownBackend(t *testing.T) {
    // 未知 backend → ErrUnknownBackend
}
```

- [ ] **Step 2: 运行并确认失败**

Run: `go test ./internal/sessions/ -run DiscoverProject -v` → FAIL

- [ ] **Step 3: 实现后端**

`DiscoverProject` 用 `instanceForBackend` 取/起实例，复用既有 `Discover` 的合并逻辑；handler 解析 query `backend` 并映射错误；openapi 同步。

- [ ] **Step 4: 运行并确认通过**

Run: `go test ./internal/sessions/ -v` → PASS

- [ ] **Step 5: 写失败前端测试**

```ts
// project-settings-panel.spec.ts
it('项目设置页没有实例列表，只有「扫描发现原生会话」按钮')
it('扫描按钮调用 discoverProjectSessions(projectId, backend)')
// diagnostics.spec.ts
it('诊断页渲染实例列表并可触发 stop')
```

Run: `cd web && npx vitest run src/tests/project-settings-panel.spec.ts src/tests/diagnostics.spec.ts` → FAIL

- [ ] **Step 6: 实现前端并删除实例面板**

`ProjectSettingsPanel.vue`：移除 `AgentInstanceList` 与实例启停 UI，改为「扫描发现原生会话」（需先选/取后端，沿用现有 `selectedBackend`）；保留启动能力（自动启动已由 continue 覆盖，但后端手动启停仍在诊断页）。`BackendDiagnostics.vue`：新增实例列表与「停止服务」按钮。删除 `AgentInstanceList.vue` 与其引用/测试。

- [ ] **Step 7: 运行并确认通过**

Run: `cd web && npx vitest run` → PASS

- [ ] **Step 8: 提交**

```bash
git add -A internal/sessions web/src docs/api/openapi.yaml
git commit -m "feat: 扫描发现移到项目级，实例面板收进诊断页"
```

---

### Task 9: 端到端测试与文档同步

**Files:**
- Create: `web/e2e/task-history.spec.ts`
- Modify: `web/e2e/sessions.spec.ts`
- Modify: `docs/agent-compatibility.md`
- Test: `web/e2e/*`

**Interfaces:**
- Consumes: 前 8 个任务的完整链路。

- [ ] **Step 1: 写 e2e**

```ts
// task-history.spec.ts
test('刷新任务页后仍显示历史消息与分隔线')
test('打开已断开的任务会自动恢复并可直接发送')
```

- [ ] **Step 2: 运行并确认失败或暴露旧断言**

Run: `cd web && npm run test:e2e -- task-history`
Expected: 在实现前 FAIL

- [ ] **Step 3: 更新旧 e2e 中依赖实例 chip / 实例面板的断言**

`web/e2e/sessions.spec.ts` 等：把「实例列表 / 实例 chip / 去项目设置启动」相关步骤改为新的自动恢复流程。

- [ ] **Step 4: 全量验证**

Run: `go test ./... && cd web && npx vitest run && npm run test:e2e`
Expected: 全部 PASS

- [ ] **Step 5: 更新文档**

`docs/agent-compatibility.md`：在能力矩阵新增 `sessionHistory` 行（两端 ✅）；改写 §Session history 段落——不再是「平台不回放」，而是「打开任务时由 adapter 读取原生历史并归一化回放，平台仍不复制、不建索引」；补充两端的历史来源与跳过规则（pi 读 JSONL 跳过 system/thinking；opencode 调 `GET /api/session/{id}/message` 跳过 reasoning/idle/model-switched）；补充 `maxHistoryEvents = 5000` 的截断行为与 opencode 历史读取依赖服务端可用的前提。

- [ ] **Step 6: 提交**

```bash
git add web/e2e docs/agent-compatibility.md
git commit -m "test(e2e)+docs: 任务历史回放端到端覆盖与兼容性文档同步"
```

---

## 自检记录

- **规格覆盖**：spec §4.1→Task 1-3；§4.2→Task 4、6、7；§4.3→Task 5；§4.4→Task 8；§5 测试→各任务内联 + Task 9；§6 第二步明确不在本计划内。
- **Review Focus 落点**：1→Task 2/3 截断测试；2→Task 2 末行截断测试、Task 6 失败态测试；3→Task 7 启动失败态测试；4→Task 5 合并测试；5→Task 2/3 缺失文件测试 + Task 6 失败态；6→Task 3 无实例测试 + Task 4 502 测试。
- **类型一致性**：`ListHistoryRequest`/`HistoryReader`/`SessionHistory`/`Replayed`/`MaxHistoryEvents` 全计划同名；store 的 `openSession`/`loadHistory`/`historyLoaded`/`historyTruncated`/`historyError` 在 Task 6/7 一致。
- **已知待实现期决定**（非开放问题）：opencode `state.output` 的确切字段、`continue` 合并辅助函数的命名、`repository.delete` 的具体实现——均已在对应任务的实现步骤里授权构建者按测试夹具与既有代码风格定夺。所有跨任务引用的标识符（`ListHistoryRequest`/`HistoryReader`/`SessionHistory`/`Replayed`/`MaxHistoryEvents`/`History`/`openSession`/`loadHistory`/`historyLoaded`/`historyTruncated`/`historyError`/`repo.delete`）已逐个与现有代码核对存在性或在本计划内定义。
