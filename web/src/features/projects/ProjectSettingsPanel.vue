<script setup lang="ts">
// Project settings, opened as a workbench tab: project facts and the
// danger-zone actions. The manage actions are handed up so the shell owns the
// single manage dialog.
//
// Backend diagnostics and machine-level agent services live in platform
// settings (Platform Settings → Agent), because they describe the machine, not
// one project. This panel therefore neither reads nor renders them.
import { computed } from 'vue'
import type { Project } from '../../api/projects'
import { useProjectsStore } from '../../stores/projects'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{
  manage: [kind: 'move' | 'rename' | 'rename-name' | 'unregister', project: Project]
}>()

const projectsStore = useProjectsStore()

const project = computed(() => projectsStore.byId(props.projectId) ?? null)

function manage(kind: 'move' | 'rename' | 'rename-name' | 'unregister') {
  if (project.value) emit('manage', kind, project.value)
}
</script>

<template>
  <section class="project-settings" aria-label="项目设置">
    <header class="header">
      <div>
        <h1>{{ project?.name ?? '项目不可用' }}</h1>
        <p class="path">{{ project?.path }}</p>
      </div>
      <div class="actions">
        <button type="button" @click="manage('rename-name')">修改名称</button>
        <button type="button" @click="manage('rename')">重命名</button>
        <button type="button" @click="manage('move')">移动</button>
        <button type="button" class="unregister" @click="manage('unregister')">解除登记</button>
      </div>
    </header>
  </section>
</template>

<style scoped>
.project-settings {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.header h1 {
  font-size: 1.25rem;
  margin: 0;
}

.path {
  color: var(--color-text-muted);
  font-size: 0.85rem;
  margin: 0.25rem 0 0;
}

.actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.actions button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
}

.actions .delete,
.actions .unregister {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.actions .unregister {
  color: var(--color-text);
  border-color: var(--color-border-strong);
}
</style>
