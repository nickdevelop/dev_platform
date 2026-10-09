<script setup lang="ts">
// Capability-gated session controls. Every control is rendered only when the
// backend/session explicitly declares the matching capability; an unsupported
// control is never shown as if it worked.
import type { Capabilities } from '../api/sessions'

const props = defineProps<{
  capabilities: Capabilities
  model?: string
  pendingToolCall?: { callId: string; name: string } | null
  busy?: boolean
}>()

const emit = defineEmits<{
  'update:model': [value: string]
  stop: []
  continue: []
  'confirm-tool': [callId: string]
  'deny-tool': [callId: string]
}>()

function onModelInput(event: Event) {
  emit('update:model', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="capability-controls">
    <div v-if="props.capabilities.modelSelection" class="control model-control">
      <label for="session-model">模型</label>
      <input
        id="session-model"
        type="text"
        autocomplete="off"
        spellcheck="false"
        :value="props.model ?? ''"
        @input="onModelInput"
      />
    </div>

    <button
      v-if="props.capabilities.sessionContinue"
      type="button"
      class="continue"
      :disabled="props.busy"
      @click="emit('continue')"
    >
      继续会话
    </button>

    <button
      v-if="props.capabilities.stop"
      type="button"
      class="stop"
      :disabled="props.busy"
      @click="emit('stop')"
    >
      停止运行
    </button>
    <p v-if="props.capabilities.stop" class="stop-note">
      后端按实例停止：停止后，同一实例上的其他会话将显示为已断开；其他实例不受影响。
    </p>

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
  </div>
</template>

<style scoped>
.capability-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 0.5rem;
  margin: 0.5rem 0;
}

.model-control {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.model-control input {
  padding: 0.35rem 0.5rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
}

.capability-controls button {
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.capability-controls .stop {
  color: #b91c1c;
  border-color: #fca5a5;
}

.stop-note {
  flex-basis: 100%;
  margin: 0;
  color: #64748b;
  font-size: 0.8rem;
}

.tool-confirmation {
  flex-basis: 100%;
  border: 1px solid #fde68a;
  background: #fffbeb;
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
}

.tool-confirmation p {
  margin: 0 0 0.4rem;
}

.no-pending {
  color: #64748b;
}
</style>
