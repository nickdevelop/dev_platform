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

- 常规密度：**表格行内操作**用 `size="small"`（命中区由 token 层收在 24px 档）；**列表页工具条**（`ListToolbar` 及其 `#actions` / `#filters` 插槽里的控件）走 default 28px，**不写 `size`**；弹窗 / 抽屉里的独立表单同样是 default。一行工具条上出现 24 与 28 两种高度即违规。
- 禁止在业务组件里写 `height`、`padding` 去"微调"某个控件的高度——需要改高度就改 density bridge。
- 圆角：`--el-border-radius-base` = `--radius` = 3px；`round` 保留全圆（只服务于开关与状态小胶囊，符合 v1 的唯一例外）。
- 阴影：只有脱离文档流的部件（dialog / message-box / dropdown / popover / message）允许边界阴影，形式固定为 `0 0 0 1px 边线 + 短距投影`；表格、表单、卡片一律纯平。
- 动效：`--el-transition-duration` = 0.12s，且 `--el-transition-all` 已被重写为显式属性列表（库默认动 `all`，会连带宽与高度）。**注意 `.el-button` 硬编码 `transition: all .1s`、不读该变量**，token 层为它另写了具名列表；实心按钮 hover 上浮 1px、按下回位（`prefers-reduced-motion: reduce` 下关闭），文字链与 `text` 按钮不参与位移（它们坐在 34px 行高里，位移会被读成抖动）。
- 数字：表格、分页、下拉选项一律 `font-variant-numeric: tabular-nums`（token 层）；库不带这个属性，缺了它日期与计数会在行间跳动。
- 行内操作的可点区：`.el-button.is-link / .is-text` 库默认 `padding: 2px`（命中区就是字形本身），token 层给到 24px 档命中区 + hover 中性底；并且 `.el-table .el-button + .el-button { margin-left: 0 }`——页面用 flex `gap` 自控间距，库的兄弟外边距叠加上去会让三个操作在 168px 列里撑到 183px 被裁掉。
- 时间列一律 `YYYY-MM-DD HH:mm` 手工拼接，**不要用 `toLocaleString('zh-CN')`**：它渲染成 `2026-11/03 10:29`，在右对齐等宽列里读起来像两种格式粘在一起。

### 布局契约：单列表页与弹窗

> **例外：设置中心画布不是单列表页。** 设置中心六张路由页走居中最宽 1080px 的**滚动画布**
> （`SettingsView.vue` 的 `.settings-canvas`），页头 + 若干分区卡，而不是铺满视口的三段式：
> 一张设置页经常并列多个主题（字体 / 本地宿主环境 / 远程服务器），没有一张工具条能兼带它们，
> 而画布自己滚动，给表格钉 `height="100%"` 会和滚动容器打架。所以设置页用
> `shared/styles/settings-page.css` 的 `.settings-page-*` / `.settings-panel*` 原语，表格
> 自然高度、**不分页**；页头右侧可以放页面级动作（与单列表页「页头不放按钮」相反）。
> 画布里的表格列宽预算见下文《设置中心画布的表格列宽预算》。


**单列表页 = 页头 + 主体三段**（`features/organization/prompts/OrgPromptsView.vue` 即范式）：

| 区块 | 内容 | 备注 |
|---|---|---|
| 页头 | 只有标题 + 一句话职责说明 | 不放计数芯片（数量在枚举下拉的选项标签里），不放操作按钮，也**不再放工具条** |
| 主体 · 工具条 | 左：全局动作按钮（新建等）；右：搜索表单 | **固定顶部**；统一用 `shared/components/ListToolbar.vue`，页面只填 `#actions` / `#filters` 两个插槽 |
| 主体 · 表格 | `el-table`（`height="100%"`）铺满中间 | 表头与操作列固定，长文本列 `show-overflow-tooltip`；正文自己滚 |
| 主体 · 分页 | `el-pagination` | **固定底部**，一条发丝上边与正文分开；无论多少行都在视野内，不会被表格顶走 |

三段容器的类在 `shared/styles/list-page.css`：`.list-page-panel`（flex column + `min-height: 0`）/ `.list-panel-toolbar` / `.list-panel-table` / `.list-panel-footer`。高度链不能断：`el-table` 只有拿到确定高度才会钉住表头，所以父级每一层都要 `min-height: 0`，表格用 `height="100%"` 接管中间段。组织四页已全部走三段式；旧的 `.list-page-toolbar`（工具条写在页头里）与无分页的 `.list-page-body` 随之删除，不要复活它们——同一件事有两种写法，下一个页面就会挑到旧的那种。

**分页约定**：`size="small"` + `background`（当前页实心，见 token 层 pagination 段）。`size` 必须写：token 层把页码方块钉在 24px，摘掉 `size` 后条数下拉会回到 28px，同一条横带出现两种高度，layout 固定 `total, sizes, prev, pager, next`（数据量小的列表不给 jumper），条数档位 10 / 20 / 50。**关键词或枚举筛选一变就回到第一页**，结果总数变小要钓回合法末页——否则用户会停在一张空页上以为被筛选滤空。

**工具条的紧凑度是契约的一部分**（数值即契约，四页共用一枚组件）：

| 项 | 值 | 说明 |
|---|---|---|
| 表单内控件间距 | **4px** | 筛选器、输入框、两个按钮是一件乐器，不是四件。库给相邻按钮的 `margin-left: 12px` 会叠在 gap 上（实测 16px），由 `ListToolbar` 自己的 `.toolbar-submit + .toolbar-submit` 清零——业务组件里不出现 `.el-*` |
| 枚举筛选框 | **120px** | `.list-page-filter`，与关键词框同档；标签最长就是「全部 (13)」这一类 |
| 关键词框 | **150px** | 是关键词框不是段落框：placeholder 写短（「搜索提示词…」），搜索范围的完整说法走 `hint` 属性（写进 `title` 与 `aria-label`） |
| 提交按钮 | 带前置矢量图标 | 搜索用 `Search`、重置用 `Refresh`：只有两个字会窄成一枚小方块。放大镜因此**只出现在搜索按钮上**，关键词框不再自带 prefix 图标（同排两个放大镜会读成两种搜索） |
| 控件高度 | 全部 28px | default 档（工具条上**不写 `size`**）；实测 select / input / 三枚按钮齐平，不要按页面单独调高某一件，也不要退回 small 档混出两种高度 |

**表格的字号阶梯：数据说话，其余标注**（实测值，改动只在 token 层）：

| 位置 | 字号 | 谁来管 |
|---|---|---|
| 数据单元格 | **12px**（`--el-font-size-base`） | 表格容器一条规则，单元格内容**继承**；等宽列只写 `font-mono`，**不要再写 `text-2xs`** 之类自带小一档——同一行的时间列比名称列小一号就是坏的 |
| 表头 | 11px | caption 档，比数据轻 |
| 行内动作（`el-button link`） | **11px** | 比数据低一档：`html .el-table .el-button.is-link` |
| 表格内 `el-tag` | **11px / 18px 高 / 0 6px** | chip 不许比它标注的那个词更响；库默认 20px 高 + 12px 字会压过正文 |
| 下拉框选中文字 | **12px** | 库在 `.el-select__wrapper` 上**硬编码 14px**（不读尺寸档），token 层统一投影成 base，所有尺寸档与分页条里的条数下拉一起生效 |
| 分页条 | **12px 一个字号** | 同一条横带里的 `共 N 条` / 页码 / 条数下拉必须同号，`--el-pagination-font-size` 走 base |

**表格的分格线只画三处**：

| 线 | 色档 | 为什么 |
|---|---|---|
| 工具条与表格之间 | `--color-border` | 操作区与数据区的区带线（旧结构里由页头的 `border-bottom` 顺带承担） |
| 表头列与列之间 | `--color-border-strong` | 只回答「哪一列从哪里开始」；比行分隔强一档，用同色实测几乎看不见等于没画 |
| 数据行之间 | `--color-border`（库默认） | 库自带 |

**数据单元格不画竖线**：竖线画进行间会把一行切成碎片，而横向连续正是扫读多列表格的依据。表头 `font-weight: 600`（库默认 400、平台原为 500，在 11px 的 caption 档上撑不住表头与正文的层级）。

**设置中心画布的表格列宽预算＝ 866px**（守卫 `src/tests/settings-table-budget.spec.ts`）：画布扣除
页面内边距、分区卡内边距与滞动条后，`el-table` 实测只有 **866px** 可用（1365×720、项目栏展开）。
`el-table` 不会换行：列宽一超预算就出横向滚动条，而 `fixed="right"` 的操作列会**盖在中间的列上面**
——实测现象是「更新时间列凭空消失」与「状态芯片被削一半」。它在真实浏览器里一眼可见，在单元测试
里完全隐形（因为标记全都在）。所以：给设置页表格加列之前，先把各列 `width` / `min-width` 加起来，
守卫会红。参考档位：服务代理八列 854px（130/84/150/130/76/64/120/100），其中时间列 120px + `show-overflow-tooltip`。

页头与节奏的唯一实现是 `shared/styles/list-page.css`：`.list-page-header / .list-page-title / .list-page-desc`（`.list-page-header / .list-page-title / .list-page-desc / .list-page-filter` + 三段式的 `.list-page-panel / .list-panel-toolbar / .list-panel-table / .list-panel-footer`）。标题字号走 `--text-size-20`，**不要在页面里写 Tailwind 的 `text-lg`**（=15.75px，不在 v1 字阶上，且四页各写一份必然漂移）。

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
- 底栏是**全平台同一对动作，同一份实现**：`取消`（关闭不保存）与 `确认`（保存并关闭）。要点：
  - **按钮名称就这两个字**。标题栏已经写了「新增 / 编辑」与记录号，按钮再复述「创建助手 / 保存修改 / 立即创建」是噪声；`EntityFormDialog` 的 `saveLabel` 默认就是 `确认`，只有动词真的携带信息时才覆盖。
  - **不带图标**。这两个动作在全平台恒定，图标零信息量，还会把两枚按钮撑成不同宽度。
  - **间距用库的默认值**。底栏容器 `.entity-dialog-actions` 只负责排布（flex + 居中），**不要在这里加 `gap`**：库给相邻按钮的 `margin-left: 12px` 会与 gap 相加（写 4px 实测变 16px），一对动作被拉开成两个模块。
  - 新建弹窗一律复用 `shared/components/EntityFormDialog.vue`，不要另写底栏。

---

## 五、组件选型对照表

新增或整改界面时按此表选取，不要在同一条链路上混用两套。

| 场景 | 选用 | 备注 |
|---|---|---|
| 主按钮 / 次按钮 / 文字链 | `el-button`（`type` / `link` / `text`） | 图标一律 `@element-plus/icons-vue`（`:icon` 或 `<el-icon>`），禁 emoji；行内操作用 `link` |
| 列表分页 | `el-pagination` | 当作工具条部件而非浮动控件：等宽数字、24px 方块、整条一个字号（12px），当前页实心 |
| 文本/数字/多行输入 | `el-input`（`type=number` 用 `el-input-number`，多行 `type="textarea"`） | 不再自绘 `<input>` |
| 表单与校验 | `el-form` + `el-form-item` + `rules` | **校验反馈优先走行内 `prop` 错误**；保存成功/失败用 `ElMessage`；不再用散落的 `actionError` 字符串 |
| 单选下拉 | `el-select` + `el-option`（长列表加 `filterable`） | 取代 `UiSelectMenu` |
| 二选一/来源切换 | `el-radio-group`；多段筛选器用 `el-segmented` | 分段选中态是**贴边细线芯片**，不是主色实心块 |
| 开关 | `el-switch` | 取代 `sr-only peer` 自绘开关 |
| 多选（候选集小） | `el-checkbox-group` | 卡片式勾选（如团队编制专家）用 `el-checkbox` 包卡片，勾选后回显主面板 |
| 布尔徽标/状态 | `el-tag`（`size="small"`） | 颜色预算仍受 v1 的 3% 约束；表格内的 chip 由 token 层收到 11px / 18px，不得大于数据字号；**枚举分类不能用 `type` 上色，见下一节 chip 双轨** |
| 确认类弹窗 | `ElMessageBox.confirm` | 破坏性操作统一走它，不再自绘 `UiConfirmDialog` |
| 新增/编辑承载 | `shared/components/EntityFormDialog.vue`（包 `el-dialog`） | 统一头部（标题 + 一句话职责 + 记录号芯片）与底栏（快捷键提示 + 取消 / 保存 + Ctrl/Cmd+S）；分区骨架类见 `shared/styles/entity-editor.css`；复杂多分区表单可用 `el-drawer`，同一模块内保持一致 |
| 轻提示 | `ElMessage` | 结果与失败都只走它一处；列表页不再挂 `UiAlert` 错误横幅（组织中心已退役，引擎中心同口径，`Ui*` 棘轮随之降到 630） |
| 列表三态 | `shared/components/ListEmptyState.vue`（`loading` / `filtered` / `empty`） | 放进 `el-table` 的 `#empty` 槽；见 §四「列表的三态」。非表格区域的大空态才用 `el-empty` |
| 列表页页头与节奏 | `shared/styles/list-page.css`（`.list-page-*`） | 页头、工具条槽、表格容器、枚举筛选档位；只放平台自己的类与 token，禁止 `.el-*` / `--el-*` |
| 列表页工具条 | `shared/components/ListToolbar.vue`（左动作 / 右搜索表单 + 搜索 + 重置） | 见 §四 布局契约；枚举筛选放 `#filters` 插槽用 `el-select` |
| 弹层/浮层 | `el-dropdown` / `el-popover` / `el-tooltip` | 禁止 `backdrop-filter: blur`（v1 禁令继续有效，遮罩为中性半透明）；库的 `is-dark` 变体拿文字色当底色与描边，已统一改回平台浮层表面 + 强描边（tooltip 最大宽度 720px），均入 token 层 |
| 图标 | `@element-plus/icons-vue`（`<el-icon>` 包裹） | 仍受 v1「零 Emoji、纯矢量细线条」约束；库内没有的业务符号继续用现有 1.5–1.8px 手绘 SVG |
| 日期/时间 | `el-date-picker` / `el-time-picker` | 新增即用，不再排期 |
| 侧栏导航 | `el-menu`（`router` 模式，见 §八） | 子级用 `el-sub-menu` 预留；条目高度 / hover 中性化 / 左缘 accent 标尺三项覆盖已入 token 层 |
| 设置中心页头与分区卡 | `shared/styles/settings-page.css`（`.settings-page-*` / `.settings-panel*` / `.settings-switch-row` / `.settings-field-hint`） | 六页同一个脸；只放平台自己的类与 token，禁止 `.el-*` / `--el-*` |
| 成块的互斥选项（主题配色 / 字号 / 渲染引擎） | `el-radio-group` + `el-radio`（卡片式） | 单选卡片高于默认 28px，由 token 层的 `.option-card` 桥接类接管高度与选中描边；业务组件里不写 `.el-radio__label` |
| 控件里的路径前后缀 | `el-input` 的 `#prefix` / `#suffix` **槽** | **库没有 `prefix` / `suffix` prop**：写成属性会落到原生 `<input>`（那里 `prefix` 是只读 getter），只喷 Vue 警告且前缀永不渲染；样式走 `.path-affix` |

**共享编辑器外壳（`shared/components/EntityFormDialog.vue`）**：所有「新增 / 编辑 X」表单统一用它，不要各页自己搭 `el-dialog` 头部与底栏。它固定：标题 + 一句话职责说明 + 记录号芯片、底栏的快捷键提示（Ctrl+S 保存 · Esc 关闭）与「取消 / 保存」按钮、`Ctrl/Cmd+S` 提交、`saving` 驱动的 loading。分区骨架（01 / 02 编号、hairline 分隔、字段下的说明文字、下拉项右侧次级说明、提示词预览块、开关行、两列栅格）放在 `shared/styles/entity-editor.css`——插槽内容在调用方作用域编译，组件的 scoped 样式够不着，所以与 `sidebar.css` 一样作为共享样式表引入；该文件只允许出现平台自己的类与 token，禁止 `.el-*` / `--el-*`。

**函数式 API 的引入方式**：`ElMessage / ElMessageBox / ElLoading` 直接 `import { ElMessage } from 'element-plus'`（样式已随全量 CSS 到位）。模板里的 `<el-*>` 一律不手写 import——交给解析器。

---

### chip 双轨：分类轨与状态轨不是一条轴

语义色（`success / warning / info / danger`）说的是「跟主题一起表达好坏」，它**不能**承担「不同即不同色」：实测 `cool` 与 `emerald` 里 `success == primary`（同一个 hex），`ocean` 与 `midnight` 里 `info == primary`，`warm` 里 `primary` 与 `warning` 近似，青竹里 角色类 / 流程类 都是橄榄绿——没有一对语义色在 12 套预设下都保得住距离，换 `type` 治不好。

| 轨 | 用在哪 | 怎么上色 |
|---|---|---|
| **分类轨** | 同一列里互斥的枚举值：提示词的 角色类 / 流程类、以后员工的生产引擎、团队的调度模式… | `class="entity-chip entity-chip--<枚举值>"`，**不写 `type`**；色相来自 `--cat-hue-1…6`（固定、与皮肤无关），档位映射只在 `shared/styles/entity-chip.css` 声明一次，同一个枚举值全平台永远同一档 |
| **状态轨** | 启用 / 异常 / 危险：如助手的「已启用平台工具」 | 继续用 `el-tag` 的语义 `type`，跟着主题走 |

分类轨的实现分两半，都不能越界：色相与 `el-tag` 三个部件变量（`--el-tag-text-color / -bg-color / -border-color`）的桥接在 token 层的「分类色轴」段——选择器必须是 `html .el-tag.entity-chip`，因为库的类型类写成 `.el-tag.el-tag--primary`（两位 class），特异度不够就被抢回去；页面只声明档位，不出现颜色。色相固定、明度向主题的 `--color-text / --color-surface / --color-border` 混合，所以浅色预设是浅底深字、深色预设自动翻深底亮字，两档在任何皮肤下都分得开。**新增枚举值 = 在映射表里加一行档位，不在页面里挑颜色。**

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
| `src/tests/settings-table-budget.spec.ts` | 设置中心四张 `el-table` 的列宽总和 ≤ 866px（画布实测可用宽） |
| `src/tests/native-dialog-guard.spec.ts` | 禁止原生 `confirm/alert`（`ElMessageBox` 不受影响） |
| `src/tests/template-attribute-guard.spec.ts` | 模板属性名合法性（Electron 132 会抛错的写法） |

浏览器侧复核（每完成一个模块整改必做）：用真实浏览器在 **浅色 / 暖阳 / 极光（深色预设）** 三档下目视核对新界面，确认没有夹带的硬编码色与错位密度；截图对比优先，不接受"只看代码没问题"。

> v1 文档里写的 `design-taste-frontend/scripts/lint.mjs` 在本机与技能包中都不存在（技能包实际提供 `gate.mjs / consistency.mjs / copy-lint.mjs`，面向静态 HTML 页面而非本 SPA），其人工审查项已并入上表与 §一的两个问题。

---

## 八、迁移状态与待决事项

**已完成**：

1. 依赖与构建接入、`element-tokens.css` token 层、本规范与 AGENTS/PROJECT 同步。
2. 组织二级导航上 `el-menu`（router 模式）。**已知不一致**：其余五栏（项目 / 对话 / 任务 / 终端 / 设置）仍是 `sidebar.css` 卡片列表（引擎二级导航已随引擎中心一轮迁完，见下文第 9 条）。待办：把五栏一并迁到 `el-menu`（需先解决 Zone A 动作区与 Zone B 搜索区的归属），随后评估从 `sidebar.css` 删除卡片段，避免两种侧栏长期并存。菜单三项覆盖（条目高度 46 / 34px、hover 中性化、库没有的左缘 2px accent 标尺）已在 `element-tokens.css` 的 menus 段；渲染层按 `children` 自动切换 `el-sub-menu`，日后加子菜单只改数据。
3. 组织中心四个子页全部迁完（菜单文案统一为「AI 提示词 / AI 助手 / AI 员工 / AI 团队」，「AI」与中文之间留半角空格）：
   - 列表：`el-table`（`height="100%"` 铺满主体），行内常驻开关 `el-switch`，行内操作 `el-button link`；
   - 工具条：统一 `shared/components/ListToolbar.vue`，枚举筛选 `el-select`（AI 助手＝常驻状态、AI 员工＝执行引擎、AI 提示词＝类型；AI 团队无枚举只留关键词），生效时机见 §四 布局契约；
   - 编辑器：`AssistantEditPanel` / `EmployeeEditPanel` / `TeamEditPanel` → `*EditDialog`，共用 `shared/components/EntityFormDialog.vue` 外壳；AI 提示词的 `PromptManageDialog` 同批迁 `el-dialog` + `el-form`；
   - 交互：删除 → `ElMessageBox`（`confirmButtonType: 'danger'`），结果反馈 → `ElMessage`，AI 团队专家编制 → 二级 `el-checkbox-group` 卡片网格勾选回显。
4. §四 布局契约落地：页头去计数芯片、工具条左动作右搜索、表格铺满；弹窗全局垂直居中 + 内部滚动 + 底栏固定。
5. 组织中心反馈与质感层（只面向 ≥ 1280 的桌面宽度，窄屏不在本轮范围）：   - **三态**：新增 `shared/components/ListEmptyState.vue`，四页的 `#empty` 槽不再各写一行灰字；loading 给行形骨架、搜不到给「重置筛选」、空库给导航同枚矢量图标 + 主动作；
   - **页头原语**：`shared/styles/list-page.css`（`.list-page-*`），页头标题从 Tailwind `text-lg`（15.75px，不在字阶上）收编到 `--text-size-20`，四页重复的 `.filter-select` / `.table-wrap` 一并删除；
   - **可反馈性**（均在 token 层）：行内 link/text 按钮从 `padding: 2px` 给到 24px 命中区与 hover 中性底；typed link 的 hover 不再把主色刷成灰色（改 `--el-button-hover-link-text-color` 按类型加深一档）；`.el-button` 的 `transition: all` 改具名列表；实心按钮 hover 上浮 / 按下回位；表格内取消库的兄弟 `margin-left`（否则三个操作在 168px 列里被裁）；
   - **数字与时间**：表格 / 分页 / 下拉开 `tabular-nums`；时间列改为手工拼接的 `YYYY-MM-DD HH:mm`；
   - `UiAlert` 页面错误横幅从组织中心退位（同一错误不再 toast + 横幅双报，横幅还会把表格顶下去），棘轮 750 → 745。

6. 一级 rail 的「组织」入口默认落点改为 **AI 提示词**（`CategoryRail.vue` 原本指向 `org-assistants`；`/org` 本身的 redirect 早已是 `org-prompts`）。
7. AI 提示词页改为**三段式主体**（§四 布局契约 v2）：页头只剩标题与职责，工具条固定顶、`el-table` 铺满中间、`el-pagination` 固定底（10/20/50，筛选一变就回第一页）；工具条紧凑度收到控件间距 2px + 关键词框 120px，搜索范围改走 `hint`（title / aria-label）。其余三页待跟进（见待办 1）。
8. **助手 / 员工 / 团队三页照同一模板改完，四页同构**
   - `AssistantListView` / `EmployeeListView` / `TeamListView` 整块模板换成三段式：工具条从页头搬进 `.list-panel-toolbar`、表格包进 `.list-panel-table`、底部加 `.list-panel-footer` + `el-pagination`（`size="small"` + `background`，10/20/50）；筛选一变回第一页、总行数缩小钳制当前页；`:data` 换成 `paged*` 切片；页头只剩标题与一句话职责。
9. **引擎菜单全量迁完（`features/engines/` 零 `Ui*`，棘轮 745 → 630）**：
   - 二级导航 `EnginesSidebar` 与 OrgSidebar 同构：`el-menu` router 模式，图标走 `@element-plus/icons-vue`，token 层 menus 段覆盖直接复用；
   - 助手 / 会话 / 协同三个面板：`el-button` 保存、`el-checkbox` 权限开关，内联「✓ 配置已保存」退位，保存结果只报一次 `ElMessage`（失败不再静默）；
   - `EngineStepControl`：自绘 `<input type=number>` → `el-input-number`（关步进器、钳在 -1），预设按钮 → `el-segmented`；预算量规保留（纯展示面，不是控件）；-1=不限语义不变：清空 / 非正数一律回 -1；
   - `AgentSettingsView` 整页：自绘 Tab → `el-tabs`（启用圆点、`agent-tab-*` 钩子保留）；路径输入 `el-input`、传输/权限模式 `el-select`、启用开关 `el-switch`、空闲断开 `el-input-number`；模型目录 `<ul>` → 小尺寸 `el-table`（行内 `el-switch` + `el-tag` 状态轨 + 行内「编辑」link）；模型编辑内联表单 → `shared/components/EntityFormDialog.vue` + `el-form`；停止 service 的内联确认条 → `ElMessageBox.confirm`（danger）；诊断 `<details>` → `el-collapse`（警告/错误默认展开）；
   - 页面级错误横幅全部退位：store 错误（agentsError / serviceError / diagnosticsError）由 watcher 统一 `ElMessage` 报一次；
   - 内嵌面板 `features/settings/BuiltinDataPanel.vue`（会话数据管理，仅被 Agent 配置页使用）同批整页迁：`el-table` + `el-pagination`，删除/清空走 `ElMessageBox`；`AgentUsagePanel` 同步迁移（`el-tag` 状态轨、配额条改用 `--color-success/warning/danger` 的 platform token 类）；两页里早年不在 tailwind 映射内的死样式类（`bg-surface/40`、`text-muted-foreground` 等）收编回 `platform-*` token 类；
   - 顺带清障：删除从未挂载的 `CoreEngineView.vue` 与 router 死 import（`/engines/core` redirect 与 API `core` 字段不动）。
   - **反向补齐**：`OrgPromptsView` 原本**完全没有** URL 同步（另三页都有 `?action=new` / `?edit=<id>`），补上同一套 `syncRouteState` + `router.replace` —— 粘贴的链接点开的东西必须和点击一样。实测四页：直链打开、点击写 query、Esc 清 query 全一致。
   - **删掉旧写法**：`.list-page-toolbar`（工具条写在页头里）与无分页的 `.list-page-body` 四页同构后零引用，已从 `list-page.css` 删除；留着就有人会挑到旧的那种。
   - **分页必须写 `size="small"`**：提示词页摘掉 `size` 后条数下拉回到 28px，而页码方块被 token 层钉在 24px，同一条横带两种高度。
   - 实测（1440×900 四页）：工具条 y=80 / 表格 123→868 铺满 / 分页底贴 900、分页带 32px、控件全 24px、字号全 12px、区带线 1px、表头竖线最后一列不画、空态与关键词回显照常、0 pageerror。

10. **设置菜单全量迁完（六张路由页 + 二级导航 + 三个弹窗 + 共享目录浏览器，`Ui*` 棘轮 630 → 343）**：
   - `SettingsNav` 与 OrgSidebar / EnginesSidebar 同构：`el-menu` router 模式，图标走 `@element-plus/icons-vue`，保留标签 + 职责描述两行与 `data-test="settings-nav"`；
   - 外观：主题配色与 PlantUML 渲染引擎 → `el-radio-group` 卡片（`.option-card` 桥接类在 token 层接管高度与选中描边），字号 → `el-radio-group` 三档卡片，快捷助手开关 → `el-switch`；
   - 模型 / 服务代理 / 服务连接 / 终端服务器：列表一律 `el-table`（画布变体：自然高度、不分页），枚举分类走 `entity-chip` 分类轨（静态目录 / 端口反代、本机 / 远端、私钥 / 口令、文本 / 图像 / 工具…），启用 → 行内 `el-switch`，删除 → `ElMessageBox`（danger），结果只报一次 `ElMessage`；
   - 弹窗：`ProxyEditDialog` / `ServerDialog` → `EntityFormDialog` + `el-form` + `rules`（校验走行内 `prop`，不再把 `actionError` 字符串堆到外壳）；「测试连接 / 浏览目录…」放 `#head-extra`，**不进底栏**（底栏永远只有取消 / 确认）；`ServerDirectoryPicker` → `el-dialog`（props / emits 零改动，两个调用方无感）；
   - 三件真问题的修复：（a）外壳只写 `root.style.colorScheme`、从不加 `dark` class，而六页写了 145 处 `dark:` 工具类，所以深色预设下卡片全是白底（全部删除，改由 `.settings-panel` + token 接管）；（b）~40 处 `bg-surface/40` / `text-foreground` / `border-border` / `text-accent` 在 `tailwind.config.js` 里根本没定义（只有 `platform.*` 命名空间），在成品 CSS 里找不到对应规则 —— 字体预设选中态与当前连接高亮是**静默失效**，不是“颜目不一致”；（c）终端字体预览硬编码 `bg-[#121316]`，现改为跟随主题 token；
   - 新增 `shared/styles/settings-page.css`（六页重复六次的 `bg-white dark:bg-slate-900 p-5 rounded-lg` 收编成一次声明）；`shared/format.ts` 新增 `formatListTime()`（手工拼接 `YYYY-MM-DD HH:mm`，不用 `toLocaleString()`）；
   - 删死代码：`SettingsRolesView.vue`（654 行，路由早已 redirect 到组织中心）及其单测、`SettingsPromptsView.vue`（壳子重定向）与 router 死 import、`ProxiesView.vue`（无路由的旧代理页）与 `features/proxies/index.ts` 的导出；
   - **实测**（Playwright 驱动真实栈，1365×720，浅色 / 暖阳 / 极光三档，六页全走）：菜单六项且恰好一项 `is-active`；深色预设下无白底面板；无死样式类残留；0 `pageerror`；同一行的控件同高同基线（实测字体行：`el-select` 与 `el-input` 均 top=196.5 / h=28）；四个弹窗均居中、`max-height: calc(100vh - 48px)` 生效、底栏固定且只一枚主按钮；
   - **两个只能靠浏览器发现的缺陷（已修）**：（a）列宽预算——服务代理八列声明 930px 而画布实测只有 866px，表格开横向滚动后 `fixed="right"` 的操作列盖住了「更新时间」，且最后一列芯片被削一半（重排到 854px，并新增 `settings-table-budget.spec.ts` 守卫）；（b）`el-input` 的 `prefix` / `suffix` 只有槽没有 prop，写成属性会落到原生 `<input>` 上只喷 Vue 警告、`/proxy/` 前缀永不渲染；
   - 顺带修一处陈旧 E2E：`settings-navigation.spec.ts` 还在等 `[data-test="project-edit"]` 与「编辑项目」弹窗，而项目树重构后入口已是 `project-settings`（跳 `/projects/:id/settings`）——本次改动前该用例就已红（stash 后跑基线证实），现改为断言路由与「常规」分区。

11. **Bot 引擎页增量落地（`BotEngineView.vue` / `BotChannelDialog.vue` / `BotQrLoginDialog.vue` / `BotChannelChatDrawer.vue`）**：严格按 Element Plus v2 与 `settings-page.css` / `entity-editor.css` 契约构建，复用 `EntityFormDialog` / `EngineStepControl` / `ListEmptyState` / `chat` 导出的 `MessageView` + `createChatStream`，零 `Ui*` 原语，渠道表格列宽控制在 866px 预算内，在 `shared/styles/entity-chip.css` 新增分类档位 `.entity-chip--wechat`（`--cat-hue-4`）。

12. **消息渠道迁移至组织中心（`features/organization/channels/`：`ChannelListView.vue` / `ChannelEditDialog.vue` / `ChannelQrLoginDialog.vue` / `ChannelChatDrawer.vue`）**：从 `引擎 -> Bot 引擎` 迁移至 `组织 -> 消息渠道`（`/org/channels`，保留 `/engines/bot` 重定向并删除旧 `BotEngineView*` 组件），从画布表格转为与组织中心另四页完全同构的 `list-page.css` 三段式布局（`ListToolbar` + `el-table` + `ListEmptyState` + `el-pagination`）；在 `entity-chip.css` 新增模式分类档位 `.entity-chip--assistant`（`--cat-hue-1`）与 `.entity-chip--agent`（`--cat-hue-3`），支持「AI 助手」与「项目 + 角色 + 内置 Agent」双模表单与 `/new` 单渠道上下文切换。

**待办**：

1. 其余模块（任务、终端、工作区）按页面逐个迁移，每页一个 PR，随页更新本节清单（设置模块已随第 10 条全量迁完；其中的 `BuiltinDataPanel` / `AgentUsagePanel` 被引擎页内嵌，更早随引擎中心迁完）；新页面直接按 §四 布局契约与「列表的三态」写（三段式主体 + `.list-page-*` 类名，空态用 `ListEmptyState`，列表带分页），不要再写页头计数芯片、把工具条塞进页头、散装筛选行与自绘空态；**但设置中心的页用 `settings-page.css` 的 `.settings-page-*` / `.settings-panel*`，不要套三段式**（见 §四 的画布例外）。
2. 助手的预设头像仍是 emoji（源码里带 `taste-skill-ignore` 标记），与 v1「零 Emoji」相冲，且缺 emoji 字体的环境会显示成豆腐块；待办：换成 v1 的手绘 SVG 图标集，头像字段改存图标 key。
3. 技术债登记：`features/settings/ProxySettings.vue` 为了复用 `ProxyEditDialog` 而 `import from '../proxies'`（设置页跨特性引用了代理特性里的编辑器），下一轮把这类跨特性共用的编辑器下沉到 `shared/components/`；`features/groups/GroupEditPanel.vue` 与 `GroupManageDialog.vue` 仍是零路由引用的死代码（本轮只登记不动），删除时一并下调棘轮基线。

---

*本规范由平台前端工程团队维护。新增界面、组件或调整主题时必须严格执行；改变本文任何一条契约，请同时更新本文与 `AGENTS.md` 摘要。*
