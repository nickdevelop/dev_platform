<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { searchProjectFiles, searchProjectContent, type FileContentMatch, type FileItem } from '../../../api/projects'
import { EXT_FILTER_OPTIONS, resolveExtFilter } from './quickSearchExt'

const props = defineProps<{
  open: boolean
  projectId: number
  worktreeId?: number
}>()

const emit = defineEmits<{
  close: []
  select: [path: string, line?: number]
}>()

const searchType = ref<'file' | 'content'>('file')
const query = ref('')
const fileResults = ref<FileItem[]>([])
const contentResults = ref<FileContentMatch[]>([])
const loading = ref(false)
const activeIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | undefined

// Content-search extension filter: a single choice mapping to a group of
// extensions, or a manually typed raw value (see ./quickSearchExt.ts).
const extFilter = ref('')

// Filter out folders in file mode
const fileList = computed(() => {
  return fileResults.value.filter((item) => !item.isDir)
})

const activeListLength = computed(() => {
  return searchType.value === 'file' ? fileList.value.length : contentResults.value.length
})

async function doSearch(q: string) {
  if (!props.projectId) return
  loading.value = true
  try {
    if (searchType.value === 'file') {
      const resp = await searchProjectFiles(props.projectId, q, 50, props.worktreeId)
      fileResults.value = resp.files || []
    } else {
      if (!q) {
        contentResults.value = []
      } else {
        const resp = await searchProjectContent(props.projectId, q, 50, props.worktreeId, resolveExtFilter(extFilter.value))
        contentResults.value = resp.matches || []
      }
    }
    activeIndex.value = 0
  } catch {
    if (searchType.value === 'file') {
      fileResults.value = []
    } else {
      contentResults.value = []
    }
  } finally {
    loading.value = false
  }
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      searchType.value = 'file'
      query.value = ''
      extFilter.value = ''
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

watch(searchType, () => {
  activeIndex.value = 0
  void doSearch(query.value.trim())
  void nextTick(() => {
    inputRef.value?.focus()
  })
})

watch(extFilter, () => {
  if (searchType.value !== 'content') return
  activeIndex.value = 0
  void doSearch(query.value.trim())
})

// While the extension el-select owns the focus, arrows/Tab/Enter belong to its
// dropdown (navigating options / confirming a created one), not to the result
// list. Escape still closes the whole dialog.
function isInsideExtSelect(e: KeyboardEvent): boolean {
  const t = e.target as HTMLElement | null
  return !!t && typeof t.closest === 'function' && !!t.closest('[data-test="quick-search-ext-select"]')
}

function onKeydown(e: KeyboardEvent) {
  if (!props.open) return

  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
  } else if (isInsideExtSelect(e) && (e.key === 'Tab' || e.key === 'Enter' || e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
    // Leave the keystroke to the select's own dropdown handling.
  } else if (e.key === 'Tab') {
    e.preventDefault()
    searchType.value = searchType.value === 'file' ? 'content' : 'file'
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (activeListLength.value > 0) {
      activeIndex.value = (activeIndex.value + 1) % activeListLength.value
    }
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (activeListLength.value > 0) {
      activeIndex.value = (activeIndex.value - 1 + activeListLength.value) % activeListLength.value
    }
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (searchType.value === 'file') {
      const selected = fileList.value[activeIndex.value]
      if (selected) {
        emit('select', selected.path)
      }
    } else {
      const selected = contentResults.value[activeIndex.value]
      if (selected) {
        emit('select', selected.path, selected.line)
      }
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
        <!-- Search Type Selector -->
        <select
          v-model="searchType"
          class="search-type-select text-xs font-medium py-1 px-1.5 rounded border border-platform-border bg-platform-surface text-platform-text cursor-pointer outline-none focus:border-platform-primary"
          data-test="quick-search-type-select"
          title="按 Tab 键在文件与内容之间快速切换"
        >
          <option value="file">文件</option>
          <option value="content">内容</option>
        </select>

        <!-- 后缀过滤只在内容模式下提供：内容搜索有截断上限，过滤必须在后端生效 -->
        <el-select
          v-if="searchType === 'content'"
          v-model="extFilter"
          class="ext-filter-select"
          size="small"
          filterable
          allow-create
          default-first-option
          clearable
          placeholder="后缀：不限"
          title="按文件后缀过滤内容搜索；可直接输入，如 *.kt 或 kt,json"
          data-test="quick-search-ext-select"
        >
          <el-option
            v-for="opt in EXT_FILTER_OPTIONS"
            :key="opt.value"
            :label="opt.label"
            :value="opt.value"
          />
        </el-select>

        <svg class="w-4 h-4 text-platform-text-muted shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8">
          <circle cx="7" cy="7" r="4.5" />
          <line x1="10.5" y1="10.5" x2="14" y2="14" />
        </svg>

        <input
          ref="inputRef"
          v-model="query"
          type="text"
          class="flex-1 bg-transparent text-sm text-platform-text placeholder:text-platform-text-muted outline-none font-mono"
          :placeholder="
            searchType === 'file'
              ? '输入文件名搜索… (Tab 切换类型，↑↓ 导航，Enter 打开)'
              : '输入文本搜索内容… (Tab 切换类型，↑↓ 导航，Enter 打开)'
          "
          data-test="quick-file-search-input"
        />

        <span v-if="loading" class="animate-spin text-xs text-platform-text-muted shrink-0">↻</span>
        <span class="text-2xs font-mono text-platform-text-muted px-1.5 py-0.5 rounded border border-platform-border bg-platform-surface">
          ESC
        </span>
      </div>

      <!-- Search Results List -->
      <div class="search-results-list flex-1 overflow-y-auto p-1 max-h-96">
        <!-- File Mode Loading / Empty -->
        <template v-if="searchType === 'file'">
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
        </template>

        <!-- Content Mode Loading / Empty -->
        <template v-else>
          <div v-if="loading && contentResults.length === 0" class="py-8 text-center text-xs text-platform-text-muted">
            正在搜索文件内容…
          </div>
          <div v-else-if="!query.trim()" class="py-8 text-center text-xs text-platform-text-muted">
            请输入搜索关键字以检索文件内容
          </div>
          <div v-else-if="contentResults.length === 0" class="py-8 text-center text-xs text-platform-text-muted">
            未找到匹配的内容
          </div>
          <ul v-else class="list-none m-0 p-0 flex flex-col gap-0.5" data-test="quick-search-content-results">
            <li
              v-for="(item, idx) in contentResults"
              :key="`${item.path}:${item.line}`"
              class="result-item content-result-item flex flex-col gap-0.5 px-2.5 py-1.5 rounded cursor-pointer transition-colors text-xs font-mono"
              :class="[
                idx === activeIndex
                  ? 'is-active bg-platform-primary-soft text-platform-primary font-medium'
                  : 'text-platform-text hover:bg-platform-surface-muted',
              ]"
              data-test="quick-search-content-item"
              @mouseenter="activeIndex = idx"
              @click="emit('select', item.path, item.line)"
            >
              <div class="flex items-center gap-1.5 min-w-0">
                <span class="text-platform-text-muted shrink-0" aria-hidden="true">
                  <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M3 2h7l3 3v9H3V2z" />
                    <polyline points="10 2 10 5 13 5" />
                  </svg>
                </span>
                <span class="file-path truncate font-semibold" :title="item.path">{{ item.path }}</span>
                <span class="line-badge text-2xs px-1.5 py-0.5 rounded bg-platform-surface-muted text-platform-text-muted border border-platform-border">
                  :{{ item.line }}
                </span>
              </div>
              <div class="content-snippet text-2xs text-platform-text-muted truncate pl-4">
                {{ item.content }}
              </div>
            </li>
          </ul>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-type-select {
  appearance: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='currentColor' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 4px center;
  padding-right: 18px;
}

/* Width of the extension filter; only a plain layout width, no .el-* overrides. */
.ext-filter-select {
  width: 9.5rem;
  flex-shrink: 0;
}
</style>
