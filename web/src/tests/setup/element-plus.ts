// Global Vitest setup for the platform's test environment.
//
// Element Plus measures its widgets through ResizeObserver (a table sizes its
// columns, a select measures its popper, a dialog its body). jsdom ships no
// implementation, so the library's layout pass throws before a component can
// mount and the spec fails for a reason that has nothing to do with the
// behaviour under test. This stub answers "nothing resized yet", which is what
// a real observer reports on the first tick anyway.
class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver
}
