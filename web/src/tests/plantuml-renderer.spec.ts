// The PlantUML engine is a third-party, single-threaded, untrusted-input
// renderer. These tests pin the four properties the chat depends on and cannot
// get from the engine itself: one diagram at a time, a timeout that does not
// wedge the queue, the error board the engine paints instead of reporting, and
// a cache that keeps dark and light drawings apart.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearDiagramCache,
  configureEngineLoader,
  encodePlantUML,
  MAX_SOURCE_CHARS,
  MAX_SOURCE_LINES,
  renderDiagram,
  RENDER_TIMEOUT_MS,
  WEDGE_COOLDOWN_MS,
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

  it('believes an engine that answers after a timeout', async () => {
    // The wedge is about an engine that stopped answering, so any answer is
    // evidence against it. Without this, one timeout plus one working diagram
    // plus one slow request silences the page while the engine is replying.
    vi.useFakeTimers()
    let mode = 'stuck'
    let calls = 0
    configureEngineLoader((lines, _opts, onSuccess, onError) => {
      calls++
      if (mode === 'stuck') return
      if (/bad/.test(lines.join(''))) onError('java.lang.WeirdThing')
      else onSuccess('<svg/>')
    })
    const first = renderDiagram({ source: '@startuml\nA->B\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await first).toEqual({ ok: false, reason: expect.stringContaining('超时') })

    // The engine answers the next request — with a failure, which is still an
    // answer. This is the case that matters: a draw that answers is believed,
    // one that fails on purpose must not be held against it.
    mode = 'live'
    expect((await renderDiagram({ source: '@startuml\nbad\n@enduml', dark: false })).ok).toBe(false)

    // So a later slow request is tried rather than refused.
    mode = 'stuck'
    const last = renderDiagram({ source: '@startuml\nE->F\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await last).toEqual({ ok: false, reason: expect.stringContaining('超时') })
    expect(calls).toBe(3)
  })

  it('gives a wedged engine another chance instead of silencing the page', async () => {
    // Two timeouts refuse the next request; that refusal must not be forever,
    // because a refused request cannot answer and so could never lift it.
    vi.useFakeTimers()
    let mode = 'stuck'
    let calls = 0
    configureEngineLoader((lines, _opts, onSuccess, onError) => {
      calls++
      if (mode === 'stuck') return
      if (/bad/.test(lines.join(''))) onError('java.lang.WeirdThing')
      else onSuccess('<svg/>')
    })
    for (const source of ['@startuml\nA->B\n@enduml', '@startuml\nC->D\n@enduml']) {
      const pending = renderDiagram({ source, dark: false })
      await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
      expect(await pending).toEqual({ ok: false, reason: expect.stringContaining('超时') })
    }
    expect(await renderDiagram({ source: '@startuml\nE->F\n@enduml', dark: false })).toEqual({
      ok: false,
      reason: expect.stringContaining('停止响应'),
    })
    expect(calls).toBe(2)

    // After the cooldown the engine is asked again, and an answer — even a
    // failure — puts it back in credit for the next slow one.
    await vi.advanceTimersByTimeAsync(WEDGE_COOLDOWN_MS)
    mode = 'live'
    expect((await renderDiagram({ source: '@startuml\nbad\n@enduml', dark: false })).ok).toBe(false)
    expect(calls).toBe(3)

    mode = 'stuck'
    const slow = renderDiagram({ source: '@startuml\nG->H\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await slow).toEqual({ ok: false, reason: expect.stringContaining('超时') })
    expect(calls).toBe(4)

    // One slow draw after an answer is not a wedge: it takes two in a row
    // again, so the next request is tried instead of refused.
    const next = renderDiagram({ source: '@startuml\nI->J\n@enduml', dark: false })
    await vi.advanceTimersByTimeAsync(RENDER_TIMEOUT_MS)
    expect(await next).toEqual({ ok: false, reason: expect.stringContaining('超时') })
    expect(calls).toBe(5)
  })

  it('asks the engine again for a diagram that failed before', async () => {
    let calls = 0
    let works = false
    configureEngineLoader((_lines, _opts, onSuccess, onError) => {
      calls++
      if (works) onSuccess('<svg/>')
      else onError('java.lang.WeirdThing')
    })
    const source = '@startuml\nA->B\n@enduml'
    expect((await renderDiagram({ source, dark: false })).ok).toBe(false)
    works = true
    expect((await renderDiagram({ source, dark: false })).ok).toBe(true)
    expect(calls).toBe(2)
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

  describe('online rendering', () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    it('encodes data using plantuml 64-char alphabet without prefix', () => {
      const data = new Uint8Array([1, 2, 3])
      const encoded = encodePlantUML(data)
      expect(encoded).not.toMatch(/^~/)
      expect(encoded).toMatch(/^[0-9A-Za-z_-]+$/)
    })

    it('renders via GET with no prefix when POST fails or is rejected', async () => {
      const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          return Promise.reject(new Error('CORS or network error'))
        }
        expect(url).toContain('/svg/')
        // Verify URL contains no ~1 prefix
        expect(url).not.toMatch(/\/svg\/~1/)
        return Promise.resolve(new Response('<svg><text>rendered</text></svg>', { status: 200 }))
      })
      globalThis.fetch = fetchMock

      const result = await renderDiagram({
        source: '@startuml\nA->B\n@enduml',
        dark: false,
        mode: 'online',
        serverUrl: 'https://www.plantuml.com/plantuml',
      })

      expect(result.ok).toBe(true)
      if (result.ok) {
        expect(result.svg).toContain('<svg')
        expect(result.svg).toContain('rendered')
      }
    })

    it('reports failure when online server returns an error board', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue(
        new Response('<svg><text>PlantUML version 1.2026.9</text><text>Fatal parsing error</text></svg>', {
          status: 200,
        }),
      )

      const result = await renderDiagram({
        source: '@startuml\nbad puml\n@enduml',
        dark: false,
        mode: 'online',
        serverUrl: 'https://www.plantuml.com/plantuml',
      })

      expect(result.ok).toBe(false)
      if (!result.ok) {
        expect(result.reason).toContain('无法解析')
      }
    })

    it('reports failure when network is down', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Network unreachable'))

      const result = await renderDiagram({
        source: '@startuml\nA->B\n@enduml',
        dark: false,
        mode: 'online',
        serverUrl: 'https://www.plantuml.com/plantuml',
      })

      expect(result.ok).toBe(false)
      if (!result.ok) {
        expect(result.reason).toContain('在线渲染')
      }
    })
  })
})
