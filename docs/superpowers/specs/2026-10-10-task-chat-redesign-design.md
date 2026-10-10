# 任务对话与新建任务重设计

日期：2026-10-10
状态：待审阅

## 1. 背景与目标

三栏布局（项目/任务树、右侧项目设置或任务对话）已上线。使用后发现以下问题：

1. 任务对话页没有按已确认的原型 B 实现。
2. 模型需要手动填写，应由 agent 提供可用模型列表。
3. 「继续会话」「停止」「运行中」含义混乱。应像 ChatGPT / DeepSeek Harness 一样：点击任务即可继续交互；一轮交互进行中显示「运行中」，等待下一条指令时不显示。
4. 新建任务只需选择 agent，模型在对话页选择。
5. 不同 agent 的功能可以不同，但交互风格必须一致。

目标：对话页与原型 B 一致；模型来自后端；「运行中」仅表示当前回合正在处理；交互风格跨 agent 统一。

非目标：项目设置页、资源区（Skill/MCP/提示词，仍为占位）、Chat 大类、会话重命名、会话列表发现（discover）的交互调整。

## 2. 已确认的决策

- 本轮允许修改后端代码与 API 契约；`docs/api/openapi.yaml` 同步更新。
- 模型列表由后端提供。
- 新建任务时，后端为所选 agent 选用本项目的运行中实例；若没有则自动启动。
- 停止语义为「只中断当前回合」。实例的启动与停止仍只在项目设置中操作。

## 3. 后端设计

### 3.1 能力声明

`agents.Capabilities` 新增字段：

- `interrupt`（bool）：adapter 能中断当前回合。opencode 与 pi 均声明为 true。

`ModelSelection` 保持原语义；为 true 的 agent 必须实现模型列表。

### 3.2 adapter 契约

`AgentAdapter` 新增两个方法：

- `ListModels(ctx, instanceID) ([]ModelInfo, error)`：仅当 `ModelSelection` 为 true 时可调用，否则返回 `unsupported_capability`。
- `Interrupt(ctx, ref SessionRef) error`：仅当 `Interrupt` 为 true 时可调用；中断该会话当前回合，不影响实例与其他会话。

`ModelInfo`：

```
{ "id": "provider/model", "name": string, "variants": []string, "provider": string }
```

`id` 与 `SendMessageRequest.Model` 的格式一致；有变体时使用 `provider/model#variant`。

实现：

- opencode：`ListModels` 调用实例服务端的 `GET /api/model`，按 `providerID + id` 组装，变体取 `variants`。`Interrupt` 调用 `POST /api/session/{id}/interrupt`。
- pi：`ListModels` 执行 `pi --list-models`（参数以数组传入，不拼接 shell 字符串），按表头解析 `provider` 与 `model` 列。`Interrupt` 终止该会话当前提示进程；需要在 adapter 中按会话记录进程句柄（现有代码只按实例取消）。

### 3.3 回合状态

会话服务为每个会话在内存中维护 `turn`：`idle` 或 `running`。

- opencode：`session.execution.started` → running；`session.execution.succeeded`、`session.execution.failed`（或 `session.step.failed`/`session.error` 导致的结束）、`session.execution.interrupted` → idle。
- pi：`agent_start` → running；`agent_settled`（或 `agent_end`）→ idle。

`turn` 不持久化。服务重启后会话本就会因实例不存在而断开，重启后默认为 idle。

会话 DTO 新增 `turn` 字段（`"idle" | "running"`）。`status` 字段保留，但前端不再用它表示「运行中」。

### 3.4 接口变更

| 接口 | 变更 |
| --- | --- |
| `GET /api/projects/{id}/instances/{instanceId}/models` | 新增。返回 `{ models: ModelInfo[] }`。不支持时 409 `unsupported_capability`；实例不存在 404 `instance_not_found`。 |
| `POST /api/projects/{id}/sessions/{sessionId}/interrupt` | 新增。需要 CSRF。回合不在运行中时视为成功（幂等），返回会话 DTO。不支持时 409 `unsupported_capability`。 |
| `POST /api/projects/{id}/sessions` | 请求体改为 `{ backend, title? }`。后端选择本项目中该 backend 的运行中实例；无则启动。`instanceId` 字段不再接受（同批次一起移除，前端同步修改）。 |
| 会话 DTO | 新增 `turn`。新增能力声明 `interrupt`。 |

`POST /api/projects/{id}/sessions` 的实例选择规则：

1. backend 不在项目中被禁用，否则 409 `unsupported_capability`（沿用现有 backend 禁用的错误码，若无则在计划阶段确定）。
2. 选取该项目该 backend 中状态为 running 的实例，取最近创建的一个。
3. 若无，调用 `StartInstance`，目录为项目路径。启动失败返回 502 `agent_operation_failed`。

`SendMessageRequest.Model` 保留。前端每次发送时携带当前选中的模型；不设会话级模型状态。

### 3.5 旧接口

- `POST /api/projects/{id}/sessions/{sessionId}/stop` 与 `continue` 保留 API（不破坏其他调用方），但前端不再使用。`stop` 的级联语义不变。
- 模型切换接口（opencode 的 `SwitchModel`）不对外暴露，仅保留 adapter 内部使用。

## 4. 前端设计

### 4.1 对话页（TaskChatPanel）

按原型 B：

- 头部：标题、后端标签、实例与工作目录（chips）、回合状态（仅 `turn=running` 时显示「运行中」）。删除「重命名」「继续」按钮。
- 消息区：所有 agent 共用同一气泡与工具调用样式。
- 底部输入卡片：左下为模型选择（来自 `GET .../models`，默认选第一项，选中项随发送携带），右下为发送按钮。
  - `turn=idle`：显示「发送」。
  - `turn=running`：发送按钮变为「停止」，调用 `interrupt`；输入框仍可输入，但发送需等待回合结束。
- 能力门控：模型选择仅在 `modelSelection` 为 true 时显示；工具确认卡片仅在 `toolConfirmation` 为 true 时显示；停止按钮在 `turn=running` 时总是显示（两个 agent 都声明 `interrupt`）。
- 实例未运行：输入区禁用并提示「实例已停止，请在项目设置中启动」（沿用上一轮修复）。

### 4.2 新建任务（NewTaskPanel）

仅一个选择：agent（来自 `GET /api/agent-backends`，并标注项目中是否启用）；可选标题。提交调用新的 `POST /sessions`，成功后跳转到新任务。删除实例选择、模型输入、「发现原生会话」按钮。

「发现原生会话」原本依赖实例选择。本次保留 `discover` 的 store 能力，但入口移到项目设置页的实例列表中（与实例管理放在一起）。

### 4.3 任务树（ProjectTree）

- 状态标签：`turn=running` 显示「运行中」，其余不显示。
- `status=disconnected` 显示「断开」；`stopped` 不再单独显示。
- 搜索与选中行为不变。

### 4.4 交互风格一致

所有 agent 使用同一套组件与布局；差异只通过能力门控体现（控件隐藏或禁用），不另写分支界面。

## 5. 错误处理

- 所有错误文案沿用 `describeApiError`。
- 发送失败：保留输入文本并在输入区上方显示错误。
- 中断失败：显示错误，回合状态保持由 SSE 决定，不做乐观更新。
- 模型列表加载失败：模型选择显示「模型列表加载失败」并禁用发送，不回退为手填。
- 自动启动实例失败：新建任务失败，显示 `agent_operation_failed` 对应文案，不创建会话。

## 6. 测试

后端（Go）：

- 能力与契约：`ListModels` / `Interrupt` 对未声明能力的 agent 返回 `unsupported_capability`。
- opencode：`ListModels` 映射（provider、变体）；`Interrupt` 请求路径；回合状态由 `session.execution.*` 事件推导。
- pi：`ListModels` 解析 `--list-models` 表格（用 fake 可执行文件）；`Interrupt` 只终止目标会话的进程；`agent_start`/`agent_settled` 推导回合状态。
- `POST /sessions`：复用运行中实例；无实例时启动；backend 禁用时拒绝。

前端（Vitest）：

- 对话页结构：头部、消息区、输入卡片；`turn` 切换显示与停止按钮；模型列表渲染与默认值；能力门控。
- 新建任务：仅 agent 选择；提交载荷不含 instanceId/model。
- 任务树：仅 `running` 显示运行中；`disconnected` 显示断开。

端到端（Playwright，`npm run test:e2e`）：

- 新建任务 → 发送 → 运行中 → 停止 → 空闲；点击树中任务可继续交互。
- 模型选择来自后端列表（fake agent 提供固定模型列表）。

e2e 中依赖旧界面的步骤（`createTask` 的实例下拉、`agent-failures` 与 `sessions` 中的「停止」级联断言）需同步更新。

## 7. 迁移与兼容

- 本次同时修改 `POST /sessions` 请求体。前端与后端在同一次提交中修改，不保留 `instanceId` 兼容分支。
- 无外部调用方（单用户内部部署）。

## 8. 开放问题

无。所有决策已在对话中确认：模型来自后端；新建任务由后端选用或启动实例；停止只中断当前回合。
