<script setup lang="ts">
// Explicit conflict resolution. The dialog never overwrites silently: the user
// either confirms overwriting the target with the platform content, or cancels
// and leaves the target as it is.
import type { SyncConflict } from '../../api/resources'

defineProps<{
  open: boolean
  conflict?: SyncConflict | null
  busy?: boolean
}>()

const emit = defineEmits<{
  resolve: []
  cancel: []
}>()
</script>

<template>
  <div v-if="open" class="overlay" data-test="conflict-dialog">
    <div class="dialog" role="dialog" aria-modal="true" aria-label="解决同步冲突">
      <h3>解决同步冲突</h3>
      <p v-if="conflict" class="target">目标：{{ conflict.target.path }}</p>
      <p v-if="conflict" class="detail">{{ conflict.detail }}</p>
      <p class="warning">
        用平台当前定义覆盖该文件？目标中的现有内容将被平台版本替换，原内容已保留为备份。
      </p>
      <div class="actions">
        <button type="button" class="confirm" data-test="conflict-confirm" :disabled="busy" @click="emit('resolve')">
          覆盖并同步
        </button>
        <button type="button" data-test="conflict-cancel" :disabled="busy" @click="emit('cancel')">
          取消
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 20;
}

.dialog {
  background: #fff;
  border-radius: 10px;
  padding: 1rem 1.25rem;
  width: min(28rem, 90vw);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dialog h3 {
  margin: 0;
}

.target {
  font-family: monospace;
  word-break: break-all;
  margin: 0;
}

.detail,
.warning {
  color: #64748b;
  font-size: 0.88rem;
  margin: 0;
}

.actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
}

.actions button {
  padding: 0.35rem 0.8rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.actions .confirm {
  background: #b91c1c;
  border-color: #b91c1c;
  color: #fff;
}
</style>
