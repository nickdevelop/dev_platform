# dev_platform 项目架构与技术规范

本文档是 `dev_platform` 项目的权威整体架构与实现设计描述。后续开发人员及 AI 助手在理解项目、检索功能代码、新增需求或进行架构重构时，**必须以本文档为准**。当需求发生变化、新增业务功能或发生架构调整时，**必须同步更新维护本文档**。

---

## 一、项目定位与核心能力

`dev_platform` 是一个面向团队和个人开发者的**多 Agent 协同 AI 研发工作台**。其核心目标在于解决不同终端 AI Coding Agent（如 OpenCode V2、Pi、DeepSeek CLI）协议各异、交互割裂、缺乏集中管控的问题，提供中性同构的平台层封装：

1. **多后端统一协同**：统一接入异构的 AI Coding Agent，支持开箱即用的多 Agent 实例隔离、生命周期管理与并发会话。
2. **实时交互工作台**：统一的 Web 端会话界面，支持类型化轮次追踪、流式响应（SSE）、代码 Diff 预览、富文本 Markdown 清洗与 PlantUML 架构图渲染；主对话流纯净承载用户消息与答复、人机问答交互卡片（已将思考过程与工具调用过程移出会话对话流，保持高信噪比与高阅读舒适度）；右侧面板整合为统一的右侧工作区工作台（包含左侧文件预览区与右侧文件树，且移除最右侧冗余的竖向导航条，在顶层 Tab 栏最右侧提供纯图标布局控制开关，操作按钮全面纯图标化）；面板整体锁定外框总宽度，文件树在面板内部靠右展开或向右折叠，内部弹性伸缩（flex: 1 撑满），主对话区宽度恒定，彻底消灭对话内容跳动与晃动。新建会话表单将项目选择与 Agent 分段开关（Switch 形态）各自独立成行（Agent 行紧随项目行），输入框工具栏整合模型选择、权限下拉模式与统一发送按钮；提示词模板下拉仅提供流程类（flow）模板（角色类模板保留给角色库与纯对话助手，历史草稿中的角色类选择会被自动清理）；发送快捷键提示内嵌于输入框占位文本，不在输入组件下方额外占行。
3. **闭环人机确认**：提供权限确认（Permission）与结构化表单（Form）的人机交互流，杜绝客户端轮询，所有交互直接挂接在会话标准事件流中。
4. **全功能研发辅助**：集成基于 PTY/WebSocket 的跨平台在线终端、支持 Cron 表达式的定时任务调度、提示词（Prompts）模板管理，以及独立于 Agent 会话的通用大模型纯 Chat 对话。
5. **快捷助手**：可在平台设置中开启的横向方形半透明工具栏入口（26px 紧凑设计，默认 0.3 半透明，悬浮与展开时高亮过渡），以 480×600 弹窗新建快捷对话，历史对话通过 Header 的最近对话下拉自行选择；工具栏与弹窗位置解绑且弹窗支持视口内自由拖拽；其会话统一沉淀在 Chat 的对应助手分组，与主 Chat 页面共享模型配置但使用独立事件流。
6. **服务代理与 Demo 预览**：提供统一的动态 ContextPath 网关，支持将服务器上的静态文件目录（Directory Serving）或本地监听端口的 HTTP 服务（Port Reverse Proxy）通过平台端口以独立 ContextPath（`/proxy/:alias/`）直接暴露；免登录公开访问，支持外部快速分享与查看 demo，全协议透传并支持 WebSocket 升级（完美兼容 Vite/Webpack 等开发工具的 HMR 热更新）。
7. **桌面端双模协同**：提供原生桌面客户端外壳（Electron 架构），支持“本地单机自带服务（Sidecar 自动拉起与优雅停机）”与“连接远程云端/团队服务”双模开箱即用与自由切换；主进程内置高性能轻量本地透明反代（Desktop Local Proxy），无缝穿透跨域与 SameSite Cookie 限制，并动态对齐 Origin 保证终端 WebSocket CSWSH 安全校验 100% 契约一致，使前端业务代码与后端服务零侵入同构。

---

## 二、技术栈全景

| 层次 | 核心技术 / 依赖 | 架构职责与说明 |
|---|---|---|
| **后端语言** | Go 1.24.0 | 静态类型、跨平台编译（Linux / Windows / macOS） |
| **HTTP 与网关** | Gin v1.11.0 | RESTful 路由编排、全局统一 SSE（Server-Sent Events）事件网关（`/api/events/bus/:uuid`），配合前端 SharedWorker 共享工作线程实现全浏览器跨标签页单长连接多路复用，根除浏览器并发长连接耗尽问题 |
| **实时通信** | Gorilla WebSocket v1.5.3 | 终端 PTY 双向字符流转发（带严密 RFC 6455 同源 Origin 校验） |
| **数据持久化** | GORM v1.25.7 + SQLite (`glebarez/sqlite`) | 纯 Go 实现的嵌入式数据库（无需 CGO），自建迁移版本链（当前 v35，含 Agent 模型快照表、快捷助手固定分组、服务代理映射表 `service_proxies`、助手平台管理与最大工具步数配置、多 Agent 协同流程流转表，以及最新升级：`prompts` 增加 `type` 字段区分角色类与流程类、全局专家角色管理表 `agent_roles`、`chat_groups` 增加 `prompt_id` 实现对话与角色对提示词管理的统一引用机制） |
| **系统与进程** | `creack/pty`、`golang.org/x/sys`、`crypto` | 跨平台子进程生命周期管理、ConPTY/PTY 分配、安全密码哈希与 SSH 客户端 |
| **调度引擎** | `robfig/cron/v3` | 支持标准 5/6/7 段 Cron 表达式（秒、分、时、日、月、周、年）及单次定时调度 |
| **前端工程** | Vue 3.5 (SFC, `<script setup>`), Vite 8.3, TypeScript 7.0, Tailwind CSS 3.4, PostCSS 8.5 | 单页面应用（SPA），组合式 API，原子化样式与全局扁平微圆角规范 |
| **前端状态与路由** | Pinia 4.0, Vue Router 5.4 | 领域驱动的 Store 分层，支持草稿跨会话暂存 |
| **前端展现渲染** | xterm.js 5.3, Marked 18.1, DOMPurify 3.4, `@plantuml/core` | 终端仿真、安全 Markdown 流式渲染、PlantUML 在线绘图 |
| **桌面客户端外壳** | Electron v34, TypeScript, Node.js 原生 HTTP/WebSocket 透明代理, `electron-builder` | 原生跨平台桌面封装（Linux/Windows/macOS，禁用/移除默认原生菜单栏）、Sidecar 本地子进程纳管、双模网络同构代理与凭据加密安全存储 |
| **测试与质量门禁** | Go Testing + AST 静态扫描守卫; Vitest 5.0 (808 单测全量通过); Node.js 原生 Test Runner; Playwright E2E | 源码级架构分层守卫、CSRF 防绕过拦截矩阵、平台 UI 库组件单测、协同流程群聊单测、桌面代理与连接单测、全流程端到端自动化 |

---

## 三、分层架构体系（Claudian 五层同构规范）

项目严格对齐 Claudian 的五层架构体系，将“与具体 Agent 无关的通用同构层”和“必须因后端而异的异构层”彻底解耦：

```
① 组合根     cmd/platform/（Go 服务入口）+ desktop/src/main/（Electron 桌面主进程）+ web/src/{main.ts, App.vue, router.ts}（Vue 挂载）
④ 契约层     internal/core/agents（标准协议/事件/注册表）、internal/core/platform（OS 抽象）
③ 领域服务   internal/app/{sessions, instances, agentmodels, projects, chat, platformtools, tasks, terminal, settings, auth, ...}
⑤ 异构适配   internal/providers/{opencode, pi, deepseek}（实现 core 契约，向组合根导出 Descriptor）
传输编排     internal/transport/http（路由编排、AccessLog 中间件、静态资源 SPA 托管）+ desktop/src/main/proxy.ts（桌面透明反代）
② 前端展示   web/src/features/{chat, workspace, projects, settings, terminal, tasks, ...}、web/src/shared/
```

### 依赖方向与防腐规则（守卫强制执行）：
- **依赖方向**：① 组合根 → 任意层；③ 领域服务 → ④ 契约层；⑤ 异构适配 → ④ 契约层；② 前端 features → 状态/API → core 契约。
- **严厉禁止**：
  - 禁止 `core → providers`、`app → providers`、`transport → providers`。
  - 禁止在通用同构层（`internal/app` 及 `internal/transport`）中出现任何特定后端 ID 字面量（如 `BackendOpenCodeV2`、`"opencode-v2"`、`"pi"` 等）或直接导入适配器包。由 `dependency_guard_test.go` 递归遍历全目录进行 AST 源码级拦截。
  - 前端通用组件禁止针对后端 ID 写 `if-else` 分支，所有功能差异必须基于契约能力集（`Capabilities`）或中性元数据（`Descriptor`）进行门控驱动。

---

## 四、模块职责划分与功能代码索引地图（核心导航罗盘）

本节是开发人员与 AI 快速定位功能代码、添加新功能的检索指南。修改代码时必须确保改动严格落在对应职责包内。

### 1. 后端同构领域服务层（`internal/app/`）
通用业务逻辑，**严禁硬编码任何特定后端 ID 或导入 `internal/providers/*`**。

| 模块目录 | 核心职责与功能边界 | 关键入口与核心文件 | 协作与边界约束 |
|---|---|---|---|
| **`events/`** | **统一多路复用事件网关**。平台级统一事件中枢与 SSE 网关服务（`UnifiedEventBusManager`）。下行事件多路复用统一收敛至单连接 `/api/events/bus/:uuid`，解耦具体业务领域（支持 `sessions`、`chat`、`tasks`、`system` 频道），通过动态主题订阅接口（`POST/DELETE .../subscriptions`）实现各业务页面即插即拔；统一心跳保活（15s keep-alive）与客户端断开内存垃圾回收（GC TTL 60s）；具备关键帧入队保护（对 `turn_ended`、`interaction_requested` 等核心状态机事件与 `message/thinking/delta/chunk` 流内容帧在缓冲满时提供 100ms 等待入队保底）、慢客户端丢帧警告日志与原子丢帧计数器（`DroppedCount`）；**缓冲溢出可靠性闭环**：关键帧丢弃、流内容帧每第 8 次丢弃或累计第 64 次丢弃时主动踢断该 bus 的 SSE 连接（kick），迫使客户端带序号重订并经 `instances.ReplaySince` 环形重放或 resync 全量重建补齐缺口——把"在线静默丢帧"转化为可恢复转移；每帧写入前施加 45s 递进写超时（write deadline = 3×心跳），防止 TCP 停滞的客户端令 StreamBus 循环永久阻塞而缓冲无限溢出；提供 `GET /api/events/diagnostics` 诊断端点（各 bus 的 active/订阅计数/积压帧数/丢帧计数）；彻底取代原先各业务烟囱式独立建长连接的历史架构，与前端 SharedWorker 协同彻底消灭浏览器同域名 6 连接耗尽假死问题。 | `service.go`（服务封装与依赖编排）、`bus.go`（多路复用总线与心跳调度、溢出 kick、诊断快照）、`handlers.go`（统一 SSE 与订阅 REST API、diagnostics 路由）、`models.go`（通用信封协议 `EventEnvelope` 与 `ChannelHandler` 契约） | 纯通用同构层，定义 `ChannelHandler` 接口供各业务领域实现快照与生命周期钩子，不直接耦合底层 Agent 或特定业务逻辑。 |
| **`sessions/`** | Agent 会话工作台核心领域。维护会话元数据索引、类型化轮次追踪、人机交互决策仲裁、平台斜杠命令定义与调度（ListCommands/ExecuteCommand/CompactSession）。平台拥有命令统一定义权（分为功能类 action 与提示词注入类 prompt），当前实现功能类上下文压缩（compact），由各 Agent 适配层通过 SessionCompactor 落实。会话索引列表（全量 List 与分页 ListPaged）统一遵循 `updated_at DESC, created_at DESC, id DESC` 降序排序；对实例重绑（rebind）与模型更新等非交互写库操作使用列级更新保护 `updated_at`，杜绝历史会话被意外刷新时间戳；下行状态事件与交互快照统一接入全局事件网关（`events.Manager`，channel: `sessions`），废除原私有 bus 路由。 | `service.go`（会话主逻辑与 ChannelHandler 实现）、`handlers.go`（HTTP 路由处理）、`event_bus.go`（实例流监听与事件转发）、`service_interactions.go`（审批管理） | 子包 `turns/` 专注轮次状态机；子包 `store/` 专注索引持久化；不关心底层进程如何生成，仅依赖 `internal/core/agents`。 |
| **`instances/`** | Agent 运行时实例生命周期管理。管理宿主子进程派生、退出监控、事件流单向上游订阅、环形重放缓冲（Ring Buffer）。定义领域级哨兵错误（`ErrDirectoryUnavailable`、`ErrProjectNotAuthorized`）隔离外部工作区异常。 | `service.go`（实例调度与停止）、`replay.go`（断线重放缓存）、`repository.go`（实例持久化与自愈） | 保证实例间严格逻辑隔离；不触碰会话层业务逻辑；停止实例走此路径。 |
| **`projects/`** | 项目工作区与文件生命周期。项目目录登记/注销/移动/重命名、安全路径校验（防路径穿越）、Git 状态与提交历史（`GitCommits` / `GitStatusWithRef`）、Git 工作区版本闭环管理（文件暂存 `StageFiles`、移出暂存 `UnstageFiles`、放弃工作区变更 `DiscardFileChanges`、提交变动 `CommitChanges` 及本地分支管理 `ListBranches` / `CheckoutBranch`）、文件统一 Diff 差异比对（`GitFileDiff`）、全量文件树遍历（去 `.gitignore` 与常规点文件过滤，严格隔离 `.git` 元数据目录）、工作区文件读取与短时只读预览 Ticket 鉴权（支持带 CSP Sandbox 的免 Cookie 通配符静态资源安全加载）。项目侧边栏设置直接与 Tab 栏和 `projects-management` 路由联动，项目排序与显隐状态通过后端数据库持久化存储（`GET/PUT /api/projects/display-config`），杜绝关闭平台后重置。 | `service.go`（项目生命周期、预览 Ticket 及显示配置持久化）、`files.go`（工作区文件树与内容读取）、`handlers.go`（项目与文件路由）、`git.go` / `git_status.go` / `git_ops.go` / `git_diff.go`（Git CLI 操作集成与差异比对） | 不知道 Agent 的存在；项目删除时协同通知 `instances` 销毁对应进程。 |
| **`chat/`** | 纯模型非 Agent 直接对话系统。支持助手（Assistant）多级对话管理：支持助手专属配置（默认模型、系统提示词隐式注入模型上下文、图标配置/首字符自适应、浮窗快捷入口挂载、平台管理权限开关 `EnablePlatformTools` 与单轮最大工具步数 `MaxToolSteps` 控制）；移除固定分组锁定限制，所有助手平级可管理；对话只能在助手上新建；多模态图片上传（支持 32MiB 大请求体）、SSE 流式补全；支持流式 Function Calling / Tool Loop 调度（通过依赖反转注入中立的 `ToolExecutor` 接口，在助手开启平台管理权限时由大模型自主调用平台工具）；下行生成增量与消息事件统一接入全局事件网关（`events.Manager`，channel: `chat`），废除原 `/api/chat-events/:uuid*` 私有总线路由。 | `service.go`（助手与对话管理、上下文系统提示词组装与 Tool Loop）、`completer.go`（调用 OpenAI 兼容端点、分片聚合 tool_calls）、`tools.go`（中立工具定义与执行接口）、`stream.go`（SSE 事件帧切分）、`event_bus.go`（多路复用事件适配器） | 严禁导入 `sessions`、`instances`、`projects` 或 Agent 契约，与 Agent 流程物理隔离；系统提示词只注入请求上下文，不在对话消息气泡中体现；事件总线统一由 `events.Manager` 调度。 |
| **`platformtools/`** | 平台自身能力工具适配层。实现 `chat.ToolExecutor` 契约，将平台的项目管理（`projects.Service`）、提示词模板管理（`prompts.Service`）以及会话索引查询（`sessions.Service`）封装为 OpenAI 兼容的 JSON Schema Tools（`platform_list_projects`、`platform_list_prompts`、`platform_get_prompt`、`platform_create_prompt`、`platform_update_prompt`、`platform_delete_prompt`、`platform_list_sessions`）；由组合根（`cmd/platform`）统一组装并注入 `chat.Service`，使 Chat 中的轻量级 Agent 能够自主查询与管理平台项目、会话与提示词，实现严格的分层依赖倒置。 | `executor.go`（工具定义清单与分发路由） | 依赖 `projects`、`prompts` 与 `sessions` 领域服务，不直接接触数据库与底层进程。 |
| **`tasks/`** | 计划任务与多 Agent 协同流程调度引擎。支持标准 5/6/7 位 Cron 表达式（秒、分、时、日、月、周、年，配合前端 CronBuilder 可视化生成器、双向反解析引擎与后10次执行时间自动推断）与单次定时、并发配额控制池（最多 5 并发）、多策略执行器（`StandardTaskExecutor` 与 `FlowTaskExecutor`）、执行历史记录；内嵌由平台内置 AI 大脑驱动的协同流程中枢引擎（`internal/app/tasks/flow/`：`Engine`、`FlowTools`、`TelemetryBus`、`EventBus`），彻底废除旧版文本正则匹配 `@角色名` 的机制，全面升级为通过 Function Calling 工具（`flow_dispatch_role` 支持创建或复用已有会话、`flow_wait_session`、`flow_ask_user`、`flow_complete_task`）自主调度角色；配备独立的流程数据采集总线（`TelemetryBus`），实现对下游 Agent 会话数据流的实时采集转发与轮次终结 Promise 自动汇聚；流程任务流式执行事件与状态变更统一接入全局事件网关（`events.Manager`，channel: `tasks`），废除原独立 SSE 端点 `/api/tasks/flows/:runId/events`。 | `scheduler.go`（后台轮询调度器）、`executor.go`（标准/协同流多策略执行器）、`service.go`（任务/角色/流程 CRUD 与状态同步）、`flow/engine.go`（内置大脑 ReAct 协同调度引擎）、`flow/tools.go`（中枢 Agent 工具集）、`flow/telemetry_bus.go`（流程数据采集总线）、`flow/event_bus.go`（协同流事件分发总线） | 任务触发时调用 `sessions.Service` 创建或复用特定 Agent 会话执行，协同流中枢大脑通过 `modelconfig.Service` 与 `Completer` 驱动，通用层与底层异构 Agent 完全解耦。 |
| **`terminal/`** | Web 在线交互式终端。本地 PTY 分配、远程 SSH 会话池、WebSocket 双向字符转发、终端窗口 Resize 协同、同源 Origin 防御。REST 路由错误响应全面对齐 `errorResponse` 与 `c.AbortWithStatusJSON` 统一错误枚举规范。 | `handlers.go`（WebSocket 升级与同源拦截、REST 路由）、`service.go`（会话调度）、`session.go`（LocalSession / SSHSession 封装） | 调用 `internal/core/platform` 分配底层 PTY；不依赖具体 Agent 业务。 |
| **`modelconfig/`** | 大语言模型服务商与模型元数据配置。管理 OpenAI 兼容服务地址、模型列表、API 凭据安全关联、端点动态解析。 | `service.go`（模型服务增删改查）、`repository.go`（配置存储） | 为 `chat` 服务及平台模型选择器提供基础模型字典。 |
| **`modelprobe/`** | 大模型连通性探针。提供模型服务地址可达性测试与模型可用性探针，向前端提供探测报告。 | `probe.go`（连通性测试与模型抓取） | 供设置中心与模型管理页调用。 |
| **`agentservices/`** | 机器级 Agent 后台守护服务管理。负责机器级第三方后台服务（如 OpenCode service）的健康探测、运维手动启停与启动门控。 | `service.go`（探测与启停协调）、`handlers.go`（运维管理 API） | 仅在后台服务支持的后端生效；不承担模型缓存失效职责。 |
| **`agentmodels/`** | 平台级 Agent 模型快照与治理。持久化每个后端最近一次刷新的模型清单，并支持平台级启用/停用及配置修改（显示名称、上下文上限、图片支持）；默认读取数据库，不调用 Agent；手动刷新时调用适配器并执行增量原子合并，保留用户自定义配置与启用状态。 | `service.go`（List/Refresh/UpdateModel）、`repository.go`（快照读写与增量合并）、`handlers.go`（模型 API） | 只依赖 `core/agents` 契约、注册表与数据库；不感知具体后端；不提供项目级清单。 |
| **`auth/`** | 用户认证鉴权与安全防御。用户密码哈希校验、Session 令牌生命周期、本机首次免命令行密码设置引导、全局 CSRF 双提交校验与自动轮转中间件。 | `middleware.go`（`RequireCSRF` / `RequireSession`）、`csrf.go`（令牌生成与双提交比对）、`handlers.go`（登录/登出/首次密码设置） | 全局路由拦截第一关。 |
| **`secrets/`** | 敏感凭据脱敏与安全存储。管理第三方 API Key 与 SSH 密码，提供流式/日志敏感数据实时脱敏过滤器。 | `store.go`（加密存储与读写）、`redact.go`（正则脱敏过滤） | 用于模型配置、终端 SSH 凭据以及防止日志泄露。 |
| **`settings/`** | 平台全局偏好设置。管理外观主题（浅色/暖阳/冷灰，移除深色）、快捷助手开关、全局 Agent 系统提示词（AGENTS.md）落盘与跨后端配置同步、Tab 隔离的多 Agent 后端开关配置、可执行文件路径与运行模式、以及 dsh 守护进程空闲断开时长。 | `service.go`（系统偏好与快捷助手开关）、`general.go`（全局系统提示词文件管理与跨 Agent 同步）、`agents.go`（各 Backend 的配置状态与空闲超时）、`handlers.go`（设置 API） | 仅消费契约的 `Catalog`，不直接感知后端实现细节；全局提示词以平台数据目录 `dataDir/AGENTS.md` 为唯一权威源，跨平台文件同步写入各后端，不依赖 Windows 受限的软链接；快捷助手开关独立于外观配置存储。 |
| **`proxy/`** | 动态 ContextPath 服务代理网关。将服务器静态文件目录（含目录树浏览与防穿越）或本地端口 HTTP 服务反向代理至平台单一端口；全协议透明转发、自动剥离 ContextPath 前缀、支持 WebSocket 协议升级（HMR）与端口连通性探测。 | `service.go`（CRUD、参数校验与端口探测）、`gateway.go`（动态反代与文件服务分发器）、`handlers.go`（管理 API） | 数据面（`/proxy/*`）完全免鉴权公开直通；控制面（`/api/proxies`）严格执行 Session 与 CSRF 认证；不感知具体 Agent 业务。 |
| **`prompts/`** | 平台提示词模板管理。管理角色类（`role`）与流程类（`flow`）模板，支持 `{user_prompt}` 变量占位符、多维度关键词与分类筛选，供角色管理、纯对话助手及多 Agent 协同流程作为统一权威源引用；新建项目会话入口仅消费流程类模板，角色类模板不对该入口开放。 | `model.go`（模型定义）、`service.go`（CRUD 与分类过滤）、`repository.go`（持久化存储）、`handlers.go`（REST 路由） | 供平台各中心与 `platformtools` 组合调用。 |
| **`roles/`** | 全局专家角色管理。将多 Agent 协同流程中的角色抽取为平台一等实体，统一人设名称、职责描述、执行引擎（Backend）、分配模型（Model）与标识色彩；统一关联并动态同步提示词库中的角色类提示词，供多 Agent 协同流程与对话助手按需选用与复用。 | `model.go`（角色模型与 DTO）、`service.go`（角色 CRUD 与提示词动态同步）、`repository.go`（持久化存储与平滑兼容转换）、`handlers.go`（REST API 路由） | 依赖 `core/agents` 契约与 `prompts.Service` 接口，与具体后端实现解耦。 |
| **`database/`** | 平台数据持久化基础设施。GORM 数据库连接池初始化、版本化自动迁移（当前 v35 迁移链，含 `prompts` 表增加 `type` 字段区分角色类与流程类、全局专家角色管理表 `agent_roles`、`chat_groups` 增加 `prompt_id` 实现对话助手对提示词管理的统一引用机制、`agent_models` / `agent_model_catalogs` 增加 `enabled` 字段、服务代理映射表 `service_proxies`，以及多 Agent 协同流体系核心表 `flow_roles`、`flow_runs`、`flow_messages` 与 `tasks.orchestrator_model`）。设置纯 Go 嵌入式 SQLite 驱动安全连接池上限（MaxOpenConns=1），结合 WAL 模式与 5 秒 busy_timeout 消除并发写锁争用。 | `database.go`（SQLite 连接与连接池配置）、`migrations.go`（版本演化迁移逻辑） | 单一 SQLite 数据库；纯 Go 驱动无需系统 SQLite 动态库。 |
| **`runtime/`** | 单实例互斥控制。通过工作目录文件锁（lockfile）防止多个服务端进程并发冲突打开同一个 SQLite 数据库。 | `single_instance.go`（跨平台文件排他锁实现） | 组合根启动时最先获取，退出时最后释放。 |
| **`paths/`** | 跨平台路径解析与安全性校验。规范化路径表示，防范 `../` 目录穿越攻击，检验项目工作区授权边界。 | `paths.go`（安全路径判定与绝对路径规范化） | 被 `projects`、`instances` 等模块作为安全基座调用。 |

---

### 2. 核心契约与抽象层（`internal/core/`）
纯定义与中立策略，**禁止依赖任何业务层或异构适配层**。

| 模块目录 | 核心职责与关键文件 |
|---|---|
| **`agents/`** | **Agent 统一核心契约**：<br>• `contracts.go`：`BackendID`、`AgentAdapter` 主接口、能力声明 `Capabilities`、标准消息/模型载荷。<br>• `events.go`：类型化轮次 `EventTurn`、标准交互事件 `EventInteraction`、流式增量事件。<br>• `extensions.go`：平台命令模型（`PlatformCommand`、`CommandCategory`：action/prompt）与可选扩展接口（`SessionCompactor` 上下文压缩、`PermissionReplier` 审批、`HistoryReader` 历史回放、`ServiceController` 机器服务、`RawEventOpener`；命令由平台统一定义，不再由下游 agent 同步）。<br>• `descriptor.go` & `registry.go`：后端元数据描述符、`Catalog` 目录树与全局适配器注册表。 |
| **`platform/`** | **操作系统差异化抽象**：<br>• `platform.go`：统一导出 `Process`、`PTY`、`Terminal` 端口接口。<br>• `new_unix.go` / `new_windows.go`：Linux/Unix 与 Windows 的条件编译实现。<br>• 子包 `policy/`、`fake/`、`unix/`：跨平台路径策略、测试伪造平台实现。 |
| **`textutil/`** | **通用文本工具**：<br>• `title.go`：依据首条消息智能截断与推导会话标题。 |

---

### 3. 异构适配层（`internal/providers/`）
具体 Agent CLI 或服务协议的物理翻译官。向组合根导出 `Descriptor` 并实现 `AgentAdapter`。

| 模块目录 | 适配模式与通信机制 | 职责说明 |
|---|---|---|
| **`opencode/`** | **CLI + RPC + 机器级 Service** | 封装 OpenCode V2 协议；内部按单一职责严格拆分为：`adapter.go`（核心适配契约与实例状态管理）、`server.go`（服务子进程启动/健康检查/端口与密码探测）、`stream.go`（SSE 事件流消费泵与多 Subscriber 广播分发；空闲 watchdog 的 idle 判定仅在泵阻塞于读取时生效——publish 正在等待慢 subscriber 的下游反压期间保持守门不复位，避免把"消费者慢"误判为"服务端静默"而强制重拨丢失上游原生事件）、`normalize.go`（原生事件清洗规整化与工具输出截断折叠）；自动从全局 `~/.config/opencode/AGENTS.md` 同步加载平台级系统规约；将原生 `session.text.delta`、`session.reasoning.*` 映射为标准事件流。 |
| **`pi/`** | **常驻子进程 + JSON-RPC** | 以 `pi --mode rpc` 运行；通过 `--append-system-prompt` 原生挂载数据目录全局 `AGENTS.md` 文件指令；通过 stdin/stdout 全双工派发 `prompt`、`abort`、`set_model` 命令；解析 Edit Diff 增量；承接扩展 UI 的 `confirm` → 权限确认映射。pi core 无内置工具审批，平台默认不注入审批扩展，行为与 CLI 一致；仅当 operator 通过 `PI_EXTENSIONS` 提供扩展时才可能出现确认卡片。 |
| **`deepseek/`** | **共享 Host 进程 + WebSocket** | 启动并监控共享的 `dsh` 服务（懒启动、30s 启动超时、支持平台设置空闲回收时长，默认 30min；注入 `DSH_AGENTS_HOME` 原生加载全局指令）；通过 WebSocket 客户端双向转发消息；管理子代理（Subagent）级联关系。 |

---

### 4. 前端应用架构（`web/src/`）

```
web/src/
  ├── api/             ── 网络传输层（client.ts 统管 CSRF，子文件映射后端 RESTful API）
  ├── stores/          ── 全局领域状态（agents, projects, settings, auth, models, chats, quickAssistant, drafts, viewport, ...）
  ├── shared/          ── Provider 中性通用控件与工具（Markdown、PlantUML、Color、Format）
  │     ├── events/    ── 统一事件总线前端设施（event-bus.worker.ts 跨标签页单连接 SharedWorker：连接 error/CLOSED 后 3s 自愈重拨且状态保持 `reconnecting`（触发主线程 reconnectListeners 的带序号重订重放）、busId 所有权仲裁（首个 tab 建立 bus，后续 tab 经 init 回复采纳胜者 busId 并持久化，杜绝多 tab 各持 sessionStorage busId 互踩单连接）；UnifiedEventBusClient.ts 主线程单例与 fallback 本地 EventSource（CLOSED 同样自愈重拨）；event-types.ts 全线事件类型单一来源清单（worker 与 fallback 共享，含 `lifecycle/unknown/ready`，防漂移））
  │     ├── components/── Provider 中性业务通用控件（CronBuilder.vue 可视化 Cron 生成器，支持秒/分/时/日/月/周/年 7 位配置、反解析与后10次执行时间推断；RunningIndicator.vue 呼吸态标识；BackendTag.vue；BackendDiagnostics.vue）
  │     └── ui/        ── 平台标准 UI 组件库（纯中性业务解耦设计，基于 Tailwind CSS）：
  │           ├── atomic/    ── 原子组件（UiButton, UiInput, UiTextarea, UiSelect, UiDropdown, UiBadge, UiProgress, UiTooltip, UiTabs, UiAlert, UiEmpty, UiSpinner, UiDivider）
  │           ├── compound/  ── 复合容器组件（UiDialog, UiFormDialog, UiComposer, UiScrollFollow, UiBubble, UiTree, UiTreeNode）
  │           └── index.ts   ── 统一入口导出
  ├── features/        ── 业务领域垂直拆分目录：
  │     ├── chat/      ── Agent 工作台（核心）
  │     │     ├── composer/      ── 输入框组件、模型选择器、图片附件槽、斜杠命令调度器（slashActionDispatcher.ts）与多源补全、通用附件上传调度器（useAttachmentUploader.ts）与键盘快捷键控制器（useComposerKeyboard.ts）
  │     │     ├── execution/     ── SessionEventStream.ts 事件流消费与状态维护；EventBusClient.ts 会话↔统一总线装配（带序号重订触发 `ReplaySince`/resync 重放）；会话 store 的 applyTurn 在轮次结束时若仍有未收到 final 的流式条目，延迟触发 repairDanglingEntries 按 history final 快照回填（在线丢帧自愈）
  │     │     ├── interactions/  ── 权限确认卡片、表单队列、审批事件派发
  │     │     ├── pane/          ── 会话右侧统一工作区面板管理（已彻底移除最右侧独立导航 Rail，顶层 Tab 栏最右侧集成纯图标布局开关，预览面板头部支持折叠文件树）、左预览右文件树排布、右侧工作区外框总宽度锁定与拖拽（主对话区宽度零跳动）、右侧文件树在面板内靠右展开与向右折叠、useActivityPane 共享视图状态
  │     │     ├── rendering/     ── MessageView.vue 消息/工具卡片渲染（完整承载主对话流思考与工具折叠卡片）、logModel.ts 纯逻辑
  │     │     ├── state/         ── 会话日志推导状态模型（sessionLog.ts）
  │     │     ├── pure/          ── 独立纯 Chat 对话工作台（第二栏专属展示助手列表，第三栏无 Tab 左右双列：左侧主对话面板 + 右侧助手对话清单）
  │     │     └── quick/         ── 快捷助手横向方形工具栏、每次打开新建对话的 480×600 弹窗、自由拖拽定位与最近对话下拉
  │     ├── workspace/ ── 平台三栏主框架（AppShell.vue）、分类导航条（CategoryRail.vue，一级菜单顺序：项目 → 对话 → 任务 → 终端 → 设置）、会话 Tab
  │     ├── projects/  ── 项目侧边栏（ProjectSidebar.vue，统管 Zone A 操作【新建项目/项目设置】/Zone B 检索与过滤/Zone C 树容器）、全局项目列表管理视图（ProjectsManagementView.vue，支持 Tab 中调整项目顺序与显隐，操作栏支持就地改名与解除登记）、项目树（ProjectTree.vue，取消定时轮询，采用本地状态就地响应与按需加载；项目标题行提供铅笔编辑入口）、会话卡片项（ProjectSessionItem.vue）、统一项目弹窗（ProjectManageDialog.vue，register/create/edit/unregister 四种模式，其中 edit 仅修改平台展示名称并只读展示路径）、新建项目弹窗（支持自定义项目名称、可视化路径选择与子目录创建）
  │     ├── proxies/   ── 服务代理管理组件（ProxiesView.vue）、创建/编辑表单弹窗（ProxyEditDialog.vue）、可视化目录选择器（ProxyDirectoryPicker.vue）
  │     ├── settings/  ── 设置中心（SettingsView.vue）：外观设置（AppearanceSettings.vue）、角色管理（SettingsRolesView.vue，多 Agent 专家角色库管理、统一人设与模型、提示词库统一引用、新建专家角色 Agent 后端选择全面对齐任务中心采用胶囊单选组并复用标准 ModelSelect 组件、提示词长描述分离排版与 min-w-0 布局防御）、提示词管理（SettingsPromptsView.vue，分类管理角色类与流程类提示词模版）、大模型配置（ModelSettings.vue）、Agent 全局设置（AgentSettings.vue，全局系统提示词 AGENTS.md 编辑与落盘同步、Tab 隔离后端开关与空闲断开配置）、服务代理设置（ProxySettings.vue）、服务连接（ConnectionSettings.vue）、设置侧边导航（SettingsNav.vue，无缝对齐 AppShell 单头部规范）
  │     ├── terminal/  ── Web 终端工作区（TerminalWorkspace.vue）、xterm 画布（XtermView.vue）、终端侧边栏（TerminalSidebar.vue）
  │     ├── tasks/     ── 计划任务管理、执行面板、任务专属对话弹窗（TasksView.vue）、任务侧边栏（TasksSidebar.vue）、新建面板（TaskCreatePanel.vue，支持普通任务与多 Agent 协同流程创建，中控调度模型基于已配置 Chat 文本模型下拉选取，团队协同角色直接从角色库选用并展示）、协同工作流子目录 `flow/`（`FlowChatView.vue` 核心 IM 群聊协同工作台、`FlowMessageItem.vue` 多角色气泡，内建完备的 `.markdown-body` 排版体系、代码块复制与代码内 `@角色` 保护、`FlowRoleBadge.vue` 角色专属彩色徽标、`FlowRoleManager.vue` 团队角色配置面板，支持从全局角色库导入专家角色并统一引用提示词）
  │     ├── prompts/   ── 提示词管理核心组件与模版逻辑（PromptManageDialog.vue 支持角色类与流程类单选区分、promptTemplate.ts 变量槽替换）
  │     └── files/     ── 文件查看器（FileViewer.vue，支持普通查看与 Diff 双模式）、变更树组件（ChangeTree.vue，树形层级展示变动文件，支持暂存/取消暂存/放弃修改悬浮操作）、本地分支选择器（BranchSelector.vue，支持分支切换与快速新建）、Git 提交操作栏（GitCommitBar.vue，支持 Ctrl+Enter 快捷提交与自动暂存闭环）、Diff 预览器（DiffViewer.vue，Unified Diff 高亮对比与双列行号）、Markdown/PlantUML/图片 Artifact 预览、HTML 源码高亮与“🌐 预览”独立弹窗沙箱预览（HtmlArtifactView.vue、HtmlPreviewModal.vue）
  └── router.ts        ── 路由映射与鉴权守卫（SPA 路由入口）
```

#### 平台标准 UI 组件库与设计系统规范（Data Terminal）
> 详细设计系统规范、色彩 Token、字符对齐律、状态交互法则（The Move）与图标规范详见：👉 [`docs/architecture/design-system.md`](./docs/architecture/design-system.md)

1. **纯中性设计与业务解耦**：
   - 提取的组件严格置于 `shared/ui/`，保持与具体业务领域（Chat、Tasks、Projects 等）彻底解耦。
   - 例如：`UiSelectMenu` 提炼为通用的现代下拉选择器（支持触发器展示主副文本、弹出浮层、搜索过滤、快捷清除与多选/单选无障碍支持，彻底替代各业务模块的裸露原生 `<select>`）；`UiProgress` 抽离为支持 Tooltip 提示的通用进度条组件；`UiComposer` 抽离为通用可拖拽高度的复合输入交互容器；`UiTree` 抽象通用的树形层级交互骨架。
2. **第二栏与侧边栏列表标准规范（基于 design-taste-frontend 深度重构）**：
   - 全平台第二栏（中间导航与对象索引栏：项目、对话、任务、提示词、终端、设置全部 6 个菜单）统一采用**三段式紧凑架构（Header & Actions / Search & Filter / Entity List）**，样式统一由 `shared/styles/sidebar.css` 标准化承载：
     - **Zone A 紧凑 Header 与标准化操作区**：标题高度收敛（13px，`font-weight: 600`），右侧集成 22px 微型方圆角停靠与展开控制按钮（`var(--radius, 4px)`）；操作按钮统一收敛为 26px 极简扁平微圆角按钮，前置 1.5px 几何矢量 SVG 加号图标（消除纯字符 `+`）；设置页彻底移除内部冗余二级标题，与外壳单头部绝对对齐。
     - **Zone B 紧凑检索与微过滤**：统一 26px 极简工具栏，内嵌 1.5px 细线放大镜矢量图标与一键清空按钮，移除输入框聚焦外发光（glow / ring）与外层大按钮，大幅释放首屏纵向阅读高度。
     - **Zone C 紧凑列表与导航卡片（Dense Pitch）**：
       - **层级导航**：项目树由独立的 `ProjectSidebar.vue` 包装，`ProjectTree` 采用两级树状折叠（1.5px 矢量旋转 SVG 折叠角、等宽计数徽标及悬浮快捷加号微按钮）；对话模块第二栏 `ChatSidebar` 则专门聚焦为**纯助手列表**（图标徽标 + 助手名称 + 助手名称旁快捷新建对话按钮 + 会话计数 + 悬浮淡入的编辑/删除操作），下属对话剥离至第三栏右侧对话清单（`ChatHistoryPane`）呈现；对话清单严格按当前选中助手（包含未分组与指定助手）按需分页拉取，底部支持“加载更多”按需追加，保障大量对话场景下的流畅度，并支持单会话删除（Pinia store `deleteChat` 级联自愈）。
       - **实体列表项排版**（覆盖会话项、助手项、对话项、任务项、提示词项、服务器项、设置项）：全量采用标准紧凑卡片（徽标/状态/呼吸指示器 + 语义标题单行截断 + 悬停淡入的 18×18px 快捷操作工具条；属性元数据 + 右对齐 `tabular-nums` 等宽时间戳）。
       - **选中与激活法则（The Move）**：所有列表项激活态严格收敛为**左侧边缘 2px accent 科技蓝实线标尺（`border-l-2 border-l-[var(--color-accent,#4a8ad6)]`）** 与阶梯底色（`var(--surface-muted)`），全平台第二栏彻底清除静止投影（`box-shadow`）与毛玻璃模糊。
       - **色彩预算**：彩色严格控制在 3% 以内，任务与终端状态仅在运行/阻断错误时显色，已完成与常规状态保持中性阶梯底色。
   - **折叠/展开交互按钮**：全平台统一采用紧凑方圆角微按钮（`var(--radius, 4px)`），展开与收起按钮双向镜像对称，消除突兀的长胶囊贴边条与文字大块按钮。
   - **会话级统一右侧工作区与顶层 Tab 布局联动**：彻底移除会话右侧冗余的竖向细导航栏（ChatSideRail），在页面顶层会话 Tab 栏（SessionTabs）最右侧增设极简纯图标的布局控制按钮（`SidebarRight` 图标），统一控制右侧工作区面板（预览 + 文件树）的展开与收起；右侧面板内部采用「左侧文件预览 + 右侧文件树」的习惯排布，文件树可在预览面板头部通过纯图标快捷向右收起与展开；外层总宽度（`previewWidth`）锁定并支持平滑拖拽调整，文件树折叠时左侧预览区自然撑满（`flex: 1`），外框对主对话区的占用宽度分毫不变，彻底消除主对话区文字与代码块的任何跳变和晃动；操作按钮全面纯图标化，去除冗余文字描述，留出最大的代码审阅空间；支持 `hideSideRail` 精简模式，用于任务会话查看弹窗等内嵌场景。
   - **终端操作体验增强**：在线终端（XtermView）原生集成“划选即复制”（Selection Copy）与“右键粘贴”（Contextmenu Paste），提升终端操作流体验。
3. **Data Terminal 极客终端视觉风格与 Token 规范**：
   - 全局遵循 **Data Terminal** 终端工程风格（基于 `design-taste-frontend` 开发者工具标准）：
     - **色彩预算**：视口彩色像素严格在 3% 以内；深石墨灰基调（`--ground: #0f1115`，`--surface: #14171f`），纯平 1px 细线边框（`--border: #222631`），彻底清除静止卡片阴影（`box-shadow`）与毛玻璃模糊（`backdrop-filter: blur`）。
     - **排版与对齐**：全局强制数字等宽（`font-variant-numeric: tabular-nums`），离线内嵌 Google Sans Code 与 Noto Sans SC；严禁全大写宽字距标签（micro-label 禁令）。
     - **圆角收敛**：组件外框与卡片统一收敛为平直微圆角（`--radius: 3px~4px`）。
     - **交互激活（The Move）**：所有 Tab 标签、导航项、范围选择器的激活状态统一使用紧贴底边或侧边的 1px/2px 科技蓝强调线（`--color-accent: #4a8ad6`）。
     - **纯净矢量图标系统**：全站杜绝原生 UI Emoji，统一升级为 1.5px/1.8px 细线条极简几何矢量 SVG 图标。
4. **操作体验规范统一**：
   - 弹窗操作顺序统一规范：取消按钮居左（Secondary 样式），确认/执行按钮居右（Primary 或 Danger 样式）；全量统一使用 `UiDialog` / `UiFormDialog`，消除各业务自造的手写遮罩层；全量支持 Esc 关闭与 Tab 焦点陷阱。
   - 破坏性/危险操作一律采用 Danger 红色警示按钮。
   - 异常状态与重试引导统一使用 `UiAlert`。
```

---

### 5. 桌面客户端外壳架构（`desktop/`）

桌面端外壳由 Electron 驱动，完全解耦于 Web 端代码库与 Go 核心后端，通过同构网络代理透明桥接：

| 模块文件 | 核心职责与架构说明 |
|---|---|
| **`desktop/src/main/index.ts`** | Electron 主进程入口。负责单实例锁校验（防止重复运行）、主窗口几何尺寸与应用图标配置管理、系统生命周期监控（`before-quit` 级联停机）、统一 IPC 通道分发及窗口级快捷键注册。 |
| **`desktop/src/main/icon.ts`** | 桌面端图标路径调度器。提供自适应环境感知（兼容未打包开发调试态、纯 Node.js 测试态及 asar 生产打包态），安全解析窗口与任务栏应用图标路径。 |
| **`desktop/src/main/shortcuts.ts`** | 桌面端窗口快捷键调度器。通过 `webContents` 的 `before-input-event` 在输入事件下发前置阶段精确捕获快捷键，在移除应用菜单栏后稳定支持 `Ctrl+R` / `Cmd+R`（普通重载）、`Ctrl+Shift+R` / `Cmd+Shift+R`（忽略缓存强制重载）及 `F5` 刷新。 |
| **`desktop/src/main/service-manager.ts`** | 本地 Go 服务端（Sidecar）生命周期调度器。在本地模式下派生 `bin/platform` 子进程，监听回环地址随机端口（`-listen 127.0.0.1:0`），自动从日志中探测绑定端口并做 `/api/health` 可达性校验；应用关闭时发送 SIGTERM 优雅退出，并在 5 秒超时后兜底回收，彻底消除孤儿进程。 |
| **`desktop/src/main/proxy.ts`** | 本地透明反向代理（Desktop Local Proxy）。主进程在 `127.0.0.1:DESKTOP_PORT` 启动轻量高性能代理，前端无论连接本地还是远程后端均访问该同源端口；代理负责 HTTP/SSE 请求透明转发、静态文件与 SPA 路由直接响应，并在 WebSocket 握手时自动重写 Host 与 Origin，保证终端 `checkSameOrigin` 与 CSRF 双提交校验 100% 同构兼容。 |
| **`desktop/src/main/connection-store.ts`** | 多环境连接配置与安全凭据管理。持久化存储“本地单机内置服务”及“远程服务器”连接列表，利用 Electron 原生 `safeStorage`（系统 Keychain / DPAPI）对敏感凭据进行操作系统级硬件加密存储。 |
| **`desktop/src/preload/index.ts`** | Preload 安全上下文隔离网桥。向渲染进程安全暴露只读中立契约 `window.desktopAPI`，包含获取连接、保存配置、切换连接、连通性探测及原生目录对话框。 |

**发布打包与图标规范**：
- **发布产物**：`./release.sh`（`make release`）一次产出两类产物并统一生成 `SHA256SUMS`：① Web 服务端包（Linux amd64/arm64、Windows amd64，内含二进制 + `web/dist` + 运维指南）；② 桌面安装包（`electron-builder`：Linux AppImage/tar.gz、Windows zip），包内以 extraResources 内嵌同架构 Go sidecar（`resources/bin/platform*`）与 `web/dist`，安装后即可本地自带后端运行。桌面应用版本默认为 `desktop/package.json` 的 `version`；发布标签为语义化版本（如 `./release.sh v0.2.0`）时自动对该版本。`web/dist` 只构建一次、两类产物共用；`--skip-desktop` / `--skip-server` 可只构建其中一类。
- **跨平台应用图标（Desktop）**：打包资源统一收敛于 `desktop/build/`（`icon.png` 对应 Linux，多尺寸 `icon.ico` 对应 Windows，`icon.icns` 对应 macOS），由 `electron-builder` 注入各平台可执行文件与快捷方式；运行时窗口图标位于 `desktop/assets/icon.png`，由 `files` 字段打包进 `app.asar` 并在主进程中加载。Windows 产物针对 Linux 交叉构建环境缺失 Wine 的问题，通过 `afterPack` 钩子（`desktop/scripts/after-pack.js`）利用纯 JavaScript PE 资源库 `resedit` 原生改写 `AI-Dev-Platform.exe` 的 PE 图标资源组（Group ID 1 / 1033），确保生成的可执行文件脱离 Electron 默认图标。
- **浏览器标签页图标（Web Favicon）**：存放在 `web/public/`（`favicon.ico` 与 `favicon.png`），经 Vite 构建自动分发至 `web/dist/`，由 `web/index.html` 声明并在 Go 静态服务与桌面透明代理中直出。

---

## 五、核心业务流程与端到端时序流转全景

### 1. Agent 消息交互与多路事件驱动流（Message Turn Flow）

```
[ 用户在输入框按 Enter ]
           │
           ▼
1. Web 前端 (SessionChatPanel.vue / send())
   - 提取输入文本、图片附件与选中模型
   - 调用 api/sessions.ts 的 sendProjectSessionMessage()
           │
           ▼ (HTTP POST 经 client.ts 自动注入 X-CSRF-Token 与 Session Cookie)
2. 传输层 (router.go / RequireCSRF 中间件)
   - 验证 Double-Submit CSRF 令牌一致性
   - 路由分发至 sessions.handleSendMessage
           │
           ▼
3. 会话领域服务 (internal/app/sessions/service.go)
   - 检查会话归属，校验模型能力
   - 标记当前会话为 TurnRunning
   - 唤醒底层 instances.Service 确保适配器进程存活
           │
           ▼
4. 异构适配层 (internal/providers/*)
   - 将标准消息请求转化为后端专有命令 (如 pi 的 rpcCommandPrompt)
   - 捕获后端原生输出 (stdout / WebSocket)
   - 适配器将原生事件转化为标准 NormalizedEvent:
       • EventTurn (started)
       • EventMessage (text_delta, reasoning_delta)
       • EventThinking (思考内容流式输出)
       • EventToolCall (各后端提问工具如 ask_user_question 由适配器统一归一化为标准 question 契约；大入参由 core/agents.PruneToolInput 瘦身裁剪) / EventToolResult (输出由 core/agents.SummarizeToolResult 瘦身为紧凑语义化中文摘要，如“读取了 xxx 文件”、“编辑了 xxx 文件”、“执行了 xxx 命令”，清空大 diff metadata；问答工具保留完整答案 payload 供前端结构化解析)
       • EventTurn (ended, completed/failed/cancelled)
           │
           ▼
5. 统一事件网关接入 (internal/app/events/ & internal/app/sessions/event_bus.go)
   - 将标准化事件以 channel="sessions", targetId=sessionID 封装并广播至平台统一事件网关
   - 溢出与断连可靠性闭环：缓冲满载承压时保底帧/流内容帧 100ms 入队等待，超限丢帧计数并按策略 kick 踢断该 bus 的 SSE（关键帧丢即踢、流内容帧每 8 次丢、累计每 64 次）；客户端 SharedWorker/主线程线程自愈重拨后带 lastEventId 重订，服务端按 `instances.ReplaySince` 环形重放补帧或推 resync 触发前端全量 history 重建；每帧写超时防 TCP 停滞；`/api/events/diagnostics` 暴露丢帧观测
           │
           ▼ (经 /api/events/bus/:uuid 单连接由前端 SharedWorker 跨标签页共享多路复用分发)
6. 前端执行引擎 (features/chat/execution/SessionEventStream.ts & rendering)
   - 消费 SSE 事件帧，更新 sessionLog 状态
   - 问答交互工具与纯净对话流（MessageView display="messages"），完整承载用户消息与答复、人机问答交互卡片（已将思考过程与工具调用过程移出会话对话流，保持高信噪比与高阅读舒适度）；答案在所有题目输出完成后于底部统一展示，解析 JSON 答案并以 1 2 3 序号每行逐项对应
   - 主对话流动态运行状态指示（MessageView kind-running）：移除原有静态“正在运行...”，采用 deriveRunningAction 基于最新活动动态解析动作语义（如“正在思考...”、“执行 <cmd>”、“编辑 <path>”、“读取 <path>”、“搜索 <query>”等）并以微动效卡片实时呈现
   - 右侧面板整合为统一工作区面板（左侧为文件预览与 Diff 比对，右侧为文件树），由顶层 Tab 栏最右侧纯图标布局按钮统一控制显隐；文件树在右侧面板内展开或向右折叠，内部弹性伸缩（flex: 1 撑满），外框总宽度恒定锁定，绝不占用或晃动主对话窗口宽度；操作按钮全面纯图标化。
   - 会话历史读取瘦身（GET /projects/{id}/sessions/{sessionId}/history）：会话领域服务统一对适配器历史事件进行按需瘦身过滤，仅保留主对话视图所需的消息（EventMessage、EventError）以及成对的问答交互工具（IsQuestionTool 识别之 EventToolCall 及配套 EventToolResult），海量思维链 EventThinking 与非问答类工具事件均不透传，进一步压降前后端交互体量
   - 会话视口状态记忆（Session & Chat Viewport State Restoration）：通过 useViewportStore 独立记录每个会话主消息列的滚动条位置（scrollTop）与粘底状态（pinned）。当用户切换菜单、路由或切换 Tab 时，离开前自动固化当前视口状态；再次切回该会话时：若此前处于非粘底状态（如翻看历史中间某段），无缝精确恢复到离开时的 scrollTop 与悬浮“跳至最新”状态，绝不无故强制重置滚到底部；若此前处于粘底状态或首次打开，则平滑跟随最新内容。
   - 轮次结束时将会话状态恢复为 TurnIdle
```

---

### 2. 人机协同工具授权闭环流（Interactions Flow）

```
[ Agent 执行高危操作：命令执行 / 文件修改 / 提问 ]
                      │
                      ▼
1. 适配器层 (internal/providers/*)
   - 捕获后端的审批请求，构造 agents.PermissionRequest
   - 产生标准 EventInteraction (requested, 携带表单或权限载荷)
                      │
                      ▼
2. 会话领域层 (internal/app/sessions/)
   - 记录会话待处理交互项 (Interactions Snapshot)
   - 经 event_bus 广播至 SSE 事件流
                      │
                      ▼
3. 前端交互层 (features/chat/interactions/)
   - 收到 interactions 帧，交互队列挂起当前轮次
   - 在主界面弹出 Permission 审批卡片或 QuestionForm 表单
                      │
                      ▼
[ 用户点击“允许执行”或提交表单 ]
                      │
                      ▼
4. 传输与响应 (api/interactions.ts / mutate('POST'))
   - 调用 POST /api/projects/{id}/sessions/{sid}/interactions/permission/reply
   - 注入 CSRF 令牌发送审批结果 (once / always / reject)
                      │
                      ▼
5. 适配器解除阻塞
   - 适配器扩展接口 (PermissionReplier) 接收应答，通知 Agent 子进程恢复执行
   - 产生 EventInteraction (resolved, 带 id)，前端关闭弹窗
```

---

### 3. Web 终端交互与 PTY 全双工字符流（Terminal Flow）

```
[ 用户在左侧导航进入“终端”，选择本地终端或 SSH ]
                         │
                         ▼
1. Web 前端 (features/terminal/XtermView.vue)
   - 初始化 xterm.js 实例并挂载 FitAddon
   - 发起 WebSocket 连接: ws://host/api/terminal/ws/{serverId}?cols=80&rows=24
                         │
                         ▼ (GET 请求带 Origin 与 Session Cookie)
2. 传输与同源防护 (internal/app/terminal/handlers.go)
   - 执行 checkSameOrigin 严格比对 Origin 与 Host 是否一致 (杜绝 CSWSH)
   - 校验 SessionCookie 鉴权
   - 协议升级为 WebSocket 连接 (Upgrader)
                         │
                         ▼
3. 终端领域服务 (internal/app/terminal/service.go)
   - 本地模式：调用 core/platform 分配伪终端 (PTY)，派生交互式 bash/sh 进程
   - 远程模式：解密 secretStore 凭据，建立 SSH Client Session
                         │
                         ▼
4. 双向全双工转发 Goroutines 与生命周期治理
   - Goroutine A：循环读取 PTY 字符流 ──► 写入 WebSocket ──► xterm.js 屏幕刷新
   - Goroutine B：循环读取 WebSocket 击键 ──► 写入 PTY 进程 stdin
   - 窗口伸缩：前端 ResizeObserver 捕获尺寸 ──► 发送二进制帧 ──► 调用 PTY.Resize()
   - 终端工作区常驻保活（Persistent Terminal Workspace）：Web 终端工作区（TerminalWorkspace）在主框架 AppShell 中常驻挂载保活，通过 v-show 与 visible 属性管理显隐状态，避免切换左侧菜单导航时触发组件卸载与 WebSocket 连接断开。切换离开时正在运行的后台命令持续执行，切回时自动触发 FitAddon 自适应尺寸与焦点重获；仅在用户显式在 Tab 栏点击关闭按钮时才销毁连接并释放资源。
   - Linux PTY 异步进程回收与杜绝僵死（Zombie / Defunct Reaping）：在 core/platform/unix 的 PTYSession 启动时派生专有异步回收协程（cmd.Wait()），并在 Close() 阶段执行优雅终止与等待同步闭环；不论是用户主动退出、断开连接还是进程被 kill，子进程退出状态均由内核及时消费，杜绝任何 <defunct> 僵死进程残留。
```

---

### 4. 纯 Chat 通用大模型流式对话流（Pure Chat Flow）

```
[ 用户在“Chat”分类下新建或进入模型对话 ]
                    │
                    ▼
1. Web 前端 (features/chat/pure/ChatPanel.vue + ChatHistoryPane.vue + ChatSidebar.vue)
   - 布局架构：第二栏专属展示助手列表；第3栏移除 Tab，组织为左右两列（左侧对话主面板，右侧助手对话清单且顶部支持助手下拉切换与新建会话），双向响应式联动
   - 选用 OpenAI 兼容的模型配置 (model_id)
   - 发送文本及多张图片 (最大支持 32MiB 请求体)
   - 输入区 ChatComposer.vue 与 Agent 会话输入区同构：发送按钮为分裂按钮（主半边发送/停止，右半边下拉选 Enter 或 Ctrl+Enter 发送），
     快捷键选择由 features/chat/composer/sendShortcut.ts 统一持久化，两个入口共用同一个提交键
                    │
                    ▼
2. 后端纯 Chat 服务 (internal/app/chat/)
   - 校验会话归属，存储用户消息记录
   - 委托 modelconfig 解析模型服务地址与 API Key
   - 调用 completer.go 发起带 context 的流式 HTTP 调用
                    │
                    ▼ (SSE 数据切分与事件总线分发)
3. 统一网关事件推送 (internal/app/events/ & internal/app/chat/service.go)
   - stream.go 以 SSE 格式逐块切分并持久化 Assistant 消息
   - 经 events.Manager 聚合至全局单一 SSE 管道 (/api/events/bus/:uuid，channel="chat")
   - 前端通过统一事件总线（UnifiedEventBusClient / SharedWorker）跨标签页复用单条长连接，主面板与快捷助手基于 chatId 动态订阅，消除每会话独立连接开销
   - 若对话无标题，触发 core/textutil/title.go 自动提炼标题并落库
```

---

### 5. 多 Agent 协同流程与 IM 群聊调度流（Collaborative Flow Task & Multi-Agent IM Turn Flow）

面向复杂复合研发场景，平台提供以平台内置 AI 大脑作为中枢调度核心、多个专业 Agent 角色分工闭环执行的协同工作流系统：

```
[ 用户在任务中心创建/触发多 Agent 协同流程任务 ]
                      │
                      ▼
1. Web 前端 (features/tasks/flow/FlowChatView.vue)
   - 展现为团队协同 IM 群聊对话流
   - 经由统一事件网关（channel="tasks", targetId=runId）多路复用订阅流式事件，不再独占独立长连接
                      │
                      ▼
2. 协同流程中枢引擎 (internal/app/tasks/flow/engine.go)
   - 提取协同总目标，初始化 FlowRun 与首条用户目标消息
   - 启动基于 Function Calling 工具驱动的 ReAct 自主调度状态机
                      │
                      ▼
3. 内置大脑规划与工具决策 (Orchestrator Step / FlowTools)
   - 动态装配可用角色清单（角色名、职责、后端、模型、系统提示词及历史会话 ID）
   - 调用中控模型流式生成分析与规划，经 EventBus 广播打字机动画
   - 自主决策并发起标准 Function Calls 工具调用：
       • flow_dispatch_role: 指派角色执行（支持新建会话或传入 session_id 复用会话多轮连续对话）
       • flow_wait_session:  显式等待角色会话执行完毕并汇总结果
       • flow_ask_user:      遇到重大疑问或需人工确认时向群聊提问并挂起
       • flow_complete_task: 所有子任务合格且目标达成时提交最终报告
                      │
                      ├─────────────────────────────────────────┐
                      ▼                                         ▼
         【A. 纯推理型角色 (llm)】                   【B. 工作区 Coding Agent 角色】
         - 依赖 chat.Completer                      - 依赖 sessions.SessionLauncher
         - 携带角色设定与当前任务指令              - 启动或复用项目工作区 Agent 会话
         - 流式补全输出分析或设计方案              - 真实修改文件、运行自测
                      │                                         │
                      └────────────────────┬────────────────────┘
                                           │
                                           ▼
4. 流程数据采集总线 (internal/app/tasks/flow/telemetry_bus.go)
   - 挂接各 Agent 会话事件流（PipeLiveStream），实时将流式增量推送到群聊前端
   - 两阶段状态机监听（Running -> Idle），会话结束时自动提取最终回复/错误摘要
   - 封装为结构化 TurnOutcome，以 tool 响应形式确定性回填给中枢大脑
                                           │
                                           ▼
5. 中枢大脑闭环评估与验收 (Orchestrator Evaluation)
   - 观察工具回传的角色执行产出，决定推进方向：
       • 需继续推进：调用 flow_dispatch_role 指派下一个角色或对同一角色继续追问
       • 目标圆满达成：调用 flow_complete_task 提交最终成果总结，标记 FlowRun 为 completed
       • 需用户决策：调用 flow_ask_user 挂起流程为 waiting_user
       • 达到最大步数上限 (默认 20 步)：自动挂起等待人工干预
                                           │
                                           ▼
6. 人机协同介入 (Human-in-the-Loop)
   - 用户在 IM 对话底部随时发言插话，作为最高优先级指令唤醒中枢大脑进入重规划
```

---

### 6. 平台级 Agent 诊断与模型快照流（Diagnostics & Model Snapshot）

- **诊断归属**：`GET /api/agent-backends/diagnostics` 是平台级接口。前端只在「平台设置 → Agent」中展示：每张 Agent 卡片内提供可折叠诊断详情，`warning` / `error` 默认展开。项目设置不再读取诊断，也不再展示 machine service 提示。
- **模型持久化与治理**：`internal/app/agentmodels` 管理平台级模型快照与治理。`agent_models` 保存每个 backend 的模型行（含 `enabled` 开关、显示名称、上下文上限等个性化配置），`agent_model_catalogs` 保存最后同步时间；唯一索引为 `(backend, model_id)`。
- **读取**：`GET /api/agent-backends/{backend}/models` 只读数据库，不调用 Agent；Agent 离线时仍返回上次成功快照。从未刷新返回 `models: []` 与 `updatedAt: null`。
- **手动刷新与增量合并**：`POST /api/agent-backends/{backend}/models/refresh` 调用适配器 `ListModels(ctx, "")`。刷新采用**增量差量合并（Upsert/Merge）算法**：保留用户已配置的启用/停用状态及修改后的名称与上下文限制，补充/更新后端新模型或属性，清理已废弃模型。刷新失败保留旧快照；空结果清空模型但更新同步时间。
- **模型配置修改与启用/停用**：`PATCH /api/agent-backends/{backend}/models`（支持带 `/*model` 或请求体指定 `modelId`）支持更新指定模型的启用状态、自定义名称、上下文上限 Token 限制及多模态图片输入支持。前端 Agent 设置页提供快捷启停 Checkbox 与模型编辑抽屉。
- **下游消费端过滤**：工作台会话模型选择器（`ModelSelect.vue`）与计划任务创建器（`TaskManageDialog.vue`）自动过滤 `enabled: false` 的模型，只向终端用户展示已启用的模型；若历史选中的模型被停用，选择器平滑回退至首个可用模型。
- **刷新失败可观测性**：HTTP 响应只给稳定错误码（`model_list_failed` / `service_not_running` / `unsupported_capability`），适配器返回的底层原因（例如 deepseek 共享宿主启动超时）在服务端日志以 `agent models: refresh <backend> failed: ...` 记录并做密钥脱敏；排障时先看这条日志，不要只看 access log 的 502。
- **deepseek 宿主启动预算**：deepseek 的刷新会懒启动共享 `dsh` 宿主，启动超时默认 30s（`dsh` 自身打印 launch URL 实测 4.9~5.3s，因此不可下调到秒级；可用 `DEEPSEEK_STARTUP_TIMEOUT` 覆盖）。参见 `docs/agent-compatibility.md` 的 deepseek transport 小节。
- **刷新策略**：无 TTL、无后台刷新、无轮询、无定时任务。只有运维员在 Agent 设置页点击“刷新”才会同步。空目录表示平台级/后端默认作用域，不考虑项目目录级模型配置。
- **边界**：Agent 模型属于各 Agent 后端的模型字典，支持平台级启停与配置覆盖；平台「模型」设置中的 OpenAI 兼容模型服务保持独立，服务于纯 Chat 对话。

---

### 7. 平台级 Agent 全局系统提示词流（Global Agent System Prompt / AGENTS.md）

```
[ 设置 → Agent → 全局设置 (编辑并保存系统提示词) ]
                       │
                       ▼
1. 平台设置 (internal/app/settings)
   - 权威落盘：直接写入平台数据目录 dataDir/AGENTS.md，数据库 settings 表持久化双重保障
   - 跨平台普通文件同步：以标准安全写入（非软链接，彻底杜绝 Windows 1314 特权限制）同步写入 OpenCode 全局路径 ~/.config/opencode/AGENTS.md
   - REST 接口：GET /api/settings/agent-general 与 PUT /api/settings/agent-general
                       │
                       ├──────────────────────────┬──────────────────────────┐
                       ▼                          ▼                          ▼
2. OpenCode 适配器 (opencode)             3. Pi 适配器 (pi)             4. DeepSeek 适配器 (deepseek)
   - 机器级守护服务内建 Watcher 自动热重载   - 启动 RPC 子进程时参数追加：  - 派生 dsh 进程时环境变量注入：
   - 原生监视 ~/.config/opencode/AGENTS.md   `--append-system-prompt`       `DSH_AGENTS_HOME=<dataDir>`
   - 会话创建与消息流零代码侵入            指向数据目录 AGENTS.md 文件      - dsh 原生直接加载数据目录指令
                       │                          │                          │
                       └──────────────────────────┴──────────────────────────┘
                                                  │
                                                  ▼
5. 会话工作台与主对话流（高信噪比与零注入风险保证）
   - 系统提示词直接进入各 Agent 内核最高优先级系统级上下文（System / Harness Instructions）
   - 会话创建与轮次调度零代码侵入，主对话流（用户消息气泡、历史记录回放）100% 保持纯净，绝无拼接与泄露
```

---

### 8. 快捷助手漂浮对话流（Quick Assistant Flow）

```
[ 设置 → 外观 → 开启“显示快捷助手漂浮按钮” ]
                    │
                    ▼
1. 平台设置 (internal/app/settings)
   - 读写 settings 表 quick-assistant.config，默认关闭
   - GET/PUT /api/settings/quick-assistant
                    │
                    ▼
2. 主框架 (web/src/features/workspace/AppShell.vue)
   - 开关开启时挂载 QuickAssistantLauncher
   - 中列折叠/展开由第一栏右边缘的 rail-edge-toggle 把手统一控制（CategoryRail 不再自带折叠按钮）
   - 工具栏呈现为横向方形按钮组，默认在视口右上角贴边固定，上下左右无边距限制且支持自由拖拽到界面任意位置，默认半透明，鼠标悬停完全显示；位置独立持久化
                    │
                    ▼
3. 弹窗 (features/chat/quick/QuickAssistantPanel.vue)
   - 弹窗位置与工具栏完全解绑，支持按住 Header 在视口内自由拖拽定位（带 12px 边界保护与独立持久化）
   - 以 480×600 打开所选助手的对话弹窗；每次打开（含切换助手）都进入新的空白对话草稿
   - 首条消息发送时才在助手分组内创建会话记录，避免产生空会话；关闭再打开不会恢复上一次会话
   - Header 操作区支持最近 5 条对话下拉选择切换与开启新对话；历史会话由用户在下拉中自行选择
   - 显示所选助手的名称、图标与专属模型，独立 quickAssistant store
                    │
                    ▼
4. 纯 Chat 领域 (internal/app/chat)
   - 助手具备默认模型、系统提示词、自定义图标和浮窗配置
   - 新建对话在指定助手下创建，系统提示词隐式注入模型上下文且不污染消息列表
   - 彻底解除历史固定分组锁定限制，所有助手平级可管理
   - 主 Chat 页面与弹窗各自维护 SSE 流，互不关闭
```

---

### 9. 服务代理与 ContextPath 动态分发流（Service Proxy & Gateway Flow）

```
[ 客户端访问 http://host:port/proxy/:alias/*subpath ]
                           │
                           ▼
1. 传输路由编排 (internal/transport/http/router.go)
   - 路由优先命中 /proxy/*any，先于 SPA 静态回退 (NoRoute) 处理
   - 检验尾部斜杠：若为裸别名 /proxy/:alias，返回 301 永久重定向至 /proxy/:alias/，确保相对资源正常解析
                           │
                           ▼ (免登录公开直连，数据面无 Session 校验)
2. 动态网关分发器 (internal/app/proxy/gateway.go)
   - 提取 alias，自 service/repository 查找匹配且处于 Enabled 状态的代理规则；不存在或未启用直接返回 404
   - 分支 A：静态目录 (directory)
       • 防穿越安全保障：目标路径由 paths.Resolve 规范化绝对路径，通过 http.Dir + http.StripPrefix 进行静态文件服务
       • 默认索引与目录树：目录下有 index.html 则直接渲染；无 index.html 则自动生成安全文件树浏览列表，支持直接点击下载
   - 分支 B：端口反代 (port)
       • WebSocket 升级侦测：若标头包含 Upgrade: websocket，通过 c.Writer.Hijack() 升级原生 TCP 连接，全双工双向 io.Copy 透传 (满足 Vite/Webpack HMR)
       • HTTP 反向代理：通过 httputil.ReverseProxy 透明代理至 127.0.0.1:<target_port>，自动剥离 /proxy/:alias 前缀并追加 X-Forwarded-Prefix: /proxy/:alias
                           │
                           ▼
3. 控制面治理 (internal/app/proxy/handlers.go & web/src/features/proxies)
   - 代理管理接口 (/api/proxies) 强制继承 RequireSession 与 RequireCSRF 双提交防护
   - 支持实时连通性探测 (net.DialTimeout 200ms 检测回环端口是否处于监听状态)
   - 前端提供一键在新标签页打开、剪贴板一键复制链接、在线状态徽标与开关启停
```

---

### 10. 桌面应用双模连接与本地透明代理流（Desktop Dual-Mode & Proxy Flow）

```
[ 用户启动桌面客户端 / 进入“设置 → 服务连接”切换服务器 ]
                         │
                         ▼
1. 桌面壳主进程 (desktop/src/main/index.ts)
   - 执行单实例互斥锁判定 (requestSingleInstanceLock)，防多进程竞争；
   - 读取 ConnectionStore 配置；若当前激活目标为本地 (local)，唤醒 LocalServiceManager；
   - LocalServiceManager 自动派生 bin/platform 并分配 127.0.0.1 动态端口，等待健康探测就绪。
                         │
                         ▼
2. 本地透明反向代理 (desktop/src/main/proxy.ts)
   - 在 127.0.0.1:DESKTOP_PORT 启动轻量透明网关，主窗口加载该同源地址；
   - 静态资源：直接由代理从本地打包的 web/dist 毫秒级托管响应 (含 SPA NoRoute 兜底)；
   - 数据面与 API 请求：
       • 模式 A (本地)：请求原样管道透传至 127.0.0.1:LOCAL_PORT；
       • 模式 B (远程)：请求转发至目标远程服务器，主进程自动注入凭据并动态重写 Host 与 Origin，
         不仅彻底规避了浏览器 CORS 与 SameSite=Strict Cookie 限制，更使终端 WebSocket 的
         checkSameOrigin 校验 100% 契约一致通过；
       • WebSocket 升级：原生全双工管道透传，完美承载在线交互式终端与开发热更新。
                         │
                         ▼
3. 渲染进程与连接管理器 (features/settings/ConnectionSettings.vue & shared/desktop.ts)
   - 通过 preload 注入的 window.desktopAPI 暴露连接增删改查、连通性实时探测与动态切换；
   - 用户切换目标连接时：原子化重置前端活动 Tab、断开正在运行的 SSE 与 WebSocket 连接，清空
     CSRF/Auth Store，平滑过渡至目标服务环境，保证状态绝对物理隔离。
```

---

### 9. 用户认证鉴权与本机免命令行首次初始化流（First-Run Localhost Setup & Auth Flow）

```
[ 平台首次启动且数据库未配置密码 ]
                 │
                 ▼
1. 访问来源探测 (internal/app/auth/handlers.go)
   - 客户端访问 GET /api/auth/session
   - 服务端提取底层 TCP 连接 remote address (c.Request.RemoteAddr) 并校验 net.ParseIP().IsLoopback()
   - 若 !authenticated 且 !passwordConfigured 且 isLoopback:
       • 响应 sessionResponse 携带 { authenticated: false, needsSetup: true, csrfToken: "..." }
   - 若来自非本机（外部网络/公网）：
       • needsSetup 保持 false，绝不向外部网络泄漏系统未初始化的脆弱状态
                 │
                 ▼
2. Web 前端渲染 (views/LoginView.vue & stores/auth.ts)
   - Auth store 解析 session 响应，检测到 needsSetup === true
   - 页面自动切换为“首次平台密码初始化”引导模式（提示：检测到首次在本机启动，请设置初始访问密码）
   - 表单呈现“设置平台密码”与“确认密码”两个输入框，前端校验两次输入一致性
                 │
                 ▼ (用户点击“设置密码并登录”，经 client.ts 自动注入 X-CSRF-Token)
3. 首次密码持久化 (POST /api/auth/setup)
   - 传输层 RequireCSRF 中间件执行 Double-Submit 校验
   - handler 再次强制校验 isLocalClient(c)，非本机直接 403 Forbidden (local_access_only)
   - 校验密码非空与原子防重 (若已设置过立即返回 409 Conflict password_already_set)
   - 执行 Argon2id 哈希持久化 (auth.SetPassword)
   - 自动登录闭环：直接为当前连接创建会话 (s.sessions.Create)，植入 Session Cookie 并签发新 CSRF Token
                 │
                 ▼
4. 前端状态流转
   - 接收到已认证的 sessionResponse (authenticated: true)
   - Auth store 状态更新为已登录，路由无缝跳转至主工作区 (home)
   - 命令行 platform set-password 保留作为无浏览器/无头服务器运维的兼容兜底
```

---

### 11. 全平台统一事件总线与跨标签页连接收敛体系（Unified Event Bus & SharedWorker Architecture）

针对浏览器对同域名并发长连接限制（最多 6 个）导致的“长连接过多引发页面假死/请求挂起”顽疾，平台在传输层构建了全局统一的事件网关体系，彻底消灭分散的烟囱式长连接：

```
【多标签页浏览器环境】                                               【服务端传输与领域层】
 ┌──────────────────────────────────────────────┐                ┌───────────────────────────────┐
 │ 标签页 A (Tasks)   标签页 B (Chat/Session)    │                │ 领域服务                      │
 │      │                  │                    │                │ (sessions / chat / tasks)     │
 │      └────────┬─────────┘                    │                └──────────────┬────────────────┘
 │               │ 内部消息通道 (MessagePort)     │                               │ 内部发布领域事件
 │               ▼                              │                               ▼
 │ ┌──────────────────────────────────────────┐ │                ┌───────────────────────────────┐
 │ │  SharedWorker 共享线程                   │ │ 全浏览器唯一   │  全局统一事件网关             │
 │ │  (event-bus.worker.ts)                   │ │   SSE 长连接   │  (internal/app/events/)       │
 │ │  - 维持唯一直连长连接                     │◄╞═══════════════╡│  - 维护 clientBus 映射        │
 │ │  - 跨页合并订阅 / 去重广播                │ │  (/api/events/ │  - 统一动态订阅 / 退订中枢    │
 │ └──────────────────────────────────────────┘ │   bus/:uuid)   └───────────────────────────────┘
 └──────────────────────────────────────────────┘
 ┌──────────────────────────────────────────────┐                ┌───────────────────────────────┐
 │ 控制台终端面板 (Terminal Pane)                │ 专用独立 WS   │  PTY 终端管理器               │
 │ (按需开启，关闭即销毁)                        │◄══════════════╡│  (/api/terminal/:id)          │
 └──────────────────────────────────────────────┘                └───────────────────────────────┘
```

- **核心架构原则**：
  1. **终端物理隔离**：控制台终端（PTY）包含高频 ANSI 控制字符与流式键盘输入，维持独立的物理 WebSocket，严防海量终端日志阻塞业务指令与推送通道。
  2. **下行统一多路复用**：除终端外的所有业务状态推送（工作台会话 `sessions`、纯对话 `chat`、协同流程任务 `tasks` 及系统通知 `system`），全部多路复用收敛到单一 SSE 管道：`GET /api/events/bus/:uuid`。
  3. **前端 SharedWorker 跨页单连接共享**：前端引入 `web/src/shared/events/event-bus.worker.ts`。无论用户在浏览器中打开多少个标签页，整个浏览器进程与后端**始终只保持 1 条 SSE 物理连接**；各标签页通过 `MessagePort` 接入该 Worker，由 Worker 实现跨页订阅合并去重与精准消息分发。
  4. **优雅降级（Graceful Degradation）**：针对不支持 SharedWorker 的特殊无痕模式沙箱或 Vitest 测试环境，前端自适应降级为单标签页主线程直连，对外保持 100% 相同的前端调用契约。
  5. **统一信封协议（EventEnvelope）**：下行数据统一格式：
     ```json
     {
       "busId": "bus-uuid",
       "channel": "sessions" | "chat" | "tasks" | "system",
       "targetId": "s1" | "1024",
       "eventType": "delta" | "status" | "turn" | "interactions" | ...,
       "timestamp": 1700000000000,
       "payload": { ... }
     }
     ```
  6. **安全规范对齐与轻量快照重连（策略 A）**：
     - 动态订阅/退订接口（`POST/DELETE /api/events/bus/:uuid/subscriptions`）保持在主线程通过 `client.ts` 的 `mutate()` 发送，严格受 CSRF 传输层双提交令牌与 Session 会话保护。
     - 当客户端因网络波动重连成功时（触发 `reconnected`），各业务 Store 对当前正处于激活展示态的实体（如当前会话、当前流程任务）主动调用现有 HTTP GET 接口刷新一次全量快照，实现零后端内存堆积的高可靠断线自愈。
  7. **废弃旧路由（零兼容包袱）**：
     已彻底废除并移除原各自烟囱式构建的私有总线与长连接端点：
     - 原会话总线：`/api/events/:uuid`、`/api/events/:uuid/subscriptions`
     - 原纯对话总线：`/api/chat-events/:uuid`、`/api/chat-events/:uuid/subscriptions`
     - 原协同流程任务端点：`/api/tasks/flows/:runId/events`

---

## 六、API 传输层与安全规范（ADR-0001）

前端与后端的数据通信拥有严格的安全架构约束：
1. **统一通信契约**：
   前端所有 API 请求必须通过 `web/src/api/client.ts` 暴露的 `get()` 和 `mutate()` 发送：
   - 读操作使用 `get<T>(url)`；
   - 写操作（`POST`/`PUT`/`DELETE`/`PATCH`）统一使用 `mutate<T>(method, url, body)`。
2. **CSRF 双提交令牌防御**：
   - 全局服务端在 `router.go` 挂载 `auth.RequireCSRF()` 中间件；
   - 令牌由传输层（`client.ts`）统一从 `auth` store 中拉取并自动轮转，禁止任何业务代码直接 `fetch`、手写 `X-CSRF-Token` 或将 `csrfToken` 作为函数参数传递；
   - 前端守卫测试 `web/src/tests/csrf-coverage.spec.ts` 会在源码静态分析层面拦截违规调用；后端 `csrf_matrix_guard_test.go` 遍历所有非安全路由测试防御有效性。

---

## 七、新增 Agent 后端标准步骤

新增一个 Agent 后端无需改动任何前端代码，仅需以下 6 步：
1. 在 `internal/core/agents/contracts.go` 中为 `BackendID` 增加常量定义，并添加到 `BackendID.Valid()` 白名单中。
2. 新建适配器包 `internal/providers/<name>`，实现 `agents.AgentAdapter`（包含显式声明的 `Capabilities()`）。
3. 按需实现可选扩展接口：`PermissionReplier`、`HistoryReader`、`ServiceController`、`RawEventOpener`。
4. 在该适配器包导出 `Descriptor`（声明可执行名、色调/品牌色、展示顺序等），并编写 `descriptor_test.go`。
5. 在 `cmd/platform/main.go` 中注册适配器并加入 Catalog；启动时 `checkBackendWiring` 会自动校验注册表与 Catalog 的一致性。
6. 更新 `docs/agent-compatibility.md` 能力兼容矩阵。

---

## 八、测试体系与验证规范

每次代码变更后，必须确保以下质量门禁全部通过：

```bash
# 1. 运行核心集成检验（包含 vet、Go 全量单测、前端 Vitest 单测、前端类型检查、前端生产构建）
make verify

# 2. 运行端到端集成测试（Playwright E2E，全量套件）
make e2e
```

- **E2E 运行环境规范**：
  E2E 测试由 `web/scripts/run-e2e.mjs` 编排，默认使用 `E2E_VITE_PORT=15173` 独立端口，避免与开发者本机 5173 端口冲突；测试桩（`web/e2e/fixtures/bin/`）完整支持各后端的最新协议模式（如 Pi 的 `--mode rpc`）。
- **E2E 模型快照**：`web/e2e/global-setup.ts` 在测试开始前通过真实 API 执行一次平台 Agent 模型刷新，把模型快照写入临时数据库。产品默认只读数据库，因此这一步是测试对“运维员手动刷新”的模拟，不是产品自动刷新。
- **E2E 串行执行**：所有 spec 共享同一个后端、SQLite 数据库与机器级 opencode service，部分 spec 会停止/启动该 service，因此 `playwright.config.ts` 固定 `workers: 1`，避免跨文件竞态。

---

## 九、文档演进与维护约定（重要）

1. **同步更新约束**：
   后续当有新的功能需求、字段变动、安全策略或分层边界调整时，**必须在修改代码的同时同步更新维护本 `PROJECT.md` 文件**，严禁代码与架构文档脱节。
2. **参照源码分析同步**：
   若在开发过程中深入查阅了本机 `/data/projects/opencode`、`/data/projects/pi` 或 `/data/projects/claudian` 源码并获得了重要结论，需同步更新根目录下的 `opencode_source.md`、`pi_source.md` 或 `claudian_source.md`，保证团队沉淀长期有效。
