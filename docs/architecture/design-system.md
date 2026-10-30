# dev_platform 设计系统规范：Data Terminal 极客终端

本规范定义了 `dev_platform` 的前端设计系统与视觉工程准则。该规范基于 `design-taste-frontend` 的 **App Screens（开发者工具/工作台）** 专章，面向长会话、高强度、沉浸式多 Agent 编码与实时系统协同场景制定。

---

## 一、设计哲学与核心定位

### 1. 核心定位：Data Terminal（数据终端）
平台并非普通的 SaaS 营销展示页或粗浅的信息看板，而是一个**面向专业工程师的 AI Coding Agent 协同作业工作台（Coding Harness）**。视觉风格必须呈现出如专业金融监控终端、现代命令行 IDE（Neovim/Linear/Claude Code 终端）般的克制、精密、深邃与高效。

### 2. 四大核心设计原则
1. **数据高于镀铬（Data over Chrome）**：外壳（Chrome）、边框、背景保持高度克制退后，所有视觉重心让位于用户代码、Agent 思考推理、Diff 变更与核心统计指标。
2. **纯平线阶取代伪立体（Rules over Shadows）**：彻底清除卡片静止状态下的模糊投影（`box-shadow`）与毛玻璃模糊（`backdrop-filter: blur`）。空间层级仅依靠 1px 细微明度断阶线（1px Rules）与阶梯底色（Tone Steps）区分。
3. **字符网格与对齐律（Tabular Alignment）**：全站强制数字等宽（`tabular-nums`），无论是代码行号、时间戳、上下文 Token 用量、吞吐率还是监控指标，必须在字符网格上保持严谨对齐。
4. **纯净矢量符号系统（Pure Vector Glyphs）**：全站禁止使用跨平台渲染参差不齐、带有低幼模板感的系统原生 Emoji，统一由 1.5px / 1.8px 细线条极简几何矢量 SVG 构成。

---

## 二、色彩体系与 Token 规范

### 1. 色彩预算（Color Budget < 3%）
在整个视口中，彩色像素的占比必须严格控制在 **3% 以内**。色彩绝不可用于装饰性背景、大幅度彩斑或花哨渐变，仅允许出现在：
* **数据状态与运行生命周期**（正在运行的呼吸点）；
* **代码与文件 Diff 增删**（1px 细线边缘与极浅半透明底衬）；
* **阻塞性交互决策**（等待用户授权确认的工具卡片、高危删除操作）。

### 2. 核心色阶层级（Dark 调色板基准）
所有组件必须引用 CSS 自定义属性（Tokens），严禁在组件内部使用硬编码的 Hex 色值或未经 Token 映射的 Tailwind 杂色：

| Token 名称 | 变量定义 | 色值基准 | 语义职责与使用场景 |
|---|---|---|---|
| `--ground` / `--color-bg` | 页面最底层底座 | `#0f1115` | 工作台外层视口底色，极低色噪的深石墨黑。 |
| `--surface` / `--color-surface` | 主工作区表面 | `#14171f` | 会话面板、活动流面板、弹窗、卡片等主要承载容器。 |
| `--surface-muted` | 次级与衬底表面 | `#1a1e27` | 面板顶部栏、输入栏工具条、代码块背景、折叠抽屉。 |
| `--border` | 基础 1px 标尺线 | `#222631` | 面板分割线、表头底边线、普通控件外框。 |
| `--border-strong` | 强调 1px 细边线 | `#2e3545` | 输入框、浮层弹窗边框、聚焦与悬停状态边线。 |
| `--color-text` | 纯净主文字 | `#e6e8ef` | 标题、主要消息内容、代码文字。 |
| `--color-text-muted` | 中性辅助文字 | `#8a8c92` | 次级描述、路径标签、元数据、表单帮助说明。 |
| `--color-accent` / `--accent` | 终端科技蓝印 | `#4a8ad6` | **全局唯一的激活与焦点色**，仅用于 1px/2px 贴边线条或数据高亮。 |
| `--color-success` | 成功/已就绪状态 | `#44a282` | 终端青绿，用于状态正常、执行成功、通过检查。 |
| `--color-danger` | 危险/错误状态 | `#e5566f` | 终端深粉红，用于报错、阻断、失败、删除危险操作。 |
| `--color-warning` | 警告/调度状态 | `#d9a441` | 琥珀金，用于等待中、自动审批警示、周期任务调度。 |

### 3. 禁忌色彩（Banned Palettes）
* **禁用紫色与靛蓝（Violet / Indigo 禁令）**：全站禁止使用 `#7c3aed`、`#a855f7`、`#8b5cf6` 等紫色调，这是通用大模型生成式前端的典型“模板标志”，削弱了工程严肃性。
* **禁用渐变大色块（Gradient Blobs 禁令）**：按钮、卡片、背景一律平整单色，禁止渐变背景与发光描边（唯一的例外是长文本折叠淡出遮罩）。

---

## 三、排版体系与字符节奏

### 1. 本地离线字体栈（Zero Network Dependency）
平台内置字体资产，完全零外网请求依赖：
```css
/* 主体字体栈：西文/数字/代码首选 Google Sans Code，中文字符 fallback 到 Noto Sans SC */
--font-family: 'Google Sans Code', 'Noto Sans SC', sans-serif;
/* 专门代码与路径字体栈 */
--font-mono: 'Google Sans Code', 'Noto Sans SC', ui-monospace, SFMono-Regular, Menlo, monospace;
```

### 2. 强制等宽数字（Tabular Figures）
在 `style.css` 全局注入：
```css
html, body, button, input, select, textarea {
  font-variant-numeric: tabular-nums;
}
```
**规范要求**：任何呈现数值变动（Token 数、耗时 `00:02:14`、Diff 行数 `+12 / -4`、时间戳、版本号）的容器，必须保持字符等宽，防止数字刷新产生横向抖动。

### 3. 字阶与字距规则
* **标题高度收敛**：主标题最大限制在 `20px ~ 24px`，小标题 `14px ~ 16px`，正文 `13px ~ 14px`，辅助标签 `12px`。严禁在控制台内出现大于 32px 的宣传型巨型排版。
* **禁用全大写宽字距（Micro-label 禁令）**：表单字段标题与卡片标签一律使用常规字距（Sentence case），禁止使用 `uppercase tracking-wider text-[10px]` 这类过度修饰的伪工业标签。

### 4. 直角与微倒角（Border Radius）
* 卡片、输入框、弹窗面板、活动条目统一收敛为 **3px ~ 4px 微倒角**。
* 严禁 12px ~ 16px 的浮肿大圆角（只有极少数状态小胶囊徽章允许 9999px 全圆角）。

---

## 四、交互规范与状态法则（The Move）

### 1. 选中与激活法则（The Move）
> **“Every selection (tab, toggle, range, nav item) is a 1px or 2px accent line placed on the nearest existing edge.”**

* **标签页（SessionTabs / FileViewer Tabs）**：激活项不是浮动的圆角卡片，而是底部紧贴底框的 **2px accent 科技蓝实线**，背景透明或与工作区平齐，文字纯净高对比。
* **左侧导航轨（CategoryRail）**：激活项以左侧边缘紧贴的 **2px accent 竖直标尺线** 与白色文字高亮标记。
* **补全下拉项（ComposerDropdown）**：悬停与选中项以左边缘的 **2px accent 竖线** 指示。
* **聚焦输入框（Composer Box）**：获取焦点时直接将外框变为 `1px solid var(--color-accent)`，禁止额外添加带有模糊扩散的外发光环（glow / ring-4）。

### 2. 静止阴影与玻璃拟态禁令
* **零静止阴影**：卡片、输入框、按钮在未悬停的常态下不得包含任何 `box-shadow`。只有全屏模态弹窗或下拉浮动菜单在脱离文档流时允许极细微的边界辅助阴影。
* **零背景模糊**：遮罩层一律采用中性深色半透明（`rgba(10, 12, 16, 0.75)`），全面移除 `backdrop-filter: blur(...)`，保障流畅的渲染性能。

### 3. 动效预算与原则（Motion Budget）
* **严禁 `transition: all`**：过度指定 `all` 会触发昂贵的重排与意外的尺寸拉伸。所有过渡必须明确指定受控属性，如：
  ```css
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
  ```
* **禁止注意力夺取动效**：严禁在 UI 状态指示上使用循环跳跃、放大缩小或 `animate-ping` 脉冲，运行状态通过呼吸明度或极简流式光标呈现。

---

## 五、纯净矢量图标系统（Iconography）

### 1. Emoji 替代规范对照表
所有界面控件、状态、工具标识彻底杜绝使用操作系统原生 Emoji，一律替换为规范的 **1.5px / 1.8px 细线条极简几何矢量 SVG**：

| 业务场景 | 禁用符号 (Emoji) | 规范替代方案 (SVG) | 视觉特征 |
|---|---|---|---|
| **工具：代码编辑** | `✏️` / `✎` | 1.5px 斜向画笔 SVG | 简洁斜尖几何线框 |
| **工具：文件读取** | `📖` / `📄` | 1.5px 折角文档 SVG | 带 45° 翻折角的直角线框 |
| **工具：终端命令行** | `⚡` / `💻` | 1.8px 终端提示符 SVG | `>_` 命令行控制台折线 |
| **工具：全文检索** | `🔍` | 1.8px 细线放大镜 SVG | 单圆环 + 45° 直柄 |
| **工具：网络请求** | `🌐` | 1.5px 全球经纬球体 SVG | 圆环 + 椭圆经线 |
| **状态：执行成功** | `✓` / `✅` | 2.0px 折线对勾 SVG | 3 节点折角，无填充 |
| **状态：错误与关闭** | `✕` / `❌` | 1.8px 双斜交叉线 SVG | 居中交叉，纯净直角 |
| **状态：等待与计时** | `⏳` / `⏰` | 1.8px 极简时钟 SVG | 圆环 + 直角时分指针 |
| **Agent / 思考过程** | `🧠` / `🤖` | 1.8px 神经网络脑图 SVG / 极简机器人 | 1px 细线节点几何 |

---

## 六、核心工作台布局与骨架标准

### 1. 全局三栏/四栏工作台
```
┌──────────┬──────────────┬────────────────────────────────────────────────────────┐
│ 50px     │ 240px        │ 主工作区 (Tab Strip + Split Pane)                      │
│ Icon Rail│ Drawer/Tree  │                                                        │
│          │              ├──────────────────────────┬─────────────────────────────┤
│ (固定窄轨)│ (可折叠/浮动)│ 对话消息流与输入器 (Message) │ 思考与工具活动流 (Activity) │
│          │              │                          │ 及文件查看器 (FileViewer)   │
└──────────┴──────────────┴──────────────────────────┴─────────────────────────────┘
```

1. **第一栏：CategoryRail（50px 极窄全局业务轨）**：
   * 仅承载高阶功能出口（项目、会话、任务、提示词、终端、连接、设置）。
   * 图标居中，下方附 11px 单词描述；激活项通过左侧 2px 蓝线贴边。
2. **第二栏：中间抽屉列表（Drawer）**：
   * 支持固定（Pinned）与抽屉浮出（Floating）双模；
   * 列表项采用标准两行式紧凑布局（标题截断 + 属性元数据），高度统一。
3. **顶部标签栏：SessionTabs**：
   * 紧贴工作区顶部，消除外边距；
   * 标签项采用直角紧凑 Tab，激活标签底部贴合 2px 实线。
4. **核心会话工作区：SessionChatPanel**：
   * **双分屏联动（Conversation Split）**：左侧消息与复合输入器，右侧思考活动与文件预览，中间设 1px 细线拖拽调节器（Resizer）。
   * **复合输入器（Composer）**：
     * 1px 细线边框，聚焦时 1px 强调蓝线；
     * 拖拽手柄、输入域、工具栏（图片添加、模型选择、上下文 Token 仪表盘、审批模式切换、发送/停止分割按钮）高度统一，字符对齐。

---

## 七、质量门禁与设计审查守卫（Taste Linter）

为确保后续开发不退化回粗糙模板，项目集成了 `design-taste-frontend` 的自动化审查脚本：

```bash
# 运行前端设计规范审查
node /root/.agents/skills/design-taste-frontend/scripts/lint.mjs --app web/src/features web/src/shared web/src/theme web/src/views
```

### 违规拦截指标（Fail-on-Error）：
1. `[emoji]`：源码中出现原生 Emoji 用于 UI 控件；
2. `[purple]`：使用任何未经批准的紫色/靛蓝字面量；
3. `[micro-label]`：出现全大写字母加宽字距的装饰性小标签；
4. `[eyebrow]`：出现脱离主标题的悬浮 kicker 标签；
5. `[gradient]`：出现非折叠遮罩的彩色渐变。

### 合理例外标记（Escape Hatch）：
针对用户自定义数据（例如用户可选的助手机器人预设图标池）或特殊折叠淡出遮罩，在**包含该特性的代码所在行末尾**明确标注：
```html
<div class="scrim">...</div> <!-- taste-skill-ignore: neutral fade scrim -->
```
未标注或滥用注释放行的代码无法通过门禁。

---

*本规范由平台前端工程团队维护。当新增界面、组件或调整主题时，必须严格执行本规范标准。*
