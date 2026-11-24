<script setup lang="ts">
import { ref } from 'vue'
import type { GitCommitItem } from './types'

const props = defineProps<{
  commit?: GitCommitItem
}>()

const emit = defineEmits<{
  reset: []
}>()

const copied = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | undefined

async function copyHash() {
  if (!props.commit?.hash) return
  try {
    await navigator.clipboard.writeText(props.commit.hash)
    copied.value = true
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copied.value = false
    }, 1500)
  } catch {
    // Clipboard permission or fallback
  }
}
</script>

<template>
  <div
    class="git-commit-detail-bar border-t border-slate-200 dark:border-slate-800 p-2 bg-slate-50/70 dark:bg-slate-900/60 shrink-0"
    data-test="git-commit-detail-bar"
  >
    <div v-if="props.commit" class="relative flex flex-col gap-1.5">
      <!-- Top header bar: commit badge, hash copy, and reset button -->
      <div class="flex items-center justify-between gap-1.5">
        <div class="flex items-center gap-1.5 min-w-0">
          <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-2xs font-medium bg-surface-muted text-foreground border border-border shrink-0">
            <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
              <circle cx="8" cy="8" r="3" />
              <line x1="8" y1="1" x2="8" y2="5" />
              <line x1="8" y1="11" x2="8" y2="15" />
            </svg>
            <span>提交</span>
          </span>
          <button
            type="button"
            class="commit-hash-btn inline-flex items-center gap-1 text-2xs font-mono text-slate-600 dark:text-slate-300 hover:text-accent cursor-pointer truncate transition-colors"
            :title="props.commit.hash"
            data-test="copy-commit-hash-btn"
            @click="copyHash"
          >
            <span>{{ props.commit.shortHash || props.commit.hash.slice(0, 7) }}</span>
            <span class="text-2xs text-slate-400">
              {{ copied ? '已复制' : '复制' }}
            </span>
          </button>
        </div>

        <button
          type="button"
          class="reset-btn inline-flex items-center justify-center h-5 px-1.5 rounded text-2xs text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200 hover:bg-slate-200/60 dark:hover:bg-slate-800 transition-colors shrink-0 cursor-pointer"
          data-test="btn-reset-to-working"
          title="返回当前未提交工作区变更"
          @click="emit('reset')"
        >
          返回未提交
        </button>
      </div>

      <!-- Commit Message body -->
      <div
        class="commit-message-box w-full text-xs px-2 py-1.5 rounded border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 text-slate-800 dark:text-slate-200 font-mono whitespace-pre-wrap break-words max-h-24 overflow-y-auto select-text"
        data-test="commit-message-box"
      >
        {{ props.commit.message || '(无提交说明)' }}
      </div>

      <!-- Footer meta info: author and date -->
      <div class="flex items-center justify-between gap-1 text-2xs text-slate-400 tabular-nums">
        <span class="author-name truncate" :title="props.commit.author">
          {{ props.commit.author }}
        </span>
        <span class="commit-date shrink-0">
          {{ props.commit.date }}
        </span>
      </div>
    </div>

    <div v-else class="text-xs text-slate-400 py-2 text-center" data-test="commit-detail-empty">
      未选择或未找到提交信息
    </div>
  </div>
</template>
