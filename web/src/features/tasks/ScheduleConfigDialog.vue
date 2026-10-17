<script setup lang="ts">
import { computed, ref } from 'vue'
import { describeApiError } from '../../api/client'
import type { ScheduleType, Task } from '../../api/tasks'
import { useTasksStore } from '../../stores/tasks'

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
  <div class="dialog-backdrop" @click.self="emit('close')">
    <div class="schedule-dialog" role="dialog" aria-label="调度配置">
      <div class="dialog-header">
        <h2>任务调度配置</h2>
        <button type="button" class="close-btn" @click="emit('close')" aria-label="关闭">×</button>
      </div>

      <form @submit.prevent="submit" class="dialog-body">
        <p v-if="errorMessage" class="error-banner" role="alert">{{ errorMessage }}</p>

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

        <div class="form-group toggle-row">
          <div>
            <label class="toggle-label">启用自动调度</label>
            <p class="field-hint">到达设定时刻后，由平台后台自动启动 Agent 会话执行</p>
          </div>
          <input
            type="checkbox"
            v-model="scheduleEnabled"
            :disabled="!canSchedule"
            class="switch-input"
          />
        </div>

        <template v-if="scheduleEnabled">
          <div class="form-group">
            <label>调度模式</label>
            <div class="type-options">
              <label class="type-card" :class="{ selected: scheduleType === 'once' }">
                <input type="radio" v-model="scheduleType" value="once" name="scheduleType" />
                <div class="type-info">
                  <span class="type-name">定时执行（一次性）</span>
                  <span class="type-desc">在指定时间点自动触发一次执行</span>
                </div>
              </label>

              <label
                v-if="isTransactional"
                class="type-card"
                :class="{ selected: scheduleType === 'cron' }"
              >
                <input type="radio" v-model="scheduleType" value="cron" name="scheduleType" />
                <div class="type-info">
                  <span class="type-name">周期执行（Cron）</span>
                  <span class="type-desc">按照周期时间规律反复自动触发</span>
                </div>
              </label>
            </div>
            <p v-if="!isTransactional" class="field-hint dev-hint">
              * 开发类任务不支持周期执行，仅支持定时执行一次。
            </p>
          </div>

          <!-- Once mode: datetime picker -->
          <div v-if="scheduleType === 'once'" class="form-group">
            <label for="schedule-at">设定执行时间 <span class="required">*</span></label>
            <input
              id="schedule-at"
              v-model="scheduledAtInput"
              type="datetime-local"
              required
            />
          </div>

          <!-- Cron mode: cron expr and presets -->
          <div v-if="scheduleType === 'cron' && isTransactional" class="form-group">
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
          </div>
        </template>

        <div class="dialog-actions">
          <button type="button" class="btn-cancel" @click="emit('close')">取消</button>
          <button type="submit" class="btn-primary" :disabled="saving || (!canSchedule && scheduleEnabled)">
            {{ saving ? '保存中...' : '保存配置' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.dialog-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(2px);
}

.schedule-dialog {
  background: var(--color-bg, #1e1e24);
  border: 1px solid var(--color-border, #333);
  border-radius: 8px;
  width: 500px;
  max-width: 90vw;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.4);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-border, #333);
}

.dialog-header h2 {
  font-size: 1.15rem;
  margin: 0;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 1.5rem;
  line-height: 1;
  color: var(--color-text-muted, #888);
  cursor: pointer;
}

.dialog-body {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.guard-notice {
  display: flex;
  gap: 12px;
  background: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.35);
  border-radius: 6px;
  padding: 12px;
}

.notice-icon {
  font-size: 1.3rem;
}

.notice-title {
  font-weight: 600;
  color: #fbbf24;
  font-size: 0.9rem;
  display: block;
  margin-bottom: 4px;
}

.notice-desc {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text, #e0e0e0);
  line-height: 1.4;
}

.error-banner {
  background: rgba(220, 53, 69, 0.15);
  border: 1px solid rgba(220, 53, 69, 0.4);
  color: #ff6b6b;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 0.9rem;
  margin: 0;
}

.toggle-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid var(--color-border, #333);
}

.toggle-label {
  font-size: 0.95rem;
  font-weight: 600;
}

.switch-input {
  width: 20px;
  height: 20px;
  cursor: pointer;
}

.switch-input:disabled {
  cursor: not-allowed;
  opacity: 0.4;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group label {
  font-size: 0.88rem;
  font-weight: 500;
  color: var(--color-text, #e0e0e0);
}

.field-hint {
  font-size: 0.8rem;
  color: var(--color-text-muted, #888);
  margin: 2px 0 0 0;
}

.dev-hint {
  color: #fbbf24;
}

.type-options {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.type-card {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border, #444);
  border-radius: 6px;
  background: var(--color-card-bg, #25252c);
  cursor: pointer;
}

.type-card.selected {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.08);
}

.type-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.type-name {
  font-weight: 600;
  font-size: 0.88rem;
}

.type-desc {
  font-size: 0.8rem;
  color: var(--color-text-muted, #999);
}

input[type='text'],
input[type='datetime-local'] {
  background: var(--color-input-bg, #2a2a32);
  border: 1px solid var(--color-border, #444);
  color: var(--color-text, #fff);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 0.9rem;
  outline: none;
}

.presets {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin-top: 4px;
}

.presets-label {
  font-size: 0.78rem;
  color: var(--color-text-muted, #888);
}

.preset-chip {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid var(--color-border, #444);
  color: var(--color-text, #ddd);
  font-size: 0.76rem;
  padding: 3px 8px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s;
}

.preset-chip:hover {
  background: rgba(59, 130, 246, 0.2);
  border-color: var(--color-primary, #3b82f6);
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 10px;
}

.btn-cancel {
  background: transparent;
  border: 1px solid var(--color-border, #555);
  color: var(--color-text, #eee);
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 8px 18px;
  border-radius: 6px;
  font-weight: 500;
  cursor: pointer;
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
