# dev_platform 开发约定

## 本项目分层（claudian 五层同构）

项目已按 claudian 的分层架构组织（对照 `docs/architecture/backend-boundaries.md` 与 `docs/architecture/claudian-refactor-plan.md`）：

```
① 组合根   cmd/platform/（Go）+ web/src/{main.ts,App.vue,router.ts}（Vue）
④ 契约     internal/core/agents（agent 契约/注册表/事件）、internal/core/platform（OS 抽象）
③ 领域服务 internal/app/{sessions,instances,projects,settings,auth,chat,...}
⑤ 异构适配 internal/providers/{opencode,pi}（实现 core 契约，导出 Descriptor）
传输编排   internal/transport/http（路由/静态文件/accesslog）
② 前端     web/src/features/{chat,workspace,projects,settings}、web/src/shared/
```

- 禁止：`core → providers`、`app/features → providers`、通用层出现后端 ID 字面量（守卫测试执行）。
- 前端组件按 feature 归位；provider 中性的可复用控件与工具放 `web/src/shared/`；chat 的执行机制在
  `features/chat/execution/`、状态模型在 `features/chat/state/`、渲染模型在 `features/chat/rendering/`。
- **前端 API 调用只能经 `web/src/api/client.ts` 的 `get()` / `mutate()`**：读用 `get`，写用 `mutate`；
  不得直接 `fetch(`、不得手写 `X-CSRF-Token`、不得把 `csrfToken` 作为函数参数透传（token 由传输层
  从 auth store 注入并自动刷新，见 `docs/adr/0001-csrf-in-the-transport.md`）。`web/src/tests/csrf-coverage.spec.ts`
  会在源码层拦截上述写法，`internal/transport/http/csrf_matrix_guard_test.go` 会遍历全部非安全路由校验 CSRF。
- 新增 agent 后端步骤：契约包加 `BackendID` → `internal/providers/<name>` 实现 `AgentAdapter` 并导出
  `Descriptor` → 组合根注册 → 更新 `docs/agent-compatibility.md`。前端零改动。

## 参照源码（重要）

开发本项目时经常需要参照 opencode、pi 与 claudian 的实现。三个源码仓库位于本机，路径如下：

| 项目 | 源码路径 | 说明 |
|---|---|---|
| **opencode** | `/data/projects/opencode` | 一个 AI coding agent（终端/桌面/Web 多端）。当前为 **V2 重构版**（版本 `2.0.27`），代码拆成多个 `@opencode/*` 包（`schema / protocol / core / server / client / sdk / cli / tui / app` …），核心逻辑在 `packages/core`。 |
| **pi** | `/data/projects/pi` | 一个**极简、可扩展的 agent harness**，核心产品是终端 AI coding agent（作者团队 Earendil Works）。子包 `@earendil-works/*`（`coding-agent / agent / ai / tui / durable / chord / codemode / mcp / env / telemetry / evals`）。core 保持极简，功能尽量放到 extensions/skills/prompt templates/theme 中。 |
| **claudian** | `/data/projects/claudian` | 一个 **Obsidian 桌面插件**（版本 `2.3.21`，id `realclaudian`）：把多种 AI coding agent（Claude Code、Codex、Grok Build、OpenCode、Pi、DeepSeek）嵌入 vault，以 vault 作为工作目录。特点是**多 provider 适配器 + provider 中性契约分层**（`providers/ → core/ → app/ → features/ → composition/`），可作为多 agent 集成/会话管理/流式渲染的借鉴。 |

## 分析结果（优先使用，避免重复读源码）

为便于后续直接复用已有分析、减少重复读取源码，已把三个项目的源码结构分析整理成独立文档：

| 文档 | 内容 |
|---|---|
| [`opencode_source.md`](./opencode_source.md) | opencode 源码架构概览：顶层目录、包结构、核心运行时（session/tool/agent/provider）、关键入口、配置 schema、快速定位速查表。 |
| [`pi_source.md`](./pi_source.md) | pi 源码架构概览：项目定位、包结构、核心运行时（agent loop、session、provider、tools、扩展系统）、关键入口、构建测试、快速定位速查表。 |
| [`claudian_source.md`](./claudian_source.md) | claudian 源码架构概览：项目定位（Obsidian 插件）、分层架构约束（providers/core/app/features/composition）、多 provider 适配器、会话管理/执行协调、关键入口、构建测试、快速定位速查表。 |

**使用顺序：**
1. 需要参照 opencode、pi 或 claudian 的实现时，**先读对应的 `*_source.md`**（含路径与速查表，通常足够直接定位）。
2. 只有当文档信息不足或需要确认实现细节时，才进入 `/data/projects/opencode`、`/data/projects/pi` 或 `/data/projects/claudian` 源码继续读取分析。
3. 若从源码获得了新的重要结论（路径变更、新增模块、设计澄清等），**请同步更新对应的 `*_source.md`**，保持分析结果长期可用。

## 通用偏好

- 与用户交互始终使用简体中文：回复、计划、总结、审查说明等一律用中文。
- 代码、命令、路径、标识符、报错原文保持原样，不翻译。
