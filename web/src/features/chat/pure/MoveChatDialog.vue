<script setup lang="ts">
import { ref } from 'vue'
import type { Chat } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import AppDialog from '../../../shared/components/AppDialog.vue'

const props = defineProps<{
  chat: Chat
}>()

const emit = defineEmits<{
  close: []
  submitted: []
}>()

const chats = useChatsStore()
const selectedGroupId = ref<number>(props.chat.groupId ?? 0)
const submitting = ref(false)
const error = ref('')

async function onSubmit() {
  submitting.value = true
  error.value = ''
  try {
    const success = await chats.moveChatToGroup(props.chat.id, selectedGroupId.value)
    if (success) {
      emit('submitted')
      emit('close')
    } else {
      error.value = chats.actionError || '移动分组失败。'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AppDialog :title="`移动对话「${chat.title || '未命名对话'}」`" @close="emit('close')">
    <form class="move-form" @submit.prevent="onSubmit">
      <label class="field">
        <span class="label-text">选择目标分组</span>
        <select v-model.number="selectedGroupId" class="select-input">
          <option :value="0">未分组</option>
          <option v-for="group in chats.groups" :key="group.id" :value="group.id">
            {{ group.name }}
          </option>
        </select>
      </label>

      <p v-if="error" class="error-msg" role="alert">{{ error }}</p>

      <div class="actions">
        <button type="button" class="btn cancel" @click="emit('close')">取消</button>
        <button type="submit" class="btn submit" :disabled="submitting">
          {{ submitting ? '移动中…' : '确认移动' }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
.move-form {
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

.select-input {
  padding: calc(var(--space-unit) * 0.8) calc(var(--space-unit) * 1.2);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.select-input:focus {
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
