# 后端边界：通用层与异构层

本文说明平台如何把"与具体 agent 无关的部分"（同构层）和"必须因后端而异的部分"（异构层）分开，并给出新增后端的步骤。规则由 `internal/sessions/dependency_guard_test.go` 中的两条守卫测试执行。

## 分层

| 层 | 包 | 职责 | 能否出现后端 ID 或后端包 |
| --- | --- | --- | --- |
| 同构：平台配置 | `internal/settings`、`internal/database` | 外观、后端的启用与可执行路径 | 否，只读 `agents.Catalog` |
| 同构：项目管理 | `internal/projects`、`internal/paths` | 项目记录、目录的安全生命周期 | 否，不知道 agent 的存在 |
| 同构：会话与实例编排 | `internal/sessions`、`internal/agents/instances`、`internal/http` | 会话索引、实例生命周期、SSE、路由 | 否，只依赖 `internal/agents` 契约 |
| 契约 | `internal/agents`（`contracts.go`、`extensions.go`、`descriptor.go`、`registry.go`、`service.go`） | 能力声明、标准事件、扩展接口、后端描述符 | 仅在 `BackendID` 常量定义处 |
| 异构：适配器 | `internal/agents/opencode`、`internal/agents/pi` | 与具体 CLI 或服务协议打交道，导出 `Descriptor` | 是（本包即为该后端） |
| 组合根 | `cmd/platform/main.go` | 注册适配器、构建 catalog、校验两者一致 | 是 |
| 异构：前端展示 | `web/src/stores/agents.ts` 与各组件 | 从后端列表读取标签、排序、能力 | 否（组件内不写后端 ID） |

## 契约层提供的通用机制

- **能力声明 `Capabilities`**：`AgentAdapter` 的一部分，每个布尔值都必须显式声明。通用层用能力门控控件，不用后端 ID 分支。
- **可选扩展接口**：`agents.PermissionReplier`（工具确认与表单）、`agents.RawEventOpener`（原始事件流）、`agents.HistoryReader`（历史）、`agents.ServiceController`（机器级服务）。适配器实现才可用，未实现时通用层返回 `ErrUnsupportedCapability`。
- **后端描述符 `BackendDescriptor` 与 `Catalog`**：每个适配器包导出 `Descriptor`，包含显示名、可执行文件名、默认启用、展示顺序、测试版本范围、是否有机器级服务。组合根用它们构建 catalog。
- **注册表 `Registry`**：适配器的显式注册点。组合根在启动时调用 `checkBackendWiring`，确认注册表与 catalog 描述的后端集合完全一致，否则启动失败。

## 新增一个后端的步骤

1. 在 `internal/agents/contracts.go` 为 `BackendID` 增加常量，并把它加入 `BackendID.Valid()`。这是唯一需要改动契约包的地方。
2. 新建适配器包 `internal/agents/<name>`，实现 `agents.AgentAdapter`（包括 `Capabilities()`）。按需实现 `PermissionReplier`、`RawEventOpener`、`HistoryReader`、`ServiceController`，并与 `Capabilities` 中对应的标志保持一致。
3. 在该包导出 `Descriptor`，并写 `descriptor_test.go` 钉住版本范围与 `ServiceSupported`。
4. 在 `cmd/platform/main.go` 中：把 `Descriptor` 加入 `agents.NewCatalog(...)`，并在注册表中注册适配器。`checkBackendWiring` 会检查两者是否一致。
5. 在 `docs/agent-compatibility.md` 增加该后端的能力矩阵与版本表。
6. 前端无需改动：标签、排序、服务控件、资源页的后端复选框都由后端列表驱动。

## 禁止事项

- 通用层（`internal/sessions`、`internal/settings`、`internal/http`）不得 import 具体后端包，也不得出现后端 ID 字面量。守卫测试会检查。
- 不得用后端 ID 做功能分支。需要的能力必须在 `Capabilities` 中声明，或通过可选扩展接口表达。
- 不得在通用代码里补默认值。未知后端必须显式报错或返回 `none`，不得回退到某个已知后端。
- 组件不得写死后端名称或服务名称。需要展示的事实从 `agents` store（`backendLabel`、`orderedBackends`、`serviceBackends`、`backendFor`）读取。
- 只有组合根与适配器包能决定某个后端"存在"。业务路径不得启动、停止或替换 agent 的机器级服务（见 `docs/agent-compatibility.md`）。

## 已知的剩余耦合（有意保留）

| 位置 | 说明 | 处理 |
| --- | --- | --- |
| `internal/sessions/handlers_opencode.go` 的文件名与 `StreamOpencodeRaw`、`ListOpencodePermissions` 等方法名 | 仍带 opencode 前缀，但内部已不依赖 opencode 包 | 重命名会扩大改动面，待后续单独处理 |
| `web/src/stores/opencodeRaw.ts` | 仅服务于原始事件与工具确认，只在声明了对应能力时才调用 | 保留，它是 opencode 事件的专属视图 |
| `internal/resourcesync` 的 `OpenCode{}`、`Pi{}` 适配器 | 资源同步仍在组合根手工装配 | 待 catalog 稳定后评估合并进描述符 |
| `internal/agents/instances` 的项目级后端开关 `AgentBackendConfig` | 留在实例包，与启动耦合紧密 | 有意保留，见实施计划的"不在本计划内"一节 |
| 机器级服务的界面文案 | 文案中的"opencode service"由 `label` 小写生成，保持了原有字符串 | 新增带服务的后端时文案自动生成 |
