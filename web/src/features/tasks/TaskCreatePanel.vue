<script setup lang="ts">
// Task Creation Panel: structured with clean session template fields,
// task type selector and execution mode configuration.
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
import { UiButton, UiInput, UiSelect, UiTextarea, UiAlert } from '../../shared/ui'

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
  <div class="task-create-container flex justify-center items-start p-6 md:p-8 overflow-y-auto h-full bg-[var(--color-bg)]" data-test="task-create-panel">
    <div class="create-card w-full max-w-3xl bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-6 md:p-8 space-y-6">
      <!-- Header -->
      <header class="create-header flex items-start justify-between pb-4 border-b border-[var(--color-border)]">
        <div class="header-titles space-y-1">
          <h1 class="text-xl font-semibold text-[var(--color-text)] tracking-tight">新建任务</h1>
          <p class="subtitle text-xs text-[var(--color-text-muted)]">配置自动化会话模板与排期调度策略</p>
        </div>
        <button
          type="button"
          class="btn-close text-[var(--color-text-muted)] hover:text-[var(--color-text)] p-1.5 rounded transition-colors inline-flex items-center justify-center"
          title="取消"
          aria-label="关闭面板"
          @click="emit('cancel')"
        >
          <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
            <line x1="3" y1="3" x2="13" y2="13" />
            <line x1="13" y1="3" x2="3" y2="13" />
          </svg>
        </button>
      </header>

      <form class="create-form space-y-6" @submit.prevent="handleCreateTask">
        <!-- 1. Task Type Selection -->
        <div class="form-section space-y-2">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">任务类型</label>
          <div class="task-type-selector grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div
              class="type-card flex items-start gap-3 p-3.5 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                selectedTaskType === 'standard'
                  ? 'active border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="selectedTaskType = 'standard'"
            >
              <div class="type-icon text-[var(--color-primary)] shrink-0 mt-0.5">
                <svg viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8">
                  <polygon points="9 2 2 10 8 10 7 15 14 7 8 7 9 2" />
                </svg>
              </div>
              <div class="type-info flex-1">
                <div class="flex items-center gap-2">
                  <span class="type-title text-xs font-semibold text-[var(--color-text)]">普通任务</span>
                  <span class="type-pill standard">推荐</span>
                </div>
                <p class="type-desc text-xs text-[var(--color-text-muted)] mt-1 leading-relaxed">基于会话模板运行，支持手动执行、定时执行与周期自动化调度。</p>
              </div>
            </div>

            <div
              class="type-card disabled-card flex items-start gap-3 p-3.5 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors opacity-60"
              :class="[
                selectedTaskType === 'flow'
                  ? 'active border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)]',
              ]"
              @click="selectedTaskType = 'flow'"
            >
              <div class="type-icon text-[var(--color-text-muted)] shrink-0 mt-0.5">
                <svg viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8">
                  <rect x="2" y="2" width="4" height="4" rx="1" />
                  <rect x="10" y="2" width="4" height="4" rx="1" />
                  <rect x="6" y="10" width="4" height="4" rx="1" />
                  <path d="M4 6v2a2 2 0 0 0 2 2h2m4-4v2a2 2 0 0 1-2 2H8" />
                </svg>
              </div>
              <div class="type-info flex-1">
                <div class="flex items-center gap-2">
                  <span class="type-title text-xs font-semibold text-[var(--color-text)]">流程任务</span>
                  <span class="type-pill flow">扩展槽</span>
                </div>
                <p class="type-desc text-xs text-[var(--color-text-muted)] mt-1 leading-relaxed">多阶段工作流与复杂节点编排引擎（后续插件扩展预留）。</p>
              </div>
            </div>
          </div>
        </div>

        <!-- 2. Basic Info -->
        <div class="form-section space-y-3.5">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">基本信息</label>
          <div class="form-group flex flex-col gap-1.5">
            <label for="task-title" class="text-xs font-medium text-[var(--color-text)]">
              任务名称 <span class="text-red-500">*</span>
            </label>
            <UiInput
              id="task-title"
              v-model="title"
              placeholder="例如：每日生产服务健康巡检与错误汇总"
              required
              block
            />
          </div>
          <div class="form-group flex flex-col gap-1.5">
            <label for="task-desc" class="text-xs font-medium text-[var(--color-text)]">
              任务描述 (可选)
            </label>
            <UiInput
              id="task-desc"
              v-model="description"
              placeholder="简要说明此任务的目的、触发背景或交付物"
              block
            />
          </div>
        </div>

        <!-- 3. Session Template Configuration -->
        <div class="form-section space-y-3.5">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">会话执行模板</label>

          <!-- Project Selector -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="project-select" class="text-xs font-medium text-[var(--color-text)]">
              关联项目 <span class="text-red-500">*</span>
            </label>
            <UiSelect
              id="project-select"
              v-model="selectedProjectId"
              required
              block
            >
              <option :value="null" disabled>请选择项目…</option>
              <option
                v-for="p in projectsStore.projects ?? []"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }} ({{ p.path }})
              </option>
            </UiSelect>
          </div>

          <!-- Agent Backend -->
          <div class="form-group flex flex-col gap-1.5">
            <label class="text-xs font-medium text-[var(--color-text)]">
              Agent 后端 <span class="text-red-500">*</span>
            </label>
            <div class="backend-switch flex flex-wrap gap-2" role="radiogroup">
              <button
                v-for="b in selectableBackends"
                :key="b.backend"
                type="button"
                role="radio"
                class="backend-option px-3 py-1.5 rounded-[var(--radius,6px)] border text-xs font-medium transition-colors select-none"
                :class="[
                  selectedBackend === b.backend
                    ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)] text-[var(--color-primary)]'
                    : 'border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-muted)]',
                ]"
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
            class="form-group flex flex-col gap-1.5"
          >
            <label class="text-xs font-medium text-[var(--color-text)]">
              运行模型 <span class="text-red-500">*</span>
            </label>
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
          <div class="form-group flex flex-col gap-1.5">
            <label for="prompt-select" class="text-xs font-medium text-[var(--color-text)]">
              预设提示词模板 (可选)
            </label>
            <UiSelect
              id="prompt-select"
              v-model="selectedPromptId"
              block
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
            </UiSelect>
          </div>

          <!-- Prompt Template Textarea -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="task-prompt" class="text-xs font-medium text-[var(--color-text)]">
              执行提示词模板 <span class="text-red-500">*</span>
              <span class="label-hint text-[var(--color-text-muted)] font-normal">（每次执行此任务时，Agent 将使用该提示词开启新会话）</span>
            </label>
            <UiTextarea
              id="task-prompt"
              v-model="promptText"
              :rows="5"
              placeholder="详细描述任务要求、执行步骤与产出规范…"
              required
            />
          </div>
        </div>

        <!-- 4. Execution Mode Options -->
        <div class="form-section space-y-3.5">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">执行方式选项</label>
          <div class="schedule-mode-grid grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div
              class="mode-card p-3 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                scheduleType === 'manual'
                  ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="scheduleType = 'manual'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="manual"
                class="sr-only"
              />
              <div class="mode-header flex items-center gap-1.5 text-xs font-semibold text-[var(--color-text)] mb-1">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <polygon points="5 3 13 8 5 13 5 3" />
                </svg>
                <span>手动执行</span>
              </div>
              <p class="mode-desc text-[11px] text-[var(--color-text-muted)] leading-relaxed">仅在任务创建完成后，由人工在任务详情页点击按钮立即触发。</p>
            </div>

            <div
              class="mode-card p-3 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                scheduleType === 'once'
                  ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="scheduleType = 'once'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="once"
                class="sr-only"
              />
              <div class="mode-header flex items-center gap-1.5 text-xs font-semibold text-[var(--color-text)] mb-1">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <circle cx="8" cy="8" r="6" />
                  <polyline points="8 4.5 8 8 10.5 9.5" />
                </svg>
                <span>定时执行 (单次)</span>
              </div>
              <p class="mode-desc text-[11px] text-[var(--color-text-muted)] leading-relaxed">在设定的未来时间点自动启动并执行一次任务会话。</p>
            </div>

            <div
              class="mode-card p-3 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                scheduleType === 'cron'
                  ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="scheduleType = 'cron'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="cron"
                class="sr-only"
              />
              <div class="mode-header flex items-center gap-1.5 text-xs font-semibold text-[var(--color-text)] mb-1">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M2.5 8a5.5 5.5 0 0 1 9.4-3.9L13.5 6" />
                  <polyline points="13.5 2.5 13.5 6 10 6" />
                  <path d="M13.5 8a5.5 5.5 0 0 1-9.4 3.9L2.5 10" />
                  <polyline points="2.5 13.5 2.5 10 6 10" />
                </svg>
                <span>周期执行 (Cron)</span>
              </div>
              <p class="mode-desc text-[11px] text-[var(--color-text-muted)] leading-relaxed">按照预设周期或 Cron 表达式自动循环执行任务并沉淀历史。</p>
            </div>
          </div>

          <!-- Once config -->
          <div v-if="scheduleType === 'once'" class="schedule-config-box p-3.5 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] space-y-1.5">
            <label for="schedule-datetime" class="text-xs font-medium text-[var(--color-text)]">
              定时触发时间 <span class="text-red-500">*</span>
            </label>
            <UiInput
              id="schedule-datetime"
              v-model="scheduledAtDate"
              type="datetime-local"
              required
              block
            />
          </div>

          <!-- Cron config -->
          <div v-if="scheduleType === 'cron'" class="schedule-config-box p-3.5 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] space-y-2.5">
            <div class="cron-row grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div class="cron-field flex flex-col gap-1.5">
                <label class="text-xs font-medium text-[var(--color-text)]">快速预设周期</label>
                <UiSelect v-model="cronShortcut" block>
                  <option value="custom">自定义 Cron 表达式</option>
                  <option value="hourly">每小时执行一次 (0 * * * *)</option>
                  <option value="daily_9am">每天早上 09:00 (0 9 * * *)</option>
                  <option value="weekly_mon">每周一早上 09:00 (0 9 * * 1)</option>
                </UiSelect>
              </div>

              <div class="cron-field flex flex-col gap-1.5">
                <label for="cron-expr" class="text-xs font-medium text-[var(--color-text)]">
                  Cron 表达式 (分 时 日 月 周)
                </label>
                <UiInput
                  id="cron-expr"
                  v-model="customCronExpr"
                  class="font-mono text-xs"
                  placeholder="0 9 * * *"
                  required
                  block
                />
              </div>
            </div>
            <p class="cron-tip text-[11px] text-[var(--color-text-muted)]">
              示例：<code>0 9 * * *</code> 表示每天 9:00 执行；<code>*/30 * * * *</code> 表示每 30 分钟执行一次。
            </p>
          </div>
        </div>

        <UiAlert v-if="errorMessage" variant="error" class="error-banner">
          {{ errorMessage }}
        </UiAlert>

        <!-- Actions -->
        <div class="form-actions flex items-center justify-end gap-3 pt-4 border-t border-[var(--color-border)]">
          <UiButton type="button" variant="secondary" class="btn-cancel text-xs" @click="emit('cancel')">取消</UiButton>
          <UiButton
            type="submit"
            variant="primary"
            class="btn-submit text-xs font-medium px-4 py-2"
            :disabled="creating || selectedTaskType === 'flow'"
            :loading="creating"
          >
            {{ creating ? '创建中…' : '创建任务' }}
          </UiButton>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
@import './tasks-view.css';
</style>
