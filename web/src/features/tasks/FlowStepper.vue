<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FlowDef, StepDef, Task } from '../../api/tasks'

const props = defineProps<{
  task: Task
  flowDef?: FlowDef
}>()

const emit = defineEmits<{
  advance: [confirmed: boolean, notes?: string]
}>()

const notesInput = ref('')
const advancing = ref(false)

const steps = computed<StepDef[]>(() => props.flowDef?.steps ?? [])

const currentStepDef = computed<StepDef | undefined>(() =>
  steps.value.find((s) => s.index === props.task.currentStep),
)

const isFinalStep = computed(() => {
  if (!currentStepDef.value) return false
  return currentStepDef.value.isFinal
})

// Highlight whether this step is the critical "all confirmations clear" step
const isConfirmationCriticalStep = computed(() => {
  if (props.task.flowType === 'transactional' && props.task.currentStep === 1) {
    return true
  }
  if (props.task.flowType === 'development' && props.task.currentStep === 3) {
    return true
  }
  return false
})

function handleConfirmAndAdvance() {
  advancing.value = true
  emit('advance', true, notesInput.value.trim())
  notesInput.value = ''
  advancing.value = false
}
</script>

<template>
  <div class="flow-stepper-box">
    <!-- Steps track -->
    <div class="stepper-track">
      <div
        v-for="step in steps"
        :key="step.index"
        class="step-item"
        :class="{
          active: step.index === task.currentStep,
          completed: step.index < task.currentStep || (step.isFinal && task.isConfirmed),
          confirmed: step.index === task.currentStep && task.stepStatus === 'confirmed',
        }"
      >
        <div class="step-indicator">
          <span v-if="step.index < task.currentStep || (step.isFinal && task.isConfirmed)">✓</span>
          <span v-else>{{ step.index }}</span>
        </div>
        <div class="step-name">{{ step.name }}</div>
      </div>
    </div>

    <!-- Active step detail card -->
    <div v-if="currentStepDef" class="step-detail-card">
      <div class="detail-header">
        <div class="detail-title-group">
          <span class="step-badge">第 {{ currentStepDef.index }} 步</span>
          <h3 class="step-title">{{ currentStepDef.name }}</h3>
        </div>
        <div class="detail-status">
          <span v-if="task.isConfirmed && isFinalStep" class="status-pill ready">
            全部事项已确认就绪
          </span>
          <span v-else-if="task.stepStatus === 'confirmed'" class="status-pill confirmed">
            当前阶段已确认
          </span>
          <span v-else class="status-pill in-progress">进行中</span>
        </div>
      </div>

      <p class="step-desc">{{ currentStepDef.description }}</p>

      <!-- Critical confirmation notice -->
      <div v-if="isConfirmationCriticalStep" class="confirmation-notice">
        <div class="notice-badge">⚠️ 关键免打断要求</div>
        <p class="notice-text">
          {{
            task.flowType === 'transactional'
              ? '在【任务确认】阶段，应将任务执行过程中所可能遇到的需要确认的事项确认清楚，避免执行过程中被打断。'
              : '在【制定计划】步骤中，需要将所有确认的事项确认清楚，避免在最后执行过程中打断。'
          }}
        </p>
      </div>

      <!-- Step prompt guide -->
      <div class="prompt-guide">
        <span class="guide-label">阶段系统指引与目标：</span>
        <div class="guide-content">{{ currentStepDef.prompt }}</div>
      </div>

      <!-- Action & Notes bar -->
      <div class="stepper-actions">
        <div v-if="!task.isConfirmed || !isFinalStep" class="notes-field">
          <input
            v-model="notesInput"
            type="text"
            placeholder="记录已确认的关键决策或事项备忘（可选）..."
            class="notes-input"
          />
        </div>

        <button
          v-if="!isFinalStep"
          type="button"
          class="btn-advance"
          :disabled="advancing"
          @click="handleConfirmAndAdvance"
        >
          ✓ 确认事项并进入下一步
        </button>
        <button
          v-else-if="!task.isConfirmed"
          type="button"
          class="btn-advance"
          :disabled="advancing"
          @click="handleConfirmAndAdvance"
        >
          ✓ 确认所有事项已就绪（可开启调度）
        </button>
        <div v-else class="ready-banner">
          <span>✨ 任务已进入最终执行阶段，前置确认全部就绪，可在下方直接执行或配置自动调度</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.flow-stepper-box {
  background: var(--color-card-bg, #22222a);
  border: 1px solid var(--color-border, #333);
  border-radius: 8px;
  padding: 18px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.stepper-track {
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: relative;
  padding: 0 10px;
}

.stepper-track::before {
  content: '';
  position: absolute;
  top: 16px;
  left: 30px;
  right: 30px;
  height: 2px;
  background: var(--color-border, #444);
  z-index: 1;
}

.step-item {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.step-indicator {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--color-bg, #1a1a20);
  border: 2px solid var(--color-border, #555);
  color: var(--color-text-muted, #888);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.88rem;
  font-weight: 600;
  transition: all 0.2s;
}

.step-name {
  font-size: 0.8rem;
  color: var(--color-text-muted, #888);
  white-space: nowrap;
}

.step-item.active .step-indicator {
  border-color: var(--color-primary, #3b82f6);
  background: var(--color-primary, #3b82f6);
  color: #fff;
  box-shadow: 0 0 10px rgba(59, 130, 246, 0.5);
}

.step-item.active .step-name {
  color: var(--color-text, #fff);
  font-weight: 600;
}

.step-item.completed .step-indicator {
  border-color: #10b981;
  background: #10b981;
  color: #fff;
}

.step-item.completed .step-name {
  color: #10b981;
}

.step-detail-card {
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--color-border, #3a3a46);
  border-radius: 6px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.detail-title-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.step-badge {
  background: rgba(59, 130, 246, 0.15);
  color: var(--color-primary, #3b82f6);
  font-size: 0.75rem;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
}

.step-title {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 600;
}

.status-pill {
  font-size: 0.75rem;
  padding: 3px 8px;
  border-radius: 12px;
}

.status-pill.ready {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.status-pill.confirmed {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.status-pill.in-progress {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
  border: 1px solid rgba(245, 158, 11, 0.3);
}

.step-desc {
  margin: 0;
  font-size: 0.85rem;
  color: var(--color-text-muted, #aaa);
}

.confirmation-notice {
  background: rgba(245, 158, 11, 0.12);
  border-left: 3px solid #f59e0b;
  padding: 8px 12px;
  border-radius: 0 4px 4px 0;
}

.notice-badge {
  font-size: 0.78rem;
  font-weight: 700;
  color: #f59e0b;
  margin-bottom: 2px;
}

.notice-text {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text, #ddd);
  line-height: 1.4;
}

.prompt-guide {
  background: var(--color-bg, #1a1a20);
  border: 1px solid var(--color-border, #333);
  border-radius: 4px;
  padding: 10px 12px;
}

.guide-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--color-text-muted, #888);
  display: block;
  margin-bottom: 4px;
}

.guide-content {
  font-size: 0.82rem;
  color: var(--color-text, #ccc);
  white-space: pre-wrap;
  line-height: 1.4;
}

.stepper-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
}

.notes-field {
  flex: 1;
}

.notes-input {
  width: 100%;
  background: var(--color-input-bg, #2a2a32);
  border: 1px solid var(--color-border, #444);
  color: var(--color-text, #fff);
  padding: 7px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  outline: none;
}

.notes-input:focus {
  border-color: var(--color-primary, #3b82f6);
}

.btn-advance {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: opacity 0.2s;
}

.btn-advance:hover {
  opacity: 0.9;
}

.btn-advance:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ready-banner {
  color: #34d399;
  font-size: 0.85rem;
  font-weight: 500;
}
</style>
