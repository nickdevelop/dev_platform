# dev_platform 项目架构与技术规范

本文档是 `dev_platform` 项目的权威整体架构与实现设计描述。后续开发人员及 AI 助手在理解项目、检索功能代码、新增需求或进行架构重构时，**必须以本文档为准**。当需求发生变化、新增业务功能或发生架构调整时，**必须同步更新维护本文档**。

---

## 一、项目定位与核心能力

`dev_platform` 是一个面向团队和个人开发者的**多 Agent 协同 AI 研发工作台**。其核心目标在于解决不同终端 AI Coding Agent（如 OpenCode V2、Pi、DeepSeek CLI）协议各异、交互割裂、缺乏集中管控的问题，提供中性同构的平台层封装：

1. **工作空间与多后端统一协同**：以工作空间（Workspace）作为核心研发容器，统一接入异构的 AI Coding Agent，支持开箱即用的多 Agent 实例隔离、生命周期管理与并发会话。
2. **实时交互工作台**：统一的 Web 端会话界面，支持类型化轮次追踪、流式响应（SSE）、代码 Diff 预览、富文本 Markdown 清洗与 PlantUML / Mermaid 交互式图表渲染；主对话流纯净承载用户消息与答复、人机问答交互卡片（已将思考过程与工具调用过程移出会话对话流，保持高信噪比与高阅读舒适度）；Markdown 渲染安全放行 `file://` 等本地协议以支撑外部 Agent 输出的文件锚点跳转，流式渲染具备未闭合代码块自动补全防御机制杜绝排版跳动；会话与 Markdown 预览代码块全面接入轻量级语法高亮体系（按平台 light、warm、cool、dark 4 大主题自适应高对比度语法配色，代码块底色采用表面弱化主题色而非突兀黑底，右上角配备纯图标悬浮复制按钮不占用额外空行）；Mermaid 架构图与流程图渲染支持原生 SVG 文本与主题自适应；会话 Markdown 中的文件超链接（`file:///...` 或工作空间相对/绝对路径、系统临时目录 `/tmp` 与外部 Agent 运行时文件）点击时自动拦截并直接在右侧文件面板中打开，自动展开面板，全链路支持工作空间内绝对路径识别、URL 编码解码与 Hash 行号剥离，后端 `resolveProjectSafePath` 具备安全作用域内的绝对路径智能换算防御以及受信任外部安全目录（`/tmp`、`/var/tmp`、Agent 数据/缓存目录）白名单校验；右侧文件树懒加载具备规范的父子相对路径级联，确保多级子目录展开时完整保留父级层级前缀且支持无限层级平滑展开；PlantUML 与 Mermaid 架构图渲染支持标准标签及模型前缀变体（`plantuml`、`puml`、`uml`、`mermaid`、`mmd` 等），工作空间文件预览同样支持图表交互式渲染（Markdown 与 PlantUML 文件预览默认使用源码方式，可一键切换渲染预览与源码，支持共享放大查看器）；工作区文件预览支持直接编辑非二进制且未截断的文本文件（支持 `Ctrl+S` / `Cmd+S` 快捷保存、Tab 缩进、未保存状态指示与放弃修改二次确认），保存时按当前项目/工作树作用域安全写回磁盘；闭合围栏即时就绪绘制（无需死等整条长回复结束），Markdown 切片 `.markdown-seg` 具备独立 BFC 隔离（`display: flow-root`），且无论正常预览、流式等待或源码退化均具备完整的独立卡片容器与等宽隔离，杜绝图表与文本混排错乱；会话模型严格遵循持久化记忆优先级（用户显式选择 > 会话创建指定 > 会话持久化记录 `session.model` > 历史执行回溯 > 首次可用兜底），避免切换与刷新导致模型重置；右侧面板整合为统一的文件与预览面板（文件树直接作为常驻 Tab 在预览面板中展示，与打开的文件预览 Tab 平行并列切换，彻底消灭面板内左右分栏对代码阅读区的挤压；在文件树中点击文件新增/切换文件预览 Tab，关闭所有文件预览时自动切回文件树 Tab，在顶层 Tab 栏最右侧提供纯图标面板展开/收起开关；会话 History 与实时 SSE 传输实施高信噪比事件削减，仅传输并保留对话、交互问答与权限确认，彻底剥离冗余的思维链和常规工具调用，大幅减轻网络与渲染开销，保证高并发长会话下的极致性能与连接稳定性）；面板整体锁定外框总宽度，内部弹性伸缩（flex: 1 撑满），主对话区宽度恒定，彻底消灭对话内容跳动与晃动；动态运行指示器文案统一为「正在执行...」。新建会话表单将工作空间选择与 Agent 分段开关（Switch 形态）各自独立成行（Agent 行紧随工作空间行），Git 工作空间在工作空间行后增加“工作树”选择（主工作区或已创建的工作树，草稿按工作空间保存 worktreeId）；会话右侧文件树、文件预览与 Git 面板按该会话的 worktreeId 作用域读取，与 Agent 实际工作目录一致；输入框工具栏整合模型选择、权限下拉模式与统一发送按钮；提示词下拉同时提供角色类（role）与流程类（flow）提示词，角色类以 `<Role name="名称">` 标签、流程类以 `<Flow name="名称">` 标签作为首条用户提示词发送，用户输入随后追加，会话标题只取用户输入；会话气泡将提示词标签渲染为“启用角色/流程：名称”与可折叠的提示词正文；发送快捷键提示内嵌于输入框占位文本，不在输入组件下方额外占行；顶层视图引入 KeepAlive 缓存已打开标签页，配合 Markdown LRU 记忆化解析与 Git 状态跨 Tab 复用，保证 Tab 切换毫秒级秒切且杜绝长会话重复重绘，并在关闭 Tab 时自动回收非活跃消息内存；Agent 身份以官方图标标识：会话列表与会话 Tab 显示目录提供的 SVG（`iconUrl`），内置 Agent 为平台自绘图标，缺图标时回退 `shortName` 文字芯片；右侧文件树的展开状态、搜索、变更/所有文件 Tab 与提交选择按会话保存在 `stores/fileTreeState.ts` 内存中，收起文件树或右侧面板后再打开保持不变，关闭该会话 Tab 即释放；会话面板实现完整的 onActivated/onDeactivated 生命周期，当已关闭或数据被释放的 Tab 再次打开时自动检测并恢复会话历史与事件流，视口滚动记忆具备组件 detach 虚假滚动过滤与 double-rAF 尺寸就绪保证，杜绝复用休眠实例导致的界面空白与滚动重置到头部问题；支持全局快捷键 `Alt+1` 快速收折/展开左侧侧边栏、`Alt+2` 唯一收折/展开右侧工作区面板、`Alt+\`` 快速呼出/最小化终端浮窗（在 AppShell 集中管理防重入并在事件捕获阶段优先拦截，配合 Xterm 自定义按键处理器放行，确保终端聚焦时仍可平滑切换最小化）；会话主界面支持快捷键 `Ctrl+P`（Mac 为 `Cmd+P`）快速呼出搜索弹层（Quick Open），内置搜索类型下拉选择（文件 / 内容，默认文件，可在输入框内直接按 `Tab` 键秒级循环切换类型），文件搜索在 Git 项目下严格遵循 `.gitignore` 过滤无关文件与目录、非 Git 项目全量扫描不过滤，内容搜索极速返回匹配文件、行号及代码摘要，回车确认即可在右侧预览区打开并自动滚动高亮到对应行号；文件查看器支持 `Ctrl+F` 行内查找高亮，并类似 VSCode 支持 `Ctrl+G`（Mac 为 `Cmd+G`）快速跳转到指定行号；代码与文本预览（CodeView）初始化默认开启自动换行（word wrap），并在右上角配备快速切换换行按钮；文件树切换为“变更”时，点击变更文件通过参数透传直接以 Diff 模式在预览区打开 Git 差异对比视图并标有 Diff 徽标（在“所有文件”下点击则保持全量内容浏览）；会话对话界面顶部轻量化精简（彻底移除标题下方的物理路径、Git 分支及运行中等元信息，仅保留 Agent 后端标识、会话标题与删除按钮），将项目真实物理路径与当前 Git 分支统一归位至右侧“工作区文件”Tab 顶部栏并在“所有文件”选项右侧直观呈现。
3. **闭环人机确认**：提供权限确认（Permission）与结构化表单（Form）的人机交互流，杜绝客户端轮询，所有交互直接挂接在会话标准事件流中。
4. **组织中心（AI 资产与对象治理）**：一级「组织」中心纯粹负责 AI 资产对象的定义、配置与编排，下辖 **AI 助手**（单模型人设与平台工具配置）、**AI 员工**（具备指定引擎与模型的专职数字员工）、**AI 团队**（由中控调度大脑及编制内 AI 员工构成的作战单元），组织内部不承载事务性执行，专注对象生命周期管理。界面设计遵循统一标准：新增与编辑采用 **`el-dialog` 弹窗 + `el-form` 行内校验**（表单字段错误走行内提示，保存结果用 `ElMessage`），**破坏性操作统一用 `ElMessageBox.confirm` 确认**；AI 助手、AI 员工与 AI 团队三者保持严格一致的弹窗表单结构与字段分区，并保留**路由 Query 双向同步**（`?action=new` / `?edit=id` 可直接链接到具体对象）；列表统一由 `el-table` 承载，多选编制采用 `el-checkbox-group` 卡片网格勾选后回显。详见 `docs/architecture/frontend-ui-system.md`。
5. **快捷助手与即时交互**：全站移除独立的一级对话菜单，日常即时对话与平台轻量工具交互统一收敛至右侧或悬浮工具栏中的「快捷助手」（Quick Assistant Launcher），通过 480×600 可拖拽弹窗随时呼出，支持历史会话切换、多模态图片与流式响应。
6. **服务代理与 Demo 预览**：提供统一的动态 ContextPath 网关，支持将服务器上的静态文件目录（Directory Serving）或本地监听端口的 HTTP 服务（Port Reverse Proxy）通过平台端口以独立 ContextPath（`/proxy/:alias/`）直接暴露；免登录公开访问，支持外部快速分享与查看 demo，全协议透传并支持 WebSocket 升级（完美兼容 Vite/Webpack 等开发工具的 HMR 热更新）。
7. **桌面端双模协同**：提供原生桌面客户端外壳（Electron 架构），支持“本地单机自带服务（Sidecar 自动拉起与优雅停机）”与“连接远程云端/团队服务”双模开箱即用与自由切换；主进程内置高性能轻量本地透明反代（Desktop Local Proxy），无缝穿透跨域与 SameSite Cookie 限制，并动态对齐 Origin 保证终端 WebSocket CSWSH 安全校验 100% 契约一致，使前端业务代码与后端服务零侵入同构。

---

## 二、技术栈全景

| 层次 | 核心技术 / 依赖 | 架构职责与说明 |
|---|---|---|
| **后端语言** | Go 1.24.0 | 静态类型、跨平台编译（Linux / Windows / macOS） |
| **HTTP 与网关** | Gin v1.11.0 | RESTful 路由编排、全局统一 SSE（Server-Sent Events）事件网关（`/api/events/bus/:uuid`），配合前端 SharedWorker 共享工作线程实现全浏览器跨标签页单长连接多路复用，根除浏览器并发长连接耗尽问题 |
| **实时通信** | Gorilla WebSocket v1.5.3 | 终端 PTY 双向字符流转发（带严密 RFC 6455 同源 Origin 校验） |
| **数据持久化** | GORM v1.25.7 + SQLite (`glebarez/sqlite`) | 纯 Go 实现的嵌入式数据库（无需 CGO），自建迁移版本链（当前 v40，含 Agent 模型快照表、快捷助手固定分组、服务代理映射表 `service_proxies`、助手平台管理与最大工具步数配置、多 Agent 协同流程流转表、内置 Agent 会话转写表 `agent_builtin_sessions` / `agent_builtin_messages`、`prompts` 增加 `type` 字段区分角色类与流程类、全局专家角色管理表 `agent_roles`、`chat_groups` / `chats` 增加 `prompt_id` 实现对话与角色对提示词管理的统一引用机制、`agent_models` 增加 `name_customized` 标记、v38 Git 工作树体系 `project_worktrees`、v39 笔记持久化体系 `notes` 表，以及 **v40 团队中心与多 Agent 协同体系**：新增 `groups`、`group_members`、`group_tasks` 表，并在 `flow_runs` 增加 `group_id` 与 `worktree_id` 支撑工作树隔离的多 Agent 团队协同会话） |
| **系统与进程** | `creack/pty`、`golang.org/x/sys`、`crypto` | 跨平台子进程生命周期管理、Linux/Unix POSIX PTY 与 Windows 原生 ConPTY（Pseudo Console API）分配、安全密码哈希与 SSH 客户端 |
| **调度引擎** | `robfig/cron/v3` | 支持标准 5/6/7 段 Cron 表达式（秒、分、时、日、月、周、年）及单次定时调度 |
| **前端工程** | Vue 3.5 (SFC, `<script setup>`), Vite 8.3, TypeScript 7.0, Tailwind CSS 3.4, PostCSS 8.5, **Element Plus 2.14 + @element-plus/icons-vue + unplugin-vue-components** | 单页面应用（SPA），组合式 API，原子化样式与全局扁平微圆角规范。**UI 基座已迁至 Element Plus**：模板里的 `<el-*>` 由解析器按需引入 JS，样式采**单文件全量** `element-plus/dist/index.css`（函数式 API 不在模板解析器射程内），层叠顺序固定 `element-plus → style.css → theme/element-tokens.css`；全部库级 token 映射与密度桥接集中在 `web/src/theme/element-tokens.css`；新增/编辑表单统一走共享外壳 `web/src/shared/components/EntityFormDialog.vue`（分区骨架类在 `web/src/shared/styles/entity-editor.css`），列表页主体为三段式（工具条固定顶 / `el-table` 铺满中间 / `el-pagination` 固定底），工具条统一走 `web/src/shared/components/ListToolbar.vue`（左全局动作按钮 / 右搜索表单 + 搜索 + 重置，表单内控件间距 4px、枚举筛选框 120px / 关键词框 150px、控件统一 default 28px；枚举分类值走 `entity-chip` + 与皮肤无关的 `--cat-hue-1…6` 分类色轴，状态值才用语义 `type`），列表空行统一走 `web/src/shared/components/ListEmptyState.vue`（loading 行形骨架 / filtered / empty 三态），列表页页头与节奏类在 `web/src/shared/styles/list-page.css`，弹窗由 `App.vue` 的 `el-config-provider` 全局垂直居中；规范见 `docs/architecture/frontend-ui-system.md`，`web/src/tests/ui-element-boundary.spec.ts` 负责把库内部选择器挡在业务组件之外并锁定 `Ui*` 退役棘轮 |
| **前端状态与路由** | Pinia 4.0, Vue Router 5.4 | 领域驱动的 Store 分层，支持草稿跨会话暂存 |
| **前端展现渲染** | xterm.js 5.3, Marked 18.1, DOMPurify 3.4, `@plantuml/core` | 终端仿真、安全 Markdown 流式渲染、PlantUML 在线绘图 |
| **桌面客户端外壳** | Electron v34, TypeScript, Node.js 原生 HTTP/WebSocket 透明代理, `electron-builder` | 原生跨平台桌面封装（Linux/Windows/macOS，禁用/移除默认原生菜单栏）、Sidecar 本地子进程纳管、双模网络同构代理与凭据加密安全存储 |
| **测试与质量门禁** | Go Testing + AST 静态扫描守卫; Vitest 5.0 (1000+ 单测全量通过); Node.js 原生 Test Runner; Playwright E2E | 源码级架构分层守卫、CSRF 防绕过拦截矩阵、平台 UI 库组件单测、**Element Plus 边界守卫（`ui-element-boundary.spec.ts`：库内部 `--el-*` / `.el-*` 不得泄漏到业务组件，退役中的 `Ui*` 原语引用数只降不升）**、协同流程群聊单测、桌面代理与连接单测、全流程端到端自动化 |

---

## 三、分层架构体系（Claudian 五层同构规范）

项目严格对齐 Claudian 的五层架构体系，将“与具体 Agent 无关的通用同构层”和“必须因后端而异的异构层”彻底解耦：

```
① 组合根     cmd/platform/（Go 服务入口）+ desktop/src/main/（Electron 桌面主进程）+ web/src/{main.ts, App.vue, router.ts}（Vue 挂载）
④ 契约层     internal/core/agents（标准协议/事件/注册表）、internal/core/platform（OS 抽象）
③ 领域服务   internal/app/{groups, sessions, instances, agentmodels, agentusage, projects, chat, platformtools, tasks, terminal, settings, auth, ...}
⑤ 异构适配   internal/providers/{opencode, pi, deepseek, builtin, agy}（实现 core 契约，向组合根导出 Descriptor）
传输编排     internal/transport/http（路由编排、AccessLog 与 gzip 压缩中间件、静态资源 SPA 托管）+ desktop/src/main/proxy.ts（桌面透明反代）
② 前端展示   web/src/features/{organization, workspace, projects, settings, terminal, tasks, notes, engines}、web/src/shared/
```

### 依赖方向与防腐规则（守卫强制执行）：
- **依赖方向**：① 组合根 → 任意层；③ 领域服务 → ④ 契约层；⑤ 异构适配 → ④ 契约层；② 前端 features → 状态/API → core 契约。
- **严厉禁止**：
  - 禁止 `core → providers`、`app → providers`、`transport → providers`。
  - 禁止在通用同构层（`internal/app` 及 `internal/transport`）中出现任何特定后端 ID 字面量（如 `BackendOpenCodeV2`、`"opencode-v2"`、`"pi"` 等）或直接导入适配器包。由 `dependency_guard_test.go` 递归遍历全目录进行 AST 源码级拦截。
  - 前端通用组件禁止针对后端 ID 写 `if-else` 分支，所有功能差异必须基于契约能力集（`Capabilities`）或中性元数据（`Descriptor`）进行门控驱动。
  - **前端 UI 规范（强制，唯一权威）**：[`docs/architecture/frontend-ui-system.md`](./docs/architecture/frontend-ui-system.md)（v2 · Element Plus 基座，含引入契约、token 覆盖契约、密度契约、组件选型对照表与可执行质量门禁）；其视觉暂承自 [`docs/architecture/design-system.md`](./docs/architecture/design-system.md)（v1 · Data Terminal，现为视觉附录）。以下为摘录：
    0. **组件库优先与单一覆盖点**：控件一律用 Element Plus（模板不手写 import），对库样式的任何干预只能写在 `web/src/theme/element-tokens.css`；皮肤只来源平台 token，组件内不得出现 hex 色值与像素字号。
    1. **静谧中性镀层（Quiet Chrome）**：外壳、边框、侧栏底纹使用低对比度中性语义 token（`bg-surface-muted`、`border-border`、`text-platform-text-muted`），禁止随意引入高饱和彩虹色相（如多色彩点、硬编码 `blue-*` / `purple-*` / 霓虹发光底纹）。全站强调色统一收敛至语义 Accent（`text-accent` / `bg-accent` / `border-accent`）。
    2. **标准字阶阶梯（Typographic Scale）**：Page Title 20px（`--text-size-20`，不写 Tailwind `text-lg`）、Card Header 14px (`text-sm`)、Body 与表格数据单元格 12px（继承 `--el-font-size-base`，**不写 `text-2xs`**——数据区一行里出现两档字号就是排版坏了）、表头 / 行内动作 / 表格内 chip 11px（chip 高 18px，不得大于数据字号）。`text-2xs`（10px）不再进表格；严禁 `text-3xs` 与非标准十进制 rem。全站字体族固定 "Google Sans Code" + "Noto Sans SC"，由 `tailwind.config.js` 的 `fontFamily` 接 `--font-family` / `--font-mono`，组件内禁止硬编码字体栈。
    3. **严格 4px 栅格与真实度**：所有间距、高宽均需对齐 4px 倍数栅格（如 h-8=32px、p-1.5=6px、gap-2=8px），杜绝非标准小数类名；去除无意义的 `animate-ping` 呼吸抖动动效，保持界面专业、稳定与静谧。控件密度固定三档 small 24 / default 28（`--control-height`）/ large 32：**列表页工具条与弹窗表单用 default 28（不写 `size`）**，只有表格行内操作用 `size="small"`，同一行不得出现两种高度；分页 `size="small"` + `background`（页码方块被 token 层钉在 24px，摘掉 `size` 会让条数下拉回 28px）。弹窗底栏固定「取消 / 确认」两个词、不带图标、间距交回库的 12px。

---

## 四、模块职责划分与功能代码索引地图（核心导航罗盘）

本节是开发人员与 AI 快速定位功能代码、添加新功能的检索指南。修改代码时必须确保改动严格落在对应职责包内。

### 1. 后端同构领域服务层（`internal/app/`）
通用业务逻辑，**严禁硬编码任何特定后端 ID 或导入 `internal/providers/*`**。

| 模块目录 | 核心职责与功能边界 | 关键入口与核心文件 | 协作与边界约束 |
|---|---|---|---|
| **`events/`** | **统一多路复用事件网关**。平台级统一事件中枢与 SSE 网关服务（`UnifiedEventBusManager`）。下行事件多路复用统一收敛至单连接 `/api/events/bus/:uuid`，解耦具体业务领域（支持 `sessions`、`chat`、`tasks`、`system` 频道），通过动态主题订阅接口（`POST/DELETE .../subscriptions`）实现各业务页面即插即拔；统一心跳保活（15s keep-alive）与客户端断开内存垃圾回收（GC TTL 60s）；**单消费独占与连接置换保证（Connection Displacement）**：同一 ClientBus UUID 接入新连接（重连、刷新或竞态请求）时，分配单调递增代次 `connID`，立即主动终止并置换（displace）旧连接的 `StreamBus` 循环，并将残留未读事件 FIFO 顺延平滑搬移至新 channel，彻底根除同一 bus 多连接并发轮询 channel 导致的事件分流与序号跳号问题；旧连接退出时经 `connID` 守卫验证，绝不误将新连接置为 inactive 或触发 GC 销毁，退出 defer 仅在无新连接接替且失活时才排空未投递消息；具备关键帧入队保护（对 `turn_ended`、`interaction_requested` 等核心状态机事件与 `message/thinking/delta/chunk` 流内容帧在缓冲满时提供 100ms 等待入队保底）、慢客户端丢帧警告日志与原子丢帧计数器（`DroppedCount`）；**缓冲溢出可靠性闭环**：关键帧丢弃、流内容帧每第 8 次丢弃或累计第 64 次丢弃时主动踢断该 bus 的 SSE 连接（kick），迫使客户端带序号重订并经 `instances.ReplaySince` 环形重放或 resync 全量重建补齐缺口——把"在线静默丢帧"转化为可恢复转移；每帧写入前施加 45s 递进写超时（write deadline = 3×心跳），防止 TCP 停滞的客户端令 StreamBus 循环永久阻塞而缓冲无限溢出；提供 `GET /api/events/diagnostics` 诊断端点（各 bus 的 active/订阅计数/积压帧数/丢帧计数）；彻底取代原先各业务烟囱式独立建长连接的历史架构，与前端 SharedWorker 协同彻底消灭浏览器同域名 6 连接耗尽假死问题。 | `service.go`（服务封装与依赖编排）、`bus.go`（多路复用总线与心跳调度、连接置换与独占消费、溢出 kick、诊断快照）、`handlers.go`（统一 SSE 与订阅 REST API、diagnostics 路由）、`models.go`（通用信封协议 `EventEnvelope` 与 `ChannelHandler` 契约） | 纯通用同构层，定义 `ChannelHandler` 接口供各业务领域实现快照与生命周期钩子，不直接耦合底层 Agent 或特定业务逻辑。 |
| **`sessions/`** | Agent 会话工作台核心领域。维护会话元数据索引、类型化轮次追踪、人机交互决策仲裁、平台斜杠命令定义与技能调用调度（ListCommands/ListProjectCommands/ExecuteCommand/CompactSession/SendMessage/Create）。会话索引记录所属项目的 `worktree_id`：0 表示主工作区，非 0 表示平台登记的 Git 工作树；创建时经 `CreateInWorktree` 在该作用域复用/启动实例，续接、历史读取与重绑均回到原工作树。平台拥有命令与技能统一定义权（分为功能类 action 与提示词/技能注入类 prompt）：`ListCommands` 动态聚合系统动作（如 `compact`）与当前工作区/全局的三层 Agent Skills，并提供 `GET /api/projects/:id/commands` 供新建会话面板在无会话上下文时补全项目与全局技能；在 `SendMessage` 与会话新建初始消息中自动拦截并解析 `/<skill-name> [参数]`，从对应技能文件抽取详细指令并组装为 `<invoked_skill>` 标准化复合 Prompt 投递底层 Agent 适配器，实现**全 Agent 后端（builtin、opencode-v2、pi、deepseek、agy）统一透明支持 `/` 命令调用 Skill**；功能类上下文压缩（compact）由各 Agent 适配层通过 SessionCompactor 落实。Agent 图标经 `GET /api/agent-backends/:backend/icon` 返回 Descriptor 内嵌的 SVG（目录项 `iconUrl` 指向该路径，响应带沙箱 CSP 与 nosniff，无图标返回 404）；会话索引列表（全量 List 与分页 ListPaged）统一遵循 `updated_at DESC, created_at DESC, id DESC` 降序排序；`updated_at` 是**最后交互时间**，只有真实交互（发送消息、实例流上的实时事件、轮次边界与轮次结束回调）才抬升它，实例重绑（rebind）、续接（continue）、模型更新与**历史回放读取**等查看类操作一律走列级更新保护，不触碰该列（`HistoryPage` 不再依据回放事件时间戳回写 `updated_at`，前端 `loadHistory` 同样不覆盖会话 `updatedAt`），杜绝「点开历史会话即被顶到列表最前」的排序漂移；下行状态事件与交互快照统一接入全局事件网关（`events.Manager`，channel: `sessions`），彻底退役废弃的 `GET /projects/:id/sessions/:sessionId/events` 端点及 `StreamSession`；新增面向任务编排与内部订阅的强类型通道订阅接口 `SubscribeSession`（直接返回 AgentEvent 信封结构体 channel），杜绝字符串管道拼接与反序列化开销。 | `service.go`（会话主逻辑、工作树作用域、SubscribeSession 通道与 ChannelHandler 实现、技能调用 Prompt 组装与 ListCommands 聚合）、`handlers.go`（HTTP 路由处理与 `worktreeId`）、`event_bus.go`（实例流监听与事件转发）、`service_interactions.go`（审批管理） | 子包 `turns/` 专注轮次状态机；子包 `store/` 专注索引持久化；不关心底层进程如何生成，仅依赖 `internal/core/agents` 与 `internal/core/skills`。 |
| **`instances/`** | Agent 运行时实例生命周期管理。管理宿主子进程派生、退出监控、事件流单向上游订阅、环形重放缓冲（按 16 MiB / 2048 条双上限淘汰，保留连续事件后缀）。实例按“项目 + 后端 + `worktree_id`”隔离：主工作区为 0，非 0 时经 `ProjectDirectoryResolver.ResolveWorktreeDirectory` 取得工作树目录；支持 `StartInWorktree` 与 `StopProjectWorktreeInstances`。定义领域级哨兵错误（`ErrDirectoryUnavailable`、`ErrWorktreeUnavailable`、`ErrProjectNotAuthorized`）隔离外部工作区异常。 | `service.go`（实例调度、工作树启动/停止与停止）、`replay.go`（断线重放缓存）、`repository.go`（实例持久化与自愈，含 `worktree_id`） | 保证实例间严格逻辑隔离；不触碰会话层业务逻辑；停止实例走此路径。 |
| **`projects/`** | 项目工作区与文件生命周期。项目目录登记/注销/移动/重命名、安全路径校验（防路径穿越）、工作区文件安全上传（`UploadFile`）、安全删除（`DeletePath`，严禁删除项目根目录）、附件流式下载（`handleFileDownload`，注入 `Content-Disposition` 附件头）、项目 Git 状态（`IsGit` 标记与 `GitInfo`）、Git 提交历史（`GitCommits` / `GitStatusWithRef`）、Git 工作区版本闭环管理（文件暂存 `StageFiles`、移出暂存 `UnstageFiles`、放弃工作区变更 `DiscardFileChanges`、提交变动 `CommitChanges` 及本地分支管理 `ListBranches` / `CheckoutBranch`）、文件统一 Diff 差异比对（`GitFileDiff`）、全量文件树遍历（去 `.gitignore` 与常规点文件过滤，严格隔离 `.git` 元数据目录）、工作区文件读取与短时只读预览 Ticket 鉴权（支持带 CSP Sandbox 的免 Cookie 通配符静态资源安全加载）。**Git Worktree 生命周期**：`ListWorktrees` / `CreateWorktree` / `RemoveWorktree`，默认创建到仓库同级 `<repo>-worktrees/<branch-slug>`，分支名严格校验，移除前停止该工作树全部实例并拒绝脏工作区，分支保留；所有文件/Git/预览接口支持可选 `worktreeId` 作用域（0 为主工作区）。项目侧边栏设置直接与 Tab 栏和 `projects-management` 路由联动，项目排序与显隐状态通过后端数据库持久化存储（`GET/PUT /api/projects/display-config`），杜绝关闭平台后重置。 | `service.go`（项目生命周期、预览 Ticket、工作树服务与显示配置）、`worktree.go`（工作树创建/移除/目录解析）、`files.go`（工作区文件树、上传/删除/下载与内容读取，支持工作树作用域）、`handlers.go`（项目、工作树与文件路由）、`git.go` / `git_status.go` / `git_ops.go` / `git_diff.go`（Git CLI 操作集成、差异比对与工作树作用域） | 不知道 Agent 的存在；项目删除时协同通知 `instances` 销毁对应进程；移除工作树通过 `InstanceStopper` 停止该工作树实例。 |
| **`chat/`** | 纯模型非 Agent 直接对话系统。支持助手（Assistant）多级对话管理：支持助手专属配置（默认模型、系统提示词隐式注入模型上下文、图标配置/首字符自适应、浮窗快捷入口挂载、平台管理权限开关 `EnablePlatformTools` 与单轮最大工具步数 `MaxToolSteps` 控制）；移除固定分组锁定限制，所有助手平级可管理；对话只能在助手上新建；多模态图片上传（支持 32MiB 大请求体）、SSE 流式补全；支持流式 Function Calling / Tool Loop 调度（通过依赖反转注入中立的 `ToolExecutor` 接口，在助手开启平台管理权限时由大模型自主调用平台工具）；下行生成增量与消息事件统一接入全局事件网关（`events.Manager`，channel: `chat`），彻底退役 `GET /chats/:id/events`、`StreamChat` 及废弃的内部私有事件总线（已删除 `event_bus.go`）。 | `service.go`（助手与对话管理、上下文系统提示词组装与 Tool Loop）、`completer.go`（调用 OpenAI 兼容端点、分片聚合 tool_calls）、`tools.go`（中立工具定义与执行接口）、`stream.go`（SSE 事件帧切分） | 严禁导入 `sessions`、`instances`、`projects` 或 Agent 契约，与 Agent 流程物理隔离；系统提示词只注入请求上下文，不在对话消息气泡中体现；事件总线统一由 `events.Manager` 调度。 |
| **`platformtools/`** | 平台自身能力工具适配层。实现 `chat.ToolExecutor` 契约，将平台的项目管理（`projects.Service`）、提示词管理（`prompts.Service`）以及会话索引查询（`sessions.Service`）封装为 OpenAI 兼容的 JSON Schema Tools（`platform_list_projects`、`platform_list_prompts`、`platform_get_prompt`、`platform_create_prompt`、`platform_update_prompt`、`platform_delete_prompt`、`platform_list_sessions`）；由组合根（`cmd/platform`）统一组装并注入 `chat.Service`，使 Chat 中的轻量级 Agent 能够自主查询与管理平台项目、会话与提示词，实现严格的分层依赖倒置。 | `executor.go`（工具定义清单与分发路由） | 依赖 `projects`、`prompts` 与 `sessions` 领域服务，不直接接触数据库与底层进程。 |
| **`groups/`** | **团队领域服务（Team Center）**。负责团队定义（`Group`，含中控调度大脑模型 `OrchestratorModel`、中控大脑系统提示词与团队规范 `OrchestratorPrompt`）与团队专家成员纳管（`GroupMember`，直接多选复用全局角色库 `agent_roles`）；彻底移除独立分散的群组任务列表与流程任务表单，统一收敛至项目协同会话（Collab Sessions）：用户在新建会话中选择团队后，系统自动创建并调度中控协同会话，通过 `TouchAgentSessionByTaskID` 保证最后交互时间精确抬升；协同事件统一接入全局事件网关（`events.Manager`，channel: `tasks`）。 | `model.go`（团队领域模型/DTO）、`service.go`（业务逻辑与协同生命周期）、`repository.go`（持久化存储与成员联合查询）、`group_provider.go`（flow.GroupProvider 契约适配）、`handlers.go`（REST 路由编排） | 纯通用同构层；依赖 `roles`、`projects`、`tasks/flow` 与 `events`，不直接耦合底层特定 Agent 后端。 |
| **`tasks/`** | 计划任务与单 Agent 自动化调度引擎。支持标准 5/6/7 位 Cron 表达式（秒、分、时、日、月、周、年，配合前端 CronBuilder 可视化生成器、双向反解析引擎与后10次执行时间自动推断）与单次定时、并发配额控制池（最多 5 并发）、执行历史记录；任务中心已彻底移除多 Agent 流程任务（统一收敛升级至团队协同会话），聚焦于标准单 Agent 模板自动化任务。底层保留协同流程中枢引擎（`internal/app/tasks/flow/`：`Engine`、`FlowTools`、`TelemetryBus`、`EventBus`）专供团队协同会话驱动多 Agent 工作树闭环；全面支持团队协同提供者（`GroupProvider`），在派发专家 Agent 时直连会话启动器 `CreateInWorktree`，实现工作树隔离的团队任务闭环；配备独立的流程数据采集总线（`TelemetryBus`），通过 `sessions.Service.SubscribeSession` 强类型通道直连下游 Agent 会话实时事件流，实现低开销采集转发与轮次终结 Promise 自动汇聚；**协同子会话过滤屏蔽**：`GetTaskSessionIDs` 联合查询所有任务流（`flow_runs` 及对应 `flow_messages`）派生的底层专家子会话 ID，默认在普通项目/任务会话列表中过滤隐藏，确保会话树聚焦高阶业务流；流程任务流式执行事件与状态变更统一接入全局事件网关（`events.Manager`，channel: `tasks`），废除原独立 SSE 端点 `/api/tasks/flows/:runId/events`。 | `scheduler.go`（后台轮询调度器）、`executor.go`（标准任务执行器）、`service.go`（任务 CRUD 与状态同步）、`flow/engine.go`（内置大脑 ReAct 协同调度引擎与工作树派发）、`flow/tools.go`（中枢 Agent 工具集）、`flow/telemetry_bus.go`（流程数据采集总线）、`flow/event_bus.go`（协同流事件分发总线） | 任务触发时调用 `sessions.Service` 创建或复用特定 Agent 会话执行，协同流中枢大脑通过 `modelconfig.Service` 与 `Completer` 驱动，通用层与底层异构 Agent 完全解耦。 |
| **`terminal/`** | Web 在线交互式终端。本地 PTY 分配（Linux POSIX PTY 与 Windows 原生 ConPTY 双向支持）、本地 Shell 自动扫描（Windows 环境下优先探测并推荐 Git-Bash，兼顾 PowerShell 7 / Windows PowerShell / CMD；Linux 探测 bash/zsh/sh）与自定义路径手动配置及合法性校验（配置由 `settings` 表 `terminal.local_shell` 持久化，本地会话统一生效）、终端字体偏好配置（`GET/PUT /api/terminal/config` 与 `POST /api/terminal/config/reset`，支持用户自定义等宽字体并以平台统一字体 `Google Sans Code` + `Noto Sans SC` 作为默认与回退栈）、远程 SSH 会话池、WebSocket 双向字符转发、终端窗口 Resize 协同、同源 Origin 防御。REST 路由错误响应全面对齐 `errorResponse` 与 `c.AbortWithStatusJSON` 统一错误枚举规范。 | `handlers.go`（WebSocket 升级与同源拦截、REST 路由）、`service.go`（会话调度、本地 Shell 与终端配置管理）、`session.go`（LocalSession / SSHSession 封装） | 调用 `internal/core/platform` 分配底层 PTY；不依赖具体 Agent 业务。 |
| **`modelconfig/`** | 大语言模型服务商与模型元数据配置。管理 OpenAI 兼容服务地址、模型列表、API 凭据安全关联、端点动态解析。 | `service.go`（模型服务增删改查）、`repository.go`（配置存储） | 为 `chat` 服务及平台模型选择器提供基础模型字典。 |
| **`modelprobe/`** | 大模型连通性探针。提供模型服务地址可达性测试与模型可用性探针，向前端提供探测报告。 | `probe.go`（连通性测试与模型抓取） | 供设置中心与模型管理页调用。 |
| **`agentservices/`** | 机器级 Agent 后台守护服务管理。负责机器级第三方后台服务（如 OpenCode service）的健康探测、运维手动启停与启动门控。 | `service.go`（探测与启停协调）、`handlers.go`（运维管理 API） | 仅在后台服务支持的后端生效；不承担模型缓存失效职责。 |
| **`agentmodels/`** | 平台级 Agent 模型快照与治理。持久化每个后端最近一次刷新的模型清单，并支持平台级启用/停用及配置修改（显示名称、上下文上限、图片支持）；默认读取数据库，不调用 Agent；手动刷新时调用适配器并执行增量原子合并，保留用户自定义配置与启用状态；提供单模型编辑与全量启用/清空批量接口（`PUT /agent-backends/{backend}/models/enabled`）。 | `service.go`（List/Refresh/UpdateModel/SetAllEnabled）、`repository.go`（快照读写、增量合并与批量启停）、`handlers.go`（模型 API） | 只依赖 `core/agents` 契约、注册表与数据库；不感知具体后端；不提供项目级清单。 |
| **`agentusage/`** | 平台级 Agent 使用量与配额查询服务。提供对支持配额统计的 Agent 后端（实现 `UsageReporter` 契约）的实时配额、速率限制与使用量查询（路由 `GET /api/agent-backends/{backend}/usage`）；通过统一注册表解耦异构后端，通用层不包含任何具体后端字面量，支持前端手动刷新与实时状态呈现。 | `service.go`（服务封装与类型断言）、`handlers.go`（REST 路由编排） | 纯通用同构层，仅依赖 `core/agents` 的 `Registry` 与 `UsageReporter` 契约；不感知具体后端。 |
| **`auth/`** | 用户认证鉴权与安全防御。用户密码哈希校验、Session 令牌生命周期、本机首次免命令行密码设置引导、全局 CSRF 双提交校验与自动轮转中间件。 | `middleware.go`（`RequireCSRF` / `RequireSession`）、`csrf.go`（令牌生成与双提交比对）、`handlers.go`（登录/登出/首次密码设置） | 全局路由拦截第一关。 |
| **`secrets/`** | 敏感凭据脱敏与安全存储。管理第三方 API Key 与 SSH 密码，提供流式/日志敏感数据实时脱敏过滤器。 | `store.go`（加密存储与读写）、`redact.go`（正则脱敏过滤） | 用于模型配置、终端 SSH 凭据以及防止日志泄露。 |
| **`settings/`** | 平台全局偏好设置。管理外观主题（浅色/暖阳/冷灰，移除深色；字号仅接受 12/14/16 三档，历史 12–20 值读取时归并到最近档位）、快捷助手开关、全局 Agent 系统提示词（AGENTS.md）落盘与跨后端配置同步、Tab 隔离的多 Agent 后端开关配置、可执行文件路径与运行模式、以及 dsh 守护进程空闲断开时长；并新增后端工具权限模式（confirm / auto）配置与"平台持久化会话数据"门控标志（`sessionDataOwner`，由 Descriptor 声明）。 | `service.go`（系统偏好与快捷助手开关）、`general.go`（全局系统提示词文件管理与跨 Agent 同步）、`agents.go`（各 Backend 的配置状态、空闲超时与权限模式）、`handlers_agents.go`（设置 API 与 agent DTO） | 仅消费契约的 `Catalog`，不直接感知后端实现细节；全局提示词以平台数据目录 `dataDir/AGENTS.md` 为唯一权威源，跨平台文件同步写入各后端，不依赖 Windows 受限的软链接；快捷助手开关独立于外观配置存储。 |
| **`proxy/`** | 动态 ContextPath 服务代理网关。将服务器静态文件目录（含目录树浏览与防穿越）或本地端口 HTTP 服务反向代理至平台单一端口；全协议透明转发、自动剥离 ContextPath 前缀、支持 WebSocket 协议升级（HMR）与端口连通性探测。 | `service.go`（CRUD、参数校验与端口探测）、`gateway.go`（动态反代与文件服务分发器）、`handlers.go`（管理 API） | 数据面（`/proxy/*`）完全免鉴权公开直通；控制面（`/api/proxies`）严格执行 Session 与 CSRF 认证；不感知具体 Agent 业务。 |
| **`prompts/`** | 平台提示词管理。管理角色类（`role`）与流程类（`flow`）提示词，支持多维度关键词与分类筛选，供角色管理、纯对话助手、多 Agent 协同流程与新建 Agent 会话作为统一权威源引用；新建 Agent 会话同时消费角色类与流程类提示词，角色类以 `<Role name="名称">`、流程类以 `<Flow name="名称">` 包裹后作为首条用户提示词发送；不再支持 `{user_prompt}` 占位符替换。 | `model.go`（模型定义）、`service.go`（CRUD 与分类过滤）、`repository.go`（持久化存储）、`handlers.go`（REST 路由） | 供平台各中心与 `platformtools` 组合调用；与具体 Agent 后端无关。 |
| **`agentdata/`** | 内置 Agent 转写数据域服务。承载"声明拥有自身会话数据"（Descriptor `OwnsSessionData`）的后端在平台数据库中的会话转写：adapter 侧经组合根注入的 `TranscriptStore` 契约读写，设置中心侧提供会话数据管理（分页列表 / 单条删除 / 批量清空，路由 `GET/DELETE /api/settings/agent-data/sessions...`）。删除时联动清理会话工作台索引行（组合根绑定 `sessions.Service.DeleteIndexByNative`），运行中的轮次拒绝删除（409 `session_busy`）；本包不出现任何后端 ID 字面量，服务的目标后端由组合根绑定。 | `store.go`（GORM 转写表读写与生命周期管理）、`handlers.go`（管理 REST API） | 只依赖 `database` 与 `core/agents`；purger 与 activity gate 为组合根注入的函数钩子，保持包的完全后端中立。 |
| **`notes/`** | 平台笔记与技术随笔管理。支持 Markdown 格式笔记的独立持久化存储、全局与指定项目所属维度筛选、模糊关键词检索、防抖自动保存与快捷键保存；提供标准的 RESTful 路由编排（`GET/POST/PUT/DELETE /api/notes`），挂载于会话鉴权与 CSRF 保护之下。 | `model.go`（模型与 DTO）、`service.go`（业务规则与校验）、`repository.go`（持久化读写与过滤）、`handlers.go`（REST 路由编排） | 仅依赖数据库与标准库，不依赖具体 Agent 后端；作为平台一等中立知识记录实体。 |
| **`roles/`** | 全局专家角色管理。将多 Agent 协同流程中的角色抽取为平台一等实体，统一人设名称、职责描述、执行引擎（Backend）、分配模型（Model）与标识色彩；统一关联并动态同步提示词库中的角色类提示词，供多 Agent 协同流程与对话助手按需选用与复用。 | `model.go`（角色模型与 DTO）、`service.go`（角色 CRUD 与提示词动态同步）、`repository.go`（持久化存储与平滑兼容转换）、`handlers.go`（REST API 路由） | 依赖 `core/agents` 契约与 `prompts.Service` 接口，与具体后端实现解耦。 |
| **`database/`** | 平台数据持久化基础设施。GORM 数据库连接池初始化、版本化自动迁移（当前 v40 迁移链，v35 含 `prompts` 表增加 `type` 字段区分角色类与流程类、全局专家角色管理表 `agent_roles`、`chat_groups` 增加 `prompt_id` 实现对话助手对提示词管理的统一引用机制、`agent_models` / `agent_model_catalogs` 增加 `enabled` 字段、服务代理映射表 `service_proxies`，以及多 Agent 协同流体系核心表 `flow_roles`、`flow_runs`、`flow_messages` 与 `tasks.orchestrator_model`；v36 新增内置 Agent 平台持久化转写表 `agent_builtin_sessions` / `agent_builtin_messages`；v37 为 `agent_models` 增加 `name_customized` 并回填旧自动名称；v38 新增 `project_worktrees` 工作树表，并为 `agent_instances` / `agent_sessions` 增加 `worktree_id`；v39 新增 `notes` 表实现平台知识笔记持久化；v40 新增 `groups`、`group_members`、`group_tasks` 表，并在 `flow_runs` 中引入 `group_id` 与 `worktree_id`）。迁移历史模型与当前活动模型（`models.go`）严格解耦：历史已 drop 的废弃表结构体（`legacyWorkspaceRoot`, `legacyAgentBackendConfig`, `legacyResource`, `legacyProjectResource`, `legacySessionPrompt`, `legacyManagedResourceFile`）作为私有影子类型内聚在 `migrations.go` 局部迁移闭包中，彻底移除 `models.go` 中已废弃的僵尸模型定义。设置纯 Go 嵌入式 SQLite 驱动安全连接池上限（MaxOpenConns=1），结合 WAL 模式与 5 秒 busy_timeout 消除并发写锁争用。 | `database.go`（SQLite 连接与连接池配置）、`migrations.go`（版本演化迁移逻辑与私有迁移模型） | 单一 SQLite 数据库；纯 Go 驱动无需系统 SQLite 动态库。 |
| **`runtime/`** | 单实例互斥控制。通过工作目录文件锁（lockfile）防止多个服务端进程并发冲突打开同一个 SQLite 数据库。 | `single_instance.go`（跨平台文件排他锁实现） | 组合根启动时最先获取，退出时最后释放。 |
| **`paths/`** | 跨平台路径解析与安全性校验。规范化路径表示，防范 `../` 目录穿越攻击，检验项目工作区授权边界。 | `paths.go`（安全路径判定与绝对路径规范化） | 被 `projects`、`instances` 等模块作为安全基座调用。 |

---

### 2. 核心契约与抽象层（`internal/core/`）
纯定义与中立策略，**禁止依赖任何业务层或异构适配层**。

| 模块目录 | 核心职责与关键文件 |
|---|---|
| **`agents/`** | **Agent 统一核心契约**：<br>• `contracts.go`：`BackendID`、`AgentAdapter` 主接口、能力声明 `Capabilities`、标准消息/模型载荷。<br>• `events.go`：类型化轮次 `EventTurn`、标准交互事件 `EventInteraction`、流式增量事件。<br>• `extensions.go`：平台命令模型（`PlatformCommand`、`CommandCategory`：action/prompt）与可选扩展接口（`SessionCompactor` 上下文压缩、`PermissionReplier` 审批、`HistoryReader` 历史回放、`ServiceController` 机器服务、`RawEventOpener`、`UsageReporter` 使用量与配额查询契约；命令由平台统一定义，不再由下游 agent 同步）。<br>• `descriptor.go` & `registry.go`：后端元数据描述符、`Catalog` 目录树与全局适配器注册表。 |
| **`skills/`** | **Agent Skills 平台通用规范与技能基础设施**：<br>• `skills.go`：Skill 模型定义、YAML Frontmatter 解析、技能指令提取（`ReadSkillInstructions`）、技能调用 Prompt 组装（`BuildSkillInvocationPrompt`）与 System Prompt 格式化（`FormatSkillsForPrompt`）。<br>• `loader.go`：三层级技能发现（项目级 `<projectDir>/.skills/`、用户级 `~/.agents/skills/`、平台级 `<dataDir>/skills/`）、优先级去重与快速解析匹配（`ResolveSkill` / `FindSkillByName`）。 |
| **`platform/`** | **操作系统差异化抽象**：<br>• `platform.go`：统一导出 `Process`、`PTY`（含 `ScanShells` 与 `DefaultShell` 契约）、`Terminal` 端口接口。<br>• `new_unix.go` / `new_windows.go`：Linux/Unix 与 Windows 的条件编译实现，Windows 下实现基于 Windows 10/11 `kernel32.dll` 的 ConPTY 封装与 Git-Bash/PowerShell 注册表/PATH 探测器。<br>• 子包 `policy/`、`fake/`、`unix/`、`windows/`：跨平台路径策略、测试伪造平台实现、各 OS 原生实现。 |
| **`textutil/`** | **通用文本工具**：<br>• `title.go`：依据首条消息智能截断与推导会话标题。 |

---

### 3. 异构适配层（`internal/providers/`）
具体 Agent CLI 或服务协议的物理翻译官。向组合根导出 `Descriptor` 并实现 `AgentAdapter`。

| 模块目录 | 适配模式与通信机制 | 职责说明 |
|---|---|---|
| **`opencode/`** | **CLI + RPC + 机器级 Service** | 封装 OpenCode V2 协议；内部按单一职责严格拆分为：`adapter.go`（核心适配契约与实例状态管理）、`server.go`（服务子进程启动/健康检查/端口与密码探测）、`stream.go`（SSE 事件流消费泵与多 Subscriber 广播分发；空闲 watchdog 的 idle 判定仅在泵阻塞于读取时生效——publish 正在等待慢 subscriber 的下游反压期间保持守门不复位，避免把"消费者慢"误判为"服务端静默"而强制重拨丢失上游原生事件）、`normalize.go`（原生事件清洗规整化与工具输出截断折叠）；自动从全局 `~/.config/opencode/AGENTS.md` 同步加载平台级系统规约；将原生 `session.text.delta`、`session.reasoning.*` 映射为标准事件流。 |
| **`pi/`** | **常驻子进程 + JSON-RPC** | 以 `pi --mode rpc` 运行；通过 `--append-system-prompt` 原生挂载数据目录全局 `AGENTS.md` 文件指令；通过 stdin/stdout 全双工派发 `prompt`、`abort`、`set_model` 命令；解析 Edit Diff 增量；承接扩展 UI 的 `confirm` → 权限确认映射。pi core 无内置工具审批，平台默认不注入审批扩展，行为与 CLI 一致；仅当 operator 通过 `PI_EXTENSIONS` 提供扩展时才可能出现确认卡片。 |
| **`deepseek/`** | **共享 Host 进程 + WebSocket** | 启动并监控共享的 `dsh` 服务（懒启动、30s 启动超时、支持平台设置空闲回收时长，默认 30min；注入 `DSH_AGENTS_HOME` 原生加载全局指令）；通过 WebSocket 客户端双向转发消息；管理子代理（Subagent）级联关系。 |
| **`builtin/`** | **进程内直驱（无外部二进制）** | 平台内置兜底 Coding Agent：直接驱动平台共享的 OpenAI 兼容模型服务（经组合根注入 `ModelResolver`：优先列出支持 function calling 的模型，当平台未勾选任何"支持工具调用"模型时回退列出全部可用文本模型以保证兜底可用；目录 ID 采用平台 wire model_id（跨服务同名时按 "provider/model" 限定），按标记行精确解析并兼容历史数字 ID），进程内自建补充 SSE 客户端与 ReAct 工具循环；工具集 `tools/`（`write_file`/`edit_file`/`execute_command`/`read_file`/`list_directory`/`glob_search`/`grep_search`/`git_status`/`git_diff`，内置受控进程执行与**读写分离的安全路径策略**：`read_file` 与 `list_directory` 通过 `ResolveReadPath` 支持绝对路径与 `~` 主目录展开，原生打通外部多层级 Agent Skills 与全局规约读取；`write_file` 与 `edit_file` 等变更工具由 `SafeResolveWritePath` 实施工作区与受控系统临时目录白名单（`os.TempDir()`、`/tmp`、`/var/tmp` 等规范化路径）沙箱隔离，并结合逐级符号链接校验防逃逸与敏感文件篡改；Git 与 Search 工具则维持 `SafeResolvePath` 严格单工作区限制）；高危工具经权限确认卡片（confirm / auto 两种模式，设置中心可配置）；**会话转写持久化于平台数据库**（经组合根注入 `TranscriptStore` → `internal/app/agentdata`），重启后会话经 `ContinueSession` + `HistoryReader` 从平台表恢复回放与 LLM 上下文；`Descriptor` 声明 `Embedded`（无路径/版本检测）与 `OwnsSessionData`（设置中心据此提供会话数据管理面板）。**系统提示词多层规约与技能体系**：集成多层级 `AGENTS.md` 动态加载（平台级 `dataDir/AGENTS.md` → 用户全局级 `~/.agents/AGENTS.md` → 项目工作区级 `<projectDir>/AGENTS.md`，支持 UTF-8 BOM 安全剥离与单文件上限保护）；集成 Agent Skills 规范技能体系（基于平台通用 `internal/core/skills`：扫描 `<projectDir>/.skills/`、`~/.agents/skills/`、`dataDir/skills/`，支持 YAML Frontmatter 元数据解析、项目级高优先级覆盖去重、并输出标准 `<available_skills>` XML 引导大模型通过既有 `read_file` 按需加载）；自动注入当前系统时钟（年月日/星期/时区）与操作系统架构信息消灭模型时序幻觉；**实现 `agents.SessionCompactor` 契约**，打通平台 `/compact` 命令行上下文压缩能力，避免长会话 Token 溢出。工具调用事件与转写严格成对：`tool_call` 转写记录只由执行入口 `executeToolCall` 写入一次，事件、转写与结果共用同一 `callId`（模型缺省时生成稳定兜底 ID），历史回放按 `callId` 一一配对；中断或执行中被取消的调用补发 `isError` 结果，终态不再悬挂为“运行中”；重建 LLM 上下文时将同一步的连续调用合并为一条 assistant `tool_calls`，并过滤无对应调用的孤儿结果，保证 OpenAI 兼容协议合法；流式文本/思考增量即时下发，不晚于其后的工具调用事件。 |
| **`agy/`** | **常驻子进程 + ACP (Agent Client Protocol)** | 封装 Google Antigravity CLI；以 `agy acp --stdio` 运行常驻子进程，基于全双工 JSON-RPC 2.0 传输层接入；通过 `initialize` 握手协商协议版本与能力，新建会话（`session/new`）或恢复会话（`session/load`），派发 `session/prompt` 驱动轮次，流式消费 `session/update` 通知（涵盖 `agent_message_chunk`、`user_message_chunk`、`agent_thought_chunk`、`tool_call`、`tool_call_update`）；支持模型切换（`session/set_model`）、模式设置（`session/set_mode`）与权限审批（`session/request_permission`）；新建会话不等待 agy 启动，先以 `pending-` 占位登记（`CreateSession` 不启动子进程），首轮 `SendMessage` 才启动子进程并从 `session/new` 取得真实 native `sessionId`，经 `SendMessageRequest.OnNativeSessionBound` 回写会话索引与事件总线重绑后派发消息；轮次执行与外部 HTTP 请求上下文解耦（采用独立 `turnCtx` 派生自实例上下文并由专属 `turnCancel` 保护，杜绝请求返回后引起的伪中断与状态跳变）；发送消息由平台 `sendTurn` 同步广播带完整附件的 `role: user` 消息，底层 ACP 的 `user_message_chunk` 通知作为入流确认静默消费，确保实时流严格有序且杜绝重复用户消息；工具调用解析保留真实 `name`（如 `ask_question`），确保人机交互工具通过 `DeliveryGate` 门禁；`tool_call_update` 设状态守卫仅放行终态（`completed`/`failed`），杜绝重复产生 `EventToolResult`，并从 `content` 数组无损提取文件 Diff；轮次结束时将 `EndTurn` 最终文本设为全量累积快照用于客户端收敛；精准映射 `session/cancel` 结果为 `TurnCancelled` 并提供超时兜底；实现 `SessionTurnDetector` 主动探测子进程轮次运行状态（`DetectTurn` 校验子进程活跃度并支持页面刷新、重连及自愈巡检时对齐 `TurnRunning`）；实现 `HistoryReader` 原生解析 `~/.gemini/antigravity-cli/brain/<id>/.system_generated/logs/transcript.jsonl` 转写文件，并在遇到 `truncated_fields` 截断标记时自动通过单向游标从同级 `transcript_full.jsonl` 按需无损补全被裁剪的回复正文与工具调用，杜绝历史回放中的 `<truncated>` 省略并保持毫秒级极速解析；**回放事件时间戳严格取自转写的 `created_at`**（缺失时沿用上一条真实时间、再以文件 mtime 兜底），收尾 `TurnEnded` 亦使用最后一步的真实时间，绝不使用读取时刻的 `time.Now()`，避免查看历史污染会话最后交互时间；通过 `agy models` 清洗提取平台级可用模型列表；实现 `UsageReporter` 契约执行 `agy -p /usage --output-format json` 查询结构化与纯文本使用量及配额，由设置中心提供可视化面板与手动刷新；支持空闲超时自动回收与进程组生命周期管理：统一接入 `platform.Process().ConfigureDetached(cmd)` 使子进程开启新会话、脱离平台控制终端，并通过 `platform.Process().Terminate(pid)` 级联终结整个进程树，结合 `cmd.Cancel` 上下文终止器与 `Attach` 孤儿托管，彻底杜绝僵尸进程与残留孤儿。 |

---

### 4. 前端应用架构（`web/src/`）

```
web/src/
  ├── api/             ── 网络传输层（client.ts 统管 CSRF，子文件映射后端 RESTful API）
  ├── stores/          ── 全局领域状态（agents, projects, settings, auth, models, chats, quickAssistant, drafts, viewport, debugPanel, fileTreeState, ...）
  ├── shared/          ── Provider 中性通用控件与工具（Markdown、PlantUML、Color、Format）
  │     ├── events/    ── 统一事件总线前端设施（event-bus.worker.ts 跨标签页单连接 SharedWorker：连接 error/CLOSED 后 3s 自愈重拨且状态保持 `reconnecting`（触发主线程 reconnectListeners 的带序号重订重放）、busId 所有权仲裁（首个 tab 建立 bus，后续 tab 经 init 回复采纳胜者 busId 并持久化，杜绝多 tab 各持 sessionStorage busId 互踩单连接）；UnifiedEventBusClient.ts 主线程单例与 fallback 本地 EventSource（CLOSED 同样自愈重拨）；`addEnvelopeTap` 观测钩子：在 `dispatchEnvelope` 单一分发汇聚点 try-catch 隔离地通报每一条信封（SharedWorker 与 fallback 双路径、含 `dispatchRawFrame` 兜底合成信封天然全覆盖），供调试捕获等纯观测消费者挂接，tap 内抛出异常绝不影响正常消息分发；event-types.ts 全线事件类型单一来源清单（worker 与 fallback 共享，含 `lifecycle/unknown/ready`，防漂移））
  │     ├── composables/ ── Provider 中性通用设施（useDraggablePosition.ts 浮动拖拽定位与持久化；useActivityPane.ts 会话工作区活动面板激活状态与布局控制）
  │     ├── components/── Provider 中性业务通用控件（ModelSelect.vue 平台统一模型选择器，下沉至通用层，供 Chat、Flow 任务与角色管理统一复用；ServerDirectoryPicker.vue 服务器目录选择器，供服务代理与项目管理可视化浏览/选定目录；CronBuilder.vue 可视化 Cron 生成器；RunningIndicator.vue 呼吸态标识；BackendTag.vue；BackendIcon.vue Agent 官方图标（无图标回退短名芯片，图标加载失败同样回退）；PlantumlViewer.vue PlantUML 放大查看器（缩放/平移，供聊天图表块与文件预览共用）；BackendDiagnostics.vue）
  │     ├── utils/     ── Provider 中性工具函数（promptBlock.ts 协议化 Prompt 块文本解析与处理）
  │     ├── styles/    ── Provider 中性样式片段（composer-select.css 统一选择器微质感样式）
  │     └── ui/        ── 平台标准 UI 组件库（纯中性业务解耦设计，基于 Tailwind CSS）：
  │           ├── atomic/    ── 原子组件（UiButton, UiInput, UiTextarea, UiSelect, UiDropdown, UiBadge, UiProgress, UiTooltip, UiTabs, UiAlert, UiEmpty, UiSpinner, UiDivider）
  │           ├── compound/  ── 复合容器组件（UiDialog, UiFormDialog, UiComposer, UiScrollFollow, UiBubble, UiTree, UiTreeNode）
  │           └── index.ts   ── 统一入口导出
  ├── features/        ── 业务领域垂直拆分目录（各 feature 规范化通过 index.ts 导出对外接口，非 AppShell 跨 Feature 越界相对引用清零）：
  │     ├── chat/      ── Agent 工作台（核心）
  │     │     ├── composer/      ── 输入框组件、模型选择器、图片附件槽、斜杠命令调度器（slashActionDispatcher.ts）与多源补全、通用附件上传调度器（useAttachmentUploader.ts）与键盘快捷键控制器（useComposerKeyboard.ts）
  │     │     ├── execution/     ── SessionEventStream.ts 事件流消费与状态维护；EventBusClient.ts 会话↔统一总线装配（带序号重订触发 `ReplaySince`/resync 重放）；会话 store 的 applyTurn 在轮次结束时若仍有未收到 final 的流式条目，延迟触发 repairDanglingEntries 按 history final 快照回填（在线丢帧自愈）
  │     │     ├── interactions/  ── 权限确认卡片、表单队列、审批事件派发
  │     │     ├── pane/          ── 会话主面板子组件解耦拆分：SessionHeader.vue 接管会话头部元数据、运行状态、工作目录/分支与删除触发；WorkspacePreviewPane.vue 接管会话右侧工作区（已彻底物理移除废弃的 ChatSideRail.vue，文件树与文件预览 Tab 切换、外框总宽度锁定与拖拽、纯图标折叠控制）；useActivityPane.ts 共享视图状态
  │     │     ├── rendering/     ── MessageView.vue 消息/交互卡片渲染（纯净主对话流只渲染对话消息、问答交互卡片与权限审批卡片，剥离思考与常规工具调用）、logModel.ts 纯逻辑
  │     │     ├── state/         ── 会话日志推导状态模型（sessionLog.ts）
  │     │     ├── pure/          ── 独立纯 Chat 对话工作台（第二栏专属展示助手列表，第三栏无 Tab 左右双列：左侧主对话面板 + 右侧助手对话清单）
  │     │     └── quick/         ── 快捷助手横向方形工具栏、每次打开新建对话的 480×600 弹窗、自由拖拽定位（复用 shared/composables/useDraggablePosition）与最近对话下拉
  │     ├── workspace/ ── 平台三栏主框架（AppShell.vue，48px 紧凑一级栏与 272px 二级侧栏，z-index: 70 全圆角防遮挡折叠把手；安装 openTabs 容量裁剪守卫：轮次 running 的会话 Tab 永不被自动 trim 挤掉——被挤掉即触发无条件退订、后台流式会话失联，手动关闭仍走退订；引入 groupsMode 调度中间栏群组列表 GroupsSidebar 与 enginesMode 调度引擎侧栏 EnginesSidebar）、分类导航条（CategoryRail.vue，48px 紧凑宽度，一级菜单顺序：项目 → 对话 → 团队 → 任务 → 笔记 → [隔离线] → 引擎 → 设置，终端以全局悬浮窗 ProjectTerminalModal 与设置中心终端服务器配置协同提供）、全局浮动入口挂载点（quickAssistant 启动器 + debugPanel 调试入口 `DebugLauncher.vue`，随各自开关条件挂载）、会话 Tab
  │     ├── engines/   ── 智能体与引擎配置中心（EnginesView.vue 主视图、EnginesSidebar.vue 二级导航（`el-menu` router 模式，与组织侧栏同构））：助手引擎（AssistantEngineView.vue，单轮最大工具调用步数与平台管理工具控制）、会话引擎（CoderEngineView.vue，单回合最大执行步数控制与工作区规范分层）、协同引擎（OrchestratorEngineView.vue，多 Agent 流程最大编排步数调度）、Agent配置（AgentSettingsView.vue，异构 Agent 后端启用/路径检测/模型目录/空闲断开与 service 守护进程，内嵌 features/settings/ 的 BuiltinDataPanel 会话数据管理与 AgentUsagePanel 用量配额面板）；菜单全量 Element Plus（零 Ui*）：步数控件统一 EngineStepControl（el-input-number + el-segmented），保存结果只报一次 ElMessage，破坏性确认走 ElMessageBox，模型编辑走 EntityFormDialog，详见 docs/architecture/frontend-ui-system.md §八
  │     ├── organization/ ── 组织中心（OrganizationView.vue 主视图、OrgSidebar.vue 二级侧栏（`el-menu` router 模式），二级菜单顺序：AI 提示词 → AI 助手 → AI 员工 → AI 团队，纯对象资产与实体治理）：AI 提示词（OrgPromptsView.vue，分类管理角色类与流程类提示词资产）、AI 助手（AssistantListView.vue / AssistantEditDialog.vue）、AI 员工（EmployeeListView.vue / EmployeeEditDialog.vue）、AI 团队（TeamListView.vue / TeamEditDialog.vue）；四页共用 `shared/components/ListToolbar.vue`（工具条）与 `shared/components/ListEmptyState.vue`（loading / filtered / empty 三态），页头与节奏类来自 `shared/styles/list-page.css`
  │     ├── groups/    ── 团队群组工作台（GroupsView.vue 团队主视图、GroupsSidebar.vue 遵循 sidebar.css 标准规范的三段式侧栏、GroupEditPanel.vue 主工作台全屏面板支持团队创建与编辑无遮挡无浮层体验、GroupDetailView.vue 团队详情/中控大脑设定预览/专家成员看板/项目与工作树隔离任务列表、GroupTaskCreateModal.vue 群组任务创建弹窗、GroupFlowChatDialog.vue 独立弹窗协同群聊、CollabSessionChatView.vue 协同会话群聊主体视图，支持顶部中控/专家芯片、协同消息流、决策卡片与底层专家子会话抽屉检视）
  │     ├── projects/  ── 项目侧边栏（ProjectSidebar.vue，统管 Zone A 操作【新建项目/项目管理】/Zone B 检索与过滤/Zone C 树容器）、项目设置 Tab 页面（ProjectSettingsView.vue，项目改名、路径只读、Git 状态、工作树创建/列表/移除与非 Git 说明）、全局项目列表管理视图（ProjectsManagementView.vue，支持 Tab 中调整项目顺序与显隐、打开项目设置、解除登记）、项目树（ProjectTree.vue，取消定时轮询，采用本地状态就地响应与按需加载；Git 项目名称前显示 Git 标识；项目行提供新建会话与项目设置入口；展开项目时加载工作树标签）、会话卡片项（ProjectSessionItem.vue，展示工作树分支标签）、统一项目弹窗（ProjectManageDialog.vue，仅 register/create 两种模式）、新建项目弹窗（支持自定义项目名称、可视化路径选择与子目录创建）
  │     ├── proxies/   ── 服务代理的可复用部件：创建/编辑表单弹窗（ProxyEditDialog.vue，`EntityFormDialog` + `el-form` 校验）、可视化目录选择器（ProxyDirectoryPicker.vue，薄包 shared/components/ServerDirectoryPicker.vue）。旧的整页列表 ProxiesView.vue 已删除（无路由引用），代理管理唯一入口是设置中心 → 服务代理
  │     ├── settings/  ── 设置中心（SettingsView.vue，居中 1080px 滚动画布）：外观设置（AppearanceSettings.vue，12 款预设主题走 `el-radio-group` 卡片、字号三档、PlantUML 渲染引擎与快捷助手开关）、大模型配置（ModelSettings.vue，每服务一张分区卡 + 内嵌 `el-table`，候选模型走 `el-checkbox-group`，服务/模型两个 `EntityFormDialog`）、服务代理设置（ProxySettings.vue，`el-table` + 行内 `el-switch`）、服务连接（ConnectionSettings.vue，`el-table` + `EntityFormDialog`，「测试连接」在弹窗 `#head-extra`）、终端服务器（TerminalSettingsView.vue，字体预设 `el-select` + 字体族输入、本地 shell `el-radio-group` 卡片、SSH 服务器 `el-table` 三态空态）、调试面板设置（DebugSettings.vue，localStorage 本浏览器开关控制全局调试入口显隐，不落库）、设置侧边导航（SettingsNav.vue，`el-menu` router 模式，与 OrgSidebar / EnginesSidebar 同构，顺序固定：外观 → 模型 → 服务代理 → 服务连接 → 终端服务器 → 调试面板）。**全模块已 Element Plus 化（零 `Ui*`、零 `dark:` 工具类）**：页头与分区卡节奏来自 `shared/styles/settings-page.css`（画布例外，不套 `.list-page-*` 三段式、表格不分页），表格列宽预算 866px 由 `tests/settings-table-budget.spec.ts` 守卫，破坏确认走 `ElMessageBox`、结果只报一次 `ElMessage`；时间列用 `shared/format.ts` 的 `formatListTime()` 手工拼接 `YYYY-MM-DD HH:mm`。历史死代码 SettingsRolesView.vue / SettingsPromptsView.vue（提示词管理早已迁至组织中心首项）已删除；包内 `BuiltinDataPanel.vue` / `AgentUsagePanel.vue` 空存在 `features/settings/` 但只被引擎中心（Agent 配置页）使用
  │     ├── terminal/  ── Web 终端悬浮窗工作区（ProjectTerminalModal.vue 全局悬浮弹窗支持多窗口、多 Tab 拖拽缩放与多服务器连接、XtermView.vue xterm.js 核心终端画布交互、ServerDialog.vue 服务器配置弹窗）
  │     ├── notes/     ── 笔记中心（NotesView.vue 沉浸式 Markdown 双栏/仅编辑/仅预览工作台、NotesSidebar.vue 三段式列表、NoteItem.vue 实体卡片，支持项目归属维度与全局随笔记录、800ms 防抖自动落库与 Ctrl+S 快捷保存）
  │     ├── tasks/     ── 计划任务管理、执行面板、任务专属对话弹窗（TasksView.vue）、任务侧边栏（TasksSidebar.vue）、新建面板（TaskCreatePanel.vue，支持普通任务与多 Agent 协同流程创建，中控调度模型基于已配置 Chat 文本模型下拉选取，团队协同角色直接从角色库选用并展示）、协同工作流子目录 `flow/`（`FlowChatView.vue` 核心 IM 群聊协同工作台、`FlowMessageItem.vue` 多角色气泡，`.markdown-body` 排版复用 `shared/markdown-body.css` 全局样式（组件内只保留用户气泡的白色覆盖）、代码块复制与代码内 `@角色` 保护、`FlowRoleBadge.vue` 角色专属彩色徽标、`FlowRoleManager.vue` 团队角色配置面板，支持从全局角色库导入专家角色并统一引用提示词）
  │     ├── prompts/   ── 提示词管理核心组件（PromptManageDialog.vue 支持角色类与流程类单选区分、promptBlock.ts 角色/流程标签构造与解析；会话首条提示词与消息渲染共用该协议）
  │     ├── files/     ── 文件查看器（FileViewer.vue，支持普通查看与 Diff 双模式，CodeView 支持常用 37+ 语言语法高亮、右上角换行切换，移除冗余复制/查找按钮并保留 Ctrl+F 查找快捷键与行内搜索匹配项/当前项视觉高亮）、文件树与侧边栏（FileSidebar.vue / FileTree.vue，样式全面适配系统主题变量，支持大文件上传【1 GiB 请求上限与流式写入】、实时上传进度条【UiProgress】与滚动文件清单、下载与删除二次确认防误删）、变更树组件（ChangeTree.vue，树形层级展示变动文件，支持暂存/取消暂存/放弃修改悬浮操作）、本地分支选择器（BranchSelector.vue，支持分支切换与快速新建）、Git 提交操作栏（GitCommitBar.vue，支持 Ctrl+Enter 快捷提交与自动暂存闭环）、Diff 预览器（DiffViewer.vue，Unified Diff 高亮对比与双列行号）、Markdown/PlantUML/Mermaid/图片 Artifact 预览（MarkdownArtifactView.vue 预览集成 Mermaid 与 PlantUML 图表渲染和代码高亮、PlantumlArtifactView.vue 为 `.puml` 等源文件的图表预览，默认出图、预览/源码切换）、HTML 源码高亮与“🌐 预览”独立弹窗沙箱预览（HtmlArtifactView.vue、HtmlPreviewModal.vue）
  │     └── debug/     ── SSE 总线调试面板（settings 开关只控制全局浮动入口按钮；抽屉内「开启 / 停止」显式控制经 unifiedEventBus.addEnvelopeTap 的捕获，关闭抽屉不停止捕获、不清空已捕获帧，停止后再开启按暂停/续接处理；关闭入口开关会停止捕获并保留当前缓冲；频道/标识关键字/事件类型三重过滤，环形缓冲最近 500 帧、超限计数丢弃提示；列表序号直接取统一总线信封的 SSE id / sequence，缺失显示 —，不在前端重新编号；不持久化任何消息；开关仅存本浏览器 localStorage `dev-platform-debug-panel-v1`，纯观测、零后端改动。浅色台账式界面：顶部四格捕获摘要（缓冲 / 当前命中 / 缓冲丢弃 / 错误，错误可点击直达）、频道计数标签、时间/事件/频道/目标/标识列、行内展开原始载荷并一键复制、平板与手机端两行自适应）
  └── router.ts        ── 路由映射与鉴权守卫（SPA 路由入口）
```

#### 平台标准 UI 组件库与设计系统规范（Data Terminal）
> 详细设计系统规范、色彩 Token、字符对齐律、状态交互法则（The Move）与图标规范详见：👉 [`docs/architecture/design-system.md`](./docs/architecture/design-system.md)

1. **纯中性设计与业务解耦**：
   - 提取的组件严格置于 `shared/ui/`，保持与具体业务领域（Chat、Tasks、Projects 等）彻底解耦。
   - 例如：`UiSelectMenu` 提炼为通用的现代下拉选择器（支持触发器展示主副文本、弹出浮层、搜索过滤、快捷清除与多选/单选无障碍支持，彻底替代各业务模块的裸露原生 `<select>`）；`UiProgress` 抽离为支持 Tooltip 提示的通用进度条组件；`UiComposer` 抽离为通用可拖拽高度的复合输入交互容器；`UiTree` 抽象通用的树形层级交互骨架；`UiConfirmDialog` 统一承载删除、中止等二次确认，默认 Danger 按钮、支持加载态与弹窗内错误展示。
2. **第二栏与侧边栏列表标准规范（基于 design-taste-frontend 深度重构）**：
   - 全平台第二栏（中间导航与对象索引栏：项目、对话、群组、任务、笔记、设置）统一采用**三段式紧凑架构（Header & Actions / Search & Filter / Entity List）**，样式统一由 `shared/styles/sidebar.css` 标准化承载：
     - **Zone A 紧凑 Header 与标准化操作区**：标题高度收敛（13px，`font-weight: 600`），右侧集成 22px 微型方圆角停靠与展开控制按钮（`var(--radius, 4px)`）；操作按钮统一收敛为 26px 极简扁平微圆角按钮，前置 1.5px 几何矢量 SVG 加号图标（消除纯字符 `+`）；设置页彻底移除内部冗余二级标题，与外壳单头部绝对对齐。
     - **Zone B 紧凑检索与微过滤**：统一 26px 极简工具栏，内嵌 1.5px 细线放大镜矢量图标与一键清空按钮，移除输入框聚焦外发光（glow / ring）与外层大按钮，大幅释放首屏纵向阅读高度。
     - **Zone C 紧凑列表与导航卡片（Dense Pitch）**：
       - **层级导航**：项目树由独立的 `ProjectSidebar.vue` 包装，`ProjectTree` 采用两级树状折叠（1.5px 矢量旋转 SVG 折叠角、等宽计数徽标及悬浮快捷加号微按钮）；对话模块第二栏 `ChatSidebar` 则专门聚焦为**纯助手列表**（图标徽标 + 助手名称 + 助手名称旁快捷新建对话按钮 + 会话计数 + 悬浮淡入的编辑/删除操作），下属对话剥离至第三栏右侧对话清单（`ChatHistoryPane`）呈现；对话清单严格按当前选中助手（包含未分组与指定助手）按需分页拉取，底部支持“加载更多”按需追加，保障大量对话场景下的流畅度，并支持单会话删除（Pinia store `deleteChat` 级联自愈）。
       - **实体列表项排版**（覆盖会话项、助手项、群组项、任务项、笔记项、设置项）：全量采用标准紧凑卡片（徽标/状态/呼吸指示器 + 语义标题单行截断 + 悬停淡入的 18×18px 快捷操作工具条；属性元数据 + 右对齐 `tabular-nums` 等宽时间戳）。
       - **选中与激活法则（The Move）**：所有列表项激活态严格收敛为**左侧边缘 2px accent 科技蓝实线标尺（`border-l-2 border-l-[var(--color-accent,#4a8ad6)]`）** 与阶梯底色（`var(--surface-muted)`），全平台第二栏彻底清除静止投影（`box-shadow`）与毛玻璃模糊。
       - **色彩预算**：彩色严格控制在 3% 以内，任务与终端状态仅在运行/阻断错误时显色，已完成与常规状态保持中性阶梯底色。
   - **折叠/展开交互按钮**：全平台统一采用紧凑方圆角微按钮（`var(--radius, 4px)`），展开与收起按钮双向镜像对称，消除突兀的长胶囊贴边条与文字大块按钮。
   - **会话工作台解耦与右侧工作区布局联动**：彻底物理移除已废弃的会话右侧细导航栏组件（`ChatSideRail.vue`）及其孤儿状态，将原先 2600+ 行的巨石组件 `SessionChatPanel.vue` 按职责解耦拆分，抽离出独立的 `SessionHeader.vue`（承载头部状态与信息）与 `WorkspacePreviewPane.vue`（承载右侧工作区）；在页面顶层会话 Tab 栏（SessionTabs）最右侧增设极简纯图标的布局控制按钮（`SidebarRight` 图标），统一控制右侧工作区面板的展开与收起；工作区面板内部采用统一的 Tab 架构排布，文件树直接作为常驻 Tab（`tab-filetree`）展示，与打开的文件预览 Tab 平行并列切换，彻底消灭左右两列分割对代码阅读区的挤压；在文件树中点击文件时新增/激活对应文件 Tab，关闭所有文件预览时自动切回文件树 Tab；外层总宽度（`previewWidth`）锁定并支持平滑拖拽调整，外框对主对话区的占用宽度分毫不变，彻底消除主对话区文字与代码块的任何跳变和晃动；操作按钮全面纯图标化，去除冗余文字描述，留出最大的代码审阅空间；支持 `hideSideRail` 精简模式，用于任务会话查看弹窗等内嵌场景。
   - **终端操作体验增强**：在线终端（XtermView）原生集成“划选即复制”（Selection Copy）与“右键粘贴”（Contextmenu Paste），提升终端操作流体验。
3. **Data Terminal 极客终端视觉风格与 Token 规范**：
   - 全局遵循 **Data Terminal** 终端工程风格（基于 `design-taste-frontend` 开发者工具标准）：
     - **色彩预算**：视口彩色像素严格在 3% 以内；深石墨灰基调（`--ground: #0f1115`，`--surface: #14171f`），纯平 1px 细线边框（`--border: #222631`），彻底清除静止卡片阴影（`box-shadow`）与毛玻璃模糊（`backdrop-filter: blur`）。
     - **排版与对齐**：全局强制数字等宽（`font-variant-numeric: tabular-nums`），离线内嵌 Google Sans Code 与 Noto Sans SC；严禁全大写宽字距标签（micro-label 禁令）。
     - **圆角收敛**：组件外框与卡片统一收敛为平直微圆角（`--radius: 3px~4px`）。
     - **交互激活（The Move）**：所有 Tab 标签、导航项、范围选择器的激活状态统一使用紧贴底边或侧边的 1px/2px 科技蓝强调线（`--color-accent: #4a8ad6`）。
     - **纯净矢量图标系统**：全站杜绝原生 UI Emoji，统一升级为 1.5px/1.8px 细线条极简几何矢量 SVG 图标。
4. **操作体验规范统一**：
   - 弹窗操作顺序统一规范：取消按钮居左（Secondary 样式），确认/执行按钮居右（Primary 或 Danger 样式）；全量统一使用 `UiDialog` / `UiFormDialog`，二次确认统一使用 `UiConfirmDialog`，消除各业务自造的手写遮罩层；全量支持 Esc 关闭与 Tab 焦点陷阱。
   - 状态标识不得混用语义：设置中心 Agent 标签（`AgentSettings.vue`）标题后的状态圆点表示**平台启用状态**（绿=已启用、灰=未启用，取 `agent.enabled`），可执行文件检测结果只在卡片内部以徽标呈现（`agent.available` → 已检测到/未检测到）；共享侧栏卡片 `.sidebar-card-row-bottom` 默认 `align-items: center`，任何把它改成纵向堆叠的变体（如 `NoteItem.vue` 的 `.note-bottom`）必须显式声明 `align-items: stretch; justify-content: flex-start; text-align: left` 并让元信息行 `width: 100%`，否则摘要与项目/时间会被水平居中。
   - **禁止在生产代码中调用浏览器原生 `window.confirm` / `window.alert` / `window.prompt`**；确认统一使用 `UiConfirmDialog`，错误提示统一使用 `UiAlert`，并由 `web/src/tests/native-dialog-guard.spec.ts` 扫描源码阻断回归。
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
| **`desktop/src/main/service-manager.ts`** | 本地 Go 服务端（Sidecar）生命周期调度器。在本地模式下派生 `bin/platform` 子进程，监听回环地址随机端口（`-listen 127.0.0.1:0`），自动从日志中探测绑定端口并做 `/api/health` 可达性校验（端口识别后立即解绑标准输出/标准错误流监听并清空缓冲区，防止日志无限膨胀阻塞主进程事件循环）；应用关闭时发送 SIGTERM 优雅退出，并在 5 秒超时后兜底回收，彻底消除孤儿进程。 |
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
4. 异构适配层 (internal/providers/*)。内置 Agent（builtin）无外部子进程：其适配器在进程内驱动模型补全与工具循环，并以 `OwnsSessionData` 后端身份把会话转写写入平台数据库（`internal/app/agentdata`），历史回放与重启续接均取自平台表
   - 将标准消息请求转化为后端专有命令 (如 pi 的 rpcCommandPrompt)
   - 捕获后端原生输出 (stdout / WebSocket)
   - 适配器将原生事件转化为标准 NormalizedEvent:
       • EventTurn (started)
       • EventMessage (text_delta, reasoning_delta)
       • EventThinking (思考内容流式输出)
       • EventToolCall (各后端提问工具如 ask_user_question 由适配器统一归一化为标准 question 契约；入参原样透传，不做裁剪) / EventToolResult (输出为工具原始输出，不做摘要；编辑工具经 metadata.files 携带统一 diff 供前端渲染 +N/-N 与补丁，缺失时回退纯文本；问答工具保留完整答案 payload 供前端结构化解析)
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
   - 问答交互工具与纯净对话流（MessageView display="messages"），完整承载用户消息与答复、人机问答交互卡片（已将思考过程与工具调用过程移出会话对话流，保持高信噪比与高阅读舒适度）；底部待回答问答面板支持向下收起/展开，展开时限高并在内部滚动，避免面板过大挤压对话审阅区；答案在所有题目输出完成后于底部统一展示，解析 JSON 答案并以 1 2 3 序号每行逐项对应
   - 主对话流动态运行状态指示（MessageView kind-running）：移除原有静态“正在运行...”，采用 deriveRunningAction 基于最新活动动态解析动作语义（如“正在思考...”、“执行 <cmd>”、“编辑 <path>”、“读取 <path>”、“搜索 <query>”等）并以微动效卡片实时呈现
   - 右侧面板整合为统一工作区面板（彻底移除「链路」Tab，文件树直接作为常驻 Tab 与打开的文件预览并列切换），由顶层 Tab 栏最右侧纯图标布局按钮统一控制显隐；在文件树中点击文件时新增/激活对应文件 Tab，关闭所有文件预览时自动切回文件树 Tab；外框总宽度恒定锁定，绝不占用或晃动主对话窗口宽度；操作按钮全面纯图标化。
   - 会话历史与实时 SSE 报文精简削减：为消除巨量 Thinking 思维链、频繁的后台工具调用（含 pi 的 `toolcall_*` 增量）及未识别事件（unknown）对平台性能与长连接稳定性的冲击，削减在**消息格式归一时**完成：各适配器的归一化出口（pi 解析器与历史读取、opencode 实时泵与历史读取、agy 归一化与转写解析、deepseek 帧/投影归一化与历史、builtin 实时发布与转写回放）统一经过契约层 `internal/core/agents/delivery.go` 的 `DeliveryGate`（`Admit` 逐条准入、`AdmitAll` 批量准入）。仅放行对话正文（EventMessage）、人机问答交互（`IsQuestionTool` 的 ToolCall 与 ToolResult）、权限确认（EventInteraction）、轮次状态（EventTurn）及生命周期/错误；EventThinking、非问答的普通工具调用与结果、EventUnknown 在归一化阶段即被移除，不进入实例 ReplayBuffer、会话总线、SSE 或历史回放，`internal/app/sessions` 不再做二次过滤（原 `filter.go` 已删除）。内置 agent 的 LLM 上下文仍由转写库（TranscriptStore）中的完整记录重建，不受削减影响；分页由 `limit`/`before`（返回 `hasMore` 与不透明 `nextCursor`）承担，工作台首屏取最近 200 条并在顶部按需加载更早页，mention/repair 仅取 100 条尾部窗口；每页按用户消息边界对齐、`sequence` 从 1 重排，游标依赖 transcript 追加写；`includeImages=false` 时支持懒加载的适配器（opencode、builtin）清空内联 base64 并返回图片 URL，由 `GET /projects/{id}/sessions/{sessionId}/history/images/{imageId}` 按需读取，前端 `<img loading="lazy">` 渲染
   - Markdown 代码块复制按钮：结构由 `web/src/shared/markdown.ts` 输出，结构样式统一在 `web/src/shared/markdown-code.css`（在 `main.ts` 全局引入），点击委托统一在 `web/src/shared/markdown-code-copy.ts` 的 `handleCodeCopyClick`。所有 `.markdown-body` 表面（聊天、任务流、笔记预览）共用；表面如需自定义代码块配色（如聊天页的深色代码块），仅在其 scoped 样式中覆盖颜色，禁止再复制结构规则。
    - Markdown 通用排版（标题层级、列表符号、行内代码、引用、表格、链接）统一抽在 `web/src/shared/markdown-body.css`，同样在 `main.ts` 全局引入，为 `.markdown-body` 直接提供非 scoped 的默认排版；Tailwind preflight 会清零标题字号与 `ul/ol` 列表符号，任何新增的 Markdown 预览表面只要挂上 `.markdown-body` 即自动获得完整渲染，无需在组件内重复定义（聊天与任务流气泡原先的 scoped 重复规则已删除，仅保留深色代码块配色覆盖）。
    - 用户消息长文本折叠（`features/chat/rendering/FoldableUserText.vue`）：用户气泡正文默认只显示前 10 个**视觉行**，超出部分默认向上折叠，尾部提供「展开 / 收起」按钮。折叠高度按渲染后的实际行高（`lineHeight × 10`）与元素 `scrollHeight` 比对得出，自动换行按视觉行计数而非按换行符，并由 `ResizeObserver` 跟随气泡宽度变化重新测量；未超过 10 行不渲染按钮。历史回放与实时消息共用同一组件，提示词标签（`<Role>/<Flow>`）标签后的用户正文同样适用。
    - 右侧工作区文件面板一体化与快捷操作：文件树直接作为常驻 Tab 在右侧文件面板中展示，与打开的文件预览 Tab 并列切换，移除单独抽屉、浮动层与遮罩；顶层 Tab 栏右侧提供面板总开关；文件树基于 resolveProjectSafeDir 规范级联目录相对路径，完整支持任意层级的深层子目录懒加载展开；会话 Markdown 中的文件超链接点击时直接在文件面板中打开并自动展开面板，全链路支持项目内绝对路径识别、外部安全目录（/tmp、/var/tmp、Agent 数据目录）白名单安全放行、URL 编码解码与 Hash 行号剥离。
    - 会话视口状态记忆（Session & Chat Viewport State Restoration）：通过 useViewportStore 独立记录每个会话主消息列的滚动条位置（scrollTop）与粘底状态（pinned）。当用户切换菜单、路由或切换 Tab 时，离开前自动固化当前视口状态（具备组件卸载虚假滚动事件过滤，防止记录 scrollTop=0）；再次切回该会话时：若此前处于非粘底状态（如翻看历史中间某段），无缝精确恢复到离开时的 scrollTop 与悬浮“跳至最新”状态，绝不无故强制重置滚到底部；若此前处于粘底状态或首次打开，则通过 double-rAF 等待异步排版就绪后平滑跟随最新内容，彻底根治 Tab 切换时对话界面默认滚动回顶部的问题。
    - 动态运行指示器与任务流状态：动态运行胶囊与任务流状态统一采用「正在执行...」与「正在执行中…」文案。
    - 源码选区划词批注与结构化 XML 封装（`features/files/CodeView.vue`、`features/chat/composer/ComposerCommentCards.vue`）：源码预览查看器（`CodeView.vue`）支持划选多行文本在末尾行号上展示 `+` 按钮，点击展开平滑嵌入且宽度上限为 300px 的行内批注面板输入说明或提问（避免弹窗移出视口）；提取文件路径、起止行号、代码片段与批注内容并注入会话输入框内顶部卡片托盘（`ComposerCommentCards.vue`），卡片固定 150px 宽并按「文件、代码摘要、描述」紧凑呈现（其中代码摘要展示两行），多张卡片并排展示并在溢出时启用横向平滑滚动条，支持快速移除及点击卡片反查定位代码行；消息提交时采用标准 XML 标签封装（`<CodeComment path="..." line="...">` 包含 `<Description>` 与 `<Snippet>`），与自然用户正文合并；对话流渲染层（`FoldableUserText.vue` / `MessageView.vue`）自动解析 `<CodeComment>` 并呈现为与输入区一致的 150px 紧凑卡片托盘，代码片段与描述提供两行缩略并支持鼠标悬停浮层展示完整内容（无需额外点击展开），点击头部可联动工作区面板平滑滚动至对应行号。
    - 新建会话联动工作区面板（`features/chat/composer/NewSessionPanel.vue`）：采用与 `SessionChatPanel` 同构的双栏自适应布局，集成右侧统一工作区面板（`WorkspacePreviewPane`）；头部支持「单 Agent 会话 / 团队协同会话」模式切换：单 Agent 模式支持配置具体 Agent 后端与模型，协同会话模式支持选择团队群组（实时呈现中控模型与专家成员）；提交协同会话时调用 `POST /api/groups/:id/collab-sessions`，自动创建群组任务与会话索引，并自动触发首次协同运行；重新选择关联项目或工作树时，自动清空已打开的文件预览标签、重置激活项为文件树，并驱动底层文件树自动重新拉取；支持在新建会话阶段直接划选代码生成批注卡片，首条消息创建会话时一并打包传递给 Agent。
    - 协同会话全链路集成（`features/chat/SessionChatPanel.vue`）：当会话后端为 `group-flow` 时，主对话区无缝切换为协同群聊工作台（`CollabSessionChatView.vue`），右侧面板完全复用 `WorkspacePreviewPane`，确保协同会话与普通会话的交互操作、文件树浏览、Diff 审阅与尺寸调整体验 100% 保持一致。
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
[ 用户在左侧导航进入“终端”，或在工作区文件路径旁点击终端按钮唤出 800*500 悬浮终端窗口 ]
                         │
                         ▼
1. Web 前端 (features/terminal/XtermView.vue & ProjectTerminalModal.vue)
   - 初始化 xterm.js 实例并挂载 FitAddon
   - 滚动条自适应规则：覆盖注入 overflow-y: auto !important，未超出行数时不显滚动条，消除初始与 clear 后的突兀常驻，超出时展示精致细滚动条
   - 字体统一规范：全站终端字体默认与文件预览代码（CodeView 0.82rem，14px 基准下精准换算为 11.5px）完全对齐一致，支持用户在设置中自定义等宽字体并以平台统一字体作为默认与回退栈，切换时自动触发度量重校准与布局重排；
   - 键盘输入透传守护：绝不拦截 Escape 键关闭窗口，确保 Vim / Nano / 命令取消等快捷键原生直达 PTY
   - 浮动多终端并发与点击置顶：移除阻断性全屏黑色遮罩，窗口以 800*500 浮动展现，支持不同会话并发打开多个独立 Terminal；新窗口自动阶梯错开排布，各浮窗支持独立拖拽，点击任意窗口触发 bringToFront 自动提升 zIndex 置顶显示
   - 全局快捷栏集成：快捷工具栏（QuickAssistantLauncher）作为全局常驻容器，多终端最小化时分别缩小为对应的方形终端快捷按钮（>_），排在快捷助手按钮之后；若快捷助手未开启，终端按钮作为首项显示；点击立即还原并置顶；仅点击窗口关闭按钮（×）才真正退出终端并释放进程
   - 发起 WebSocket 连接: ws://host/api/terminal/ws/{serverId}?cols=80&rows=24&cwd={encodeURIComponent(path)}
                         │
                         ▼ (GET 请求带 Origin 与 Session Cookie)
2. 传输与同源防护 (internal/app/terminal/handlers.go)
   - 执行 checkSameOrigin 严格比对 Origin 与 Host 是否一致 (杜绝 CSWSH)
   - 校验 SessionCookie 鉴权
   - 解析并验证可选的初始工作目录 (cwd)，由 filepath.Clean 与目录有效性检查守护
   - 协议升级为 WebSocket 连接 (Upgrader)
                         │
                         ▼
3. 终端领域服务 (internal/app/terminal/service.go)
   - 本地模式：调用 core/platform 分配伪终端 (PTY)，优先读取用户配置的本地 Shell（支持自动探测的 Git-Bash、PowerShell 或自定义路径），若未配置则回退平台默认 Shell；若指定合法 cwd 则绑定 cmd.Dir，派生交互式 Shell 进程
   - 远程模式：解密 secretStore 凭据，建立 SSH Client Session
                         │
                         ▼
4. 双向全双工转发 Goroutines 与生命周期治理
   - Goroutine A：循环读取 PTY 字符流 ──► 写入 WebSocket ──► xterm.js 屏幕刷新
   - Goroutine B：循环读取 WebSocket 击键 ──► 写入 PTY 进程 stdin
   - 窗口伸缩：前端 ResizeObserver 捕获尺寸 ──► 发送二进制帧 ──► 调用 PTY.Resize()
   - 终端全局多窗口常驻保活（Persistent Project Terminal Modal）：Web 终端（ProjectTerminalModal）在主框架 AppShell 中全局挂载，支持任意页面随时唤出（快捷键 Alt+` 或工具栏按钮），支持多独立悬浮窗口、拖拽移动、缩放与多 Tab 切换。后台 PTY 与 SSH 会话在切换主视图时持续保持连接，不因页面路由改变而中断；关闭窗口或显式关闭 Tab 时安全销毁连接与清理资源。
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

### 5. 团队中心与多 Agent 协同流程工作树隔离调度流（Team Center & Flow Worktree Execution Flow）

平台将团队协同体系升级为「团队中心（Teams）」结合「项目协同会话（Collab Sessions）」：团队中心仅负责团队资产与专家角色定义，协同会话则是团队落地的唯一载体。实现了“定义团队成员库与中控大脑 → 在项目中创建协同会话并指定关联团队与 Git Worktree → 中控大脑带专属 Prompt 自主规划派发 → 专家 Agent 在物理隔离工作树中修改代码与自测”的全闭环：

```
[ 用户在团队中心配置专家成员与中控大脑设定，并在项目新建会话中发起协同会话（指定团队与 Git Worktree）]
                      │
                      ▼
1. Web 前端 (features/groups/GroupDetailView.vue / CollabSessionChatView.vue)
   - 展现为团队协同 IM 群聊对话流与专家工作台
   - 经由统一事件网关（channel="tasks", targetId=runId）多路复用订阅流式事件，不再独占独立长连接
                      │
                      ▼
2. 团队领域服务与中枢引擎 (internal/app/groups/ & internal/app/tasks/flow/engine.go)
   - 提取协同总目标、团队中控模型与专属中控设定（OrchestratorPrompt）
   - 初始化 FlowRun（携带 group_id 与 worktree_id）与首条用户目标消息
   - 启动基于 Function Calling 工具驱动的 ReAct 自主调度状态机
                      │
                      ▼
3. 内置中控大脑规划与工具决策 (Orchestrator Step / FlowTools)
   - 动态装配群组专家成员清单（角色名、职责、执行后端、模型、系统提示词及历史会话 ID）
   - 将群组定义的 OrchestratorPrompt 作为【群组中控大脑角色设定与专属规范】注入系统上下文
   - 调用中控模型流式生成分析与规划，经 EventBus 广播打字机动画
   - 自主决策并发起标准 Function Calls 工具调用：
       • flow_dispatch_role: 指派角色执行（支持新建会话或传入 session_id 复用会话多轮连续对话）
       • flow_wait_session:  显式等待角色会话执行完毕并汇总结果
       • flow_ask_user:      遇到重大疑问或需人工确认时向群聊提问并挂起
       • flow_complete_task: 所有子任务合格且目标达成时提交最终报告
                      │
                      ├─────────────────────────────────────────┐
                      ▼                                         ▼
         【A. 纯推理型专家 (llm)】                 【B. 工作区 Coding Agent 角色】
         - 依赖 chat.Completer                     - 依赖 sessions.CreateInWorktree(projectID, worktreeID)
         - 携带角色设定与当前任务指令             - 在专属 Git 工作树目录中启动/复用 Agent 实例
         - 流式补全输出分析或设计方案             - 物理隔离代码变动，不污染主开发分支
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

#### 5.1 群组任务融入项目会话与协同会话自动执行流（Collab Session Workflow）

平台支持在项目会话中直接创建与运行多 Agent 团队协同会话，将高阶中枢群聊与普通会话工作台同构融合，同时杜绝底层专家子会话对会话树的视觉污染：

```
[ 用户在新建会话面板切换为“团队协同会话”并选择群组，输入总目标 ]
                               │
                               ▼
1. Web 前端 (features/chat/composer/NewSessionPanel.vue)
   - 提取输入任务描述与选中的群组 ID (groupId)
   - 调用 api/groups.ts 的 createCollabSession(groupId, { projectId, worktreeId, title, prompt })
                               │
                               ▼ (POST /api/groups/:id/collab-sessions)
2. 传输层与群组领域服务 (internal/app/groups/service.go)
   - 校验群组成员与中控配置完整性
   - 自动创建 GroupTask 实体 (type="flow", status="active", 绑定项目与工作树)
   - 在 agent_sessions 登记平台中立索引行：
       backend = "group-flow", instanceId = "group-<groupId>", native_session_id = "gt-<taskId>"
   - 立即调用 flowController.StartGroupRun 自动触发首次协同运行
   - 返回统一的 AgentSession 与 Task 实体
                               │
                               ▼
3. 前端会话 Tab 打开与同构渲染 (SessionChatPanel.vue)
   - 自动跳转并原地置换为协同会话 Tab（`openTabs.promoteNewSession` 携带 title 与 `backend: 'group-flow'`，同时将新会话同步注入 `sessionsStore` 本地索引缓存）
   - 检测到 `session.backend === 'group-flow'` 时，绕过单 Agent 专属的 `openSession` 与单 Agent 历史请求，消除 `unsupported_capability` 报错与 `disconnected` 误标
   - 主对话区同构渲染 `CollabSessionChatView.vue`，右侧工作区无缝复用 `WorkspacePreviewPane.vue`，保持左右双栏布局、文件树与代码预览体验完全一致
   - 顶部呈现中控大脑与专家成员芯片，实时监听任务事件网关 (`channel="tasks"`, `targetId=runId`，字段对齐 `envelope.eventType` 与增量 `chunk`)，断线重连（`onReconnect`）自动补齐增量消息
                               │
                               ▼
4. 底层专家会话静默执行、防挂起与隔离防污染 (internal/app/tasks/flow/)
   - 中控大脑通过 `flow_dispatch_role` 派发的 Coding Agent 专家会话在专属工作树中执行
   - **自动化子会话免挂起机制（Zero-Hanging Execution）**：
     • Prompt 级注入协同执行规范：严禁底层 Agent 调用提问类工具（如 `question`、`ask_user_question`）挂起等待，要求根据常识与工程规范自主决策推进，并在总结报告中呈现疑问；
     • 遥测总线双层自动解挂（`InteractionAutoReplier`）：`pipeLiveStream` 在实时流收到 `EventInteraction (InteractionRequested)` 时立即自动应答表单并批准权限；`CollectSessionTurn` 轮询中扫描挂起表单与权限并自动应答，杜绝子 Agent 进程卡死在无人交互界面上
   - `tasks.GetTaskSessionIDs` 自动联合查询所有协同流派生的底层 session_id
   - 左侧项目会话树默认过滤并隐藏这些专家子会话，防止会话列表爆炸
   - 用户如需审查专家执行细节，可在 CollabSessionChatView 中点击“查看专家会话”在模态窗中直接下钻检视
```

---

### 6. 平台级 Agent 诊断与模型快照流（Diagnostics & Model Snapshot）

- **诊断归属**：`GET /api/agent-backends/diagnostics` 是平台级接口。前端只在「平台设置 → Agent」中展示：每张 Agent 卡片内提供可折叠诊断详情，`warning` / `error` 默认展开。项目设置不再读取诊断，也不再展示 machine service 提示。
- **模型持久化与治理**：`internal/app/agentmodels` 管理平台级模型快照与治理。`agent_models` 保存每个 backend 的模型行（含 `enabled` 开关、显示名称、上下文上限等个性化配置，并以 `name_customized` 区分用户自定义名称与后端同步名称），`agent_model_catalogs` 保存最后同步时间；唯一索引为 `(backend, model_id)`。
- **读取**：`GET /api/agent-backends/{backend}/models` 只读数据库，不调用 Agent；Agent 离线时仍返回上次成功快照。从未刷新返回 `models: []` 与 `updatedAt: null`。
- **手动刷新与增量合并**：`POST /api/agent-backends/{backend}/models/refresh` 调用适配器 `ListModels(ctx, "")`。刷新采用**增量差量合并（Upsert/Merge）算法**：保留用户已配置的启用/停用状态、用户自定义名称与上下文限制；未被用户自定义的名称会跟随后端最新名称更新（pi 无实例时通过短生命周期 RPC 读取带显示名称的目录），补充/更新后端新模型或属性，清理已废弃模型。刷新失败保留旧快照；空结果清空模型但更新同步时间。
- **模型配置修改与启用/停用**：`PATCH /api/agent-backends/{backend}/models`（支持带 `/*model` 或请求体指定 `modelId`）支持更新指定模型的启用状态、自定义名称、上下文上限 Token 限制及多模态图片输入支持。`PUT /api/agent-backends/{backend}/models/enabled` 提供全量启用/清空，供 Agent 设置页一键全选/清空使用。前端 Agent 设置页提供按名称搜索、已启用优先排序、快捷启停 Checkbox 与模型编辑表单。
- **下游消费端过滤**：工作台会话模型选择器（`ModelSelect.vue`）与计划任务创建器（`TaskManageDialog.vue`）自动过滤 `enabled: false` 的模型，只向终端用户展示已启用的模型；若历史选中的模型被停用，选择器平滑回退至首个可用模型。
- **刷新失败可观测性**：HTTP 响应只给稳定错误码（`model_list_failed` / `service_not_running` / `unsupported_capability`），适配器返回的底层原因（例如 deepseek 共享宿主启动超时）在服务端日志以 `agent models: refresh <backend> failed: ...` 记录并做密钥脱敏；排障时先看这条日志，不要只看 access log 的 502。
- **deepseek 宿主启动预算**：deepseek 的刷新会懒启动共享 `dsh` 宿主，启动超时默认 30s（`dsh` 自身打印 launch URL 实测 4.9~5.3s，因此不可下调到秒级；可用 `DEEPSEEK_STARTUP_TIMEOUT` 覆盖）。参见 `docs/agent-compatibility.md` 的 deepseek transport 小节。
- **刷新策略**：无 TTL、无后台刷新、无轮询、无定时任务。只有运维员在 Agent 设置页点击“刷新”才会同步。空目录表示平台级/后端默认作用域，不考虑项目目录级模型配置。
- **边界**：Agent 模型属于各 Agent 后端的模型字典，支持平台级启停与配置覆盖；平台「模型」设置中的 OpenAI 兼容模型服务保持独立，服务于纯 Chat 对话。
- **Agent 专属控制**：内置 Agent（`builtin`）支持在平台设置中动态配置单轮最大工具调用步数（`PUT /api/agents/{backend}/max-turn-steps`），Descriptor 默认 30 步，支持自定义步数或配置 `-1` 解除次数限制（无限制循环）。支持动态热更新并在下一轮对话中即时生效。
  - **交互调试观测与落盘导出**：内置 Agent 支持在设置中动态开启/关闭调试模式（`PUT /api/agents/{backend}/debug-mode`）。开启后，内置 Agent 将每一轮对话中与大模型（包含完整 System Prompt、历史上下文 Messages、Tools Schemas 声明、思考链 Reasoning、模型终态回复及耗时）以及与各类工具（包含入参 JSON、权限判定结果、执行原始 Stdout/Stderr 输出、错误状态及执行耗时）的交互详情以原子追加写（JSONL 格式）落盘至 `<dataDir>/debug_logs/session-<sessionId>.jsonl`。前端会话头部提供「导出调试」入口，通过单独的 HTTP 文件下载接口（`GET /api/projects/{id}/sessions/{sessionId}/debug-export`）安全流式下载，不与现有的会话 SSE 复用，不干扰主对话流性能与信噪比；契约层定义 `agents.DebugLogExporter` 可选扩展接口，通用层零后端字面量，非内置 Agent 会话调用统一返回 409 `unsupported_capability`；删除会话时自动级联清理关联的磁盘调试文件。

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
   - 用户切换目标连接时：主进程先硬切断经透明代理建立的全部 SSE 与 WebSocket 管道连接
     （DesktopProxy.closeAllConnections()，逐请求重解析的 getTarget() 只影响新连接，
     既有长连接 必须 destroy 以免旧环境事件串流），渲染进程随后调用 unifiedEventBus.resetBusId()
     清空持久化 busId 与订阅状态，同时原子化重置前端活动 Tab、清空 CSRF/Auth Store，
     各页面在新环境自愈重连并带序号重订重放，平滑过渡至目标服务环境，保证状态绝对物理隔离。
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
  7. **废弃旧路由与私有流（零兼容包袱）**：
     已彻底废除并物理移除原各自烟囱式构建的私有总线、单点长连接端点与死代码：
     - 原会话单点事件流与总线：`GET /projects/:id/sessions/:sessionId/events`（及 `StreamSession`）、`/api/events/:uuid`、`/api/events/:uuid/subscriptions`
     - 原纯对话单点事件流与私有总线：`GET /chats/:id/events`（及 `StreamChat`）、`internal/app/chat/event_bus.go`、`/api/chat-events/:uuid`、`/api/chat-events/:uuid/subscriptions`
     - 原协同流程任务端点：`/api/tasks/flows/:runId/events`
     - 前端死代码：彻底清理废弃的 `sessionEventsPath` 与 `chatEventsPath`，长连接推送完全收敛至单长连接网关 `/api/events/bus/:uuid`

---

### 12. Git Worktree 并行开发与工作区作用域流（Git Worktree Flow）

```
 [ 项目设置 Tab：ProjectSettingsView ]
                 │
                 ▼
 1. 项目设置 (internal/app/projects/worktree.go)
    - GET  /api/projects/:id/worktrees
    - POST /api/projects/:id/worktrees  { branch?, baseBranch?, commit?, path? }
    - DELETE /api/projects/:id/worktrees/:worktreeId
    - 默认目录：<项目父目录>/<仓库名>-worktrees/<branch-slug>
    - 创建：git worktree add -b <branch> <path> <base/commit>（支持基于分支或通过 Commit ID 创建，基于 Commit ID 时若留空分支名自动派生 wt-<shortHash>）
    - 移除：先校验脏工作区 → StopProjectWorktreeInstances → git worktree remove
    - 分支保留，工作树记录删除
                 │
                 ▼
 2. 会话创建 (POST /api/projects/:id/sessions, worktreeId 可选)
    - sessions.CreateInWorktree
    - instanceForBackend 按 (projectId, backend, worktreeId) 精确复用实例
    - 无实例时 instances.StartInWorktree → 经
      ProjectDirectoryResolver.ResolveWorktreeDirectory 取得规范化目录
    - agent_instances.worktree_id / agent_sessions.worktree_id 同时落库
                 │
                 ▼
 3. 会话内文件与 Git 面板
    - 所有文件/Git/预览接口接受可选 ?worktreeId=
    - projects.workspaceRoot 统一解析作用域：0 = 主工作区，非 0 = 工作树
    - 预览 Ticket 保存 projectId + worktreeId，跨作用域无法复用
    - 分支切换成功后同步 project_worktrees.branch
                 │
                 ▼
 4. 前端状态
    - stores/worktrees.ts 按项目缓存工作树，创建/移除同步刷新
    - NewSessionPanel 仅对 Git 项目展示“工作树”选择，草稿保存 worktreeId
    - SessionChatPanel 以 activeSession.worktreeId 作为文件/Git 面板作用域
    - ProjectTree / ProjectSessionItem 展示工作树分支标签
```

**边界与语义**：
- 工作树只能在 Git 仓库根目录创建；项目子目录不算仓库根。
- 自定义目录必须是绝对路径、位于项目工作区之外、目标不存在。
- 移除工作树会停止其运行实例；历史会话仍保留索引，但续接会返回 `worktree_unavailable`。
- 项目解除登记只删除平台记录与工作树记录，磁盘上的项目目录与工作树目录均保留。

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
   - 前端守卫测试 `web/src/tests/csrf-coverage.spec.ts` 会在源码静态分析层面拦截违规调用；后端 `csrf_matrix_guard_test.go` 遍历所有非安全路由测试防御有效性；
   - 测试夹具 `router_fixture_test.go` 具备反射断言，确保全平台所有微服务依赖注入完整；`openapi_guard_test.go` 严格核对全量 REST 路由与 `docs/api/openapi.yaml` 契约定义的一致性；
   - 专家角色（Roles）与提示词模板（Prompts）具备级联引用完整性保护：删除前自动校验其是否被群组（group_members）、角色（agent_roles）或对话（chat_groups/chats）引用，被引用时返回 409 Conflict（role_in_use / prompt_in_use）并阻断物理删除。
3. **HTTP 响应压缩**：
   - 中间件链为 `AccessLog → Compress(gzip) → Recovery → RequireCSRF → ...`；`Compress` 位于 `internal/transport/http/compress.go`，对协商了 gzip 的可压缩响应启用压缩；
   - 压缩成功时设置 `Content-Encoding: gzip`、`Vary: Accept-Encoding`，并删除 `Content-Length` 与 `Accept-Ranges`；已知 `Content-Length < 1KB` 的响应跳过；
   - SSE（`text/event-stream`）、WebSocket Upgrade、Range/206、204/304、已有 `Content-Encoding`、带 ETag 或 `Cache-Control: no-transform` 的响应一律跳过；
   - 包装器实现 `Unwrap()`，保证 SSE handler 的 `http.NewResponseController(c.Writer).SetWriteDeadline` 仍能触达底层连接。
4. **请求体上限与大文件上传**：
   - 通用管理接口应用 `limitRequestBody(1 MiB)` 防御非法大包；
   - 对话与会话接口因支持图片附件（最多 4 张 5 MiB 图片 base64 展开）配置为 32 MiB；
   - 工作区文件上传接口（`POST /api/projects/:id/files/upload`）挂载于独立上传组，放宽至 1 GiB（`maxFileUploadRequestBodyBytes`），满足大型归档、模型权重、二进制包等文件的上传需要；
   - 前端 `client.ts` 的 `mutate()` 支持 `onUploadProgress` 回调，底层通过 `XMLHttpRequest` 监听 `upload.onprogress`，在 `FileSidebar.vue` 中无缝驱动 `UiProgress` 进度条、已传大小显示及滚动文件清单，上传错误独立展示不摧毁文件树。

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
