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

defineExpose({
  expanded,
  isExpanded,
  toggleExpand,
})
</script>

<template>
  <ul class="file-tree-list list-none m-0 p-0 select-none">
    <li v-for="node in props.nodes" :key="node.path" class="file-tree-item">
      <div
        class="file-tree-row flex items-center gap-1.5 h-7 px-2 rounded cursor-pointer transition-colors text-xs"
        :class="[
          props.activePath === node.path
            ? 'is-active bg-blue-50/70 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300 font-medium'
            : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/70',
          node.isDir ? 'is-dir' : '',
          props.gitChangesMap.get(node.path) ? 'has-change' : '',
        ]"
        :style="{ paddingLeft: `${props.level * 14 + 8}px` }"
        :draggable="!node.isDir"
        @click="onNodeClick(node)"
        @dragstart="onDragStart($event, node.path)"
      >
        <span
          v-if="node.isDir"
          class="expand-arrow inline-block w-3 text-[10px] text-slate-400 text-center transition-transform duration-150"
          :class="{ 'is-open rotate-90': isExpanded(node.path) }"
        >
          ▶
        </span>
        <span v-else class="expand-arrow-spacer w-3 inline-block"></span>

        <span class="file-icon text-sm leading-none shrink-0" aria-hidden="true">{{ getFileIcon(node.name, node.isDir) }}</span>
        <span class="file-name truncate flex-1 min-w-0" :title="node.path">{{ node.name }}</span>

        <span
          v-if="!node.isDir && props.gitChangesMap.get(node.path)"
          class="git-badge shrink-0 px-1 py-0.2 text-[10px] font-mono rounded font-semibold border"
          :class="[
            `git-${props.gitChangesMap.get(node.path)}`,
            props.gitChangesMap.get(node.path) === 'added'
              ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border-emerald-200 dark:border-emerald-800'
              : props.gitChangesMap.get(node.path) === 'modified'
              ? 'bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400 border-amber-200 dark:border-amber-800'
              : props.gitChangesMap.get(node.path) === 'deleted'
              ? 'bg-red-50 dark:bg-red-950/40 text-red-600 dark:text-red-400 border-red-200 dark:border-red-800'
              : 'bg-slate-100 dark:bg-slate-800 text-slate-500 border-slate-200 dark:border-slate-700',
          ]"
          :title="getChangeBadge(props.gitChangesMap.get(node.path))?.label"
        >
          {{ getChangeBadge(props.gitChangesMap.get(node.path))?.text }}
        </span>
      </div>

      <div
        v-if="node.isDir && isExpanded(node.path) && node.loading"
        class="tree-state-row flex items-center gap-1.5 h-6 text-xs text-slate-400"
        :style="{ paddingLeft: `${(props.level + 1) * 14 + 8}px` }"
        data-test="tree-node-loading"
      >
        <span class="tree-loading-spinner animate-spin inline-block text-[11px]" aria-hidden="true">↻</span>
        <span class="tree-state-text">加载中…</span>
      </div>

      <div
        v-else-if="node.isDir && isExpanded(node.path) && node.loaded && (!node.children || node.children.length === 0)"
        class="tree-state-row flex items-center h-6 text-xs text-slate-400 italic"
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
