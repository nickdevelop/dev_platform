# 任务历史回放与「点击即用」设计

日期：2026-10-10
状态：待审阅
关系：本文档取代 `2026-10-10-task-chat-redesign-design.md` 中「实例未运行则禁用输入并提示去项目设置启动」与「对话页头部展示实例 chip」两处设计。其余内容继续有效。

## 1. 背景与目标

两个已确认的问题，都来自代码与实机核查：

1. **任务打不开历史。** 消息日志只活在浏览器内存里：`web/src/stores/sessions.ts` 的 `messages` 唯一填充来源是实时 SSE 事件，`web/src/api/sessions.ts` 没有任何历史接口；平台侧 `internal/sessions/repository.go` 与 `internal/database/models.go` 明确只存 ID 映射，`AgentAdapter` 契约没有历史方法（`docs/agent-compatibility.md` §Session history 记录为刻意边界）。刷新或重开任务即空。
2. **任务不能直接使用。** `TaskChatPanel.vue` 在实例不运行时禁用输入并提示「实例已停止，请在项目设置中启动后再发送消息」；平台重启后适配器内存中的实例句柄消失，会话变 `disconnected`，用户必须先去项目设置手动启动。头部还展示实例 UUID chip（`实例：01a12479…`），对用户没有意义。

历史数据其实一直都在，缺的只是读出与送出：

- pi：`~/.pi/agent/sessions/<目录 slug>/<时间戳>_<会话id>.jsonl`，version 3。实机确认记录类型为 `session` / `model_change` / `thinking_level_change` / `message`；`message.message.role` 为 `system` / `user` / `assistant` / `toolResult`，其中 `user`、`assistant` 的 `content` 是块数组（`text`、`thinking`、`toolCall{id,name,arguments}`），`toolResult` 消息级带 `toolCallId`、`toolName`、`isError`。
- opencode：`GET /api/session/{id}/message`（实机 2.0.24 验证）返回 `{"data":[…]}`，消息 `type` 为 `user`（顶层 `text`）、`assistant`（`content` 为块数组：`text`、`reasoning`、`tool{id,name,state{input,output,status},time}`）、`idle`（回合结束标记，带 `outcome`）、`model-switched`（跳过）。

目标：**点击任务即可用，且立刻看到历史对话。**

非目标（本次不做）：跨任务检索历史；历史全文搜索；把历史复制进平台数据库；删除 adapter 契约中的实例概念（见 §6，第二步）。

## 2. 已确认的决策

- 历史权威仍在 agent 原生存储，平台只读、不复制、不建索引（沿用 `2026-10-08-ai-development-platform-design.md` 的边界）。
- 分两步：**本文档只实施第一步（体验层）**；第二步（底层去掉实例概念）记录在 §6，本次不动。
- 现有任务/实例数据不必保留，出现数据冲突时以「重建」为准（但第一步本身不含表结构变更）。
- 去掉对话页的实例 UUID chip 与项目设置里的实例列表面板。
- 「扫描发现原生会话」入口移到项目级；「停止 agent 服务」入口移到诊断页。

## 3. 第一步的范围

做：

1. adapter 新增**与实例无关的**历史读操作，pi 与 opencode 各自实现（§4.1）。
2. 新增会话历史 HTTP 接口与前端加载（§4.2）。
3. 打开任务时自动确保后端可用并重新挂接会话，取代「去项目设置启动」（§4.3）。
4. UI 收敛：删实例 chip、删实例面板、扫描移到项目级、停止服务移到诊断页（§4.4）。

不做：

- 不改数据库表结构（保留 `agent_instances` 与 `agent_sessions.instance_id`）。
- 不把 opencode 服务改成整机一份（仍为每项目一个 serve）。
- 不改 `POST …/sessions/{id}/stop` 的旧级联语义（前端已不使用；仅在第二步清理）。
- 不给历史做分页浏览 UI。

## 4. 设计

### 4.1 adapter 契约：历史读操作

`internal/agents/contracts.go` 新增：

```go
// ListHistoryRequest scopes a native transcript read. Deliberately
// instance-free so the read does not depend on a live instance handle.
type ListHistoryRequest struct {
    Directory string `json:"directory"` // 已授权的项目工作目录
    SessionID string `json:"sessionId"` // agent 原生会话 id
}

// AgentAdapter 新增
// ListHistory returns a session's prior turns as normalized events, oldest
// first. Requires Capabilities.SessionHistory.
ListHistory(ctx context.Context, request ListHistoryRequest) ([]NormalizedEvent, error)
```

`Capabilities` 新增 `SessionHistory bool`（json `sessionHistory`）；两个后端均为 `true`。

`NormalizedEvent` 新增 `Replayed bool`（json `replayed,omitempty`），标记该事件来自历史回放而非实时流。

**为什么签名里没有 instanceID**：第二步要删掉实例，历史读取只需要「目录 + 原生会话 id」。现在就按最终形状定义，第二步不必二次改动这个操作（对应 §2 的分步策略）。

**事件构造规则（与实时流保持同一套渲染语义）**：

| 来源 | 归一化事件 |
| --- | --- |
| pi `role=user` 的 `text` 块 | `message`（role=user, text, final=true） |
| pi `role=assistant` 的 `text` 块 | `message`（role=assistant, text, final=true） |
| pi `role=assistant` 的 `toolCall{id,name,arguments}` | `tool_call`（callId=id, name, input=arguments） |
| pi `role=toolResult`（content 的 text 拼接、`isError`、`toolCallId`） | `tool_result`（callId=toolCallId, output=文本, isError） |
| pi `role=system`、`thinking` 块、`model_change`、`thinking_level_change` | 跳过 |
| opencode `type=user` 的 `text` | `message`（role=user, final=true） |
| opencode `type=assistant` 的 `text` 块 | `message`（role=assistant, final=true） |
| opencode `type=assistant` 的 `tool` 块 | `tool_call`（callId=id, name, input=state.input），随后 `tool_result`（callId=id, output=state.output, isError=state.status≠completed） |
| opencode `reasoning` 块、`idle`、`model-switched` | 跳过 |

跳过 `thinking`/`reasoning` 是与实时流一致的有意选择：pi 的实时解析器丢弃 `thinking_*`（`internal/agents/pi/event_parser.go`），opencode 的实时归一化也不产出推理事件。这样「回放出来的历史」与「当时看到的实时流」外观一致，不产生多一层或少一层的割裂。

**顺序与规模**：`sequence` 在历史空间内按文件/响应顺序 1..N 递增，`replayed=true`。前端按到达顺序追加，不跨空间比较 `sequence`。单会话历史事件数上限 `maxHistoryEvents = 5000`，超出时**保留最近的 5000 条**并在响应上置 `truncated: true`（理由是 1.8 MB 的 pi transcript 属于常见量级，必须有界）。

**错误语义**：原生读取失败（文件不可读、服务不可达）返回带上下文的 error（`ErrBackendFailed` 包装）；能力未声明返回 `ErrUnsupportedCapability`；目录或会话 id 为空按契约校验错误处理。

**信封字段的填法**：`ListHistory` 的入参不含实例，adapter 因此不知道 `instanceId`。历史事件的 `instanceId`、`sessionId`、`backend` 由**会话服务层**在读出行后按会话行统一盖上（adapter 一律留空），保证 `NormalizedEvent` 信封始终合法、前端的实例/会话过滤逻辑可以直接复用。

**pi 的会话到文件解析**：复用 `ListSessions` 已有的扫描逻辑（`Directory` → 目录 slug → 在 `sessionDir` 下按 JSONL 头部 `id` 匹配 `SessionID`），`--session-dir` 覆盖行为与写入保持一致。

### 4.2 历史接口与前端加载

HTTP：

```
GET /api/projects/{projectId}/sessions/{sessionId}/history
→ 200 { "events": NormalizedEvent[], "truncated": bool }
```

- 会话鉴权与其它会话接口一致；`sessionId` 必须属于 `projectId`（沿用 `loadScoped`）。
- `404 session_not_found`；`409 unsupported_capability`；`502 agent_operation_failed`。
- 单独成接口而不塞进会话 DTO：历史可能很大，且它的失败不应影响会话元数据渲染。
- `docs/api/openapi.yaml` 同步新增该路径与 `SessionHistory` 能力字段、`replayed` 字段。

前端（`web/src/stores/sessions.ts` 新增）：

- `loadHistory(projectId, sessionId)`：调用上面的接口，把 `events` 转成 `ChatEntry` 追加进 `messages[sessionId]`，并用 `historyLoaded[sessionId]` 标记，重复调用不再拉取。
- 事件→条目的映射复用现有 `handleEvent` 的分支（同一套 `ChatEntry` 形状），因此历史与实时的气泡、工具卡片样式天然一致。
- **加载时序**：`continueSession`（§4.3）→ `loadHistory` → `subscribe`。
- **去重**：实时流可能重新投递历史尾部的同一条助手消息。规则：仅当该会话是「本次打开且刚从历史恢复」时，若 `message` 事件的 `(role, text)` 与日志中**最后一条同 role 条目**相同，则丢弃。用户自身回合沿用现有的乐观去重规则。
- 分隔线：历史条目之后插入一条 `kind: 'separator'` 的条目（「以上为历史记录」），其后为实时事件。
- 历史加载失败：消息区顶部显示一行错误与「重试」按钮，实时流照常订阅（历史不可得不应阻断继续对话）。
- `truncated=true` 时在分隔线上注明「仅显示最近 5000 条」；后端未声明 `SessionHistory` 时显示「该 agent 不支持历史回放」并直接订阅实时流。

**时序必须由 store 保证，不能靠调用方自觉。** 当前 `activateSession` 是同步订阅（`subscribe` 立即打开 EventSource）。若历史加载与订阅并发，实时事件会先于历史落盘渲染，历史补进日志后顺序就乱了。因此 store 新增一个**有序**的 `openSession(projectId, sessionId)` 动作，按 `continue → loadHistory → subscribe` 依次 await 执行；`TaskChatPanel.vue` 挂载时只调它，不再直接调 `activateSession`。同时删除 `interactEnabled` 对 `agents.instances` 的依赖。

**opencode 的顺序不是可选优化，是依赖**：其 `ListHistory` 需要一个可用的服务端（client 挂在实例上），所以必须先 `continue` 把实例拉起，再读历史。pi 的 `ListHistory` 只读磁盘文件，不依赖进程。

### 4.3 点击即用：自动确保后端可用并重新挂接

复用并**修改现有** `POST /api/projects/{id}/sessions/{sessionId}/continue` 的语义：它当前要求实例已在运行（`scopedInstance` + `instanceRunnable`，否则报 `ErrInstanceNotRunning`）。改为：

1. 解析项目工作目录与该会话的 backend；
2. 若该项目该 backend 没有运行中的实例，则调用 `StartInstance`（目录为项目路径）——与 `instanceForBackend` 同一套规则；
3. 把该会话行重新绑定到可用实例（更新 `agent_sessions.instance_id`）；
4. 调用 adapter `ContinueSession(ref)` 并返回最新会话 DTO。

**重绑必须处理唯一键冲突（否则每次平台重启都会冒出重复任务）。** `(instance_id, native_session_id)` 是唯一索引，而同一原生会话在新实例上被 `Discover`/`upsertRef` 看到时会**再建一行**。因此重绑规则为：先按 `(新 instance_id, native_session_id)` 查是否已存在行；

- 不存在 → 直接把当前行改绑到新实例；
- 已存在 → 保留**用户当前打开的**这一行（保持平台会话 id 稳定，避免前端被打开的任务突然变 id），删除另一行；两行的 `title`/`created_at` 取更完整的那个。

该合并逻辑要与 `internal/sessions/service.go` 的 `upsertRef`/`indexRef` 共用同一套判定，避免两处规则漂移。

前端在打开任务时先调用它。效果：

- 平台重启后打开旧任务，不再出现「实例已停止，请去项目设置启动」——自动起实例并挂回原会话。
- 用户点一下任务就能继续，不需要理解实例。
- `continue` 幂等：已可用的会话重复调用无副作用。

状态照旧持久化 `status`，但语义变为「打开时会自动修复的瞬时状态」，不再要求用户手工干预。`disconnected` 的任务打开后恢复 `active`。

**已知边界（记录，不修）**：若某会话正在另一个浏览器标签里跑一个回合，打开时历史与实时订阅之间可能漏掉该回合的中间事件。第一步不做缓冲/补投。

### 4.4 UI 收敛

- `TaskChatPanel.vue` 头部：删除 `实例：<uuid>` chip；保留 backend 标签与工作目录。
- 删除「实例已停止，请在项目设置中启动后再发送消息」提示，改为自动启动；启动中显示一行瞬时状态「正在准备 agent…」，失败则显示原因（未安装 / 启动失败）与「重试」。
- `ProjectSettingsPanel.vue`：移除实例列表（`AgentInstanceList.vue` 的使用点），新增「扫描发现原生会话」按钮（逻辑沿用 `discoverProjectInstanceSessions`，由后端选择当前可用实例，前端不再需要先选实例）。
- `BackendDiagnostics.vue`（诊断页）：新增运行中实例列表与「停止服务」动作，复用现有 `POST /api/projects/{id}/instances/{instanceId}/stop` 或实例服务的停止能力。诊断页取代原实例面板成为唯一能看到实例的地方。
- `NewTaskPanel.vue` 不变（上一轮已把实例选择删掉）。

## 5. 测试

后端（Go）：

- 契约：未声明 `SessionHistory` 的 adapter 返回 `ErrUnsupportedCapability`；`ListHistoryRequest` 校验。
- pi：以实机 JSONL 的**形状**手工构造 `testdata` 夹具（含 system、thinking、text、toolCall、toolResult、isError），断言映射结果与跳过规则；断言 `maxHistoryEvents` 截断保留最近 N 条。
- opencode：以实机 `/api/session/{id}/message` 响应的**形状**手工构造夹具（含 user / assistant 的 text+reasoning+tool / idle / model-switched），断言映射与跳过规则；夹具同时固定 `state.output` 等字段名，避免猜测。

两类夹具都不得直接提交真实会话内容（含用户对话与工具输出的原文）；按实机字段结构合成示例数据。
- 会话服务：`continue` 在实例缺失时自动启动并重绑 `instance_id`；已可用时不重复启动；跨项目/跨 backend 拒绝。
- HTTP：history 路径的成功、404、409、502，以及 openapi 与实现的字段一致性。
- `docs/agent-compatibility.md` 与 `Capabilities` 表的同步更新（该文档的能力矩阵由测试钉住）。

前端（Vitest）：

- `openSession`：严格的 `continue → history → subscribe` 调用顺序（用假 API 断言顺序，防止时序回归）。
- `loadHistory`：填充条目、`historyLoaded` 去重、分隔线插入、失败显示重试、`truncated` 提示、实时去重规则。
- `TaskChatPanel`：挂载时按 `continue → history → subscribe` 顺序调用；无实例 chip；自动启动文案与失败态。
- 项目设置：扫描按钮存在且提交正确的后端调用。
- 诊断页：实例列表与停止动作。

端到端（Playwright）：

- 刷新任务页后仍显示历史消息与分隔线。
- 打开一个 `disconnected` 的任务 → 自动恢复 → 可直接发送。
- 现有断言中依赖实例 chip / 实例面板的步骤同步更新。

## 6. 第二步（记录，本次不实施）

去掉「实例」这一领域概念，改为**后端运行时**：

- opencode：整机唯一一个 `opencode serve`，懒启动。实机验证：一个 serve 进程同时服务多个目录，`directory` 是逐请求 query 参数，事件流为全局 `/api/event` 且携带 `Location.Directory`。当前「每项目一个 serve」是浪费，也是实例概念的来源。
- pi：无常驻服务；每次发送在项目目录运行 `pi --mode json --session-id S -- <prompt>`，跑完即退。
- 任务 = `(project_id, backend, native_session_id)`；删 `agent_instances` 与 `agent_sessions.instance_id`，唯一键改为 `(project_id, backend, native_session_id)`。
- 契约把实例作用域方法替换为后端服务作用域：`StartInstance`/`StopInstance`/`Subscribe(instanceID)` 改为服务级启停与服务级事件流（opencode）或每进程流（pi）。
- 状态在读取时现算，不再持久化实例状态；平台重启不再产生任何「断开」概念。
- `internal/agents/instances` 包改造为「后端运行时管理器」。
- 清理 `POST …/stop` 的级联语义与前端 `stopSession`。

**风险（第一步已实测、第二步需再验证）**：整机一个 opencode serve 用哪个 cwd 启动。实测跨目录的会话列举/创建正常，但真实模型调用下 cwd 是否有副作用未验证（`docs/agent-compatibility.md` 中 opencode 相关条目本就是 provisional）。退路：按目录去重，即每项目一个 serve——用户体感不变，只是后端进程数不变。

## 7. 迁移与兼容

- 无数据库迁移；`openapi.yaml` 同步新增接口与字段。
- 已存在的会话可直接被新前端打开（历史 `loadHistory` + `continue` 自动挂接）。
- 本批次同时改 `continue` 的语义（从「要求已运行」到「缺则启动」），前端与后端同一提交内修改，不保留旧行为分支；无外部调用方（单用户内部部署）。

## 8. 开放问题

无待用户决策项。以下为实现期内由构建者按测试夹具固定的细节，已在此声明为「构建者的判断」：

- opencode 工具块 `state.output` / `state.status` 的确切取值形态，以实机夹具为准。
- `maxHistoryEvents = 5000` 的具体数值可在计划阶段按实机性能微调。
