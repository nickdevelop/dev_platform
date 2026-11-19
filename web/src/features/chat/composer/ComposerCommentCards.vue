<script setup lang="ts">
import type { CodeCommentCard } from '../../files/types'

const props = defineProps<{
  cards: CodeCommentCard[]
}>()

const emit = defineEmits<{
  remove: [id: string]
  locate: [card: CodeCommentCard]
}>()

function formatLine(card: CodeCommentCard): string {
  if (card.startLine === card.endLine) {
    return `L${card.startLine}`
  }
  return `L${card.startLine}-${card.endLine}`
}

function snippetPreview(snippet: string): string {
  if (!snippet) return ''
  const lines = snippet.trim().split('\n')
  const twoLines = lines.slice(0, 2)
  return twoLines.map((line) => (line.length > 22 ? `${line.slice(0, 22)}…` : line)).join('\n')
}
</script>

<template>
  <div v-if="props.cards && props.cards.length > 0" class="composer-comment-cards" data-test="composer-comment-cards">
    <div
      v-for="card in props.cards"
      :key="card.id"
      class="comment-card"
      data-test="comment-card"
    >
      <!-- Line 1: File, Line, and Remove Button -->
      <div class="card-line card-line-file">
        <button
          type="button"
          class="card-anchor"
          :title="`在工作区定位 ${card.filePath}:${formatLine(card)}`"
          data-test="card-locate-btn"
          @click="emit('locate', card)"
        >
          <svg class="file-icon" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M3 2h7l4 4v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z" />
            <polyline points="10 2 10 6 14 6" />
          </svg>
          <span class="file-name" :title="card.fileName">{{ card.fileName }}</span>
          <span class="line-badge">{{ formatLine(card) }}</span>
        </button>
        <button
          type="button"
          class="btn-remove"
          title="移除此卡片"
          aria-label="移除此卡片"
          data-test="btn-remove-card"
          @click.stop="emit('remove', card.id)"
        >
          ×
        </button>
      </div>

      <!-- Line 2: Code Snippet Summary -->
      <div class="card-line card-line-snippet" :title="card.codeSnippet">
        <code class="code-snippet-summary">{{ snippetPreview(card.codeSnippet) || '—' }}</code>
      </div>

      <!-- Line 3: Description -->
      <div class="card-line card-line-comment" :title="card.comment">
        <span class="comment-text">{{ card.comment || '无备注' }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.composer-comment-cards {
  display: flex;
  flex-direction: row;
  flex-wrap: nowrap;
  align-items: stretch;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  background: var(--color-surface-muted);
  border-bottom: 1px solid var(--color-border);
  border-top-left-radius: inherit;
  border-top-right-radius: inherit;
  box-sizing: border-box;
  width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}

.composer-comment-cards::-webkit-scrollbar {
  height: 4px;
}

.composer-comment-cards::-webkit-scrollbar-thumb {
  background: var(--color-border-strong, #cbd5e1);
  border-radius: 2px;
}

.composer-comment-cards::-webkit-scrollbar-track {
  background: transparent;
}

.comment-card {
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

.comment-card:hover {
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
  flex: 1;
  overflow: hidden;
  transition: background 0.15s ease, color 0.15s ease;
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

.btn-remove {
  border: none;
  background: transparent;
  color: var(--color-text-muted, #94a3b8);
  font-size: 0.875rem;
  line-height: 1;
  cursor: pointer;
  padding: 0 0.125rem;
  border-radius: 3px;
  flex-shrink: 0;
  transition: color 0.15s ease, background 0.15s ease;
}

.btn-remove:hover {
  color: var(--color-text, #0f172a);
  background: rgba(0, 0, 0, 0.06);
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
}

.comment-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}
</style>
