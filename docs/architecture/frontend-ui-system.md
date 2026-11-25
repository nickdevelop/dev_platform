# dev_platform 前端 UI 规范 v2：Element Plus 基座

> **本文件是前端 UI 的唯一权威规范（v2）。**
> [`design-system.md`](./design-system.md)（v1 · Data Terminal 视觉附录）继续有效：其色彩预算、字阶阶梯、纯净矢量图标、动效预算与工作台布局骨架**照旧约束本平台**；凡本文与 v1 冲突之处（组件密度、静态阴影的例外范围、组件级样式覆盖方式）**以本文为准**。
> 本文同时废止两条旧约定：**「全面废除模态弹窗」（见 `PROJECT.md` §一.4 组织中心段，已同步改写）**与「UI 由 `shared/ui` 自绘原语承担」。

---

## 一、定位：库做基座，token 做皮肤

平台不是普通 SaaS 页面，而是长会话、高强度的 AI Coding Agent 工作台（见 v1 的 Data Terminal 定位）。引入 Element Plus 的目的是**消除自绘控件的重复成本与可达性缺陷**（焦点管理、键盘导航、弹层定位、表单校验、空态与 loading 态），而不是换一套视觉。

一句话原则：**组件来自 Element Plus，皮肤只来自平台 token。** 界面上任何一处颜色、圆角、字号、控件高度、阴影或动效时长，都必须能追溯到 `--color-* / --radius / --control-height / --text-size-* / --font-*`。

判定一个 `el-*` 用法是否合规，只需回答两个问题：

1. 它的观感是否完全由 `web/src/theme/element-tokens.css` 控制（组件内没有夹带颜色/圆角/像素高度）？
2. 它是否出现在选型对照表（§五）允许的范围内？

---

## 二、依赖与引入契约

| 项目 | 契约 | 落点 |
|---|---|---|
| 组件解析 | `unplugin-vue-components` + `ElementPlusResolver({ noStyles: true })`，模板里写 `<el-xxx>` 即自动按需引入 **JS**；`dirs: []`，本地组件一律保持显式 import，禁止隐式全局注册 | `web/vite.config.ts` |
| 组件类型 | 解析器生成的 `web/src/types/components.d.ts` **必须提交**，否则 CI 里 `vue-tsc` 无法识别 `<el-*>` | `web/src/types/` |
| 样式 | **单文件全量**：`import 'element-plus/dist/index.css'`（+47 kB gzip）。JS 仍是按需 tree-shaken，只有 CSS 全量 | `web/src/main.ts` |
| 样式顺序 | 固定三段：`element-plus/dist/index.css` → `./style.css`（Tailwind + 平台 token）→ `./theme/element-tokens.css`（覆盖层，最后） | `web/src/main.ts` |
| 为什么全量 CSS | 函数式 API（`ElMessage` / `ElMessageBox` / `ElLoading`）不在模板解析器的射程内，按组件注入样式会让它们**裸奔无样式**；单文件同时让层叠顺序变成确定性事实 | 见 `main.ts` 注释 |
| 中文语料 | `<el-config-provider :locale="zhCn">` 包住 `RouterView`。已验证：表格空态「暂无数据」、`ElMessageBox` 的取消/确定均随其生效 | `web/src/App.vue` |
| 深浅色 | **不引入** `element-plus/theme-chalk/dark/css-vars.css`。深色预设只是普通调色板，其 `--color-*` 本身是暗的。UA 附件（滚动条、原生控件）由 `applyAppearance` 写 `root.style.colorScheme` 切换，**不要**用 `dark` class（会一次性唤醒 280 处沉睡的 `dark:` Tailwind 工具类） | `web/src/theme/apply.ts` |
| 测试 | `test.server.deps.inline: ['element-plus']`（否则 Vitest 把 `.css` 交给 Node 报错）；`src/tests/setup/element-plus.ts` 提供 `ResizeObserver` 桩 | `web/vite.config.ts` |

体积基线（`npm run build`，2025-07）：接入前 JS 1,636 kB / CSS 375 kB（gzip 459 / 80）；接入后 JS 1,646 kB / CSS 746 kB（gzip 463 / 130）。JS 增量 3.8 kB gzip，CSS 增量 47 kB gzip，全部来自样式单文件。

---

## 三、Token 契约（唯一覆盖点）

**所有对 Element Plus 的样式干预只允许写在 `web/src/theme/element-tokens.css`。** 业务组件的 `<style>` 里出现 `--el-*`、`.el-*` 即为违规（守卫 `ui-element-boundary.spec.ts` 执行）。

### 1. 三种选择器形状（用错等于没写）

| 目标 | 写法 | 原因 |
|---|---|---|
| 重指向库在 `:root` 上声明的变量 | `html:root { --el-color-primary: … }` | `:root` 是 (0,1,0)，`html:root` 是 (0,1,1)，谁后加载都赢 |
| 重指向库**作用域到组件自身**的变量（如 `--el-segmented-item-selected-bg-color`、`--el-messagebox-border-radius`） | `html .el-segmented { --el-…: … }` | 继承变量取「最近的声明祖先」，写在根元素上永远传不进子节点；必须打在同一元素上并压过库的类选择器 |
| 覆盖库的组件规则 | `html .el-button { height: … }` | 裸 `.el-button` 与库同权重，会因源序失效 |

### 2. 派生色阶约定

库把 `light-3/5/7/8/9` 混向白色；平台**混向页面底色** `var(--color-bg)`，同一行声明在浅色预设与深色预设上都成立。压暗步骤（`dark-2`）优先用平台自带的 `--color-primary-hover`，没有对应 token 时才混向 `#000`。
`light-9 / light-8` 直接映射到平台的 `*-soft / *-border`（库把它们用作 alert/tag/message 的底色与描边，语义完全对齐）。

### 3. 禁止

- 禁止在 token 文件之外的地方发明 `--el-*` 变量或写入 hex 色值；
- 禁止用 `!important` 解决层叠问题，先修选择器形状；
- 禁止引入 `theme-chalk/dark/css-vars.css`；
- 禁止把 `--el-*` 当布局工具用（间距、宽度、排版仍走 Tailwind 原子类与平台 token）。

已验证的坑（不要重复踩）：`--el-border-color-strong` **不存在**，只有 `-dark / -darker`；`el-button` 的高度、`el-select__wrapper` 的 min-height、`el-form-item__label` 的 40px 是硬编码常量，不吃 `--el-component-size`；`el-segmented` 的滑块（`.el-segmented__item-selected`）是选项的**兄弟节点**而非子节点。`theme-tokens.spec.ts` 会在源码层拦截未定义变量。

---

## 四、密度、尺寸与布局契约

平台外壳是 4px 栅格 + 28px 控件高度，比 Element Plus 默认密一档。

| 档位 | 平台 token | 库默认 | 处置 |
|---|---|---|---|
| small | 24px | 24px | 直接可用（`size="small"`） |
| default | `--control-height` = 28px | 32px | **由 token 文件的「density bridge」段桥接** |
| large | 32px | 40px | 同上；large 仅用于独立大表单，禁止塞进面板 |

- 常规密度：面板内工具条、表格行内操作、筛选器一律 `size="small"`；弹窗/抽屉里的独立表单用 default。
- 禁止在业务组件里写 `height`、`padding` 去"微调"某个控件的高度——需要改高度就改 density bridge。
- 圆角：`--el-border-radius-base` = `--radius` = 3px；`round` 保留全圆（只服务于开关与状态小胶囊，符合 v1 的唯一例外）。
- 阴影：只有脱离文档流的部件（dialog / message-box / dropdown / popover / message）允许边界阴影，形式固定为 `0 0 0 1px 边线 + 短距投影`；表格、表单、卡片一律纯平。
- 动效：`--el-transition-duration` = 0.12s，且 `--el-transition-all` 已被重写为显式属性列表（库默认动 `all`，会连带宽与高度）。**注意 `.el-button` 硬编码 `transition: all .1s`、不读该变量**，token 层为它另写了具名列表；实心按钮 hover 上浮 1px、按下回位（`prefers-reduced-motion: reduce` 下关闭），文字链与 `text` 按钮不参与位移（它们坐在 34px 行高里，位移会被读成抖动）。
- 数字：表格、分页、下拉选项一律 `font-variant-numeric: tabular-nums`（token 层）；库不带这个属性，缺了它日期与计数会在行间跳动。
- 行内操作的可点区：`.el-button.is-link / .is-text` 库默认 `padding: 2px`（命中区就是字形本身），token 层给到 24px 档命中区 + hover 中性底；并且 `.el-table .el-button + .el-button { margin-left: 0 }`——页面用 flex `gap` 自控间距，库的兄弟外边距叠加上去会让三个操作在 168px 列里撑到 183px 被裁掉。
- 时间列一律 `YYYY-MM-DD HH:mm` 手工拼接，**不要用 `toLocaleString('zh-CN')`**：它渲染成 `2026-11/03 10:29`，在右对齐等宽列里读起来像两种格式粘在一起。

### 布局契约：单列表页与弹窗

**单列表页 = 页头 + 工具条 + 铺满的表格**（`features/organization/*` 四页即范式）：

| 区块 | 内容 | 备注 |
|---|---|---|
| 页头 | 标题 + 一句话职责说明 | 不放计数芯片（数量在枚举下拉的选项标签里），也不放操作按钮 |
| 工具条 | 左：全局动作按钮（新建等）；右：搜索表单 | 统一用 `shared/components/ListToolbar.vue`，页面只填 `#actions` / `#filters` 两个插槽 |
| 主体 | `el-table`（`height="100%"`）占满剩余高度 | 表头与操作列固定，长文本列 `show-overflow-tooltip` |

页头与节奏的唯一实现是 `shared/styles/list-page.css`：`.list-page-header / .list-page-title / .list-page-desc / .list-page-toolbar / .list-page-body / .list-page-filter`。标题字号走 `--text-size-20`，**不要在页面里写 Tailwind 的 `text-lg`**（=15.75px，不在 v1 字阶上，且四页各写一份必然漂移）。

### 列表的三态（没有行不等于没有话说）

空表格有三种成因，必须说得出区别，统一由 `shared/components/ListEmptyState.vue` 承担（放在 `el-table` 的 `#empty` 槽里）：

| 状态 | 触发 | 给什么 |
|---|---|---|
| `loading` | 首批数据在途（store 的 `loading` 且无行） | **行形骨架**：按表格 34px 行高排布、左对齐、每行几条与列数对应的灰条。库默认的 `:rows` 骨架是一段篇文章的形状，读起来是「有文章要来」而不是「有行要来」 |
| `filtered` | 关键词搜不到 | 复述关键词 + 「重置筛选」退路 |
| `empty` | 真的还没有 | 与本页导航**同一枚**矢量图标作视觉锚 + 一句话职责 + 主动按钮（默认插槽） |

- **枚举筛选单独命中空档不算「搜不到」**：「这个引擎下还没有人」的退路是新建（弹窗带着该类型打开），不是重置。
- 空态图标必须与左侧导航同枚（组织中心：EditPen / ChatDotRound / User / Connection），零 emoji。
- 空态在表格剩余高度里垂直居中，骨架贴顶（它标的是行将要落下的位置）；两者都由 token 层的 `.el-table__empty-block / __empty-text` 交出容器高度后由组件自己决定，库里那条 `width:50%; line-height:60px` 会把两行文字变成飘着的细条。
- **失败只报一次**：动作失败一律 `ElMessage.error`，不再额外挂 `UiAlert` 页面横幅——同一条错误既弹 toast 又在表格上方占一行，会把表格整个顶下去。

- 搜索表单的顺序固定：枚举筛选 → 关键词输入框 → **搜索** → **重置**，且以 `</form>` 包住，Enter 与搜索按钮同一条路径。
- **生效时机是契约的一部分**：枚举下拉（`el-select`）变更即时过滤；关键词是草稿，只有点搜索或回车才落到列表（半截词不该让表格闪）；用输入框的 ✕ 清空等同于提交一次空关键词；重置清空全部条件并立即回到全量。
- 枚举筛选一律 `el-select`，不再用 `el-segmented`（选项会随数据增长，分段控件扛不住）；`el-segmented` 只留给弹窗内二选一的小开关（如"引用提示词库 / 自定义人设"）。

**弹窗一律垂直居中、内部滚动、底栏固定**：

- 居中由 `App.vue` 的 `el-config-provider` 全局开启（`:dialog="{ alignCenter: true }"`，库会把 overlay 变 flex + `margin: auto`）；**不要在页面里逐个写 `align-center` 或 `top`**。
- 高度上限与内部滚动在 token 层：弹窗按列排布、`max-height: calc(100vh - 48px)`，`__body` 自己滚（`overscroll-behavior: contain`），因此头部与底栏（取消 / 保存）永远在视野内——库默认不限制弹窗高度，长表单会把按钮顶出屏幕。
- 二级弹窗（如团队编制勾选）同样继承该全局配置，不要单独设置。

---

## 五、组件选型对照表

新增或整改界面时按此表选取，不要在同一条链路上混用两套。

| 场景 | 选用 | 备注 |
|---|---|---|
| 主按钮 / 次按钮 / 文字链 | `el-button`（`type` / `link` / `text`） | 图标一律 `@element-plus/icons-vue`（`:icon` 或 `<el-icon>`），禁 emoji；行内操作用 `link` |
| 列表分页 | `el-pagination` | 当作工具条部件而非浮动控件：等宽小字、24px 方块，当前页实心 |
| 文本/数字/多行输入 | `el-input`（`type=number` 用 `el-input-number`，多行 `type="textarea"`） | 不再自绘 `<input>` |
| 表单与校验 | `el-form` + `el-form-item` + `rules` | **校验反馈优先走行内 `prop` 错误**；保存成功/失败用 `ElMessage`；不再用散落的 `actionError` 字符串 |
| 单选下拉 | `el-select` + `el-option`（长列表加 `filterable`） | 取代 `UiSelectMenu` |
| 二选一/来源切换 | `el-radio-group`；多段筛选器用 `el-segmented` | 分段选中态是**贴边细线芯片**，不是主色实心块 |
| 开关 | `el-switch` | 取代 `sr-only peer` 自绘开关 |
| 多选（候选集小） | `el-checkbox-group` | 卡片式勾选（如团队编制专家）用 `el-checkbox` 包卡片，勾选后回显主面板 |
| 布尔徽标/状态 | `el-tag`（`size="small"`） | 颜色预算仍受 v1 的 3% 约束 |
| 确认类弹窗 | `ElMessageBox.confirm` | 破坏性操作统一走它，不再自绘 `UiConfirmDialog` |
| 新增/编辑承载 | `shared/components/EntityFormDialog.vue`（包 `el-dialog`） | 统一头部（标题 + 一句话职责 + 记录号芯片）与底栏（快捷键提示 + 取消 / 保存 + Ctrl/Cmd+S）；分区骨架类见 `shared/styles/entity-editor.css`；复杂多分区表单可用 `el-drawer`，同一模块内保持一致 |
| 轻提示 | `ElMessage` | 结果与失败都只走它一处；列表页不再挂 `UiAlert` 错误横幅（组织中心已退役，`Ui*` 棘轮随之降到 745） |
| 列表三态 | `shared/components/ListEmptyState.vue`（`loading` / `filtered` / `empty`） | 放进 `el-table` 的 `#empty` 槽；见 §四「列表的三态」。非表格区域的大空态才用 `el-empty` |
| 列表页页头与节奏 | `shared/styles/list-page.css`（`.list-page-*`） | 页头、工具条槽、表格容器、枚举筛选档位；只放平台自己的类与 token，禁止 `.el-*` / `--el-*` |
| 列表页工具条 | `shared/components/ListToolbar.vue`（左动作 / 右搜索表单 + 搜索 + 重置） | 见 §四 布局契约；枚举筛选放 `#filters` 插槽用 `el-select` |
| 弹层/浮层 | `el-dropdown` / `el-popover` / `el-tooltip` | 禁止 `backdrop-filter: blur`（v1 禁令继续有效，遮罩为中性半透明）；库的 `is-dark` 变体拿文字色当底色与描边，已统一改回平台浮层表面 + 强描边（tooltip 最大宽度 720px），均入 token 层 |
| 图标 | `@element-plus/icons-vue`（`<el-icon>` 包裹） | 仍受 v1「零 Emoji、纯矢量细线条」约束；库内没有的业务符号继续用现有 1.5–1.8px 手绘 SVG |
| 日期/时间 | `el-date-picker` / `el-time-picker` | 新增即用，不再排期 |
| 侧栏导航 | `el-menu`（`router` 模式，见 §八） | 子级用 `el-sub-menu` 预留；条目高度 / hover 中性化 / 左缘 accent 标尺三项覆盖已入 token 层 |

**共享编辑器外壳（`shared/components/EntityFormDialog.vue`）**：所有「新增 / 编辑 X」表单统一用它，不要各页自己搭 `el-dialog` 头部与底栏。它固定：标题 + 一句话职责说明 + 记录号芯片、底栏的快捷键提示（Ctrl+S 保存 · Esc 关闭）与「取消 / 保存」按钮、`Ctrl/Cmd+S` 提交、`saving` 驱动的 loading。分区骨架（01 / 02 编号、hairline 分隔、字段下的说明文字、下拉项右侧次级说明、提示词预览块、开关行、两列栅格）放在 `shared/styles/entity-editor.css`——插槽内容在调用方作用域编译，组件的 scoped 样式够不着，所以与 `sidebar.css` 一样作为共享样式表引入；该文件只允许出现平台自己的类与 token，禁止 `.el-*` / `--el-*`。

**函数式 API 的引入方式**：`ElMessage / ElMessageBox / ElLoading` 直接 `import { ElMessage } from 'element-plus'`（样式已随全量 CSS 到位）。模板里的 `<el-*>` 一律不手写 import——交给解析器。

---

## 六、`Ui*` 原语退役流程

`shared/ui` 的 15 个 atomic + 8 个 compound 组件**全部保留**，作为尚未迁移页面的现有契约。迁移节奏：

1. 改造某个页面时，**整页迁到 Element Plus**，禁止半页 `el-*` 半页 `Ui*`；
2. 某原语的引用数降到 0 后，在其 `index.ts` 条目上标注 `@deprecated`，下一个 PR 删除组件与其单测；
3. 守卫 `ui-element-boundary.spec.ts` 冻结 `features/**` 对 `Ui*` 的引用计数：**只准降不准升**（基线快照随迁移下调）；同时禁止在 `features/**` 出现 `--el-*` / `.el-*` 选择器。

新增页面（不存在迁移负担的）直接用 Element Plus，不得再新增 `Ui*` 用法。

---

## 七、质量门禁（真实可执行）

```bash
cd web && npm run typecheck          # vue-tsc，含 <el-*> 模板类型
cd web && npx vitest run            # 单测 + 全部源码级守卫
cd web && npm run build              # 生产构建，关注 css/js 体积不要失控
```

守卫清单：

| 守卫 | 拦截内容 |
|---|---|
| `src/tests/theme-tokens.spec.ts` | 任何 `var(--x)` 必须有定义（含 `--el-*`）；禁硬编码 px 字号；冻结 4px/28px/3px 布局 token |
| `src/tests/ui-element-boundary.spec.ts` | `features/**` 不得出现 `--el-*`/`.el-*`；`Ui*` 引用数不得高于基线 |
| `src/tests/native-dialog-guard.spec.ts` | 禁止原生 `confirm/alert`（`ElMessageBox` 不受影响） |
| `src/tests/template-attribute-guard.spec.ts` | 模板属性名合法性（Electron 132 会抛错的写法） |

浏览器侧复核（每完成一个模块整改必做）：用真实浏览器在 **浅色 / 暖阳 / 极光（深色预设）** 三档下目视核对新界面，确认没有夹带的硬编码色与错位密度；截图对比优先，不接受"只看代码没问题"。

> v1 文档里写的 `design-taste-frontend/scripts/lint.mjs` 在本机与技能包中都不存在（技能包实际提供 `gate.mjs / consistency.mjs / copy-lint.mjs`，面向静态 HTML 页面而非本 SPA），其人工审查项已并入上表与 §一的两个问题。

---

## 八、迁移状态与待决事项

**已完成**：

1. 依赖与构建接入、`element-tokens.css` token 层、本规范与 AGENTS/PROJECT 同步。
2. 组织二级导航上 `el-menu`（router 模式）。**已知不一致**：其余五栏（项目 / 对话 / 任务 / 终端 / 设置）仍是 `sidebar.css` 卡片列表。待办：把五栏一并迁到 `el-menu`（需先解决 Zone A 动作区与 Zone B 搜索区的归属），随后评估从 `sidebar.css` 删除卡片段，避免两种侧栏长期并存。菜单三项覆盖（条目高度 46 / 34px、hover 中性化、库没有的左缘 2px accent 标尺）已在 `element-tokens.css` 的 menus 段；渲染层按 `children` 自动切换 `el-sub-menu`，日后加子菜单只改数据。
3. 组织中心四个子页全部迁完（菜单文案统一为「AI 提示词 / AI 助手 / AI 员工 / AI 团队」，「AI」与中文之间留半角空格）：
   - 列表：`el-table`（`height="100%"` 铺满主体），行内常驻开关 `el-switch`，行内操作 `el-button link`；
   - 工具条：统一 `shared/components/ListToolbar.vue`，枚举筛选 `el-select`（AI 助手＝常驻状态、AI 员工＝执行引擎、AI 提示词＝类型；AI 团队无枚举只留关键词），生效时机见 §四 布局契约；
   - 编辑器：`AssistantEditPanel` / `EmployeeEditPanel` / `TeamEditPanel` → `*EditDialog`，共用 `shared/components/EntityFormDialog.vue` 外壳；AI 提示词的 `PromptManageDialog` 同批迁 `el-dialog` + `el-form`；
   - 交互：删除 → `ElMessageBox`（`confirmButtonType: 'danger'`），结果反馈 → `ElMessage`，AI 团队专家编制 → 二级 `el-checkbox-group` 卡片网格勾选回显。
4. §四 布局契约落地：页头去计数芯片、工具条左动作右搜索、表格铺满；弹窗全局垂直居中 + 内部滚动 + 底栏固定。
5. 组织中心反馈与质感层（只面向 ≥ 1280 的桌面宽度，窄屏不在本轮范围）：
   - **三态**：新增 `shared/components/ListEmptyState.vue`，四页的 `#empty` 槽不再各写一行灰字；loading 给行形骨架、搜不到给「重置筛选」、空库给导航同枚矢量图标 + 主动作；
   - **页头原语**：`shared/styles/list-page.css`（`.list-page-*`），页头标题从 Tailwind `text-lg`（15.75px，不在字阶上）收编到 `--text-size-20`，四页重复的 `.filter-select` / `.table-wrap` 一并删除；
   - **可反馈性**（均在 token 层）：行内 link/text 按钮从 `padding: 2px` 给到 24px 命中区与 hover 中性底；typed link 的 hover 不再把主色刷成灰色（改 `--el-button-hover-link-text-color` 按类型加深一档）；`.el-button` 的 `transition: all` 改具名列表；实心按钮 hover 上浮 / 按下回位；表格内取消库的兄弟 `margin-left`（否则三个操作在 168px 列里被裁）；
   - **数字与时间**：表格 / 分页 / 下拉开 `tabular-nums`；时间列改为手工拼接的 `YYYY-MM-DD HH:mm`；
   - `UiAlert` 页面错误横幅从组织中心退位（同一错误不再 toast + 横幅双报，横幅还会把表格顶下去），棘轮 750 → 745。

**待办**：

1. 其余模块（任务、设置、终端、工作区）按页面逐个迁移，每页一个 PR，随页更新本节清单；新页面直接按 §四 布局契约与「列表的三态」写（页头用 `.list-page-*`，空态用 `ListEmptyState`），不要再写页头计数芯片、散装筛选行与自绘空态。
2. 助手的预设头像仍是 emoji（源码里带 `taste-skill-ignore` 标记），与 v1「零 Emoji」相冲，且缺 emoji 字体的环境会显示成豆腐块；待办：换成 v1 的手绘 SVG 图标集，头像字段改存图标 key。

---

*本规范由平台前端工程团队维护。新增界面、组件或调整主题时必须严格执行；改变本文任何一条契约，请同时更新本文与 `AGENTS.md` 摘要。*
