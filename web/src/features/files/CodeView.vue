<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { highlightCode, splitHighlightedHtmlIntoLines } from '../../shared/highlight'

const props = withDefaults(
  defineProps<{
    content: string
    filePath?: string
    showLineNumbers?: boolean
  }>(),
  {
    showLineNumbers: true,
  },
)

const searchVisible = ref(false)
const searchQuery = ref('')
const searchIndex = ref(0)
const searchInputRef = ref<HTMLInputElement | null>(null)
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
    void nextTick(() => {
      searchInputRef.value?.focus()
      searchInputRef.value?.select()
    })
  } else {
    searchQuery.value = ''
    searchIndex.value = 0
  }
}

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
  } else if (e.key === 'Escape' && searchVisible.value) {
    e.preventDefault()
    toggleSearch()
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
    <div ref="codeContainerRef" class="code-scroller">
      <div class="code-table" :class="{ 'is-wrapped': wrapLines }">
        <div
          v-for="(lineHtml, idx) in renderedLines"
          :key="idx"
          class="code-row"
          :data-line="idx + 1"
        >
          <div
            v-if="props.showLineNumbers"
            class="line-number"
            :data-line-number="idx + 1"
            aria-hidden="true"
          >
            {{ idx + 1 }}
          </div>
          <div class="code-line">
            <span class="code-text" v-html="lineHtml || ' '"></span>
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

.line-number {
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
</style>
