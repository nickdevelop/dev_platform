# 后端边界：通用层与异构层

本文说明平台如何把"与具体 agent 无关的部分"（同构层）和"必须因后端而异的部分"（异构层）分开，并给出新增后端的步骤。规则由 `internal/app/sessions/dependency_guard_test.go` 中的两条守卫测试与 `internal/core/platform/guard_test.go` 的 OS 中立守卫执行。

## 分层

| 层 | 包 | 职责 | 能否出现后端 ID 或后端包 |
| --- | --- | --- | --- |
| 同构：平台配置 | `internal/app/settings`、`internal/app/database` | 外观、后端的启用与可执行路径 | 否，只读 `agents.Catalog` |
| 同构：项目管理 | `internal/app/projects`、`internal/app/paths` | 项目记录、目录的安全生命周期 | 否，不知道 agent 的存在 |
| 同构：会话与实例编排 | `internal/app/sessions`（子包 `store` 会话索引、`turns` 轮次跟踪）、`internal/app/instances`、`internal/transport/http` | 会话索引、轮次状态、实例生命周期、SSE、路由 | 否，只依赖 `internal/core/agents` 契约 |
| 同构：机器级 agent 服务 | `internal/app/agentservices` | 服务的探测、运维员触发的启停、启动门控 | 否，只依赖 `internal/core/agents` 契约与适配器注册表 |
| 同构：纯 Chat | `internal/app/chat` | 全局、非 agent 的模型对话：对话/消息持久化、模型调用、SSE、标题 | 否，只依赖 `modelconfig` 的模型解析与 `core/textutil`；不得 import agent 契约、providers、projects、instances、sessions |
| 契约（core） | `internal/core/agents`（`contracts.go`、`extensions.go`、`descriptor.go`、`registry.go`、`service.go`） | 能力声明、标准事件、扩展接口、后端描述符 | 仅在 `BackendID` 常量定义处 |
| OS 抽象（core） | `internal/core/platform`（接口 + `unix/windows/fake/policy`） | OS 差异端口与平台无关策略 | 否 |
| 异构：适配器（providers） | `internal/providers/opencode`、`internal/providers/pi`、`internal/providers/deepseek` | 与具体 CLI 或服务协议打交道，导出 `Descriptor` | 是（本包即为该后端） |
| 组合根 | `cmd/platform/main.go` | 注册适配器、构建 catalog、校验两者一致 | 是 |
| 异构：前端展示 | `web/src/stores/agents.ts` 与各组件 | 从后端列表读取标签、排序、能力 | 否（组件内不写后端 ID） |

上表对应 `docs/architecture/claudian-refactor-plan.md` 中的 claudian 五层映射：契约与 OS 抽象在 `core/`，领域服务在 `app/`，异构适配器在 `providers/`，路由编排归 `transport/`。

## 契约层提供的通用机制

- **能力声明 `Capabilities`**：`AgentAdapter` 的一部分，每个布尔值都必须显式声明。通用层用能力门控控件，不用后端 ID 分支。
- **类型化轮次 `EventTurn`**：轮次边界（`started` / `ended`，结果为 `completed` / `cancelled` / `failed`）是会话忙碌状态的唯一来源。`lifecycle` 的状态名只用于展示，不改变轮次。被用户中断或停止的轮次结束为 `cancelled`，不是 `failed`；失败轮次之后必有一个 `error` 事件说明原因。
- **交互事件 `EventInteraction`**：工具确认（`permission`）与问题表单（`form`）的请求与解决（`requested` 携带对应载荷，`resolved` 只带 id）。它是待处理交互的唯一通道，前端不再轮询，也不订阅原始事件。
- **会话交互快照**：会话事件流在回放之后、实时事件之前写入一帧 `interactions`（无 id），内容为该会话当前待处理的权限与表单，客户端据此整体替换待处理集合。仅当后端声明 `toolConfirmation` 时写入。
- **可选扩展接口**：`agents.PermissionReplier`（工具确认与表单）、`agents.RawEventOpener`（原始事件流）、`agents.HistoryReader`（历史）、`agents.ServiceController`（机器级服务）。适配器实现才可用，未实现时通用层返回 `ErrUnsupportedCapability`。
- **后端描述符 `BackendDescriptor` 与 `Catalog`**：每个适配器包导出 `Descriptor`，包含显示名、可执行文件名、默认启用、展示顺序、测试版本范围、是否有机器级服务、可选的传输模式列表（如 pi 的 `cli`/`rpc`），以及可选的标签色调或品牌色（`Tone` / `Color`）。`Color` 必须是 `#RRGGBB` 并在 UI 中优先于 `Tone`，因此新增后端仍然只需描述符声明，前端不改代码。组合根用这些描述符构建 catalog。
- **注册表 `Registry`**：适配器的显式注册点。组合根在启动时调用 `checkBackendWiring`，确认注册表与 catalog 描述的后端集合完全一致，否则启动失败。

## 新增一个后端的步骤

1. 在 `internal/core/agents/contracts.go` 为 `BackendID` 增加常量，并把它加入 `BackendID.Valid()`。这是唯一需要改动契约包的地方。
2. 新建适配器包 `internal/providers/<name>`，实现 `agents.AgentAdapter`（包括 `Capabilities()`）。按需实现 `PermissionReplier`、`RawEventOpener`、`HistoryReader`、`ServiceController`，并与 `Capabilities` 中对应的标志保持一致。
3. 在该包导出 `Descriptor`，并写 `descriptor_test.go` 钉住版本范围与 `ServiceSupported`。
4. 在 `cmd/platform/main.go` 中：把 `Descriptor` 加入 `agents.NewCatalog(...)`，并在注册表中注册适配器。`checkBackendWiring` 会检查两者是否一致。
5. 在 `docs/agent-compatibility.md` 增加该后端的能力矩阵与版本表。
6. 前端无需改动：标签、排序、服务控件、资源页的后端复选框都由后端列表驱动。

## 禁止事项

- 通用层（`internal/app/sessions`、`internal/app/settings`、`internal/transport/http`）不得 import 具体后端包，也不得出现后端 ID 字面量。守卫测试会检查。
- 不得用后端 ID 做功能分支。需要的能力必须在 `Capabilities` 中声明，或通过可选扩展接口表达。
- 不得在通用代码里补默认值。未知后端必须显式报错或返回 `none`，不得回退到某个已知后端。
- 组件不得写死后端名称或服务名称。需要展示的事实从 `agents` store（`backendLabel`、`orderedBackends`、`serviceBackends`、`backendFor`）读取。
- 只有组合根与适配器包能决定某个后端"存在"。业务路径不得启动、停止或替换 agent 的机器级服务（见 `docs/agent-compatibility.md`）。
- 纯 Chat 不得复用 agent 会话层或实现成特殊 agent 后端。它只消费 `modelconfig.Service.ResolveModel` 返回的服务端模型信息，不把 API Key 放入 DTO、日志或错误文本。
- 会话层（`internal/app/sessions`）不得出现后端原生轮次事件名。守卫测试 `TestSessionsDoesNotNameNativeTurnEvents` 禁止这 9 个名字（`execution_started/succeeded/interrupted/failed`、`agent_start/agent_end/agent_settled`、`turn_start/turn_end`），轮次状态只能消费契约的 `EventTurn`。原生名到契约事件的翻译属于适配器职责。
- 会话级 Stop 只停止本会话：门控 `Interrupt` 能力，运行中的轮次先经适配器 `Interrupt`，成功后才把本会话标记为 `stopped`。它不得调用 `instances.Stop`，也不得改动实例或兄弟会话。停止实例是另一条路径（`POST /projects/{id}/instances/{instanceId}/stop`），门控 `Stop` 能力。

## 已消除与剩余的耦合

| 位置 | 说明 | 处理 |
| --- | --- | --- |
| 交互子路由的 URL 前缀（权限/表单/raw-events） | 历史遗留的 `/opencode/` URL 前缀已彻底移除，变更为中性路由 `/projects/{id}/sessions/{sessionId}/...` | 已完成中性化更名 |
| 项目级后端开关 `AgentBackendConfig` | 已随第 2 项移除（迁移 v18 删表），后端启用只剩平台设置一处 | 已完成 |
| 原始事件端点（`/raw-events`）与前端 `api/interactions.ts` 的 `rawEventsPath` | 原始流保留为调试通道，待处理交互已改走归一化流，前端交互不再订阅原始流 | 保留端点与 `rawEvents` 能力标志；仅供后端专属调试视图使用，通用交互路径不依赖它 |
| 机器级服务的界面文案 | 文案中的"opencode service"由 `label` 小写生成，保持了原有字符串 | 新增带服务的后端时文案自动生成 |
