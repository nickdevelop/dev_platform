<script setup lang="ts">
// Capability-gated tool confirmation. The model picker and the send/stop action
// live in the composer; this component renders only when the session explicitly
// declares toolConfirmation, so an unsupported control is never shown as if it
// worked.
import type { Capabilities } from '../api/sessions'

const props = defineProps<{
  capabilities: Capabilities
  pendingToolCall?: { callId: string; name: string } | null
}>()

const emit = defineEmits<{
  'confirm-tool': [callId: string]
  'deny-tool': [callId: string]
}>()
</script>

<template>
  <section
    v-if="props.capabilities.toolConfirmation"
    class="tool-confirmation"
    data-test="tool-confirmation"
    aria-label="工具确认"
  >
    <template v-if="props.pendingToolCall">
      <p>工具 {{ props.pendingToolCall.name }} 请求执行，需要确认。</p>
      <button type="button" class="confirm-tool" @click="emit('confirm-tool', props.pendingToolCall.callId)">
        允许
      </button>
      <button type="button" class="deny-tool" @click="emit('deny-tool', props.pendingToolCall.callId)">
        拒绝
      </button>
    </template>
    <p v-else class="no-pending">当前没有待确认的工具调用。</p>
  </section>
</template>

<style scoped>
.tool-confirmation {
  border: 1px solid var(--color-warning-border);
  background: var(--color-warning-soft);
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
  margin: 0.5rem 0;
}

.tool-confirmation p {
  margin: 0 0 0.4rem;
}

.tool-confirmation button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  margin-right: 0.4rem;
}

.no-pending {
  color: var(--color-text-muted);
}
</style>
