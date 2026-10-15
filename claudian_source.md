# claudian 源码分析（架构概览）

> 源码路径：`/data/projects/claudian`
> 版本：`2.3.21`（git HEAD `71671efd`）；Obsidian 插件 id `realclaudian`，名称 Claudian
> 用途：开发时需要借鉴 claudian 实现时，先读本文档定位；不足时再进入源码细读。

## 0. 一句话定位

Claudian 是一个 **Obsidian 桌面插件**：把多种 AI coding agent（Claude Code、Codex、Grok Build、OpenCode、Pi、DeepSeek Harness）嵌入 Obsidian vault，**以 vault 作为 agent 的工作目录**，可用文件读写/搜索/bash/多步工作流。

- 运行形态：桌面端 Obsidian 插件（`isDesktopOnly: true`），构建产物为单文件 CJS bundle `main.js` + `styles.css` + `manifest.json`。
- 核心是 Claude Code 集成（依赖官方 `@anthropic-ai/claude-agent-sdk`），但**多 provider**：6 个内建 provider 适配器 + 共享 ACP。
- 技术栈：TypeScript；esbuild 打包（CJS, target es2022）；Obsidian API 1.13.0；Node `>=24 <25`（`.node-version` = 24.16.0）；Jest + ts-jest 测试。
- 依赖亮点：`@anthropic-ai/claude-agent-sdk`、`@modelcontextprotocol/sdk`、`ws`（本地回环 session 工具）、`jsonc-parser`、`yaml`、`@codemirror/*`、`@lezer/markdown`、`cross-spawn`。
- 包管理器：同时维护 `bun.lock` 与 `package-lock.json`，CI 校验两者一致。
- 约定：代码/注释/提交用英文；未提交的笔记、临时脚本放 `.context/`（已 gitignore）；`AGENTS.md` 是权威约束文档，每个同级 `CLAUDE.md` 内容仅为 `@AGENTS.md`。

## 1. 顶层目录结构

| 路径 | 职责 |
|---|---|
| `src/` | 全部源码（见第 2、3 节） |
| `src/main.ts` | **唯一组合根 / 生命周期发布者**（见第 4 节） |
| `scripts/` | 构建、测试编排、架构边界/依赖一致性/启动性能等静态检查脚本（`.mjs`/`.js`/`.cjs`） |
| `tests/` | 单元 + 集成测试（Jest）与 fixtures/helpers/mocks |
| `.github/workflows/` | CI、Release、Nightly、CodeQL、Claude 辅助、issue 去重、stale |
| `assets/` | README 截图与 `assets/sponsors/` |
| `esbuild.config.mjs` | esbuild 打包配置 + 多个自定义插件（SDK patch、省略未用 peer、压缩静态资源、renderer 安全 timer patch、拷贝到 vault） |
| `manifest.json` | Obsidian 插件清单 |
| `paseo.json` | 外部工具（Paseo）的 worktree setup/teardown 与 build 脚本入口，指向 `.context/paseo-setup.sh` 等 |
| `jest.config.js` / `tsconfig.jest.json` | Jest 双 project（unit/integration）+ ts-jest + 别名映射 |
| `eslint.config.mjs` | ESLint 10 flat config，含自定义文件名命名规则与 `eslint-plugin-obsidianmd` |
| `tsconfig.json` | 编译选项 + 别名 `@/*`→`src/*`、`@test/*`→`tests/*` |
| `AGENTS.md` / `CLAUDE.md` / `CONTRIBUTING.md` / `README.md` | 架构与贡献约束文档 |
| `.node-version` / `.npmrc` / `.env.local.example` | Node 24.16.0；npm 配置；`OBSIDIAN_VAULT` 开发自动拷贝示例 |
| `.context/` | 本地（gitignore）分析产物：**`.context/architecture/` 已有中文架构图**（见第 7 节） |

CI workflows：`ci.yml`、`release.yml`、`nightly.yml`、`codeql.yml`、`claude.yml`、`claude-code-review.yml`、`duplicate-issues.yml`、`stale.yml`。

## 2. src/ 模块结构与规模

| 目录 | 文件数 | 代码行数 | 定位 |
|---|---|---|---|
| `src/providers/` | 324 | ~58,434 | **agent 适配器实现**（6 provider + 共享 ACP），最大 |
| `src/features/` | 229 | ~42,895 | 面向用户的 UI 与功能（chat / inline-edit / settings / skills） |
| `src/core/` | 109 | ~13,378 | **provider 中性契约**：execution / providers / prompt / process / tools / types |
| `src/app/` | 21 | ~4,933 | 领域服务：会话、设置、存储、启动装配 |
| `src/shared/` | 31 | ~4,132 | 可复用 UI 组件、设置控件、下拉/mention |
| `src/utils/` | 16 | ~1,985 | 无领域依赖的叶子工具 |
| `src/composition/` | 4 | ~524 | main 与 app/features 之间共享的宿主对象与视图接线 |
| `src/i18n/` | 3 (+10 JSON) | ~189 | 国际化，10 个 locale（含 `zh-CN`/`zh-TW`） |
| `src/style/` | CSS | — | 模块化 CSS，由 `index.css` 汇聚 |

### 分层依赖方向（架构硬约束）

```
① 组合根:   src/main.ts + src/composition/**   （唯一装配点）
② UI:      src/features/**                     （provider 中性）
③ 领域服务: src/app/**                          （provider 中性）
④ 契约:     src/core/**                          （不 import 任何具体实现）
⑤ 实现:     src/providers/**                     （实现 core 契约，驱动外部 agent 进程）
```

禁止：`core → providers`、`features → providers`、`features → 原生协议(HTTP/JSONL/ACP)`。由 `scripts/check-architecture-boundaries.test.mjs` 自动校验。

## 3. 各层关键文件

### `src/core/`（provider 中性契约）
| 子目录 | 关键文件 | 职责 |
|---|---|---|
| `core/execution/`（16） | `ProviderExecutionBackend.ts`、`ProviderExecutionSession.ts`、`ProviderExecutionEvent.ts`、`ProviderExecutionLifecycleRegistry.ts`、`ProviderSessionSnapshot.ts`、`index.ts`（barrel）、`RewindableExecutionSession.ts`、`BranchableExecutionSession.ts` | 运行/会话/事件/交互原语；生命周期租约与 transition |
| `core/providers/`（15） | `ProviderRegistry.ts`（静态注册表）、`ProviderWorkspaceRegistry.ts`、`ProviderHost.ts`（provider 可见能力接口）、`types.ts`、`ProviderSettingsCoordinator.ts`、`models/`、`cli/`、`commands/`、`metadata/`、`settings/` | provider 注册、能力声明、workspace 服务、模型/命令元数据契约 |
| `core/process/`（7） | `ManagedStdioProcess.ts`、`cliBinaryLocator.ts`、`cliPath.ts`、`env.ts`、`loginShellPath.ts`、`windowsCmdShim.ts`、`ProcessProbe.ts` | CLI 定位、子进程管理、登录 shell PATH 解析、Windows shim |
| `core/prompt/`（9） | `mainAgent.ts`、`promptContext.ts`、`historyContext.ts`、`editorContext.ts`、`browserContext.ts`、`canvasContext.ts`、`inlineEdit.ts`、`titleGeneration.ts`、`promptXML.ts` | 提示词与上下文编码 |
| `core/session-tools/`（6） | `LoopbackSessionToolServer.ts`、`sessionToolMCP.ts`、`sessionToolSpecs.ts`、`invokeSessionTool.ts`、`SessionToolEndpoint.ts`、`sessionMessageEnvelope.ts` | 跨会话消息工具的 127.0.0.1 回环服务（含 MCP 暴露） |
| `core/tools/`（8） | `toolNames.ts`、`toolDiff.ts`、`toolIcons.ts`、`toolInput.ts`、`toolResultContent.ts`、`toolResultDetails.ts`、`toolProviderPayload.ts`、`todo.ts` | 工具名/图标/结果归一化的中性层 |
| `core/types/`（7） | `index.ts`（barrel）、`chat.ts`、`settings.ts`、`tools.ts`、`provider.ts`、`diff.ts`、`turnStats.ts` | 全局类型契约（`ClaudianSettings`、`Conversation`、`VIEW_TYPE_CLAUDIAN` 等） |
| 其它 | `core/commands/`（3）、`core/auxiliary/`（6）、`core/bootstrap/`（3）、`core/storage/`（2，`VaultFileAdapter`）、`core/rpc/`（`JSONRPCTransport`）、`core/security/`（`approvalRules`）、`core/device/`、`core/path/`（`LinkedContentPath`）、`core/performance/`（`StartupProfiler`） | 命令解析、辅助服务（标题生成/内联编辑）、存储适配、RPC、审批规则等 |

### `src/app/`（领域服务）
| 子目录 | 关键文件 | 职责 |
|---|---|---|
| `app/conversations/` | `ConversationRepository.ts`（**1398 行，最大**）、`ConversationService.ts`、`SessionMetadataLoader.ts`、`NativeSessionArchiveSync.ts`、`ConversationExecutionBindings.ts`、`ConversationModelRecovery.ts`、`ConversationRecordFence.ts` | 会话唯一权威：创建/删除/改名/归档/模型选择/sessionId 绑定；原生历史归档同步 |
| `app/settings/` | `ClaudianSettingsStorage.ts`（610）、`RuntimeSettingsCoordinator.ts`（289）、`SettingsCoordinator.ts`、`EnvironmentSettingsService.ts`、`ChatModelSelectionCoordinator.ts`、`defaultSettings.ts` | 设置持久化与串行化变更、运行时环境变量 |
| `app/storage/` | `SessionStorage.ts`（373）、`SharedStorageService.ts`、`ConversationPersistenceStore.ts`、`migrateSessionSidecars.ts` | vault 内 `.claudian/` 存储、会话 sidecar 迁移 |
| `app/startup/` | `ApplicationStartup.ts`（231） | `startApplication()`：加载设置 + 会话元数据，组装 `ApplicationDomains` |

### `src/features/`（UI 与功能）
| 子目录 | 规模 | 职责 |
|---|---|---|
| `features/chat/` | 最大（数十文件） | 侧边栏聊天：`ClaudianView.ts`（768）、`ChatFeatureHost.ts`、`ChatSettings.ts` |
| `features/inline-edit/` | 9 | 内联编辑 modal 与 provider 编辑服务（`ui/InlineEditModal.ts` 1031） |
| `features/settings/` | 2 | `ClaudianSettings.ts`（825，设置外壳：General/Providers/Skills 三 tab） |
| `features/agent-skills/` | 12 | Vault skill 管理（`.claude/skills` ⇄ `.agents/skills`）：`AgentSkillSettings.ts`、`ClaudeSkillSync.ts` |
| `features/FeatureHost.ts` | — | feature 层可见的宿主能力接口 |

`features/chat/` 关键子目录：

| 子目录 | 关键文件 | 职责 |
|---|---|---|
| `chat/tabs/`（26，含 `forking/`、`persistence/`、`runtime/`） | `TabManager.ts`（942）、`ChatTab.ts`、`TabLifecycle.ts`、`ParkedDrafts.ts`、`ForkTarget.ts` | Tab 生命周期/身份/持久化/分叉/宽窄布局 |
| `chat/execution/` | `ChatExecutionCoordinator.ts`（**1207，全局最大**）、`consumeExecutionEvents.ts`、`ExecutionInteractions.ts`、`ExecutionSessionSupervisor.ts`、`SessionEventStream.ts` | 驱动 provider execution、事件消费、交互、usage |
| `chat/turns/` | `MainTurnExecution.ts`（642）、`StreamController.ts`、`TurnQueue.ts`、`TurnSteering.ts`、`ToolCallStream.ts` | 单轮执行、流式渲染、排队、steering |
| `chat/sessions/` | `ChatSessionRuntime.ts`、`SessionRuntimePool.ts`、`createChatSessionRuntime.ts`、`SessionTranscript.ts`、`SessionComposer.ts` | session 池与运行时组装（不依赖 tab/DOM） |
| `chat/rendering/` | `MessageRenderer.ts`（709）、`markdown/`（5）、`tools/`（14：`ToolCallRenderer.ts`、`DiffRenderer.ts`…）、`ResponseFinalizer.ts` | 消息/工具/Markdown/Mermaid 渲染 |
| `chat/subagents/` | `SubagentManager.ts`（767）、`SubagentStreamRouter.ts`、`SubagentRenderer.ts` | 子 agent 卡片与流路由 |
| `chat/session-manager/` | `SessionBrowser.ts`（764）、`SessionSidebar.ts`、`SessionActions.ts`、`SessionNavigation.ts` | 双栏模式下的会话管理侧栏 |
| `chat/interactions/` | `InlineAskUserQuestion.ts`（756）、`SessionInteractions.ts`、`interactionPromptPort.ts` | 审批/提问交互 |
| `chat/composer/`（含 `toolbar/`） | `InputController.ts`（628）、`ComposerEditor.ts`、`toolbar/ModelSelector.ts`、`toolbar/PermissionToggle.ts`、`ContextUsageMeter.ts` | 输入区、草稿、工具栏（模型/模式/权限/effort） |
| `chat/input/` | `SelectionController.ts`（474）、`BrowserSelectionController.ts`、`CanvasSelectionController.ts` | 编辑器/浏览器/画布选区上下文 |
| `chat/side-chat/` | `SideChatRuntime.ts`（603）、`SideChatController.ts`、`SideChatPanel.ts` | `/side`、`/btw` 临时对话 |
| `chat/zen/` | `ZenModeController.ts`、`ZenModePanel.ts`、`ZenComposerLayout.ts`、`ZenPanelDock.ts` | 禅模式 |
| `chat/state/` | `ChatState.ts`（448）、`ResponseModel.ts`、`usageInfo.ts` | 会话状态模型 |
| `chat/session-messaging/` | `SessionMessagingService.ts`、`SessionMailbox.ts`、`formatSessionSnapshot.ts` | 跨会话消息投递（`@mention` 其他会话） |
| `chat/linked-content/` | `LinkedContentController.ts`、`LinkedContentChip.ts` | linked content（笔记作为会话身份） |
| `chat/workspace/` | `ChatViewPublisher.ts`、`ChatTabCommands.ts`、`ChatFocusCommand.ts`、`VaultContentEvents.ts` | 视图发布、命令注册、vault 事件 |
| `chat/conversation/` | `ConversationController.ts`、`ConversationLifecycle.ts`、`ConversationBranching.ts`、`ConversationRewind.ts`、`InactiveSessionArchiver.ts` | 会话控制/生命周期/分叉/回退/归档 |
| `chat/navigation/`、`chat/utils/`、`chat/view/` | `NavigationController.ts`、`DualPaneLayout.ts`、`ChatPresentationPlacement.ts` | 导航、双栏布局、展示位 |

### `src/providers/`（agent 适配器）

`src/providers/index.ts` 是注册入口（`registerBuiltInProviders()`），`defaultProviderConfigs.ts` 汇总各 provider 默认配置。

| Provider | 行数 / 文件 | 集成协议 | 目录要点 |
|---|---|---|---|
| `providers/codex/` | 16,104 / 71 | Codex app-server（JSON-RPC，25 文件 `runtime/`） | `execution/`（ThreadBinder/TurnBinding）、`runtime/`（AppServer、RPCTransport、DynamicToolRegistry、ModelDiscovery）、`history/`、`normalization/` |
| `providers/claude/` | 11,160 / 70 | Claude Agent SDK + 原生 JSONL 历史 | `execution/`（Backend/Session/EventNormalizer/InteractionRouter）、`history/`（13）、`runtime/`（CLIResolver/Rewind/Models）、`sdk/`、`app/ClaudeWorkspaceServices.ts` |
| `providers/opencode/` | 8,625 / 53 | **v2: HTTP+SSE**（`opencode serve`）/ **v1: ACP**，`OpencodeSessionKernel.ts` 按版本分发 | `execution/`（`OpencodeHTTPSessionKernel`、`OpencodeACPSessionKernel`）、`http/`、`history/`（SQLite `OpencodeSqliteReader`） |
| `providers/pi/` | 8,202 / 39 | **RPC over stdio（JSONL）**，`pi --mode rpc` | `execution/`、`runtime/`（RPCTransport/Subprocess/LaunchSpecBuilder）、`history/`（JSONL 树/分支）、`normalization/` |
| `providers/grok/` | 7,202 / 39 | Grok Build **ACP** 适配 | `execution/`、`runtime/`、`history/`、`normalization/`、`prompt/`、`ui/` |
| `providers/deepseek/` | 4,634 / 35 | DeepSeek Harness **native Remote**；单进程 `DeepSeekHost` + `DeepSeekRoster` | `remote/DeepSeekRemoteClient.ts`、`runtime/`（Host/HostProcess/Roster/LaunchArtifacts）、`history/` |
| `providers/acp/` | 2,456 / 15 | **共享 ACP 协议机制**（无 provider 策略） | `ACPClientConnection.ts`、`ACPJSONRPCTransport.ts`、`ACPSubprocess.ts`、`ACPExecutionEventNormalizer.ts`、`buildACPUsageInfo.ts` |

每个 provider 统一结构（以 `claude/` 为例）：`registration.ts`（`ProviderModule` 契约）、`capabilities.ts`、`settings.ts`、`ui/`（ChatUIConfig + SettingsTab）、`execution/`、`runtime/`、`history/`、`normalization/`、`app/`（WorkspaceServices）、`AGENTS.md`。

`ProviderModule` 契约见 `src/providers/claude/registration.ts`（`id/displayName/blankTabOrder/isEnabled/capabilities/modelPolicy/chatUIConfig/settingsReconciler/createExecutionBackend/historyService/workspace` 等）。

### `src/shared/` 与 `src/utils/`

- `shared/components/`：`CopyableCodeFence.ts`、`ResumeSessionDropdown.ts`、`SelectionHighlight.ts`
- `shared/composer-dropdown/`：`ComposerDropdownController/View`、`MentionSource`、`SlashCommandSource`（`/` 与 `@` 下拉）
- `shared/mention/`：`VaultMentionCache.ts`、`VaultMentionDataProvider.ts`
- `shared/settings/`（11）：`ProviderModelPicker.ts`、`CLIInstallationSetting.ts`、`EnvSnippetManager.ts`、`DebouncedSettingsWriter.ts`
- `shared/toolbar/`：`ModelSelector.ts`、`EffortSelector.ts`、`ToolbarMenu.ts`
- `utils/`：`path.ts`（`getVaultPath`）、`jsonlFile.ts`、`LineBuffer.ts`、`electronCompat.ts`、`markdownMath/Segments/TextTokens.ts`、`imageEmbed.ts`、`fileLink.ts`

## 4. 关键入口点

| 入口 | 路径 | 说明 |
|---|---|---|
| **插件主入口** | `src/main.ts` | `export default class ClaudianPlugin extends Plugin`；`onload()` 注册视图/命令/设置面板，`loadApplication()` 组装 provider/feature/chat host，`onunload()` 有序关闭 |
| 应用启动装配 | `src/app/startup/ApplicationStartup.ts`（`startApplication()`） | 返回 `ApplicationDomains`（storage/settings/conversations/…） |
| provider 注册 | `src/providers/index.ts`（`registerBuiltInProviders()`） | 写入 `ProviderRegistry` 与 `ProviderWorkspaceRegistry` |
| 视图注册 | `src/main.ts`（`registerView(VIEW_TYPE_CLAUDIAN, ...)`） | 视图实现 `src/features/chat/ClaudianView.ts`；定位/打开 `src/composition/ClaudianViews.ts` |
| 命令注册 | `src/main.ts` | `open-view`、inline-edit、chat tab 命令、focus、zen toggle 等；工厂在 `features/chat/workspace/ChatTabCommands.ts`、`ChatFocusCommand.ts`、`features/inline-edit/inlineEditCommand.ts` |
| 设置面板 | `src/features/settings/ClaudianSettings.ts`（`ClaudianSettingTab`） | plugin-facing `FeatureHost` |
| 宿主接口（契约） | `src/features/FeatureHost.ts`、`src/features/chat/ChatFeatureHost.ts`、`src/core/providers/ProviderHost.ts` | feature / chat / provider 三层窄接口；实现类在 `src/composition/ClaudianFeatureHosts.ts`、`ClaudianProviderHost.ts` |
| Provider 注册表 API | `src/core/providers/ProviderRegistry.ts`（静态） | `createExecutionBackend`、`getCapabilities`、`getModelPolicy`、`getChatUIConfig` 等 |

`main.ts` 组装流程：`patchSetMaxListenersForElectron()`（SDK import 前）→ `import './providers'` → `loadApplication()` → 构造 `ClaudianProviderHost` / `ClaudianFeatureHost` / `ClaudianChatFeatureHost` → `SessionRuntimePool` / `SessionMessagingService` / `ConversationLifecycle`。

## 5. 核心功能实现定位

| 功能 | 主要实现位置 |
|---|---|
| **LLM/agent 交互** | `providers/*/execution/*ExecutionBackend.ts` + `*ExecutionSession.ts`；中性层 `core/execution/ProviderExecutionBackend.ts`；UI 驱动 `features/chat/execution/ChatExecutionCoordinator.ts` |
| **启动组装** | `src/main.ts` + `src/app/startup/ApplicationStartup.ts` + `src/composition/*` |
| **会话管理** | `app/conversations/ConversationRepository.ts`（权威）、`ConversationService.ts`、`SessionMetadataLoader.ts`；UI 侧 `features/chat/session-manager/*`、`features/chat/sessions/SessionRuntimePool.ts` |
| **工具/命令** | provider 归一化 `providers/*/normalization/`；中性工具层 `core/tools/`；渲染 `features/chat/rendering/tools/*`；斜杠命令 `core/commands/slashCommand.ts`、`builtInCommands.ts`、`compactCommand.ts` |
| **UI/视图** | 主视图 `features/chat/ClaudianView.ts`；渲染 `features/chat/rendering/*`；Tab/布局 `features/chat/tabs/*`、`features/chat/view/DualPaneLayout.ts`；禅模式 `features/chat/zen/*` |
| **设置面板** | 外壳 `features/settings/ClaudianSettings.ts`；各 provider `providers/*/ui/*SettingsTab.ts`；共享控件 `shared/settings/*` |
| **权限/审批** | `core/security/approvalRules.ts`；provider 权限模式 `providers/*/permissionModes.ts`；交互 `features/chat/interactions/*` + `core/execution/ProviderInteractionPort.ts` |
| **跨会话消息 / session 工具** | `core/session-tools/LoopbackSessionToolServer.ts`（127.0.0.1 回环）、`sessionToolMCP.ts`、`invokeSessionTool.ts`；UI 侧 `features/chat/session-messaging/*` |
| **内联编辑** | `features/inline-edit/ui/InlineEditModal.ts`、`features/inline-edit/InlineEditSessionOwner.ts`；`core/auxiliary/InlineEditService.ts` |
| **标题生成** | `core/auxiliary/TitleGenerationService.ts` + `RoutedTitleGenerationService.ts` |
| **存储** | `app/storage/SessionStorage.ts`、`SharedStorageService.ts`、`core/storage/VaultFileAdapter.ts`；会话元数据 JSON 在 vault `.claudian/sessions/<deviceKey>/<id>.meta.json` |

## 6. 构建 / 测试 / 发布

**npm scripts（`package.json`）**：
| 命令 | 作用 |
|---|---|
| `npm run dev` | `build:css` + `node esbuild.config.mjs`（watch，inline sourcemap） |
| `npm run build` | `node scripts/build.mjs production`（CSS + esbuild 生产打包，minify） |
| `npm run build:css` | `scripts/build-css.mjs` 拼接 `src/style/**` → `styles.css` |
| `npm run typecheck` | `tsc --noEmit` |
| `npm run lint` / `lint:fix` | ESLint（`{src,tests}/**/*.ts` + manifest + package.json，`--max-warnings=0`） |
| `npm run test` | `scripts/run-tests.js`（Jest + 脚本/架构/配置检查） |
| `npm run test:unit` | `scripts/run-jest.js` |
| `npm run test:affected` | `scripts/run-affected-tests.mjs`（CI 依赖图选测） |
| `npm run test:architecture` | `node --test scripts/check-architecture-boundaries.test.mjs` |
| `npm run test:cross-platform` | 跨平台真实进程测试 |
| `npm run check:lockfile` | `bun install --frozen-lockfile` |
| `npm run check:open-handles` / `check:performance` | 句柄泄漏 / 启动性能检查 |
| `npm run version` | `scripts/sync-version.js` 同步 manifest 版本 |

**构建细节**（`esbuild.config.mjs`）：入口 `src/main.ts`；`external` 排除 obsidian/electron/codemirror/Node builtins；自定义插件补丁 Claude SDK 的 `import.meta.url`、剔除未用 peer（zod/MCP）、压缩静态资源、修正 renderer 不安全 timer、构建后拷贝 `main.js/manifest.json/styles.css` 到 `$OBSIDIAN_VAULT/.obsidian/plugins/claudian`；prod 时用 `runtimeDependencyParity.mjs` 校验依赖一致性。

**测试**（`jest.config.js`）：两个 project（`unit` / `integration`），`ts-jest` + `tsconfig.jest.json`；`setupFilesAfterEnv: tests/setupWindow.ts`；别名映射及 `@anthropic-ai/claude-agent-sdk`/`obsidian` 的 mock（`tests/__mocks__/`）。规模：unit 420 文件（~132k 行）、integration 126 文件（~38k 行）、helpers 33、fixtures 19；集成测试含大量 `.dom.test.ts`（Testing Library + jest-axe）。

**CI**（`.github/workflows/ci.yml`）：`workflow-lint`(actionlint) / `lockfile`（bun+npm）/ `quality`（typecheck+lint+`npm audit`）/ `diff-hygiene` / `dependency-review` / `test-scope`（`ciTestSelection.mjs` 选测）/ `selected-tests` / `test`（聚合门禁）/ `cross-platform-smoke`（macOS/Windows 真实进程）/ `build-artifacts`（build + `check:performance`）/ `build`。`release.yml`（tag 触发）：跑 `ci.yml` → attest + changelog + 发布 `main.js/manifest.json/styles.css`。`nightly.yml`：全平台定时回归。

## 7. 借鉴 claudian 时最有价值的文件/目录

**已存在的架构图（本地分析产物，最省时）** `.context/architecture/`：
- `01-overview.puml`（分层架构，含 OpenCode/Pi 代表 + 存储边界）+ `Claudian-Architecture.svg`
- `02-opencode-v2-sequence.puml`（OpenCode v2 一次对话：spawn `opencode serve --stdio` → SSE → 渲染 → 审批）
- `03-pi-sequence.puml`（Pi 一次对话：JSONL over stdio）
- `04-session-storage.puml`（会话/存储与 `providerState` 边界）
- `05-dependency-direction.puml`（依赖方向硬约束）

**权威设计/约束文档（必读，从宽到窄）**：
- `AGENTS.md`（顶层：加载/验证流程、架构约束、命名约定、回归验证）
- `src/core/AGENTS.md`（provider 生命周期、提示词一致性、模型/持久化策略）
- `src/app/AGENTS.md`（会话权威、模型恢复、linked content）
- `src/features/chat/AGENTS.md`、`src/features/chat/{tabs,side-chat,zen}/AGENTS.md`
- `src/providers/{claude,codex,opencode,pi,grok,deepseek}/AGENTS.md`（各 provider 原生协议与归一化细节）
- `src/features/agent-skills/AGENTS.md`、`src/style/AGENTS.md`、`scripts/AGENTS.md`、`tests/AGENTS.md`

**关键契约与类型定义**：
- `src/core/execution/index.ts`（execution 事件/会话/交互类型出口）
- `src/core/providers/ProviderRegistry.ts` / `ProviderHost.ts` / `types.ts`
- `src/features/FeatureHost.ts` / `src/features/chat/ChatFeatureHost.ts`
- `src/core/types/index.ts` + `settings.ts`（`ClaudianSettings`、`Conversation`、`SlashCommand`）
- `src/providers/claude/registration.ts`（`ProviderModule` 契约完整示例）

**示例实现（跨 provider 对照）**：
- 会话池与运行时组装：`features/chat/sessions/{SessionRuntimePool,createChatSessionRuntime,ChatSessionRuntime}.ts`
- 执行协调（全局最大文件，事件/交互/流）：`features/chat/execution/ChatExecutionCoordinator.ts`
- 版本分发范式：`providers/opencode/execution/OpencodeSessionKernel.ts`（按原生版本选 HTTP v2 / ACP v1）
- 共享协议机制复用：`providers/acp/*`
- 回环 session 工具与跨会话消息：`core/session-tools/*` + `features/chat/session-messaging/*`

**自动化校验脚本（借鉴工程规范）**：`scripts/check-architecture-boundaries.test.mjs`、`ciTestSelection.mjs`、`check-startup-performance.mjs`、`check-open-handles.mjs`、`runtimeDependencyParity.mjs`。

## 8. 数据存储与外部协议速查

- **会话元数据**：vault `.claudian/sessions/<deviceKey>/<id>.meta.json`（仅 id/providerId/title/sessionId/selectedModel/providerState(不透明)/usage/linkedContentPath/resumeAtMessageId/lastActivityAt，不含消息体/base64）。
- **外部进程协议**：
  - Claude → Claude Agent SDK
  - Codex → app-server JSON-RPC
  - OpenCode → `opencode serve --stdio` HTTP+SSE（v2）/ ACP（v1）
  - Pi → `pi --mode rpc` JSONL over stdio
  - Grok → ACP
  - DeepSeek → native Remote（单 `DeepSeekHost` 进程）
- **Session 工具回环**：`127.0.0.1`，拒绝浏览器 `Origin` 与外部 `Host`，身份仅通过不可猜测可撤销的 route id 承载。

## 9. 快速定位速查

| 想找什么 | 去哪里 |
|---|---|
| 插件主入口/装配 | `src/main.ts` + `src/app/startup/ApplicationStartup.ts` + `src/composition/*` |
| provider 注册与契约 | `src/providers/index.ts` + `src/core/providers/ProviderRegistry.ts` + `providers/*/registration.ts` |
| provider 实现（某家） | `src/providers/{claude,codex,opencode,pi,grok,deepseek}/` |
| 共享 ACP 机制 | `src/providers/acp/*` |
| 执行/事件/会话运行时 | `src/core/execution/*` + `src/features/chat/execution/ChatExecutionCoordinator.ts` |
| 会话权威 | `src/app/conversations/ConversationRepository.ts` |
| 聊天 UI/视图 | `src/features/chat/ClaudianView.ts` + `chat/rendering/*` + `chat/tabs/*` |
| 设置面板 | `src/features/settings/ClaudianSettings.ts` + `providers/*/ui/*SettingsTab.ts` |
| 工具展示/归一化 | `src/core/tools/*` + `features/chat/rendering/tools/*` + `providers/*/normalization/*` |
| 跨会话消息/session 工具 | `src/core/session-tools/*` + `src/features/chat/session-messaging/*` |
| 架构约束/分层 | `AGENTS.md` + `.context/architecture/*.puml` + `scripts/check-architecture-boundaries.test.mjs` |
| 构建脚本 | `esbuild.config.mjs` + `scripts/build.mjs` + `scripts/build-css.mjs` |
