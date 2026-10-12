// The PlantUML engine is a third-party, single-threaded, untrusted-input
// renderer. These tests pin the four properties the chat depends on and cannot
// get from the engine itself: one diagram at a time, a timeout that does not
// wedge the queue, the error board the engine paints instead of reporting, and
// a cache that keeps dark and light drawings apart.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearDiagramCache,
  configureEngineLoader,
  MAX_SOURCE_LINES,
  renderDiagram,
  RENDER_TIMEOUT_MS,
  type Engine,
} from '../plantuml/renderer'

// A test engine that answers synchronously (or rejects) with whatever the case
// needs. Mirrors the real engine's callback shape.
function fakeEngine(
  render: (lines: string[], opts: { dark: boolean }) => string | Promise<string>,
): Engine {
  return (lines, opts, onSuccess, onError) => {
    void Promise.resolve(render(lines, opts)).then(onSuccess, onError)
  }
}

describe('plantuml renderer', () => {
  beforeEach(() => {
    clearDiagramCache()
    configureEngineLoader(null)
  })

  afterEach(() => {
    clearDiagramCache()
    configureEngineLoader(null)
    vi.useRealTimers()
  })

  it('returns the engine SVG sanitized', async () => {
    configureEngineLoader(fakeEngine(() => '<svg><text onload="alert(1)">hi</text></svg>'))
    const result = await renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })
    expect(result.ok).toBe(true)
    const svg = (result as { svg: string }).svg
    expect(svg).toContain('<svg')
    expect(svg).not.toContain('onload')
  })

  it('treats the engine error board as a failure', async () => {
    configureEngineLoader(
      fakeEngine(() => '<svg><text>PlantUML version 1.2026.8</text><text>Fatal parsing error</text></svg>'),
    )
    const result = await renderDiagram({
      source: '@startuml\n!include <C4/C4_Container>\n@enduml',
      dark: false,
    })
    expect(result).toEqual({ ok: false, reason: expect.stringContaining('PlantUML') })
  })

  it('reports the engine error message when it throws', async () => {
    configureEngineLoader((_lines, _opts, _ok, onError) => onError('java.lang.IndexOutOfBoundsException'))
    const result = await renderDiagram({ source: '@startuml\nA->B', dark: false })
    expect(result).toEqual({ ok: false, reason: expect.stringContaining('IndexOutOfBounds') })
  })

  it('refuses a source longer than MAX_SOURCE_LINES without touching the engine', async () => {
    let calls = 0
    configureEngineLoader(
      fakeEngine(() => {
        calls++
        return '<svg/>'
      }),
    )
    const result = await renderDiagram({
      source: Array(MAX_SOURCE_LINES + 1)
        .fill('A -> B')
        .join('\n'),
      dark: false,
    })
    expect(calls).toBe(0)
    expect(result.ok).toBe(false)
  })

  it('renders one diagram at a time even when several are requested together', async () => {
    let live = 0
    let peak = 0
    configureEngineLoader((_lines, _opts, onSuccess) => {
      live++
      peak = Math.max(peak, live)
      setTimeout(() => {
        live--
        onSuccess('<svg/>')
      }, 5)
    })
    const pending = [1, 2, 3].map((i) => renderDiagram({ source: `@startuml\nA${i}->B\n@enduml`, dark: false }))
    const results = await Promise.all(pending)
    expect(peak).toBe(1)
    expect(results.every((result) => result.ok)).toBe(true)
  })

  it('fails fast for later requests once the engine stops answering', async () => {
    vi.useFakeTimers()
    configureEngineLoader(() => {
      // Never answers: the engine wedged.
    })
    const stuck = renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await stuck).toEqual({ ok: false, reason: expect.stringContaining('超时') })
    const next = await renderDiagram({ source: '@startuml\nC->D\n@enduml', dark: false })
    expect(next.ok).toBe(false)
  })

  it('answers a repeated request from the cache and keeps dark separate', async () => {
    let calls = 0
    configureEngineLoader(
      fakeEngine(() => {
        calls++
        return '<svg/>'
      }),
    )
    const source = '@startuml\nA->B\n@enduml'
    await renderDiagram({ source, dark: false })
    await renderDiagram({ source, dark: false })
    await renderDiagram({ source, dark: true })
    expect(calls).toBe(2)
  })

  it('passes the dark flag through to the engine', async () => {
    const seen: { dark: boolean }[] = []
    configureEngineLoader(
      fakeEngine((_lines, opts) => {
        seen.push(opts)
        return '<svg/>'
      }),
    )
    await renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: true })
    expect(seen).toEqual([{ dark: true }])
  })
})
