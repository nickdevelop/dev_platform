<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getGitFileDiff } from '../../api/files'
import type { GitFileDiffResponse } from './types'
import { getChangeStatusLabel } from './changeTreeUtil'

const props = defineProps<{
  projectId: number
  filePath: string
  commitRef?: string
  commitMessage?: string
}>()

const emit = defineEmits<{
  close?: []
}>()

const loading = ref(false)
const error = ref('')
const diffData = ref<GitFileDiffResponse | null>(null)
const copied = ref(false)

interface DiffLine {
  kind: 'hunk' | 'addition' | 'deletion' | 'context'
  oldNum?: number
  newNum?: number
  text: string
}

const parsedLines = computed<DiffLine[]>(() => {
  if (!diffData.value?.patch) return []
  const rawLines = diffData.value.patch.split('\n')
  const result: DiffLine[] = []

  let oldCounter = 0
  let newCounter = 0

  for (const raw of rawLines) {
    if (raw.startsWith('diff --git') || raw.startsWith('index ') || raw.startsWith('--- ') || raw.startsWith('+++ ') || raw.startsWith('new file mode') || raw.startsWith('deleted file mode') || raw.startsWith('\\ No newline')) {
      continue
    }

    if (raw.startsWith('@@')) {
      // Parse hunk header: @@ -oldStart,oldCount +newStart,newCount @@
      const match = raw.match(/@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@/)
      if (match) {
        oldCounter = parseInt(match[1], 10)
        newCounter = parseInt(match[2], 10)
      }
      result.push({
        kind: 'hunk',
        text: raw,
      })
      continue
    }

    if (raw.startsWith('+')) {
      result.push({
        kind: 'addition',
        newNum: newCounter++,
        text: raw.slice(1),
      })
    } else if (raw.startsWith('-')) {
      result.push({
        kind: 'deletion',
        oldNum: oldCounter++,
        text: raw.slice(1),
      })
    } else {
      const text = raw.startsWith(' ') ? raw.slice(1) : raw
      result.push({
        kind: 'context',
        oldNum: oldCounter++,
        newNum: newCounter++,
        text,
      })
    }
  }

  return result
})

async function fetchDiff() {
  if (!props.projectId || !props.filePath) {
    diffData.value = null
    return
  }

  loading.value = true
  error.value = ''
  try {
    const res = await getGitFileDiff(props.projectId, props.filePath, props.commitRef)
    diffData.value = res
  } catch (err) {
    error.value = err instanceof Error ? err.message : '获取文件 Diff 失败'
    diffData.value = null
  } finally {
    loading.value = false
  }
}

watch(
  [() => props.projectId, () => props.filePath, () => props.commitRef],
  () => {
    void fetchDiff()
  },
  { immediate: true },
)

async function copyPatch() {
  if (!diffData.value?.patch) return
  try {
    await navigator.clipboard.writeText(diffData.value.patch)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    // fallback
  }
}
</script>

<template>
  <div class="diff-viewer flex flex-col h-full bg-white dark:bg-slate-900 overflow-hidden font-sans">
    <!-- Header bar -->
    <div class="diff-header flex items-center justify-between px-4 py-2.5 border-b border-slate-200 dark:border-slate-800 bg-slate-50/80 dark:bg-slate-900/80 shrink-0 select-none">
      <div class="flex items-center gap-2.5 min-w-0">
        <!-- Status badge -->
        <span
          v-if="diffData?.status"
          class="shrink-0 px-1.5 py-0.5 rounded text-[11px] font-mono font-semibold"
          :class="getChangeStatusLabel(diffData.status).class"
        >
          {{ getChangeStatusLabel(diffData.status).label }}
        </span>

        <!-- File path -->
        <span class="font-mono text-xs font-medium text-slate-800 dark:text-slate-200 truncate" :title="props.filePath">
          {{ props.filePath }}
        </span>

        <!-- Additions / Deletions count -->
        <div v-if="diffData" class="flex items-center gap-1.5 text-xs font-mono shrink-0 ml-1">
          <span v-if="diffData.additions > 0" class="text-emerald-600 dark:text-emerald-400 font-medium">
            +{{ diffData.additions }}
          </span>
          <span v-if="diffData.deletions > 0" class="text-red-600 dark:text-red-400 font-medium">
            -{{ diffData.deletions }}
          </span>
        </div>

        <!-- Commit or working tree badge -->
        <span
          v-if="props.commitRef"
          class="hidden sm:inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 font-mono"
          :title="props.commitMessage || props.commitRef"
        >
          <span>commit:</span>
          <span class="font-bold">{{ props.commitRef.slice(0, 7) }}</span>
          <span v-if="props.commitMessage" class="truncate max-w-[140px] opacity-80">{{ props.commitMessage }}</span>
        </span>
        <span
          v-else
          class="hidden sm:inline-flex items-center text-[11px] px-2 py-0.5 rounded bg-slate-200/60 dark:bg-slate-800 text-slate-600 dark:text-slate-400 font-mono"
        >
          工作区未提交变更
        </span>
      </div>

      <!-- Action buttons -->
      <div class="flex items-center gap-1.5 shrink-0 ml-2">
        <button
          v-if="diffData?.patch"
          type="button"
          class="diff-btn px-2 py-1 text-xs text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-200/60 dark:hover:bg-slate-800 rounded transition-colors"
          :title="copied ? '已复制 Patch' : '复制 Patch'"
          @click="copyPatch"
        >
          {{ copied ? '已复制' : '复制' }}
        </button>
        <button
          type="button"
          class="diff-btn px-2 py-1 text-xs text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-200/60 dark:hover:bg-slate-800 rounded transition-colors"
          title="刷新 Diff"
          :disabled="loading"
          @click="fetchDiff"
        >
          刷新
        </button>
      </div>
    </div>

    <!-- Body content -->
    <div class="diff-content flex-1 overflow-auto bg-white dark:bg-slate-950 font-mono text-xs leading-relaxed">
      <!-- Loading state -->
      <div v-if="loading" class="flex items-center justify-center h-48 text-slate-500 dark:text-slate-400">
        <span class="animate-spin mr-2">⟳</span> 正在加载文件变更…
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="p-6 text-center text-red-600 dark:text-red-400">
        <p>{{ error }}</p>
        <button
          type="button"
          class="mt-3 px-3 py-1 text-xs bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 rounded transition-colors"
          @click="fetchDiff"
        >
          重试
        </button>
      </div>

      <!-- Binary file notice -->
      <div v-else-if="diffData?.isBinary" class="p-8 text-center text-slate-500 dark:text-slate-400">
        <div class="text-2xl mb-2">📄</div>
        <p class="font-medium text-sm text-slate-700 dark:text-slate-300">二进制文件变更</p>
        <p class="text-xs mt-1 text-slate-500">无法在文本编辑器中展示二进制文件的差异。</p>
      </div>

      <!-- No change notice -->
      <div v-else-if="!diffData || parsedLines.length === 0" class="p-8 text-center text-slate-500 dark:text-slate-400">
        <p class="text-sm">该文件在此版本中没有文本变更。</p>
      </div>

      <!-- Diff lines table -->
      <div v-else class="diff-table w-full select-text">
        <div
          v-for="(line, idx) in parsedLines"
          :key="idx"
          class="diff-row flex items-baseline hover:bg-black/5 dark:hover:bg-white/5"
          :class="`diff-line-${line.kind}`"
        >
          <!-- Old line number -->
          <span
            class="line-num shrink-0 w-10 text-right pr-2 text-slate-400 dark:text-slate-600 select-none text-[11px]"
            aria-hidden="true"
          >
            {{ line.oldNum ?? '' }}
          </span>

          <!-- New line number -->
          <span
            class="line-num shrink-0 w-10 text-right pr-2 text-slate-400 dark:text-slate-600 select-none text-[11px] border-r border-slate-200 dark:border-slate-800/80"
            aria-hidden="true"
          >
            {{ line.newNum ?? '' }}
          </span>

          <!-- Diff marker symbol (+, -, @@, space) -->
          <span class="diff-sign shrink-0 w-5 text-center select-none font-bold" aria-hidden="true">
            {{ line.kind === 'addition' ? '+' : line.kind === 'deletion' ? '-' : line.kind === 'hunk' ? '@@' : ' ' }}
          </span>

          <!-- Code line text -->
          <span class="line-code flex-1 whitespace-pre-wrap break-all pr-4">
            {{ line.text }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.git-modified {
  background: color-mix(in srgb, #f59e0b 16%, transparent);
  color: #d97706;
}

.git-added {
  background: color-mix(in srgb, #10b981 16%, transparent);
  color: #059669;
}

.git-deleted {
  background: color-mix(in srgb, #ef4444 16%, transparent);
  color: #dc2626;
}

.git-untracked {
  background: color-mix(in srgb, #06b6d4 16%, transparent);
  color: #0891b2;
}

.git-renamed {
  background: color-mix(in srgb, #8b5cf6 16%, transparent);
  color: #7c3aed;
}

:root.dark .git-modified {
  color: #fbbf24;
}

:root.dark .git-added {
  color: #34d399;
}

:root.dark .git-deleted {
  color: #f87171;
}

:root.dark .git-untracked {
  color: #22d3ee;
}

:root.dark .git-renamed {
  color: #a78bfa;
}

.diff-line-hunk {
  background: color-mix(in srgb, #3b82f6 12%, transparent);
  color: #2563eb;
  font-weight: 500;
}

:root.dark .diff-line-hunk {
  background: color-mix(in srgb, #3b82f6 18%, transparent);
  color: #60a5fa;
}

.diff-line-addition {
  background: color-mix(in srgb, #10b981 15%, transparent);
  color: #047857;
}

:root.dark .diff-line-addition {
  background: color-mix(in srgb, #10b981 18%, transparent);
  color: #34d399;
}

.diff-line-deletion {
  background: color-mix(in srgb, #ef4444 14%, transparent);
  color: #b91c1c;
}

:root.dark .diff-line-deletion {
  background: color-mix(in srgb, #ef4444 18%, transparent);
  color: #f87171;
}

.diff-line-context {
  color: inherit;
}
</style>
