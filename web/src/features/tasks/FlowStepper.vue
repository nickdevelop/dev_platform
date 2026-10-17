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
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 3) calc(var(--space-unit) * 3.333);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 2.667);
}

.stepper-track {
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: relative;
  padding: 0 calc(var(--space-unit) * 1.667);
}

.stepper-track::before {
  content: '';
  position: absolute;
  top: 1rem;
  left: calc(var(--space-unit) * 5);
  right: calc(var(--space-unit) * 5);
  height: 2px;
  background: var(--color-border);
  z-index: 1;
}

.step-item {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: calc(var(--space-unit) * 1);
}

.step-indicator {
  width: 2rem;
  height: 2rem;
  border-radius: 50%;
  background: var(--color-surface-muted);
  border: 2px solid var(--color-border-strong);
  color: var(--color-text-muted);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.886rem;
  font-weight: 600;
}

.step-name {
  font-size: 0.8rem;
  color: var(--color-text-muted);
  white-space: nowrap;
}

.step-item.active .step-indicator {
  border-color: var(--color-primary);
  background: var(--color-primary);
  color: var(--color-on-primary);
  box-shadow: 0 0 0 3px var(--color-primary-soft);
}

.step-item.active .step-name {
  color: var(--color-text);
  font-weight: 600;
}

.step-item.completed .step-indicator {
  border-color: var(--color-success);
  background: var(--color-success);
  color: var(--color-on-primary);
}

.step-item.completed .step-name {
  color: var(--color-success);
}

.step-detail-card {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 2.667);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 2);
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: calc(var(--space-unit) * 1);
  flex-wrap: wrap;
}

.detail-title-group {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 1.667);
}

.step-badge {
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-size: 0.75rem;
  font-weight: 600;
  padding: calc(var(--space-unit) * 0.25) calc(var(--space-unit) * 1);
  border-radius: var(--radius);
  white-space: nowrap;
}

.step-title {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 600;
}

.status-pill {
  font-size: 0.75rem;
  padding: calc(var(--space-unit) * 0.333) calc(var(--space-unit) * 1);
  border-radius: 999px;
  white-space: nowrap;
}

.status-pill.ready {
  background: var(--color-success-soft);
  color: var(--color-success);
  border: 1px solid var(--color-success-border);
}

.status-pill.confirmed {
  background: var(--color-info-soft);
  color: var(--color-info);
  border: 1px solid var(--color-info-border);
}

.status-pill.in-progress {
  background: var(--color-warning-soft);
  color: var(--color-warning);
  border: 1px solid var(--color-warning-border);
}

.step-desc {
  margin: 0;
  font-size: 0.857rem;
  color: var(--color-text-muted);
}

.confirmation-notice {
  background: var(--color-warning-soft);
  border-left: 3px solid var(--color-warning);
  padding: calc(var(--space-unit) * 1.333) calc(var(--space-unit) * 2);
  border-radius: 0 var(--radius) var(--radius) 0;
}

.notice-badge {
  font-size: 0.786rem;
  font-weight: 700;
  color: var(--color-warning);
  margin-bottom: calc(var(--space-unit) * 0.333);
}

.notice-text {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text);
  line-height: 1.4;
}

.prompt-guide {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 1.667) calc(var(--space-unit) * 2);
}

.guide-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--color-text-muted);
  display: block;
  margin-bottom: calc(var(--space-unit) * 0.667);
}

.guide-content {
  font-size: 0.82rem;
  color: var(--color-text);
  white-space: pre-wrap;
  line-height: 1.4;
}

.stepper-actions {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 2);
  margin-top: calc(var(--space-unit) * 0.667);
  flex-wrap: wrap;
}

.notes-field {
  flex: 1;
  min-width: 14rem;
}

.notes-input {
  width: 100%;
  height: var(--control-height);
  background: var(--color-surface);
  border: 1px solid var(--color-border-strong);
  color: var(--color-text);
  padding: 0 calc(var(--space-unit) * 1.667);
  border-radius: var(--radius);
  font: inherit;
  font-size: 0.857rem;
}

.notes-input:focus {
  outline: none;
  border-color: var(--color-primary);
}

.btn-advance {
  height: var(--control-height);
  background: var(--color-primary);
  border: 1px solid var(--color-primary);
  color: var(--color-on-primary);
  padding: 0 calc(var(--space-unit) * 2.667);
  border-radius: var(--radius);
  font: inherit;
  font-size: 0.857rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
}

.btn-advance:hover {
  background: var(--color-primary-hover);
  border-color: var(--color-primary-hover);
}

.btn-advance:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ready-banner {
  color: var(--color-success);
  font-size: 0.857rem;
  font-weight: 500;
}
</style>
