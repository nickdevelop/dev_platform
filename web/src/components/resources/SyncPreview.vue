<script setup lang="ts">
// Renders the per-target synchronization preview. Every state is shown
// explicitly — including unsupported and failed — and a conflict is resolved
// as a whole resource, never silently overwritten.
import type { ConflictKind, SyncPreview, SyncState } from '../../api/resources'

defineProps<{
  previews: SyncPreview[]
  busy?: boolean
}>()

const emit = defineEmits<{
  apply: [force: boolean]
  resolve: [resourceId: number]
}>()

const stateLabels: Record<SyncState, string> = {
  pending: '待首次应用',
  applied: '已同步',
  conflict: '冲突',
  failed: '失败',
  unsupported: '不支持',
}

const conflictLabels: Record<ConflictKind, string> = {
  'unmanaged-target': '目标已存在且非平台管理',
  'external-drift': '目标被平台外部修改',
  'target-invalid': '目标不可写',
}

function stateLabel(state: SyncState): string {
  return stateLabels[state] ?? state
}

function conflictLabel(kind: ConflictKind): string {
  return conflictLabels[kind] ?? kind
}
</script>

<template>
  <div class="sync-preview" data-test="sync-preview">
    <ul v-if="previews.length" class="preview-list">
      <li
        v-for="preview in previews"
        :key="`${preview.resourceId}:${preview.target.backendId ?? ''}:${preview.target.path}`"
        class="preview-item"
        :data-state="preview.state"
      >
        <div class="preview-head">
          <span class="path">{{ preview.target.path }}</span>
          <span class="state" :class="`state-${preview.state}`">{{ stateLabel(preview.state) }}</span>
        </div>
        <p class="summary">{{ preview.changeSummary }}</p>
        <p v-if="preview.conflict" class="conflict" role="alert">
          {{ conflictLabel(preview.conflict.kind) }}
          <span v-if="preview.conflict.detail">— {{ preview.conflict.detail }}</span>
        </p>
        <button
          v-if="preview.state === 'conflict'"
          type="button"
          class="resolve"
          :data-test="`resolve-${preview.resourceId}`"
          :disabled="busy"
          @click="emit('resolve', preview.resourceId)"
        >
          解决冲突
        </button>
      </li>
    </ul>
    <p v-else class="hint">尚未预览。点击“预览同步”查看将要写入的文件。</p>

    <div class="sync-actions">
      <button
        type="button"
        data-test="apply-sync"
        :disabled="busy"
        @click="emit('apply', false)"
      >
        应用同步
      </button>
      <button
        type="button"
        class="force"
        data-test="force-sync"
        :disabled="busy"
        @click="emit('apply', true)"
      >
        强制覆盖冲突目标
      </button>
    </div>
  </div>
</template>

<style scoped>
.sync-preview {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.preview-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.preview-item {
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.4rem 0.6rem;
  font-size: 0.88rem;
}

.preview-head {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
}

.path {
  font-family: monospace;
  word-break: break-all;
}

.state {
  color: #475569;
  white-space: nowrap;
}

.state-conflict,
.state-failed {
  color: #b91c1c;
}

.state-unsupported {
  color: #92400e;
}

.summary {
  color: #64748b;
  margin: 0.2rem 0 0;
}

.conflict {
  color: #b91c1c;
  margin: 0.2rem 0 0;
}

.resolve {
  margin-top: 0.35rem;
}

.sync-actions {
  display: flex;
  gap: 0.5rem;
}

.sync-actions button {
  padding: 0.35rem 0.8rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.sync-actions .force {
  color: #b91c1c;
  border-color: #fca5a5;
}

.hint {
  color: #64748b;
  font-size: 0.85rem;
  margin: 0;
}
</style>
