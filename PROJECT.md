# dev_platform 项目架构与技术规范

本文档是 `dev_platform` 项目的权威整体架构与实现设计描述。后续开发人员及 AI 助手在理解项目、检索功能代码、新增需求或进行架构重构时，**必须以本文档为准**。当需求发生变化、新增业务功能或发生架构调整时，**必须同步更新维护本文档**。

---

## 一、项目定位与核心能力

`dev_platform` 是一个面向团队和个人开发者的**多 Agent 协同 AI 研发工作台**。其核心目标在于解决不同终端 AI Coding Agent（如 OpenCode V2、Pi、DeepSeek CLI）协议各异、交互割裂、缺乏集中管控的问题，提供中性同构的平台层封装：

1. **多后端统一协同**：统一接入异构的 AI Coding Agent，支持开箱即用的多 Agent 实例隔离、生命周期管理与并发会话。
2. **实时交互工作台**：统一的 Web 端会话界面，支持类型化轮次追踪、流式响应（SSE）、代码 Diff 预览、富文本 Markdown 清洗与 PlantUML 架构图渲染；问答交互工具在主对话流直接呈现交互卡片，思考内容直接输出，后台工具输出后端摘要化极简流转。
3. **闭环人机确认**：提供权限确认（Permission）与结构化表单（Form）的人机交互流，杜绝客户端轮询，所有交互直接挂接在会话标准事件流中。
4. **全功能研发辅助**：集成基于 PTY/WebSocket 的跨平台在线终端、支持 Cron 表达式的定时任务调度、提示词（Prompts）模板管理，以及独立于 Agent 会话的通用大模型纯 Chat 对话。
5. **快捷助手**：可在平台设置中开启的横向方形半透明工具栏入口，悬浮完全显示，以 480×600 弹窗新建快捷对话，历史对话通过 Header 的最近对话下拉自行选择；工具栏与弹窗位置解绑且弹窗支持视口内自由拖拽；其会话统一沉淀在 Chat 的“快捷助手”固定分组，与主 Chat 页面共享模型配置但使用独立事件流。
6. **服务代理与 Demo 预览**：提供统一的动态 ContextPath 网关，支持将服务器上的静态文件目录（Directory Serving）或本地监听端口的 HTTP 服务（Port Reverse Proxy）通过平台端口以独立 ContextPath（`/proxy/:alias/`）直接暴露；免登录公开访问，支持外部快速分享与查看 demo，全协议透传并支持 WebSocket 升级（完美兼容 Vite/Webpack 等开发工具的 HMR 热更新）。
7. **桌面端双模协同**：提供原生桌面客户端外壳（Electron 架构），支持“本地单机自带服务（Sidecar 自动拉起与优雅停机）”与“连接远程云端/团队服务”双模开箱即用与自由切换；主进程内置高性能轻量本地透明反代（Desktop Local Proxy），无缝穿透跨域与 SameSite Cookie 限制，并动态对齐 Origin 保证终端 WebSocket CSWSH 安全校验 100% 契约一致，使前端业务代码与后端服务零侵入同构。

---

## 二、技术栈全景

| 层次 | 核心技术 / 依赖 | 架构职责与说明 |
|---|---|---|
| **后端语言** | Go 1.24.0 | 静态类型、跨平台编译（Linux / Windows / macOS） |
| **HTTP 与网关** | Gin v1.11.0 | RESTful 路由编排、SSE（Server-Sent Events）长连接多路广播 |
| **实时通信** | Gorilla WebSocket v1.5.3 | 终端 PTY 双向字符流转发（带严密 RFC 6455 同源 Origin 校验） |
| **数据持久化** | GORM v1.25.7 + SQLite (`glebarez/sqlite`) | 纯 Go 实现的嵌入式数据库（无需 CGO），自建迁移版本链（当前 v31，含 Agent 模型快照表与快捷助手固定分组及服务代理映射表 `service_proxies`） |
| **系统与进程** | `creack/pty`、`golang.org/x/sys`、`crypto` | 跨平台子进程生命周期管理、ConPTY/PTY 分配、安全密码哈希与 SSH 客户端 |
| **调度引擎** | `robfig/cron/v3` | 支持标准 5/6 段 Cron 表达式及单次定时调度 |
| **前端工程** | Vue 3.5 (SFC, `<script setup>`), Vite 8.3, TypeScript 7.0, Tailwind CSS 3.4, PostCSS 8.5 | 单页面应用（SPA），组合式 API，原子化样式与全局扁平微圆角规范 |
| **前端状态与路由** | Pinia 4.0, Vue Router 5.4 | 领域驱动的 Store 分层，支持草稿跨会话暂存 |
| **前端展现渲染** | xterm.js 5.3, Marked 18.1, DOMPurify 3.4, `@plantuml/core` | 终端仿真、安全 Markdown 流式渲染、PlantUML 在线绘图 |
| **桌面客户端外壳** | Electron v34, TypeScript, Node.js 原生 HTTP/WebSocket 透明代理, `electron-builder` | 原生跨平台桌面封装（Linux/Windows/macOS，禁用/移除默认原生菜单栏）、Sidecar 本地子进程纳管、双模网络同构代理与凭据加密安全存储 |
| **测试与质量门禁** | Go Testing + AST 静态扫描守卫; Vitest 5.0 (717 单测通过); Node.js 原生 Test Runner; Playwright E2E | 源码级架构分层守卫、CSRF 防绕过拦截矩阵、平台 UI 库组件单测、桌面代理与连接单测、全流程端到端自动化 |

---

## 三、分层架构体系（Claudian 五层同构规范）

项目严格对齐 Claudian 的五层架构体系，将“与具体 Agent 无关的通用同构层”和“必须因后端而异的异构层”彻底解耦：

```
① 组合根     cmd/platform/（Go 服务入口）+ desktop/src/main/（Electron 桌面主进程）+ web/src/{main.ts, App.vue, router.ts}（Vue 挂载）
④ 契约层     internal/core/agents（标准协议/事件/注册表）、internal/core/platform（OS 抽象）
③ 领域服务   internal/app/{sessions, instances, agentmodels, projects, chat, tasks, terminal, settings, auth, ...}
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
| **`sessions/`** | Agent 会话工作台核心领域。维护会话元数据索引、类型化轮次追踪、多路复用事件广播总线、人机交互决策仲裁、SSE 长连接响应、平台斜杠命令定义与调度（ListCommands/ExecuteCommand/CompactSession）。平台拥有命令统一定义权（分为功能类 action 与提示词注入类 prompt），当前实现功能类上下文压缩（compact），由各 Agent 适配层通过 SessionCompactor 落实。 | `service.go`（会话主逻辑）、`handlers.go`（HTTP/SSE 处理）、`event_bus.go`（事件总线）、`service_interactions.go`（审批管理） | 子包 `turns/` 专注轮次状态机；子包 `store/` 专注索引持久化；不关心底层进程如何生成，仅依赖 `internal/core/agents`。 |
| **`instances/`** | Agent 运行时实例生命周期管理。管理宿主子进程派生、退出监控、事件流单向上游订阅、环形重放缓冲（Ring Buffer）。定义领域级哨兵错误（`ErrDirectoryUnavailable`、`ErrProjectNotAuthorized`）隔离外部工作区异常。 | `service.go`（实例调度与停止）、`replay.go`（断线重放缓存）、`repository.go`（实例持久化与自愈） | 保证实例间严格逻辑隔离；不触碰会话层业务逻辑；停止实例走此路径。 |
| **`projects/`** | 项目工作区与文件生命周期。项目目录登记/注销/移动/重命名、安全路径校验（防路径穿越）、Git 状态与提交历史（`GitCommits` / `GitStatusWithRef`）、Git 工作区版本闭环管理（文件暂存 `StageFiles`、移出暂存 `UnstageFiles`、放弃工作区变更 `DiscardFileChanges`、提交变动 `CommitChanges` 及本地分支管理 `ListBranches` / `CheckoutBranch`）、文件统一 Diff 差异比对（`GitFileDiff`）、全量文件树遍历（去 `.gitignore` 与常规点文件过滤，严格隔离 `.git` 元数据目录）、工作区文件读取与短时只读预览 Ticket 鉴权（支持带 CSP Sandbox 的免 Cookie 通配符静态资源安全加载）。 | `service.go`（项目生命周期与预览 Ticket）、`files.go`（工作区文件树与内容读取）、`handlers.go`（项目与文件路由）、`git.go` / `git_status.go` / `git_ops.go` / `git_diff.go`（Git CLI 操作集成与差异比对） | 不知道 Agent 的存在；项目删除时协同通知 `instances` 销毁对应进程。 |
| **`chat/`** | 纯模型非 Agent 直接对话系统。支持助手（Assistant）多级对话管理：支持助手专属配置（默认模型、系统提示词隐式注入模型上下文、图标配置/首字符自适应、浮窗快捷入口挂载）；移除固定分组锁定限制，所有助手平级可管理；对话只能在助手上新建；多模态图片上传（支持 32MiB 大请求体）、SSE 流式补全；采用多路复用事件总线架构（ChatEventBus），多个对话或组件（主面板与快捷助手）共用单条长连接。 | `service.go`（助手与对话管理、上下文系统提示词组装）、`completer.go`（调用 OpenAI 兼容端点）、`stream.go`（SSE 事件帧切分）、`event_bus.go`（多路复用事件总线） | 严禁导入 `sessions`、`instances`、`projects` 或 Agent 契约，与 Agent 流程物理隔离；系统提示词只注入请求上下文，不在对话消息气泡中体现；事件总线与 Agent Session 总线平级独立。 |
| **`tasks/`** | 计划任务与自动化调度引擎。支持标准 Cron 表达式与单次定时、并发配额控制池（最多 5 并发）、多策略执行器、执行历史记录。 | `scheduler.go`（后台轮询调度器）、`executor.go`（任务派发执行）、`service.go`（任务 CRUD 与状态同步） | 任务触发时调用 `sessions.Service` 创建或复用特定 Agent 会话执行。 |
| **`terminal/`** | Web 在线交互式终端。本地 PTY 分配、远程 SSH 会话池、WebSocket 双向字符转发、终端窗口 Resize 协同、同源 Origin 防御。REST 路由错误响应全面对齐 `errorResponse` 与 `c.AbortWithStatusJSON` 统一错误枚举规范。 | `handlers.go`（WebSocket 升级与同源拦截、REST 路由）、`service.go`（会话调度）、`session.go`（LocalSession / SSHSession 封装） | 调用 `internal/core/platform` 分配底层 PTY；不依赖具体 Agent 业务。 |
| **`modelconfig/`** | 大语言模型服务商与模型元数据配置。管理 OpenAI 兼容服务地址、模型列表、API 凭据安全关联、端点动态解析。 | `service.go`（模型服务增删改查）、`repository.go`（配置存储） | 为 `chat` 服务及平台模型选择器提供基础模型字典。 |
| **`modelprobe/`** | 大模型连通性探针。提供模型服务地址可达性测试与模型可用性探针，向前端提供探测报告。 | `probe.go`（连通性测试与模型抓取） | 供设置中心与模型管理页调用。 |
| **`agentservices/`** | 机器级 Agent 后台守护服务管理。负责机器级第三方后台服务（如 OpenCode service）的健康探测、运维手动启停与启动门控。 | `service.go`（探测与启停协调）、`handlers.go`（运维管理 API） | 仅在后台服务支持的后端生效；不承担模型缓存失效职责。 |
| **`agentmodels/`** | 平台级 Agent 模型快照与治理。持久化每个后端最近一次刷新的模型清单，并支持平台级启用/停用及配置修改（显示名称、上下文上限、图片支持）；默认读取数据库，不调用 Agent；手动刷新时调用适配器并执行增量原子合并，保留用户自定义配置与启用状态。 | `service.go`（List/Refresh/UpdateModel）、`repository.go`（快照读写与增量合并）、`handlers.go`（模型 API） | 只依赖 `core/agents` 契约、注册表与数据库；不感知具体后端；不提供项目级清单。 |
| **`auth/`** | 用户认证鉴权与安全防御。用户密码哈希校验、Session 令牌生命周期、本机首次免命令行密码设置引导、全局 CSRF 双提交校验与自动轮转中间件。 | `middleware.go`（`RequireCSRF` / `RequireSession`）、`csrf.go`（令牌生成与双提交比对）、`handlers.go`（登录/登出/首次密码设置） | 全局路由拦截第一关。 |
| **`secrets/`** | 敏感凭据脱敏与安全存储。管理第三方 API Key 与 SSH 密码，提供流式/日志敏感数据实时脱敏过滤器。 | `store.go`（加密存储与读写）、`redact.go`（正则脱敏过滤） | 用于模型配置、终端 SSH 凭据以及防止日志泄露。 |
| **`settings/`** | 平台全局偏好设置。管理外观主题、快捷助手开关、Agent 后端启用开关、可执行文件路径与运行模式配置。 | `service.go`（系统偏好与快捷助手开关）、`agents.go`（各 Backend 的配置状态）、`handlers.go`（设置 API） | 仅消费契约的 `Catalog`，不直接感知后端实现细节；快捷助手开关独立于外观配置存储。 |
| **`proxy/`** | 动态 ContextPath 服务代理网关。将服务器静态文件目录（含目录树浏览与防穿越）或本地端口 HTTP 服务反向代理至平台单一端口；全协议透明转发、自动剥离 ContextPath 前缀、支持 WebSocket 协议升级（HMR）与端口连通性探测。 | `service.go`（CRUD、参数校验与端口探测）、`gateway.go`（动态反代与文件服务分发器）、`handlers.go`（管理 API） | 数据面（`/proxy/*`）完全免鉴权公开直通；控制面（`/api/proxies`）严格执行 Session 与 CSRF 认证；不感知具体 Agent 业务。 |
| **`database/`** | 平台数据持久化基础设施。GORM 数据库连接池初始化、版本化自动迁移（当前 v32 迁移链，含 `agent_models` / `agent_model_catalogs` 增加 `enabled` 字段、`chat_groups` 助手扩展字段、`chats.system_prompt` 及服务代理映射表 `service_proxies`）。设置纯 Go 嵌入式 SQLite 驱动安全连接池上限（MaxOpenConns=1），结合 WAL 模式与 5 秒 busy_timeout 消除并发写锁争用。 | `database.go`（SQLite 连接与连接池配置）、`migrations.go`（版本演化迁移逻辑） | 单一 SQLite 数据库；纯 Go 驱动无需系统 SQLite 动态库。 |
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
| **`opencode/`** | **CLI + RPC + 机器级 Service** | 封装 OpenCode V2 协议；管理 Sidecar 与本地服务探测；将原生 `session.text.delta`、`session.reasoning.*` 映射为标准事件流。 |
| **`pi/`** | **常驻子进程 + JSON-RPC** | 以 `pi --mode rpc` 运行；通过 stdin/stdout 全双工派发 `prompt`、`abort`、`set_model` 命令；解析 Edit Diff 增量；承接工具确认扩展。 |
| **`deepseek/`** | **共享 Host 进程 + WebSocket** | 启动并监控共享的 `dsh` 服务（懒启动、30s 启动超时、30min 空闲回收）；通过 WebSocket 客户端双向转发消息；管理子代理（Subagent）级联关系。 |

---

### 4. 前端应用架构（`web/src/`）

```
web/src/
  ├── api/             ── 网络传输层（client.ts 统管 CSRF，子文件映射后端 RESTful API）
  ├── stores/          ── 全局领域状态（agents, projects, settings, auth, models, chats, quickAssistant, drafts, viewport, ...）
  ├── shared/          ── Provider 中性通用控件与工具（Markdown、PlantUML、Color、Format）
  │     └── ui/        ── 平台标准 UI 组件库（纯中性业务解耦设计，基于 Tailwind CSS）：
  │           ├── atomic/    ── 原子组件（UiButton, UiInput, UiTextarea, UiSelect, UiDropdown, UiBadge, UiProgress, UiTooltip, UiTabs, UiAlert, UiEmpty, UiSpinner, UiDivider）
  │           ├── compound/  ── 复合容器组件（UiDialog, UiFormDialog, UiComposer, UiScrollFollow, UiBubble, UiTree, UiTreeNode）
  │           └── index.ts   ── 统一入口导出
  ├── features/        ── 业务领域垂直拆分目录：
  │     ├── chat/      ── Agent 工作台（核心）
  │     │     ├── composer/      ── 输入框组件、模型选择器、图片附件槽、斜杠命令调度器（slashActionDispatcher.ts）与多源补全
  │     │     ├── execution/     ── SessionEventStream.ts 事件流消费与状态维护
  │     │     ├── interactions/  ── 权限确认卡片、表单队列、审批事件派发
  │     │     ├── pane/          ── 会话右侧 34px 细导航（ChatSideRail，文件树/预览双入口）、右侧活动面板（思考/工具/文件预览）折叠与宽度拖拽、右侧文件树停靠/浮出、`useActivityPane` 视图状态
  │     │     ├── rendering/     ── MessageView.vue 消息/工具卡片渲染、SessionActivityLog.vue 右侧活动面板专属展示、logModel.ts 纯逻辑
  │     │     ├── state/         ── 会话日志推导状态模型（sessionLog.ts）
  │     │     ├── pure/          ── 独立纯 Chat 对话工作台（第二栏专属展示助手列表，第三栏无 Tab 左右双列：左侧主对话面板 + 右侧助手对话清单）
  │     │     └── quick/         ── 快捷助手横向方形工具栏、每次打开新建对话的 480×600 弹窗、自由拖拽定位与最近对话下拉
  │     ├── workspace/ ── 平台三栏主框架（AppShell.vue）、分类导航条（CategoryRail.vue，一级菜单顺序：项目 → 对话 → 任务 → 提示词 → 终端）、会话 Tab
  │     ├── projects/  ── 项目树（ProjectTree.vue）、会话卡片项（ProjectSessionItem.vue）、项目设置、打开/创建项目弹窗
  │     ├── proxies/   ── 服务代理管理组件（ProxiesView.vue）、创建/编辑表单弹窗（ProxyEditDialog.vue）、可视化目录选择器（ProxyDirectoryPicker.vue）
  │     ├── settings/  ── 外观设置、大模型配置、Agent 后端开关配置、服务代理设置（ProxySettings.vue）
  │     ├── terminal/  ── Web 终端工作区（TerminalWorkspace.vue）、xterm 画布（XtermView.vue）
  │     ├── tasks/     ── 定时任务管理、执行面板、任务专属对话弹窗（TasksView.vue）
  │     ├── prompts/   ── 提示词模板库管理（PromptsView.vue）
  │     └── files/     ── 文件查看器（FileViewer.vue，支持普通查看与 Diff 双模式）、变更树组件（ChangeTree.vue，树形层级展示变动文件，支持暂存/取消暂存/放弃修改悬浮操作）、本地分支选择器（BranchSelector.vue，支持分支切换与快速新建）、Git 提交操作栏（GitCommitBar.vue，支持 Ctrl+Enter 快捷提交与自动暂存闭环）、Diff 预览器（DiffViewer.vue，Unified Diff 高亮对比与双列行号）、Markdown/PlantUML/图片 Artifact 预览、HTML 源码高亮与“🌐 预览”独立弹窗沙箱预览（HtmlArtifactView.vue、HtmlPreviewModal.vue）
  └── router.ts        ── 路由映射与鉴权守卫（SPA 路由入口）
```

#### 平台标准 UI 组件库与设计系统规范（Data Terminal）
> 详细设计系统规范、色彩 Token、字符对齐律、状态交互法则（The Move）与图标规范详见：👉 [`docs/architecture/design-system.md`](./docs/architecture/design-system.md)

1. **纯中性设计与业务解耦**：
   - 提取的组件严格置于 `shared/ui/`，保持与具体业务领域（Chat、Tasks、Projects 等）彻底解耦。
   - 例如：`UiSelectMenu` 提炼为通用的现代下拉选择器（支持触发器展示主副文本、弹出浮层、搜索过滤、快捷清除与多选/单选无障碍支持，彻底替代各业务模块的裸露原生 `<select>`）；`UiProgress` 抽离为支持 Tooltip 提示的通用进度条组件；`UiComposer` 抽离为通用可拖拽高度的复合输入交互容器；`UiTree` 抽象通用的树形层级交互骨架。
2. **第二栏与侧边栏列表标准规范（基于 design-taste-frontend 深度重构）**：
   - 全平台第二栏（中间导航与对象索引栏：项目、对话、任务、提示词、终端）统一采用**三段式紧凑架构（Header / Search & Filter / Entity List）**：
     - **Zone A 紧凑 Header**：标题高度收敛（13px，`font-weight: 600`），右侧集成 22px 微型方圆角停靠与展开控制按钮（`var(--radius, 4px)`），消除冗余大块装饰。
     - **Zone B 紧凑检索与微过滤**：统一 26~28px 极简工具栏，内嵌 1.5px 极简放大镜矢量图标与微型选项胶囊，移除输入框聚焦外发光（glow / ring）与外层大按钮，大幅释放首屏纵向阅读高度。
     - **Zone C 紧凑列表与导航卡片（Dense Pitch）**：
       - **层级导航**：项目树 `ProjectTree` 采用两级树状折叠（1.5px 矢量旋转 SVG 折叠角、等宽计数徽标及悬浮快捷加号微按钮）；对话模块第二栏 `ChatSidebar` 则专门聚焦为**纯助手列表**（图标徽标 + 助手名称 + 会话计数 + 编辑/删除操作），下属对话剥离至第三栏右侧对话清单（`ChatHistoryPane`）呈现。
       - **实体列表项排版**（覆盖会话项、助手项、对话项、任务项、提示词项、服务器项）：全量采用标准紧凑卡片（徽标/状态/呼吸指示器 + 语义标题单行截断 + 悬停淡入的 18×18px 快捷操作工具条；属性元数据 + 右对齐 `tabular-nums` 等宽时间戳）。
       - **选中与激活法则（The Move）**：所有列表项激活态严格收敛为**左侧边缘 2px accent 科技蓝实线标尺（`border-l-2 border-l-[var(--color-accent,#4a8ad6)]`）** 与阶梯底色（`var(--surface-muted)`），全平台第二栏彻底清除静止投影（`box-shadow`）与毛玻璃模糊。
       - **色彩预算**：彩色严格控制在 3% 以内，任务与终端状态仅在运行/阻断错误时显色，已完成与常规状态保持中性阶梯底色。
   - **折叠/展开交互按钮**：全平台统一采用紧凑方圆角微按钮（`var(--radius, 4px)`），展开与收起按钮双向镜像对称，消除突兀的长胶囊贴边条与文字大块按钮。
   - **会话级右侧细导航栏**：项目会话对话区右侧设 34px 竖向细导航，集中承载「文件树」与「思考与工具 / 文件预览」两个入口；使用 26×26px 图标按钮、竖排微标签与 Tooltip，面板收起后入口常驻。展开状态以 active 样式与 `aria-pressed` 表达，按钮不得再放回会话标题行，预览入口不显示活动数量角标。组件位于 `features/chat/pane/ChatSideRail.vue`，状态仍由 `useActivityPane`（localStorage）管理；该组件带会话业务语义，禁止下沉到 `shared/ui/`。文件树与预览均停靠右侧；两者同时展开时，预览按会话区容器宽度自动转为覆盖消息列的覆盖层，文件树保持可见，避免把右轨挤出视口；消息列最小宽度 300px，文件树与预览可分别收缩至 210px / 300px。
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
| **`desktop/src/main/index.ts`** | Electron 主进程入口。负责单实例锁校验（防止重复运行）、主窗口几何尺寸管理、系统生命周期监控（`before-quit` 级联停机）、统一 IPC 通道分发。 |
| **`desktop/src/main/service-manager.ts`** | 本地 Go 服务端（Sidecar）生命周期调度器。在本地模式下派生 `bin/platform` 子进程，监听回环地址随机端口（`-listen 127.0.0.1:0`），自动从日志中探测绑定端口并做 `/api/health` 可达性校验；应用关闭时发送 SIGTERM 优雅退出，并在 5 秒超时后兜底回收，彻底消除孤儿进程。 |
| **`desktop/src/main/proxy.ts`** | 本地透明反向代理（Desktop Local Proxy）。主进程在 `127.0.0.1:DESKTOP_PORT` 启动轻量高性能代理，前端无论连接本地还是远程后端均访问该同源端口；代理负责 HTTP/SSE 请求透明转发、静态文件与 SPA 路由直接响应，并在 WebSocket 握手时自动重写 Host 与 Origin，保证终端 `checkSameOrigin` 与 CSRF 双提交校验 100% 同构兼容。 |
| **`desktop/src/main/connection-store.ts`** | 多环境连接配置与安全凭据管理。持久化存储“本地单机内置服务”及“远程服务器”连接列表，利用 Electron 原生 `safeStorage`（系统 Keychain / DPAPI）对敏感凭据进行操作系统级硬件加密存储。 |
| **`desktop/src/preload/index.ts`** | Preload 安全上下文隔离网桥。向渲染进程安全暴露只读中立契约 `window.desktopAPI`，包含获取连接、保存配置、切换连接、连通性探测及原生目录对话框。 |

**发布打包**：`./release.sh`（`make release`）一次产出两类产物并统一生成 `SHA256SUMS`：① Web 服务端包（Linux amd64/arm64、Windows amd64，内含二进制 + `web/dist` + 运维指南）；② 桌面安装包（`electron-builder`：Linux AppImage/tar.gz、Windows zip），包内以 extraResources 内嵌同架构 Go sidecar（`resources/bin/platform*`）与 `web/dist`，安装后即可本地自带后端运行。桌面应用版本默认为 `desktop/package.json` 的 `version`；发布标签为语义化版本（如 `./release.sh v0.2.0`）时自动对该版本。`web/dist` 只构建一次、两类产物共用；`--skip-desktop` / `--skip-server` 可只构建其中一类。

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
5. 多路复用事件总线 (internal/app/sessions/event_bus.go)
   - 将标准化事件广播至对应的 sessionID 订阅通道
           │
           ▼ (HTTP SSE 流持续推送，传输体积下降 90% 以上)
6. 前端执行引擎 (features/chat/execution/SessionEventStream.ts & rendering)
   - 消费 SSE 事件帧，更新 sessionLog 状态
   - 问答交互工具归入主对话流（MessageView display="messages"），作为交互卡片展示提问与选项；答案在所有题目输出完成后于底部统一展示，解析 JSON 答案并以 1 2 3 序号每行逐项对应
   - 主对话流动态运行状态指示（MessageView kind-running）：移除原有静态“正在运行...”，采用 deriveRunningAction 基于最新活动动态解析动作语义（如“正在思考...”、“执行 <cmd>”、“编辑 <path>”、“读取 <path>”、“搜索 <query>”等）并以微动效卡片实时呈现
   - 右侧活动面板（display="activity"，由右轨预览开关控制）排除问答工具，思考内容直接排版输出（无折叠壳），其他工具统一展示为紧凑单行摘要；同一右轨的文件树开关控制文件树停靠/浮出
   - 会话历史读取瘦身（GET /projects/{id}/sessions/{sessionId}/history）：会话领域服务统一对适配器历史事件进行按需瘦身过滤，仅保留主对话视图所需的消息（EventMessage、EventError）以及成对的问答交互工具（IsQuestionTool 识别之 EventToolCall 及配套 EventToolResult），海量思维链 EventThinking 与非问答类工具事件均不透传，进一步压降前后端交互体量
   - 会话视口状态记忆（Session & Chat Viewport State Restoration）：通过 useViewportStore 独立记录每个会话主消息列与右侧活动面板的滚动条位置（scrollTop）与粘底状态（pinned）。当用户切换菜单、路由或切换 Tab 时，离开前自动固化当前视口状态；再次切回该会话时：若此前处于非粘底状态（如翻看历史中间某段），无缝精确恢复到离开时的 scrollTop 与悬浮“跳至最新”状态，绝不无故强制重置滚到底部；若此前处于粘底状态或首次打开，则平滑跟随最新内容。
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
3. 多路复用事件推送 (internal/app/chat/event_bus.go)
   - stream.go 以 SSE 格式逐块切分并持久化 Assistant 消息
   - 经 event_bus.go 聚合至客户端级单一 SSE 管道 (/api/chat-events/:uuid)
   - 前端通过 ChatEventBusClient 维护单条长连接，主面板与快捷助手等多组件基于 chatId 动态订阅，消除每会话独立连接开销
   - 若对话无标题，触发 core/textutil/title.go 自动提炼标题并落库
```

---

### 5. 平台级 Agent 诊断与模型快照流（Diagnostics & Model Snapshot）

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

### 6. 快捷助手漂浮对话流（Quick Assistant Flow）

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

### 7. 服务代理与 ContextPath 动态分发流（Service Proxy & Gateway Flow）

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

### 8. 桌面应用双模连接与本地透明代理流（Desktop Dual-Mode & Proxy Flow）

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
