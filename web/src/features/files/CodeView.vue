<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { highlightCode, splitHighlightedHtmlIntoLines } from '../../shared/highlight'
import type { CodeCommentCard } from './types'

const props = withDefaults(
  defineProps<{
    content: string
    filePath?: string
    showLineNumbers?: boolean
    initialLine?: number
  }>(),
  {
    showLineNumbers: true,
  },
)

const emit = defineEmits<{
  'add-comment': [card: CodeCommentCard]
}>()

const searchVisible = ref(false)
const searchQuery = ref('')
const searchIndex = ref(0)
const searchInputRef = ref<HTMLInputElement | null>(null)

const gotoLineVisible = ref(false)
const gotoLineInput = ref('')
const gotoLineInputRef = ref<HTMLInputElement | null>(null)
const focusedLineNumber = ref<number | null>(null)

const codeContainerRef = ref<HTMLElement | null>(null)
const copied = ref(false)
const wrapLines = ref(false)

const lines = computed(() => {
  if (!props.content) return []
  return props.content.split('\n')
})

const totalLines = computed(() => lines.value.length)

const highlightedLines = computed(() => {
  if (!props.content) return []
  const highlighted = highlightCode(props.content, props.filePath)
  return splitHighlightedHtmlIntoLines(highlighted)
})

// Search matches: line index and match offsets
interface SearchMatch {
  lineIndex: number
  startCol: number
  endCol: number
}

const matches = computed<SearchMatch[]>(() => {
  const q = searchQuery.value
  if (!q) return []
  const res: SearchMatch[] = []
  const lowerQ = q.toLowerCase()

  lines.value.forEach((line, lIdx) => {
    let start = 0
    const lowerLine = line.toLowerCase()
    while (start < lowerLine.length) {
      const idx = lowerLine.indexOf(lowerQ, start)
      if (idx === -1) break
      res.push({
        lineIndex: lIdx,
        startCol: idx,
        endCol: idx + q.length,
      })
      start = idx + q.length
    }
  })
  return res
})

const lineMatchesMap = computed(() => {
  const map = new Map<number, Array<{ match: SearchMatch; globalIndex: number }>>()
  if (!searchQuery.value) return map
  matches.value.forEach((m, gIdx) => {
    let arr = map.get(m.lineIndex)
    if (!arr) {
      arr = []
      map.set(m.lineIndex, arr)
    }
    arr.push({ match: m, globalIndex: gIdx })
  })
  return map
})

function renderLineWithHighlight(lineHtml: string, lineIdx: number): string {
  const lineMatches = lineMatchesMap.value.get(lineIdx)
  if (!lineMatches || lineMatches.length === 0) {
    return lineHtml || ' '
  }

  const tokens = lineHtml.split(/(<[^>]+>)/)
  let currCol = 0
  let result = ''

  for (const token of tokens) {
    if (!token) continue
    if (token.startsWith('<') && token.endsWith('>')) {
      result += token
      continue
    }

    const tokenLen = token.length
    const tokenStart = currCol
    const tokenEnd = currCol + tokenLen

    const intersecting = lineMatches.filter(
      (lm) => lm.match.startCol < tokenEnd && lm.match.endCol > tokenStart,
    )

    if (intersecting.length === 0) {
      result += token
    } else {
      let subIdx = 0
      while (subIdx < tokenLen) {
        const absCol = tokenStart + subIdx
        const activeMatch = intersecting.find(
          (lm) => absCol >= lm.match.startCol && absCol < lm.match.endCol,
        )

        if (activeMatch) {
          const matchEndInToken = Math.min(tokenLen, activeMatch.match.endCol - tokenStart)
          const matchedText = token.slice(subIdx, matchEndInToken)
          const isCurrent = activeMatch.globalIndex === searchIndex.value
          const currentClass = isCurrent ? ' is-current' : ''
          result += `<mark class="search-match${currentClass}">${matchedText}</mark>`
          subIdx = matchEndInToken
        } else {
          let nextMatchStartInToken = tokenLen
          for (const lm of intersecting) {
            if (lm.match.startCol > absCol) {
              const relStart = lm.match.startCol - tokenStart
              if (relStart < nextMatchStartInToken) {
                nextMatchStartInToken = relStart
              }
            }
          }
          result += token.slice(subIdx, nextMatchStartInToken)
          subIdx = nextMatchStartInToken
        }
      }
    }

    currCol += tokenLen
  }

  return result || ' '
}

const renderedLines = computed(() => {
  if (highlightedLines.value.length === 0) return []
  if (!searchQuery.value) return highlightedLines.value
  return highlightedLines.value.map((lineHtml, idx) => renderLineWithHighlight(lineHtml, idx))
})

function toggleSearch() {
  searchVisible.value = !searchVisible.value
  if (searchVisible.value) {
    gotoLineVisible.value = false
    void nextTick(() => {
      searchInputRef.value?.focus()
      searchInputRef.value?.select()
    })
  } else {
    searchQuery.value = ''
    searchIndex.value = 0
  }
}

function toggleGotoLine() {
  gotoLineVisible.value = !gotoLineVisible.value
  if (gotoLineVisible.value) {
    searchVisible.value = false
    gotoLineInput.value = focusedLineNumber.value ? String(focusedLineNumber.value) : ''
    void nextTick(() => {
      gotoLineInputRef.value?.focus()
      gotoLineInputRef.value?.select()
    })
  }
}

function scrollToLine(targetLine: number) {
  if (targetLine < 1 || targetLine > totalLines.value) return
  focusedLineNumber.value = targetLine
  void nextTick(() => {
    const lineEl = codeContainerRef.value?.querySelector(`[data-line="${targetLine}"]`)
    if (typeof lineEl?.scrollIntoView === 'function') {
      lineEl.scrollIntoView({ block: 'center', behavior: 'smooth' })
    }
  })
}

function handleGotoLineSubmit() {
  const parsed = parseInt(String(gotoLineInput.value || '').trim(), 10)
  if (!isNaN(parsed) && parsed >= 1 && parsed <= totalLines.value) {
    scrollToLine(parsed)
    gotoLineVisible.value = false
  }
}

watch(
  () => props.initialLine,
  (newLine) => {
    if (newLine && newLine > 0) {
      void nextTick(() => {
        scrollToLine(newLine)
      })
    }
  },
  { immediate: true },
)

function nextMatch() {
  if (matches.value.length === 0) return
  searchIndex.value = (searchIndex.value + 1) % matches.value.length
  scrollToCurrentMatch()
}

function prevMatch() {
  if (matches.value.length === 0) return
  searchIndex.value = (searchIndex.value - 1 + matches.value.length) % matches.value.length
  scrollToCurrentMatch()
}

function scrollToCurrentMatch() {
  const current = matches.value[searchIndex.value]
  if (!current || !codeContainerRef.value) return
  const lineEl = codeContainerRef.value.querySelector(`[data-line="${current.lineIndex + 1}"]`)
  if (typeof lineEl?.scrollIntoView === 'function') {
    lineEl.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }
}

function onKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
    e.preventDefault()
    toggleSearch()
  } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'g') {
    e.preventDefault()
    toggleGotoLine()
  } else if (e.key === 'Escape') {
    if (gotoLineVisible.value) {
      e.preventDefault()
      gotoLineVisible.value = false
    } else if (searchVisible.value) {
      e.preventDefault()
      toggleSearch()
    }
  }
}

async function copyCode() {
  try {
    await navigator.clipboard.writeText(props.content)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 1500)
  } catch {
    // Clipboard permission or failure fallback
  }
}

const fileName = computed(() => {
  if (!props.filePath) return 'untitled'
  const parts = props.filePath.split('/')
  return parts[parts.length - 1] || props.filePath
})

const activeSelectionEndLine = ref<number | null>(null)
const selectedStartLine = ref(1)
const selectedEndLine = ref(1)
const selectedSnippet = ref('')
const inlineEditorVisible = ref(false)
const inlineCommentText = ref('')
const inlineCommentInputRef = ref<HTMLTextAreaElement | null>(null)

function isLineInSelection(lineNum: number): boolean {
  if (activeSelectionEndLine.value === null) return false
  return lineNum >= selectedStartLine.value && lineNum <= selectedEndLine.value
}

function findLineNumber(node: Node | null): number | null {
  let curr = node instanceof HTMLElement ? node : node?.parentElement
  while (curr && curr !== codeContainerRef.value) {
    if (curr.dataset && curr.dataset.line) {
      const parsed = parseInt(curr.dataset.line, 10)
      if (!isNaN(parsed)) return parsed
    }
    curr = curr.parentElement
  }
  return null
}

function handleCodeSelection() {
  setTimeout(() => {
    const selection = window.getSelection()
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
      if (!inlineEditorVisible.value) {
        activeSelectionEndLine.value = null
      }
      return
    }

    const text = selection.toString()
    if (!text.trim()) {
      if (!inlineEditorVisible.value) {
        activeSelectionEndLine.value = null
      }
      return
    }

    const range = selection.getRangeAt(0)
    if (!codeContainerRef.value || !codeContainerRef.value.contains(range.commonAncestorContainer)) {
      return
    }

    const anchorLine = findLineNumber(selection.anchorNode)
    const focusLine = findLineNumber(selection.focusNode)

    if (anchorLine === null || focusLine === null) {
      return
    }

    selectedStartLine.value = Math.min(anchorLine, focusLine)
    selectedEndLine.value = Math.max(anchorLine, focusLine)
    selectedSnippet.value = text
    activeSelectionEndLine.value = selectedEndLine.value
  }, 10)
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
  window.getSelection()?.removeAllRanges()
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

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="code-view" data-test="code-view">
    <!-- Floating search toolbar -->
    <div v-if="searchVisible" class="search-bar" data-test="code-search-bar">
      <input
        ref="searchInputRef"
        v-model="searchQuery"
        type="text"
        class="search-bar-input"
        placeholder="在文件中查找…"
        @keydown.enter.prevent="nextMatch"
      />
      <span class="match-count">
        {{ matches.length > 0 ? `${searchIndex + 1}/${matches.length}` : searchQuery ? '无匹配' : '' }}
      </span>
      <button type="button" class="search-btn" title="上一个 (Shift+Enter)" @click="prevMatch">↑</button>
      <button type="button" class="search-btn" title="下一个 (Enter)" @click="nextMatch">↓</button>
      <button type="button" class="search-close" title="关闭 (Esc)" @click="toggleSearch">×</button>
    </div>

    <!-- Floating goto line toolbar (Ctrl+G) -->
    <div v-if="gotoLineVisible" class="search-bar goto-line-bar" data-test="goto-line-bar">
      <span class="goto-line-label">跳转到行:</span>
      <input
        ref="gotoLineInputRef"
        v-model="gotoLineInput"
        type="number"
        min="1"
        :max="totalLines"
        class="search-bar-input goto-line-input"
        :placeholder="`1 - ${totalLines}`"
        data-test="goto-line-input"
        @keydown.enter.prevent="handleGotoLineSubmit"
        @keydown.esc.prevent="gotoLineVisible = false"
      />
      <button
        type="button"
        class="search-btn"
        title="跳转 (Enter)"
        data-test="goto-line-submit-btn"
        @click="handleGotoLineSubmit"
      >
        跳转
      </button>
      <button
        type="button"
        class="search-close"
        title="关闭 (Esc)"
        @click="gotoLineVisible = false"
      >
        ×
      </button>
    </div>

    <!-- Quick action buttons -->
    <div class="code-actions">
      <button
        type="button"
        class="action-btn"
        :class="{ 'is-active': wrapLines }"
        :title="wrapLines ? '关闭自动换行' : '开启自动换行'"
        data-test="toggle-wrap-btn"
        @click="wrapLines = !wrapLines"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
          <polyline points="10 9 13 12 10 15" />
          <path d="M3 4h10v5H4" />
        </svg>
        <span>换行</span>
      </button>
    </div>

    <!-- Code rows with line numbers -->
    <div
      ref="codeContainerRef"
      class="code-scroller"
      data-test="code-scroller"
      @mouseup="handleCodeSelection"
    >
      <div class="code-table" :class="{ 'is-wrapped': wrapLines }">
        <template v-for="(lineHtml, idx) in renderedLines" :key="idx">
          <div
            class="code-row"
            :class="{
              'is-focused-line': focusedLineNumber === idx + 1,
              'is-in-selection': isLineInSelection(idx + 1),
            }"
            :data-line="idx + 1"
          >
            <div
              v-if="props.showLineNumbers"
              class="line-number"
              :class="{ 'has-comment-add-btn': activeSelectionEndLine === idx + 1 }"
              :data-line-number="idx + 1"
              aria-hidden="true"
            >
              <span class="line-num-text">{{ idx + 1 }}</span>
              <button
                v-if="activeSelectionEndLine === idx + 1 && !inlineEditorVisible"
                type="button"
                class="line-add-comment-btn"
                :title="`在第 ${idx + 1} 行添加选区备注`"
                aria-label="添加选区备注"
                data-test="line-add-comment-btn"
                @click.stop="openInlineCommentEditor"
              >
                <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2.2">
                  <line x1="8" y1="3" x2="8" y2="13" />
                  <line x1="3" y1="8" x2="13" y2="8" />
                </svg>
              </button>
            </div>
            <div class="code-line">
              <span class="code-text" v-html="lineHtml || ' '"></span>
            </div>
          </div>

          <!-- Inline Comment Editor attached right beneath the selection end row -->
          <div
            v-if="inlineEditorVisible && activeSelectionEndLine === idx + 1"
            class="inline-comment-row"
            data-test="inline-comment-row"
          >
            <div class="inline-comment-gutter" aria-hidden="true"></div>
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
                  :placeholder="`添加备注… (Enter 确认)`"
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
        </template>
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
  background: var(--color-surface, #ffffff);
  overflow: hidden;
  font-family: var(--font-mono);
}

.search-bar {
  position: absolute;
  top: 10px;
  right: 20px;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 4px;
  background: var(--color-surface, #ffffff);
  padding: 4px 8px;
  border-radius: 6px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  border: 1px solid var(--color-border, #e3e6eb);
}

.search-bar-input {
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: 4px;
  padding: 3px 6px;
  font-size: 0.8rem;
  font-family: var(--font-family);
  outline: none;
  width: 140px;
}

.search-bar-input:focus {
  border-color: var(--color-primary, #2563eb);
}

.match-count {
  font-size: var(--text-size-meta);
  color: var(--color-text-muted);
  min-width: 44px;
  text-align: center;
}

.search-btn,
.search-close {
  border: none;
  background: transparent;
  padding: 2px 6px;
  border-radius: 3px;
  cursor: pointer;
  font-size: 0.85rem;
  color: var(--color-text);
}

.search-btn:hover,
.search-close:hover {
  background: var(--color-surface-muted, #f8fafc);
}

.code-actions {
  position: absolute;
  top: 8px;
  right: 14px;
  z-index: 5;
  display: flex;
  gap: 6px;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface, #ffffff);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: var(--text-size-meta);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
  font-family: var(--font-family);
}

.action-btn:hover {
  color: var(--color-text);
  border-color: var(--color-border-strong, #d0d5dd);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}

.action-btn.is-active {
  background: var(--color-primary-soft, #e8effd);
  color: var(--color-primary, #2563eb);
  border-color: var(--color-primary, #2563eb);
  font-weight: 500;
}

.code-scroller {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px 0 24px;
}

.code-table {
  display: flex;
  flex-direction: column;
  min-width: 100%;
  font-size: 0.82rem;
  line-height: 1.55;
  width: max-content;
}

.code-table.is-wrapped {
  width: 100%;
  min-width: 100%;
}

.code-row {
  display: flex;
  align-items: flex-start;
  min-height: 22px;
  line-height: 22px;
  width: 100%;
}

.code-row:hover {
  background: var(--color-surface-muted, #f8fafc);
}

.code-row.is-in-selection {
  background: rgba(37, 99, 235, 0.09);
}

.line-number {
  position: relative;
  flex-shrink: 0;
  padding: 0 10px 0 8px;
  text-align: right;
  user-select: none;
  color: var(--color-text-muted);
  background: var(--color-surface-muted, #f8fafc);
  border-right: 1px solid var(--color-border, #e3e6eb);
  min-width: 44px;
  height: 22px;
  line-height: 22px;
}

.line-number.has-comment-add-btn {
  background: var(--color-surface-selected, #e2e8f0);
}

.line-add-comment-btn {
  position: absolute;
  right: 2px;
  top: 50%;
  transform: translateY(-50%);
  width: 17px;
  height: 17px;
  border-radius: 50%;
  border: none;
  background: var(--color-primary, #2563eb);
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 1px 4px rgba(37, 99, 235, 0.35);
  transition: transform 0.15s cubic-bezier(0.16, 1, 0.3, 1), background 0.15s ease;
  z-index: 4;
  padding: 0;
}

.line-add-comment-btn:hover {
  transform: translateY(-50%) scale(1.15);
  background: var(--color-primary-hover, #1d4ed8);
}

.inline-comment-row {
  display: flex;
  position: sticky;
  left: 0;
  width: fit-content;
  max-width: 100%;
  margin: 4px 0 8px;
  z-index: 5;
  box-sizing: border-box;
}

.inline-comment-gutter {
  width: 44px;
  min-width: 44px;
  flex-shrink: 0;
  background: var(--color-surface-muted, #f8fafc);
  border-right: 1px solid var(--color-border, #e3e6eb);
  box-sizing: border-box;
}

.inline-comment-box {
  width: 300px;
  max-width: 300px;
  margin: 0 8px;
  background: var(--color-surface, #ffffff);
  border: 1px solid var(--color-border, #cbd5e1);
  border-left: 3px solid var(--color-primary, #2563eb);
  border-radius: 6px;
  box-shadow: 0 4px 16px -2px rgba(0, 0, 0, 0.08), 0 2px 6px -1px rgba(0, 0, 0, 0.04);
  overflow: hidden;
  box-sizing: border-box;
  font-family: var(--font-family);
  animation: inline-slide-down 0.15s ease-out;
}

@keyframes inline-slide-down {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.inline-comment-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.3125rem 0.5rem;
  background: var(--color-surface-subtle, #f8fafc);
  border-bottom: 1px solid var(--color-border-subtle, #f1f5f9);
}

.header-left {
  display: inline-flex;
  align-items: center;
  gap: 0.3125rem;
  min-width: 0;
  overflow: hidden;
}

.file-icon {
  flex-shrink: 0;
  color: var(--color-text-muted, #64748b);
}

.file-name {
  font-size: 0.71875rem;
  font-weight: 600;
  color: var(--color-text, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 140px;
}

.line-badge {
  font-family: var(--font-mono);
  font-size: 0.65625rem;
  padding: 0.0625rem 0.25rem;
  border-radius: 3px;
  background: var(--color-surface-selected, #e2e8f0);
  color: var(--color-text, #334155);
  flex-shrink: 0;
}

.inline-close-btn {
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 0 0.25rem;
  font-size: 1.125rem;
  line-height: 1;
  color: var(--color-text-muted, #94a3b8);
  border-radius: 3px;
}

.inline-close-btn:hover {
  color: var(--color-text, #0f172a);
  background: rgba(0, 0, 0, 0.05);
}

.inline-comment-body {
  padding: 0.5rem 0.625rem;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.inline-comment-textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 0.375rem 0.5rem;
  font-size: 0.75rem;
  line-height: 1.45;
  border: 1px solid var(--color-border, #cbd5e1);
  border-radius: 4px;
  resize: vertical;
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #0f172a);
  outline: none;
  font-family: inherit;
}

.inline-comment-textarea:focus {
  border-color: var(--color-primary, #3b82f6);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
}

.inline-comment-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.inline-hint {
  font-size: 0.6875rem;
  color: var(--color-text-muted, #94a3b8);
}

.inline-btn-group {
  display: flex;
  gap: 0.375rem;
}

.inline-btn {
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid transparent;
  transition: all 0.15s ease;
}

.cancel-btn {
  background: transparent;
  color: var(--color-text-muted, #64748b);
  border-color: var(--color-border, #cbd5e1);
}

.cancel-btn:hover {
  background: var(--color-surface-hover, #f1f5f9);
  color: var(--color-text, #0f172a);
}

.submit-btn {
  background: var(--color-primary, #2563eb);
  color: #ffffff;
}

.submit-btn:hover:not(:disabled) {
  background: var(--color-primary-hover, #1d4ed8);
}

.submit-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.code-line {
  flex: 1;
  min-width: 0;
  padding: 0 12px;
  white-space: pre;
  tab-size: 2;
  overflow-x: visible;
}

.code-table.is-wrapped .code-line {
  white-space: pre-wrap;
  word-break: break-all;
}

.code-text {
  font-family: var(--font-mono);
  display: inline-block;
  min-width: 100%;
}

:deep(.search-match) {
  background-color: rgba(234, 179, 8, 0.4);
  color: inherit;
  border-radius: 2px;
}

:deep(.search-match.is-current) {
  background-color: #f59e0b;
  color: #000000;
  box-shadow: 0 0 0 1px #d97706;
  font-weight: 600;
}

.goto-line-bar {
  display: flex;
  align-items: center;
  gap: 6px;
}

.goto-line-label {
  font-size: var(--text-size-12, 12px);
  color: var(--color-text-muted);
  white-space: nowrap;
}

.goto-line-input {
  width: 90px;
}

.code-row.is-focused-line {
  background-color: var(--color-primary-soft, rgba(59, 130, 246, 0.12));
}

.code-row.is-focused-line .line-number {
  background-color: var(--color-primary-soft, rgba(59, 130, 246, 0.2));
  color: var(--color-primary);
  font-weight: 600;
}
</style>
