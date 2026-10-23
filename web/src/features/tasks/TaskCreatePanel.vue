<script setup lang="ts">
// Task Creation Panel: structured like NewSessionPanel with session template fields,
// adding task type selection (standard vs flow placeholder) and execution mode configuration.
import { computed, onMounted, ref, watch } from 'vue'
import type { BackendId } from '../../api/agents'
import type { ScheduleType, TaskType } from '../../api/tasks'
import ModelSelect from '../chat/composer/ModelSelect.vue'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { usePromptsStore } from '../../stores/prompts'
import { useTasksStore } from '../../stores/tasks'
import { useDraftsStore } from '../../stores/drafts'
import { applyUserPromptTemplate, hasUserPromptPlaceholder } from '../prompts/promptTemplate'

const props = defineProps<{
  initialProjectId?: number
}>()

const emit = defineEmits<{
  created: [taskId: number]
  cancel: []
}>()

const agentsStore = useAgentsStore()
const projectsStore = useProjectsStore()
const promptsStore = usePromptsStore()
const tasksStore = useTasksStore()
const draftsStore = useDraftsStore()

const draft = draftsStore.taskCreateDraft

// Task Type: Standard (Session Template) is active; Flow is a visible extension point.
const selectedTaskType = ref<TaskType>(draft.taskType ?? 'standard')

// Basic Task Info
const title = ref(draft.title ?? '')
const description = ref(draft.description ?? '')

// Template / Session Info
const selectedProjectId = ref<number | null>(
  props.initialProjectId ?? (draft.projectId !== null && draft.projectId !== undefined ? Number(draft.projectId) : null),
)
const selectedBackend = ref<BackendId | ''>(draft.backend ?? '')
const selectedModel = ref(draft.model ?? '')
const modelOk = ref(false)
const promptText = ref(draft.promptText ?? '')
const selectedPromptId = ref<number | ''>(draft.promptId ?? '')

// Execution Mode / Schedule Info
const scheduleType = ref<ScheduleType>(draft.scheduleType ?? 'manual')
const scheduledAtDate = ref(draft.scheduledAtDate ?? '')
const cronShortcut = ref(draft.cronShortcut ?? 'custom')
const customCronExpr = ref(draft.customCronExpr ?? '0 9 * * *')

watch(
  [
    selectedTaskType,
    title,
    description,
    selectedProjectId,
    selectedBackend,
    selectedModel,
    promptText,
    selectedPromptId,
    scheduleType,
    scheduledAtDate,
    cronShortcut,
    customCronExpr,
  ],
  () => {
    draftsStore.setTaskCreateDraft({
      taskType: selectedTaskType.value,
      title: title.value,
      description: description.value,
      projectId: selectedProjectId.value,
      backend: selectedBackend.value,
      model: selectedModel.value,
      promptText: promptText.value,
      promptId: selectedPromptId.value,
      scheduleType: scheduleType.value,
      scheduledAtDate: scheduledAtDate.value,
      cronShortcut: cronShortcut.value,
      customCronExpr: customCronExpr.value,
    })
  },
)

const creating = ref(false)
const errorMessage = ref('')

const selectableBackends = computed(() =>
  agentsStore.backends.filter((entry) => entry.enabled !== false),
)

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agentsStore.capabilitiesFor(selectedBackend.value),
)

const selectedPrompt = computed(() =>
  selectedPromptId.value !== '' ? promptsStore.byId(Number(selectedPromptId.value)) ?? null : null,
)

// Synchronize cron shortcut with customCronExpr
watch(cronShortcut, (val) => {
  if (val === 'hourly') {
    customCronExpr.value = '0 * * * *'
  } else if (val === 'daily_9am') {
    customCronExpr.value = '0 9 * * *'
  } else if (val === 'weekly_mon') {
    customCronExpr.value = '0 9 * * 1'
  }
})

onMounted(async () => {
  await Promise.all([
    projectsStore.load(),
    agentsStore.loadBackends(),
    promptsStore.loadPrompts(),
  ])

  if (!selectedProjectId.value && projectsStore.projects.length > 0) {
    selectedProjectId.value = projectsStore.projects[0].id
  }
  if (selectedBackend.value === '' && selectableBackends.value.length > 0) {
    selectedBackend.value = selectableBackends.value[0].backend
  }
})

function selectBackend(backend: BackendId) {
  if (!agentsStore.backendEnabledFor(backend)) return
  if (selectedBackend.value === backend) return
  selectedBackend.value = backend
  selectedModel.value = ''
  modelOk.value = false
}

function onPromptTemplateChange() {
  if (selectedPrompt.value) {
    // If user prompt hasn't been edited, fill with template content
    if (!promptText.value.trim()) {
      promptText.value = selectedPrompt.value.content
    }
  }
}

async function handleCreateTask() {
  if (selectedTaskType.value === 'flow') {
    errorMessage.value = '流程任务功能正在扩展中，敬请期待！'
    return
  }

  if (!title.value.trim()) {
    errorMessage.value = '请输入任务名称'
    return
  }
  if (!selectedProjectId.value) {
    errorMessage.value = '请选择关联项目'
    return
  }
  if (!selectedBackend.value) {
    errorMessage.value = '请选择 Agent 后端'
    return
  }
  if (selectedCapabilities.value?.modelSelection && !selectedModel.value) {
    errorMessage.value = '请选择使用的模型'
    return
  }
  if (!promptText.value.trim() && !selectedPrompt.value) {
    errorMessage.value = '请填写执行提示词模板'
    return
  }

  // Validate schedule settings
  let scheduledAt: string | undefined
  let cronExpr: string | undefined
  let scheduleEnabled = false

  if (scheduleType.value === 'once') {
    if (!scheduledAtDate.value) {
      errorMessage.value = '请选择定时执行的具体时间'
      return
    }
    const targetDate = new Date(scheduledAtDate.value)
    if (isNaN(targetDate.getTime()) || targetDate.getTime() <= Date.now()) {
      errorMessage.value = '定时执行时间必须为未来时间'
      return
    }
    scheduledAt = targetDate.toISOString()
    scheduleEnabled = true
  } else if (scheduleType.value === 'cron') {
    if (!customCronExpr.value.trim()) {
      errorMessage.value = '请输入有效的 Cron 表达式'
      return
    }
    cronExpr = customCronExpr.value.trim()
    scheduleEnabled = true
  }

  const finalPrompt = selectedPrompt.value
    ? hasUserPromptPlaceholder(selectedPrompt.value.content)
      ? applyUserPromptTemplate(selectedPrompt.value.content, promptText.value)
      : promptText.value || selectedPrompt.value.content
    : promptText.value.trim()

  creating.value = true
  errorMessage.value = ''

  try {
    const created = await tasksStore.createTask({
      title: title.value.trim(),
      description: description.value.trim(),
      taskType: 'standard',
      projectId: selectedProjectId.value,
      backend: selectedBackend.value,
      model: selectedModel.value || 'default',
      promptTemplate: finalPrompt,
      scheduleType: scheduleType.value,
      scheduledAt,
      cronExpr,
      scheduleEnabled,
    })

    emit('created', created.id)
    draftsStore.clearTaskCreateDraft()
  } catch (err: any) {
    errorMessage.value = err.message || '创建任务失败'
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="task-create-container" data-test="task-create-panel">
    <div class="create-card">
      <header class="create-header">
        <div class="header-titles">
          <h2>创建任务</h2>
          <p class="subtitle">配置可手动、定时或周期执行的会话模板</p>
        </div>
        <button type="button" class="btn-close" title="取消" @click="emit('cancel')">✕</button>
      </header>

      <form class="create-form" @submit.prevent="handleCreateTask">
        <!-- 1. Task Type Selection (Heterogeneous Architecture) -->
        <div class="form-section">
          <label class="section-label">任务类型</label>
          <div class="task-type-selector">
            <div
              class="type-card"
              :class="{ active: selectedTaskType === 'standard' }"
              @click="selectedTaskType = 'standard'"
            >
              <div class="type-icon">⚡</div>
              <div class="type-info">
                <span class="type-title">普通任务</span>
                <span class="type-desc">基于会话模板运行，支持手动执行、定时执行与周期性自动化调度。</span>
              </div>
              <span class="type-badge active-badge">主打推荐</span>
            </div>

            <div
              class="type-card disabled-card"
              :class="{ active: selectedTaskType === 'flow' }"
              @click="selectedTaskType = 'flow'"
            >
              <div class="type-icon">🔀</div>
              <div class="type-info">
                <span class="type-title">流程任务</span>
                <span class="type-desc">多阶段工作流与复杂节点编排（DAG 异构引擎扩展插槽）。</span>
              </div>
              <span class="type-badge pending-badge">即将推出</span>
            </div>
          </div>
        </div>

        <!-- 2. Basic Info -->
        <div class="form-section">
          <label class="section-label">基本信息</label>
          <div class="form-group">
            <label for="task-title">任务名称 <span class="required">*</span></label>
            <input
              id="task-title"
              v-model="title"
              type="text"
              class="form-input"
              placeholder="例如：每日生产服务健康巡检与错误汇总"
              required
            />
          </div>
          <div class="form-group">
            <label for="task-desc">任务描述 (可选)</label>
            <input
              id="task-desc"
              v-model="description"
              type="text"
              class="form-input"
              placeholder="简要说明此任务的目的、触发背景或交付物"
            />
          </div>
        </div>

        <!-- 3. Session Template Configuration (Referenced from NewSessionPanel) -->
        <div class="form-section">
          <label class="section-label">会话执行模板</label>

          <!-- Project Selector -->
          <div class="form-group">
            <label for="project-select">关联项目 <span class="required">*</span></label>
            <select
              id="project-select"
              v-model="selectedProjectId"
              class="form-select"
              required
            >
              <option :value="null" disabled>请选择项目…</option>
              <option
                v-for="p in projectsStore.projects ?? []"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }} ({{ p.path }})
              </option>
            </select>
          </div>

          <!-- Agent Backend -->
          <div class="form-group">
            <label>Agent 后端 <span class="required">*</span></label>
            <div class="backend-switch" role="radiogroup">
              <button
                v-for="b in selectableBackends"
                :key="b.backend"
                type="button"
                role="radio"
                class="backend-option"
                :class="{ active: selectedBackend === b.backend }"
                :aria-checked="selectedBackend === b.backend"
                @click="selectBackend(b.backend)"
              >
                <span class="backend-name">{{ b.label || b.backend }}</span>
              </button>
            </div>
          </div>

          <!-- Model Select -->
          <div
            v-if="selectedCapabilities?.modelSelection && selectedBackend !== ''"
            class="form-group"
          >
            <label>运行模型 <span class="required">*</span></label>
            <div class="model-select-wrapper">
              <ModelSelect
                v-model="selectedModel"
                :key="selectedBackend"
                :backend="selectedBackend"
                :model-selection="true"
                @status="modelOk = $event"
              />
            </div>
          </div>

          <!-- Prompt Preset -->
          <div class="form-group">
            <label for="prompt-select">预设提示词模板 (可选)</label>
            <select
              id="prompt-select"
              v-model="selectedPromptId"
              class="form-select"
              @change="onPromptTemplateChange"
            >
              <option value="">不使用预设模板（直接在下方输入）</option>
              <option
                v-for="p in promptsStore.prompts"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }}{{ p.description ? ` (${p.description})` : '' }}
              </option>
            </select>
          </div>

          <!-- Prompt Template Textarea -->
          <div class="form-group">
            <label for="task-prompt">
              执行提示词模板 <span class="required">*</span>
              <span class="label-hint">（每次执行此任务时，Agent 将使用该提示词开启新会话）</span>
            </label>
            <textarea
              id="task-prompt"
              v-model="promptText"
              rows="5"
              class="form-textarea"
              placeholder="详细描述任务要求、执行步骤与产出规范…"
              required
            ></textarea>
          </div>
        </div>

        <!-- 4. Execution Mode Options -->
        <div class="form-section">
          <label class="section-label">执行方式选项</label>
          <div class="schedule-mode-grid">
            <label
              class="mode-card"
              :class="{ selected: scheduleType === 'manual' }"
              @click="scheduleType = 'manual'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="manual"
                class="hidden-radio"
              />
              <div class="mode-header">
                <span class="mode-icon">👆</span>
                <span class="mode-title">手动执行</span>
              </div>
              <p class="mode-desc">仅在任务创建完成后，由人工在任务详情页点击按钮立即触发。</p>
            </label>

            <label
              class="mode-card"
              :class="{ selected: scheduleType === 'once' }"
              @click="scheduleType = 'once'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="once"
                class="hidden-radio"
              />
              <div class="mode-header">
                <span class="mode-icon">⏰</span>
                <span class="mode-title">定时执行 (单次)</span>
              </div>
              <p class="mode-desc">在设定的未来时间点自动启动并执行一次任务会话。</p>
            </label>

            <label
              class="mode-card"
              :class="{ selected: scheduleType === 'cron' }"
              @click="scheduleType = 'cron'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="cron"
                class="hidden-radio"
              />
              <div class="mode-header">
                <span class="mode-icon">🔄</span>
                <span class="mode-title">周期执行 (Cron)</span>
              </div>
              <p class="mode-desc">按照预设周期或 Cron 表达式自动循环执行任务并沉淀历史。</p>
            </label>
          </div>

          <!-- Once config -->
          <div v-if="scheduleType === 'once'" class="schedule-config-box">
            <label for="schedule-datetime">定时触发时间 <span class="required">*</span></label>
            <input
              id="schedule-datetime"
              v-model="scheduledAtDate"
              type="datetime-local"
              class="form-input"
              required
            />
          </div>

          <!-- Cron config -->
          <div v-if="scheduleType === 'cron'" class="schedule-config-box">
            <div class="cron-row">
              <div class="cron-field">
                <label>快速预设周期</label>
                <select v-model="cronShortcut" class="form-select">
                  <option value="custom">自定义 Cron 表达式</option>
                  <option value="hourly">每小时执行一次 (0 * * * *)</option>
                  <option value="daily_9am">每天早上 09:00 (0 9 * * *)</option>
                  <option value="weekly_mon">每周一早上 09:00 (0 9 * * 1)</option>
                </select>
              </div>

              <div class="cron-field">
                <label for="cron-expr">Cron 表达式 (分 时 日 月 周)</label>
                <input
                  id="cron-expr"
                  v-model="customCronExpr"
                  type="text"
                  class="form-input code-font"
                  placeholder="0 9 * * *"
                  required
                />
              </div>
            </div>
            <p class="cron-tip">示例：<code>0 9 * * *</code> 表示每天 9:00 执行；<code>*/30 * * * *</code> 表示每 30 分钟执行一次。</p>
          </div>
        </div>

        <p v-if="errorMessage" class="error-banner" role="alert">{{ errorMessage }}</p>

        <!-- Actions -->
        <div class="form-actions">
          <button type="button" class="btn-cancel" @click="emit('cancel')">取消</button>
          <button
            type="submit"
            class="btn-submit"
            :disabled="creating || selectedTaskType === 'flow'"
          >
            {{ creating ? '创建中…' : '创建任务' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.task-create-container {
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding: 2rem 1.5rem;
  overflow-y: auto;
  height: 100%;
  background: var(--color-bg);
}

.create-card {
  width: 100%;
  max-width: 48rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.06);
  padding: 1.75rem 2rem;
}

.create-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--color-border);
}

.header-titles h2 {
  font-size: 1.35rem;
  font-weight: 700;
  color: var(--color-text);
  margin: 0;
}

.subtitle {
  font-size: 0.88rem;
  color: var(--color-text-muted);
  margin: 0.25rem 0 0 0;
}

.btn-close {
  background: none;
  border: none;
  font-size: 1.25rem;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}
.btn-close:hover {
  background: var(--color-surface-muted);
  color: var(--color-text);
}

.form-section {
  margin-bottom: 1.5rem;
}

.section-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--color-primary, #3b82f6);
  margin-bottom: 0.75rem;
}

.task-type-selector {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.type-card {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 1rem;
  border: 1.5px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.type-card:hover {
  border-color: var(--color-primary, #3b82f6);
}

.type-card.active {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.05);
}

.type-card.disabled-card {
  opacity: 0.7;
}

.type-icon {
  font-size: 1.5rem;
  line-height: 1;
}

.type-info {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  flex: 1;
}

.type-title {
  font-weight: 600;
  font-size: 0.95rem;
  color: var(--color-text);
}

.type-desc {
  font-size: 0.8rem;
  color: var(--color-text-muted);
  line-height: 1.35;
}

.type-badge {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  font-size: 0.7rem;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  font-weight: 600;
}

.active-badge {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.pending-badge {
  background: rgba(156, 163, 175, 0.2);
  color: var(--color-text-muted);
}

.form-group {
  margin-bottom: 1rem;
}

.form-group label {
  display: block;
  font-size: 0.88rem;
  font-weight: 500;
  color: var(--color-text);
  margin-bottom: 0.4rem;
}

.label-hint {
  font-size: 0.8rem;
  font-weight: normal;
  color: var(--color-text-muted);
}

.required {
  color: #ef4444;
}

.form-input,
.form-select,
.form-textarea {
  width: 100%;
  padding: 0.55rem 0.75rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 0.9rem;
  transition: border-color 0.15s;
}

.form-input:focus,
.form-select:focus,
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary, #3b82f6);
}

.code-font {
  font-family: monospace;
}

.backend-switch {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.backend-option {
  padding: 0.45rem 0.9rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface-muted);
  color: var(--color-text);
  cursor: pointer;
  font-size: 0.88rem;
  transition: all 0.15s;
}

.backend-option.active {
  border-color: var(--color-primary, #3b82f6);
  background: var(--color-primary, #3b82f6);
  color: #fff;
  font-weight: 600;
}

.model-select-wrapper {
  max-width: 20rem;
}

.schedule-mode-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0.75rem;
  margin-bottom: 0.75rem;
}

.mode-card {
  position: relative;
  display: flex;
  flex-direction: column;
  padding: 0.85rem;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface-muted);
  cursor: pointer;
  transition: all 0.15s;
}

.mode-card:hover {
  border-color: var(--color-primary, #3b82f6);
}

.mode-card.selected {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.05);
}

.hidden-radio {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.mode-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.35rem;
}

.mode-icon {
  font-size: 1.15rem;
}

.mode-title {
  font-weight: 600;
  font-size: 0.9rem;
  color: var(--color-text);
}

.mode-desc {
  font-size: 0.78rem;
  color: var(--color-text-muted);
  line-height: 1.3;
  margin: 0;
}

.schedule-config-box {
  margin-top: 0.75rem;
  padding: 1rem;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: 8px;
}

.cron-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.cron-tip {
  font-size: 0.8rem;
  color: var(--color-text-muted);
  margin: 0.5rem 0 0 0;
}

.error-banner {
  padding: 0.65rem 0.9rem;
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border-radius: 6px;
  font-size: 0.88rem;
  margin-bottom: 1rem;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid var(--color-border);
}

.btn-cancel {
  padding: 0.55rem 1.25rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font-size: 0.9rem;
}

.btn-submit {
  padding: 0.55rem 1.5rem;
  border: none;
  border-radius: 6px;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.9rem;
  transition: opacity 0.15s;
}

.btn-submit:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
