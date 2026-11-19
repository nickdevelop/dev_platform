<script setup lang="ts">
// A user turn's text shows its first FOLD_LINES visual lines and folds the rest
// behind a toggle. If the message contains <CodeComment> XML blocks, they are
// parsed and rendered as structured cards aligned with ComposerCommentCards.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  parseCodeCommentsMessage,
} from '../../../shared/utils/codeCommentBlock'

const props = defineProps<{ text: string }>()

const emit = defineEmits<{
  'open-file': [filePath: string, line?: number]
}>()

const parsedData = computed(() => parseCodeCommentsMessage(props.text))
const hasCodeComments = computed(() => parsedData.value.comments.length > 0)
const displayRestText = computed(() => parsedData.value.restText)

function formatLine(comment: { line?: string; startLine?: number; endLine?: number }): string {
  if (comment.line) {
    return comment.line.startsWith('L') ? comment.line : `L${comment.line}`
  }
  if (comment.startLine !== undefined) {
    if (comment.startLine === comment.endLine || comment.endLine === undefined) {
      return `L${comment.startLine}`
    }
    return `L${comment.startLine}-${comment.endLine}`
  }
  return ''
}

function snippetPreview(snippet: string): string {
  if (!snippet) return ''
  const lines = snippet.trim().split('\n')
  const twoLines = lines.slice(0, 2)
  return twoLines.map((line) => (line.length > 22 ? `${line.slice(0, 22)}…` : line)).join('\n')
}

function getFileName(filePath: string): string {
  if (!filePath) return ''
  const parts = filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || filePath
}

const FOLD_LINES = 10
const textEl = ref<HTMLElement | null>(null)
const expanded = ref(false)
const overflowing = ref(false)
const foldHeight = ref(0)
let observer: ResizeObserver | null = null

function measure() {
  const el = textEl.value
  if (!el) return
  const style = getComputedStyle(el)
  const lineHeight = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.5
  foldHeight.value = lineHeight * FOLD_LINES
  overflowing.value = el.scrollHeight > foldHeight.value + 1
}

watch(
  () => displayRestText.value,
  () => void nextTick(measure),
)

onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && textEl.value) {
    observer = new ResizeObserver(measure)
    observer.observe(textEl.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
})
</script>

<template>
  <div class="user-text">
    <!-- Render CodeComment cards if present -->
    <div v-if="hasCodeComments" class="user-code-comments-tray" data-test="user-code-comments-tray">
      <div
        v-for="(comment, idx) in parsedData.comments"
        :key="idx"
        class="user-code-comment-card comment-card"
        data-test="user-code-comment-card"
      >
        <!-- Line 1: File, Line button -->
        <div class="card-line card-line-file">
          <button
            type="button"
            class="card-anchor"
            :title="`在工作区定位 ${comment.path}:${formatLine(comment)}`"
            data-test="comment-card-header"
            @click="emit('open-file', comment.path, comment.startLine)"
          >
            <svg class="file-icon" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M3 2h7l4 4v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z" />
              <polyline points="10 2 10 6 14 6" />
            </svg>
            <span class="file-name comment-card-path" :title="comment.path">{{ getFileName(comment.path) }}</span>
            <span class="line-badge comment-card-line">{{ formatLine(comment) }}</span>
          </button>
        </div>

        <!-- Line 2: Code Snippet Summary (Hover for full snippet) -->
        <div class="card-line card-line-snippet" :title="comment.snippet" data-test="comment-card-snippet">
          <code class="code-snippet-summary">{{ snippetPreview(comment.snippet) || '—' }}</code>
        </div>

        <!-- Line 3: Description (Hover for full description) -->
        <div class="card-line card-line-comment" :title="comment.description" data-test="comment-card-desc">
          <span class="comment-text">{{ comment.description || '无备注' }}</span>
        </div>
      </div>
    </div>

    <!-- Main user text body (remaining text or whole text if no comments) -->
    <template v-if="displayRestText">
      <div
        ref="textEl"
        class="message-text user-text-body"
        :class="{ 'is-folded': overflowing && !expanded }"
        :style="overflowing && !expanded ? { maxHeight: `${foldHeight}px` } : undefined"
        data-test="user-text-body"
      >{{ displayRestText }}</div>
      <button
        v-if="overflowing"
        type="button"
        class="user-text-toggle"
        data-test="user-text-toggle"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >{{ expanded ? '收起' : '展开' }}</button>
    </template>
  </div>
</template>

<style scoped>
.user-code-comments-tray {
  display: flex;
  flex-direction: row;
  flex-wrap: nowrap;
  align-items: stretch;
  gap: 0.5rem;
  padding: 0.25rem 0;
  margin-bottom: 0.5rem;
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}

.user-code-comments-tray::-webkit-scrollbar {
  height: 4px;
}

.user-code-comments-tray::-webkit-scrollbar-thumb {
  background: var(--color-border-strong, #cbd5e1);
  border-radius: 2px;
}

.user-code-comments-tray::-webkit-scrollbar-track {
  background: transparent;
}

.user-code-comment-card {
  flex: 0 0 150px;
  width: 150px;
  min-width: 150px;
  max-width: 150px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 0.25rem;
  padding: 0.3125rem 0.375rem;
  background: var(--color-surface, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  font-size: 0.75rem;
  line-height: 1.25;
  color: var(--color-text, #1e293b);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.user-code-comment-card:hover {
  border-color: var(--color-border-strong, #cbd5e1);
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
}

.card-line {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}

.card-line-file {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.25rem;
}

.card-anchor {
  display: inline-flex;
  align-items: center;
  gap: 0.1875rem;
  cursor: pointer;
  padding: 0.0625rem 0.25rem;
  border-radius: 3px;
  border: none;
  background: var(--color-surface-muted);
  color: inherit;
  min-width: 0;
  width: 100%;
  overflow: hidden;
  transition: background 0.15s ease, color 0.15s ease;
  text-align: left;
}

.card-anchor:hover {
  background: var(--color-primary-soft);
  color: var(--color-primary, #2563eb);
}

.file-icon {
  flex-shrink: 0;
  opacity: 0.75;
}

.file-name {
  font-weight: 600;
  font-size: 0.6875rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  flex: 1;
}

.line-badge {
  font-family: var(--font-mono, monospace);
  font-size: 0.625rem;
  color: var(--color-text-muted, #64748b);
  flex-shrink: 0;
}

.card-line-snippet {
  font-family: var(--font-mono, monospace);
  font-size: 0.625rem;
  color: var(--color-text-muted, #64748b);
  background: var(--color-surface-muted);
  padding: 0.125rem 0.25rem;
  border-radius: 3px;
  overflow: hidden;
  line-height: 1.35;
  box-sizing: border-box;
  cursor: help;
}

.code-snippet-summary {
  font-family: inherit;
  font-size: inherit;
  color: inherit;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: pre-wrap;
  word-break: break-all;
}

.card-line-comment {
  font-size: 0.6875rem;
  font-weight: 500;
  color: var(--color-text, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.3;
  cursor: help;
}

.comment-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}

.user-text-body {
  white-space: pre-wrap;
  word-break: break-word;
}

.user-text-body.is-folded {
  overflow: hidden;
}

.user-text-toggle {
  display: block;
  margin-top: 0.25rem;
  padding: 0;
  border: none;
  background: none;
  color: inherit;
  font-size: 0.75rem;
  opacity: 0.75;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}

.user-text-toggle:hover {
  opacity: 1;
}
</style>
