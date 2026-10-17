<script setup lang="ts">
import { computed, ref } from 'vue'
import { describeApiError } from '../../api/client'
import type { ScheduleType, Task } from '../../api/tasks'
import { useTasksStore } from '../../stores/tasks'
import AppDialog from '../../shared/components/AppDialog.vue'

const props = defineProps<{
  task: Task
}>()

const emit = defineEmits<{
  close: []
  updated: [task: Task]
}>()

const tasksStore = useTasksStore()

const isTransactional = computed(() => props.task.flowType === 'transactional')
const isFinalStep = computed(() => {
  if (isTransactional.value) {
    return props.task.currentStep === 2
  }
  return props.task.currentStep === 4
})

// Premise: must be in the final step AND confirmed
const canSchedule = computed(() => isFinalStep.value && props.task.isConfirmed)

const scheduleEnabled = ref(props.task.scheduleEnabled)
const scheduleType = ref<ScheduleType>(
  props.task.scheduleType !== 'none'
    ? props.task.scheduleType
    : isTransactional.value
      ? 'cron'
      : 'once',
)

// ScheduledAt string formatted for datetime-local
function formatDateForInput(isoString?: string): string {
  if (!isoString) {
    const d = new Date(Date.now() + 3600 * 1000)
    d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
    return d.toISOString().slice(0, 16)
  }
  const d = new Date(isoString)
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
  return d.toISOString().slice(0, 16)
}

const scheduledAtInput = ref(formatDateForInput(props.task.scheduledAt))
const cronExprInput = ref(props.task.cronExpr || '0 2 * * *')
const saving = ref(false)
const errorMessage = ref('')

const cronPresets = [
  { label: '每天凌晨 02:00', expr: '0 2 * * *' },
  { label: '工作日早 09:00', expr: '0 9 * * 1-5' },
  { label: '每周一凌晨 03:00', expr: '0 3 * * 1' },
  { label: '每小时整点', expr: '0 * * * *' },
]

function applyPreset(expr: string) {
  cronExprInput.value = expr
}

async function submit() {
  if (!canSchedule.value && scheduleEnabled.value) {
    errorMessage.value = '任务未完成前置确认并推进至最后一步，无法开启调度'
    return
  }

  saving.value = true
  errorMessage.value = ''

  try {
    let targetTime: string | undefined = undefined
    if (scheduleType.value === 'once' && scheduledAtInput.value) {
      targetTime = new Date(scheduledAtInput.value).toISOString()
    }

    const updated = await tasksStore.updateSchedule(props.task.id, {
      scheduleType: scheduleEnabled.value ? scheduleType.value : 'none',
      scheduleEnabled: scheduleEnabled.value,
      scheduledAt: targetTime,
      cronExpr: scheduleType.value === 'cron' ? cronExprInput.value.trim() : undefined,
    })
    emit('updated', updated)
    emit('close')
  } catch (err) {
    errorMessage.value = describeApiError(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <AppDialog title="任务调度配置" @close="emit('close')">
    <form class="dialog-form" @submit.prevent="submit">
      <p v-if="errorMessage" class="dialog-error" role="alert">{{ errorMessage }}</p>

      <!-- Guard warning if not ready -->
      <div v-if="!canSchedule" class="guard-notice">
        <div class="notice-icon">⚠️</div>
        <div class="notice-content">
          <span class="notice-title">未满足自动调度条件</span>
          <p class="notice-desc">
            为了确保无人值守执行时不会因向用户提问而被打断，任务必须已经推进至<strong>最后一步</strong>（{{
              isTransactional ? '任务执行' : '执行'
            }}）且前期<strong>所有事项均已确认就绪</strong>方可启用定时或周期调度。
          </p>
        </div>
      </div>

      <div class="toggle-row">
        <div>
          <span class="field-label">启用自动调度</span>
          <p class="hint">到达设定时刻后，由平台后台自动启动 Agent 会话执行</p>
        </div>
        <input
          type="checkbox"
          v-model="scheduleEnabled"
          :disabled="!canSchedule"
          class="switch-input"
        />
      </div>

      <template v-if="scheduleEnabled">
        <span class="field-label">调度模式</span>
        <div class="type-options">
          <label class="type-card" :class="{ selected: scheduleType === 'once' }">
            <input type="radio" v-model="scheduleType" value="once" name="scheduleType" />
            <span class="type-info">
              <span class="type-name">定时执行（一次性）</span>
              <span class="type-desc">在指定时间点自动触发一次执行</span>
            </span>
          </label>

          <label
            v-if="isTransactional"
            class="type-card"
            :class="{ selected: scheduleType === 'cron' }"
          >
            <input type="radio" v-model="scheduleType" value="cron" name="scheduleType" />
            <span class="type-info">
              <span class="type-name">周期执行（Cron）</span>
              <span class="type-desc">按照周期时间规律反复自动触发</span>
            </span>
          </label>
        </div>
        <p v-if="!isTransactional" class="hint dev-hint">
          * 开发类任务不支持周期执行，仅支持定时执行一次。
        </p>

        <!-- Once mode: datetime picker -->
        <template v-if="scheduleType === 'once'">
          <label for="schedule-at">设定执行时间 <span class="required">*</span></label>
          <input id="schedule-at" v-model="scheduledAtInput" type="datetime-local" required />
        </template>

        <!-- Cron mode: cron expr and presets -->
        <template v-if="scheduleType === 'cron' && isTransactional">
          <label for="cron-expr">Cron 表达式 <span class="required">*</span></label>
          <input
            id="cron-expr"
            v-model="cronExprInput"
            type="text"
            placeholder="0 2 * * * (分 时 日 月 周)"
            required
          />
          <div class="presets">
            <span class="presets-label">常用预设：</span>
            <button
              v-for="p in cronPresets"
              :key="p.expr"
              type="button"
              class="preset-chip"
              @click="applyPreset(p.expr)"
            >
              {{ p.label }}
            </button>
          </div>
        </template>
      </template>

      <div class="dialog-actions">
        <button type="button" class="cancel" @click="emit('close')">取消</button>
        <button type="submit" class="confirm" :disabled="saving || (!canSchedule && scheduleEnabled)">
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
.dialog-form {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.75);
}

.field-label {
  font-weight: 600;
  display: inline-block;
  margin-top: calc(var(--space-unit) * 0.5);
}

.guard-notice {
  display: flex;
  gap: calc(var(--space-unit) * 2);
  background: var(--color-warning-soft);
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 2);
}

.notice-icon {
  font-size: 1.3rem;
}

.notice-title {
  font-weight: 600;
  color: var(--color-warning);
  font-size: 0.9rem;
  display: block;
  margin-bottom: calc(var(--space-unit) * 0.667);
}

.notice-desc {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text);
  line-height: 1.4;
}

.dialog-error {
  color: var(--color-danger);
  margin: 0;
}

.toggle-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: calc(var(--space-unit) * 2);
  padding: calc(var(--space-unit) * 1.667) 0;
  border-bottom: 1px solid var(--color-border);
}

.switch-input {
  flex: none;
  width: 1.25rem;
  height: 1.25rem;
  cursor: pointer;
}

.switch-input:disabled {
  cursor: not-allowed;
  opacity: 0.4;
}

.hint {
  color: var(--color-text-muted);
  font-size: 0.8rem;
  margin: calc(var(--space-unit) * 0.333) 0 0 0;
}

.dev-hint {
  color: var(--color-warning);
}

.type-options {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1);
}

.type-card {
  display: flex;
  align-items: flex-start;
  gap: calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.333) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  cursor: pointer;
}

.type-card.selected {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.type-card input[type='radio'] {
  margin-top: calc(var(--space-unit) * 0.5);
}

.type-info {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.25);
}

.type-name {
  font-weight: 600;
  font-size: 0.886rem;
}

.type-desc {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.dialog-form input[type='text'],
.dialog-form input[type='datetime-local'] {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
}

.dialog-form .required {
  color: var(--color-danger);
}

.dialog-form label {
  font-weight: 600;
  margin-top: calc(var(--space-unit) * 0.5);
}

.presets {
  display: flex;
  flex-wrap: wrap;
  gap: calc(var(--space-unit) * 0.667);
  align-items: center;
  margin-top: calc(var(--space-unit) * 0.667);
}

.presets-label {
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.preset-chip {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  color: var(--color-text);
  font: inherit;
  font-size: 0.76rem;
  padding: calc(var(--space-unit) * 0.333) calc(var(--space-unit) * 1);
  cursor: pointer;
}

.preset-chip:hover {
  border-color: var(--color-primary);
  color: var(--color-primary-hover);
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: calc(var(--space-unit) * 0.75);
  margin-top: calc(var(--space-unit) * 1.5);
}

.dialog-actions button {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
}

.dialog-actions .confirm {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-on-primary);
}

.dialog-actions .confirm:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}
</style>
