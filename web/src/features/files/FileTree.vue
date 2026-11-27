<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FileTreeNode, FileTreeMenuTarget, GitFileChangeStatus } from './types'

// Expansion is either owned by the caller (pass expandedPaths and handle
// toggle, so the state survives the tree unmounting) or kept here when the
// caller does not care. Nested levels always receive the root's set.
const props = withDefaults(
  defineProps<{
    nodes: FileTreeNode[]
    activePath?: string
    gitChangesMap?: Map<string, GitFileChangeStatus>
    level?: number
    expandedPaths?: ReadonlySet<string>
  }>(),
  {
    level: 0,
    gitChangesMap: () => new Map(),
    expandedPaths: undefined,
  },
)

const emit = defineEmits<{
  select: [path: string]
  dragstart: [event: DragEvent, path: string]
  loadChildren: [node: FileTreeNode]
  toggle: [path: string]
  // A row was right-clicked. The tree itself owns no menu and no action: it
  // reports which node and where the cursor was, and the host that owns the
  // actions (FileSidebar) opens the context menu there.
  menu: [target: FileTreeMenuTarget]
}>()

const localExpanded = ref<Set<string>>(new Set())
const expanded = computed<ReadonlySet<string>>(() => props.expandedPaths ?? localExpanded.value)

function isExpanded(path: string): boolean {
  return expanded.value.has(path)
}

function toggleExpand(path: string) {
  if (props.expandedPaths) {
    emit('toggle', path)
    return
  }
  const next = new Set(localExpanded.value)
  if (next.has(path)) {
    next.delete(path)
  } else {
    next.add(path)
  }
  localExpanded.value = next
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

// Right click is the only way to reach a row's actions, so the browser's own
// menu is suppressed (the template uses the prevent modifier).
function onContextMenu(node: FileTreeNode, event: MouseEvent) {
  emit('menu', { path: node.path, isDir: node.isDir, x: event.clientX, y: event.clientY })
}

export type FileKind = 'folder' | 'markdown' | 'diagram' | 'image' | 'code' | 'config' | 'file'

function getFileKind(name: string, isDir: boolean): FileKind {
  if (isDir) return 'folder'
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (ext === 'md' || ext === 'markdown') return 'markdown'
  if (['puml', 'plantuml', 'pu', 'uml'].includes(ext)) return 'diagram'
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)) return 'image'
  if (['ts', 'js', 'vue', 'go', 'rs', 'py', 'java', 'c', 'cpp', 'sh'].includes(ext)) return 'code'
  if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return 'config'
  return 'file'
}

function getFileIcon(name: string, isDir: boolean): string {
  return getFileKind(name, isDir)
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
            ? 'is-active bg-platform-primary-soft text-platform-primary font-medium'
            : 'text-platform-text hover:bg-platform-surface-muted',
          node.isDir ? 'is-dir' : '',
          props.gitChangesMap.get(node.path) ? 'has-change' : '',
        ]"
        :style="{ paddingLeft: `${props.level * 14 + 8}px` }"
        :draggable="!node.isDir"
        @click="onNodeClick(node)"
        @dragstart="onDragStart($event, node.path)"
        @contextmenu.prevent="onContextMenu(node, $event)"
      >
        <span
          v-if="node.isDir"
          class="expand-arrow inline-flex items-center justify-center w-3 text-platform-text-muted transition-transform duration-150"
          :class="{ 'is-open rotate-90': isExpanded(node.path) }"
        >
          <svg viewBox="0 0 16 16" width="9" height="9" fill="currentColor">
            <polygon points="5 3 11 8 5 13" />
          </svg>
        </span>
        <span v-else class="expand-arrow-spacer w-3 inline-block"></span>

        <span class="file-icon shrink-0 inline-flex items-center text-platform-text-muted" aria-hidden="true">
          <template v-if="getFileKind(node.name, node.isDir) === 'folder'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M2 3h4l2 2h6v8H2V3z" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'markdown'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2.5" y="2" width="11" height="12" rx="1" />
              <path d="M5 6v4l2-2 2 2V6" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'diagram'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="1.5" y="2" width="5" height="4" rx="0.8" />
              <rect x="9.5" y="10" width="5" height="4" rx="0.8" />
              <path d="M6.5 4h2.5a1 1 0 0 1 1 1v5" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'image'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="2" width="12" height="12" rx="1" />
              <circle cx="5.5" cy="5.5" r="1" />
              <polyline points="3 13 8 8 13 13" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'code'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M3 2h7l3 3v9H3V2z" />
              <polyline points="10 2 10 5 13 5" />
              <path d="M6 8l-1.5 1.5 1.5 1.5M9 8l1.5 1.5-1.5 1.5" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'config'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <circle cx="8" cy="8" r="2.5" />
              <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3.05 3.05l1.41 1.41M11.54 11.54l1.41 1.41M3.05 12.95l1.41-1.41M11.54 4.46l1.41-1.41" />
            </svg>
          </template>
          <template v-else>
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M3 2h7l3 3v9H3V2z" />
              <polyline points="10 2 10 5 13 5" />
            </svg>
          </template>
        </span>
        <span class="file-name truncate flex-1 min-w-0" :title="node.path">{{ node.name }}</span>

        <span
          v-if="!node.isDir && props.gitChangesMap.get(node.path)"
          class="git-badge shrink-0 px-1.5 py-0.5 text-2xs font-mono rounded font-semibold border"
          :class="[
            `git-${props.gitChangesMap.get(node.path)}`,
            props.gitChangesMap.get(node.path) === 'added'
              ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border-emerald-200 dark:border-emerald-800'
              : props.gitChangesMap.get(node.path) === 'modified'
              ? 'bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400 border-amber-200 dark:border-amber-800'
              : props.gitChangesMap.get(node.path) === 'deleted'
              ? 'bg-red-50 dark:bg-red-950/40 text-red-600 dark:text-red-400 border-red-200 dark:border-red-800'
              : 'bg-platform-surface-muted text-platform-text-muted border-platform-border',
          ]"
          :title="getChangeBadge(props.gitChangesMap.get(node.path))?.label"
        >
          {{ getChangeBadge(props.gitChangesMap.get(node.path))?.text }}
        </span>
      </div>

      <div
        v-if="node.isDir && isExpanded(node.path) && node.loading"
        class="tree-state-row flex items-center gap-1.5 h-6 text-xs text-platform-text-muted"
        :style="{ paddingLeft: `${(props.level + 1) * 14 + 8}px` }"
        data-test="tree-node-loading"
      >
        <span class="tree-loading-spinner animate-spin inline-block text-2xs" aria-hidden="true">↻</span>
        <span class="tree-state-text">加载中…</span>
      </div>

      <div
        v-else-if="node.isDir && isExpanded(node.path) && node.loaded && (!node.children || node.children.length === 0)"
        class="tree-state-row flex items-center h-6 text-xs text-platform-text-muted italic"
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
        :expanded-paths="expanded"
        @select="(p) => emit('select', p)"
        @dragstart="(e, p) => emit('dragstart', e, p)"
        @load-children="(n) => emit('loadChildren', n)"
        @toggle="toggleExpand"
        @menu="(t) => emit('menu', t)"
      />
    </li>
  </ul>
</template>
