# dev_platform 项目架构与技术规范

本文档是 `dev_platform` 项目的权威整体架构与实现设计描述。后续开发人员及 AI 助手在理解项目、检索功能代码、新增需求或进行架构重构时，**必须以本文档为准**。当需求发生变化、新增业务功能或发生架构调整时，**必须同步更新维护本文档**。

---

## 一、项目定位与核心能力

`dev_platform` 是一个面向团队和个人开发者的**多 Agent 协同 AI 研发工作台**。其核心目标在于解决不同终端 AI Coding Agent（如 OpenCode V2、Pi、DeepSeek CLI）协议各异、交互割裂、缺乏集中管控的问题，提供中性同构的平台层封装：

1. **多后端统一协同**：统一接入异构的 AI Coding Agent，支持开箱即用的多 Agent 实例隔离、生命周期管理与并发会话。
2. **实时交互工作台**：统一的 Web 端会话界面，支持类型化轮次追踪、流式响应（SSE）、代码 Diff 预览、富文本 Markdown 清洗与 PlantUML 架构图渲染。
3. **闭环人机确认**：提供权限确认（Permission）与结构化表单（Form）的人机交互流，杜绝客户端轮询，所有交互直接挂接在会话标准事件流中。
4. **全功能研发辅助**：集成基于 PTY/WebSocket 的跨平台在线终端、支持 Cron 表达式的定时任务调度、提示词（Prompts）模板管理，以及独立于 Agent 会话的通用大模型纯 Chat 对话。
5. **快捷助手**：可在平台设置中开启的右上角可拖拽漂浮入口，以 480×600 弹窗续接最近快捷对话或新建对话；其会话统一沉淀在 Chat 的“快捷助手”固定分组，与主 Chat 页面共享模型配置但使用独立事件流。

---

## 二、技术栈全景

| 层次 | 核心技术 / 依赖 | 架构职责与说明 |
|---|---|---|
| **后端语言** | Go 1.24.0 | 静态类型、跨平台编译（Linux / Windows / macOS） |
| **HTTP 与网关** | Gin v1.11.0 | RESTful 路由编排、SSE（Server-Sent Events）长连接多路广播 |
| **实时通信** | Gorilla WebSocket v1.5.3 | 终端 PTY 双向字符流转发（带严密 RFC 6455 同源 Origin 校验） |
| **数据持久化** | GORM v1.25.7 + SQLite (`glebarez/sqlite`) | 纯 Go 实现的嵌入式数据库（无需 CGO），自建迁移版本链（当前 v29，含 Agent 模型快照表与快捷助手固定分组） |
| **系统与进程** | `creack/pty`、`golang.org/x/sys`、`crypto` | 跨平台子进程生命周期管理、ConPTY/PTY 分配、安全密码哈希与 SSH 客户端 |
| **调度引擎** | `robfig/cron/v3` | 支持标准 5/6 段 Cron 表达式及单次定时调度 |
| **前端工程** | Vue 3.5 (SFC, `<script setup>`), Vite 8.3, TypeScript 7.0, Tailwind CSS 3.4, PostCSS 8.5 | 单页面应用（SPA），组合式 API，原子化样式与全局扁平微圆角规范 |
| **前端状态与路由** | Pinia 4.0, Vue Router 5.4 | 领域驱动的 Store 分层，支持草稿跨会话暂存 |
| **前端展现渲染** | xterm.js 5.3, Marked 18.1, DOMPurify 3.4, `@plantuml/core` | 终端仿真、安全 Markdown 流式渲染、PlantUML 在线绘图 |
| **测试与质量门禁** | Go Testing + AST 静态扫描守卫; Vitest 5.0 (653 单测通过); Playwright E2E | 源码级架构分层守卫、CSRF 防绕过拦截矩阵、平台 UI 库组件单测、全流程端到端自动化 |

---

## 三、分层架构体系（Claudian 五层同构规范）

项目严格对齐 Claudian 的五层架构体系，将“与具体 Agent 无关的通用同构层”和“必须因后端而异的异构层”彻底解耦：

```
① 组合根     cmd/platform/（Go 程序入口）+ web/src/{main.ts, App.vue, router.ts}（Vue 挂载）
④ 契约层     internal/core/agents（标准协议/事件/注册表）、internal/core/platform（OS 抽象）
③ 领域服务   internal/app/{sessions, instances, agentmodels, projects, chat, tasks, terminal, settings, auth, ...}
⑤ 异构适配   internal/providers/{opencode, pi, deepseek}（实现 core 契约，向组合根导出 Descriptor）
传输编排     internal/transport/http（路由编排、AccessLog 中间件、静态资源 SPA 托管）
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
| **`sessions/`** | Agent 会话工作台核心领域。维护会话元数据索引、类型化轮次追踪、多路复用事件广播总线、人机交互决策仲裁、SSE 长连接响应。 | `service.go`（会话主逻辑）、`handlers.go`（HTTP/SSE 处理）、`event_bus.go`（事件总线）、`service_interactions.go`（审批管理） | 子包 `turns/` 专注轮次状态机；子包 `store/` 专注索引持久化；不关心底层进程如何生成，仅依赖 `internal/core/agents`。 |
| **`instances/`** | Agent 运行时实例生命周期管理。管理宿主子进程派生、退出监控、事件流单向上游订阅、环形重放缓冲（Ring Buffer）。 | `service.go`（实例调度与停止）、`replay.go`（断线重放缓存）、`repository.go`（实例持久化与自愈） | 保证实例间严格逻辑隔离；不触碰会话层业务逻辑；停止实例走此路径。 |
| **`projects/`** | 项目工作区与文件生命周期。项目目录登记/注销/移动/重命名、安全路径校验（防路径穿越）、Git 状态与提交历史、文件树遍历。 | `service.go`（项目生命周期）、`files.go`（工作区文件树与内容读取）、`git.go` / `git_status.go`（Git CLI 操作集成） | 不知道 Agent 的存在；项目删除时协同通知 `instances` 销毁对应进程。 |
| **`chat/`** | 纯模型非 Agent 直接对话系统。支持助手（Assistant）多级对话管理：支持助手专属配置（默认模型、系统提示词隐式注入模型上下文、图标配置/首字符自适应、浮窗快捷入口挂载）；移除固定分组锁定限制，所有助手平级可管理；对话只能在助手上新建；多模态图片上传（支持 32MiB 大请求体）、SSE 流式补全。 | `service.go`（助手与对话管理、上下文系统提示词组装）、`completer.go`（调用 OpenAI 兼容端点）、`stream.go`（SSE 事件帧切分） | 严禁导入 `sessions`、`instances`、`projects` 或 Agent 契约，与 Agent 流程物理隔离；系统提示词只注入请求上下文，不在对话消息气泡中体现。 |
| **`tasks/`** | 计划任务与自动化调度引擎。支持标准 Cron 表达式与单次定时、并发配额控制池（最多 5 并发）、多策略执行器、执行历史记录。 | `scheduler.go`（后台轮询调度器）、`executor.go`（任务派发执行）、`service.go`（任务 CRUD 与状态同步） | 任务触发时调用 `sessions.Service` 创建或复用特定 Agent 会话执行。 |
| **`terminal/`** | Web 在线交互式终端。本地 PTY 分配、远程 SSH 会话池、WebSocket 双向字符转发、终端窗口 Resize 协同、同源 Origin 防御。 | `handlers.go`（WebSocket 升级与同源拦截）、`service.go`（会话调度）、`session.go`（LocalSession / SSHSession 封装） | 调用 `internal/core/platform` 分配底层 PTY；不依赖具体 Agent 业务。 |
| **`modelconfig/`** | 大语言模型服务商与模型元数据配置。管理 OpenAI 兼容服务地址、模型列表、API 凭据安全关联、端点动态解析。 | `service.go`（模型服务增删改查）、`repository.go`（配置存储） | 为 `chat` 服务及平台模型选择器提供基础模型字典。 |
| **`modelprobe/`** | 大模型连通性探针。提供模型服务地址可达性测试与模型可用性探针，向前端提供探测报告。 | `probe.go`（连通性测试与模型抓取） | 供设置中心与模型管理页调用。 |
| **`agentservices/`** | 机器级 Agent 后台守护服务管理。负责机器级第三方后台服务（如 OpenCode service）的健康探测、运维手动启停与启动门控。 | `service.go`（探测与启停协调）、`handlers.go`（运维管理 API） | 仅在后台服务支持的后端生效；不承担模型缓存失效职责。 |
| **`agentmodels/`** | 平台级 Agent 模型快照。持久化每个后端最近一次手动刷新的模型清单；默认读取数据库，不调用 Agent；刷新时调用适配器并原子替换快照。 | `service.go`（List/Refresh）、`repository.go`（快照读写）、`handlers.go`（模型 API） | 只依赖 `core/agents` 契约、注册表与数据库；不感知具体后端；不提供项目级清单。 |
| **`auth/`** | 用户认证鉴权与安全防御。用户密码哈希校验、Session 令牌生命周期、全局 CSRF 双提交校验与自动轮转中间件。 | `middleware.go`（`RequireCSRF` / `RequireSession`）、`csrf.go`（令牌生成与双提交比对）、`handlers.go`（登录/登出） | 全局路由拦截第一关。 |
| **`secrets/`** | 敏感凭据脱敏与安全存储。管理第三方 API Key 与 SSH 密码，提供流式/日志敏感数据实时脱敏过滤器。 | `store.go`（加密存储与读写）、`redact.go`（正则脱敏过滤） | 用于模型配置、终端 SSH 凭据以及防止日志泄露。 |
| **`settings/`** | 平台全局偏好设置。管理外观主题、快捷助手开关、Agent 后端启用开关、可执行文件路径与运行模式配置。 | `service.go`（系统偏好与快捷助手开关）、`agents.go`（各 Backend 的配置状态）、`handlers.go`（设置 API） | 仅消费契约的 `Catalog`，不直接感知后端实现细节；快捷助手开关独立于外观配置存储。 |
| **`database/`** | 平台数据持久化基础设施。GORM 数据库连接池初始化、版本化自动迁移（当前 v30 迁移链，含 `agent_models` / `agent_model_catalogs` 与 `chat_groups` 助手扩展字段及 `chats.system_prompt`）。 | `database.go`（SQLite 连接）、`migrations.go`（版本演化迁移逻辑） | 单一 SQLite 数据库；纯 Go 驱动无需系统 SQLite 动态库。 |
| **`runtime/`** | 单实例互斥控制。通过工作目录文件锁（lockfile）防止多个服务端进程并发冲突打开同一个 SQLite 数据库。 | `single_instance.go`（跨平台文件排他锁实现） | 组合根启动时最先获取，退出时最后释放。 |
| **`paths/`** | 跨平台路径解析与安全性校验。规范化路径表示，防范 `../` 目录穿越攻击，检验项目工作区授权边界。 | `paths.go`（安全路径判定与绝对路径规范化） | 被 `projects`、`instances` 等模块作为安全基座调用。 |

---

### 2. 核心契约与抽象层（`internal/core/`）
纯定义与中立策略，**禁止依赖任何业务层或异构适配层**。

| 模块目录 | 核心职责与关键文件 |
|---|---|
| **`agents/`** | **Agent 统一核心契约**：<br>• `contracts.go`：`BackendID`、`AgentAdapter` 主接口、能力声明 `Capabilities`、标准消息/模型载荷。<br>• `events.go`：类型化轮次 `EventTurn`、标准交互事件 `EventInteraction`、流式增量事件。<br>• `extensions.go`：可选扩展接口（`PermissionReplier` 审批、`HistoryReader` 历史回放、`ServiceController` 机器服务、`RawEventOpener`）。<br>• `descriptor.go` & `registry.go`：后端元数据描述符、`Catalog` 目录树与全局适配器注册表。 |
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
  ├── stores/          ── 全局领域状态（agents, projects, settings, auth, models, chats, quickAssistant, drafts, ...）
  ├── shared/          ── Provider 中性通用控件与工具（Markdown、PlantUML、Color、Format）
  │     └── ui/        ── 平台标准 UI 组件库（纯中性业务解耦设计，基于 Tailwind CSS）：
  │           ├── atomic/    ── 原子组件（UiButton, UiInput, UiTextarea, UiSelect, UiDropdown, UiBadge, UiProgress, UiTooltip, UiTabs, UiAlert, UiEmpty, UiSpinner, UiDivider）
  │           ├── compound/  ── 复合容器组件（UiDialog, UiFormDialog, UiComposer, UiScrollFollow, UiBubble, UiTree, UiTreeNode）
  │           └── index.ts   ── 统一入口导出
  ├── features/        ── 业务领域垂直拆分目录：
  │     ├── chat/      ── Agent 工作台（核心）
  │     │     ├── composer/      ── 输入框组件、模型选择器、图片附件槽、斜杠命令
  │     │     ├── execution/     ── SessionEventStream.ts 事件流消费与状态维护
  │     │     ├── interactions/  ── 权限确认卡片、表单队列、审批事件派发
  │     │     ├── pane/          ── 右侧活动面板（思考/工具/文件树）折叠与宽度拖拽
  │     │     ├── rendering/     ── MessageView.vue 消息/工具卡片渲染、logModel.ts 纯逻辑
  │     │     ├── state/         ── 会话日志推导状态模型（sessionLog.ts）
  │     │     ├── pure/          ── 独立纯 Chat 对话界面与树形两级导航
  │     │     └── quick/         ── 快捷助手漂浮按钮、480×600 弹窗与拖拽定位
  │     ├── workspace/ ── 平台三栏主框架（AppShell.vue）、分类导航条（CategoryRail.vue）、会话 Tab
  │     ├── projects/  ── 项目树（ProjectTree.vue）、会话卡片项（ProjectSessionItem.vue）、项目设置、打开/创建项目弹窗
  │     ├── settings/  ── 外观设置、大模型配置、Agent 后端开关配置
  │     ├── terminal/  ── Web 终端工作区（TerminalWorkspace.vue）、xterm 画布（XtermView.vue）
  │     ├── tasks/     ── 定时任务管理、执行面板、任务专属对话弹窗（TasksView.vue）
  │     ├── prompts/   ── 提示词模板库管理（PromptsView.vue）
  │     └── files/     ── 文件查看器、Markdown/PlantUML/图片 Artifact 预览
  └── router.ts        ── 路由映射与鉴权守卫（SPA 路由入口）
```

#### 平台标准 UI 组件库与样式管理规范
1. **纯中性设计与业务解耦**：
   - 提取的组件严格置于 `shared/ui/`，保持与具体业务领域（Chat、Tasks、Projects 等）彻底解耦。
   - 例如：`UiProgress` 抽离为支持 Tooltip 提示的通用进度条组件，可供上下文用量、任务进度或磁盘限额等任何场景使用；`UiComposer` 抽离为通用可拖拽高度的复合输入交互容器；`UiTree` 抽象通用的树形层级交互骨架。
2. **样式管理与视觉风格**：
   - 全局引入 Tailwind CSS，预置 Slate/Zinc 为中性调色板，Blue 为 Primary 主色，Emerald/Amber/Red 为语义状态色。
   - 视觉风格采用现代扁平化设计（Flat Design），组件圆角统一设定为轻微小圆角（`rounded: 6px`），消除厚重投影。
   - 控件高度统一基准阶梯（标准控件高 34px / 小号 28px），操作焦点统一使用高亮光圈（Focus Ring）。
3. **操作体验规范统一**：
   - 弹窗操作顺序统一规范：取消按钮居左（Secondary 样式），确认/执行按钮居右（Primary 或 Danger 样式）；全量支持 Esc 关闭与 Tab 焦点陷阱。
   - 破坏性/危险操作一律采用 Danger 红色警示按钮。
   - 异常状态与重试引导统一使用 `UiAlert`。
```

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
       • EventToolCall / EventToolResult
       • EventTurn (ended, completed/failed/cancelled)
           │
           ▼
5. 多路复用事件总线 (internal/app/sessions/event_bus.go)
   - 将标准化事件广播至对应的 sessionID 订阅通道
           │
           ▼ (HTTP SSE 流持续推送)
6. 前端执行引擎 (features/chat/execution/SessionEventStream.ts)
   - 消费 SSE 事件帧，更新 sessionLog 状态
   - 驱动 MessageView.vue 与活动面板实时重绘
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
4. 双向全双工转发 Goroutines
   - Goroutine A：循环读取 PTY 字符流 ──► 写入 WebSocket ──► xterm.js 屏幕刷新
   - Goroutine B：循环读取 WebSocket 击键 ──► 写入 PTY 进程 stdin
   - 窗口伸缩：前端 ResizeObserver 捕获尺寸 ──► 发送二进制帧 ──► 调用 PTY.Resize()
```

---

### 4. 纯 Chat 通用大模型流式对话流（Pure Chat Flow）

```
[ 用户在“Chat”分类下新建或进入模型对话 ]
                    │
                    ▼
1. Web 前端 (features/chat/pure/ChatPanel.vue)
   - 选用 OpenAI 兼容的模型配置 (model_id)
   - 发送文本及多张图片 (最大支持 32MiB 请求体)
                    │
                    ▼
2. 后端纯 Chat 服务 (internal/app/chat/)
   - 校验会话归属，存储用户消息记录
   - 委托 modelconfig 解析模型服务地址与 API Key
   - 调用 completer.go 发起带 context 的流式 HTTP 调用
                    │
                    ▼ (SSE 数据切分)
3. 响应推送与标题自动推导
   - stream.go 以 SSE 格式逐块推给前端并持久化 Assistant 消息
   - 若对话无标题，触发 core/textutil/title.go 自动提炼标题并落库
```

---

### 5. 平台级 Agent 诊断与模型快照流（Diagnostics & Model Snapshot）

- **诊断归属**：`GET /api/agent-backends/diagnostics` 是平台级接口。前端只在「平台设置 → Agent」中展示：每张 Agent 卡片内提供可折叠诊断详情，`warning` / `error` 默认展开。项目设置不再读取诊断，也不再展示 machine service 提示。
- **模型持久化**：`internal/app/agentmodels` 管理平台级模型快照。`agent_models` 保存每个 backend 的模型行，`agent_model_catalogs` 保存最后同步时间；唯一索引为 `(backend, model_id)`。
- **读取**：`GET /api/agent-backends/{backend}/models` 只读数据库，不调用 Agent；Agent 离线时仍返回上次成功快照。从未刷新返回 `models: []` 与 `updatedAt: null`。
- **手动刷新**：`POST /api/agent-backends/{backend}/models/refresh` 才调用适配器 `ListModels(ctx, "")`，并在事务内原子替换快照。刷新失败保留旧快照；空结果清空模型但更新同步时间。
- **刷新失败可观测性**：HTTP 响应只给稳定错误码（`model_list_failed` / `service_not_running` / `unsupported_capability`），适配器返回的底层原因（例如 deepseek 共享宿主启动超时）在服务端日志以 `agent models: refresh <backend> failed: ...` 记录并做密钥脱敏；排障时先看这条日志，不要只看 access log 的 502。
- **deepseek 宿主启动预算**：deepseek 的刷新会懒启动共享 `dsh` 宿主，启动超时默认 30s（`dsh` 自身打印 launch URL 实测 4.9~5.3s，因此不可下调到秒级；可用 `DEEPSEEK_STARTUP_TIMEOUT` 覆盖）。参见 `docs/agent-compatibility.md` 的 deepseek transport 小节。
- **刷新策略**：无 TTL、无后台刷新、无轮询、无定时任务。只有运维员在 Agent 设置页点击“刷新”才会同步。空目录表示平台级/后端默认作用域，不考虑项目目录级模型配置。
- **边界**：Agent 模型是 Agent 的只读快照，不是平台模型配置；平台「模型」设置中的 OpenAI 兼容模型服务保持独立。

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
   - 按钮可拖拽至视口任意位置（保留 12px 防误触边距），位置写入 localStorage
                    │
                    ▼
3. 弹窗 (features/chat/quick/QuickAssistantPanel.vue)
   - 以 480×600 打开所选助手的对话弹窗，或直接开启新对话
   - 显示所选助手的名称、图标与专属模型，独立 quickAssistant store
                    │
                    ▼
4. 纯 Chat 领域 (internal/app/chat)
   - 助手具备默认模型、系统提示词、自定义图标和浮窗配置
   - 新建对话在指定助手下创建，系统提示词隐式注入模型上下文且不污染消息列表
   - 彻底解除历史固定分组锁定限制，所有助手平级可管理
   - 主 Chat 页面与弹窗各自维护 SSE 流，互不关闭
```

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
