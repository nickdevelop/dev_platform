<script setup lang="ts">
import { computed, ref } from 'vue'
import { commitGitChanges } from '../../api/files'
import type { GitCommitItem } from './types'

const props = defineProps<{
  projectId: number
  stagedCount: number
  unstagedCount: number
}>()

const emit = defineEmits<{
  committed: [commit: GitCommitItem]
  stageAllAndCommit: [message: string]
  error: [msg: string]
}>()

const commitMessage = ref('')
const committing = ref(false)
const localError = ref('')

const canCommit = computed(() => {
  return commitMessage.value.trim().length > 0 && (props.stagedCount > 0 || props.unstagedCount > 0)
})

async function submitCommit() {
  const msg = commitMessage.value.trim()
  if (!msg || committing.value) return

  localError.value = ''

  // If no staged changes but there are unstaged changes, stage all and commit
  if (props.stagedCount === 0 && props.unstagedCount > 0) {
    emit('stageAllAndCommit', msg)
    commitMessage.value = ''
    return
  }

  committing.value = true
  try {
    const commit = await commitGitChanges(props.projectId, msg)
    commitMessage.value = ''
    emit('committed', commit)
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : '提交失败'
    localError.value = errorMsg
    emit('error', errorMsg)
  } finally {
    committing.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault()
    void submitCommit()
  }
}
</script>

<template>
  <div class="git-commit-bar border-t border-slate-200 dark:border-slate-800 p-2 bg-slate-50/70 dark:bg-slate-900/60 shrink-0 select-none">
    <div class="relative flex flex-col gap-1.5">
      <div class="relative">
        <textarea
          v-model="commitMessage"
          rows="2"
          class="w-full text-xs px-2 py-1.5 rounded border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 text-slate-800 dark:text-slate-200 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500 dark:focus:border-blue-400 resize-none font-mono"
          placeholder="提交说明 (Ctrl+Enter 提交)"
          :disabled="committing"
          @keydown="onKeydown"
        ></textarea>
      </div>

      <div v-if="localError" class="text-[11px] text-red-600 dark:text-red-400 truncate" :title="localError">
        {{ localError }}
      </div>

      <div class="flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-slate-400 tabular-nums">
          <template v-if="props.stagedCount > 0">
            已暂存 {{ props.stagedCount }} 个
          </template>
          <template v-else-if="props.unstagedCount > 0">
            将全部暂存并提交
          </template>
          <template v-else>
            工作区干净
          </template>
        </span>

        <button
          type="button"
          class="inline-flex items-center justify-center gap-1 h-6 px-2.5 rounded text-xs font-medium transition-colors"
          :class="[
            canCommit
              ? 'bg-blue-600 hover:bg-blue-700 text-white dark:bg-blue-500 dark:hover:bg-blue-600 cursor-pointer'
              : 'bg-slate-200/80 dark:bg-slate-800 text-slate-400 dark:text-slate-600 cursor-not-allowed',
          ]"
          :disabled="!canCommit || committing"
          @click="submitCommit"
        >
          <svg
            v-if="committing"
            class="animate-spin -ml-0.5 h-3 w-3 text-white"
            viewBox="0 0 24 24"
            fill="none"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <svg
            v-else
            viewBox="0 0 16 16"
            width="12"
            height="12"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path d="M13 4L6 11l-3-3" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span>提交</span>
        </button>
      </div>
    </div>
  </div>
</template>
