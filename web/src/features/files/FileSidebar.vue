<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  commitGitChanges,
  discardGitFile,
  getFileTree,
  getGitCommits,
  getGitStatus,
  stageGitFiles,
  unstageGitFiles,
} from '../../api/files'
import { searchProjectFiles } from '../../api/projects'
import { UiButton, UiDialog } from '../../shared/ui'
import BranchSelector from './BranchSelector.vue'
import ChangeTree from './ChangeTree.vue'
import { buildChangeTree } from './changeTreeUtil'
import FileTree from './FileTree.vue'
import GitCommitBar from './GitCommitBar.vue'
import type { FileTreeNode, GitCommitItem, GitFileChangeStatus, GitStatusItem } from './types'

const props = defineProps<{
  projectId: number
  activePath?: string
}>()

const emit = defineEmits<{
  select: [path: string, options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string }]
  dragstart: [event: DragEvent, path: string]
}>()

const activeTab = ref<'changes' | 'all'>('all')
const loading = ref(false)
const error = ref('')
const treeNodes = ref<FileTreeNode[]>([])
const gitChanges = ref<GitStatusItem[]>([])
const commits = ref<GitCommitItem[]>([])
const selectedCommit = ref<string>('') // empty means working tree
const searchQuery = ref('')
const searchResults = ref<Array<{ path: string; isDir: boolean }>>([])
const searchLoading = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | undefined

const isStagedGroupExpanded = ref(true)
const isUnstagedGroupExpanded = ref(true)
const expandedChangeDirs = ref<Set<string>>(new Set())

// Discard confirmation dialog state
const discardModalOpen = ref(false)
const discardConfirmPath = ref('')
const discarding = ref(false)

const isHistoryCommit = computed(() => Boolean(selectedCommit.value))

const gitChangesMap = computed(() => {
  const map = new Map<string, GitFileChangeStatus>()
  for (const item of gitChanges.value) {
    map.set(item.path, item.status)
  }
  return map
})

const currentCommitMessage = computed(() => {
  if (!selectedCommit.value) return ''
  const c = commits.value.find((item) => item.hash === selectedCommit.value)
  return c?.message || ''
})

const changesCount = computed(() => gitChanges.value.length)

const displayChanges = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return gitChanges.value
  return gitChanges.value.filter((item) => item.path.toLowerCase().includes(q))
})

// Staged and unstaged partitioning for working directory
const stagedItems = computed(() => {
  if (isHistoryCommit.value) return []
  return displayChanges.value.filter((item) => Boolean(item.staged))
})

const unstagedItems = computed(() => {
  if (isHistoryCommit.value) return displayChanges.value
  return displayChanges.value.filter((item) => !item.staged)
})

const stagedTreeResult = computed(() => {
  return buildChangeTree(stagedItems.value)
})

const unstagedTreeResult = computed(() => {
  return buildChangeTree(unstagedItems.value)
})

const historyTreeResult = computed(() => {
  return buildChangeTree(displayChanges.value)
})

async function refresh() {
  if (!props.projectId) return
  loading.value = true
  error.value = ''
  try {
    const [tree, status, recentCommits] = await Promise.all([
      getFileTree(props.projectId),
      getGitStatus(props.projectId, selectedCommit.value).catch(() => []),
      getGitCommits(props.projectId, 20).catch(() => []),
    ])
    treeNodes.value = tree
    gitChanges.value = status
    commits.value = recentCommits
    updateExpandedChangeDirs()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载文件失败'
  } finally {
    loading.value = false
  }
}

async function loadCommitChanges() {
  if (!props.projectId) return
  loading.value = true
  error.value = ''
  try {
    const status = await getGitStatus(props.projectId, selectedCommit.value)
    gitChanges.value = status
    updateExpandedChangeDirs()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载变更失败'
  } finally {
    loading.value = false
  }
}

function updateExpandedChangeDirs() {
  const { allDirPaths } = buildChangeTree(gitChanges.value)
  expandedChangeDirs.value = new Set(allDirPaths)
}

function toggleChangeDir(dirPath: string) {
  const next = new Set(expandedChangeDirs.value)
  if (next.has(dirPath)) {
    next.delete(dirPath)
  } else {
    next.add(dirPath)
  }
  expandedChangeDirs.value = next
}

function toggleAllChangeDirs() {
  const allDirs = isHistoryCommit.value
    ? historyTreeResult.value.allDirPaths
    : [...stagedTreeResult.value.allDirPaths, ...unstagedTreeResult.value.allDirPaths]
  if (expandedChangeDirs.value.size >= allDirs.length && allDirs.length > 0) {
    expandedChangeDirs.value = new Set()
  } else {
    expandedChangeDirs.value = new Set(allDirs)
  }
}

async function onLoadChildren(node: FileTreeNode) {
  if (node.loading || node.loaded) return
  node.loading = true
  try {
    const children = await getFileTree(props.projectId, node.path)
    node.children = children
    node.loaded = true
  } catch (err) {
    console.error('Failed to load sub-directory files:', err)
  } finally {
    node.loading = false
  }
}

// Git Stage / Unstage / Discard handlers
async function onStageFile(path: string) {
  try {
    await stageGitFiles(props.projectId, [path])
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '暂存失败'
  }
}

async function onUnstageFile(path: string) {
  try {
    await unstageGitFiles(props.projectId, [path])
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '取消暂存失败'
  }
}

async function onStageAll() {
  try {
    await stageGitFiles(props.projectId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '暂存全部失败'
  }
}

async function onUnstageAll() {
  try {
    await unstageGitFiles(props.projectId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '取消全部暂存失败'
  }
}

function onDiscardFile(path: string) {
  discardConfirmPath.value = path
  discardModalOpen.value = true
}

function closeDiscardModal() {
  if (discarding.value) return
  discardModalOpen.value = false
  discardConfirmPath.value = ''
}

async function confirmDiscard() {
  if (!discardConfirmPath.value || discarding.value) return
  discarding.value = true
  try {
    await discardGitFile(props.projectId, discardConfirmPath.value)
    discardModalOpen.value = false
    discardConfirmPath.value = ''
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '放弃修改失败'
  } finally {
    discarding.value = false
  }
}

async function onStageAllAndCommit(message: string) {
  try {
    await stageGitFiles(props.projectId)
    await commitGitChanges(props.projectId, message)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '提交失败'
  }
}

watch(
  () => props.projectId,
  () => {
    selectedCommit.value = ''
    void refresh()
  },
  { immediate: true },
)

watch(selectedCommit, () => {
  void loadCommitChanges()
})

watch(searchQuery, (newQuery) => {
  const q = newQuery.trim()
  if (searchTimer) clearTimeout(searchTimer)
  if (!q) {
    searchResults.value = []
    searchLoading.value = false
    return
  }
  searchLoading.value = true
  searchTimer = setTimeout(async () => {
    try {
      const resp = await searchProjectFiles(props.projectId, q, 50)
      if (searchQuery.value.trim() === q) {
        searchResults.value = resp.files || []
      }
    } catch {
      searchResults.value = []
    } finally {
      if (searchQuery.value.trim() === q) {
        searchLoading.value = false
      }
    }
  }, 150)
})

function onSelectChangeFile(path: string) {
  emit('select', path, {
    mode: 'diff',
    commitRef: selectedCommit.value || undefined,
    commitMessage: currentCommitMessage.value || undefined,
  })
}

function onSelectAllFile(path: string) {
  emit('select', path, { mode: 'view' })
}

function onDragStart(event: DragEvent, path: string) {
  emit('dragstart', event, path)
}
</script>

<template>
  <div class="file-sidebar flex flex-col h-full bg-white dark:bg-slate-950 border-r border-slate-200 dark:border-slate-800 overflow-hidden select-none" data-test="file-sidebar">
    <!-- Top header: Tabs + Branch Selector + Refresh -->
    <div class="sidebar-header flex items-center justify-between p-2 border-b border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-900/60 shrink-0 gap-1.5">
      <div class="sidebar-tabs flex items-center gap-1">
        <button
          type="button"
          class="sidebar-tab inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium transition-colors cursor-pointer"
          :class="[
            activeTab === 'changes'
              ? 'is-active text-blue-600 dark:text-blue-400 bg-white dark:bg-slate-800 font-semibold'
              : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800/50',
          ]"
          @click="activeTab = 'changes'"
        >
          变更
          <span
            v-if="changesCount > 0"
            class="badge-count px-1.5 py-0.2 rounded-full text-[10px] bg-blue-100 dark:bg-blue-950 text-blue-600 dark:text-blue-400 font-mono font-semibold"
          >
            {{ changesCount }}
          </span>
        </button>
        <button
          type="button"
          class="sidebar-tab inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium transition-colors cursor-pointer"
          :class="[
            activeTab === 'all'
              ? 'is-active text-blue-600 dark:text-blue-400 bg-white dark:bg-slate-800 font-semibold'
              : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800/50',
          ]"
          @click="activeTab = 'all'"
        >
          所有文件
        </button>
      </div>

      <div class="flex items-center gap-1">
        <!-- Branch selector in changes tab -->
        <BranchSelector
          v-if="activeTab === 'changes' && !isHistoryCommit"
          :project-id="props.projectId"
          @branch-changed="refresh"
          @error="error = $event"
        />

        <button
          type="button"
          class="refresh-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 hover:bg-slate-200/60 dark:hover:bg-slate-800 transition-colors cursor-pointer"
          title="刷新文件树"
          aria-label="刷新文件树"
          :disabled="loading"
          @click="refresh"
        >
          <svg
            class="w-3.5 h-3.5"
            :class="{ 'animate-spin': loading }"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path d="M13.5 8a5.5 5.5 0 11-1.3-3.5L14 3m0 0v4m0-4h-4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Changes Tab: Commit Selector -->
    <div v-if="activeTab === 'changes'" class="commit-selector-box px-2 py-1.5 border-b border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-900/40 shrink-0">
      <div class="flex items-center gap-1.5 w-full">
        <select
          v-model="selectedCommit"
          class="commit-select flex-1 text-xs px-2 py-1 rounded border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 text-slate-800 dark:text-slate-200 outline-none truncate font-mono"
          title="选择查看提交变更"
        >
          <option value="">当前工作区变更 (未提交)</option>
          <optgroup v-if="commits.length > 0" label="最近提交">
            <option
              v-for="c in commits"
              :key="c.hash"
              :value="c.hash"
            >
              [{{ c.shortHash }}] {{ c.message }} ({{ c.author }})
            </option>
          </optgroup>
        </select>
        <button
          type="button"
          class="collapse-toggle-btn text-[11px] px-1.5 py-1 text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded shrink-0 font-mono transition-colors"
          :title="expandedChangeDirs.size > 0 ? '全部折叠' : '全部展开'"
          @click="toggleAllChangeDirs"
        >
          {{ expandedChangeDirs.size > 0 ? '折叠' : '展开' }}
        </button>
      </div>
    </div>

    <!-- Search filter input -->
    <div class="search-box relative px-2 py-1.5 border-b border-slate-200 dark:border-slate-800 shrink-0">
      <input
        v-model="searchQuery"
        type="text"
        class="w-full text-xs px-2 py-1 pr-6 rounded border border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-900/60 text-slate-800 dark:text-slate-200 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500 dark:focus:border-blue-400 font-mono"
        placeholder="搜索文件…"
      />
      <button
        v-if="searchQuery"
        type="button"
        class="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 text-xs cursor-pointer"
        title="清空搜索"
        @click="searchQuery = ''"
      >
        ✕
      </button>
    </div>

    <!-- Content body -->
    <div class="sidebar-body flex-1 min-h-0 overflow-y-auto p-1.5">
      <div v-if="loading && treeNodes.length === 0 && gitChanges.length === 0" class="state-message text-center py-6 text-xs text-slate-400">
        正在读取文件…
      </div>
      <div v-else-if="error" class="state-error text-center py-4 px-2 text-xs text-red-600 dark:text-red-400 flex flex-col gap-2 items-center">
        <span>{{ error }}</span>
        <button type="button" class="px-2 py-0.5 rounded border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-950/50 text-red-600 dark:text-red-400 text-xs cursor-pointer" @click="refresh">重试</button>
      </div>

      <!-- Changes view (Tree layout) -->
      <div v-else-if="activeTab === 'changes'" class="changes-list flex flex-col gap-2">
        <div v-if="displayChanges.length === 0" class="state-message text-center py-6 text-xs text-slate-400">
          {{ searchQuery ? '未找到匹配的变更文件' : selectedCommit ? '该提交无文件变更' : '工作区当前无变更' }}
        </div>

        <!-- History Commit Mode: single tree -->
        <div v-else-if="isHistoryCommit" class="change-tree-container">
          <ChangeTree
            :nodes="historyTreeResult.tree"
            :active-path="props.activePath"
            :expanded-paths="expandedChangeDirs"
            :is-staged="false"
            :show-actions="false"
            @select="onSelectChangeFile"
            @toggle="toggleChangeDir"
            @dragstart="onDragStart"
          />
        </div>

        <!-- Working Directory Mode: Staged & Changes partitions -->
        <div v-else class="flex flex-col gap-2">
          <!-- Section 1: Staged Changes -->
          <div v-if="stagedItems.length > 0" class="staged-group flex flex-col">
            <div
              class="group-header flex items-center justify-between h-6 px-1.5 rounded bg-slate-100/70 dark:bg-slate-900/50 text-xs font-mono font-medium text-slate-700 dark:text-slate-300 cursor-pointer"
              @click="isStagedGroupExpanded = !isStagedGroupExpanded"
            >
              <div class="flex items-center gap-1.5">
                <svg
                  class="w-3 h-3 text-slate-400 transition-transform duration-150"
                  :class="{ 'rotate-90': isStagedGroupExpanded }"
                  viewBox="0 0 16 16"
                  fill="currentColor"
                >
                  <polygon points="5 3 11 8 5 13" />
                </svg>
                <span>已暂存的更改</span>
                <span class="text-[10px] text-slate-400 font-mono">({{ stagedItems.length }})</span>
              </div>
              <button
                type="button"
                class="unstage-all-btn p-0.5 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
                title="取消暂存所有更改"
                @click.stop="onUnstageAll"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="3" y1="8" x2="13" y2="8" />
                </svg>
              </button>
            </div>

            <div v-if="isStagedGroupExpanded" class="staged-tree-body mt-1">
              <ChangeTree
                :nodes="stagedTreeResult.tree"
                :active-path="props.activePath"
                :expanded-paths="expandedChangeDirs"
                :is-staged="true"
                :show-actions="true"
                @select="onSelectChangeFile"
                @toggle="toggleChangeDir"
                @dragstart="onDragStart"
                @unstage="onUnstageFile"
              />
            </div>
          </div>

          <!-- Section 2: Unstaged Changes -->
          <div v-if="unstagedItems.length > 0" class="unstaged-group flex flex-col">
            <div
              class="group-header flex items-center justify-between h-6 px-1.5 rounded bg-slate-100/70 dark:bg-slate-900/50 text-xs font-mono font-medium text-slate-700 dark:text-slate-300 cursor-pointer"
              @click="isUnstagedGroupExpanded = !isUnstagedGroupExpanded"
            >
              <div class="flex items-center gap-1.5">
                <svg
                  class="w-3 h-3 text-slate-400 transition-transform duration-150"
                  :class="{ 'rotate-90': isUnstagedGroupExpanded }"
                  viewBox="0 0 16 16"
                  fill="currentColor"
                >
                  <polygon points="5 3 11 8 5 13" />
                </svg>
                <span>更改</span>
                <span class="text-[10px] text-slate-400 font-mono">({{ unstagedItems.length }})</span>
              </div>
              <button
                type="button"
                class="stage-all-btn p-0.5 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
                title="暂存所有更改"
                @click.stop="onStageAll"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="8" y1="3" x2="8" y2="13" />
                  <line x1="3" y1="8" x2="13" y2="8" />
                </svg>
              </button>
            </div>

            <div v-if="isUnstagedGroupExpanded" class="unstaged-tree-body mt-1">
              <ChangeTree
                :nodes="unstagedTreeResult.tree"
                :active-path="props.activePath"
                :expanded-paths="expandedChangeDirs"
                :is-staged="false"
                :show-actions="true"
                @select="onSelectChangeFile"
                @toggle="toggleChangeDir"
                @dragstart="onDragStart"
                @stage="onStageFile"
                @discard="onDiscardFile"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- All files view -->
      <div v-else class="tree-container">
        <!-- Search query mode: show flat search results across workspace -->
        <div v-if="searchQuery">
          <div v-if="searchLoading" class="state-message text-center py-6 text-xs text-slate-400">
            正在搜索工作区文件…
          </div>
          <div v-else-if="searchResults.length === 0" class="state-message text-center py-6 text-xs text-slate-400">
            未找到匹配的文件
          </div>
          <ul v-else class="changes-ul list-none m-0 p-0 flex flex-col gap-0.5">
            <li
              v-for="item in searchResults"
              :key="item.path"
              class="change-item flex items-center justify-between px-2 py-1 rounded text-xs text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/60 cursor-pointer transition-colors"
              :class="{ 'bg-blue-50/80 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300 font-medium': props.activePath === item.path }"
              :draggable="!item.isDir"
              @click="item.isDir ? undefined : onSelectAllFile(item.path)"
              @dragstart="item.isDir ? undefined : onDragStart($event, item.path)"
            >
              <div class="flex items-center gap-1.5 truncate">
                <span class="search-file-icon text-slate-400 shrink-0" aria-hidden="true">
                  <svg v-if="item.isDir" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M2 3h4l2 2h6v8H2V3z" />
                  </svg>
                  <svg v-else viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M3 2h7l3 3v9H3V2z" />
                    <polyline points="10 2 10 5 13 5" />
                  </svg>
                </span>
                <span class="change-file-name truncate font-mono text-[11px]" :title="item.path">{{ item.path }}</span>
              </div>
            </li>
          </ul>
        </div>

        <!-- Normal tree mode: lazy loaded directory tree -->
        <div v-else-if="treeNodes.length === 0" class="state-message text-center py-6 text-xs text-slate-400">
          工作区为空
        </div>
        <FileTree
          v-else
          :nodes="treeNodes"
          :active-path="props.activePath"
          :git-changes-map="gitChangesMap"
          @select="onSelectAllFile"
          @dragstart="onDragStart"
          @load-children="onLoadChildren"
        />
      </div>
    </div>

    <!-- Bottom Git Commit Bar (visible in changes tab for working tree) -->
    <GitCommitBar
      v-if="activeTab === 'changes' && !isHistoryCommit"
      :project-id="props.projectId"
      :staged-count="stagedItems.length"
      :unstaged-count="unstagedItems.length"
      @committed="refresh"
      @stage-all-and-commit="onStageAllAndCommit"
      @error="error = $event"
    />

    <!-- Discard Confirmation Dialog -->
    <UiDialog
      v-if="discardModalOpen"
      title="放弃文件更改"
      width-class="w-full max-w-md"
      data-test="discard-confirm-dialog"
      @close="closeDiscardModal"
    >
      <div class="text-xs text-slate-600 dark:text-slate-300 flex flex-col gap-2">
        <p>确定要放弃对以下文件的所有更改吗？</p>
        <div class="p-2 rounded bg-slate-100 dark:bg-slate-900 font-mono text-[11px] text-red-600 dark:text-red-400 break-all border border-slate-200 dark:border-slate-800">
          {{ discardConfirmPath }}
        </div>
        <p class="text-[11px] text-slate-400">注意：未暂存或未提交的修改将永久丢失，此操作无法撤销。</p>
      </div>

      <template #footer>
        <UiButton
          type="button"
          variant="secondary"
          size="sm"
          :disabled="discarding"
          data-test="cancel-discard-btn"
          @click="closeDiscardModal"
        >
          取消
        </UiButton>
        <UiButton
          type="button"
          variant="danger"
          size="sm"
          :disabled="discarding"
          :loading="discarding"
          data-test="confirm-discard-btn"
          @click="confirmDiscard"
        >
          {{ discarding ? '放弃中…' : '确认放弃' }}
        </UiButton>
      </template>
    </UiDialog>
  </div>
</template>
