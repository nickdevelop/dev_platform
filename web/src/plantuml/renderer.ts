// In-browser PlantUML rendering for the chat. Three properties of the engine
// (docs/superpowers/specs/2026-10-12-plantuml-preview-design.md, "约束与已实测的事实")
// shape this module and cannot be worked around elsewhere:
//
//  1. It renders one diagram at a time. Requests handed to it concurrently
//     never come back, so every request here goes through a single queue.
//  2. A diagram it cannot parse is not an error: it answers with a normal-looking
//     SVG that has the failure painted inside it (`PlantUML version …`,
//     `Fatal parsing error`). Only a missing `@enduml` or an oversized diagram
//     reaches the error callback, so both paths are failures here.
//  3. `renderToString` takes `{ dark }` as its fourth argument, which is how a
//     dark palette gets a dark drawing instead of white-on-white.
//
// The source is agent output, so the SVG is sanitized before anyone mounts it.
import DOMPurify from 'dompurify'

export interface DiagramRequest {
  source: string
  dark: boolean
}

export type DiagramResult = { ok: true; svg: string } | { ok: false; reason: string }

// The engine call, shaped the way this module needs it. The real engine is
// wrapped into this signature once; tests hand in their own.
export type Engine = (
  lines: string[],
  opts: { dark: boolean },
  onSuccess: (svg: string) => void,
  onError: (message: unknown) => void,
) => void

// What one synchronous render costs, measured in Chromium against the real
// engine with the main thread blocked for the whole call: a 50-line diagram
// takes ~0.9 s, a 100-line one ~1.4 s, and anything much larger is either
// refused by the engine's own size cap or costs tens of seconds. `RENDER_TIMEOUT_MS`
// cannot interrupt a render — the callback only arrives once the work is done —
// so these gates are the only protection, and they are set to seconds of freeze
// rather than to what a diagram could theoretically hold. Rendering in a worker
// would remove the freeze altogether; see the spec's follow-ups.
export const MAX_SOURCE_LINES = 400

// A single line can be a whole participant list written without newlines, so
// the line count alone does not bound the cost.
export const MAX_SOURCE_CHARS = 20_000

// How long to wait for one drawing before concluding the engine wedged. Only a
// dropped callback can reach it: a slow render finishes first.
export const RENDER_TIMEOUT_MS = 15000

// Two timeouts in a row and the engine is treated as gone: the next request is
// tried (an engine that drops one callback often answers the next), then later
// ones are refused instead of each burning another timeout.
const TIMEOUTS_BEFORE_WEDGE = 2

// Diagrams kept for the lifetime of the page: switching preview and source back
// and forth, or scrolling through a long history, must not redraw anything.
const CACHE_LIMIT = 32

// What the engine paints when it could not parse the source: an SVG that looks
// like a drawing but holds a failure report. Measured against the real engine,
// every error board carries this header and no successful drawing does — the
// English prose under it (`Syntax Error`, `Empty description`) is ordinary label
// text in a diagram *about* errors, so it cannot be the mark.
const ERROR_BOARD = /PlantUML version/

// Engine messages worth translating because the person reading them can act on
// them. Anything else is shown as the engine wrote it: it is the only clue there
// is.
const ENGINE_MESSAGES: [RegExp, string][] = [
  [/too large for browser/i, '图太大,浏览器里画不出来'],
  [/IndexOutOfBounds/, '图源码不完整,常见原因是缺少 @enduml'],
]

let injectedEngine: Engine | null = null
let realEngine: Engine | null = null
// The queue: one drawing in flight, everything else behind it.
let queue: Promise<unknown> = Promise.resolve()
let timeouts = 0
const cache = new Map<string, DiagramResult>()
// Requests already in the queue, so the same diagram asked for twice in one
// reply costs one render and one freeze, not two.
const inFlight = new Map<string, Promise<DiagramResult>>()

function cacheKey(request: DiagramRequest): string {
  return `${request.dark ? 'd' : 'l'}\u0000${request.source}`
}

function fail(reason: unknown): DiagramResult {
  const first = String(reason).split('\n')[0].slice(0, 200)
  for (const [pattern, message] of ENGINE_MESSAGES) {
    if (pattern.test(first)) return { ok: false, reason: `${message}(${first})` }
  }
  return { ok: false, reason: first }
}

// The engine is ~1.7 MB of gzipped JavaScript: it is fetched the first time a
// reply actually contains a diagram, never on page load.
async function loadEngine(): Promise<Engine> {
  if (injectedEngine) return injectedEngine
  if (realEngine) return realEngine
  // The Graphviz layout must exist before the engine runs.
  await import('@plantuml/core/viz-global.js')
  const { renderToString } = await import('@plantuml/core/plantuml.js')
  realEngine = (lines, opts, onSuccess, onError) => {
    renderToString(lines, onSuccess, onError, { dark: opts.dark })
  }
  return realEngine
}

async function draw(request: DiagramRequest): Promise<DiagramResult> {
  if (timeouts >= TIMEOUTS_BEFORE_WEDGE) return fail('PlantUML 渲染引擎已停止响应')

  let engine: Engine
  try {
    engine = await loadEngine()
  } catch (error) {
    return fail(`PlantUML 引擎未能加载:${error instanceof Error ? error.message : error}`)
  }

  return new Promise<DiagramResult>((resolve) => {
    let settled = false
    const finish = function (result: DiagramResult): void {
      if (settled) return
      settled = true
      clearTimeout(timer)
      resolve(result)
    }
    const timer = setTimeout(() => {
      if (settled) return
      // A dropped callback may be the engine wedging or may be one lost
      // callback it recovers from, so this only counts; the next request is
      // still tried, and the one after that is not.
      timeouts++
      finish(fail(`渲染超时(超过 ${RENDER_TIMEOUT_MS / 1000} 秒)`))
    }, RENDER_TIMEOUT_MS)

    try {
      engine(request.source.split('\n'), { dark: request.dark }, (svg) => {
        if (ERROR_BOARD.test(svg)) {
          finish(fail('PlantUML 无法解析这段图源码'))
        } else {
          timeouts = 0
          finish({ ok: true, svg: DOMPurify.sanitize(svg) })
        }
      }, (message) => finish(fail(message)))
    } catch (error) {
      finish(fail(error))
    }
  })
}

export function renderDiagram(request: DiagramRequest): Promise<DiagramResult> {
  const lines = request.source.split('\n')
  if (lines.length > MAX_SOURCE_LINES || request.source.length > MAX_SOURCE_CHARS) {
    return Promise.resolve(
      fail(`图源码过长(超过 ${MAX_SOURCE_LINES} 行或 ${MAX_SOURCE_CHARS} 字符)`),
    )
  }

  const key = cacheKey(request)
  const cached = cache.get(key)
  if (cached) {
    // Re-insert so the cache falls out oldest-first.
    cache.delete(key)
    cache.set(key, cached)
    return Promise.resolve(cached)
  }
  const alreadyQueued = inFlight.get(key)
  if (alreadyQueued) return alreadyQueued

  const running = queue.then(() => draw(request), () => draw(request))
  queue = running.catch(() => undefined)
  const settledResult = running.then((result) => {
    inFlight.delete(key)
    if (result.ok) {
      cache.set(key, result)
      const oldest = cache.keys().next()
      if (cache.size > CACHE_LIMIT && !oldest.done) cache.delete(oldest.value)
    }
    return result
  })
  inFlight.set(key, settledResult)
  return settledResult
}

// Swap the engine out — used by tests, and by nothing in production. Resetting
// the queue and the timeout count keeps a stale engine from leaking into the next
// subject under test.
export function configureEngineLoader(loader: Engine | null): void {
  injectedEngine = loader
  realEngine = null
  timeouts = 0
  queue = Promise.resolve()
  inFlight.clear()
}

export function clearDiagramCache(): void {
  cache.clear()
}
