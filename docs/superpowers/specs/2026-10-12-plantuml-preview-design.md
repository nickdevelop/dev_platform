# 对话中 PlantUML 图表的预览渲染

日期:2026-10-12
状态:待实现
关联:`docs/superpowers/specs/2026-10-12-builtin-fonts-design.md`(同一判断标准:不接受运行时的外部依赖)、
`docs/superpowers/specs/2026-10-10-task-chat-redesign-design.md`(消息气泡与 Thinking/工具块的既有形态)

## 意图与理由

Agent 回答架构问题时常直接给一段 ```` ```plantuml ````。今天它落到 `MessageView.vue` 的
`renderMarkdown` 里,和一个 `ls` 的输出长得一样:一块等宽文字。使用者要的是**看图**,不是读 DSL
—— 评审一张时序图时,眼睛要顺着箭头走,而不是在脑内编译 `participant ... as FE`。

所以:agent 回复里出现 plantuml 代码块时,默认就是图;想看原文时切一下就有。

## 目标

- agent 气泡里的 ```` ```plantuml ```` / ```` ```puml ```` 块渲染成 SVG 图,默认显示图。
- 块自身可切换预览与源码,形态与会话里已有的 Thinking、工具块同构(一行标题 + 右侧动作)。
- 切换只影响这一个块,同一条消息里的多个图互不干扰。
- 渲染全部在浏览器内完成:平台不请求任何外部服务,不需要服务器上装 Java,内网与离线部署同样工作。
- 画不出来时必须看得见原因,并把源码摊开,不白屏、不静默。
- 引擎代码只在消息里真的出现过 plantuml 块时才被下载。

## 非目标

- **不支持外部图形库**:`!include <C4/…>`、aws / kubernetes / material 图标精灵、emoji 与 openiconic
  库都不带。这类源码按失败处理,退回源码。(需求方明确排除。)
- 不做图片导出、不做全屏查看/缩放、不做图表编辑。
- 不扩展到用户消息、Thinking、工具输出或任何文档页面 —— 只有 agent 气泡里的 markdown 走这条路。
- 不改 `renderMarkdown` 既有的语义与它现在的测试。
- 不引入服务端 PlantUML/Kroki 服务,不引入 Java 依赖。
- 不处理 `!theme`:官方 `themes.js` 不随包发布,缺它时 PlantUML 会用无主题样式画图并在控制台告警,
  图本身仍然正确,可接受。

## 约束与已实测的事实

以下结论来自一次一次性验证(`/tmp`,未进仓库,未改任何产品代码),用 Playwright + Chromium 真实跑通:

- 引擎选 **`@plantuml/core` 1.2026.8**(PlantUML 官方引擎,TeaVM 编译成 JS,浏览器内渲染,自带
  Graphviz 布局,无需服务端)。**采用方案由需求方选定**,理由是与字体那次相同的"零运行时外部依赖"。
- 体积:`plantuml.js` 3.9 MB、`viz-global.js` 1.4 MB、`themes.js` 0.3 MB,raw 5.5 MB / **gzip 约 1.7 MB**。
  因此**必须懒加载**,不能进主 bundle。
- 性能:首张时序图 **约 520 ms**,同页第二张起 **约 60 ms**(引擎常驻)。
- **并发会挂死**:同一页面同时提交多张图,只有一张的回调会来,其余永久不返回。实现上**必须全局串行**。
- **错误不总是错误**:源码解析失败时引擎**不报错**,而是返回一张正常形状的 SVG,图里写着
  `PlantUML version …`、`Fatal parsing error`、`Empty description` 之类文字。真正走 `onError` 回调的
  只有两类:缺 `@enduml(`java.lang.IndexOutOfBoundsException`,约 8 ms)和图超过 `maxSvgSize`
  (默认 8192 px)。错误判定必须**两条路都覆盖**。
- API:`renderToString(lines, onSuccess, onError, options)`,第 4 个参数支持 `{ dark: true, maxSvgSize }`。
  深色主题重画因此可行。
- `viz-global.js`(Graphviz 布局)**必须先于** `plantuml.js` 加载;包内 `sideEffects` 已声明它。
- 图源码来自 agent,不可信:产出的 SVG 在插入 DOM 前必须再过一次 DOMPurify。

## 方案

### 1. 消息切分:`renderMarkdown` 保持纯函数

新增 `web/src/markdown-segments.ts`:

```
splitDiagramBlocks(source: string): Segment[]
Segment = { kind: 'markdown'; text: string } | { kind: 'plantuml'; text: string; closed: boolean }
```

- 按行扫围栏:```` ``` ```` 与 `~~~` 都认,信息串的**第一个词**(大小写不敏感)是 `plantuml` 或 `puml`
  才算图表块;其余围栏(包括围栏内容里出现的伪围栏)原样留在 markdown 段里交给 marked。
- 扫到结尾仍未闭合的围栏 → `plantuml` 段且 `closed: false`,这就是流式进行中的形态。
- 返回的段按原文顺序拼起来必须能还原整条消息,便于测试与推理。

取舍:写在这里而不是 marked 的 renderer 里,是因为切分会把**嵌套在列表或引用里的** plantuml 围栏提到
顶层。对话里"列表项里嵌一个 plantuml 块"极罕见,不值得为它换成"渲染后再走 DOM 挂载组件"那套更绕的做法。

### 2. `MessageView.vue`

agent 气泡从"一次 `v-html`"改成:同一个 `.markdown-body` 容器里 `v-for` 段,markdown 段仍走
`renderMarkdown` + `v-html`,plantuml 段渲染 `<PlantumlBlock>`。用户 turn、Thinking、工具块的分支不变。

### 3. `PlantumlBlock.vue`(方案 C 的形态)

- 结构:标题行(`▾ 图表 · plantuml` + 右侧蓝色"查看源码 / 返回预览")、画布、失败时的一行原因 + 源码 `<pre>`。
- 标题行整体是 `<button aria-expanded>`,点它和点右侧文字都算切换;默认预览。
- 状态是组件内 `ref`,不进 store、不落盘:刷新后回到默认预览。
- `closed === false` 或 `entry.streaming` 时不渲染,标题行右侧写"等待完成…",画布位置留一行"正在生成图"。
  消息结束后由 Vue 的响应式重渲染触发真正的渲染。
- 失败态:标题行下方一行红字"无法渲染:{原因}",下面直接摊开源码,标题行右侧动作变为不可用或隐藏
  (因为没有预览可返回)。
- 观感、间距、密度、圆角沿用 MessageView 里 Thinking/工具块已用的 CSS 变量,不新增主题项。

### 4. `web/src/plantuml/renderer.ts`

- `loadEngine(): Promise<Engine>`:先副作用导入 `viz-global.js`,再动态 `import()` `plantuml.js`,
  只成功一次,失败记录原因供各块显示。
- 全局**串行队列**(单条 promise 链):所有渲染请求排队,一张画完才轮到下一张。
- 每次渲染**超时 15 s** 记为失败并放行队列(依据:并发挂死时引擎永不返回,不能让一个块卡死整条队列)。
- 错误判定:`onError` 回调,**或** SVG 文本命中
  `/PlantUML version|Fatal parsing error|Syntax Error|Empty description|not a diagram/`。
- 结果缓存:`Map`,key = 源码 + dark 标记,值 = 已 sanitize 的 SVG;带 LRU 上限(长会话不至于攒出几百张图),
  具体上限由 plan 定。缓存的目的就是让"来回切预览/源码"和"往上翻历史"都不重画。
- 输出前 `DOMPurify.sanitize(svg)`。

### 5. 主题

`ThemeColors`/presets 里需要一个"这套配色算不算深色"的标记(字段名由 plan 定),`PlantumlBlock` 从
settings store 读当前主题并据此决定是否传 `{ dark: true }`。判断由我下:**只有 `dark` 这套算深色**,
`warm`、`cool` 仍按浅底画。深浅切换后重画,缓存 key 已把 dark 分开,互不污染。
图本身始终画在不透明的浅色或深色底上,画布区给一个固定底色,避免图与气泡背景糊在一起。

### 6. 依赖与打包

首选:加 `@plantuml/core` 到 `web/package.json` 的 `dependencies`,`renderer.ts` 里动态 `import()`,
让 Vite 切出一个独立 chunk(与字体"手动脚本取源、二进制进仓库"的做法**不同**,因为这是 JS,不是资产)。

**风险(plan 的第一步必须验证)**:`viz-global.js` 是 UMD,内部有 `require("url")` 的 Node 专用分支;
它在 Vite 的依赖预构建与生产构建里能否原样通过,尚未验证。若不通,退路是照搬字体先例:把
`plantuml.js`、`viz-global.js` 放进 `web/public/plantuml/`,运行时注入 `<script>` 标签加载,不进主 bundle,
构建期一行代码都不用碰这两个文件。

## 验证

- `markdown-segments` 单测:闭合与未闭合围栏、`~~~`、`puml` 别名、大小写、围栏内容里的伪围栏、
  其他语言块不受影响、段拼接可还原原文。
- `PlantumlBlock` 单测(注入假 renderer,不在 jsdom 里跑真引擎):默认显示预览、点击切到源码、
  失败态同时给出原因与源码、流式期间不请求渲染。
- `renderer` 单测:并发请求必须串行完成;一个请求超时后队列继续;同一 key 命中缓存不再调用引擎;
  错误 SVG 被识别为失败。
- 现有 `markdown.spec.ts`、`message-view.spec.ts` 继续通过。
- 端到端(Playwright,真实引擎,放 `web/e2e/`):一段含 plantuml 块的回复 → 画布里有 `<svg>`;
  点"查看源码" → 出现 `@startuml`;一段含 `!include <C4/…>` 的回复 → 出现失败行与源码。
- 手动在四套主题、三档密度下看一眼观感。

## 我自己下的判断(无需再确认,但可在阅读时推翻)

- 语言标签认 `plantuml` 与 `puml` 两个。
- 源码视图只显示围栏**内部**的图源码,不带 ```` ``` ```` 标记;不做语法高亮。
- 引擎自身加载失败(chunk 拉不下来、初始化抛异常)与图本身画失败共用失败态,文案区分不开就写
  “PlantUML 引擎未能加载”。
- 单块源码上限 2000 行,超出则不送引擎,直接进失败态并说明过长(防一段狂抬 10 万行的回复把
  浏览器卡住)。
- 块卸载(消息被历史分页摸掉)不取消已入队的渲染,引擎反正会把那张图画完。
- 历史消息与实时消息同构处理,不做区分。

## 留给 plan / builder 的决定

- 引擎最终按 npm 依赖还是 vendoring 落地(取决于上面那条验证)。
- 标题行的图标与文案:是否给个 📊 与 🧠/🔧 并列,还是纯文字。
- LRU 上限取值;"正在生成图"与"等待完成…"的具体措辞。
- 超长图的最大高度与内部滚动策略(没有它,一张 3000 px 的时序图会把整条消息撑爆)。
