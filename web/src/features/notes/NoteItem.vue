<script setup lang="ts">
import { computed } from 'vue'
import type { Note } from '../../api/notes'
import { useProjectsStore } from '../../stores/projects'

const props = defineProps<{
  note: Note
  active: boolean
}>()

const emit = defineEmits<{
  select: [id: number]
  delete: [note: Note]
}>()

const projectsStore = useProjectsStore()

const projectName = computed(() => {
  if (!props.note.projectId) return '全局'
  const p = projectsStore.projects.find((proj) => proj.id === props.note.projectId)
  return p ? p.name : `项目 #${props.note.projectId}`
})

const snippet = computed(() => {
  if (!props.note.content) return '无内容'
  // 取首个非空行作为摘要
  const lines = props.note.content.split('\n').map((l) => l.trim()).filter(Boolean)
  return lines[0] || '无内容'
})

const formattedTime = computed(() => {
  if (!props.note.updatedAt) return ''
  const date = new Date(props.note.updatedAt)
  const now = new Date()
  const isToday =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  if (isToday) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
  return date.toLocaleDateString([], { month: 'numeric', day: 'numeric' })
})

function onClick() {
  emit('select', props.note.id)
}

function onDelete(event: MouseEvent) {
  event.stopPropagation()
  emit('delete', props.note)
}
</script>

<template>
  <div
    class="sidebar-card note-item"
    :class="{ active }"
    role="button"
    tabindex="0"
    :aria-pressed="active"
    @click="onClick"
    @keydown.enter.space="onClick"
  >
    <div class="sidebar-card-row-top">
      <div class="note-title-wrap">
        <span class="sidebar-card-title note-title" :title="note.title">
          {{ note.title || '无标题笔记' }}
        </span>
      </div>
      <div class="note-actions">
        <button
          type="button"
          class="note-delete-btn"
          title="删除笔记"
          aria-label="删除笔记"
          @click="onDelete"
        >
          <svg class="note-btn-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="3 6 5 6 21 6" />
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
          </svg>
        </button>
      </div>
    </div>

    <div class="sidebar-card-row-bottom note-bottom">
      <div class="note-snippet" :title="snippet">
        {{ snippet }}
      </div>
      <div class="note-meta">
        <span class="note-badge" :class="{ 'is-project': note.projectId > 0 }">
          {{ projectName }}
        </span>
        <span class="note-time tabular-nums">{{ formattedTime }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.note-item {
  gap: 4px;
}

.note-title-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex: 1;
}

.note-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.note-actions {
  display: flex;
  align-items: center;
  opacity: 0;
  transition: opacity 0.12s ease-out;
}

.note-item:hover .note-actions {
  opacity: 1;
}

.note-delete-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  border-radius: var(--radius, 3px);
  cursor: pointer;
  transition: color 0.12s ease-out, background-color 0.12s ease-out;
}

.note-delete-btn:hover {
  color: var(--color-danger, #ef4444);
  background: var(--color-surface);
}

.note-btn-icon {
  width: 12px;
  height: 12px;
}

.note-bottom {
  display: flex;
  flex-direction: column;
  /* The shared row-bottom rule centers its children; in this vertical stack
     that would center the snippet and meta line horizontally, so undo it. */
  align-items: stretch;
  justify-content: flex-start;
  text-align: left;
  gap: 4px;
}

.note-snippet {
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.note-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  width: 100%;
  font-size: var(--text-size-11);
}

.note-badge {
  display: inline-flex;
  align-items: center;
  padding: 1px 5px;
  border-radius: 2px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-size: var(--text-size-10);
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.note-badge.is-project {
  color: var(--color-accent, #4a8ad6);
  background: rgba(74, 138, 214, 0.08);
}

.note-time {
  color: var(--color-text-muted);
  font-size: var(--text-size-11);
}
</style>
