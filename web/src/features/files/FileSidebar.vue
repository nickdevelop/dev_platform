<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { getFileTree, getGitStatus } from '../../api/files'
import { searchProjectFiles } from '../../api/projects'
import type { FileTreeNode, GitFileChangeStatus, GitStatusItem } from './types'
import FileTree from './FileTree.vue'

const props = defineProps<{
  projectId: number
  activePath?: string
}>()

const emit = defineEmits<{
  select: [path: string]
  dragstart: [event: DragEvent, path: string]
}>()

const activeTab = ref<'changes' | 'all'>('all')
const loading = ref(false)
const error = ref('')
const treeNodes = ref<FileTreeNode[]>([])
const gitChanges = ref<GitStatusItem[]>([])
const searchQuery = ref('')
const searchResults = ref<Array<{ path: string; isDir: boolean }>>([])
const searchLoading = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | undefined

const gitChangesMap = computed(() => {
  const map = new Map<string, GitFileChangeStatus>()
  for (const item of gitChanges.value) {
    map.set(item.path, item.status)
  }
  return map
})

const changesCount = computed(() => gitChanges.value.length)

async function refresh() {
  if (!props.projectId) return
  loading.value = true
  error.value = ''
  try {
    const [tree, status] = await Promise.all([
      getFileTree(props.projectId),
      getGitStatus(props.projectId).catch(() => []),
    ])
    treeNodes.value = tree
    gitChanges.value = status
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载文件失败'
  } finally {
    loading.value = false
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

watch(() => props.projectId, () => {
  void refresh()
}, { immediate: true })

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

const displayChanges = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return gitChanges.value
  return gitChanges.value.filter((item) =>
    item.path.toLowerCase().includes(q),
  )
})

function onSelectFile(path: string) {
  emit('select', path)
}

function onDragStart(event: DragEvent, path: string) {
  emit('dragstart', event, path)
}

function getChangeStatusLabel(status: GitFileChangeStatus): { text: string; label: string } {
  switch (status) {
    case 'added':
      return { text: 'A', label: '新增' }
    case 'modified':
      return { text: 'M', label: '修改' }
    case 'deleted':
      return { text: 'D', label: '删除' }
    case 'untracked':
      return { text: 'U', label: '未跟踪' }
    case 'renamed':
      return { text: 'R', label: '重命名' }
    default:
      return { text: 'M', label: '变更' }
  }
}
</script>

<template>
  <div class="file-sidebar" data-test="file-sidebar">
    <!-- Top tabs: Changes / All -->
    <div class="sidebar-header">
      <div class="sidebar-tabs">
        <button
          type="button"
          class="sidebar-tab"
          :class="{ 'is-active': activeTab === 'changes' }"
          @click="activeTab = 'changes'"
        >
          变更
          <span v-if="changesCount > 0" class="badge-count">{{ changesCount }}</span>
        </button>
        <button
          type="button"
          class="sidebar-tab"
          :class="{ 'is-active': activeTab === 'all' }"
          @click="activeTab = 'all'"
        >
          所有文件
        </button>
      </div>
      <button
        type="button"
        class="refresh-btn"
        title="刷新文件树"
        aria-label="刷新文件树"
        :disabled="loading"
        @click="refresh"
      >
        ↻
      </button>
    </div>

    <!-- Search filter input -->
    <div class="search-box">
      <input
        v-model="searchQuery"
        type="text"
        class="search-input"
        placeholder="搜索文件…"
      />
      <button
        v-if="searchQuery"
        type="button"
        class="search-clear"
        title="清空搜索"
        @click="searchQuery = ''"
      >
        ×
      </button>
    </div>

    <!-- Content body -->
    <div class="sidebar-body">
      <div v-if="loading && treeNodes.length === 0" class="state-message">
        正在读取工作区文件…
      </div>
      <div v-else-if="error" class="state-error">
        {{ error }}
        <button type="button" class="retry-btn" @click="refresh">重试</button>
      </div>

      <!-- Changes view -->
      <div v-else-if="activeTab === 'changes'" class="changes-list">
        <div v-if="displayChanges.length === 0" class="state-message">
          {{ searchQuery ? '未找到匹配的变更文件' : '工作区当前无变更' }}
        </div>
        <ul v-else class="changes-ul">
          <li
            v-for="item in displayChanges"
            :key="item.path"
            class="change-item"
            :class="{ 'is-active': props.activePath === item.path }"
            draggable="true"
            @click="onSelectFile(item.path)"
            @dragstart="onDragStart($event, item.path)"
          >
            <span class="change-file-name" :title="item.path">{{ item.path }}</span>
            <span
              class="git-badge"
              :class="`git-${item.status}`"
              :title="getChangeStatusLabel(item.status).label"
            >
              {{ getChangeStatusLabel(item.status).text }}
            </span>
          </li>
        </ul>
      </div>

      <!-- All files view -->
      <div v-else class="tree-container">
        <!-- Search query mode: show flat search results across workspace -->
        <div v-if="searchQuery">
          <div v-if="searchLoading" class="state-message">
            正在搜索工作区文件…
          </div>
          <div v-else-if="searchResults.length === 0" class="state-message">
            未找到匹配的文件
          </div>
          <ul v-else class="changes-ul">
            <li
              v-for="item in searchResults"
              :key="item.path"
              class="change-item"
              :class="{ 'is-active': props.activePath === item.path }"
              :draggable="!item.isDir"
              @click="item.isDir ? undefined : onSelectFile(item.path)"
              @dragstart="item.isDir ? undefined : onDragStart($event, item.path)"
            >
              <span class="search-file-icon" aria-hidden="true">{{ item.isDir ? '📁' : '📄' }}</span>
              <span class="change-file-name" :title="item.path">{{ item.path }}</span>
            </li>
          </ul>
        </div>

        <!-- Normal tree mode: lazy loaded directory tree -->
        <div v-else-if="treeNodes.length === 0" class="state-message">
          工作区为空
        </div>
        <FileTree
          v-else
          :nodes="treeNodes"
          :active-path="props.activePath"
          :git-changes-map="gitChangesMap"
          @select="onSelectFile"
          @dragstart="onDragStart"
          @load-children="onLoadChildren"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.file-sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface, #ffffff);
  border-right: 1px solid var(--color-border, #e3e6eb);
  overflow: hidden;
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface-muted, #f8fafc);
  flex-shrink: 0;
}

.sidebar-tabs {
  display: flex;
  gap: 4px;
}

.sidebar-tab {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  padding: 4px 10px;
  border-radius: 4px;
  font-size: 0.82rem;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.sidebar-tab:hover {
  color: var(--color-text);
  background: rgba(0, 0, 0, 0.04);
}

.sidebar-tab.is-active {
  color: var(--color-primary, #2563eb);
  background: var(--color-surface, #ffffff);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}

.badge-count {
  font-size: 0.7rem;
  padding: 0 5px;
  border-radius: 10px;
  background: var(--color-primary-soft, #e8effd);
  color: var(--color-primary, #2563eb);
}

.refresh-btn {
  border: none;
  background: transparent;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  font-size: 1rem;
  color: var(--color-text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.refresh-btn:hover:not(:disabled) {
  background: rgba(0, 0, 0, 0.05);
  color: var(--color-text);
}

.search-box {
  position: relative;
  padding: 6px 10px;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  flex-shrink: 0;
}

.search-input {
  width: 100%;
  box-sizing: border-box;
  padding: 4px 22px 4px 8px;
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: 4px;
  font-size: 0.78rem;
  background: var(--color-bg, #f4f5f7);
  color: var(--color-text);
  outline: none;
}

.search-input:focus {
  border-color: var(--color-primary, #2563eb);
  background: var(--color-surface, #ffffff);
}

.search-clear {
  position: absolute;
  right: 16px;
  top: 50%;
  transform: translateY(-50%);
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  font-size: 0.9rem;
}

.sidebar-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 6px;
}

.state-message {
  padding: 24px 12px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.state-error {
  padding: 16px 12px;
  text-align: center;
  color: var(--color-danger, #b91c1c);
  font-size: 0.8rem;
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: center;
}

.retry-btn {
  border: 1px solid var(--color-danger-border, #fca5a5);
  background: var(--color-danger-soft, #fee2e2);
  color: var(--color-danger, #b91c1c);
  padding: 2px 8px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.75rem;
}

.changes-ul {
  list-style: none;
  margin: 0;
  padding: 0;
}

.change-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px;
  border-radius: 4px;
  font-size: 0.82rem;
  cursor: pointer;
  user-select: none;
}

.change-item:hover {
  background: var(--color-surface-muted, #f8fafc);
}

.change-item.is-active {
  background: var(--color-primary-soft, #e8effd);
  color: var(--color-primary, #2563eb);
}

.change-file-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.git-badge {
  flex-shrink: 0;
  font-size: 0.68rem;
  font-weight: 700;
  border-radius: 3px;
  padding: 0 4px;
  margin-left: 6px;
}

.git-modified {
  color: #b45309;
  background-color: #fef3c7;
}

.git-added,
.git-untracked {
  color: #15803d;
  background-color: #dcfce7;
}

.git-deleted {
  color: #b91c1c;
  background-color: #fee2e2;
  text-decoration: line-through;
}

.git-renamed {
  color: #4338ca;
  background-color: #e0e7ff;
}

.search-file-icon {
  margin-right: 6px;
  font-size: 0.9rem;
  line-height: 1;
}
</style>
