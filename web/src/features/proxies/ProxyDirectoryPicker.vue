<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { UiDialog, UiButton, UiSpinner, UiAlert } from '../../shared/ui'
import { browseDirectories, type DirectoryEntry } from '../../api/proxies'

const props = defineProps<{
  initialPath?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', path: string): void
}>()

const currentPath = ref('')
const parentPath = ref('')
const entries = ref<DirectoryEntry[]>([])
const loading = ref(false)
const errorMessage = ref('')
const selectedPath = ref('')

async function load(path?: string) {
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await browseDirectories(path)
    currentPath.value = res.current
    parentPath.value = res.parent
    entries.value = res.entries
    selectedPath.value = res.current
  } catch (err: any) {
    errorMessage.value = err?.error || err?.message || '读取目录失败'
  } finally {
    loading.value = false
  }
}

function selectEntry(entry: DirectoryEntry) {
  selectedPath.value = entry.path
}

function openEntry(entry: DirectoryEntry) {
  load(entry.path)
}

function goParent() {
  if (parentPath.value) {
    load(parentPath.value)
  }
}

function confirmSelection() {
  const chosen = selectedPath.value || currentPath.value
  if (chosen) {
    emit('select', chosen)
  }
}

onMounted(() => {
  load(props.initialPath)
})
</script>

<template>
  <UiDialog
    title="选择服务器目录"
    width-class="w-full max-w-xl"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-3 min-h-[360px] max-h-[500px]">
      <!-- Current Path Bar -->
      <div class="flex items-center gap-2 bg-slate-100 dark:bg-slate-800 px-3 py-2 rounded text-xs">
        <button
          type="button"
          class="shrink-0 text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-200 disabled:opacity-40 disabled:cursor-not-allowed font-medium flex items-center gap-1"
          :disabled="!parentPath || loading"
          title="返回上一级目录"
          @click="goParent"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="15 18 9 12 15 6" />
          </svg>
          上一级
        </button>
        <span class="text-slate-300 dark:text-slate-600 select-none">|</span>
        <code class="flex-1 font-mono truncate text-slate-700 dark:text-slate-300 select-all" :title="currentPath">
          {{ currentPath || '正在定位路径…' }}
        </code>
      </div>

      <UiAlert v-if="errorMessage" variant="danger" dismissible @dismiss="errorMessage = ''">
        {{ errorMessage }}
      </UiAlert>

      <!-- Content Area -->
      <div class="flex-1 border border-slate-200 dark:border-slate-700 rounded-lg overflow-y-auto min-h-[220px] max-h-[300px] relative">
        <div v-if="loading" class="absolute inset-0 bg-white/70 dark:bg-slate-900/70 flex items-center justify-center gap-2 text-xs text-slate-500">
          <UiSpinner size="md" />
          <span>读取中…</span>
        </div>

        <div v-else-if="entries.length === 0" class="py-12 text-center text-xs text-slate-400">
          当前目录下无其他子目录
        </div>

        <ul v-else class="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
          <li
            v-for="entry in entries"
            :key="entry.path"
            class="px-3 py-2 flex items-center justify-between cursor-pointer select-none transition-colors"
            :class="selectedPath === entry.path ? 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300' : 'hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300'"
            @click="selectEntry(entry)"
            @dblclick="openEntry(entry)"
          >
            <div class="flex items-center gap-2 truncate">
              <!-- Folder icon -->
              <svg class="w-4 h-4 text-amber-500 shrink-0" viewBox="0 0 24 24" fill="currentColor">
                <path d="M10 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-8l-2-2z" />
              </svg>
              <span class="truncate font-medium">{{ entry.name }}</span>
            </div>
            <button
              type="button"
              class="text-[11px] text-slate-400 hover:text-blue-600 dark:hover:text-blue-400 px-1.5 py-0.5 rounded hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors shrink-0"
              title="进入此目录"
              @click.stop="openEntry(entry)"
            >
              打开 →
            </button>
          </li>
        </ul>
      </div>

      <!-- Selection preview -->
      <div class="text-[11px] text-slate-500 flex items-center gap-1.5 truncate">
        <span class="shrink-0 text-slate-400">选中路径：</span>
        <code class="font-mono text-slate-700 dark:text-slate-300 truncate">{{ selectedPath || currentPath }}</code>
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-2 pt-2 border-t border-slate-200 dark:border-slate-700">
        <UiButton variant="secondary" size="sm" @click="emit('close')">
          取消
        </UiButton>
        <UiButton variant="primary" size="sm" :disabled="loading || (!selectedPath && !currentPath)" @click="confirmSelection">
          选择此目录
        </UiButton>
      </div>
    </div>
  </UiDialog>
</template>
