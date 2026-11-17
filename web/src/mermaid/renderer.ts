// In-browser Mermaid diagram rendering for chat and markdown previews.
import DOMPurify from 'dompurify'
import mermaid from 'mermaid'

export interface MermaidRequest {
  source: string
  dark: boolean
  themeKey?: string
}

export type MermaidResult = { ok: true; svg: string } | { ok: false; reason: string }

export type MermaidEngine = (
  source: string,
  opts: { dark: boolean; themeKey?: string; id: string },
) => Promise<string>

export const MAX_SOURCE_LINES = 500
export const MAX_SOURCE_CHARS = 30_000
const CACHE_LIMIT = 64

let injectedEngine: MermaidEngine | null = null
let currentConfigKey = ''
let sequenceId = 0

const cache = new Map<string, MermaidResult>()
const inFlight = new Map<string, Promise<MermaidResult>>()

export function configureMermaidEngine(engine: MermaidEngine | null): void {
  injectedEngine = engine
}

export function clearMermaidCache(): void {
  cache.clear()
  inFlight.clear()
}

function nextDiagramId(): string {
  sequenceId = (sequenceId + 1) % 1_000_000
  return `mermaid-${Date.now().toString(36)}-${sequenceId}`
}

function cacheKey(request: MermaidRequest): string {
  const theme = request.themeKey || (request.dark ? 'dark' : 'light')
  return `${theme}\u0000${request.source}`
}

function normalizeMermaidError(err: unknown): string {
  if (err instanceof Error) {
    const msg = err.message || ''
    const firstLine = msg.split('\n')[0].replace(/^Error:\s*/, '')
    return firstLine || '语法错误或无法渲染'
  }
  return typeof err === 'string' ? err : '图表解析失败'
}

function buildThemeVariables(themeKey: string, dark: boolean) {
  if (dark || themeKey === 'dark') {
    return {
      darkMode: true,
      fontFamily: 'inherit',
      textColor: '#c9d1d9',
      nodeTextColor: '#c9d1d9',
      mainBkg: '#1e1e2e',
      nodeBorder: '#454558',
      lineColor: '#8b949e',
      actorTextColor: '#c9d1d9',
      signalColor: '#8b949e',
      signalTextColor: '#c9d1d9',
      labelTextColor: '#c9d1d9',
    }
  }

  if (themeKey === 'warm') {
    return {
      darkMode: false,
      fontFamily: 'inherit',
      primaryColor: '#f9ead7',
      primaryTextColor: '#3a3226',
      primaryBorderColor: '#d3c4a8',
      textColor: '#3a3226',
      nodeTextColor: '#3a3226',
      mainBkg: '#fffaf0',
      nodeBorder: '#d3c4a8',
      lineColor: '#7a6f5c',
      actorTextColor: '#3a3226',
      signalColor: '#7a6f5c',
      signalTextColor: '#3a3226',
      labelTextColor: '#3a3226',
    }
  }

  if (themeKey === 'cool') {
    return {
      darkMode: false,
      fontFamily: 'inherit',
      primaryColor: '#dcefec',
      primaryTextColor: '#1e293b',
      primaryBorderColor: '#c2ccda',
      textColor: '#1e293b',
      nodeTextColor: '#1e293b',
      mainBkg: '#f8fafc',
      nodeBorder: '#c2ccda',
      lineColor: '#5b6b82',
      actorTextColor: '#1e293b',
      signalColor: '#5b6b82',
      signalTextColor: '#1e293b',
      labelTextColor: '#1e293b',
    }
  }

  // Default light theme
  return {
    darkMode: false,
    fontFamily: 'inherit',
    primaryColor: '#e8effd',
    primaryTextColor: '#1f2328',
    primaryBorderColor: '#cbd5e1',
    textColor: '#1f2328',
    nodeTextColor: '#1f2328',
    mainBkg: '#ffffff',
    nodeBorder: '#cbd5e1',
    lineColor: '#64748b',
    actorTextColor: '#1f2328',
    signalColor: '#64748b',
    signalTextColor: '#1f2328',
    labelTextColor: '#1f2328',
  }
}

async function renderRealEngine(
  source: string,
  opts: { dark: boolean; themeKey?: string; id: string },
): Promise<string> {
  const themeKey = opts.themeKey || (opts.dark ? 'dark' : 'light')
  const configKey = `${opts.dark ? 'd' : 'l'}:${themeKey}`

  if (currentConfigKey !== configKey) {
    const isDark = opts.dark || themeKey === 'dark'
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'loose',
      theme: isDark ? 'dark' : themeKey === 'light' ? 'default' : 'neutral',
      themeVariables: buildThemeVariables(themeKey, isDark),
      fontFamily: 'inherit',
      // htmlLabels: false forces Mermaid to emit pure SVG <text>/<tspan> elements,
      // preventing DOMPurify from stripping foreignObject content and making all text visible.
      htmlLabels: false,
      flowchart: { htmlLabels: false, useMaxWidth: true },
      sequence: { useMaxWidth: true },
      gantt: { useMaxWidth: true },
      journey: { useMaxWidth: true },
      class: { useMaxWidth: true },
      state: { useMaxWidth: true },
      er: { useMaxWidth: true },
      suppressErrorRendering: true,
    })
    currentConfigKey = configKey
  }

  const { svg } = await mermaid.render(opts.id, source)
  return svg
}

export async function renderMermaidDiagram(request: MermaidRequest): Promise<MermaidResult> {
  const trimmed = request.source.trim()
  if (!trimmed) {
    return { ok: false, reason: '图表源码为空' }
  }

  const lineCount = trimmed.split('\n').length
  if (lineCount > MAX_SOURCE_LINES || trimmed.length > MAX_SOURCE_CHARS) {
    return { ok: false, reason: '图表内容过大，超出渲染上限' }
  }

  const key = cacheKey(request)
  const cached = cache.get(key)
  if (cached) return cached

  const active = inFlight.get(key)
  if (active) return active

  const id = nextDiagramId()
  const engine = injectedEngine ?? renderRealEngine

  const promise = (async (): Promise<MermaidResult> => {
    try {
      const rawSvg = await engine(trimmed, {
        dark: request.dark,
        themeKey: request.themeKey,
        id,
      })
      const sanitized = DOMPurify.sanitize(rawSvg, {
        USE_PROFILES: { svg: true, svgFilters: true },
        ADD_TAGS: ['use', 'style', 'text', 'tspan', 'g', 'marker', 'pattern', 'defs'],
        ADD_ATTR: [
          'dominant-baseline',
          'text-anchor',
          'dy',
          'dx',
          'transform',
          'fill',
          'stroke',
          'style',
          'class',
        ],
      })
      const result: MermaidResult = { ok: true, svg: sanitized }
      if (cache.size >= CACHE_LIMIT) {
        const oldest = cache.keys().next().value
        if (oldest !== undefined) cache.delete(oldest)
      }
      cache.set(key, result)
      return result
    } catch (err) {
      const reason = normalizeMermaidError(err)
      return { ok: false, reason }
    } finally {
      inFlight.delete(key)
    }
  })()

  inFlight.set(key, promise)
  return promise
}
