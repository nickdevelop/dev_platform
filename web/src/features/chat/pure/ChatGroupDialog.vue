<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { ChatGroup } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import AppDialog from '../../../shared/components/AppDialog.vue'

const props = defineProps<{
  mode: 'create' | 'rename'
  group?: ChatGroup | null
}>()

const emit = defineEmits<{
  close: []
  submitted: [group: ChatGroup]
}>()

const chats = useChatsStore()
const nameInput = ref('')
const error = ref('')
const submitting = ref(false)

onMounted(() => {
  if (props.mode === 'rename' && props.group) {
    nameInput.value = props.group.name
  }
})

const title = computed(() => {
  return props.mode === 'create' ? '新建对话分组' : `重命名分组「${props.group?.name || ''}」`
})

const confirmLabel = computed(() => {
  return props.mode === 'create' ? '创建' : '保存'
})

async function onSubmit() {
  const name = nameInput.value.trim()
  if (!name) {
    error.value = '分组名称不能为空。'
    return
  }
  if (name.length > 60) {
    error.value = '分组名称不能超过 60 个字符。'
    return
  }

  submitting.value = true
  error.value = ''
  try {
    if (props.mode === 'create') {
      const created = await chats.createGroup(name)
      if (created) {
        emit('submitted', created)
        emit('close')
      } else {
        error.value = chats.actionError || '创建分组失败。'
      }
    } else if (props.mode === 'rename' && props.group) {
      const success = await chats.renameGroup(props.group.id, name)
      if (success) {
        emit('submitted', { ...props.group, name })
        emit('close')
      } else {
        error.value = chats.actionError || '重命名分组失败。'
      }
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AppDialog :title="title" @close="emit('close')">
    <form class="group-form" @submit.prevent="onSubmit">
      <label class="field">
        <span class="label-text">分组名称</span>
        <input
          v-model="nameInput"
          type="text"
          class="text-input"
          placeholder="例如：日常工作、项目设计"
          maxlength="60"
          autofocus
          required
        />
      </label>

      <p v-if="error" class="error-msg" role="alert">{{ error }}</p>

      <div class="actions">
        <button type="button" class="btn cancel" @click="emit('close')">取消</button>
        <button type="submit" class="btn submit" :disabled="submitting || !nameInput.trim()">
          {{ submitting ? '提交中…' : confirmLabel }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
.group-form {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
  min-width: 320px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.label-text {
  font-size: 0.857rem;
  font-weight: 500;
  color: var(--color-text-muted);
}

.text-input {
  padding: calc(var(--space-unit) * 0.8) calc(var(--space-unit) * 1.2);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.text-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-info-soft);
}

.error-msg {
  margin: 0;
  color: var(--color-danger);
  font-size: 0.857rem;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: calc(var(--space-unit) * 1);
  margin-top: calc(var(--space-unit) * 0.5);
}

.btn {
  padding: calc(var(--space-unit) * 0.6) calc(var(--space-unit) * 1.5);
  border-radius: var(--radius);
  font: inherit;
  font-size: 0.857rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn.cancel {
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface);
  color: var(--color-text);
}

.btn.cancel:hover {
  background: var(--color-surface-muted);
}

.btn.submit {
  border: 1px solid var(--color-primary);
  background: var(--color-primary);
  color: #fff;
}

.btn.submit:hover:not(:disabled) {
  opacity: 0.9;
}

.btn.submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
