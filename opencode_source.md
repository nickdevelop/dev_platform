# opencode 源码分析（架构概览）

> 源码路径：`/data/projects/opencode`
> 版本：`2.0.27`（V2 重构版）
> 用途：开发时需要参照 opencode 实现时，先读本文档定位；不足时再进入源码细读。

## 0. 一句话定位

opencode 是一个 AI coding agent（终端/桌面/Web 多端）。该仓库是 **V2 重构版**：不是单体包，而是把运行时拆成 `schema / protocol / core / server / client / sdk / cli / tui / app` 等多个 `@opencode/*` 包，核心逻辑集中在 `packages/core`。

## 1. 顶层目录结构

| 路径 | 职责 |
|---|---|
| `packages/` | monorepo 主体，所有运行时包与 UI 包（见第 3 节） |
| `services/` | 独立部署的 Cloudflare Worker：`www/`（官网+文档站，含 `openapi.json`）、`update/`（更新元数据）、`files/`（文件分发） |
| `sdks/` | 对外 SDK / 编辑器插件（目前 `sdks/vscode/`） |
| `specs/v2/` | V2 跨模块语义契约文档 |
| `infra/` | SST 基础设施即代码（Cloudflare/AWS 资源） |
| `github/` | GitHub Action（`/opencode` 评论触发） |
| `nix/` | Nix 打包表达式 |
| `script/` | 仓库级脚本（release/changelog/lint/ast-grep 规则等） |
| `perf/` | 性能测试文档 `perf/test-suite.md` |
| `patches/` | Bun patch 补丁 |
| `install` | 独立 shell 安装脚本 |
| `AGENTS.md` | 贡献者核心规约（V2 语义、架构方向、代码风格）——**参照实现必读** |
| `V2_HTTP_API_AUDIT.md` | V2 HTTP API 审查清单（139 端点逐条裁决） |
| `turbo.json` / `sst.config.ts` / `flake.nix` | 任务编排 / SST 入口 / Nix flake |

## 2. Monorepo 与包管理

- 包管理器：**Bun**（`packageManager: bun@1.4.2`，`bun.lock`）。
- Workspaces：`packages/*`、`services/*`、`packages/console/*`、`packages/stats/*`；用 `catalog:` 统一锁定依赖版本。
- 任务编排：**Turborepo**（`turbo.json`）。`@opencode/cli|server|sdk|tui|enterprise|simulation` 的 typecheck 依赖 `@opencode/core#typecheck`。
- 包间依赖用 `workspace:*`。
- **架构方向（强约束，见根 `AGENTS.md`）**：`Schema → Core/Protocol → Server` 单向依赖；Client 只能依赖 Schema/Protocol，不能依赖 Core/Server；`sdk` 组合 Client+Core+Server。
- 根脚本：`bun run check`（lint+typecheck 权威校验）、`dev`（`bun run --cwd packages/cli src/index.ts`）、`dev:live`。
- **测试不能从仓库根跑**（根 `test` 脚本显式 `exit 1`），需进入各包目录运行。

## 3. packages/ 子包（约 31 个 `@opencode/*`）

**运行时核心层**
| 目录 | 包名 | 用途 |
|---|---|---|
| `packages/schema/` | `@opencode/schema` | 公共领域形状与持久事件载荷（Effect Schema），唯一真源 |
| `packages/protocol/` | `@opencode/protocol` | HTTP 操作定义与传输错误，组装 `HttpApi` 与 OpenAPI |
| `packages/core/` | `@opencode/core` | 运行时行为与持久化（session/tool/agent/provider/mcp/db…） |
| `packages/server/` | `@opencode/server` | 组装 Core+Protocol 为 HTTP 服务 |
| `packages/client/` | `@opencode/client` | 生成的 HTTP 客户端（Promise/Effect/Solid 三套） |
| `packages/sdk/` | `@opencode/sdk` | 进程内宿主（内存中跑 Server 路由，不监听端口） |
| `packages/ai/` | `@opencode/ai` | LLM/provider SDK 抽象、协议实现、模型能力 |
| `packages/plugin/` | `@opencode/plugin` | 插件作者接口与运行时加载（Promise/Effect/TUI 三套） |
| `packages/util/` | `@opencode/util` | 通用工具（fs、路径、effect 辅助、npm、observability） |

**CLI / 界面层**
| 目录 | 包名 | 用途 |
|---|---|---|
| `packages/cli/` | `@opencode/cli` | CLI 入口 + 子命令 + 后台服务管理 |
| `packages/tui/` | `@opencode/tui` | 终端 UI（SolidJS + `@opentui/*`） |
| `packages/app/` | `@opencode/app` | Web GUI（SolidJS + Vite） |
| `packages/desktop/` | `@opencode/desktop` | Electron 桌面应用 |
| `packages/enterprise/` | `@opencode/enterprise` | 企业版界面（SolidStart） |
| `packages/gui-extensions/` | `@opencode/gui-extensions` | 桌面/Web GUI 扩展（terminal/review/browser/ssh/wsl/updater…） |
| `packages/session-ui/`、`packages/ui/` | `@opencode/session-ui`、`@opencode/ui` | 共享 UI 组件 |
| `packages/theme/` | `@opencode/theme` | 主题 token 与 schema |
| `packages/storybook/` | `@opencode/storybook` | 组件故事书 |

**渲染 / 内容辅助**：`codemode/`（受限代码执行解释器）、`latex/`、`merman/`（公式/Mermaid 渲染）、`posts/`、`web/`（Astro/Starlight 文档站）。

**测试 / 工程 / 基础设施**：`simulation/`（确定性模拟后端）、`http-recorder/`（录制回放 HTTP/WS）、`httpapi-codegen/`、`function/`（CF Worker）、`script/`、`identity/`、`containers/`（多套 Dockerfile）、`packages/console/*`、`packages/stats/*`、`effect-drizzle-sqlite/`。

## 4. 核心运行时（`packages/core/src/`）

`@opencode/core` 是关键；`exports` 为 `"./*": "./src/*.ts"`（任意子路径可直接 import）。

顶层关键文件：`bus.ts`（事件总线，durable 事件）、`session.ts`（Session 门面）、`agent.ts`、`tool.ts` + `tool-output.ts`、`provider.ts` + `model.ts` + `model-resolver.ts`、`aisdk.ts`/`aisdk-native.ts`、`config.ts` + `config/`、`mcp/`、`permission.ts` + `permission/`、`plugin.ts` + `plugin/`、`project.ts`/`worktree.ts`/`workspace.ts`/`vcs.ts`/`git.ts`、`instance.ts`/`location*.ts`、`shell.ts`+`shell/`、`pty.ts`+`pty/`、`filesystem.ts`+`filesystem/`、`database/`（Drizzle + Effect SQLite）、`effect/`、`environment/`、`v1/`（旧配置迁移）。

**Session 子系统 `session/`**：`session.ts`（ID 绑定操作）、`execution.ts`+`execution/restart.ts`（执行协调）、`run-coordinator.ts`（同会话 resume 合并、跨会话并发）、`inbox.ts`（prompt 准入/steer/queue）、`projector.ts`（事件投影落库）、`store.ts`（Drizzle 查询层）、`history.ts`、`compaction.ts`（上下文压缩）、`prompt.ts`/`revert.ts`/`diff.ts`/`transfer.ts`/`title.ts`/`usage.ts`/`move.ts`、`runner/`（**核心执行循环**）、`subagent-job.ts`/`subagent-completion.ts`。

`session/runner/`：`index.ts`（`SessionRunner.drain`）、`step.ts`（单步 LLM 调用 + 工具执行）、`model.ts`（模型解析）、`llm.ts`、`retry.ts`、`max-steps.ts`、`to-llm-message.ts`、`publish-llm-event.ts`、`prompt/system.txt`。

**Tool 子系统 `tool/`**：`tool.ts`（`Tool` 服务）+ `tool/runtime.ts`（definition/execute/名称规范化）；`tool/plugin/` 为**内置工具实现**：`read.ts`、`write.ts`、`edit.ts`、`patch.ts`、`glob.ts`、`grep.ts`、`shell.ts`、`webfetch.ts`、`websearch.ts`、`skill.ts`、`subagent.ts`、`question.ts`、`mcp-resource.ts`、`opencode.ts`、`file-diff.ts`。内置工具注册集中在 `plugin/internal.ts`。

**Plugin 子系统 `plugin/`**：`host.ts`、`supervisor.ts`、`internal.ts`（内置插件集合）、`module.ts`、`instance.ts`、`service.ts`、`sdk.ts`；`plugin/provider/` 为 **provider 适配器**（约 37 个文件：openai/opencode/github-copilot/gitlab/azure/amazon-bedrock/google-vertex/ollama/lmstudio/xai/openrouter/cloudflare-*/factory/dynamic/local…）；还有 `agent.ts`、`plan.ts`（plan agent）、`command.ts`、`skill.ts`、`system-prompt/`。配置→插件转译在 `config/plugin/`。

**关于 LSP**：仓库内**没有 core 级 LSP 运行时实现**，仅有配置 schema `schema/src/config/lsp.ts`、事件 `schema/src/lsp-event.ts`、归一化处理 `core/src/config/normalize.ts`。不要假设存在 `lsp.ts` 实现。

## 5. 关键入口点

| 入口 | 路径 |
|---|---|
| CLI 打包 bin | `packages/cli/bin/opencode.cjs`、`packages/cli/bin/opencode2.cjs` |
| **CLI 源码入口** | `packages/cli/src/index.ts` |
| 命令树定义 | `packages/cli/src/commands/commands.ts` |
| 命令处理器 | `packages/cli/src/commands/handlers/`（`default.ts` 默认 TUI、`serve.ts`、`run.ts`、`mini.ts`、`acp.ts`、`api.ts`、`auth/`、`mcp/`、`plugin/`、`service/`、`session/`、`debug/`） |
| Server 进程入口 | `packages/server/src/process.ts` |
| Server 路由组装 | `packages/server/src/routes.ts` |
| Server HttpApi 定义 | `packages/server/src/api.ts` + `packages/protocol/src/api.ts` |
| Server fetch handler（Workers/Bun/SDK） | `packages/server/src/fetch.ts` |
| Server workerd 入口 | `packages/server/src/workerd.ts` |
| Server 端点实现 | `packages/server/src/handlers/*.ts`（`session.ts`/`fs.ts`/`integration.ts`/`pty.ts`/`permission.ts`…） |
| **TUI 入口** | `packages/tui/src/index.tsx` → `packages/tui/src/app.tsx`（`run`） |
| **SDK 入口** | `packages/sdk/src/index.ts` → `packages/sdk/src/opencode.ts`（`create()`） |
| Client 入口 | `packages/client/src/`（`contract.ts`、`effect/`、`promise/`、`solid/`） |
| Schema 入口 | `packages/schema/src/index.ts` |

## 6. 核心概念与相互关系

- **Schema（形状真源）** `packages/schema/src/`：`agent.ts`、`tool.ts`、`session.ts`/`session-event.ts`/`session-message.ts`、`provider.ts`（`Provider.ID`+`Request`）、`model.ts`（`Model.Ref` 解析 `provider/model#variant`）、`permission.ts`、`mcp.ts`、`config.ts`、`event.ts`（事件清单）等。
- **Core（运行时）** `packages/core/src/`：
  - Agent：`agent.ts`（`Agent.Service`，Location 作用域；默认 agent id `"build"`）；内置 agent 在 `plugin/agent.ts`、`plugin/plan.ts`。
  - Tool：`tool.ts`（`Tool.Service` → `Tool.Snapshot`：`definitions`(LLM 工具定义) + `execute`）→ `tool/runtime.ts` → `tool/plugin/*`；注册在 `plugin/internal.ts`。
  - Provider/Model：`provider.ts`、`model.ts`/`model-resolver.ts`、`aisdk.ts`（对接 `@opencode/ai` 的 `LLMClient`）。
  - Session：`session.ts`（门面）→ `session/session.ts` → `session/inbox.ts`（准入）→ `session/execution.ts`（唤醒/协调）→ `session/runner/*`（drain）。

**一次 prompt 的链路：**
```
Session.prompt()                     core/src/session/session.ts
 └─> SessionInbox 发布事件            session/inbox.ts
      └─> SessionExecution.wake()     session/execution.ts
           └─> run-coordinator 选本地 drain   session/run-coordinator.ts
                └─> SessionRunner.drain       session/runner/index.ts
                     ├─ 解析 Agent/Model      agent.ts + runner/model.ts + model-resolver.ts
                     ├─ 组装请求              session/model-request.ts (Provider/aisdk)
                     ├─ runner/step.ts：LLMClient.stream 一次一步（无内存 tool loop）
                     │     └─ Tool.Snapshot.execute(toolCall, ctx)   tool.ts
                     └─ 事件经 Bus 发布 → Projector 落库   bus.ts → session/projector.ts
```
对应文字规约：`specs/v2/session.md`、`specs/v2/tools.md`、根 `AGENTS.md` 的 “Session Core” 段。

## 7. 配置文件（opencode.json）

- **发现**：`packages/core/src/config/discovery.ts`（`names = ["opencode.json", "opencode.jsonc"]`，按 `.claude`/`.agents`/`.opencode`/上级目录扫描）。
- **加载/归一化**：`packages/core/src/config.ts`（`Config.Service`）、`packages/core/src/config/normalize.ts`、`config/variable.ts`、`config/watch.ts`、`config/markdown.ts`。
- **Schema 权威定义**：`packages/schema/src/config.ts` + `packages/schema/src/config/`（`agent/command/compaction/experimental/formatter/lsp/mcp/media/model/plugin/policy/provider/reference/shell/tool-output/warming/watcher/websearch/worktree.ts`）。
- **配置→插件转译**：`packages/core/src/config/plugin/`。
- **V1 兼容/迁移**：`packages/core/src/v1/config/`、`packages/schema/src/v1/`。

⚠️ `packages/core/schema.json` 是 **Drizzle/SQLite 迁移快照**，不是配置 JSON Schema。

## 8. 其它重要参考

- `AGENTS.md`（根）：V2 架构约定最浓缩的文档（会话准入/执行/恢复规则、代码风格、测试、提交规范）。
- `specs/v2/`：`session.md`、`tools.md`、`event-stream-architecture.md`、`provider-policy.md` 等跨模块契约；`specs/v2/README.md` 有权威归属表（Protocol=操作/传输错误，Schema=公共形状/持久事件，Core=运行时/持久化）。
- `V2_HTTP_API_AUDIT.md`：139 个端点的逐一裁决（了解 API 边界与已移除端点）。
- OpenAPI：`packages/protocol/openapi.json`、`services/www/openapi.json`。
- 子包级 `AGENTS.md`：`packages/schema|tui|ai|cli/AGENTS.md`、`packages/core/src/tool/AGENTS.md`。
- 生成客户端：改 Protocol/Server 的 `HttpApi` 后须在 `packages/client` 跑 `bun run generate`（见根 AGENTS.md）。
- 插件/程序化集成：`packages/plugin/README.md`、`packages/sdk/README.md`。

## 9. 快速定位速查

| 想找什么 | 去哪里 |
|---|---|
| CLI 命令定义/实现 | `packages/cli/src/commands/commands.ts` + `handlers/*` |
| 核心运行时服务 | `packages/core/src/*.ts`（bus/session/tool/agent/provider/config…） |
| Session 执行循环 | `packages/core/src/session/runner/{index,step,model}.ts` |
| 内置工具 | `packages/core/src/tool/plugin/*` |
| Provider 适配 | `packages/core/src/plugin/provider/*` + `packages/ai/src/providers/*` |
| LLM 协议实现 | `packages/ai/src/protocols/*` |
| HTTP 端点契约/实现 | `packages/protocol/src/groups/*` + `packages/protocol/openapi.json` + `packages/server/src/handlers/*` |
| 公共数据结构 | `packages/schema/src/*.ts` |
| 配置 schema | `packages/schema/src/config.ts` + `config/` |
| TUI / Web GUI | `packages/tui/src/app.tsx` + `routes/` + `component/`；`packages/app/src/*` |
| 插件系统 | `packages/plugin/src/{effect,promise}/*` + `packages/core/src/plugin/*` |
| 进程内 SDK | `packages/sdk/src/opencode.ts` |
| 设计文档 | `specs/v2/*`、`AGENTS.md`、`V2_HTTP_API_AUDIT.md` |
