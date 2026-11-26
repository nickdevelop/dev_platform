<script setup lang="ts">
// NoteItem: the body of one `el-menu-item` in the notes sidebar.
//
// The row shell — hover, active edge, keyboard, navigation — belongs to the
// library's `el-menu-item` (see NotesSidebar). What is left here is the note's
// own three lines: title, first line of the body, and the project / time meta.
// The row keeps the delete action, but stops the click from reaching the menu
// item, because opening a confirmation is not the same intent as opening a note.
import { computed } from 'vue'
import { Delete } from '@element-plus/icons-vue'
import type { Note } from '../../api/notes'
import { useProjectsStore } from '../../stores/projects'
import { formatMessageTime } from '../../shared/format'

const props = defineProps<{
  note: Note
}>()

const emit = defineEmits<{
  delete: [note: Note]
}>()

const projectsStore = useProjectsStore()

const projectName = computed(() => {
  if (!props.note.projectId) return '全局'
  const project = projectsStore.projects.find((p) => p.id === props.note.projectId)
  return project ? project.name : `项目 #${props.note.projectId}`
})

const snippet = computed(() => {
  if (!props.note.content) return '无内容'
  const first = props.note.content.split('\n').map((line) => line.trim()).filter(Boolean)[0]
  return first || '无内容'
})

const updatedTime = computed(() => formatMessageTime(props.note.updatedAt))

function onDelete(event: MouseEvent) {
  // The menu item owns the click; deleting a note must not also open it.
  event.stopPropagation()
  emit('delete', props.note)
}
</script>

<template>
  <div class="note-row">
    <div class="note-row-head">
      <span class="note-title" :title="note.title">{{ note.title || '无标题笔记' }}</span>
      <el-button
        link
        type="danger"
        :icon="Delete"
        class="note-delete"
        title="删除笔记"
        aria-label="删除笔记"
        data-test="note-delete"
        @click="onDelete"
      />
    </div>
    <span class="note-snippet" :title="snippet">{{ snippet }}</span>
    <div class="note-row-meta">
      <span
        class="note-project"
        :class="{ 'note-project--bound': note.projectId > 0 }"
        :title="projectName"
      >
        {{ projectName }}
      </span>
      <span v-if="updatedTime" class="note-time font-mono">{{ updatedTime }}</span>
    </div>
  </div>
</template>

<style scoped>
.note-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  min-width: 0;
  text-align: left;
}

.note-row-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.note-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--text-size-13);
  color: var(--color-text);
}

/* Deleting is a per-row action: it appears with the pointer instead of
   shouting at every row at rest. The row fills the menu item, so its own hover
   is enough — the library's item class stays out of this file. */
.note-delete {
  flex: none;
  width: 20px;
  padding: 0;
  opacity: 0;
}

.note-row:hover .note-delete,
.note-delete:focus-visible {
  opacity: 1;
}

.note-snippet {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.note-row-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: var(--text-size-11);
}

/* Project binding is not an enum with a fixed palette (every project is its own
   value), so it stays a neutral chip; the accent tint only says「绑定了项目」,
   which is a state, not a category. */
.note-project {
  flex: none;
  max-width: 104px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 0 4px;
  border-radius: 2px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
}

.note-project--bound {
  color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 10%, var(--color-surface));
}

.note-time {
  margin-left: auto;
  flex: none;
  color: var(--color-text-muted);
}
</style>
