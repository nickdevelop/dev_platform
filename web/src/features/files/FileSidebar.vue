<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  commitGitChanges,
  deleteFile,
  discardGitFile,
  getFileDownloadUrl,
  getFileTree,
  getGitCommits,
  getGitStatus,
  stageGitFiles,
  unstageGitFiles,
  uploadFiles,
} from '../../api/files'
import { searchProjectFiles } from '../../api/projects'
import { getProjectGitInfo } from '../../api/git'
import { useFileTreeStateStore } from '../../stores/fileTreeState'
import { useProjectsStore } from '../../stores/projects'
import { UiButton, UiDialog } from '../../shared/ui'
import ChangeTree from './ChangeTree.vue'
import { buildChangeTree } from './changeTreeUtil'
import FileTree from './FileTree.vue'
import GitCommitBar from './GitCommitBar.vue'
import GitCommitDetailBar from './GitCommitDetailBar.vue'
import type { FileTreeNode, GitCommitItem, GitFileChangeStatus, GitStatusItem } from './types'

const props = defineProps<{
  projectId: number
  // The session this tree belongs to. Its expansions, tab and search survive the
  // sidebar unmounting when the panel or the tree is collapsed.
  sessionId?: string
  activePath?: string
  // Non-zero scopes every file and git call to one git worktree.
  worktreeId?: number
  // Real working directory for path display and copy.
  directory?: string
  // Current git branch for display
  branch?: string
}>()

const treeState = useFileTreeStateStore()
const saved = props.sessionId ? treeState.snapshotOf(props.sessionId) : undefined
const savedData = props.sessionId ? treeState.dataOf(props.sessionId) : undefined
// Loaded data is reused only for the same project and worktree it was loaded for.
const restoredData =
  savedData && savedData.projectId === props.projectId && savedData.worktreeId === (props.worktreeId ?? 0)
    ? savedData
    : undefined
// Stored data is copied on the way in and out, so the store never holds the
// reactive tree the sidebar mutates. A node stored mid-load is stored as not
// loading: nothing is loading in the copy.
const copyOf = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T
function settledNodes(nodes: FileTreeNode[]): FileTreeNode[] {
  return nodes.map((node) => ({
    ...node,
    loading: false,
    children: node.children ? settledNodes(node.children) : undefined,
  }))
}
// A change-tree that was seeded before (restored or already loaded) keeps the
// user's expansions; only a fresh tree expands every directory once.
let changeDirsSeeded = saved !== undefined

const emit = defineEmits<{
  select: [path: string, options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string }]
  dragstart: [event: DragEvent, path: string]
  'branch-changed': [branch: string]
}>()

const activeTab = ref<'changes' | 'all'>(saved?.activeTab ?? 'all')
const loading = ref(false)
const error = ref('')
const treeNodes = ref<FileTreeNode[]>(restoredData ? copyOf(restoredData.treeNodes) : [])
const gitChanges = ref<GitStatusItem[]>(restoredData ? copyOf(restoredData.gitChanges) : [])
const commits = ref<GitCommitItem[]>(restoredData ? copyOf(restoredData.commits) : [])
const selectedCommit = ref<string>(restoredData?.selectedCommit ?? saved?.selectedCommit ?? '') // empty means working tree
const searchQuery = ref(saved?.searchQuery ?? '')
const searchResults = ref<Array<{ path: string; isDir: boolean }>>([])
const searchLoading = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | undefined

const isStagedGroupExpanded = ref(saved?.isStagedGroupExpanded ?? true)
const isUnstagedGroupExpanded = ref(saved?.isUnstagedGroupExpanded ?? true)
const expandedChangeDirs = ref<Set<string>>(new Set(saved?.expandedChangeDirs ?? []))
const expandedTreePaths = ref<Set<string>>(new Set(saved?.expandedTreePaths ?? []))

// Discard confirmation dialog state
const projectsStore = useProjectsStore()
const currentProject = computed(() => (projectsStore.projects ?? []).find((p) => p.id === props.projectId))
const workspaceDir = computed(() => props.directory || currentProject.value?.path || '')
const localBranch = ref('')
const effectiveBranch = computed(() => props.branch || localBranch.value)

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

const selectedCommitItem = computed<GitCommitItem | undefined>(() => {
  if (!selectedCommit.value) return undefined
  return commits.value.find((item) => item.hash === selectedCommit.value)
})

const currentCommitMessage = computed(() => {
  return selectedCommitItem.value?.message || ''
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

// Keeps the loaded data for the next open of this session's tree.
function persistData() {
  if (!props.sessionId) return
  treeState.saveData(props.sessionId, {
    projectId: props.projectId,
    worktreeId: props.worktreeId ?? 0,
    selectedCommit: selectedCommit.value,
    treeNodes: copyOf(settledNodes(treeNodes.value)),
    gitChanges: copyOf(gitChanges.value),
    commits: copyOf(commits.value),
  })
}

async function refresh() {
  if (!props.projectId) return
  loading.value = true
  error.value = ''
  try {
    const [tree, status, recentCommits, gitInfo] = await Promise.all([
      getFileTree(props.projectId, undefined, props.worktreeId),
      getGitStatus(props.projectId, selectedCommit.value, props.worktreeId).catch(() => []),
      getGitCommits(props.projectId, 20, props.worktreeId).catch(() => []),
      getProjectGitInfo(props.projectId, props.worktreeId).catch(() => null),
    ])
    treeNodes.value = tree
    gitChanges.value = status
    commits.value = recentCommits
    if (gitInfo?.branch) {
      localBranch.value = gitInfo.branch
    }
    updateExpandedChangeDirs()
    // The children are loaded through the reactive nodes: writing to the raw
    // response would leave the view showing the loading state.
    await loadExpandedChildren(treeNodes.value)
    persistData()
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
    const status = await getGitStatus(props.projectId, selectedCommit.value, props.worktreeId)
    gitChanges.value = status
    updateExpandedChangeDirs()
    persistData()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载变更失败'
  } finally {
    loading.value = false
  }
}

function updateExpandedChangeDirs() {
  if (changeDirsSeeded) return
  changeDirsSeeded = true
  const { allDirPaths } = buildChangeTree(gitChanges.value)
  expandedChangeDirs.value = new Set(allDirPaths)
}

function toggleTreePath(path: string) {
  const next = new Set(expandedTreePaths.value)
  if (next.has(path)) {
    next.delete(path)
  } else {
    next.add(path)
  }
  expandedTreePaths.value = next
}

// A refresh replaces the nodes, so the children of directories the user has
// open are fetched again; otherwise those directories would show as empty.
async function loadExpandedChildren(nodes: FileTreeNode[]) {
  for (const node of nodes) {
    if (!node.isDir || !expandedTreePaths.value.has(node.path)) continue
    if (!node.loaded) await onLoadChildren(node)
    if (node.children && node.children.length > 0) await loadExpandedChildren(node.children)
  }
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
    const children = await getFileTree(props.projectId, node.path, props.worktreeId)
    node.children = children
    node.loaded = true
    persistData()
  } catch (err) {
    console.error('Failed to load sub-directory files:', err)
  } finally {
    node.loading = false
  }
}

// Git Stage / Unstage / Discard handlers
async function onStageFile(path: string) {
  try {
    await stageGitFiles(props.projectId, [path], props.worktreeId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '暂存失败'
  }
}

async function onUnstageFile(path: string) {
  try {
    await unstageGitFiles(props.projectId, [path], props.worktreeId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '取消暂存失败'
  }
}

async function onStageAll() {
  try {
    await stageGitFiles(props.projectId, undefined, props.worktreeId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '暂存全部失败'
  }
}

async function onUnstageAll() {
  try {
    await unstageGitFiles(props.projectId, undefined, props.worktreeId)
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
    await discardGitFile(props.projectId, discardConfirmPath.value, props.worktreeId)
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
    await stageGitFiles(props.projectId, undefined, props.worktreeId)
    await commitGitChanges(props.projectId, message, props.worktreeId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '提交失败'
  }
}

// The first run keeps a restored session's commit, tree and tab; a later switch
// of project or worktree starts from a clean state.
let initialized = false
watch(
  () => [props.projectId, props.worktreeId],
  () => {
    if (initialized) {
      selectedCommit.value = ''
      changeDirsSeeded = false
      expandedTreePaths.value = new Set()
      void refresh()
      return
    }
    initialized = true
    // Reopening a tree whose data is still in memory shows it at once, with no
    // request; the refresh button and the session's turn end load it again. A
    // directory that was open but had not finished loading is fetched now.
    if (restoredData) void loadExpandedChildren(treeNodes.value)
    else void refresh()
  },
  { immediate: true },
)

watch(
  () => ({
    activeTab: activeTab.value,
    selectedCommit: selectedCommit.value,
    searchQuery: searchQuery.value,
    isStagedGroupExpanded: isStagedGroupExpanded.value,
    isUnstagedGroupExpanded: isUnstagedGroupExpanded.value,
    expandedChangeDirs: [...expandedChangeDirs.value],
    expandedTreePaths: [...expandedTreePaths.value],
  }),
  (snapshot) => {
    if (props.sessionId) treeState.save(props.sessionId, snapshot)
  },
)

watch(selectedCommit, () => {
  changeDirsSeeded = false
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
      const resp = await searchProjectFiles(props.projectId, q, 50, props.worktreeId)
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

// Upload file handling
const fileInputRef = ref<HTMLInputElement | null>(null)
const uploadTargetDir = ref('')
const uploading = ref(false)

function triggerUpload(dir = '') {
  uploadTargetDir.value = dir
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
    fileInputRef.value.click()
  }
}

async function onFilesSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (!files || files.length === 0 || !props.projectId) return

  uploading.value = true
  error.value = ''
  try {
    await uploadFiles(props.projectId, files, uploadTargetDir.value, props.worktreeId)
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '上传文件失败'
  } finally {
    uploading.value = false
    uploadTargetDir.value = ''
    target.value = ''
  }
}

// Download file handling
function onDownloadFile(path: string) {
  if (!props.projectId || !path) return
  const url = getFileDownloadUrl(props.projectId, path, props.worktreeId)
  const a = document.createElement('a')
  a.href = url
  a.download = path.split('/').pop() || 'download'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

// Delete file/folder handling
const deleteModalOpen = ref(false)
const deleteConfirmPath = ref('')
const deleteConfirmIsDir = ref(false)
const deleting = ref(false)

function onDeleteFile(path: string, isDir: boolean) {
  deleteConfirmPath.value = path
  deleteConfirmIsDir.value = isDir
  deleteModalOpen.value = true
}

function closeDeleteModal() {
  if (deleting.value) return
  deleteModalOpen.value = false
  deleteConfirmPath.value = ''
  deleteConfirmIsDir.value = false
}

async function confirmDelete() {
  if (!props.projectId || !deleteConfirmPath.value) return
  deleting.value = true
  error.value = ''
  try {
    await deleteFile(props.projectId, deleteConfirmPath.value, props.worktreeId)
    deleteModalOpen.value = false
    deleteConfirmPath.value = ''
    deleteConfirmIsDir.value = false
    await refresh()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '删除失败'
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="file-sidebar flex flex-col h-full bg-platform-surface border-r border-platform-border overflow-hidden select-none" data-test="file-sidebar">
    <!-- Hidden file input for uploads -->
    <input
      ref="fileInputRef"
      type="file"
      multiple
      class="hidden"
      @change="onFilesSelected"
    />

    <!-- Top header: Tabs + Path & Branch + Upload + Refresh -->
    <div class="sidebar-header flex items-center justify-between p-2 border-b border-platform-border bg-platform-surface-muted shrink-0 gap-1.5 min-w-0">
      <div class="sidebar-tabs flex items-center gap-1.5 min-w-0 flex-1">
        <button
          type="button"
          class="sidebar-tab inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium transition-colors cursor-pointer shrink-0"
          :class="[
            activeTab === 'changes'
              ? 'is-active text-platform-primary bg-platform-surface font-semibold shadow-xs'
              : 'text-platform-text-muted hover:text-platform-text hover:bg-platform-surface-muted',
          ]"
          @click="activeTab = 'changes'"
        >
          变更
          <span
            v-if="changesCount > 0"
            class="badge-count px-1.5 py-0.2 rounded-full text-3xs bg-platform-primary-soft text-platform-primary font-mono font-semibold"
          >
            {{ changesCount }}
          </span>
        </button>
        <button
          type="button"
          class="sidebar-tab inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium transition-colors cursor-pointer shrink-0"
          :class="[
            activeTab === 'all'
              ? 'is-active text-platform-primary bg-platform-surface font-semibold shadow-xs'
              : 'text-platform-text-muted hover:text-platform-text hover:bg-platform-surface-muted',
          ]"
          @click="activeTab = 'all'"
        >
          所有文件
        </button>

        <!-- 项目路径：在所有文件选项右侧显示，显示全，位置不够显示...，移除复制按钮 -->
        <span
          v-if="workspaceDir"
          class="workdir-path text-3xs font-mono text-platform-text-muted truncate min-w-0"
          :title="workspaceDir"
          data-test="workdir-path"
        >
          {{ workspaceDir }}
        </span>

        <!-- 当前分支：在所有文件选项右侧显示，纯只读 label 标识当前分支，不允许选择切换 -->
        <span
          v-if="effectiveBranch"
          class="branch-chip inline-flex items-center gap-1 h-[22px] px-2 rounded text-xs font-mono font-medium text-slate-700 dark:text-slate-300 bg-platform-surface border border-platform-border shrink-0 max-w-[140px] truncate select-none"
          :title="`当前分支: ${effectiveBranch}`"
          data-test="git-branch"
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
          <span class="truncate">{{ effectiveBranch }}</span>
        </span>
      </div>

      <div class="flex items-center gap-1 shrink-0">

        <!-- Upload file button in all tab or general -->
        <button
          v-if="activeTab === 'all'"
          type="button"
          class="upload-btn inline-flex items-center justify-center w-6 h-6 rounded text-platform-text-muted hover:text-platform-text hover:bg-platform-surface-muted transition-colors cursor-pointer"
          title="上传文件到工作区"
          aria-label="上传文件到工作区"
          :disabled="uploading"
          @click="triggerUpload('')"
        >
          <svg
            class="w-3.5 h-3.5"
            :class="{ 'animate-spin': uploading }"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
          >
            <path d="M8 11V2M4 5l4-3 4 3M2 13h12" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>

        <button
          type="button"
          class="refresh-btn inline-flex items-center justify-center w-6 h-6 rounded text-platform-text-muted hover:text-platform-text hover:bg-platform-surface-muted transition-colors cursor-pointer"
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
    <div v-if="activeTab === 'changes'" class="commit-selector-box px-2 py-1.5 border-b border-platform-border bg-platform-surface-muted shrink-0">
      <div class="flex items-center gap-1.5 w-full">
        <select
          v-model="selectedCommit"
          class="commit-select flex-1 text-xs px-2 py-1 rounded border border-platform-border bg-platform-surface text-platform-text outline-none truncate font-mono"
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
          class="collapse-toggle-btn text-2xs px-1.5 py-1 text-platform-text-muted hover:text-platform-text hover:bg-platform-surface-muted rounded shrink-0 font-mono transition-colors"
          :title="expandedChangeDirs.size > 0 ? '全部折叠' : '全部展开'"
          @click="toggleAllChangeDirs"
        >
          {{ expandedChangeDirs.size > 0 ? '折叠' : '展开' }}
        </button>
      </div>
    </div>

    <!-- Search filter input -->
    <div class="search-box relative px-2 py-1.5 border-b border-platform-border shrink-0">
      <input
        v-model="searchQuery"
        type="text"
        class="w-full text-xs px-2 py-1 pr-6 rounded border border-platform-border bg-platform-surface text-platform-text placeholder:text-platform-text-muted focus:outline-none focus:border-platform-primary font-mono"
        placeholder="搜索文件…"
      />
      <button
        v-if="searchQuery"
        type="button"
        class="absolute right-3.5 top-1/2 -translate-y-1/2 text-platform-text-muted hover:text-platform-text text-xs cursor-pointer inline-flex items-center justify-center w-4 h-4 rounded"
        title="清空搜索"
        @click="searchQuery = ''"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="3" y1="3" x2="13" y2="13" />
          <line x1="13" y1="3" x2="3" y2="13" />
        </svg>
      </button>
    </div>

    <!-- Content body -->
    <div class="sidebar-body flex-1 min-h-0 overflow-y-auto p-1.5">
      <div v-if="loading && treeNodes.length === 0 && gitChanges.length === 0" class="state-message text-center py-6 text-xs text-platform-text-muted">
        正在读取文件…
      </div>
      <div v-else-if="error" class="state-error text-center py-4 px-2 text-xs text-platform-danger flex flex-col gap-2 items-center">
        <span>{{ error }}</span>
        <button type="button" class="px-2 py-0.5 rounded border border-platform-danger bg-platform-danger-soft text-platform-danger text-xs cursor-pointer" @click="refresh">重试</button>
      </div>

      <!-- Changes view (Tree layout) -->
      <div v-else-if="activeTab === 'changes'" class="changes-list flex flex-col gap-2">
        <div v-if="displayChanges.length === 0" class="state-message text-center py-6 text-xs text-platform-text-muted">
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
              class="group-header flex items-center justify-between h-6 px-1.5 rounded bg-platform-surface-muted text-xs font-mono font-medium text-platform-text cursor-pointer"
              @click="isStagedGroupExpanded = !isStagedGroupExpanded"
            >
              <div class="flex items-center gap-1.5">
                <svg
                  class="w-3 h-3 text-platform-text-muted transition-transform duration-150"
                  :class="{ 'rotate-90': isStagedGroupExpanded }"
                  viewBox="0 0 16 16"
                  fill="currentColor"
                >
                  <polygon points="5 3 11 8 5 13" />
                </svg>
                <span>已暂存的更改</span>
                <span class="text-3xs text-platform-text-muted font-mono">({{ stagedItems.length }})</span>
              </div>
              <button
                type="button"
                class="unstage-all-btn p-0.5 rounded text-platform-text-muted hover:text-platform-text hover:bg-platform-surface transition-colors"
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
              class="group-header flex items-center justify-between h-6 px-1.5 rounded bg-platform-surface-muted text-xs font-mono font-medium text-platform-text cursor-pointer"
              @click="isUnstagedGroupExpanded = !isUnstagedGroupExpanded"
            >
              <div class="flex items-center gap-1.5">
                <svg
                  class="w-3 h-3 text-platform-text-muted transition-transform duration-150"
                  :class="{ 'rotate-90': isUnstagedGroupExpanded }"
                  viewBox="0 0 16 16"
                  fill="currentColor"
                >
                  <polygon points="5 3 11 8 5 13" />
                </svg>
                <span>更改</span>
                <span class="text-3xs text-platform-text-muted font-mono">({{ unstagedItems.length }})</span>
              </div>
              <button
                type="button"
                class="stage-all-btn p-0.5 rounded text-platform-text-muted hover:text-platform-text hover:bg-platform-surface transition-colors"
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
          <div v-if="searchLoading" class="state-message text-center py-6 text-xs text-platform-text-muted">
            正在搜索工作区文件…
          </div>
          <div v-else-if="searchResults.length === 0" class="state-message text-center py-6 text-xs text-platform-text-muted">
            未找到匹配的文件
          </div>
          <ul v-else class="changes-ul list-none m-0 p-0 flex flex-col gap-0.5">
            <li
              v-for="item in searchResults"
              :key="item.path"
              class="change-item flex items-center justify-between px-2 py-1 rounded text-xs text-platform-text hover:bg-platform-surface-muted cursor-pointer transition-colors"
              :class="{ 'bg-platform-primary-soft text-platform-primary font-medium': props.activePath === item.path }"
              :draggable="!item.isDir"
              @click="item.isDir ? undefined : onSelectAllFile(item.path)"
              @dragstart="item.isDir ? undefined : onDragStart($event, item.path)"
            >
              <div class="flex items-center gap-1.5 truncate">
                <span class="search-file-icon text-platform-text-muted shrink-0" aria-hidden="true">
                  <svg v-if="item.isDir" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M2 3h4l2 2h6v8H2V3z" />
                  </svg>
                  <svg v-else viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M3 2h7l3 3v9H3V2z" />
                    <polyline points="10 2 10 5 13 5" />
                  </svg>
                </span>
                <span class="change-file-name truncate font-mono text-2xs" :title="item.path">{{ item.path }}</span>
              </div>
            </li>
          </ul>
        </div>

        <!-- Normal tree mode: lazy loaded directory tree -->
        <div v-else-if="treeNodes.length === 0" class="state-message text-center py-6 text-xs text-platform-text-muted">
          工作区为空
        </div>
        <FileTree
          v-else
          :nodes="treeNodes"
          :active-path="props.activePath"
          :git-changes-map="gitChangesMap"
          :expanded-paths="expandedTreePaths"
          @select="onSelectAllFile"
          @dragstart="onDragStart"
          @load-children="onLoadChildren"
          @toggle="toggleTreePath"
          @download="onDownloadFile"
          @delete="onDeleteFile"
          @upload-to="triggerUpload"
        />
      </div>
    </div>

    <!-- Bottom Git Commit Bar (visible in changes tab for working tree) -->
    <GitCommitBar
      v-if="activeTab === 'changes' && !isHistoryCommit"
      :project-id="props.projectId"
      :worktree-id="props.worktreeId"
      :staged-count="stagedItems.length"
      :unstaged-count="unstagedItems.length"
      @committed="refresh"
      @stage-all-and-commit="onStageAllAndCommit"
      @error="error = $event"
    />
    <!-- Bottom Git Commit Detail Bar (visible in changes tab for history commit) -->
    <GitCommitDetailBar
      v-else-if="activeTab === 'changes' && isHistoryCommit"
      :commit="selectedCommitItem"
      @reset="selectedCommit = ''"
    />

    <!-- Discard Confirmation Dialog -->
    <UiDialog
      v-if="discardModalOpen"
      title="放弃文件更改"
      width-class="w-full max-w-md"
      data-test="discard-confirm-dialog"
      @close="closeDiscardModal"
    >
      <div class="text-xs text-platform-text flex flex-col gap-2">
        <p>确定要放弃对以下文件的所有更改吗？</p>
        <div class="p-2 rounded bg-platform-surface-muted font-mono text-2xs text-platform-danger break-all border border-platform-border">
          {{ discardConfirmPath }}
        </div>
        <p class="text-2xs text-platform-text-muted">注意：未暂存或未提交的修改将永久丢失，此操作无法撤销。</p>
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

    <!-- Delete Confirmation Dialog -->
    <UiDialog
      v-if="deleteModalOpen"
      :title="deleteConfirmIsDir ? '删除目录' : '删除文件'"
      width-class="w-full max-w-md"
      data-test="delete-confirm-dialog"
      @close="closeDeleteModal"
    >
      <div class="text-xs text-platform-text flex flex-col gap-2">
        <p>确定要永久删除以下{{ deleteConfirmIsDir ? '目录及其所有内容' : '文件' }}吗？</p>
        <div class="p-2 rounded bg-platform-surface-muted font-mono text-2xs text-platform-danger break-all border border-platform-border">
          {{ deleteConfirmPath }}
        </div>
        <p class="text-2xs text-platform-text-muted">此操作将直接从磁盘中删除，无法撤销，请谨慎操作。</p>
      </div>

      <template #footer>
        <UiButton
          type="button"
          variant="secondary"
          size="sm"
          :disabled="deleting"
          data-test="cancel-delete-btn"
          @click="closeDeleteModal"
        >
          取消
        </UiButton>
        <UiButton
          type="button"
          variant="danger"
          size="sm"
          :disabled="deleting"
          :loading="deleting"
          data-test="confirm-delete-btn"
          @click="confirmDelete"
        >
          {{ deleting ? '删除中…' : '确认删除' }}
        </UiButton>
      </template>
    </UiDialog>
  </div>
</template>
