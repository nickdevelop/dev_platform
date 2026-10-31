<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { checkoutGitBranch, getGitBranches } from '../../api/files'

const props = defineProps<{
  projectId: number
  currentBranch?: string
}>()

const emit = defineEmits<{
  branchChanged: [branch: string]
  error: [msg: string]
}>()

const isOpen = ref(false)
const filterText = ref('')
const branches = ref<string[]>([])
const activeBranch = ref('')
const loading = ref(false)
const switching = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)
const containerRef = ref<HTMLElement | null>(null)

async function fetchBranches() {
  if (!props.projectId) return
  loading.value = true
  try {
    const res = await getGitBranches(props.projectId)
    branches.value = res.branches || []
    activeBranch.value = res.current || ''
  } catch {
    // Best-effort fetch; gracefully degrade in non-git projects or tests
  } finally {
    loading.value = false
  }
}

watch(
  () => props.projectId,
  () => {
    void fetchBranches()
  },
  { immediate: true },
)

const filteredBranches = computed(() => {
  const q = filterText.value.trim().toLowerCase()
  if (!q) return branches.value
  return branches.value.filter((b) => b.toLowerCase().includes(q))
})

const isNewBranch = computed(() => {
  const q = filterText.value.trim()
  if (!q) return false
  return !branches.value.some((b) => b.toLowerCase() === q.toLowerCase())
})

async function openDropdown() {
  isOpen.value = true
  filterText.value = ''
  await fetchBranches()
  await nextTick()
  inputRef.value?.focus()
}

function closeDropdown() {
  isOpen.value = false
}

async function selectBranch(branch: string, isCreate = false) {
  if (switching.value) return
  switching.value = true
  try {
    await checkoutGitBranch(props.projectId, branch, isCreate)
    activeBranch.value = branch
    isOpen.value = false
    emit('branchChanged', branch)
  } catch (err) {
    const msg = err instanceof Error ? err.message : '切换分支失败'
    emit('error', msg)
  } finally {
    switching.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    closeDropdown()
  } else if (e.key === 'Enter') {
    const q = filterText.value.trim()
    if (!q) return
    if (isNewBranch.value) {
      void selectBranch(q, true)
    } else if (filteredBranches.value.length > 0) {
      void selectBranch(filteredBranches.value[0], false)
    }
  }
}

function onDocClick(e: MouseEvent) {
  if (isOpen.value && containerRef.value && !containerRef.value.contains(e.target as Node)) {
    closeDropdown()
  }
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
})
</script>

<template>
  <div ref="containerRef" class="branch-selector relative inline-flex items-center">
    <!-- Trigger Button -->
    <button
      type="button"
      class="inline-flex items-center gap-1 h-[22px] px-1.5 rounded text-xs font-mono font-medium text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors cursor-pointer border border-transparent hover:border-slate-200 dark:hover:border-slate-700 max-w-[140px]"
      :title="`当前分支: ${activeBranch || props.currentBranch || '...'}`"
      @click.stop="openDropdown"
    >
      <svg
        class="shrink-0 text-slate-500"
        viewBox="0 0 16 16"
        width="11"
        height="11"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
      >
        <circle cx="4" cy="4" r="2" />
        <circle cx="4" cy="12" r="2" />
        <circle cx="12" cy="7" r="2" />
        <path d="M4 6v4M4 8c2 0 4-1 4-1s2 0 4 0" />
      </svg>
      <span class="truncate">{{ activeBranch || props.currentBranch || '分支' }}</span>
      <svg class="shrink-0 text-slate-400" viewBox="0 0 16 16" width="9" height="9" fill="currentColor">
        <polygon points="5 6 11 6 8 10" />
      </svg>
    </button>

    <!-- Dropdown Menu -->
    <div
      v-if="isOpen"
      class="absolute left-0 top-full mt-1 w-56 rounded border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 p-1.5 z-50 text-xs flex flex-col gap-1"
      style="box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);"
      @click.stop
    >
      <!-- Filter Input -->
      <div class="relative">
        <input
          ref="inputRef"
          v-model="filterText"
          type="text"
          class="w-full text-xs px-2 py-1 rounded border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900 text-slate-800 dark:text-slate-200 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500 dark:focus:border-blue-400 font-mono"
          placeholder="查找或创建新分支..."
          @keydown="onKeydown"
        />
      </div>

      <!-- New branch action option -->
      <button
        v-if="isNewBranch"
        type="button"
        class="flex items-center gap-1.5 px-2 py-1.5 rounded text-left text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-950/50 cursor-pointer font-mono"
        :disabled="switching"
        @click="selectBranch(filterText.trim(), true)"
      >
        <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.5">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span class="truncate">新建分支 "{{ filterText.trim() }}"</span>
      </button>

      <!-- Branches List -->
      <div class="max-h-48 overflow-y-auto flex flex-col gap-0.5">
        <div v-if="loading" class="text-slate-400 text-center py-2 text-[11px]">
          加载分支...
        </div>
        <template v-else-if="filteredBranches.length > 0">
          <button
            v-for="b in filteredBranches"
            :key="b"
            type="button"
            class="flex items-center justify-between px-2 py-1.5 rounded text-left transition-colors font-mono cursor-pointer"
            :class="[
              b === activeBranch
                ? 'bg-blue-50/70 dark:bg-blue-950/40 text-blue-600 dark:text-blue-400 font-medium'
                : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/60',
            ]"
            :disabled="switching"
            @click="selectBranch(b, false)"
          >
            <span class="truncate">{{ b }}</span>
            <svg
              v-if="b === activeBranch"
              viewBox="0 0 16 16"
              width="11"
              height="11"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M13 4L6 11l-3-3" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </button>
        </template>
        <div v-else-if="!isNewBranch" class="text-slate-400 text-center py-2 text-[11px]">
          无匹配分支
        </div>
      </div>
    </div>
  </div>
</template>
