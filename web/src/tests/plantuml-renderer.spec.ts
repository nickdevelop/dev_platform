// The PlantUML engine is a third-party, single-threaded, untrusted-input
// renderer. These tests pin the four properties the chat depends on and cannot
// get from the engine itself: one diagram at a time, a timeout that does not
// wedge the queue, the error board the engine paints instead of reporting, and
// a cache that keeps dark and light drawings apart.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearDiagramCache,
  configureEngineLoader,
  MAX_SOURCE_CHARS,
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

  it('explains the two engine failures a user can act on in Chinese', async () => {
    const engine = (message: string): Engine => (_lines, _opts, _ok, onError) => onError(message)

    configureEngineLoader(engine('Diagram too large for browser rendering: 37493x8790 (max 8192; override via maxSvgSize)'))
    expect(await renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })).toEqual({
      ok: false,
      reason: expect.stringContaining('图太大'),
    })

    configureEngineLoader(engine('java.lang.IndexOutOfBoundsException'))
    const truncated = await renderDiagram({ source: '@startuml\nB->C\n@enduml', dark: false })
    expect(truncated).toEqual({ ok: false, reason: expect.stringContaining('@enduml') })

    // An unrecognized engine message stays visible: it is the only clue there is.
    configureEngineLoader(engine('java.lang.WeirdThing'))
    expect(await renderDiagram({ source: '@startuml\nC->D\n@enduml', dark: false })).toEqual({
      ok: false,
      reason: expect.stringContaining('java.lang.WeirdThing'),
    })
  })

  it('draws a diagram whose labels only talk about errors', async () => {
    // Diagrams of error handling are what an agent draws. Measured against the
    // real engine: every error board carries `PlantUML version`, and every
    // successful drawing does not — so that, not the English prose, is the mark.
    configureEngineLoader(fakeEngine(() => '<svg><text>Syntax Error</text><text>Empty description</text></svg>'))
    expect(await renderDiagram({ source: '@startuml\nA --> B : Syntax Error\n@enduml', dark: false })).toEqual({
      ok: true,
      svg: '<svg><text>Syntax Error</text><text>Empty description</text></svg>',
    })

    configureEngineLoader(
      fakeEngine(() => '<svg><text>Syntax Error</text><text>PlantUML version 1.2026.8</text></svg>'),
    )
    expect((await renderDiagram({ source: '@startuml\nA --> B : Syntax Error\n@enduml', dark: true })).ok).toBe(false)
  })

  it('answers the same diagram asked for twice with one engine call', async () => {
    let calls = 0
    configureEngineLoader((_lines, _opts, onSuccess) => {
      calls++
      setTimeout(() => onSuccess('<svg/>'), 10)
    })
    const source = '@startuml\nA->B\n@enduml'
    const both = await Promise.all([renderDiagram({ source, dark: false }), renderDiagram({ source, dark: false })])
    expect(calls).toBe(1)
    expect(both.every((result) => result.ok)).toBe(true)
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

  it('refuses a source that is long in characters but not in lines', async () => {
    // One 30 kB line is a participant list written without newlines, and the
    // engine is synchronous: the line count alone does not bound the freeze.
    let calls = 0
    configureEngineLoader(
      fakeEngine(() => {
        calls++
        return '<svg/>'
      }),
    )
    const result = await renderDiagram({ source: `@startuml
A -> B : ${'x'.repeat(MAX_SOURCE_CHARS)}
@enduml`, dark: false })
    expect(calls).toBe(0)
    expect(result).toEqual({ ok: false, reason: expect.stringContaining('过长') })
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

  it('probes once more before concluding the engine is gone for good', async () => {
    vi.useFakeTimers()
    let calls = 0
    configureEngineLoader(() => {
      // Never answers: the engine wedged.
      calls++
    })
    const stuck = renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await stuck).toEqual({ ok: false, reason: expect.stringContaining('超时') })

    // An engine observed to drop one callback still answers the next request, so
    // the following request is tried instead of refused.
    const probe = renderDiagram({ source: '@startuml\nB->C\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await probe).toEqual({ ok: false, reason: expect.stringContaining('超时') })
    expect(calls).toBe(2)

    // Two timeouts in a row: now it stays out of the way.
    expect(await renderDiagram({ source: '@startuml\nC->D\n@enduml', dark: false })).toEqual({
      ok: false,
      reason: expect.stringContaining('停止响应'),
    })
    expect(calls).toBe(2)
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
