<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import * as monaco from 'monaco-editor'
import { ensureMonacoEnvironment, resolveMonacoLanguage } from './monaco/env'
import { applyMonacoTheme, getMonacoFontFamily, getMonacoFontSize } from './monaco/theme'
import { useSettingsStore } from '../../stores/settings'
import type { CodeCommentCard } from './types'

const props = withDefaults(
  defineProps<{
    content: string
    filePath?: string
    showLineNumbers?: boolean
    initialLine?: number
    editable?: boolean
    saving?: boolean
    saveError?: string
    readonlyReason?: string
  }>(),
  {
    showLineNumbers: true,
    editable: false,
    saving: false,
    saveError: '',
    readonlyReason: '',
  },
)

const emit = defineEmits<{
  'add-comment': [card: CodeCommentCard]
  save: [content: string]
  'dirty-change': [dirty: boolean]
}>()

let settings: ReturnType<typeof useSettingsStore> | null = null
try {
  settings = useSettingsStore()
} catch {
  settings = null
}

const editorContainerRef = ref<HTMLElement | null>(null)
const wrapLines = ref(true)

// Direct editing (always-on when `editable`): no edit-mode toggle, the model is
// the draft and dirty state is a plain comparison against the persisted content.
const draftContent = ref(props.content)

const isDirty = computed(() => props.editable && draftContent.value !== props.content)

watch(isDirty, (dirty) => {
  emit('dirty-change', dirty)
})

const fileName = computed(() => {
  if (!props.filePath) return 'untitled'
  const parts = props.filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || props.filePath
})

const languageId = computed(() => resolveMonacoLanguage(props.filePath))

// Selection & inline comment state
const activeSelectionEndLine = ref<number | null>(null)
const selectedStartLine = ref(1)
const selectedEndLine = ref(1)
const selectedSnippet = ref('')
const selectionBtnTop = ref(0)
const inlineEditorVisible = ref(false)
const inlineCommentText = ref('')
const inlineCommentInputRef = ref<HTMLTextAreaElement | null>(null)

let editorInstance: monaco.editor.IStandaloneCodeEditor | null = null
let modelInstance: monaco.editor.ITextModel | null = null
let focusedDecorations: string[] = []
let syncingFromProps = false

function updateSelectionWidgetPosition() {
  if (!editorInstance || activeSelectionEndLine.value === null) return
  const pos = editorInstance.getScrolledVisiblePosition({
    lineNumber: activeSelectionEndLine.value,
    column: 1,
  })
  if (pos) {
    selectionBtnTop.value = Math.max(4, pos.top)
  }
}

function applyFocusedLine(lineNumber?: number) {
  if (!editorInstance || !modelInstance) return
  if (!lineNumber || lineNumber < 1 || lineNumber > modelInstance.getLineCount()) {
    focusedDecorations = editorInstance.deltaDecorations(focusedDecorations, [])
    return
  }
  editorInstance.revealLineInCenter(lineNumber)
  editorInstance.setPosition({ lineNumber, column: 1 })
  focusedDecorations = editorInstance.deltaDecorations(focusedDecorations, [
    {
      range: new monaco.Range(lineNumber, 1, lineNumber, 1),
      options: {
        isWholeLine: true,
        className: 'monaco-focused-line',
      },
    },
  ])
}

function saveEdits() {
  if (!props.editable || props.saving) return
  emit('save', draftContent.value)
}

function toggleWrap() {
  wrapLines.value = !wrapLines.value
  editorInstance?.updateOptions({ wordWrap: wrapLines.value ? 'on' : 'off' })
}

function openInlineCommentEditor() {
  inlineEditorVisible.value = true
  inlineCommentText.value = ''
  void nextTick(() => {
    if (typeof inlineCommentInputRef.value?.focus === 'function') {
      inlineCommentInputRef.value.focus()
    }
  })
}

function closeInlineCommentEditor() {
  inlineEditorVisible.value = false
  inlineCommentText.value = ''
  activeSelectionEndLine.value = null
}

function submitInlineComment() {
  const comment = inlineCommentText.value.trim()
  if (!comment) return

  const card: CodeCommentCard = {
    id: `comment-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    filePath: props.filePath || 'untitled',
    fileName: fileName.value,
    startLine: selectedStartLine.value,
    endLine: selectedEndLine.value,
    codeSnippet: selectedSnippet.value,
    comment,
    createdAt: Date.now(),
  }

  emit('add-comment', card)
  closeInlineCommentEditor()
}

function onInlineEditorKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    submitInlineComment()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    closeInlineCommentEditor()
  }
}

function onContainerKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    saveEdits()
    return
  }
  // Alt+Z toggles word wrap (VSCode habit). Bound on the container instead of
  // monaco's addCommand: monaco has no Alt+Z binding so keydowns bubble here,
  // and a single handler avoids a double toggle cancelling itself out.
  if (e.altKey && !e.ctrlKey && !e.metaKey && e.key.toLowerCase() === 'z') {
    e.preventDefault()
    toggleWrap()
  }
}

onMounted(() => {
  ensureMonacoEnvironment()
  if (!editorContainerRef.value) return

  const themeName = applyMonacoTheme(settings?.appearance.theme)
  const fontSize = getMonacoFontSize(settings?.appearance.fontSize)

  modelInstance = monaco.editor.createModel(props.content, languageId.value)
  editorInstance = monaco.editor.create(editorContainerRef.value, {
    model: modelInstance,
    theme: themeName,
    readOnly: !props.editable,
    automaticLayout: true,
    wordWrap: wrapLines.value ? 'on' : 'off',
    lineNumbers: props.showLineNumbers ? 'on' : 'off',
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    fontFamily: getMonacoFontFamily(),
    fontSize,
    tabSize: 2,
    insertSpaces: true,
    renderLineHighlight: 'line',
    padding: { top: 8, bottom: 12 },
    scrollbar: {
      verticalScrollbarSize: 8,
      horizontalScrollbarSize: 8,
    },
  })

  editorInstance.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
    saveEdits()
  })

  editorInstance.onDidChangeModelContent(() => {
    if (syncingFromProps || !modelInstance) return
    draftContent.value = modelInstance.getValue()
  })

  editorInstance.onDidChangeCursorSelection((e) => {
    if (!modelInstance) return
    const sel = e.selection
    if (sel.isEmpty()) {
      if (!inlineEditorVisible.value) {
        activeSelectionEndLine.value = null
      }
      return
    }
    const text = modelInstance.getValueInRange(sel)
    if (!text.trim()) {
      if (!inlineEditorVisible.value) {
        activeSelectionEndLine.value = null
      }
      return
    }
    selectedStartLine.value = Math.min(sel.startLineNumber, sel.endLineNumber)
    selectedEndLine.value = Math.max(sel.startLineNumber, sel.endLineNumber)
    selectedSnippet.value = text
    activeSelectionEndLine.value = selectedEndLine.value
    updateSelectionWidgetPosition()
  })

  editorInstance.onDidScrollChange(() => {
    updateSelectionWidgetPosition()
  })

  if (props.initialLine && props.initialLine > 0) {
    applyFocusedLine(props.initialLine)
  }
})

watch(
  () => props.filePath,
  () => {
    draftContent.value = props.content
    closeInlineCommentEditor()
    if (modelInstance) {
      monaco.editor.setModelLanguage(modelInstance, languageId.value)
      if (modelInstance.getValue() !== props.content) {
        syncingFromProps = true
        modelInstance.setValue(props.content)
        syncingFromProps = false
      }
    }
    emit('dirty-change', false)
  },
)

watch(
  () => props.content,
  (nextContent, prevContent) => {
    // Never clobber unsaved local edits; adopt external content only when the
    // draft still matches the previous persisted content (i.e. clean).
    if (draftContent.value !== prevContent) return
    draftContent.value = nextContent
    if (modelInstance && modelInstance.getValue() !== nextContent) {
        syncingFromProps = true
        modelInstance.setValue(nextContent)
        syncingFromProps = false
      }
  },
)

watch(
  () => props.showLineNumbers,
  (show) => {
    editorInstance?.updateOptions({ lineNumbers: show ? 'on' : 'off' })
  },
)

watch(
  () => props.initialLine,
  (line) => {
    if (line && line > 0) {
      void nextTick(() => applyFocusedLine(line))
    }
  },
)

watch(
  () => settings?.appearance.theme,
  (themeKey) => {
    applyMonacoTheme(themeKey)
  },
)

watch(
  () => settings?.appearance.fontSize,
  (size) => {
    editorInstance?.updateOptions({ fontSize: getMonacoFontSize(size) })
  },
)

onUnmounted(() => {
  editorInstance?.dispose()
  editorInstance = null
  modelInstance?.dispose()
  modelInstance = null
})
</script>

<template>
  <div class="code-view" data-test="code-view" @keydown="onContainerKeydown">
    <!-- Floating status chips (readonly / unsaved); editing is always on and
         saving goes through Ctrl+S, wrapping through Alt+Z — no buttons. -->
    <div class="code-actions">
      <span
        v-if="props.readonlyReason"
        class="readonly-badge"
        :title="props.readonlyReason"
        data-test="code-readonly-badge"
      >
        只读
      </span>
      <span v-if="isDirty" class="dirty-badge" data-test="code-dirty-badge">未保存</span>
    </div>

    <div v-if="props.saveError" class="save-error-banner" role="alert" data-test="code-save-error">
      {{ props.saveError }}
    </div>

    <div class="monaco-host-wrap">
      <div
        ref="editorContainerRef"
        class="monaco-editor-host"
        data-test="monaco-editor-host"
      ></div>

      <!-- Floating + button on selection end line -->
      <button
        v-if="activeSelectionEndLine !== null && !inlineEditorVisible"
        type="button"
        class="line-add-comment-btn"
        :style="{ top: `${selectionBtnTop}px` }"
        :title="`在第 ${activeSelectionEndLine} 行添加选区备注`"
        aria-label="添加选区备注"
        data-test="line-add-comment-btn"
        @click.stop="openInlineCommentEditor"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2.2">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
      </button>

      <!-- Inline Comment Editor overlay -->
      <div
        v-if="inlineEditorVisible && activeSelectionEndLine !== null"
        class="inline-comment-row"
        data-test="inline-comment-row"
        @mouseup.stop
        @click.stop
      >
        <div class="inline-comment-box" data-test="inline-comment-box">
          <div class="inline-comment-header">
            <div class="header-left">
              <svg class="file-icon" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M3 2h7l4 4v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z" />
                <polyline points="10 2 10 6 14 6" />
              </svg>
              <span class="file-name">{{ fileName }}</span>
              <span class="line-badge">L{{ selectedStartLine }}{{ selectedStartLine !== selectedEndLine ? `-${selectedEndLine}` : '' }}</span>
            </div>
            <button
              type="button"
              class="inline-close-btn"
              title="关闭 (Esc)"
              aria-label="关闭"
              @click="closeInlineCommentEditor"
            >
              ×
            </button>
          </div>
          <div class="inline-comment-body">
            <textarea
              ref="inlineCommentInputRef"
              v-model="inlineCommentText"
              class="inline-comment-textarea"
              data-test="inline-comment-textarea"
              rows="2"
              placeholder="添加备注… (Enter 确认)"
              @keydown="onInlineEditorKeydown"
            ></textarea>
            <div class="inline-comment-actions">
              <span class="shortcut-tip">Enter 确认 · Esc 取消</span>
              <div class="inline-btn-group">
                <button
                  type="button"
                  class="inline-btn cancel-btn"
                  data-test="inline-cancel-btn"
                  @click="closeInlineCommentEditor"
                >
                  取消
                </button>
                <button
                  type="button"
                  class="inline-btn submit-btn"
                  data-test="inline-submit-btn"
                  :disabled="!inlineCommentText.trim()"
                  @click="submitInlineComment"
                >
                  添加卡片
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.code-view {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface);
  overflow: hidden;
  font-family: var(--font-mono);
}

.code-actions {
  position: absolute;
  top: 8px;
  right: 14px;
  z-index: 5;
  display: flex;
  gap: 6px;
}

.readonly-badge,
.dirty-badge {
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 8px;
  border-radius: var(--radius);
  font-size: var(--text-size-11);
  font-family: var(--font-family);
}

.readonly-badge {
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  border: 1px solid var(--color-border);
}

.dirty-badge {
  background: var(--color-warning-soft);
  color: var(--color-warning);
  border: 1px solid var(--color-warning-border);
}

.save-error-banner {
  padding: 6px 12px;
  background: var(--color-danger-soft);
  color: var(--color-danger);
  border-bottom: 1px solid var(--color-danger-border);
  font-size: var(--text-size-12);
  font-family: var(--font-family);
  flex-shrink: 0;
}

.monaco-host-wrap {
  position: relative;
  flex: 1;
  min-height: 0;
  width: 100%;
  overflow: hidden;
}

.monaco-editor-host {
  width: 100%;
  height: 100%;
}

.line-add-comment-btn {
  position: absolute;
  left: 4px;
  z-index: 6;
  width: 18px;
  height: 18px;
  border-radius: var(--radius);
  border: 1px solid var(--color-primary);
  background: var(--color-primary);
  color: var(--color-on-primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.line-add-comment-btn:hover {
  background: var(--color-primary-hover);
}

.inline-comment-row {
  position: absolute;
  left: 16px;
  right: 16px;
  bottom: 12px;
  z-index: 10;
}

.inline-comment-box {
  background: var(--color-surface);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  box-shadow: 0 0 0 1px var(--color-border), 0 4px 12px rgba(0, 0, 0, 0.12);
  overflow: hidden;
  font-family: var(--font-family);
}

.inline-comment-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  background: var(--color-surface-muted);
  border-bottom: 1px solid var(--color-border);
  font-size: var(--text-size-12);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text);
}

.file-name {
  font-weight: 600;
}

.line-badge {
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  padding: 0 6px;
  border-radius: var(--radius);
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.inline-close-btn {
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  font-size: var(--text-size-14);
  line-height: 1;
}

.inline-comment-body {
  padding: 8px 10px;
}

.inline-comment-textarea {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  padding: 6px 8px;
  font-size: var(--text-size-12);
  font-family: var(--font-family);
  resize: vertical;
  outline: none;
}

.inline-comment-textarea:focus {
  border-color: var(--color-primary);
}

.inline-comment-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 6px;
}

.shortcut-tip {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.inline-btn-group {
  display: flex;
  gap: 6px;
}

.inline-btn {
  height: 24px;
  padding: 0 10px;
  border-radius: var(--radius);
  font-size: var(--text-size-12);
  cursor: pointer;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
}

.inline-btn.submit-btn {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-on-primary);
}

.inline-btn.submit-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

:deep(.monaco-focused-line) {
  background: var(--color-primary-soft);
}
</style>
