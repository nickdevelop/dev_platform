<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'

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

const lines = computed(() => {
  if (!props.content) return []
  return props.content.split('\n')
})

const totalLines = computed(() => lines.value.length)

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
  lineEl?.scrollIntoView({ block: 'center', behavior: 'smooth' })
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
        :title="copied ? '已复制' : '复制代码'"
        @click="copyCode"
      >
        {{ copied ? '✓ 已复制' : '复制' }}
      </button>
      <button
        type="button"
        class="action-btn"
        title="搜索 (Ctrl+F)"
        @click="toggleSearch"
      >
        🔍 查找
      </button>
    </div>

    <!-- Code rows with line numbers -->
    <div ref="codeContainerRef" class="code-scroller">
      <div class="code-table">
        <div v-if="props.showLineNumbers" class="line-numbers" aria-hidden="true">
          <div
            v-for="num in totalLines"
            :key="num"
            class="line-number"
            :data-line-number="num"
          >
            {{ num }}
          </div>
        </div>
        <div class="code-body">
          <div
            v-for="(line, idx) in lines"
            :key="idx"
            class="code-line"
            :data-line="idx + 1"
          >
            <span class="code-text">{{ line || ' ' }}</span>
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
  font-size: 0.72rem;
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
  border: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface, #ffffff);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.72rem;
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

.code-scroller {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding-bottom: 24px;
}

.code-table {
  display: flex;
  min-width: 100%;
  font-size: 0.82rem;
  line-height: 1.55;
}

.line-numbers {
  flex-shrink: 0;
  padding: 10px 10px 10px 8px;
  text-align: right;
  user-select: none;
  color: var(--color-text-muted);
  background: var(--color-surface-muted, #f8fafc);
  border-right: 1px solid var(--color-border, #e3e6eb);
  min-width: 38px;
}

.line-number {
  height: 22px;
  line-height: 22px;
}

.code-body {
  flex: 1;
  padding: 10px 12px;
  white-space: pre;
  tab-size: 2;
  overflow-x: visible;
}

.code-line {
  height: 22px;
  line-height: 22px;
}

.code-text {
  font-family: var(--font-mono);
}
</style>
