<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { FlowDef, StepDef, Task } from '../../api/tasks'

const props = defineProps<{
  task: Task
  flowDef?: FlowDef
}>()

const emit = defineEmits<{
  advance: [confirmed: boolean, notes?: string, driverPrompt?: string, executionPrompt?: string]
  savePrompt: [prompt: string]
}>()

const notesInput = ref('')
const advancing = ref(false)

// For transactional task: direct execution prompt
const executionPromptInput = ref(props.task.executionPrompt || '')
watch(
  () => props.task.executionPrompt,
  (newVal) => {
    if (newVal) executionPromptInput.value = newVal
  },
)

// For development task: modal for stage transition driver prompt
const showStageModal = ref(false)
const targetNextStep = ref<number | null>(null)
const userSupplementPrompt = ref('')

const steps = computed<StepDef[]>(() => props.flowDef?.steps ?? [])

const currentStepDef = computed<StepDef | undefined>(() =>
  steps.value.find((s) => s.index === props.task.currentStep),
)

const nextStepDef = computed<StepDef | undefined>(() => {
  const nextIdx = (targetNextStep.value ?? props.task.currentStep + 1)
  return steps.value.find((s) => s.index === nextIdx)
})

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

function handleTransactionalConfirmReady() {
  const prompt = executionPromptInput.value.trim()
  if (!prompt) {
    alert('请填写或确认第二步直接执行的执行提示词')
    return
  }
  emit('savePrompt', prompt)
}

function openAdvanceModal() {
  if (props.task.flowType === 'transactional') {
    handleTransactionalConfirmReady()
    return
  }

  // Development task: open confirmation and supplement prompt modal
  targetNextStep.value = props.task.currentStep + 1
  userSupplementPrompt.value = ''
  showStageModal.value = true
}

function confirmStageTransition() {
  if (!nextStepDef.value) return
  advancing.value = true

  let fullDriverPrompt = `【推进至第 ${nextStepDef.value.index} 步：${nextStepDef.value.name}】\n阶段指引：${nextStepDef.value.prompt}`
  if (userSupplementPrompt.value.trim()) {
    fullDriverPrompt += `\n\n【用户补充要求与侧重点】：\n${userSupplementPrompt.value.trim()}`
  }

  emit('advance', true, notesInput.value.trim(), fullDriverPrompt)
  showStageModal.value = false
  notesInput.value = ''
  advancing.value = false
}

function handleFinalPlanConfirm() {
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
          <span v-if="task.status === 'completed'" class="status-pill completed-pill">
            全流程已执行完成
          </span>
          <span v-else-if="task.isConfirmed && isFinalStep" class="status-pill ready">
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
        <div class="notice-badge">⚠️ 核心免打断要求</div>
        <p class="notice-text">
          {{
            task.flowType === 'transactional'
              ? '在【任务分析】阶段，Agent 需分析任务需求，识别信息是否有歧义、执行过程是否存在不确定点与潜在风险，全面与用户确认清楚，并输出可直接执行的执行计划，作为第二步执行的提示词。'
              : '在【制定计划】步骤中，需要将所有技术方案、执行清单与前置依赖等确认事项彻底确认清楚，避免在最后执行过程中打断。'
          }}
        </p>
      </div>

      <!-- Step prompt guide -->
      <div class="prompt-guide">
        <span class="guide-label">阶段系统指引与目标：</span>
        <div class="guide-content">{{ currentStepDef.prompt }}</div>
      </div>

      <!-- Transactional Step 1 -> Step 2 execution prompt card -->
      <div
        v-if="task.flowType === 'transactional' && (!task.isConfirmed || task.currentStep === 1)"
        class="execution-prompt-box"
      >
        <div class="prompt-box-header">
          <span class="prompt-box-title">📝 提炼第二步执行提示词（Execution Prompt）</span>
          <span class="prompt-box-hint">
            任务分析完成后，请在此确认可直接执行的方案提示词，确认后任务即刻就绪。
          </span>
        </div>
        <textarea
          v-model="executionPromptInput"
          rows="3"
          placeholder="例如：请按以下已确认方案执行部署：1.拉取镜像 2.重启容器 3.健康检查..."
          class="execution-prompt-textarea"
        ></textarea>
        <button
          type="button"
          class="btn-save-prompt"
          @click="handleTransactionalConfirmReady"
        >
          ✓ 确认执行方案并就绪（保存执行提示词）
        </button>
      </div>

      <!-- Action & Notes bar -->
      <div class="stepper-actions">
        <div v-if="task.status !== 'completed' && (!task.isConfirmed || !isFinalStep)" class="notes-field">
          <input
            v-model="notesInput"
            type="text"
            placeholder="记录已确认的关键决策或事项备忘（可选）..."
            class="notes-input"
          />
        </div>

        <!-- Development task: Step 1, 2 advance -->
        <button
          v-if="task.flowType === 'development' && task.currentStep < 3"
          type="button"
          class="btn-advance"
          :disabled="advancing"
          @click="openAdvanceModal"
        >
          ✓ 确认当前阶段并进入下一步（唤起 Agent）
        </button>

        <!-- Development task: Step 3 (Plan confirm) -> Step 4 -->
        <button
          v-else-if="task.flowType === 'development' && task.currentStep === 3 && !task.isConfirmed"
          type="button"
          class="btn-advance"
          :disabled="advancing"
          @click="handleFinalPlanConfirm"
        >
          ✓ 确认执行计划已就绪（进入执行阶段）
        </button>

        <div v-else-if="task.status === 'completed'" class="completed-banner">
          <span>🎉 该开发任务已全部执行完成，生命周期已结束</span>
        </div>

        <div v-else-if="task.isConfirmed && isFinalStep" class="ready-banner">
          <span>✨ 任务已进入最终执行阶段，前置确认全部就绪，可在下方直接执行或配置自动调度</span>
        </div>
      </div>
    </div>

    <!-- Development Stage Transition Modal -->
    <div v-if="showStageModal && nextStepDef" class="modal-backdrop" @click.self="showStageModal = false">
      <div class="stage-modal" role="dialog" aria-label="推进阶段并唤起 Agent">
        <div class="modal-header">
          <h3>确认推进至第 {{ nextStepDef.index }} 步：{{ nextStepDef.name }}</h3>
          <button type="button" class="close-btn" @click="showStageModal = false">×</button>
        </div>

        <div class="modal-body">
          <div class="modal-info">
            <span class="info-label">系统将向当前会话发送该阶段指引，自动唤起 Agent 进行本阶段的处理：</span>
            <div class="default-prompt-preview">{{ nextStepDef.prompt }}</div>
          </div>

          <div class="form-group">
            <label for="user-supplement">补充要求与侧重点（可选）：</label>
            <textarea
              id="user-supplement"
              v-model="userSupplementPrompt"
              rows="3"
              placeholder="可输入对此阶段的特殊指示、设计规范要求或必须涵盖的内容..."
              class="supplement-textarea"
            ></textarea>
          </div>
        </div>

        <div class="modal-actions">
          <button type="button" class="btn-cancel" @click="showStageModal = false">取消</button>
          <button type="button" class="btn-primary" :disabled="advancing" @click="confirmStageTransition">
            {{ advancing ? '正在发送...' : '🚀 发送指令并进入下一步' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.flow-stepper-box {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
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

.status-pill.completed-pill {
  background: rgba(16, 185, 129, 0.25);
  color: #10b981;
  font-weight: 600;
  border: 1px solid rgba(16, 185, 129, 0.4);
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

/* Execution prompt box */
.execution-prompt-box {
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: 6px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.prompt-box-header {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.prompt-box-title {
  font-size: 0.88rem;
  font-weight: 600;
  color: #60a5fa;
}

.prompt-box-hint {
  font-size: 0.78rem;
  color: var(--color-text-muted, #aaa);
}

.execution-prompt-textarea {
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  font-family: inherit;
  outline: none;
}

.execution-prompt-textarea:focus {
  border-color: var(--color-primary);
}

.btn-save-prompt {
  align-self: flex-end;
  background: var(--color-primary);
  color: #fff;
  border: none;
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 0.82rem;
  font-weight: 600;
  cursor: pointer;
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
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  padding: 7px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  outline: none;
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
}

.ready-banner {
  color: #34d399;
  font-size: 0.85rem;
  font-weight: 500;
}

.completed-banner {
  color: #10b981;
  font-size: 0.9rem;
  font-weight: 600;
}

/* Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.stage-modal {
  background: var(--color-bg, #1e1e24);
  border: 1px solid var(--color-border, #333);
  border-radius: 8px;
  width: 520px;
  max-width: 90vw;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 18px;
  border-bottom: 1px solid var(--color-border, #333);
}

.modal-header h3 {
  margin: 0;
  font-size: 1.05rem;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 1.4rem;
  color: #888;
  cursor: pointer;
}

.modal-body {
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.modal-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.info-label {
  font-size: 0.82rem;
  color: var(--color-text-muted, #aaa);
}

.default-prompt-preview {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  padding: 10px;
  border-radius: 6px;
  font-size: 0.82rem;
  color: var(--color-text);
  line-height: 1.4;
  white-space: pre-wrap;
  max-height: 120px;
  overflow-y: auto;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group label {
  font-size: 0.85rem;
  font-weight: 600;
}

.supplement-textarea {
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  outline: none;
  font-family: inherit;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 18px;
  border-top: 1px solid var(--color-border, #333);
}

.btn-cancel {
  background: transparent;
  border: 1px solid #555;
  color: #eee;
  padding: 7px 14px;
  border-radius: 6px;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 7px 16px;
  border-radius: 6px;
  font-weight: 600;
  cursor: pointer;
}
</style>
