# dev_platform 项目架构与技术规范

本文档是 `dev_platform` 项目的权威整体架构与实现设计描述。后续开发人员及 AI 助手在理解项目、新增需求或进行架构重构时，**必须以本文档为准**。当需求发生变化、新增业务功能或发生架构调整时，**必须同步更新维护本文档**。

---

## 一、项目定位与核心能力

`dev_platform` 是一个面向团队和个人开发者的**多 Agent 协同 AI 研发工作台**。其核心目标在于解决不同终端 AI Coding Agent（如 OpenCode V2、Pi、DeepSeek CLI）协议各异、交互割裂、缺乏集中管控的问题，提供中性同构的平台层封装：

1. **多后端统一协同**：统一接入异构的 AI Coding Agent，支持开箱即用的多 Agent 实例隔离、生命周期管理与并发会话。
2. **实时交互工作台**：统一的 Web 端会话界面，支持类型化轮次追踪、流式响应（SSE）、代码 Diff 预览、富文本 Markdown 清洗与 PlantUML 架构图渲染。
3. **闭环人机确认**：提供权限确认（Permission）与结构化表单（Form）的人机交互流，杜绝客户端轮询，所有交互直接挂接在会话标准事件流中。
4. **全功能研发辅助**：集成基于 PTY/WebSocket 的跨平台在线终端、支持 Cron 表达式的定时任务调度、提示词（Prompts）模板管理，以及独立于 Agent 会话的通用大模型纯 Chat 对话。

---

## 二、技术栈全景

| 层次 | 核心技术 / 依赖 | 架构职责与说明 |
|---|---|---|
| **后端语言** | Go 1.24.0 | 静态类型、跨平台编译（Linux / Windows / macOS） |
| **HTTP 与网关** | Gin v1.11.0 | RESTful 路由、SSE（Server-Sent Events）长连接多路广播 |
| **实时通信** | Gorilla WebSocket v1.5.3 | 终端 PTY 双向字符流转发（带严密 Origin 同源校验） |
| **数据持久化** | GORM v1.25.7 + SQLite (`glebarez/sqlite`) | 纯 Go 实现的嵌入式数据库（无需 CGO），自建迁移版本链（v1~v18） |
| **系统与进程** | `creack/pty`、`golang.org/x/sys`、`crypto` | 跨平台子进程生命周期管理、ConPTY/PTY 分配、安全密码哈希 |
| **调度引擎** | `robfig/cron/v3` | 支持标准 5/6 段 Cron 表达式及单次定时调度 |
| **前端工程** | Vue 3.5 (SFC, `<script setup>`), Vite 8.3, TypeScript 7.0 | 单页面应用（SPA），组合式 API，严格类型安全 |
| **前端状态与路由** | Pinia 4.0, Vue Router 5.4 | 领域驱动的 Store 分层，支持草稿跨会话暂存 |
| **前端展现渲染** | xterm.js 5.3, Marked 18.1, DOMPurify 3.4, `@plantuml/core` | 终端仿真、安全 Markdown 流式渲染、PlantUML 在线绘图 |
| **测试与质量门禁** | Go Testing + AST 静态扫描守卫; Vitest 5.0 (622 单测通过); Playwright E2E | 源码级架构分层守卫、CSRF 防绕过拦截矩阵、全流程端到端自动化 |

---

## 三、分层架构体系（Claudian 五层同构规范）

项目严格对齐 Claudian 的五层架构体系，将“与具体 Agent 无关的通用同构层”和“必须因后端而异的异构层”彻底解耦：

```
① 组合根     cmd/platform/（Go 程序入口）+ web/src/{main.ts, App.vue, router.ts}（Vue 挂载）
④ 契约层     internal/core/agents（标准协议/事件/注册表）、internal/core/platform（OS 抽象）
③ 领域服务   internal/app/{sessions, instances, projects, chat, tasks, terminal, settings, auth, ...}
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

## 四、核心业务流程与运行时机制

### 1. 会话与轮次生命周期（Turn Lifecycle）
- 会话忙碌与流转状态**唯一来源于类型化轮次 `EventTurn`**（`started` / `ended`，结果为 `completed` / `cancelled` / `failed`）。
- 消息发送流程：
  `POST /api/projects/{id}/sessions/{sessionId}/messages` ──► `sessions.Service.Send` ──► 派发给后端 `AgentAdapter` ──► 适配器将原生生命周期事件翻译为标准 `EventTurn` ──► 多路复用事件总线 `event_bus.go` ──► 前端 `SessionEventStream.ts` 消费并更新 UI。
- **会话级 Stop**：仅中断当前会话的轮次（调用适配器的 `Interrupt`），绝不影响项目实例或其他并发会话。

### 2. 交互授权流（Interactions Flow）
- 工具执行审批（Permission）与表单输入（Form）直接通过会话事件流（`interactions` 快照帧与 `EventInteraction` 事件）实时推送给前端。
- 彻底废除前端轮询与侧信道流，前端处理后通过统一的 `mutate()` 接口应答适配器，恢复执行。

### 3. 终端与远程 Shell（Terminal Service）
- 基于 WebSocket 协议与后端建立全双工管道，对接本地 PTY（Linux 下使用 PTY，Windows 下对接 ConPTY）或远程 SSH。
- **安全防线**：在握手阶段强制执行严格的 RFC 6455 同源 Origin 校验（Host 严格比对），防范跨站 WebSocket 劫持（CSWSH / RCE）。

### 4. 启动期状态自愈机制（Startup Reconciler）
- 当服务因异常断电或强杀（SIGKILL）重启后，数据库迁移完成后会自动运行 `ReconcileOrphans`：
  - 将数据库中处于 `running` / `starting` 的孤儿 Agent 实例自动标记为 `stopped`；
  - 将处于 `running` 状态的孤儿定时任务执行记录自动标记为 `interrupted`；
  - 杜绝前端页面出现假死或永久运行的异常脏状态。

---

## 五、API 传输层与安全规范（ADR-0001）

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

## 六、新增 Agent 后端标准步骤

新增一个 Agent 后端无需改动任何前端代码，仅需以下 6 步：
1. 在 `internal/core/agents/contracts.go` 中为 `BackendID` 增加常量定义，并添加到 `BackendID.Valid()` 白名单中。
2. 新建适配器包 `internal/providers/<name>`，实现 `agents.AgentAdapter`（包含显式声明的 `Capabilities()`）。
3. 按需实现可选扩展接口：`PermissionReplier`、`HistoryReader`、`ServiceController`、`RawEventOpener`。
4. 在该适配器包导出 `Descriptor`（声明可执行名、色调/品牌色、展示顺序等），并编写 `descriptor_test.go`。
5. 在 `cmd/platform/main.go` 中注册适配器并加入 Catalog；启动时 `checkBackendWiring` 会自动校验注册表与 Catalog 的一致性。
6. 更新 `docs/agent-compatibility.md` 能力兼容矩阵。

---

## 七、测试体系与验证规范

每次代码变更后，必须确保以下三道质量门禁全部通过：

```bash
# 1. 运行核心集成检验（包含 vet、Go 全量单测、前端 Vitest 单测、前端类型检查、前端生产构建）
make verify

# 2. 运行端到端集成测试（Playwright E2E，全量套件）
make e2e
```

- **E2E 运行环境规范**：
  E2E 测试由 `web/scripts/run-e2e.mjs` 编排，默认使用 `E2E_VITE_PORT=15173` 独立端口，避免与开发者本机 5173 端口冲突；测试桩（`web/e2e/fixtures/bin/`）完整支持各后端的最新协议模式（如 Pi 的 `--mode rpc`）。

---

## 八、文档演进与维护约定（重要）

1. **同步更新约束**：
   后续当有新的功能需求、字段变动、安全策略或分层边界调整时，**必须在修改代码的同时同步更新维护本 `PROJECT.md` 文件**，严禁代码与架构文档脱节。
2. **参照源码分析同步**：
   若在开发过程中深入查阅了本机 `/data/projects/opencode`、`/data/projects/pi` 或 `/data/projects/claudian` 源码并获得了重要结论，需同步更新根目录下的 `opencode_source.md`、`pi_source.md` 或 `claudian_source.md`，保证团队沉淀长期有效。
