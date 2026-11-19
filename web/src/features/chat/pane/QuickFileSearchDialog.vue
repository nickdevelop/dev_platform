<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { searchProjectFiles } from '../../../api/projects'
import type { FileItem } from '../../files/types'

const props = defineProps<{
  open: boolean
  projectId: number
  worktreeId?: number
}>()

const emit = defineEmits<{
  close: []
  select: [path: string]
}>()

const query = ref('')
const results = ref<FileItem[]>([])
const loading = ref(false)
const activeIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | undefined

// Filter out folders, or rank files first
const fileList = computed(() => {
  return results.value.filter((item) => !item.isDir)
})

async function doSearch(q: string) {
  if (!props.projectId) return
  loading.value = true
  try {
    const resp = await searchProjectFiles(props.projectId, q, 50, props.worktreeId)
    results.value = resp.files || []
    activeIndex.value = 0
  } catch {
    results.value = []
  } finally {
    loading.value = false
  }
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      query.value = ''
      activeIndex.value = 0
      void doSearch('')
      void nextTick(() => {
        inputRef.value?.focus()
      })
    } else {
      if (debounceTimer) clearTimeout(debounceTimer)
    }
  },
  { immediate: true },
)

watch(query, (newQ) => {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    void doSearch(newQ.trim())
  }, 100)
})

function onKeydown(e: KeyboardEvent) {
  if (!props.open) return

  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (fileList.value.length > 0) {
      activeIndex.value = (activeIndex.value + 1) % fileList.value.length
    }
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (fileList.value.length > 0) {
      activeIndex.value = (activeIndex.value - 1 + fileList.value.length) % fileList.value.length
    }
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const selected = fileList.value[activeIndex.value]
    if (selected) {
      emit('select', selected.path)
    }
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  if (debounceTimer) clearTimeout(debounceTimer)
})
</script>

<template>
  <div
    v-if="props.open"
    class="quick-search-backdrop fixed inset-0 z-50 flex items-start justify-center pt-20 bg-black/40 backdrop-blur-xs"
    data-test="quick-file-search-dialog"
    @click.self="emit('close')"
  >
    <div
      class="quick-search-panel w-full max-w-xl mx-4 rounded-lg shadow-2xl bg-platform-surface border border-platform-border overflow-hidden flex flex-col max-h-[70vh]"
    >
      <!-- Search Input Header -->
      <div class="search-input-wrapper flex items-center gap-2 px-3 py-2.5 border-b border-platform-border bg-platform-surface-muted">
        <svg class="w-4 h-4 text-platform-text-muted shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8">
          <circle cx="7" cy="7" r="4.5" />
          <line x1="10.5" y1="10.5" x2="14" y2="14" />
        </svg>
        <input
          ref="inputRef"
          v-model="query"
          type="text"
          class="flex-1 bg-transparent text-sm text-platform-text placeholder:text-platform-text-muted outline-none font-mono"
          placeholder="输入文件名进行搜索… (↑↓ 导航，Enter 打开，Esc 关闭)"
          data-test="quick-file-search-input"
        />
        <span v-if="loading" class="animate-spin text-xs text-platform-text-muted shrink-0">↻</span>
        <span class="text-3xs font-mono text-platform-text-muted px-1.5 py-0.5 rounded border border-platform-border bg-platform-surface">
          ESC
        </span>
      </div>

      <!-- Search Results List -->
      <div class="search-results-list flex-1 overflow-y-auto p-1 max-h-96">
        <div v-if="loading && fileList.length === 0" class="py-8 text-center text-xs text-platform-text-muted">
          正在搜索文件…
        </div>
        <div v-else-if="fileList.length === 0" class="py-8 text-center text-xs text-platform-text-muted">
          未找到匹配的文件
        </div>
        <ul v-else class="list-none m-0 p-0 flex flex-col gap-0.5" data-test="quick-search-results">
          <li
            v-for="(item, idx) in fileList"
            :key="item.path"
            class="result-item flex items-center justify-between px-2.5 py-1.5 rounded cursor-pointer transition-colors text-xs font-mono"
            :class="[
              idx === activeIndex
                ? 'is-active bg-platform-primary-soft text-platform-primary font-medium'
                : 'text-platform-text hover:bg-platform-surface-muted',
            ]"
            data-test="quick-search-item"
            @mouseenter="activeIndex = idx"
            @click="emit('select', item.path)"
          >
            <div class="flex items-center gap-2 min-w-0 flex-1">
              <span class="text-platform-text-muted shrink-0" aria-hidden="true">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                  <path d="M3 2h7l3 3v9H3V2z" />
                  <polyline points="10 2 10 5 13 5" />
                </svg>
              </span>
              <span class="file-path truncate flex-1" :title="item.path">{{ item.path }}</span>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>
