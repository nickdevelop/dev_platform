# dev_platform × claudian 架构重构计划（待审批）

> 目标：把 dev_platform 的目录与分层对齐 claudian 的五层架构，前端建立 features/shared/composition 结构并拆分巨型组件。
> 状态：**计划阶段，未获审批，不得执行任何代码变更。**
> 依据：`claudian_source.md`（claudian 2.3.21 架构分析）、`docs/architecture/backend-boundaries.md`（现有边界规则）。

## 0. 已确认的决策

| 决策点 | 结论 |
|---|---|
| 后端 Go 目录 | 采用 claudian 同构命名（core / app / providers），package 名保持不变 |
| 前端重构深度 | 目录分层 + 拆分 `SessionChatPanel.vue` / `MessageView.vue` / `stores/sessions.ts` |
| 遗留耦合范围 | 仅清理命名类耦合（`handlers_opencode.go`、`api/opencode.ts`、`stores/opencodeRaw.ts`）；resourcesync 合并进 Descriptor、独立架构守护脚本**不在本次范围** |

## 1. 架构差异结论

- dev_platform 后端的契约/适配器/通用层思想已与 claudian 等价：`internal/agents` 契约包（`contracts.go`、`extensions.go`、`registry.go`、`events.go`…）、`AgentAdapter` 显式能力声明（`Capabilities`）、可扩展接口（`PermissionReplier`/`RawEventOpener`/`HistoryReader`/`ServiceController`）、组合根 `checkBackendWiring` 双向校验、守卫测试。
- 三处实质差距：
  1. 目录名没有可视化对应到五层（契约包 `internal/agents` 下混入了领域服务子包 `instances`；领域服务散在 internal 顶层）。
  2. **前端零 feature 分层**：26 个组件平铺 `web/src/components/`，`SessionChatPanel.vue` 1642 行、`MessageView.vue` 1278 行、`stores/sessions.ts` 1021 行把 execution/state 揉在一起；无宿主窄接口（composition）。
  3. 命名类耦合：`sessions/handlers_opencode.go`、`web/src/api/opencode.ts`、`web/src/stores/opencodeRaw.ts`。
- 已核实：`internal/resourcesync` 不 import 具体后端包（文档所述耦合在装配层面）；前端组件无后端 ID 字面量（grep 命中基本为测试 fixture 与误报）。

## 2. 目标目录树

```
cmd/platform/               # ① 组合根（main.go + backend wiring）
internal/
  core/                     # ④ 契约层（provider 中性；禁止 import providers）
    agents/                 # ← internal/agents（不含 instances）
    platform/               # ← internal/platform（OS 抽象 ≈ claudian core/process）
  app/                      # ③ 领域服务层（provider 中性）
    sessions/               # ← internal/sessions
    instances/              # ← internal/agents/instances
    projects/ settings/ paths/ resources/ resourcesync/
    auth/ secrets/ database/ modelconfig/ modelprobe/ runtime/
  providers/                # ⑤ 异构适配层（实现 core 契约）
    opencode/               # ← internal/agents/opencode
    pi/                     # ← internal/agents/pi
  transport/http/           # HTTP 编排（router/static/accesslog）← internal/http
web/src/
  main.ts / App.vue / router.ts      # ① 组合根
  composition/                       # ① appHost 窄接口
  shared/
    components/                      # BackendTag、ModelSelect、EditDiffBody、EditToolBlock、
                                     # ToolOutput、RunningIndicator、CapabilityControls…
    （format.ts / markdown.ts / markdown-segments.ts / plantuml/ / theme/ / fonts/）
  features/
    chat/
      ChatPanel.vue                  # ← SessionChatPanel 拆分入口
      execution/                     # ← stores/sessions.ts 的事件消费/turn 编排
      rendering/                     # ← MessageView 拆分：消息/工具块/Diff/Markdown
      composer/                      # ← 输入区 + NewSessionPanel
      interactions/                  # ← QuestionForm、QuestionQueue
      state/                         # ← chat 专属 store
    workspace/                       # ← AppShell、SessionTabs、SessionList、EmptyWorkspace、
                                     #    BackendDiagnostics、CategoryRail、CapabilityControls?
    projects/                        # ← ProjectTree、ProjectDialog、ProjectManageDialog、
                                     #    ProjectSettingsPanel、ProjectDialog
    settings/                        # ← SettingsView + settings/{AgentSettings,ModelSettings,
                                     #    AppearanceSettings}
    resources/                       # ← ResourcesSection + resources/*
  api/                               # 保留（HTTP 客户端）
  stores/                            # 保留 app 级：agents/projects/settings/auth/models/
                                     #    resources/openTabs；sessions 拆入 chat/state
  tests/ e2e/                        # 测试随引用路径同步更新
```

依赖方向（与 claudian 相同，由守卫执行）：

```
① 组合根 → 任意层      ② features → composition 接口 + app 层状态 → core
③ app → core           ④ core → 不依赖任何下层实现      ⑤ providers → 实现 core，被组合根注册
禁止：core → providers；app/features → providers（transport 只走 core 契约）
```

## 3. 修改点清单

| # | 修改点 | 风险 |
|---|---|---|
| R1 | 后端五层目录重组（mv + import 全局替换；package 名不变） | 低 |
| R2 | 守卫测试路径更新；`AGENTS.md`、`docs/architecture/backend-boundaries.md` 改写为五层对照版 | 低 |
| R3 | 命名耦合清理：`handlers_opencode.go → handlers_rawevents.go`、`StreamOpencodeRaw → StreamRawEvents`、`ListOpencodePermissions → ListRawPermissions`；前端 `api/opencode.ts → api/rawEvents.ts`、`stores/opencodeRaw.ts → stores/rawEvents.ts`。HTTP 路径与响应契约不变 | 低 |
| R4 | 前端 features/shared/composition 归位（纯搬移） | 低 |
| R5 | 拆分 `SessionChatPanel.vue`：execution/rendering/composer 子目录 + composables | 中 |
| R6 | 拆分 `MessageView.vue`：rendering 子模块收编工具块/Diff/QuestionQueue | 中 |
| R7 | 拆分 `stores/sessions.ts`：`features/chat/state/` + `features/chat/execution/` | 中 |
| R8 | 收尾：全量 e2e；文档更新 | 低 |

**明确不做**：resourcesync 适配器并入 Descriptor；新建独立架构守护脚本；后端函数级重写；行为/接口变更。

## 4. 阶段计划

| Phase | 内容 | 验证门 |
|---|---|---|
| P0 基线 | 新分支；`make verify` + e2e 绿色基线记录 | 基线全绿 |
| P1 后端重组 | R1（~15 个包目录 mv、import 替换）+ R2（守卫、文档路径） | vet + Go test 全绿 |
| P2 命名清理 | R3（前后端四文件 + 引用） | verify 全绿 |
| P3 前端分层 | R4 纯搬移（绿测推进）→ R5 → R6 → R7，每项独立提交 | 单测 + e2e |
| P4 收尾 | R8：e2e 全量、文档改写、AGENTS.md 更新 | 全绿 |

执行原则：**先搬后拆、每步保绿**；R5/R6/R7 各自独立提交，任一步失败可单独回滚；HTTP 对外契约零变更。

## 5. 任务勾销表（执行时维护）

- [x] P0 基线（分支 `refactor/claudian-architecture`；`make verify` 全绿；e2e 基线：10 通过 / 7 存量失败——layout×2、plantuml、session-history×2、sessions、agent-service，均为 selector 歧义等存量缺陷，重构期间只许减不许增）
- [x] P1 后端五层重组（R1、R2）
- [x] P2 命名耦合清理（R3）— 实际落地名：后端 `handlers_interactions.go` / `service_interactions.go`、`RegisterInteractionRoutes`、`ListPermissionRequests` / `ReplyPermissionRequest` / `ListForms` / `ReplyForm` / `StreamRawEvents`；前端 `api/interactions.ts` / `stores/interactions.ts` / `useInteractionsStore`、DTO `PermissionRequest` / `FormInfo`；openapi.yaml 同步。对外 URL `/opencode/…` 前缀保留，已记入 backend-boundaries.md 剩余耦合表
- [ ] P2 命名耦合清理（R3）
- [x] P3 前端归位（R4）
- [x] P3 拆分 SessionChatPanel（R5）— 阶段收口：1642→1304 行，subdir 落位
  `chat/pane/useActivityPane`（折叠/宽度/拖拽）、`chat/interactions/useSessionInteractions`（审批/表单/轮询）、
  `chat/composer/useComposerContext`（模型选择/工作目录/context 仪表）；事件消费等 execution 机制在
  `chat/execution/SessionEventStream`（见 R7）。**偏差**：turn 编排（open/send/stop）与子代理弹窗留在面板——
  与 attempt 的三者耦合深、独立价值低，继续拆为低边际收益。
- [x] P3 拆分 MessageView（R6）— 渲染纯逻辑迁入 `chat/rendering/logModel.ts`（约 430 行，provider 中性），
  组件 1278→981 行，仅留 display 模式/手动展开/扁平折叠与模板。
- [x] P3 拆分 stores/sessions（R7）— 1021→741 行：`chat/state/sessionLog.ts`（类型+纯推导）+
  `chat/execution/SessionEventStream.ts`（SSE 订阅/补修/事件路由，借 StreamStore 结构体解循环依赖），
  store 保留薄委托，公开 API 不变。
- [x] P4 收尾与文档（R8）— **偏差**：不新建 `web/src/composition/`：Vue 的组合根职责由 App.vue/router.ts +
  pinia（app 级 stores）承担，空目录是不必要的 ceremony（claudian 需要 FeatureHost/ChatFeatureHost 窄接口
  是因为 Obsidian 插件没有框架级 store；dev_platform 的 pinia stores 即 app 层的中性状态 seam）。
  组合根对照表固定写入 AGENTS.md 首节。
- [x] P4 e2e 回归 — 重构后全量 e2e：10 通过 / 7 失败，失败清单与 P0 基线**逐条一致**（存量 selector
  缺陷），零新增回归。对外 HTTP 契约与行为均未变更。

## 6. 实际落地记录（执行中同步维护）

### P2 命名耦合清理（2026-10-15）

- 命名从"raw-events 单点"扩展为"交互路由整体中性化"（权限/表单/raw-events 本就同属能力门控的 interaction routes）：
  - 后端：`handlers_opencode.go → handlers_interactions.go`、`service_opencode.go → service_interactions.go`；
    `RegisterOpencodeRoutes → RegisterInteractionRoutes`、`ListOpencodePermissions → ListPermissionRequests`、
    `ReplyOpencodePermission → ReplyPermissionRequest`、`ListOpencodeForms → ListForms`、
    `ReplyOpencodeForm → ReplyForm`、`StreamOpencodeRaw → StreamRawEvents`。
  - 前端：`api/opencode.ts → api/interactions.ts`、`stores/opencodeRaw.ts → stores/interactions.ts`（store id
    `interactions`）、`useOpencodeRawStore → useInteractionsStore`；DTO `OpencodePermission/OpencodeForm →
    PermissionRequest/FormInfo`（镜像中性契约）；`tests/opencode-raw.spec.ts → tests/interactions.spec.ts`。
  - 文档：`docs/api/openapi.yaml` 的 operationId/DTO 名/summary 同步；协议事实描述保留 opencode 字样。
- 对外 URL `/projects/{id}/opencode/sessions/{sid}/...` 前缀**未改**（对外契约），记入 backend-boundaries.md 剩余耦合表，安排至 API 版本变更时处理。
- 后端白名单守卫 `TestRealOpencodeAdapterSatisfiesService` 保留原名（针对真实 opencode 适配器，属于异构事实）。
