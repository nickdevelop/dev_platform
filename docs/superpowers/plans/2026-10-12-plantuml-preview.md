# 对话中 PlantUML 图表的预览渲染 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** agent 回复里的 ```` ```plantuml ```` 代码块在浏览器内渲染成图,默认显示图,可在同一块内切回源码。

**Architecture:** 消息文本先切成"markdown 段 / plantuml 段",markdown 段仍走现有 `renderMarkdown` + `v-html`,plantuml 段交给新组件 `PlantumlBlock`;后者通过一个全局串行的渲染队列调用 `@plantuml/core`(PlantUML 官方引擎,编译成 JS),引擎代码只在真的出现图表块时才动态加载。`renderMarkdown` 保持纯函数,后端与 API 一行不改。

**Tech Stack:** Vue 3 `<script setup>` + TypeScript、marked、DOMPurify、Pinia、Vitest + @vue/test-utils(jsdom)、Playwright(E2E)。

**Spec:** `docs/superpowers/specs/2026-10-12-plantuml-preview-design.md`

## Global Constraints

- 不使用任何外部服务:不请求 kroki/plantuml.com,不要求服务器有 Java;内网与离线部署必须照常出图。
- 引擎包固定 `@plantuml/core` = `1.2026.8`,加进 `web/package.json` 的 `dependencies`(不是 devDependencies)。
- `viz-global.js`(Graphviz 布局)必须先于 `plantuml.js` 加载完成。
- 引擎**同一时刻只能处理一张图**:并发提交会永久挂住,所有渲染必须串行。
- 图源码来自 agent,不可信:插入 DOM 前必须过 `DOMPurify.sanitize`。
- UI 文案一律中文,与现有会话区一致("Thinking" 是既有例外,不新增英文)。
- 全部改动在 `web/`;Go 侧、`docs/api/openapi.yaml` 不改。
- 测试命令在 `web/` 目录下跑:`npm run test -- <文件>`、`npm run typecheck`、`npm run build`。

## Review Focus

规格里 implied、但单个任务的测试容易漏掉、且真出问题最疼的几处:

- **流式未闭合的围栏**:agent 正在吐字、`@enduml` 还没到时不能反复重试渲染 —— 期望是"等待完成…",不请求引擎。
- **一段回复里的多个图**:必须排队画完一个再画下一个,并发提交会让引擎整页挂死。
- **引擎"假装成功"的报错画板**:解析失败时引擎返回形状正常的 SVG,只有图里的文字说明它失败了 —— 期望是失败态 + 摊开源码,不是把报错画板当成果展示。
- **超大 / 畸形源码**:几千行、或图尺寸超过引擎上限时 —— 期望是不卡浏览器,退化为失败态并给出原因。
- **主题**:深浅配色下各画一次,互不污染;切换后能重画。
- **非 plantuml 的代码块**:```` ```go ````、围栏内容里的伪围栏、`~~~` 围栏 —— 期望是渲染结果与今天完全一致。

---

### Task 1: 消息切分 `splitDiagramBlocks`

**Files:**
- Create: `web/src/markdown-segments.ts`
- Test: `web/src/tests/markdown-segments.spec.ts`

**Interfaces:**
- Consumes: 无
- Produces:
  ```ts
  export type Segment =
    | { kind: 'markdown'; text: string }
    | { kind: 'plantuml'; text: string; closed: boolean }
  export function splitDiagramBlocks(source: string): Segment[]
  ```
  `plantuml.text` 是围栏**内部**的原文(不含 ``` 标记与 info string),不含结尾换行。
  相邻 markdown 段不合并,拼接所有 `text` 必须还原 `source`。

- [ ] **Step 1: 写失败测试** —— `web/src/tests/markdown-segments.spec.ts`,至少覆盖:

```ts
it('cuts a closed plantuml fence out of the text', () => {
  expect(splitDiagramBlocks('前\n\n```plantuml\n@startuml\nA -> B\n@enduml\n```\n\n后')).toEqual([
    { kind: 'markdown', text: '前\n\n' },
    { kind: 'plantuml', text: '@startuml\nA -> B\n@enduml', closed: true },
    { kind: 'markdown', text: '\n后' },
  ])
})
it('treats an unterminated fence as an open diagram block', () => {
  expect(splitDiagramBlocks('```puml\n@startuml\nA -> B')).toEqual([
    { kind: 'plantuml', text: '@startuml\nA -> B', closed: false },
  ])
})
```

再加:`~~~plantuml` 围栏;```` ```PUML ```` 大小写;```` ```go ```` 块原样留在 markdown 段;围栏内容里的 ```` ``` ```` 行不提前结束块;无图表时返回**单个** markdown 段且 text 等于原串;空串返回 `[]`。

- [ ] **Step 2: 跑测试确认失败** —— `npm run test -- src/tests/markdown-segments.spec.ts`,预期:找不到 `../markdown-segments`。

- [ ] **Step 3: 实现** —— 在 `web/src/markdown-segments.ts` 写 `splitDiagramBlocks(source: string): Segment[]`。
  逐行扫描,开栏正则 `/^ {0,3}(`{3,}|~{3,})[ \t]*([^\n]*)$/`,取 info string 第一个词小写后与 `DIAGRAM_LANGUAGES = ['plantuml', 'puml']` 比对;闭栏要求同字符、长度不小于开栏、且无 info string。开栏前的内容进 markdown 段(带它原有的换行),围栏内部按 `\n` join 后作为 `text`。文件头写一段注释说明为什么在 marked 之前切分(规格 §1 的取舍:嵌套围栏会被提到顶层)。

- [ ] **Step 4: 跑测试确认通过** —— `npm run test -- src/tests/markdown-segments.spec.ts`,预期:全绿。

- [ ] **Step 5: 提交**

```bash
git add web/src/markdown-segments.ts web/src/tests/markdown-segments.spec.ts
git commit -m "feat(web): split plantuml fences out of agent markdown before rendering"
```

---

### Task 2: 渲染引擎模块(串行队列 + 缓存 + 错误判定)

**Files:**
- Modify: `web/package.json`(dependencies 增加 `"@plantuml/core": "1.2026.8"`)
- Create: `web/src/plantuml/renderer.ts`
- Test: `web/src/tests/plantuml-renderer.spec.ts`

**Interfaces:**
- Consumes: 无
- Produces:
  ```ts
  export interface DiagramRequest { source: string; dark: boolean }
  export type DiagramResult = { ok: true; svg: string } | { ok: false; reason: string }
  export type Engine = (lines: string[], opts: { dark: boolean },
    onSuccess: (svg: string) => void, onError: (message: unknown) => void) => void
  export function configureEngineLoader(loader: Engine | null): void   // 测试注入;null 恢复真实引擎
  export function renderDiagram(req: DiagramRequest): Promise<DiagramResult>
  export function clearDiagramCache(): void
  export const MAX_SOURCE_LINES = 2000
  export const RENDER_TIMEOUT_MS = 15000
  ```

- [ ] **Step 1: 装依赖并确认懒加载切块**

```bash
cd web && npm install @plantuml/core@1.2026.8 --save-exact
```

预期:`package.json` 的 dependencies 里出现 `@plantuml/core`。

（已验证过:`viz-global.js` 的 UMD 与内部 `require("url")` 分支能过 Vite 8 的构建与预构建,产物在
Chromium 里渲染成功 —— 所以走 npm 路线。规格里写的 vendoring 兵底保留为应急方案:仅当本任务
Step 6 或 Task 5 跑不通时启用,把两个文件放进 `web/public/plantuml/` 后运行时注入 `<script>`。）

- [ ] **Step 2: 写失败测试** —— `web/src/tests/plantuml-renderer.spec.ts`,用假引擎断言行为。假引擎:

```ts
function fakeEngine(render: (lines: string[], opts: { dark: boolean }) => string | Promise<string>): Engine {
  return (lines, opts, onSuccess, onError) => { void Promise.resolve(render(lines, opts)).then(onSuccess, onError) }
}
```

必须覆盖的用例(`beforeEach` 里 `clearDiagramCache()`、`configureEngineLoader(null)`,`afterEach` 同样收尾):

```ts
it('returns the engine SVG sanitized', async () => {
  configureEngineLoader(fakeEngine(() => '<svg><text onload="alert(1)">hi</text></svg>'))
  const r = await renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })
  expect(r.ok).toBe(true)
  expect((r as { svg: string }).svg).toContain('<svg')
  expect((r as { svg: string }).svg).not.toContain('onload')
})
it('treats the engine error board as a failure', async () => {
  configureEngineLoader(fakeEngine(() => '<svg><text>PlantUML version 1.2026.8</text><text>Fatal parsing error</text></svg>'))
  const r = await renderDiagram({ source: '@startuml\n!include <C4/C4_Container>\n@enduml', dark: false })
  expect(r).toEqual({ ok: false, reason: expect.stringContaining('PlantUML') })
})
it('reports the engine error message when it throws', async () => {
  configureEngineLoader((_l, _o, _ok, err) => err('java.lang.IndexOutOfBoundsException'))
  const r = await renderDiagram({ source: '@startuml\nA->B', dark: false })
  expect(r).toEqual({ ok: false, reason: expect.stringContaining('IndexOutOfBounds') })
})
it('refuses a source longer than MAX_SOURCE_LINES without touching the engine', async () => {
  let calls = 0
  configureEngineLoader(fakeEngine(() => { calls++; return '<svg/>' }))
  const r = await renderDiagram({ source: Array(MAX_SOURCE_LINES + 1).fill('A -> B').join('\n'), dark: false })
  expect(calls).toBe(0)
  expect(r.ok).toBe(false)
})
it('renders one diagram at a time even when several are requested together', async () => {
  let live = 0, peak = 0
  configureEngineLoader((lines, _o, ok) => { live++; peak = Math.max(peak, live)
    setTimeout(() => { live--; ok('<svg/>' + lines.join()) }, 5) })
  await Promise.all([1, 2, 3].map((i) => renderDiagram({ source: `@startuml\nA${i}->B\n@enduml`, dark: false })))
  expect(peak).toBe(1)
})
it('fails fast for later requests once the engine stops answering', async () => {
  vi.useFakeTimers(); configureEngineLoader(() => {})  // 永不回调
  const stuck = renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })
  await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
  expect(await stuck).toEqual({ ok: false, reason: expect.stringContaining('超时') })
  const next = await renderDiagram({ source: '@startuml\nC->D\n@enduml', dark: false })
  expect(next.ok).toBe(false)
  vi.useRealTimers()
})
it('answers a repeated request from the cache and keeps dark separate', async () => {
  let calls = 0
  configureEngineLoader(fakeEngine(() => { calls++; return '<svg/>' }))
  const src = '@startuml\nA->B\n@enduml'
  await renderDiagram({ source: src, dark: false })
  await renderDiagram({ source: src, dark: false })
  await renderDiagram({ source: src, dark: true })
  expect(calls).toBe(2)
})
```

- [ ] **Step 3: 跑测试确认失败** —— `npm run test -- src/tests/plantuml-renderer.spec.ts`,预期:找不到 `../plantuml/renderer`。

- [ ] **Step 4: 实现** —— 在 `web/src/plantuml/renderer.ts` 写上面 Interfaces 的全部导出:
  - 真实引擎:`async function realEngine(): Promise<Engine>` —— 先 `await import('@plantuml/core/viz-global.js')`(副作用导入,只成功一次),再 `const { renderToString } = await import('@plantuml/core/plantuml.js')`,把 `renderToString(lines, ok, err, opts)` 包成 `Engine`。`configureEngineLoader` 保存注入值。
  - 模块级 `let tail: Promise<unknown> = Promise.resolve()`、`let engineStuck = false`。请求先入队:`tail = tail.then(run, run)`;`run` 在 `engineStuck` 为真时立即返回 `{ ok: false, reason: 'PlantUML 渲染引擎已停止响应' }`。
  - 每个请求 `Promise.race([engineCall, timeout(RENDER_TIMEOUT_MS)])`;超时置 `engineStuck = true`,reason 含"超时"。
  - 缓存 `Map<string, DiagramResult>`,key = `${dark ? 'd' : 'l'}\u0000${source}`;只缓存成功结果;命中时重新 set 以实现 LRU;`CACHE_LIMIT = 32`,超出时删除最先插入的键(Map 迭代序即插入序)。
  - 长度闸门:按行切分后 > `MAX_SOURCE_LINES` 直接 `{ ok: false, reason: '图源码过长(超过 2000 行)' }`,不排队不调引擎。
  - `maxSvgSize` 不传,用引擎默认的 8192 px;超限时引擎会走 `onError`,自然落进失败态。
  - 错误画板判定 `ERROR_MARK = /PlantUML version|Fatal parsing error|Syntax Error|Empty description|not a diagram/`。
  - 成功路径:`DOMPurify.sanitize(svg)` 后再缓存。`onError` 的入参用 `String(...)` 取第一行、截 200 字符作为 reason。
  - 文件头注释写清三条实测事实(并发挂死、报错画板不 reject、`{dark}` 在第 4 参数),它们就是这段代码存在的理由。

- [ ] **Step 5: 跑测试确认通过** —— `npm run test -- src/tests/plantuml-renderer.spec.ts`,预期:全绿。

- [ ] **Step 6: 确认引擎没进主包** —— `npm run build`,预期:构建成功,`dist/assets/` 下 plantuml 相关 chunk 独立于入口 chunk(入口 js 不含 3.5 MB 的那个文件)。若 `viz-global.js` 被打进入口 chunk,把它的静态 `import` 改成同一个动态 `import()` 里先 `await` 它 —— 入口 chunk 必须保持原量级。

- [ ] **Step 7: 提交**

```bash
git add web/package.json web/package-lock.json web/src/plantuml/renderer.ts web/src/tests/plantuml-renderer.spec.ts
git commit -m "feat(web): add a serialized in-browser PlantUML renderer"
```

---

### Task 3: `PlantumlBlock` 组件(方案 C 的标题行)

**Files:**
- Modify: `web/src/theme/presets.ts`(给 `ThemePreset` 加 `dark: boolean`,四套配色分别置 `false/true/false/false`,并导出 `isDarkTheme(key: string): boolean`)
- Create: `web/src/components/PlantumlBlock.vue`
- Test: `web/src/tests/plantuml-block.spec.ts`

**Interfaces:**
- Consumes: Task 2 的 `renderDiagram({ source, dark }): Promise<DiagramResult>`、`configureEngineLoader`、`clearDiagramCache`
- Produces:
  ```ts
  // props
  { source: string; ready: boolean }
  ```
  渲染出的 DOM 钩子:`[data-test="plantuml-block"]`、`.plantuml-head`(按钮,`aria-expanded`)、
  `[data-test="plantuml-canvas"]`(内部是 SVG)、`[data-test="plantuml-source"]`(`<pre>`)、
  `[data-test="plantuml-error"]`、`[data-test="plantuml-waiting"]`。

- [ ] **Step 1: 写失败测试** —— `web/src/tests/plantuml-block.spec.ts`。`beforeEach`:`setActivePinia(createPinia())`、`clearDiagramCache()`;用 `configureEngineLoader` 注入假引擎。用例:

```ts
it('shows the diagram by default once ready', async () => {
  configureEngineLoader(fakeEngine(() => '<svg><text>seq</text></svg>'))
  const w = mount(PlantumlBlock, { props: { source: '@startuml\nA->B\n@enduml', ready: true } })
  await flushPromises()
  expect(w.find('[data-test="plantuml-canvas"]').html()).toContain('<svg')
  expect(w.find('[data-test="plantuml-source"]').exists()).toBe(false)
  expect(w.find('.plantuml-head').text()).toContain('图表 · plantuml')
})
it('switches to the source and back with the head row', async () => {
  configureEngineLoader(fakeEngine(() => '<svg/>'))
  const w = mount(PlantumlBlock, { props: { source: '@startuml\nA->B\n@enduml', ready: true } })
  await flushPromises()
  await w.find('.plantuml-head').trigger('click')
  expect(w.find('[data-test="plantuml-source"]').text()).toContain('@startuml')
  expect(w.find('[data-test="plantuml-canvas"]').exists()).toBe(false)
  expect(w.find('.plantuml-head').attributes('aria-expanded')).toBe('false')
  await w.find('.plantuml-head').trigger('click')
  expect(w.find('[data-test="plantuml-canvas"]').exists()).toBe(true)
})
it('waits for a closed fence and never asks the engine while streaming', async () => {
  let calls = 0
  configureEngineLoader(fakeEngine(() => { calls++; return '<svg/>' }))
  const w = mount(PlantumlBlock, { props: { source: '@startuml\nA->B', ready: false } })
  await flushPromises()
  expect(calls).toBe(0)
  expect(w.find('[data-test="plantuml-waiting"]').exists()).toBe(true)
  await w.setProps({ ready: true })
  await flushPromises()
  expect(calls).toBe(1)
})
it('reveals the source with the reason when rendering fails', async () => {
  configureEngineLoader(fakeEngine(() => { throw new Error('boom') }))
  const w = mount(PlantumlBlock, { props: { source: '@startuml\n!include <C4/C4_Container>\n@enduml', ready: true } })
  await flushPromises()
  expect(w.find('[data-test="plantuml-error"]').text()).toContain('无法渲染')
  expect(w.find('[data-test="plantuml-source"]').text()).toContain('!include')
  expect(w.find('[data-test="plantuml-canvas"]').exists()).toBe(false)
})
it('renders in dark mode for the dark palette only', async () => {
  const seen: boolean[] = []
  configureEngineLoader(fakeEngine((_l, opts) => { seen.push(opts.dark); return '<svg/>' }))
  const settings = useSettingsStore()
  settings.appearance = { ...settings.appearance, theme: 'cool' }
  const w1 = mount(PlantumlBlock, { props: { source: '@startuml\nA->B\n@enduml', ready: true } })
  await flushPromises()
  settings.appearance = { ...settings.appearance, theme: 'dark' }
  const w2 = mount(PlantumlBlock, { props: { source: '@startuml\nA->B\n@enduml', ready: true } })
  await flushPromises()
  expect(seen).toEqual([false, true])
})
```

- [ ] **Step 2: 跑测试确认失败** —— `npm run test -- src/tests/plantuml-block.spec.ts`,预期:找不到 `../components/PlantumlBlock.vue`。

- [ ] **Step 3: 实现** —— 先改 `web/src/theme/presets.ts`:加 `dark: boolean` 字段、四套预设赋值、导出 `isDarkTheme(key: string): boolean`(未知 key 返回 `false`)。

再写 `web/src/components/PlantumlBlock.vue`,`<script setup lang="ts">`:

```ts
const props = defineProps<{ source: string; ready: boolean }>()
const mode = ref<'preview' | 'source'>('preview')   // 默认预览,状态只属于这一块
const dark = computed(() => isDarkTheme(useSettingsStore().appearance.theme))
const state = ref<'waiting' | 'rendering' | 'ready' | 'failed'>('waiting')
const svg = ref(''); const reason = ref('')
```

- `watch(() => [props.ready, dark.value], draw, { immediate: true })`:`ready` 为假 → `state='waiting'` 且**不请求**;已画好同一 `source`+`dark` 组合就不重画;否则 `state='rendering'` → `renderDiagram` → 成功 `state='ready'`,失败 `mode='source'` 且 `state='failed'`。
- 模板:`.plantuml-block` 里一个 `<button class="plantuml-head" :aria-expanded="String(state === 'ready' && mode === 'preview')">`,左侧 `▾/▸` + `图表 · plantuml`,右侧 `<span class="plantuml-mode">`;右侧文案:`waiting` → `等待完成…`,`rendering` → `正在生成图…`,`ready` 时随 `mode` 显示"查看源码 / 返回预览",`failed` 时不显示动作。
- 主体:`state !== 'failed' && mode === 'preview'` 时画布(`<div data-test="plantuml-canvas">` 内有 SVG 就 `v-html`);
  `waiting`/`rendering` 时一行提示(`data-test="plantuml-waiting"`);`failed || mode === 'source'` 时
  `<pre class="plantuml-src" data-test="plantuml-source">{{ source }}</pre>`,失败时上面加一行
  `[data-test="plantuml-error"]`。根元素带 `data-test="plantuml-block"`。
- 样式复用 MessageView 里 Thinking/工具块的观感:`--color-surface-muted` 标题行、`--color-border` 分隔、`--color-primary` 的动作文字、`--radius`、密度变量;画布给 `background: var(--color-surface)` 与 `max-height: 60vh; overflow: auto;` 防超长图撑爆消息;SVG `max-width: 100%`。

- [ ] **Step 4: 跑测试确认通过** —— `npm run test -- src/tests/plantuml-block.spec.ts`,预期:全绿。

- [ ] **Step 5: 类型检查** —— `npm run typecheck`,预期:无错误(新增的 `dark` 字段若让 `presets.ts` 的调用处报错,一并补齐)。

- [ ] **Step 6: 提交**

```bash
git add web/src/theme/presets.ts web/src/components/PlantumlBlock.vue web/src/tests/plantuml-block.spec.ts
git commit -m "feat(web): add the PlantumlBlock component with preview/source toggle"
```

---

### Task 4: 接入 `MessageView`

**Files:**
- Modify: `web/src/components/MessageView.vue`(agent 气泡的渲染分支)
- Test: `web/src/tests/message-view.spec.ts`(追加一个 describe)

**Interfaces:**
- Consumes: Task 1 的 `splitDiagramBlocks`、Task 3 的 `PlantumlBlock`(`{ source, ready }`)
- Produces: agent 气泡 DOM —— `.bubble-agent .message-text` 内按段顺序排布 `.markdown-seg`(v-html)与 PlantumlBlock。

- [ ] **Step 1: 写失败测试** —— 在 `web/src/tests/message-view.spec.ts` 追加(文件顶部补 `import { createPinia, setActivePinia } from 'pinia'`、`import { clearDiagramCache, configureEngineLoader } from '../plantuml/renderer'`,`beforeEach` 里 `setActivePinia(createPinia()); clearDiagramCache()`;沿用文件已有的 `message()` 与 `capabilities()` 辅助):

```ts
describe('MessageView plantuml blocks', () => {
  it('renders a plantuml fence as a diagram block, not as a code block', async () => {
    configureEngineLoader((_l, _o, ok) => ok('<svg><text>seq</text></svg>'))
    const w = mount(MessageView, { props: { messages: [message('assistant', '看图\n\n```plantuml\n@startuml\nA->B\n@enduml\n```\n\n完毕')], capabilities: capabilities() } })
    await flushPromises()
    const block = w.find('[data-test="plantuml-block"]')
    expect(block.exists()).toBe(true)
    expect(block.find('[data-test="plantuml-canvas"] svg').exists()).toBe(true)
    expect(w.find('.bubble-agent .message-text').text()).toContain('看图')
    expect(w.find('.bubble-agent .message-text').text()).toContain('完毕')
    expect(w.find('.bubble-agent').html()).not.toContain('<code>@startuml')
  })
  it('keeps an unterminated fence waiting while the reply streams', () => {
    const entry = { ...message('assistant', '```plantuml\n@startuml\nA->B'), streaming: true }
    const w = mount(MessageView, { props: { messages: [entry], capabilities: capabilities() } })
    expect(w.find('[data-test="plantuml-waiting"]').exists()).toBe(true)
  })
  it('leaves non-plantuml code blocks alone', () => {
    const w = mount(MessageView, { props: { messages: [message('assistant', '```go\nfmt.Println("hi")\n```')], capabilities: capabilities() } })
    expect(w.find('[data-test="plantuml-block"]').exists()).toBe(false)
    expect(w.find('.bubble-agent code').text()).toContain('fmt.Println')
  })
})
```

- [ ] **Step 2: 跑测试确认失败** —— `npm run test -- src/tests/message-view.spec.ts`,预期:新 describe 的三个用例失败(找不到 plantuml-block)。

- [ ] **Step 3: 实现** —— `MessageView.vue`:
  - 顶部 `import { splitDiagramBlocks, type Segment } from '../markdown-segments'` 与 `import PlantumlBlock from './PlantumlBlock.vue'`。
  - `function segmentsOf(text: string): Segment[] { return splitDiagramBlocks(text) }`(直接内联在模板里调用也可)。
  - agent 分支从 `<div class="message-text markdown-body" v-html="renderMarkdown(entry.text)"></div>` 改成:同一个 `.message-text.markdown-body` 容器里 `v-for="(seg, i) in splitDiagramBlocks(entry.text)"` + `:key="i",`markdown` 段渲染 `<div class="markdown-seg" v-html="renderMarkdown(seg.text)"></div>`,`plantuml` 段渲染 `<PlantumlBlock :source="seg.text" :ready="seg.closed && entry.streaming !== true" />`。
  - 其余分支(user turn、thinking、tool_call/tool_result)与文件头注释同步更新一句:agent 回复里 plantuml 围栏走 PlantumlBlock。
  - `.markdown-seg` 不额外加边距 —— 段间距由原有块级 margin 决定,视觉与今天一致。

- [ ] **Step 4: 跑测试确认通过** —— `npm run test -- src/tests/message-view.spec.ts`,预期:全绿(原有 8 个用例一并跑一遍,不许回归)。

- [ ] **Step 5: 全量单测 + 类型检查** —— `npm run test`、`npm run typecheck`,预期:全绿。

- [ ] **Step 6: 提交**

```bash
git add web/src/components/MessageView.vue web/src/tests/message-view.spec.ts
git commit -m "feat(web): render plantuml fences in agent replies as diagram blocks"
```

---

### Task 5: 真实引擎的端到端验证

**Files:**
- Create: `web/e2e/plantuml.spec.ts`

**Interfaces:**
- Consumes: Task 1–4 的全部产物;E2E 脚手架(`npm run test:e2e`,Vite dev + 真后端 + 假 agent 可执行文件)
- Produces: 无(终端验证)

- [ ] **Step 1: 写 E2E** —— 参照 `web/e2e/sessions.spec.ts` 与 `web/e2e/task-history.spec.ts` 里"登录 → 建项目 → 起会话 → 收到 agent 回复"的既有写法(用 `e2e/fixtures/bin` 里的假 agent 决定回复内容,不要新造登录流程)。三个断言:
  1. agent 回复含闭合 ```` ```plantuml ```` 块 → 页面出现 `[data-test="plantuml-block"]`,且其 `[data-test="plantuml-canvas"] svg` 存在(引擎真的画出了图,不是占位);
  2. 点 `.plantuml-head` → `[data-test="plantuml-source"]` 文本含 `@startuml`;
  3. agent 回复含 `!include <C4/C4_Container>` 的块 → `[data-test="plantuml-error"]` 出现且 `[data-test="plantuml-source"]` 展示源码。

- [ ] **Step 2: 跑 E2E** —— `npm run test:e2e -- plantuml.spec.ts`,预期:三个断言全过。第一条尤其关键:它证明 npm 依赖路线在 dev server 与真浏览器里都成立(`viz-global.js` 的 `require("url")` 分支在浏览器里不会执行)。

- [ ] **Step 3: 手动看一眼真实观感** —— `./dev.sh` 起平台,发一句话让 agent 出一段 plantuml(或直接跑一条含图的历史会话),在四套主题(light/dark/warm/cool)与三档密度下确认:默认是图、切换跟手、超长图在块内滚动、深色下图与气泡底不糊。

- [ ] **Step 4: 提交**

```bash
git add web/e2e/plantuml.spec.ts
git commit -m "test(e2e): cover plantuml diagram preview and source toggle in the chat"
```

---

## 完成标准

`cd web && npm run test && npm run typecheck && npm run build` 全绿,`npm run test:e2e -- plantuml.spec.ts` 全绿,入口 bundle 未增长 3.5 MB 量级,`git log` 上每个任务一个提交。
