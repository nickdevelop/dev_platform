<script setup lang="ts">
// A user turn's text shows its first FOLD_LINES visual lines and folds the rest
// behind a toggle. If the message contains <CodeComment> XML blocks, they are
// parsed and rendered as structured cards with snippet summaries.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  parseCodeCommentsMessage,
  getCodeSnippetSummary,
} from '../../../shared/utils/codeCommentBlock'

const props = defineProps<{ text: string }>()

const emit = defineEmits<{
  'open-file': [filePath: string, line?: number]
}>()

const parsedData = computed(() => parseCodeCommentsMessage(props.text))
const hasCodeComments = computed(() => parsedData.value.comments.length > 0)
const displayRestText = computed(() => parsedData.value.restText)

const expandedSnippets = ref<Record<number, boolean>>({})

function toggleSnippet(index: number) {
  expandedSnippets.value[index] = !expandedSnippets.value[index]
}

function isSnippetExpanded(index: number): boolean {
  return !!expandedSnippets.value[index]
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
        class="user-code-comment-card"
        data-test="user-code-comment-card"
      >
        <div
          class="comment-card-header"
          :title="`在工作区定位 ${comment.path}:${comment.line}`"
          data-test="comment-card-header"
          @click="emit('open-file', comment.path, comment.startLine)"
        >
          <svg class="file-icon" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M3 2h7l4 4v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z" />
            <polyline points="10 2 10 6 14 6" />
          </svg>
          <span class="comment-card-path">{{ comment.path }}</span>
          <span class="comment-card-line">L{{ comment.line }}</span>
        </div>

        <div v-if="comment.description" class="comment-card-desc" data-test="comment-card-desc">
          {{ comment.description }}
        </div>

        <div v-if="comment.snippet" class="comment-card-snippet">
          <pre><code>{{ isSnippetExpanded(idx) ? comment.snippet : getCodeSnippetSummary(comment.snippet, 2).summary }}</code></pre>
          <button
            v-if="getCodeSnippetSummary(comment.snippet, 2).hasMore"
            type="button"
            class="snippet-toggle-btn"
            data-test="snippet-toggle-btn"
            @click="toggleSnippet(idx)"
          >
            {{ isSnippetExpanded(idx) ? '收起代码' : `展开完整代码 (共 ${getCodeSnippetSummary(comment.snippet, 2).totalLines} 行)` }}
          </button>
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
  flex-direction: column;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.user-code-comment-card {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  padding: 0.5rem 0.625rem;
  background: var(--color-surface, rgba(255, 255, 255, 0.9));
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  color: var(--color-text, #1e293b);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}

.comment-card-header {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  cursor: pointer;
  width: fit-content;
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
  background: var(--color-surface-muted);
  transition: background 0.15s ease;
}

.comment-card-header:hover {
  background: var(--color-primary-soft);
  color: var(--color-primary, #3b82f6);
}

.file-icon {
  flex-shrink: 0;
  opacity: 0.8;
}

.comment-card-path {
  font-weight: 600;
  font-size: 0.75rem;
}

.comment-card-line {
  font-family: ui-monospace, monospace;
  font-size: 0.6875rem;
  color: var(--color-text-muted, #64748b);
}

.comment-card-desc {
  font-size: 0.8125rem;
  font-weight: 500;
  line-height: 1.4;
  white-space: pre-wrap;
  word-break: break-word;
}

.comment-card-snippet {
  background: var(--color-surface-muted);
  color: var(--color-text);
  padding: 0.375rem 0.5rem;
  border-radius: 4px;
  font-family: ui-monospace, monospace;
  font-size: 0.6875rem;
  line-height: 1.4;
  overflow-x: auto;
}

.comment-card-snippet pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}

.snippet-toggle-btn {
  margin-top: 0.25rem;
  padding: 0;
  border: none;
  background: none;
  color: var(--color-primary, #60a5fa);
  font-size: 0.6875rem;
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.snippet-toggle-btn:hover {
  color: var(--color-primary-hover, #93c5fd);
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
