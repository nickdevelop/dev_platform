import { vi } from 'vitest'

class WorkerStub {
  postMessage(): void {}
  terminate(): void {}
  addEventListener(): void {}
  removeEventListener(): void {}
}

vi.mock('monaco-editor/editor/editor.worker?worker', () => ({ default: WorkerStub }))
vi.mock('monaco-editor/language/json/json.worker?worker', () => ({ default: WorkerStub }))
vi.mock('monaco-editor/language/css/css.worker?worker', () => ({ default: WorkerStub }))
vi.mock('monaco-editor/language/html/html.worker?worker', () => ({ default: WorkerStub }))
vi.mock('monaco-editor/language/typescript/ts.worker?worker', () => ({ default: WorkerStub }))

interface MockModel {
  _value: string
  _language: string
  _listeners: Array<() => void>
  getValue: () => string
  setValue: (v: string) => void
  getLineCount: () => number
  getValueInRange: (range: { startLineNumber: number; endLineNumber: number }) => string
  dispose: () => void
}

function createMockModel(value = '', language = 'plaintext'): MockModel {
  const model: MockModel = {
    _value: value,
    _language: language,
    _listeners: [],
    getValue() {
      return model._value
    },
    setValue(v: string) {
      model._value = v
      model._listeners.forEach((fn) => fn())
    },
    getLineCount() {
      return model._value ? model._value.split('\n').length : 1
    },
    getValueInRange(range) {
      const lines = model._value.split('\n')
      const start = Math.max(1, Math.min(range.startLineNumber, range.endLineNumber))
      const end = Math.min(lines.length, Math.max(range.startLineNumber, range.endLineNumber))
      return lines.slice(start - 1, end).join('\n')
    },
    dispose() {
      model._listeners = []
    },
  }
  return model
}

class MockRange {
  startLineNumber: number
  startColumn: number
  endLineNumber: number
  endColumn: number
  constructor(startLineNumber: number, startColumn: number, endLineNumber: number, endColumn: number) {
    this.startLineNumber = startLineNumber
    this.startColumn = startColumn
    this.endLineNumber = endLineNumber
    this.endColumn = endColumn
  }
}

export const monacoTestRegistry = {
  lastEditor: null as any,
  lastDiffEditor: null as any,
}

vi.mock('monaco-editor', () => {
  const definedThemes = new Map<string, unknown>()

  return {
    Range: MockRange,
    KeyMod: { CtrlCmd: 2048 },
    KeyCode: { KeyS: 49 },
    editor: {
      defineTheme(name: string, data: unknown) {
        definedThemes.set(name, data)
      },
      setTheme(_name: string) {},
      setModelLanguage(model: MockModel, lang: string) {
        model._language = lang
      },
      createModel(value: string, language = 'plaintext') {
        return createMockModel(value, language)
      },
      create(container: HTMLElement, options: Record<string, any> = {}) {
        const model: MockModel = options.model ?? createMockModel('')
        const contentListeners: Array<() => void> = []
        const selectionListeners: Array<(e: any) => void> = []
        const scrollListeners: Array<() => void> = []
        const currentOptions = { ...options }

        const syncDom = () => {
          container.dataset.monacoValue = model.getValue()
          container.dataset.monacoReadonly = String(Boolean(currentOptions.readOnly))
          container.dataset.monacoWrap = String(currentOptions.wordWrap ?? 'off')
          container.dataset.monacoLanguage = model._language
          container.textContent = model.getValue()
        }

        const notifyModelChange = () => {
          syncDom()
          contentListeners.forEach((fn) => fn())
        }
        model._listeners.push(notifyModelChange)
        syncDom()

        const instance = {
          _options: currentOptions,
          _model: model,
          _focusedLine: null as number | null,
          getModel: () => model,
          updateOptions(next: Record<string, any>) {
            Object.assign(currentOptions, next)
            syncDom()
          },
          focus: vi.fn(),
          addCommand: vi.fn(),
          revealLineInCenter: vi.fn((line: number) => {
            instance._focusedLine = line
            container.dataset.focusedLine = String(line)
          }),
          setPosition: vi.fn(),
          deltaDecorations: vi.fn((_old: string[], next: Array<{ range: MockRange }>) => {
            if (next.length > 0) {
              instance._focusedLine = next[0].range.startLineNumber
              container.dataset.focusedLine = String(next[0].range.startLineNumber)
            }
            return next.map((_, i) => `dec-${i}`)
          }),
          getScrolledVisiblePosition: vi.fn((pos: { lineNumber: number }) => ({
            top: pos.lineNumber * 18,
            left: 0,
            height: 18,
          })),
          onDidChangeModelContent(fn: () => void) {
            contentListeners.push(fn)
            return { dispose: () => {} }
          },
          onDidChangeCursorSelection(fn: (e: any) => void) {
            selectionListeners.push(fn)
            return { dispose: () => {} }
          },
          onDidScrollChange(fn: () => void) {
            scrollListeners.push(fn)
            return { dispose: () => {} }
          },
          _triggerSelection(startLine: number, endLine: number) {
            const sel = {
              startLineNumber: startLine,
              endLineNumber: endLine,
              isEmpty: () => false,
            }
            selectionListeners.forEach((fn) => fn({ selection: sel }))
          },
          dispose() {
            container.textContent = ''
          },
        }
        monacoTestRegistry.lastEditor = instance
        return instance
      },
      createDiffEditor(container: HTMLElement, options: Record<string, any> = {}) {
        const currentOptions = { ...options }
        let currentModels: { original: MockModel; modified: MockModel } | null = null

        const syncDom = () => {
          container.dataset.sideBySide = String(Boolean(currentOptions.renderSideBySide))
          if (currentModels) {
            container.dataset.original = currentModels.original.getValue()
            container.dataset.modified = currentModels.modified.getValue()
            container.dataset.language = currentModels.modified._language
            container.textContent = `${currentModels.original.getValue()}\n${currentModels.modified.getValue()}`
          }
        }

        const instance = {
          _options: currentOptions,
          setModel(models: { original: MockModel; modified: MockModel }) {
            currentModels = models
            syncDom()
          },
          updateOptions(next: Record<string, any>) {
            Object.assign(currentOptions, next)
            syncDom()
          },
          dispose() {
            container.textContent = ''
          },
        }
        monacoTestRegistry.lastDiffEditor = instance
        return instance
      },
    },
  }
})
