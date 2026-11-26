<script setup lang="ts">
// Quick Open 搜索弹层（Ctrl+P / Cmd+P 呼出）：单行搜索条 + 悬浮结果卡，
// 形态参照 .tmp/file_search.html 原型（Element Plus 复合搜索框）。
// 模式切换（文件名 / 内容）与后缀过滤都是自绘下拉；后缀过滤仅内容模式提供，
// 选中组或自定义后缀都经 exts 参数在后端生效（内容搜索有截断上限）。
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { searchProjectFiles, searchProjectContent, type FileContentMatch, type FileItem } from '../../../api/projects'
import {
  EXT_FILTER_ALL,
  EXT_FILTER_OPTIONS,
  extSelectionLabel,
  normalizeCustomExt,
  resolveExtFilter,
  type ExtFilterOption,
} from './quickSearchExt'
import { highlightMatch, clipAroundQuery } from './quickSearchHighlight'

const props = defineProps<{
  open: boolean
  projectId: number
  worktreeId?: number
}>()

const emit = defineEmits<{
  close: []
  select: [path: string, line?: number]
}>()

const MODE_OPTIONS = [
  { value: 'file' as const, label: '文件名' },
  { value: 'content' as const, label: '内容' },
]

const searchType = ref<'file' | 'content'>('file')
const query = ref('')
const fileResults = ref<FileItem[]>([])
const contentResults = ref<FileContentMatch[]>([])
const loading = ref(false)
const activeIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
const resultsRef = ref<HTMLElement | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | undefined

// 下拉与聚焦状态
const modeMenuOpen = ref(false)
const extMenuOpen = ref(false)
const inputFocused = ref(false)
const barFocused = computed(() => inputFocused.value || modeMenuOpen.value || extMenuOpen.value)

// 后缀过滤：'' = 全部后缀；预设组名；或自定义记号（如 ".kt,.json"）
const extFilter = ref('')
const extDraft = ref('')
const customExts = ref<string[]>([])
const customExtInputRef = ref<HTMLInputElement | null>(null)

// 文件模式下过滤掉目录，只列文件
const fileList = computed(() => fileResults.value.filter((item) => !item.isDir))

// 两种模式共用同一套结果行：后缀 chip + 名称行（内容模式带 :行号）+ 次行
//（文件模式是路径，内容模式是命中摘要）。
interface SearchRow {
  key: string
  path: string
  line?: number
  name: string
  ext: string
  nameHtml: string
  subHtml: string
}

const modeLabel = computed(() => (searchType.value === 'file' ? '文件名' : '内容'))

const inputPlaceholder = computed(() =>
  searchType.value === 'file' ? '输入文件名搜索…' : '输入文本搜索文件内容…',
)

const extChipLabel = computed(() => extSelectionLabel(extFilter.value))

const extScopeText = computed(() =>
  extFilter.value === EXT_FILTER_ALL ? '全部文件' : extSelectionLabel(extFilter.value),
)

// 下拉选项 = 全部后缀 + 预设组 + 本次打开期间自定义追加的后缀
const extOptions = computed<ExtFilterOption[]>(() => {
  const customs = customExts.value
    .filter((v) => !EXT_FILTER_OPTIONS.some((o) => o.value === v))
    .map((v) => ({ value: v, label: `自定义 (${extSelectionLabel(v)})` }))
  return [...EXT_FILTER_OPTIONS, ...customs]
})

const filteredExtOptions = computed(() => {
  const kw = extDraft.value.trim().toLowerCase()
  if (!kw) return extOptions.value
  return extOptions.value.filter(
    (o) => o.label.toLowerCase().includes(kw) || o.value.toLowerCase().includes(kw),
  )
})

// 草稿过滤不到任何预设时，提供「使用 *.xxx」直接应用自定义（原型行为）
const showUseCustomOption = computed(() => extDraft.value.trim() !== '' && filteredExtOptions.value.length === 0)

const useCustomLabel = computed(() => {
  const norm = normalizeCustomExt(extDraft.value)
  return norm === undefined ? `*.${extDraft.value.trim()}` : extSelectionLabel(norm)
})

function baseName(path: string): string {
  const parts = path.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || path
}

function fileExt(path: string): string {
  const name = baseName(path)
  const idx = name.lastIndexOf('.')
  return idx > 0 ? name.slice(idx).toLowerCase() : ''
}

function highlight(text: string): string {
  return highlightMatch(text, query.value)
}

const searchRows = computed<SearchRow[]>(() => {
  if (searchType.value === 'file') {
    return fileList.value.map((item) => ({
      key: item.path,
      path: item.path,
      name: baseName(item.path),
      ext: fileExt(item.path),
      nameHtml: highlight(baseName(item.path)),
      subHtml: highlight(item.path),
    }))
  }
  return contentResults.value.map((item) => ({
    key: `${item.path}:${item.line}`,
    path: item.path,
    line: item.line,
    name: baseName(item.path),
    ext: fileExt(item.path),
    nameHtml: highlight(baseName(item.path)),
    // 内容摘要：以命中位置为中心截取，长行不整段展示
    subHtml: highlight(clipAroundQuery(item.content, query.value)),
  }))
})

const activeListLength = computed(() => searchRows.value.length)

function pickRow(row: SearchRow) {
  if (row.line === undefined) {
    emit('select', row.path)
  } else {
    emit('select', row.path, row.line)
  }
}

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

const statsText = computed(() => {
  if (loading.value && activeListLength.value === 0) {
    return searchType.value === 'file' ? '正在搜索文件…' : '正在搜索文件内容…'
  }
  if (!query.value.trim()) {
    return searchType.value === 'file' ? '文件名搜索' : `内容搜索 · 范围 ${extScopeText.value}`
  }
  if (activeListLength.value === 0) return '暂无数据'
  return `共 ${activeListLength.value} 条匹配结果`
})

const showRetryAll = computed(
  () =>
    searchType.value === 'content' &&
    query.value.trim() !== '' &&
    !loading.value &&
    contentResults.value.length === 0 &&
    extFilter.value !== EXT_FILTER_ALL,
)

const noResultLabel = computed(() => (searchType.value === 'file' ? '文件' : '内容'))

function focusInput() {
  void nextTick(() => inputRef.value?.focus())
}

function closeMenus() {
  modeMenuOpen.value = false
  extMenuOpen.value = false
}

function toggleModeMenu() {
  const next = !modeMenuOpen.value
  closeMenus()
  modeMenuOpen.value = next
}

function toggleExtMenu() {
  const next = !extMenuOpen.value
  closeMenus()
  extMenuOpen.value = next
  if (next) {
    extDraft.value = ''
    void nextTick(() => customExtInputRef.value?.focus())
  }
}

function setSearchMode(mode: 'file' | 'content') {
  closeMenus()
  if (searchType.value === mode) {
    focusInput()
    return
  }
  searchType.value = mode
}

// 选中一个后缀：'' = 全部；预设组之外的值视为自定义，追加进下拉列表
function selectExt(value: string) {
  extFilter.value = value
  if (
    value !== EXT_FILTER_ALL &&
    !EXT_FILTER_OPTIONS.some((o) => o.value === value) &&
    !customExts.value.includes(value)
  ) {
    customExts.value.push(value)
  }
  extMenuOpen.value = false
  extDraft.value = ''
  focusInput()
}

function applyCustomExt() {
  const norm = normalizeCustomExt(extDraft.value)
  if (norm === undefined) {
    void nextTick(() => customExtInputRef.value?.focus())
    return
  }
  selectExt(norm)
}

function clearSearch() {
  query.value = ''
  activeIndex.value = 0
  void doSearch('')
  focusInput()
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      searchType.value = 'file'
      query.value = ''
      extFilter.value = ''
      extDraft.value = ''
      customExts.value = []
      activeIndex.value = 0
      closeMenus()
      void doSearch('')
      focusInput()
    } else {
      closeMenus()
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
  closeMenus()
  activeIndex.value = 0
  void doSearch(query.value.trim())
  focusInput()
})

watch(extFilter, () => {
  if (searchType.value !== 'content') return
  activeIndex.value = 0
  void doSearch(query.value.trim())
})

// 键盘导航时让活动项保持在可视区内（jsdom 等环境没有 scrollIntoView，静默跳过）
watch(activeIndex, (idx) => {
  void nextTick(() => {
    const row = resultsRef.value?.querySelector<HTMLElement>(`[data-index="${idx}"]`)
    row?.scrollIntoView?.({ block: 'nearest' })
  })
})

function onKeydown(e: KeyboardEvent) {
  if (!props.open) return

  // 自定义后缀输入框自己消化 Enter/Esc/Tab，不参与结果列表导航
  const t = e.target as HTMLElement | null
  if (t && typeof t.closest === 'function' && t.closest('[data-test="quick-search-ext-input"]')) return

  if (e.key === 'Escape') {
    e.preventDefault()
    if (modeMenuOpen.value || extMenuOpen.value) {
      closeMenus()
      focusInput()
    } else {
      emit('close')
    }
  } else if (e.key === 'Tab') {
    e.preventDefault()
    closeMenus()
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
    // 有下拉打开时 Enter 先收起下拉，不误开文件
    if (modeMenuOpen.value || extMenuOpen.value) {
      closeMenus()
      focusInput()
      return
    }
    const selected = searchRows.value[activeIndex.value]
    if (selected) pickRow(selected)
  }
}

// 点击组件内部、两个下拉之外时收起下拉
function onDocClick(e: MouseEvent) {
  if (!props.open) return
  const t = e.target as HTMLElement | null
  if (t && typeof t.closest === 'function' && t.closest('[data-test="quick-search-mode-select"],[data-test="quick-search-ext-select"]')) {
    return
  }
  modeMenuOpen.value = false
  extMenuOpen.value = false
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  document.addEventListener('click', onDocClick)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  document.removeEventListener('click', onDocClick)
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
    <div class="quick-search-root w-full max-w-[640px] mx-4">
      <!-- 单行搜索条：模式前缀下拉 + 输入 + 清空/后缀过滤/搜索图标 -->
      <div
        class="qs-search-bar flex items-center h-10 bg-platform-surface rounded-sm"
        :class="{ 'is-focus': barFocused }"
        data-test="quick-search-bar"
      >
        <div class="relative h-full shrink-0" data-test="quick-search-mode-select">
          <button
            type="button"
            class="mode-trigger h-full flex items-center justify-between gap-2 px-3.5 bg-platform-surface-muted/60 hover:bg-platform-surface-muted text-sm text-platform-text-muted hover:text-platform-text rounded-l-sm cursor-pointer select-none min-w-[88px]"
            title="切换搜索范围 (Tab)"
            data-test="quick-search-type-select"
            @click.stop="toggleModeMenu"
          >
            <span>{{ modeLabel }}</span>
            <svg
              class="qs-arrow w-3.5 h-3.5 shrink-0"
              :class="{ 'is-open': modeMenuOpen }"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              aria-hidden="true"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          <div
            v-if="modeMenuOpen"
            class="qs-popper absolute left-0 top-[calc(100%+6px)] w-32 py-1.5 text-xs bg-platform-surface rounded-sm border border-platform-border shadow-xl z-50"
            data-test="quick-search-mode-menu"
            @click.stop
          >
            <button
              v-for="m in MODE_OPTIONS"
              :key="m.value"
              type="button"
              class="w-full flex items-center justify-between px-3 py-1.5 text-left cursor-pointer"
              :class="
                searchType === m.value
                  ? 'text-platform-primary font-medium bg-platform-primary-soft'
                  : 'text-platform-text-muted hover:bg-platform-surface-muted hover:text-platform-text'
              "
              :data-test="`quick-search-mode-${m.value}`"
              @click="setSearchMode(m.value)"
            >
              <span>{{ m.label }}</span>
              <svg
                v-if="searchType === m.value"
                class="w-3.5 h-3.5 shrink-0 text-platform-primary"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                aria-hidden="true"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            </button>
          </div>
        </div>

        <input
          ref="inputRef"
          v-model="query"
          type="text"
          autocomplete="off"
          spellcheck="false"
          class="min-w-0 flex-1 h-full px-3 bg-transparent text-sm text-platform-text placeholder:text-platform-text-muted outline-none font-mono"
          :placeholder="inputPlaceholder"
          data-test="quick-file-search-input"
          @focus="inputFocused = true"
          @blur="inputFocused = false"
        />

        <div class="flex items-center h-full pr-2.5 gap-1.5 shrink-0">
          <!-- 清空 -->
          <button
            v-if="query"
            type="button"
            class="p-1 text-platform-text-muted hover:text-platform-text cursor-pointer"
            title="清空"
            data-test="quick-search-clear-btn"
            @click="clearSearch"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path
                fill-rule="evenodd"
                d="M12 2.25c-5.385 0-9.75 4.365-9.75 9.75s4.365 9.75 9.75 9.75 9.75-4.365 9.75-9.75S17.385 2.25 12 2.25zm-1.72 6.97a.75.75 0 10-1.06 1.06L10.94 12l-1.72 1.72a.75.75 0 101.06 1.06L12 13.06l1.72 1.72a.75.75 0 101.06-1.06L13.06 12l1.72-1.72a.75.75 0 10-1.06-1.06L12 10.94l-1.72-1.72z"
                clip-rule="evenodd"
              />
            </svg>
          </button>

          <!-- 后缀过滤（仅内容模式）：chip 触发器 + 自定义输入面板 -->
          <div v-if="searchType === 'content'" class="relative" data-test="quick-search-ext-select">
            <button
              type="button"
              class="ext-chip flex items-center gap-1 h-6 px-2 rounded-sm font-mono cursor-pointer select-none"
              :title="`过滤文件后缀（支持手动输入）：${extScopeText}`"
              data-test="quick-search-ext-btn"
              @click.stop="toggleExtMenu"
            >
              <span class="text-2xs">{{ extChipLabel }}</span>
              <svg
                class="qs-arrow w-3 h-3 shrink-0"
                :class="{ 'is-open': extMenuOpen }"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.2"
                aria-hidden="true"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
              </svg>
            </button>

            <div
              v-if="extMenuOpen"
              class="qs-popper absolute right-0 top-[calc(100%+12px)] w-64 py-1.5 text-xs bg-platform-surface rounded-sm border border-platform-border shadow-xl z-50"
              data-test="quick-search-ext-menu"
              @click.stop
            >
              <div class="px-2.5 pt-1 pb-2 border-b border-platform-border mb-1">
                <div class="flex items-center gap-1">
                  <input
                    ref="customExtInputRef"
                    v-model="extDraft"
                    type="text"
                    autocomplete="off"
                    spellcheck="false"
                    placeholder="自定义后缀，如 .vue"
                    class="min-w-0 flex-1 h-[26px] px-2 font-mono text-platform-text bg-platform-surface-muted border border-platform-border rounded-sm outline-none"
                    data-test="quick-search-ext-input"
                    @keydown.enter.stop.prevent="applyCustomExt"
                    @keydown.esc.stop.prevent="closeMenus(); focusInput()"
                    @keydown.tab.stop.prevent="closeMenus(); focusInput()"
                  />
                  <button
                    type="button"
                    class="h-[26px] px-2 rounded-sm bg-platform-primary text-platform-surface hover:bg-platform-primary-hover shrink-0 cursor-pointer"
                    data-test="quick-search-ext-apply"
                    @click="applyCustomExt"
                  >
                    确定
                  </button>
                </div>
              </div>
              <div class="max-h-44 overflow-y-auto">
                <template v-if="filteredExtOptions.length > 0">
                  <button
                    v-for="opt in filteredExtOptions"
                    :key="opt.value"
                    type="button"
                    class="w-full flex items-center justify-between gap-2 px-3 py-1.5 text-left cursor-pointer"
                    :class="
                      extFilter === opt.value
                        ? 'text-platform-primary font-medium bg-platform-primary-soft'
                        : 'text-platform-text-muted hover:bg-platform-surface-muted hover:text-platform-text'
                    "
                    :data-test="`quick-search-ext-option-${opt.value || 'all'}`"
                    @click="selectExt(opt.value)"
                  >
                    <span class="truncate">{{ opt.label }}</span>
                    <svg
                      v-if="extFilter === opt.value"
                      class="w-3.5 h-3.5 shrink-0 text-platform-primary"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.5"
                      aria-hidden="true"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </button>
                </template>
                <button
                  v-else
                  type="button"
                  class="w-full flex items-center justify-between px-3 py-1.5 text-left text-platform-primary bg-platform-primary-soft hover:bg-platform-primary-soft/70 cursor-pointer"
                  data-test="quick-search-ext-use-custom"
                  @click="applyCustomExt"
                >
                  <span class="truncate">使用 "{{ useCustomLabel }}"</span>
                  <span class="text-2xs font-mono text-platform-text-muted shrink-0">Enter</span>
                </button>
              </div>
            </div>
          </div>

          <div class="h-4 w-px bg-platform-border mx-0.5" aria-hidden="true"></div>

          <button
            type="button"
            class="p-1 text-platform-text-muted hover:text-platform-primary cursor-pointer"
            title="搜索"
            @click="focusInput"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
          </button>
        </div>
      </div>

      <!-- 悬浮结果卡 -->
      <div class="qs-popper mt-1.5 w-full bg-platform-surface rounded-sm shadow-xl overflow-hidden">
        <div ref="resultsRef" class="h-[320px] overflow-y-auto">
          <!-- 搜索中 -->
          <div v-if="loading && activeListLength === 0" class="py-8 text-center text-xs text-platform-text-muted">
            {{ searchType === 'file' ? '正在搜索文件…' : '正在搜索文件内容…' }}
          </div>

          <!-- 内容模式空输入：引导 -->
          <div v-else-if="searchType === 'content' && !query.trim()" class="py-7 px-4 text-center">
            <p class="text-xs text-platform-text-muted">请输入搜索关键字以检索文件内容</p>
          </div>

          <!-- 文件模式无文件（空项目/空目录） -->
          <div
            v-else-if="searchType === 'file' && !query.trim() && fileList.length === 0"
            class="py-8 text-center text-xs text-platform-text-muted"
          >
            暂无文件
          </div>

          <!-- 无结果 -->
          <div v-else-if="activeListLength === 0" class="py-8 px-4 text-center">
            <p class="text-xs text-platform-text-muted">
              未找到匹配 <span class="font-mono text-platform-text font-medium">"{{ query.trim() }}"</span>
              的{{ noResultLabel }}
            </p>
            <button
              v-if="showRetryAll"
              type="button"
              class="mt-2.5 px-2.5 py-1 rounded-sm text-2xs text-platform-primary bg-platform-primary-soft border border-platform-border cursor-pointer"
              data-test="quick-search-ext-retry-all"
              @click="selectExt(EXT_FILTER_ALL)"
            >
              切换到全部后缀 (*.*) 重试
            </button>
          </div>

          <!-- 统一结果行：文件模式次行为路径，内容模式次行为命中摘要 -->
          <ul
            v-else
            class="list-none m-0 p-0"
            data-test="quick-search-results"
          >
            <li
              v-for="(row, idx) in searchRows"
              :key="row.key"
              class="result-item flex items-center justify-between px-3.5 py-2.5 border-b border-platform-border/60 last:border-b-0 cursor-pointer"
              :class="idx === activeIndex ? 'is-active bg-platform-primary-soft' : 'hover:bg-platform-surface-muted/80'"
              :data-index="idx"
              :data-test="row.line === undefined ? 'quick-search-item' : 'quick-search-content-item'"
              :title="row.path"
              @mouseenter="activeIndex = idx"
              @click="pickRow(row)"
            >
              <div class="flex items-center gap-2.5 min-w-0">
                <span
                  v-if="row.ext"
                  class="px-1.5 py-0.5 rounded-sm text-2xs font-mono bg-platform-surface-muted text-platform-text-muted border border-platform-border shrink-0"
                >{{ row.ext }}</span>
                <div class="min-w-0">
                  <div class="text-xs font-medium text-platform-text truncate">
                    <span v-html="row.nameHtml"></span><span
                      v-if="row.line !== undefined"
                      class="font-mono text-platform-text-muted"
                    >:{{ row.line }}</span>
                  </div>
                  <div class="text-2xs font-mono text-platform-text-muted truncate" :title="row.path" v-html="row.subHtml"></div>
                </div>
              </div>
              <span
                class="text-xs text-platform-primary shrink-0 ml-2"
                :class="idx === activeIndex ? 'opacity-100' : 'opacity-0'"
              >打开</span>
            </li>
          </ul>
        </div>

        <!-- 底部状态条 -->
        <div class="flex items-center justify-between px-3.5 py-2 bg-platform-surface-muted/70 border-t border-platform-border text-xs text-platform-text-muted">
          <span data-test="quick-search-stats">{{ statsText }}</span>
          <div class="flex items-center gap-3">
            <span><kbd class="qs-kbd font-mono text-2xs text-platform-text">↑↓</kbd> 选择</span>
            <span><kbd class="qs-kbd font-mono text-2xs text-platform-text">Enter</kbd> 打开</span>
            <span><kbd class="qs-kbd font-mono text-2xs text-platform-text">Tab</kbd> 切换</span>
            <span><kbd class="qs-kbd font-mono text-2xs text-platform-text">Esc</kbd> 关闭</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 单行搜索条：Element 输入框式的聚焦外轮廓 */
.qs-search-bar {
  border: 1px solid var(--color-border);
  transition:
    border-color 0.2s cubic-bezier(0.645, 0.045, 0.355, 1),
    box-shadow 0.2s cubic-bezier(0.645, 0.045, 0.355, 1);
}

.qs-search-bar:hover {
  border-color: var(--color-border-strong);
}

.qs-search-bar.is-focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 12%, transparent);
}

.mode-trigger {
  transition: background-color 0.15s ease, color 0.15s ease;
}

.ext-chip {
  background: var(--color-primary-soft);
  color: var(--color-primary);
  border: 1px solid color-mix(in srgb, var(--color-primary) 25%, transparent);
  transition: background-color 0.15s ease;
}

.ext-chip:hover {
  background: color-mix(in srgb, var(--color-primary) 18%, transparent);
}

/* 下拉箭头平滑旋转 */
.qs-arrow {
  transition: transform 0.25s ease;
}

.qs-arrow.is-open {
  transform: rotate(180deg);
}

/* popper 阴影由模板里的 shadow-xl 承担，这里只留布局钩子 */

.qs-kbd {
  padding: 1px 5px;
  border: 1px solid var(--color-border);
  border-radius: 3px;
  background: var(--color-surface);
}

/* 命中高亮：highlightMatch 先转义再插 span，内容可信 */
.result-item :deep(.qs-hit) {
  color: var(--color-primary);
  background: var(--color-primary-soft);
  font-weight: 600;
  border-radius: 2px;
}
</style>
