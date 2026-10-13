# opencode service 外部化 + Agent 检测启停 + 项目设置 tab 化

日期：2026-10-13
状态：待批准

## 1. 背景与目标

opencode 后端在平台上跑的是**一台机器一个**的 `opencode serve --service`（shared server，见 `internal/agents/opencode/sidecar.go`）。目前它被平台当成自己的资源：新建任务、扫描会话、打开旧任务时如果探不到 service，平台会自己拉一个；service 进程意外退出或探活连续两次失败，平台会拆掉它并在下次使用时换一个新的；连平台自己关停都要把它杀掉。

这个前提不成立：**这个 service 不只为我们服务**。桌面端 opencode 客户端、手工启动的 `opencode serve --service`、另一个平台实例都可能连同一个 service。平台擅自重启它，等于替别人做了决定，还会打断别人正在跑的会话。

本次改动三件事（来自需求方）：

1. **任何时候不要自动重启或停止 opencode service。** 平台只观察、不伸手。
2. **平台设置 → Agent 增加自动检测**：opencode 额外显示 service 的启动状态，并提供手动启动 / 停止操作。
3. **项目设置改为以 tab 页展示**，与任务 tab 并列（两类 tab 共用一条 tab 条）。

需求方的原话锚点：「opencode service 对我们来说是一个可以调用的第三方，不仅仅为我们服务，我们可以手动管理 opencode service，但不要跟我们的业务逻辑绑死。」「手动启停是人工判定的，操作人对该操作负责。」

## 2. 范围

### 2.1 纳入

- opencode 适配器：业务路径中彻底移除对 service 进程的启动、停止、拆解；新增"仅接管"模式。
- service 状态检测与手动启停的后端能力 + HTTP API + Agent 设置页 UI。
- service 未启动时，新建任务 / 扫描会话 / 打开旧任务返回可操作的明确提示，并可跳转到 Agent 设置页。
- 项目设置从主区默认面板改为工作区 tab；tab 模型泛化为两类；项目设置面板移除"实例"区块。
- 相关文档（`docs/agent-compatibility.md`、`docs/api/openapi.yaml`）与 e2e 夹具调整。

### 2.2 不纳入

- **pi 后端不在此次范围**。pi 没有常驻服务（每实例一个子进程），Agent 页继续只有启用开关 + 可执行文件检测，不加启停按钮；平台关停仍照旧回收 pi 子进程。
- 不支持一台机器多个 opencode service、不支持选择/切换 service 端点（`service.json` 是全机唯一契约）。
- 不做 service 状态轮询、不做后台健康推送。
- 不改数据库结构（无迁移）。

### 2.3 反目标（做成了也算失败）

- 界面上出现"重启服务"这类平台替用户决定的动作。
- 任务面板出现"实例 01a12479… "这类内部概念。
- 用户看到的"运行中"被误当实时状态，而实际是十分钟前的探测。

## 3. 术语与现状

| 词 | 现状 | 改动后 |
|---|---|---|
| **opencode service** | 机器级 `opencode serve --service`，由 `~/.local/state/opencode/service.json`（`$XDG_STATE_HOME`）描述 | 不变，但平台只在设置页动它 |
| **实例 instance** | 适配器内存中的注册 + `agent_instances` 行，per project | 仍然存在（会话索引需要），但对用户完全不可见，纯后台 |
| **接管 adopt** | 读注册文件 + pid 存活 + `/api/info` 的 pid/version 与文件一致 | 不变，成为业务路径的唯一获取方式 |

## 4. 决策记录

标 ★ 的是需求方明确决定的，其余是实现判断（批准本设计即认可这些判断）。

| # | 决策 | 来源 |
|---|---|---|
| 1 | 业务路径任何情况下不启动、不停止、不重启 service；service 没跑就拒绝业务请求 | ★ |
| 2 | service 未启动时新建任务：提示「opencode service 未启动，请到平台设置 → Agent 检测并启动」 | ★ |
| 3 | 平台关停（`./dev.sh restart`、Ctrl-C、SIGTERM）不管 service，进程存活 | ★ |
| 4 | 唯一能启停 service 的入口是平台设置 → Agent；停止按 `service.json` 的 pid 真杀，**不管是谁起的进程**，责任在操作人 | ★ |
| 5 | 检测时机：进入 Agent 页时自动测一次，之后只在点「检测」时测；不轮询 | ★ |
| 6 | 用户创建任务时不选后端实例；实例注册是后台行为 | ★（推导） |
| 7 | 项目设置以 tab 展示；点项目名仍只展开/收起任务树；点项目名后面的设置按钮打开项目设置 tab | ★ |
| 8 | 项目设置中"实例"区块删除，实例概念对用户消失 | ★ |
| 9 | 平台自己拉起的 service 平台不再"拥有"：平台退出它继续活，下次启动按普通接管处理，与接管桌面端的走同一条路 | 判断 |
| 10 | 状态标签旁显示「最后检测于 hh:mm」，避免过期状态被当成实时 | 判断 |
| 11 | 诊断里 `stop and restart the affected instance(s)` 改为「到平台设置 → Agent 检测并启动」，与新提示口径统一 | 判断 |
| 12 | 一个项目的设置 tab 同时最多一个；重复点齿轮激活已有 tab；不同项目各开各的 | 判断 |
| 13 | 旧链接 `/projects/:id`（原项目设置面板路由）重定向到 `/`（home），不再渲染设置面板 | 判断 |

## 5. 后端设计

### 5.1 契约层（`internal/agents/contracts.go`）

新增**可选能力**接口，不改动 `AgentAdapter` 必选方法（pi 不实现）：

```go
type ServiceState string
const (
  ServiceRunning     ServiceState = "running"     // 注册文件 + pid 存活 + /api/info 通过
  ServiceStopped     ServiceState = "stopped"     // 无注册文件，或 pid 已死
  ServiceUnreachable ServiceState = "unreachable" // 有注册文件与活 pid，但 API 探不通或 pid/version 不符
)

type ServiceStatus struct {
  Backend   BackendID
  Supported bool        // 该后端是否有 service 概念（pi 为 false）
  State     ServiceState
  Endpoint  string      // service.json 的 url
  PID       int
  Version   string
  Executable string     // 解析出的可执行文件路径
  Detail    string      // 不可达/失败原因，已脱敏
  CheckedAt time.Time   // 本次探测时间，前端展示"最后检测于"
}

// ServiceController 由"有机器级常驻服务"的后端实现。opencode 实现，pi 不实现。
type ServiceController interface {
  DetectService(ctx context.Context) (ServiceStatus, error)   // 只读探测，不改任何内存状态
  StartService(ctx context.Context) (ServiceStatus, error)    // 已在运行则返回 ErrServiceAlreadyRunning
  StopService(ctx context.Context) (ServiceStatus, error)     // 未在运行则幂等成功
}

var ErrServiceAlreadyRunning = errors.New("service already running")
```

`instances.Service` 增加 `Probe(backend) (agents.ServiceStatus, error)`：适配器未实现 `ServiceController` → `Supported=false`。

### 5.2 业务路径摘除自动启停（opencode 适配器）

| 位置 | 现在 | 改成 |
|---|---|---|
| `sidecar.go:65` `ensureShared` 的 `a.launch(...)` 分支 | 探不到就自己起一个 | **删除**。接管失败即返回 `ErrServiceNotRunning`（新增包级错误，`errors.Is` 可判） |
| `sidecar.go:126` `watchShared` | 进程退出 / 连续两次探活失败 → `failAllInstances` + `teardownShared`（可能 kill 进程） | 保留 `failAllInstances`（会话必须变 disconnected）与丢弃内存引用（新增 `detachShared`：只清 `a.shared`/`a.sharedStop`，**不 kill**）；不再有 kill 路径 |
| `adapter.go:508` `StopInstance` 的 `remaining == 0 → teardownShared` | 最后一个实例走就拆服务 | **删除**。stop 只关本地事件流与注册 |
| `instances/service.go:417` `Close` | 逐个 Stop（间接杀 service） | 逻辑不变，但因上一行删除，opencode 的 stop 变成纯本地清理；pi 子进程回收行为不变 |
| `adapter.go:381` 诊断文案 | "stop and restart the affected instance(s)" | 「opencode service 不可达（…）；请到平台设置 → Agent 检测并启动」 |

`launchSharedServer`（`sidecar.go:181`）**保留**，但唯一调用方变成设置页的 `StartService`。`teardownShared`（会 kill 进程组）保留，唯一调用方变成 `StopService`。

`StartService` 细节：先 `DetectService`；已是 `running` → `ErrServiceAlreadyRunning`。工作目录用平台 `dataDir`（service 是机器级，无项目语义）。端口取空闲端口、密码随机（沿用现状），`waitReady` 成功后把 shared 引用指向新 service。
`StopService` 细节：读 `service.json` 的 pid → kill 进程组（**外部起的进程同样杀**，决策 4）→ 等退出 → `detachShared` + `failAllInstances("opencode service stopped from platform settings")`。若注册文件指向的进程已死，只清平台内存引用；注册文件本身**不由平台删除**（那是 opencode 的文件，决策：平台不写不删别人的状态文件，unreachable 状态下 stop 只做能安全做的部分并在 `Detail` 里说明）。

### 5.3 sessions 层的拒绝路径（`internal/sessions/service.go`）

`instanceForBackend`（358）在调用 `instances.Start` **之前**先 `instances.Probe(backend)`：若 `Supported && State != running`，直接返回新错误 `ErrServiceNotRunning`（携带后端名与建议文案），**不创建 failed 实例行**——避免用户看不见的表里堆积垃圾行。

**业务路径的探测是便宜的探测**（实现判断）：`instances.Probe` 在业务路径上只读 `service.json` + `kill(pid,0)` 判存活，**不做 HTTP `/api/info` 探活**（那是 3s 超时的网络调用，不该压在每次新建任务的同步路径上）。真正的 HTTP 探活只发生在设置页的 GET/启停、诊断、以及 `watchShared` 的失败广播。`ServiceStatus.CheckedAt` 因此对便宜探测意义不大，UI 上只有设置页展示它。

影响的入口（都会返回同一个错误）：`Create`（新建任务）、`DiscoverProject`（扫描会话）、`Continue`（打开旧任务，实例需要重绑时）。`Send` 行为不变（本来就要求实例 running）。

HTTP 映射：`409 Conflict`，`{"code":"service_not_running","message":"opencode service 未启动，请到平台设置 → Agent 检测并启动"}`。

### 5.4 HTTP API

新增到 `internal/agents/instances/handlers.go`（它已持有 registry）：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/agent-services/:backend` | 探测并返回 `ServiceStatus`（含 `supported`） |
| POST | `/api/agent-services/:backend/start` | 启动；`409` 表示已在运行 |
| POST | `/api/agent-services/:backend/stop` | 停止；需 CSRF（与现有 mutating 路由一致） |

三个路由均为登录后可用；`:backend` 非法 → 400；后端不支持 service → `supported:false`（GET）或 `400`（POST）。写入 `docs/api/openapi.yaml`。

## 6. 前端设计

### 6.1 平台设置 → Agent（`web/src/components/settings/AgentSettings.vue`）

每个后端一张卡片。opencode 卡片在现有"启用开关 / 可执行路径 / 版本"之下增加 **service 区块**：

- 状态徽标：`运行中` / `未启动` / `不可达`（不可达为 danger 色，附 `Detail`）
- 只读明细：监听地址、pid、service 版本、可执行文件、**最后检测于 hh:mm**
- 按钮：`检测`（转圈时禁用其他按钮）、`启动`（running 时禁用）、`停止`（stopped 时禁用；确认弹窗文案点明"这会停止本机所有正在使用该 service 的客户端，包括桌面端"，因为需求方明确"操作人负责"）
- 失败信息就地显示，不做 toast
- **进入页面自动调用一次 GET**；之后只有点"检测"才再调

pi 卡片：不请求 `/agent-services/pi`，或请求后按 `supported:false` 不渲染 service 区块。

### 6.2 两类 tab

`web/src/stores/openTabs.ts` 的 `OpenTab` 变为可辨识联合：

```ts
type OpenTab =
  | { kind: 'task'; projectId: number; sessionId: string; title: string }
  | { kind: 'project-settings'; projectId: number }
```

- localStorage key 换为 `dev-platform-open-tabs-v2`；**旧 key 一次性迁移**：读旧 key，无 `kind` 的条目补 `kind:'task'`，写回新 key 并删旧 key。（不迁移会导致用户刷新后所有已开任务 tab 消失。）
- `TaskTabs.vue` 泛化为工作区 tab 条：`tabs: OpenTab[]`、`activeKey: string`（task 用 sessionId，settings 用 `settings:<projectId>`）；任务 tab 用现有圆点，设置 tab 用齿轮图标 + 标题「项目名 · 设置」；`aria-label`、`data-test="open-tab"`、`data-tab-key` 保留可测性。
- 关闭逻辑：task tab 关闭时像现在一样 `sessions.unsubscribe`；settings tab 无流可清理。激活 tab 被关时的回落规则与现在一致。

新路由：`projects/:projectId/settings` → `name: 'project-settings'`，component 复用 `ProjectSettingsPanel.vue`；`AppShell` 的 tab 条渲染逻辑改为按 `openTabs.tabs` + 当前路由同步（现在按 session 路由同步，需要同时处理 settings 路由）。

`ProjectTree.vue`：现有 `.project-settings` 按钮由"聚焦项目"改为 `emit('open-settings', project)` → `AppShell` 打开/激活 settings tab。点项目名的展开/收起行为**不变**（决策 7）。

## 7. 项目设置面板内容调整

`ProjectSettingsPanel.vue` 移除"实例"区块（实例列表、启动/停止实例按钮）及 `agents.startInstance/stopInstance/loadInstances` 的相关调用；保留：项目信息、后端启用、扫描会话、资源、后端诊断、危险操作（重命名/移动/注销/删除）。面板顶部新增一行 service 归属提示：opencode 后端时显示「由 opencode service 提供服务 · 运行中/未启动/不可达」+ 未启动时的跳转链接（点击到 `settings-agents`）。数据来自**打开该 tab 时调用一次** `GET /api/agent-services/opencode`（GET 即探测，与决策 5「打开页面测一次」同构），之后不再自动刷新，面板上不做刷新按钮。

后端 API `POST /projects/:id/instances` 与 `.../stop` **保留**（pi 与将来后端仍需要，且 sessions 内部依赖 Start），只是不再被项目设置页调用。

### 7.1 停止 service 之后

`StopService` 成功后所有 opencode 实例被广播为 failed、会话 reconcile 成 disconnected。手动把 service 重新启动**不会**让会话自动接上：用户重新打开任务时走 `Continue`，平台探测到 service 已运行、后台注册新实例并重绑会话（沿用既有 reconcile/rebind 逻辑，只是不再启动进程）。这是有意为之：需求方要的是"人负责"，不做自动复活。这条要写进相关测试，避免实现时顺手加"重启后自动重连"。

## 8. 提示与文案

统一口径（前后端都从这里措辞）：

> opencode service 未启动。请到 平台设置 → Agent 检测并启动。

出现位置：新建任务面板提交后、任务面板打开旧任务失败时（`data-test="session-unavailable"` 附近）、扫描会话失败时、项目设置 service 提示行。文案中带一个可点击的「平台设置 → Agent」链接（`RouterLink` 到 `settings-agents`）。

## 9. 测试与夹具

**Go**：
- `sidecar_test.go` / `discovery_test.go` / `adapter_test.go`：现有"接管优先于启动""启动失败可诊断"等断言中，凡是经 `StartInstance` 触发的启动路径要改成经 `StartService` 触发；新增断言：注册文件缺失/进程死亡时 `StartInstance` 返回 `ErrServiceNotRunning` 且 `launch` 调用次数为 0；`StopInstance`（含最后一个实例）不调用 kill；`Close` 不杀进程。
- 新增 `ServiceController` 测试：Detect 三态、Start 已运行 → `ErrServiceAlreadyRunning`、Stop 幂等、Stop 杀外部 pid。
- `internal/sessions/service_test.go`：service 未运行时 Create/DiscoverProject/Continue 返回 `ErrServiceNotRunning` 且**未写入实例行**。
- `internal/agents/instances` handler 测试 + `internal/http/router_test.go` 路由与权限（CSRF）。

**e2e**：`web/e2e/fixtures/bin/opencode` 目前只支持 `serve --hostname --port`，靠平台自己拉起。改为：mock 支持 `--service`，**自己写** `service.json`（id/version/url/pid/password，与真实 opencode 契约一致），`web/scripts/run-e2e.mjs` 在起平台之前先起 mock 并设置 `XDG_STATE_HOME` 指向测试隔离目录；mock 增加 `--stop`（或读一个控制文件）以便测试"未启动"分支。新增 spec：Agent 页显示运行中/未启动并可启停；service 未启动时新建任务显示提示并可跳转到 Agent 页。

## 10. 兼容性

- `agent_instances` 表与既有会话行不变：老数据里 running 实例在平台重启后按现有 reconcile 变 disconnected，打开任务时后台重新注册（现在还会先探测 service）。
- `docs/agent-compatibility.md` 的 opencode 列需更新：新增"service 生命周期：外部依赖，平台不启停（设置页除外）"一行；`docs/api/openapi.yaml` 增补三个新端点。

## 11. 交给实现者的开放点

- `detachShared` 后是否需要保留旧的探活定时器实现（可以整段退化为"退出广播"），只要满足：不 kill、不重启、失败会广播到实例。
- 项目设置 tab 的排序（追加在任务 tab 之后还是按打开顺序混排）——默认按打开顺序混排，实现时如遇视觉拥挤可调整为设置 tab 恒在最左。
- `ErrServiceNotRunning` 在 OpenAPI 中的 code 字面量（本设计用 `service_not_running`）。
