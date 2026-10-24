import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup, configure } from '@testing-library/react'

configure({ testIdAttribute: 'data-test' })

// antd relies on browser APIs jsdom does not implement. Keep these polyfills in
// one place so every React test starts from the same environment. Source-scan
// guards opt into the node environment, so every browser access is guarded.
if (typeof window !== 'undefined') {
  if (!window.matchMedia) {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }),
    })
  }

  if (!globalThis.ResizeObserver) {
    class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
    globalThis.ResizeObserver = ResizeObserver as unknown as typeof ResizeObserver
  }

  if (!window.requestAnimationFrame) {
    window.requestAnimationFrame = (callback: FrameRequestCallback) =>
      window.setTimeout(() => callback(performance.now()), 0)
    window.cancelAnimationFrame = (handle: number) => window.clearTimeout(handle)
  }

  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {}
  }

  if (!window.scrollTo) {
    window.scrollTo = () => {}
  }

  if (!Range.prototype.getBoundingClientRect) {
    Range.prototype.getBoundingClientRect = () => ({
      x: 0,
      y: 0,
      width: 0,
      height: 0,
      top: 0,
      right: 0,
      bottom: 0,
      left: 0,
      toJSON: () => ({}),
    })
  }
}

afterEach(() => {
  if (typeof window === 'undefined') return
  cleanup()
  localStorage.clear()
  sessionStorage.clear()
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-density')
})
