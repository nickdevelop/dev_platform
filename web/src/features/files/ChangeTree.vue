<script setup lang="ts">
import { computed } from 'vue'
import type { ChangeTreeNode } from './types'
import { getChangeStatusLabel } from './changeTreeUtil'

const props = withDefaults(
  defineProps<{
    nodes: ChangeTreeNode[]
    activePath?: string
    expandedPaths: Set<string>
    level?: number
    isStaged?: boolean
    showActions?: boolean
  }>(),
  {
    level: 0,
    isStaged: false,
    showActions: false,
  },
)

const emit = defineEmits<{
  select: [path: string]
  toggle: [path: string]
  dragstart: [event: DragEvent, path: string]
  stage: [path: string]
  unstage: [path: string]
  discard: [path: string]
}>()

function isExpanded(path: string): boolean {
  return props.expandedPaths.has(path)
}

function onNodeClick(node: ChangeTreeNode) {
  if (node.isDir) {
    emit('toggle', node.path)
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

export type FileKind = 'folder' | 'markdown' | 'image' | 'code' | 'config' | 'file'

function getFileKind(name: string, isDir: boolean): FileKind {
  if (isDir) return 'folder'
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (ext === 'md' || ext === 'markdown') return 'markdown'
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)) return 'image'
  if (['ts', 'js', 'vue', 'go', 'rs', 'py', 'java', 'c', 'cpp', 'sh'].includes(ext)) return 'code'
  if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return 'config'
  return 'file'
}
</script>

<template>
  <ul class="change-tree-list list-none m-0 p-0 select-none">
    <li v-for="node in props.nodes" :key="node.path" class="change-tree-item">
      <div
        class="change-tree-row group flex items-center gap-1.5 h-7 px-2 rounded cursor-pointer transition-colors text-xs"
        :class="[
          props.activePath === node.path
            ? 'is-active bg-blue-50/80 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300 font-medium'
            : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/70',
          node.isDir ? 'is-dir' : '',
        ]"
        :style="{ paddingLeft: `${props.level * 14 + 8}px` }"
        :draggable="!node.isDir"
        @click="onNodeClick(node)"
        @dragstart="onDragStart($event, node.path)"
      >
        <span
          v-if="node.isDir"
          class="expand-arrow inline-flex items-center justify-center w-3 text-slate-400 transition-transform duration-150"
          :class="{ 'is-open rotate-90': isExpanded(node.path) }"
          @click.stop="emit('toggle', node.path)"
        >
          <svg viewBox="0 0 16 16" width="9" height="9" fill="currentColor">
            <polygon points="5 3 11 8 5 13" />
          </svg>
        </span>
        <span v-else class="expand-arrow-spacer w-3 inline-block"></span>

        <span class="file-icon shrink-0 inline-flex items-center text-slate-400 dark:text-slate-500" aria-hidden="true">
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
          <template v-else-if="getFileKind(node.name, node.isDir) === 'image'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="2" width="12" height="12" rx="1" />
              <circle cx="5.5" cy="5.5" r="1" />
              <polyline points="3 13 8 8 13 13" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'code'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <polyline points="5 4 1 8 5 12" />
              <polyline points="11 4 15 8 11 12" />
              <line x1="9.5" y1="2" x2="6.5" y2="14" />
            </svg>
          </template>
          <template v-else-if="getFileKind(node.name, node.isDir) === 'config'">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <circle cx="8" cy="8" r="3" />
              <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M3 13l1.5-1.5M11.5 4.5L13 3" />
            </svg>
          </template>
          <template v-else>
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M3 2h7l3 3v9H3V2z" />
              <path d="M10 2v3h3" />
            </svg>
          </template>
        </span>

        <span class="file-name truncate flex-1" :title="node.path">
          {{ node.name }}
        </span>

        <!-- Hover Actions -->
        <div
          v-if="props.showActions && !node.isDir"
          class="file-actions opacity-0 group-hover:opacity-100 flex items-center gap-0.5 shrink-0 transition-opacity"
        >
          <button
            v-if="props.isStaged"
            type="button"
            class="action-btn p-1 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
            title="取消暂存"
            @click.stop="emit('unstage', node.path)"
          >
            <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="3" y1="8" x2="13" y2="8" />
            </svg>
          </button>
          <template v-else>
            <button
              type="button"
              class="action-btn p-1 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
              title="暂存更改"
              @click.stop="emit('stage', node.path)"
            >
              <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="8" y1="3" x2="8" y2="13" />
                <line x1="3" y1="8" x2="13" y2="8" />
              </svg>
            </button>
            <button
              type="button"
              class="action-btn p-1 rounded text-slate-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
              title="放弃更改"
              @click.stop="emit('discard', node.path)"
            >
              <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M3 7v-4h4" />
                <path d="M3 7a6 6 0 112 5.5" />
              </svg>
            </button>
          </template>
        </div>

        <!-- Folder change count badge -->
        <span
          v-if="node.isDir && node.changeCount"
          class="count-badge shrink-0 px-1.5 py-0.2 rounded-full text-[10px] bg-slate-200/70 dark:bg-slate-700/60 text-slate-500 dark:text-slate-400 font-mono"
          :title="`${node.changeCount} 个文件变更`"
        >
          {{ node.changeCount }}
        </span>

        <!-- File change status badge -->
        <span
          v-else-if="!node.isDir && node.status"
          class="git-badge shrink-0 px-1 py-0.2 rounded text-[10px] font-mono font-semibold"
          :class="getChangeStatusLabel(node.status).class"
          :title="getChangeStatusLabel(node.status).label"
        >
          {{ getChangeStatusLabel(node.status).text }}
        </span>
      </div>

      <!-- Recursive sub-tree -->
      <ChangeTree
        v-if="node.isDir && isExpanded(node.path) && node.children && node.children.length > 0"
        :nodes="node.children"
        :active-path="props.activePath"
        :expanded-paths="props.expandedPaths"
        :level="props.level + 1"
        :is-staged="props.isStaged"
        :show-actions="props.showActions"
        @select="emit('select', $event)"
        @toggle="emit('toggle', $event)"
        @dragstart="(ev, p) => emit('dragstart', ev, p)"
        @stage="emit('stage', $event)"
        @unstage="emit('unstage', $event)"
        @discard="emit('discard', $event)"
      />
    </li>
  </ul>
</template>

<style scoped>
.git-badge {
  line-height: 1.2;
}

.git-modified {
  background: color-mix(in srgb, #f59e0b 16%, transparent);
  color: #d97706;
}

.git-added {
  background: color-mix(in srgb, #10b981 16%, transparent);
  color: #059669;
}

.git-deleted {
  background: color-mix(in srgb, #ef4444 16%, transparent);
  color: #dc2626;
}

.git-untracked {
  background: color-mix(in srgb, #06b6d4 16%, transparent);
  color: #0891b2;
}

.git-renamed {
  background: color-mix(in srgb, #8b5cf6 16%, transparent);
  color: #7c3aed;
}

:root.dark .git-modified {
  color: #fbbf24;
}

:root.dark .git-added {
  color: #34d399;
}

:root.dark .git-deleted {
  color: #f87171;
}

:root.dark .git-untracked {
  color: #22d3ee;
}

:root.dark .git-renamed {
  color: #a78bfa;
}
</style>
