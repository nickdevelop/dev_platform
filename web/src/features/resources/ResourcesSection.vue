<script setup lang="ts">
// Resource management for one project: the canonical resource list, the
// project's associations, and the managed-file sync preview. Credential values
// are never held here — the editor sends a write-only value straight through
// the store, and only the opaque reference is retained.
import { onMounted, ref, watch } from 'vue'
import type { ResourceDefinition, ResourceDefinitionBody, SyncConflict } from '../../api/resources'
import { useResourcesStore } from '../../stores/resources'
import ConflictDialog from './ConflictDialog.vue'
import ResourceEditor from './ResourceEditor.vue'
import SyncPreview from './SyncPreview.vue'

const props = defineProps<{ projectId: number }>()

const resources = useResourcesStore()

const editorOpen = ref(false)
const editingId = ref<number | null>(null)
const conflict = ref<SyncConflict | null>(null)
const conflictResourceId = ref<number | null>(null)

onMounted(load)
watch(() => props.projectId, load)

async function load() {
  await Promise.all([resources.loadProject(props.projectId), resources.loadAll()])
}

function openNew() {
  editingId.value = null
  editorOpen.value = true
}

function openEdit(resource: ResourceDefinition) {
  editingId.value = resource.id
  editorOpen.value = true
}

function closeEditor() {
  editorOpen.value = false
  editingId.value = null
}

async function save(body: ResourceDefinitionBody) {
  const saved =
    editingId.value === null
      ? await resources.create(body)
      : await resources.update(editingId.value, body)
  if (saved) closeEditor()
}

async function remove(resource: ResourceDefinition) {
  await resources.remove(resource.id)
}

async function toggleAssociation(resource: ResourceDefinition) {
  if (resources.isAssociated(resource.id)) {
    await resources.disassociate(props.projectId, resource.id)
  } else {
    await resources.associate(props.projectId, resource.id)
  }
}

async function previewSync() {
  await resources.preview(props.projectId)
}

async function applySync(force: boolean) {
  await resources.apply(props.projectId, force)
}

function openConflict(resourceId: number) {
  const preview = resources.previews.find(
    (item) => item.resourceId === resourceId && item.conflict,
  )
  conflict.value = preview?.conflict ?? null
  conflictResourceId.value = resourceId
}

function cancelConflict() {
  conflict.value = null
  conflictResourceId.value = null
}

async function confirmConflict() {
  if (conflictResourceId.value !== null) {
    await resources.resolve(props.projectId, conflictResourceId.value)
  }
  cancelConflict()
}

const kindLabels: Record<string, string> = { skill: 'Skill', mcp: 'MCP', prompt: '提示词' }

function kindLabel(kind: string): string {
  return kindLabels[kind] ?? kind
}
</script>

<template>
  <section class="resources" aria-label="资源关联">
    <div class="section-head">
      <h2>资源管理</h2>
      <div class="section-actions">
        <button type="button" data-test="new-resource" @click="openNew">新建资源</button>
        <button
          type="button"
          data-test="preview-sync"
          :disabled="resources.syncLoading"
          @click="previewSync"
        >
          预览同步
        </button>
      </div>
    </div>

    <p v-if="resources.error" class="error" role="alert" data-test="resources-error">
      {{ resources.error }}
    </p>

    <ul class="resource-list">
      <li
        v-for="resource in resources.resources"
        :key="resource.id"
        class="resource-item"
        :data-resource-id="resource.id"
      >
        <label class="associate">
          <input
            type="checkbox"
            :checked="resources.isAssociated(resource.id)"
            :data-test="`associate-${resource.id}`"
            @change="toggleAssociation(resource)"
          />
          <span class="kind">{{ kindLabel(resource.kind) }}</span>
          <span class="name">{{ resource.name }}</span>
        </label>
        <span v-if="resource.secretRef" class="secret-ref" data-test="secret-ref">已配置凭据</span>
        <span v-if="!resource.enabled" class="disabled">已停用</span>
        <span class="spacer"></span>
        <button type="button" :data-test="`edit-${resource.id}`" @click="openEdit(resource)">
          编辑
        </button>
        <button
          type="button"
          class="delete"
          :data-test="`delete-${resource.id}`"
          @click="remove(resource)"
        >
          删除
        </button>
      </li>
      <li v-if="resources.resources.length === 0" class="empty">暂无资源。</li>
    </ul>

    <ResourceEditor
      v-if="editorOpen"
      :resource="editingId === null ? null : resources.byId(editingId)"
      @submit="save"
      @cancel="closeEditor"
    />

    <SyncPreview
      :previews="resources.previews"
      :busy="resources.syncLoading"
      @apply="applySync"
      @resolve="openConflict"
    />

    <ConflictDialog
      :open="conflict !== null"
      :conflict="conflict"
      :busy="resources.syncLoading"
      @resolve="confirmConflict"
      @cancel="cancelConflict"
    />
  </section>
</template>

<style scoped>
.resources {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-head h2 {
  margin: 0;
  font-size: 1rem;
}

.section-actions {
  display: flex;
  gap: 0.4rem;
}

.section-actions button,
.resource-item button {
  padding: 0.3rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
}

.resource-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 0.35rem 0.6rem;
}

.resource-item .associate {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.kind {
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.name {
  font-weight: 600;
}

.secret-ref {
  color: var(--color-success);
  font-size: 0.8rem;
}

.disabled {
  color: var(--color-warning);
  font-size: 0.8rem;
}

.resource-item .delete {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.spacer {
  flex: 1;
}

.empty {
  color: var(--color-text-muted);
  font-size: 0.9rem;
}

.error {
  color: var(--color-danger);
  margin: 0;
}
</style>
