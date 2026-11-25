# dev_platform 开发约定

## 项目全局理解与架构规范（PROJECT.md）

本项目的完整架构全景、技术栈、核心业务流程（会话与类型化轮次、交互授权流、终端与 PTY、孤儿自愈机制）、传输层安全规范及质量门禁请参见：

👉 [`PROJECT.md`](./PROJECT.md)

**强制维护约定**：
- 后续 AI 助手或开发人员在参与本项目时，**必须首先阅读并遵循 `PROJECT.md` 中的设计规范与架构约束**。
- **后续需求的新增或变化时，若涉及架构、业务流程、依赖、配置或接口的调整，必须在修改代码的同时同步对 `PROJECT.md` 文件进行更新维护，确保代码实现与架构描述严格一致**。

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

## 前端 UI 规范（Element Plus 基座，强制）

前端 UI 的唯一权威规范是 👉 [`docs/architecture/frontend-ui-system.md`](./docs/architecture/frontend-ui-system.md)（v2）。
`docs/architecture/design-system.md`（v1 · Data Terminal）降级为视觉附录：色彩预算、字阶、零 Emoji 矢量图标、
动效预算、工作台布局骨架继续有效；**弹窗策略与组件密度以 v2 为准**。改动任何界面前先读 v2。

硬性规则（守卫测试执行，违反即 CI 红）：

1. **组件库优先**：控件一律使用 Element Plus（`el-table / el-form / el-select / el-switch /
   el-segmented / el-dialog / el-input / ElMessage / ElMessageBox` 等，选型表见 v2 §五）。
   模板里 `<el-*>` 由 `unplugin-vue-components` 自动按需引入，**不要手写 import**；
   函数式 API（`ElMessage / ElMessageBox / ElLoading`）才从 `'element-plus'` 显式 import。
   新增页面禁止再写 `Ui*` 原语；改造页面必须**整页**迁移，不得半页 `el-*` 半页 `Ui*`。
2. **唯一覆盖点**：对 Element Plus 的任何样式干预（`--el-*` 变量、`.el-*` 选择器、密度桥接）
   只能写在 `web/src/theme/element-tokens.css`；业务组件的 `<style>` 里出现即违规
   （`web/src/tests/ui-element-boundary.spec.ts` 拦截，并冻结 `Ui*` 引用数——只准降不准升）。
3. **皮肤只来自 token**：颜色/圆角/字号/控件高度/阴影/时长必须可追溯到
   `--color-* / --radius / --control-height / --text-size-* / --font-*`；组件内出现 hex 色值、
   像素字号或手工微调控件高度即违规（`theme-tokens.spec.ts` 与 v2 §四密度表拦截）。
   控件档位固定 small 24 / default 28 / large 32：表格行内操作用 `size="small"`，列表页工具条（含 `ListToolbar`
   插槽内控件）与弹窗表单走 default 28 且**不写 `size`**——同一行工具条不许出现两种高度。
   字号阶梯：数据单元格继承 12px（不要写 `text-2xs` 自带小一档），表头 / 行内动作 / 表格内 chip 走 11px
   （chip 18px 高，不得大于数据字号），下拉框与分页条统一 12px（库在 `.el-select__wrapper` 硬编码 14px，
   由 token 层投影回 base）。
   表头 `font-weight: 600`。分格线只画三处：工具条与表格之间 `--color-border`、表头列之间
   `--color-border-strong`（最后一列不画）、数据行之间用库默认；**数据单元格不画竖线**。
3b. **chip 双轨**：枚举分类值（角色类 / 流程类…）用 `class="entity-chip entity-chip--<值>"`，走与皮肤无关的固定
   分类色轴 `--cat-hue-1…6`（**不写 `type`**）——语义色在预设之间会并色（`cool` / `emerald` 里 `success == primary`），
   给不了「不同即不同色」；状态值（启用 / 异常）才继续用语义 `type`。档位映射只在
   `web/src/shared/styles/entity-chip.css` 声明，新增枚举值只在那里加一行。
4. **引入方式不可变更**：解析器 `ElementPlusResolver({ noStyles: true })` + 全量单文件 CSS
   （`element-plus/dist/index.css`，因函数式 API 不在模板解析器射程内会裸奔无样式）；顺序固定
   `element-plus → style.css → theme/element-tokens.css`；`web/src/types/components.d.ts` 必须提交；
   **禁止**引入 `theme-chalk/dark/css-vars.css` 或在 `html` 上加 `dark` class（会唤醒 280 处沉睡的
   `dark:` 工具类），深色预设靠自身 palette + `root.style.colorScheme` 生效。
5. **列表页与弹窗布局契约**（v2 §四）：单列表页 = 页头（**只有**标题 + 一句话职责，不写计数芯片、不放工具条）
   + 主体三段：**工具条固定顶 / `el-table` 铺满中间 / `el-pagination` 固定底**，容器类用
   `web/src/shared/styles/list-page.css` 的（四页已统一，旧 `.list-page-toolbar` / `.list-page-body` 已删）—— `.list-page-panel / .list-panel-toolbar / .list-panel-table / .list-panel-footer`。
   工具条左＝全局动作、右＝搜索表单，顺序固定「枚举 `el-select` → 关键词 `el-input` → 搜索 → 重置」，
   统一用 `web/src/shared/components/ListToolbar.vue`；**紧凑度已定**：表单内控件间距 4px、枚举框 120px、
   关键词框 150px、控件高度统一 24px，placeholder 写短、搜索范围走 `hint`（title / aria-label），
   搜索 / 重置带前置矢量图标（放大镜只出现在搜索按钮上，关键词框不再自带 prefix 图标）。
   分页固定 `size="small"` + `background` + layout `total, sizes, prev, pager, next`，关键词或枚举筛选一变就回第一页。
   枚举筛选变更即时生效、关键词是草稿只在搜索/回车时生效。标题走 `--text-size-20`（不写 Tailwind `text-lg`）。
   没有行的三种成因必须说清：`el-table` 的 `#empty` 槽一律放
   `web/src/shared/components/ListEmptyState.vue`（`loading` 行形骨架 / `filtered` 给重置退路 /
   `empty` 给导航同枚矢量图标 + 主动作），枚举筛选命中空档不算「搜不到」；
   失败只报一次（只走 `ElMessage`，不再挂 `UiAlert` 页面横幅）。弹窗一律垂直居中（`App.vue` 的
   `el-config-provider` 上 `:dialog="{ alignCenter: true }"`，**不要在页面里逐个写 `align-center`/`top`**），
   高度上限与内部滚动、底栏固定由 token 层负责。底栏固定为 `取消`（关闭）/ `确认`（保存）两个词、不带图标、间距走库的默认值
   （`.entity-dialog-actions` 只排布不加 gap，否则与库的 `margin-left: 12px` 相加）；新建弹窗复用
   `shared/components/EntityFormDialog.vue`，不要另写底栏。
6. **中文语料**：全局 `<el-config-provider :locale="zhCn">`（`App.vue`）已生效于表格空态与
   `ElMessageBox` 按钮，不要在局部另设 locale。
7. **文档同步**：UI 规范任何一条契约变化，必须同时更新 v2 文档与本节摘要。

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
