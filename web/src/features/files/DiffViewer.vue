<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import * as monaco from 'monaco-editor'
import { getGitFileDiff } from '../../api/files'
import { useSettingsStore } from '../../stores/settings'
import type { GitFileDiffResponse } from './types'
import { getChangeStatusLabel } from './changeTreeUtil'
import { ensureMonacoEnvironment, resolveMonacoLanguage } from './monaco/env'
import { applyMonacoTheme, getMonacoFontFamily, getMonacoFontSize } from './monaco/theme'

const props = defineProps<{
  projectId: number
  filePath: string
  commitRef?: string
  commitMessage?: string
  worktreeId?: number
}>()

defineEmits<{
  close?: []
}>()

let settings: ReturnType<typeof useSettingsStore> | null = null
try {
  settings = useSettingsStore()
} catch {
  settings = null
}

const loading = ref(false)
const error = ref('')
const diffData = ref<GitFileDiffResponse | null>(null)
const copied = ref(false)
const sideBySide = ref(false)
const diffContainerRef = ref<HTMLElement | null>(null)

let diffEditor: monaco.editor.IStandaloneDiffEditor | null = null
let originalModel: monaco.editor.ITextModel | null = null
let modifiedModel: monaco.editor.ITextModel | null = null

const hasTextDiff = computed(() => {
  if (!diffData.value || diffData.value.isBinary) return false
  const hasPatch = Boolean(diffData.value.patch && diffData.value.patch.trim().length > 0)
  const hasContents =
    diffData.value.originalContent !== undefined || diffData.value.modifiedContent !== undefined
  return hasPatch || hasContents
})

function disposeDiffEditor() {
  diffEditor?.dispose()
  diffEditor = null
  originalModel?.dispose()
  originalModel = null
  modifiedModel?.dispose()
  modifiedModel = null
}

function renderDiffEditor() {
  if (!diffData.value || diffData.value.isBinary || !hasTextDiff.value) {
    disposeDiffEditor()
    return
  }
  if (!diffContainerRef.value) return

  ensureMonacoEnvironment()
  const themeName = applyMonacoTheme(settings?.appearance.theme)
  const fontSize = getMonacoFontSize(settings?.appearance.fontSize)
  const lang = resolveMonacoLanguage(props.filePath)

  const origText = diffData.value.originalContent ?? ''
  const modText = diffData.value.modifiedContent ?? ''

  if (!diffEditor) {
    diffEditor = monaco.editor.createDiffEditor(diffContainerRef.value, {
      theme: themeName,
      readOnly: true,
      automaticLayout: true,
      renderSideBySide: sideBySide.value,
      scrollBeyondLastLine: false,
      fontFamily: getMonacoFontFamily(),
      fontSize,
      minimap: { enabled: false },
      enableSplitViewResizing: true,
      renderOverviewRuler: false,
    })
  }

  const nextOrig = monaco.editor.createModel(origText, lang)
  const nextMod = monaco.editor.createModel(modText, lang)

  diffEditor.setModel({
    original: nextOrig,
    modified: nextMod,
  })

  originalModel?.dispose()
  modifiedModel?.dispose()
  originalModel = nextOrig
  modifiedModel = nextMod
}

async function fetchDiff() {
  if (!props.projectId || !props.filePath) {
    diffData.value = null
    disposeDiffEditor()
    return
  }

  loading.value = true
  error.value = ''
  try {
    const res = await getGitFileDiff(props.projectId, props.filePath, props.commitRef, props.worktreeId)
    diffData.value = res
    loading.value = false
    await nextTick()
    renderDiffEditor()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '获取文件 Diff 失败'
    diffData.value = null
    disposeDiffEditor()
    loading.value = false
  }
}

function toggleSideBySide() {
  sideBySide.value = !sideBySide.value
  diffEditor?.updateOptions({ renderSideBySide: sideBySide.value })
}

watch(
  [() => props.projectId, () => props.filePath, () => props.commitRef, () => props.worktreeId],
  () => {
    void fetchDiff()
  },
  { immediate: true },
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
    diffEditor?.updateOptions({ fontSize: getMonacoFontSize(size) })
  },
)

onUnmounted(() => {
  disposeDiffEditor()
})

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
  <div class="diff-viewer flex flex-col h-full bg-platform-surface overflow-hidden font-sans" data-test="diff-viewer">
    <!-- Header bar -->
    <div class="diff-header flex items-center justify-between px-4 py-2 border-b border-platform-border bg-platform-surface-muted shrink-0 select-none">
      <div class="flex items-center gap-2.5 min-w-0">
        <!-- Status badge -->
        <span
          v-if="diffData?.status"
          class="status-badge shrink-0 px-1.5 py-0.5 rounded font-mono font-semibold"
          :class="getChangeStatusLabel(diffData.status).class"
        >
          {{ getChangeStatusLabel(diffData.status).label }}
        </span>

        <!-- File path -->
        <span class="font-mono text-xs font-medium text-platform-text truncate" :title="props.filePath">
          {{ props.filePath }}
        </span>

        <!-- Additions / Deletions count -->
        <div v-if="diffData" class="flex items-center gap-1.5 text-xs font-mono shrink-0 ml-1">
          <span v-if="diffData.additions > 0" class="stat-add font-medium">
            +{{ diffData.additions }}
          </span>
          <span v-if="diffData.deletions > 0" class="stat-del font-medium">
            -{{ diffData.deletions }}
          </span>
        </div>

        <!-- Commit or working tree badge -->
        <span
          v-if="props.commitRef"
          class="hidden sm:inline-flex items-center gap-1 commit-badge px-2 py-0.5 rounded bg-platform-surface text-platform-text border border-platform-border font-mono"
          :title="props.commitMessage || props.commitRef"
        >
          <span>commit:</span>
          <span class="font-bold">{{ props.commitRef.slice(0, 7) }}</span>
          <span v-if="props.commitMessage" class="truncate max-w-[140px] opacity-80">{{ props.commitMessage }}</span>
        </span>
        <span
          v-else
          class="hidden sm:inline-flex items-center commit-badge px-2 py-0.5 rounded bg-platform-surface text-platform-text-muted border border-platform-border font-mono"
        >
          工作区未提交变更
        </span>
      </div>

      <!-- Action buttons -->
      <div class="flex items-center gap-1.5 shrink-0 ml-2">
        <button
          v-if="hasTextDiff"
          type="button"
          class="diff-btn px-2 py-1 text-xs rounded transition-colors"
          :class="{ 'is-active': sideBySide }"
          title="切换内联 / 并排对比"
          data-test="diff-toggle-mode-btn"
          @click="toggleSideBySide"
        >
          {{ sideBySide ? '内联' : '并排' }}
        </button>
        <button
          v-if="diffData?.patch"
          type="button"
          class="diff-btn px-2 py-1 text-xs rounded transition-colors"
          :title="copied ? '已复制 Patch' : '复制 Patch'"
          @click="copyPatch"
        >
          {{ copied ? '已复制' : '复制' }}
        </button>
        <button
          type="button"
          class="diff-btn px-2 py-1 text-xs rounded transition-colors"
          title="刷新 Diff"
          :disabled="loading"
          @click="fetchDiff"
        >
          刷新
        </button>
      </div>
    </div>

    <!-- Body content -->
    <div class="diff-content flex-1 min-h-0 overflow-hidden bg-platform-surface font-mono text-xs">
      <!-- Loading state -->
      <div v-if="loading" class="flex items-center justify-center h-48 text-platform-text-muted">
        <span class="animate-spin mr-2">⟳</span> 正在加载文件变更…
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="p-6 text-center text-platform-danger">
        <p>{{ error }}</p>
        <button
          type="button"
          class="mt-3 px-3 py-1 text-xs bg-platform-surface-muted hover:bg-platform-border text-platform-text rounded transition-colors"
          @click="fetchDiff"
        >
          重试
        </button>
      </div>

      <!-- Binary file notice -->
      <div v-else-if="diffData?.isBinary" class="p-8 text-center text-platform-text-muted">
        <div class="inline-flex items-center justify-center w-10 h-10 rounded-md border border-platform-border bg-platform-surface-muted text-platform-text-muted mb-3 mx-auto">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="16" y1="13" x2="8" y2="13" />
            <line x1="16" y1="17" x2="8" y2="17" />
            <polyline points="10 9 9 9 8 9" />
          </svg>
        </div>
        <p class="font-medium text-sm text-platform-text">二进制文件变更</p>
        <p class="text-xs mt-1 text-platform-text-muted">无法在文本编辑器中展示二进制文件的差异</p>
      </div>

      <!-- No change notice -->
      <div v-else-if="!diffData || !hasTextDiff" class="p-8 text-center text-platform-text-muted">
        <p class="text-sm">该文件在此版本中没有文本变更</p>
      </div>

      <!-- Monaco Diff Editor host -->
      <div
        v-else
        ref="diffContainerRef"
        class="monaco-diff-host"
        data-test="monaco-diff-host"
      ></div>
    </div>
  </div>
</template>

<style scoped>
.monaco-diff-host {
  width: 100%;
  height: 100%;
}

.status-badge,
.commit-badge {
  font-size: var(--text-size-11);
}

.stat-add {
  color: var(--color-success);
}

.stat-del {
  color: var(--color-danger);
}

.diff-btn {
  color: var(--color-text-muted);
  border: 1px solid transparent;
}

.diff-btn:hover:not(:disabled) {
  color: var(--color-text);
  background: var(--color-surface);
  border-color: var(--color-border);
}

.diff-btn.is-active {
  color: var(--color-primary);
  background: var(--color-primary-soft);
  border-color: var(--color-primary);
}

.git-modified {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.git-added {
  background: var(--color-success-soft);
  color: var(--color-success);
}

.git-deleted {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.git-untracked {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.git-renamed {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}
</style>
