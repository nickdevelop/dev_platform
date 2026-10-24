<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FileTreeNode, GitFileChangeStatus } from './types'

const props = withDefaults(
  defineProps<{
    nodes: FileTreeNode[]
    activePath?: string
    gitChangesMap?: Map<string, GitFileChangeStatus>
    level?: number
  }>(),
  {
    level: 0,
    gitChangesMap: () => new Map(),
  },
)

const emit = defineEmits<{
  select: [path: string]
  dragstart: [event: DragEvent, path: string]
  loadChildren: [node: FileTreeNode]
}>()

const expanded = ref<Set<string>>(new Set())

function isExpanded(path: string): boolean {
  return expanded.value.has(path)
}

function toggleExpand(path: string) {
  const next = new Set(expanded.value)
  if (next.has(path)) {
    next.delete(path)
  } else {
    next.add(path)
  }
  expanded.value = next
}

function onNodeClick(node: FileTreeNode) {
  if (node.isDir) {
    if (!isExpanded(node.path)) {
      if (!node.loaded && (!node.children || node.children.length === 0)) {
        emit('loadChildren', node)
      }
      toggleExpand(node.path)
    } else {
      toggleExpand(node.path)
    }
  } else {
    emit('select', node.path)
  }
}

function onDragStart(event: DragEvent, path: string) {
  if (event.dataTransfer) {
    event.dataTransfer.setData('text/plain', `@${path}`)
    event.dataTransfer.effectAllowed = 'copy'
  }
  emit('dragstart', event, path)
}

function getFileIcon(name: string, isDir: boolean): string {
  if (isDir) return '📁'
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (ext === 'md' || ext === 'markdown') return '📝'
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)) return '🖼️'
  if (['ts', 'js', 'vue', 'go', 'rs', 'py', 'java', 'c', 'cpp', 'sh'].includes(ext)) return '📄'
  if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return '⚙️'
  return '📃'
}

function getChangeBadge(status?: GitFileChangeStatus): { text: string; label: string } | null {
  if (!status) return null
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
      return null
  }
}
</script>

<template>
  <ul class="file-tree-list" :style="{ '--tree-level': props.level }">
    <li v-for="node in props.nodes" :key="node.path" class="file-tree-item">
      <div
        class="file-tree-row"
        data-test="file-tree-row"
        :data-path="node.path"
        :class="{
          'is-active': props.activePath === node.path,
          'is-dir': node.isDir,
          'has-change': !!props.gitChangesMap.get(node.path),
        }"
        :style="{ paddingLeft: `${props.level * 14 + 8}px` }"
        :draggable="!node.isDir"
        @click="onNodeClick(node)"
        @dragstart="onDragStart($event, node.path)"
      >
        <span v-if="node.isDir" class="expand-arrow" :class="{ 'is-open': isExpanded(node.path) }">
          ▶
        </span>
        <span v-else class="expand-arrow-spacer"></span>

        <span class="file-icon" aria-hidden="true">{{ getFileIcon(node.name, node.isDir) }}</span>
        <span class="file-name" :title="node.path">{{ node.name }}</span>

        <span
          v-if="!node.isDir && props.gitChangesMap.get(node.path)"
          class="git-badge"
          :class="`git-${props.gitChangesMap.get(node.path)}`"
          :title="getChangeBadge(props.gitChangesMap.get(node.path))?.label"
        >
          {{ getChangeBadge(props.gitChangesMap.get(node.path))?.text }}
        </span>
      </div>

      <div
        v-if="node.isDir && isExpanded(node.path) && node.loading"
        class="tree-state-row"
        :style="{ paddingLeft: `${(props.level + 1) * 14 + 8}px` }"
        data-test="tree-node-loading"
      >
        <span class="tree-loading-spinner" aria-hidden="true">↻</span>
        <span class="tree-state-text">加载中…</span>
      </div>

      <div
        v-else-if="node.isDir && isExpanded(node.path) && node.loaded && (!node.children || node.children.length === 0)"
        class="tree-state-row"
        :style="{ paddingLeft: `${(props.level + 1) * 14 + 8}px` }"
        data-test="tree-node-empty"
      >
        <span class="tree-state-text muted">（空文件夹）</span>
      </div>

      <FileTree
        v-else-if="node.isDir && isExpanded(node.path) && node.children && node.children.length > 0"
        :nodes="node.children"
        :active-path="props.activePath"
        :git-changes-map="props.gitChangesMap"
        :level="props.level + 1"
        @select="(p) => emit('select', p)"
        @dragstart="(e, p) => emit('dragstart', e, p)"
        @load-children="(n) => emit('loadChildren', n)"
      />
    </li>
  </ul>
</template>

<style scoped>
.file-tree-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.file-tree-item {
  margin: 0;
  padding: 0;
}

.file-tree-row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding-right: 8px;
  border-radius: 4px;
  font-size: 0.82rem;
  color: var(--color-text);
  cursor: pointer;
  user-select: none;
  transition: background-color 0.15s ease;
}

.file-tree-row:hover {
  background-color: var(--color-surface-muted, #f1f5f9);
}

.file-tree-row.is-active {
  background-color: var(--color-primary-soft, #e8effd);
  color: var(--color-primary, #2563eb);
  font-weight: 500;
}

.expand-arrow {
  display: inline-block;
  font-size: 0.65rem;
  color: var(--color-text-muted);
  transition: transform 0.15s ease;
  width: 12px;
  text-align: center;
}

.expand-arrow.is-open {
  transform: rotate(90deg);
}

.expand-arrow-spacer {
  display: inline-block;
  width: 12px;
}

.file-icon {
  font-size: 0.95rem;
  line-height: 1;
}

.file-name {
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
  line-height: 1.4;
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

.tree-state-row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 24px;
  font-size: 0.78rem;
  color: var(--color-text-muted, #64748b);
  user-select: none;
}

.tree-loading-spinner {
  display: inline-block;
  animation: treeSpin 1s linear infinite;
  font-size: 0.85rem;
  line-height: 1;
}

@keyframes treeSpin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.tree-state-text.muted {
  color: var(--color-text-muted, #94a3b8);
  font-style: italic;
}

.git-renamed {
  color: #4338ca;
  background-color: #e0e7ff;
}
</style>
