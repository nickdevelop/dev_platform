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

// A reply longer than this is not a diagram anyone can look at, and handing it
// to the engine risks the whole page instead of one block.
export const MAX_SOURCE_LINES = 2000

// How long to wait for one drawing before concluding the engine wedged.
export const RENDER_TIMEOUT_MS = 15000

// Diagrams kept for the lifetime of the page: switching preview and source back
// and forth, or scrolling through a long history, must not redraw anything.
const CACHE_LIMIT = 32

// What the engine paints inside the SVG when it could not parse the source.
const ERROR_MARK = /PlantUML version|Fatal parsing error|Syntax Error|Empty description|not a diagram/

let injectedEngine: Engine | null = null
let realEngine: Engine | null = null
// The queue: one drawing in flight, everything else behind it.
let queue: Promise<unknown> = Promise.resolve()
let wedged = false
const cache = new Map<string, DiagramResult>()

function cacheKey(request: DiagramRequest): string {
  return `${request.dark ? 'd' : 'l'}\u0000${request.source}`
}

function fail(reason: unknown): DiagramResult {
  const first = String(reason).split('\n')[0].slice(0, 200)
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
  if (wedged) return fail('PlantUML 渲染引擎已停止响应')

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
      // A wedged engine never recovers, so later blocks fail fast instead of
      // each waiting out their own timeout.
      wedged = true
      finish(fail(`渲染超时(超过 ${RENDER_TIMEOUT_MS / 1000} 秒)`))
    }, RENDER_TIMEOUT_MS)

    try {
      engine(request.source.split('\n'), { dark: request.dark }, (svg) => {
        if (ERROR_MARK.test(svg)) {
          finish(fail('PlantUML 无法解析这段图源码'))
        } else {
          finish({ ok: true, svg: DOMPurify.sanitize(svg) })
        }
      }, (message) => finish(fail(message)))
    } catch (error) {
      finish(fail(error))
    }
  })
}

export function renderDiagram(request: DiagramRequest): Promise<DiagramResult> {
  if (request.source.split('\n').length > MAX_SOURCE_LINES) {
    return Promise.resolve(fail(`图源码过长(超过 ${MAX_SOURCE_LINES} 行)`))
  }

  const key = cacheKey(request)
  const cached = cache.get(key)
  if (cached) {
    // Re-insert so the cache falls out oldest-first.
    cache.delete(key)
    cache.set(key, cached)
    return Promise.resolve(cached)
  }

  const running = queue.then(() => draw(request), () => draw(request))
  queue = running.catch(() => undefined)
  return running.then((result) => {
    if (result.ok) {
      cache.set(key, result)
      const oldest = cache.keys().next()
      if (cache.size > CACHE_LIMIT && !oldest.done) cache.delete(oldest.value)
    }
    return result
  })
}

// Swap the engine out — used by tests, and by nothing in production. Resetting
// the queue and the wedged flag keeps a stale engine from leaking into the next
// subject under test.
export function configureEngineLoader(loader: Engine | null): void {
  injectedEngine = loader
  realEngine = null
  wedged = false
  queue = Promise.resolve()
}

export function clearDiagramCache(): void {
  cache.clear()
}
