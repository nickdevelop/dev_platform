# Handoff — 每条消息显示所用模型（opencode-v2）

日期：2026-10-13
分支/工作区：`/data/projects/dev_platform`（未提交，改动都在工作区）
状态：**已完成（B/C/D 全部落地；与计划的唯一偏离：夹具的 model id 修正为不带 provider 前缀的 `acme-code`，原因见下）。**

## 目标

参照 opencode TUI/desktop：会话里**每条 assistant 消息**显示它当时使用的模型。

## 关键事实与设计决策（已验证）

- opencode v2 的会话消息记录本身带模型：`Session.Message.Assistant.model`（`Model.Ref = {id, providerID, variant}`，required 字段），另有 `model-switched` 消息。opencode 客户端就是读这个字段渲染的。
- 平台的 `agent_sessions` 表**只存索引**，不存 transcript（设计如此）；模型随消息走，平台侧**不需要**新表。
- 平台的实时流事件（`session.text.*` 等）**不带**模型；`Session.Info.model` 是会话当前模型。
- 因此实现策略：
  - **历史回放**：用每条 assistant 消息**自己的** model（权威、逐条准确）。
  - **实时流**：适配器记住会话当前模型（`Session.Info.model` + 每次 `SwitchModel` 刷新），给实时 assistant 消息打戳。会话内换模型时，下一次发送前平台会调 `SwitchModel`，所以后续消息戳正确。
- 模型 id 统一用平台的 `provider/model` 形式（与 `ModelInfo.ID` 一致）；`variant` 单独带。
- pi 后端**不在本次范围**：`MessagePayload.Model` 是 `omitempty`，pi 不填，前端自然不显示。

## 已完成（Go 后端）

`git diff --stat` 当前：`events.go`、`opencode/adapter.go`、`opencode/client.go`。`go build ./...` 通过（但 `gofmt` 未跑）。

1. `internal/agents/events.go`
   - `MessagePayload` 新增 `Model *MessageModel \`json:"model,omitempty"\``
   - 新增类型：
     ```go
     type MessageModel struct {
         ID       string `json:"id"`       // provider/model
         Provider string `json:"provider"`
         Variant  string `json:"variant,omitempty"`
     }
     ```
2. `internal/agents/opencode/client.go`
   - `SessionInfo` 新增 `Model *ModelRef`
   - `Message` 新增 `Model *ModelRef`
3. `internal/agents/opencode/adapter.go`
   - `instance` 新增 `sessionModels map[string]agents.MessageModel`，并在实例构造处初始化。
   - 新增 `setSessionModel` / `sessionModel` / `messageModel`（`messageModel` 把 `ModelRef` 转成 `agents.MessageModel`，id 拼成 `provider/id`）。
   - 在 `CreateSession`、`ContinueSession`、`ListSessions`、`SendMessage`（`SwitchModel` 成功后）写入会话模型。

## 未完成 → 完成记录

### A. 后端（normalize 与 history）— 已完成（handoff 记录后、本次之前已在工作区落地）

### B. 后端测试 — 已完成

- `TestListHistoryMapsOpenCodeMessages` 断言 `events[2].Message.Model` 为
  `{ID:"acme/acme-code", Provider:"acme", Variant:"default"}`，并补了「user 消息不带 model」的断言。
- 新增 `TestAdapterSubscribeStampsSessionModel`：create 响应带 `model`，2 条
  `session.text.delta/ended`，断言两条 `EventMessage` 戳 `prj/model-x`（Provider/Variant 正确）。
- `TestAdapterSendMessagePromptsAndSwitchesModel` 补验证：`SwitchModel` 后 normalize 一条
  delta，断言戳的是新模型 `prov/model#fast`。
- **偏离说明**：夹具 `testdata/messages.json` 的 model id 原写 `acme/acme-code`，而
  opencode v2 spec 规定 `Model.Ref.id` 只在 provider 内唯一（不带前缀），`messageModel`
  会再拼一次前缀，于是得到 `acme/acme/acme-code`。已把夹具修正为裸 id `acme-code`。

### C. 前端 — 已完成

- `web/src/api/sessions.ts`：`MessageModel` + `MessagePayload.model`。
- `web/src/stores/sessions.ts`：`ChatEntry.model`、`appendMessage` 可选参数（合并进仍打开的
  条目时 `model` 存在则覆盖）、`handleEvent` 与 `loadHistory` 均传 `event.message.model`。
- `web/src/components/MessageView.vue`：agent 气泡内 `.message-text` 后渲染
  `[data-test="message-model"]`，`modelLabel` 显示 `model.id`（有 variant 追加 `#variant`），
  user 消息永不显示；样式小号、muted 色、`margin-top: 0.25rem`。
- 测试：`message-view.spec.ts` 新增 per-message model describe（带/不带/variant/user 四例）；
  `sessions-store.spec.ts` 新增 per-message model describe（到达即带、不带、流式增量补戳、
  历史回放四例）。

### D. 收尾 — 已完成

- `gofmt -w` adapter.go、client.go、events.go（pi/adapter_test.go 按约定未动）。
- `docs/agent-compatibility.md` Session history 章节补了模型说明段落。
- e2e fixture `web/e2e/fixtures/bin/opencode`：`sessionInfo` 与 assistant `recordMessage`
  均加 `model:{id:"model-a",providerID:"mock"}`。
- 验证：`go vet ./... && go test ./...` ✓；`npx vitest run`（305 通过）+ `tsc` ✓；
  `make verify` ✓；`npm run test:e2e`（17 通过）✓。

## 提交计划

1. `feat(opencode):` 后端归一化 + 测试 + 兼容文档
2. `feat(web):` 前端展示 + 测试 + e2e fixture
3. `docs:` 本 handoff 文档

## 相关既有实现（背景，勿改）

- 输入框模型选择器已按 opencode 桌面版改为「紧凑按钮 + 可搜索浮层」，并按会话记忆选择：
  `web/src/components/ModelSelect.vue`、`web/src/stores/sessions.ts` 的 `selectedModels`/`modelFor`/`setModel`、
  `web/src/components/TaskChatPanel.vue`。本次的「每条消息模型」是独立特性，不与该会话级选择冲突。
- 历史回放跳过规则：pi 跳过 `system`/`thinking`，opencode 跳过 `reasoning`/`idle`/`model-switched`。
