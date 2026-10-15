# pi 源码分析（架构概览）

> 源码路径：`/data/projects/pi`
> 根包版本：`0.0.3`（git HEAD `7d75a0e3e`）；各子包统一版本 `1.2.0`
> 用途：开发时需要参照 pi 实现时，先读本文档定位；不足时再进入源码细读。

## 0. 一句话定位

pi 是一个**极简、可扩展的 agent harness（智能体运行框架）**，核心产品是终端 AI coding agent。设计哲学：**core 保持极简，功能尽量放到扩展（extensions）/skill/prompt template/theme 中**；sub-agents、plan mode 等被有意省略，交给扩展实现。

发布形态：npm 包 `@earendil-works/pi-coding-agent`、pi.dev 安装脚本、独立二进制（Bun 编译 + esbuild bundle）、Nix flake。作者团队 **Earendil Works**（主要作者 Mario Zechner / badlogic），旧项目代号 `pi-mono`。

四种使用面：
- 交互式 TUI（`pi`）
- print / JSON 模式（`--print`、`--mode json`）
- RPC 模式（`--mode rpc`，JSONL 协议，可嵌入其它语言进程）
- **TypeScript SDK**（`@earendil-works/pi-coding-agent`，进程内 `createAgentSession()`）

**与 opencode 的关系**：README/文档中未描述从属或 fork 关系。pi 仅把 OpenCode 当作一个可连接的模型 provider（`packages/ai/src/providers/opencode.ts`、`opencode-go.ts`、`opencode-headers.ts`）。

## 1. 顶层目录结构

```
/data/projects/pi
├── packages/            monorepo 所有子包（见第 3、4 节）
├── scripts/             构建/发布/模型目录/各类 check 脚本
├── nix/                 Nix 打包（package.nix、model-catalog.json 版本 pin）
├── .github/workflows/   CI（ci.yml、build-binaries.yml、pr-gate.yml、nix.yml…）
├── .husky/pre-commit    提交前钩子（lockfile 守卫等）
├── .pi/                 本仓库自用 pi 工程资源：extensions/、skills/、prompts/
├── README.md / CONTRIBUTING.md / AGENTS.md / SECURITY.md
├── package.json / package-lock.json / biome.json
├── tsconfig.base.json / tsconfig.json / vitest.base.ts
├── flake.nix / flake.lock
├── test.sh              隔离环境跑全量测试
├── pi-test.sh           从源码直跑 pi CLI（支持 --no-env）
├── pi-test.ps1 / .bat   Windows 启动脚本
└── .npmrc               save-exact=true、min-release-age=2（供应链加固）
```

- `pi-test.sh` 通过 `--import` 加载 `packages/coding-agent/src/experimental/source-resolver.ts`，再执行 `packages/coding-agent/src/cli.ts`，实现**不改产物、直接从 TS 源码运行**。

## 2. Monorepo 与包管理

- **npm workspaces**（根 `package.json`，根包名 `pi-monorepo`，`private: true`，`type: module`）。
- workspaces：`packages/*`、`apps/*`（**预留，目录当前不存在**），以及 coding-agent 下示例扩展目录（`examples/extensions/with-deps`、`custom-provider-anthropic`、`custom-provider-gitlab-duo`、`sandbox`、`gondolin`）。
- 版本：子包统一 `1.2.0`；`version:*` 脚本用 `npm version --workspaces` + `scripts/sync-versions.js`。
- **供应链加固**：直接外部依赖 pin 精确版本、内部 workspace 包用 `^`；`save-exact=true`；`package-lock.json` 为依赖真相源（pre-commit 阻止误提交）；`packages/coding-agent/install-lock/` 由根 lockfile 生成；大量 `check:*` 脚本校验 pinned deps / runtime deps / TS 相对导入 / 入口图 / install-lock。
- **构建顺序**（根 `build`）：chord → tui → telemetry → codemode → mcp → ai → durable → env → agent → coding-agent。

## 3. packages/ 子包（11 个）

| 目录 | 包名 | 用途 | 入口 |
|---|---|---|---|
| `packages/coding-agent` | `@earendil-works/pi-coding-agent` | 交互式 coding agent CLI（核心产品包） | `src/index.ts`（SDK）、`src/cli.ts`（CLI）、`src/rpc-entry.ts`；`bin.pi = dist/bundle/cli.js` |
| `packages/agent` | `@earendil-works/pi-agent-core` | 通用 agent 运行时：工具调用、状态管理、传输抽象 | `src/index.ts`（agent.ts / agent-loop.ts / proxy.ts / stream-fn.ts / types.ts） |
| `packages/ai` | `@earendil-works/pi-ai` | 统一多 provider LLM API（OpenAI/Anthropic/Google…） | `src/index.ts`、`src/compat.ts`、`src/oauth.ts`；`bin.pi-ai` |
| `packages/tui` | `@earendil-works/pi-tui` | 差分渲染终端 UI 库 | `src/index.ts`；原生模块 `native/{darwin,linux,win32}` |
| `packages/durable` | `@earendil-works/pi-durable` | 持久化 conversation/task/document 运行时 | `src/index.ts`，子路径 `./harness`、`./session`、`./storage/*`、`./tools`、`./codemode`、`./images/*` |
| `packages/chord` | `@earendil-works/chord` | 可热重载插件的类型化宿主运行时（contracts、复制状态、JSON link RPC） | `src/index.ts`，`./context`、`./delta`、`./node` |
| `packages/codemode` | `@earendil-works/pi-codemode` | 沙箱化 JS 执行（唯一能力是调用注入工具），基于 quickjs-wasi | `src/index.ts`，`./declarations`、`./source`、`./portable`、`./cloudflare`、`./worker` |
| `packages/mcp` | `@earendil-works/pi-mcp` | 独立 MCP 客户端 | `src/index.ts`，`./oauth`、`./testing` |
| `packages/env` | `@earendil-works/pi-env` | Pi Durable 的远程执行环境（SSH 小 daemon + ExecutionEnv 客户端） | `src/index.ts`；Rust daemon `daemon/` |
| `packages/telemetry` | `@earendil-works/pi-telemetry` | 厂商中立 telemetry 契约、参考适配器、一致性测试 | `src/index.ts`，`./testing` |
| `packages/evals` | `@earendil-works/pi-evals`（private） | 评测框架，依赖 coding-agent | `src/cli.ts`、`src/harness.ts` |

> `tsconfig.json` 中存在 `@earendil-works/pi-agent-old` 路径映射，但无对应目录，属历史遗留。

## 4. 核心运行时代码（以 coding-agent 为主）

主运行时目录：`packages/coding-agent/src/`

**顶层文件**：`cli.ts`（最薄入口，`setupCli()` + `main()`）、`main.ts`（**真正启动主逻辑**，约 1000 行：参数解析、建 session/runtime、选 appMode、分发）、`index.ts`（**SDK 公共导出面**：`createAgentSession`、`SessionManager`、工具工厂、UI 组件、theme）、`config.ts`（路径解析、安装方式检测、自更新、用户配置目录 `~/.pi/agent`）、`rpc-entry.ts`、`migrations.ts`、`package-manager-cli.ts`。

**子目录**：
| 目录 | 职责 |
|---|---|
| `src/cli/` | 参数与命令实现：`args.ts`、`auth-check.ts`、`auth-command.ts`、`credential-print.ts`、`file-processor.ts`、`initial-message.ts`、`list-models.ts`、`project-trust.ts`、`session-picker.ts`、`setup.ts`、`startup-ui.ts`、`config-selector.ts` |
| `src/core/` | 引擎核心（60+ 文件，见下） |
| `src/core/tools/` | 内置工具：`read.ts`、`bash.ts`、`edit.ts`、`write.ts`、`grep.ts`、`find.ts`、`ls.ts`、`powershell.ts`、`edit-diff.ts`、`truncate.ts`、`renderers/` |
| `src/core/extensions/` | 扩展系统：`loader.ts`、`runner.ts`、`types.ts`、`jiti-loader.ts`、`wrapper.ts`、`virtual-modules.ts` |
| `src/core/compaction/` | 上下文压缩：`compaction.ts`、`branch-summarization.ts`、`utils.ts` |
| `src/core/export-html/` | 会话导出 HTML |
| `src/modes/` | 运行模式：`interactive/`、`rpc/`、`print-mode.ts`、`json-event.ts` |
| `src/modes/interactive/` | TUI 模式：`interactive-mode.ts`、`tui-renderer.ts`、`chat-viewport.ts`、`external-editor.ts`、`components/`（48 个 UI 组件）、`theme/` |
| `src/modes/rpc/` | RPC：`rpc-mode.ts`、`rpc-client.ts`、`rpc-types.ts`、`jsonl.ts` |
| `src/extensions/` | 内置扩展：`codemode/`、`mcp/`、`tool-search/`、`llama/`、`index.ts`（`builtInExtensions`） |
| `src/utils/` | 通用工具（37 文件）：shell、git、image-*、clipboard、syntax-highlight、zip、wsl 等 |
| `src/bun/` | Bun 二进制入口：`cli.ts`、`runtime-setup.ts`、`sandbox-env-setup.ts` |
| `src/experimental/` | `source-resolver.ts`（源码直跑解析器） |

**`src/core/` 关键文件（引擎心脏）**：
- 会话/运行时：`agent-session.ts`（**核心抽象，被所有 mode 共用**：状态、事件订阅、持久化、模型/thinking、compaction、bash、分支）、`agent-session-runtime.ts`、`agent-session-services.ts`
- 会话存储：`session-manager.ts`（JSONL 条目树，权威上下文）、`session-export.ts`、`session-cwd.ts`
- 模型：`model-runtime.ts`（ModelRuntime）、`model-registry.ts`、`model-resolver.ts`、`model-config.ts`、`provider-composer.ts`、`remote-catalog-provider.ts`、`virtual-models.ts`、`models-store.ts`
- 配置：`settings-manager.ts`、`settings-schema.ts`、`settings-defaults.ts`、`trust-manager.ts`、`project-trust.ts`
- 资源：`resource-loader.ts`、`package-manager.ts`、`skills.ts`、`prompt-templates.ts`、`slash-commands.ts`、`system-prompt.ts`、`pi-manifest.ts`、`mcp-servers.ts`
- 鉴权：`auth-storage.ts`、`runtime-credentials.ts`、`auth-guidance.ts`
- 其它：`sdk.ts`（工厂，含 `createAgentSession`）、`event-bus.ts`、`bash-executor.ts`、`output-guard.ts`、`crash-log.ts`、`bug-report.ts`、`telemetry.ts`、`keybindings.ts`

**相邻运行时包**：
- **agent-core** `packages/agent/src/`：`agent.ts`（`Agent` 类，持有 state/messages/tools）、`agent-loop.ts`（**agent 主循环**：流式响应、tool 调用并行/串行、steering/follow-up、before/afterToolCall 钩子、`runToolCall`）、`proxy.ts`、`stream-fn.ts`、`types.ts`。
- **pi-ai** `packages/ai/src/`：`providers/`（约 95 文件，每 provider `.ts` + `.models.ts`）、`api/`（约 43 文件，各 API 协议实现 + `.lazy.ts`）、`auth/`、`compat.ts`、`models.ts`、`model-catalog.ts`、`oauth.ts`、`bedrock-provider.ts`、`utils/`、`scripts/generate-models.ts`（生成 `models.generated.ts`，**禁止手改**）。
- **pi-tui** `packages/tui/src/`：`tui.ts`、`tui-main-screen.ts`、`tui-alt-screen.ts`、`terminal.ts`、`editor-component.ts`、`keybindings.ts`、`layout.ts`、`colors.ts`、`components/`（box/editor/markdown/select-list/scroll-view/image/loader）、`native-*.ts`。
- **pi-durable** `packages/durable/src/`：`harness/`（22 文件：agent、scheduler、task-graph、prompt、provider、registry、inbox、events）、`session/`（session、transaction、forks、observation）、`storage/`（`jsonl/`、`sqlite/`、memory、scan）、`tools/`、`codemode/`、`env/`、`images/`、`tool-search/`。
- **chord** `packages/chord/src/`：`host.ts`、`contract.ts`、`registry.ts`、`manager.ts`、`link.ts`、`wire.ts`、`websocket.ts`、`state.ts`、`reload.ts`、`loader.ts`；子目录 `context/`、`delta/`。
- **pi-codemode** `packages/codemode/src/`：`runtime/`（`vm.ts`、`worker.ts`、`execution.ts`、`host.ts`、`protocol.ts`）、`cloudflare.ts`、`declarations.ts`、`source.ts`、`portable.ts`、`wasm.ts`。
- **pi-mcp** `packages/mcp/src/`：`client.ts`、`transports/`（stdio、streamable-http、in-memory）、`protocol/`、`oauth/`。
- **pi-telemetry**：`index.ts`、`memory.ts`、`noop.ts`、`testing/`。
- **pi-env**：`remote-env.ts`、`connection.ts`、`ssh.ts`、`terminal.ts`、`watch.ts` + `daemon/`（Rust）。

## 5. 关键入口点

- CLI 源码入口：`/data/projects/pi/packages/coding-agent/src/cli.ts`
- CLI 主逻辑：`/data/projects/pi/packages/coding-agent/src/main.ts`
- RPC 入口：`/data/projects/pi/packages/coding-agent/src/rpc-entry.ts`
- Bun 二进制入口：`/data/projects/pi/packages/coding-agent/src/bun/cli.ts`
- SDK 入口：`/data/projects/pi/packages/coding-agent/src/index.ts`；工厂 `/data/projects/pi/packages/coding-agent/src/core/sdk.ts`
- agent-core 入口：`/data/projects/pi/packages/agent/src/index.ts`
- pi-ai 入口：`/data/projects/pi/packages/ai/src/index.ts`（现代 API）与 `.../compat.ts`（旧全局 API）
- tui 入口：`/data/projects/pi/packages/tui/src/index.ts`
- 启动脚本：`pi-test.sh`（→ `packages/coding-agent/src/cli.ts`）、`test.sh`
- Nix 入口：`nix/package.nix`（`makeWrapper node .../dist/bundle/cli.js`）

## 6. 构建 / 测试

- **Lint/格式化**：`biome.json`（Biome 2.3.5；tab、indentWidth 3、lineWidth 120；Grit 插件 `scripts/biome/model-type-comparison.grit`）。检查脚本 `scripts/biome-check.mjs`。
- **TS**：`tsconfig.base.json`（ES2024、module Node16、strict、`erasableSyntaxOnly`、`verbatimModuleSyntax`、`allowImportingTsExtensions`）；`tsconfig.json` 定义 workspace 路径别名，`noEmit: true`。
- **测试**：`vitest.base.ts`（根，别名指向 workspace 源码）；各包 `vitest.config.ts`（coding-agent 默认 `PI_OFFLINE=1`）；**`packages/tui` 例外，用 node:test**。
- **根 scripts**：
  - 构建：`build`、`build:offline`、`build:native:{darwin,linux,win32}`
  - 检查：`check` = biome + pinned-deps + runtime-deps + ts-imports + entry-graphs + install-lock + `tsc --noEmit` + browser-smoke
  - 测试：`test` = `test:scripts`（node --test）+ 各 workspace test；`test:mcp-conformance`
  - 模型数据：`generate:models`、`hydrate:model-data`、`update:model-catalog-pin`、`publish-model-catalog`
  - 发布：`publish`、`release:{patch,minor,major}`、`pack:packages`；评测：`eval`
- **构建脚本** `scripts/`：`build-coding-agent-bundle.mjs`（esbuild → `dist/bundle`）、`build-binaries.sh`（跨平台二进制 → `packages/coding-agent/binaries/`）、`generate-coding-agent-install-lock.mjs`、`pack-packages.mjs`；校验类 `check-*.mjs`。
- **CI**：`.github/workflows/`（`ci.yml`、`build-binaries.yml`、`pr-gate.yml`、`npm-audit.yml`、`nix.yml`、`publish-model-catalog.yml`）。

## 7. 其它重要参考

- **文档（最系统）**：`packages/coding-agent/docs/`（42 文件）。重点：`how-pi-works.md`（agent loop/context/session/interfaces/extensions 总览）、`index.md`、`quickstart.md`、`usage.md`、`cli.md`、`json.md`、`rpc.md`、`rpc-commands.md`、`sdk.md`、`extensions.md`、`skills.md`、`prompt-templates.md`、`themes.md`、`packages.md`、`configuration.md`、`settings.md`、`session-format.md`、`sessions.md`、`message-types.md`、`compaction.md`、`models.md`、`providers.md`、`custom-provider.md`、`virtual-models.md`、`mcp.md`、`codemode.md`、`security.md`、`containerization.md`。
- **示例**：`packages/coding-agent/examples/sdk/01-minimal.ts` … `14-codemode-mcp.ts`；扩展示例 80 个在 `examples/extensions/`（`plan-mode/`、`subagent/`、`sandbox/`、`permission-gate.ts`、`dynamic-tools.ts`、`todo.ts`、`git-checkpoint.ts`…）。
- **测试用例**（行为规格）：`packages/coding-agent/test/`（约 205 项，文件名即功能地图）；夹具 `test/fixtures/`、`test/suite/harness.ts`。
- **本仓库自用 pi 资源**：`.pi/extensions/`、`.pi/skills/{add-llm-provider,interactive-testing,release}.md`、`.pi/prompts/*`。
- **打包/供应链**：`packages/coding-agent/install-lock/`、`nix/package.nix`、`nix/model-catalog.json`、`flake.nix`。
- **环境变量**：`PI_CODING_AGENT_DIR`、`PI_CODING_AGENT_SESSION_DIR`、`PI_OFFLINE`、`PI_PACKAGE_DIR`、`PI_NO_LOCAL_LLM`、`PI_STARTUP_BENCHMARK`（集中在 `src/config.ts`）。
- **CLI 命令全集**（见 `docs/cli.md`）：`pi`、`pi install/remove/uninstall/update/list/config`、`pi auth <check|print-api-key|print-bearer-token>`、`pi mcp <list|login|logout>`。

## 8. 快速定位速查

| 想找什么 | 去哪里 |
|---|---|
| CLI 参数/命令 | `packages/coding-agent/src/cli/args.ts` + `main.ts` |
| Agent 主循环 | `packages/agent/src/agent-loop.ts` |
| Agent 状态对象 | `packages/agent/src/agent.ts` |
| 会话/分支/持久化 | `packages/coding-agent/src/core/session-manager.ts`、`agent-session.ts` |
| 内置工具实现 | `packages/coding-agent/src/core/tools/*.ts` |
| 扩展系统与钩子 | `packages/coding-agent/src/core/extensions/{loader,runner,types}.ts`、`docs/extensions.md` |
| 模型/Provider | `packages/ai/src/providers/`、`packages/ai/src/api/`、`coding-agent/src/core/model-runtime.ts` |
| TUI 组件 | `packages/tui/src/components/`、`coding-agent/src/modes/interactive/components/` |
| RPC 协议 | `packages/coding-agent/src/modes/rpc/`、`docs/rpc.md`、`docs/rpc-commands.md` |
| SDK 用法 | `packages/coding-agent/src/core/sdk.ts`、`docs/sdk.md`、`examples/sdk/` |
| 上下文压缩 | `packages/coding-agent/src/core/compaction/` |
| MCP / codemode | `packages/mcp/`、`packages/codemode/`、`coding-agent/src/extensions/{mcp,codemode}` |
| Nix / 打包 | `nix/`、`flake.nix`、`scripts/build-binaries.sh`、`coding-agent/install-lock/` |
