<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { UiSelect, UiInput } from '../ui'

const props = withDefaults(
  defineProps<{
    modelValue?: string
    disabled?: boolean
    id?: string
  }>(),
  {
    modelValue: '0 9 * * *',
    disabled: false,
    id: 'cron-expr',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

export type CronMode = 'minute' | 'hour' | 'daily' | 'workday' | 'weekly' | 'monthly' | 'custom'

const mode = ref<CronMode>('daily')

// Minute interval mode
const minuteInterval = ref<number>(15)

// Hour interval mode
const hourInterval = ref<number>(1)
const hourMinute = ref<number>(0)

// Fixed time: hour & minute
const selectedHour = ref<number>(9)
const selectedMinute = ref<number>(0)

// Weekly: 1=Mon, 2=Tue, 3=Wed, 4=Thu, 5=Fri, 6=Sat, 7=Sun
const selectedWeekdays = ref<number[]>([1])

// Monthly: days of month 1-31
const selectedMonthDays = ref<number[]>([1])

// Custom expression
const customExpr = ref<string>('0 9 * * *')

const weekdaysList = [
  { value: 1, label: '周一' },
  { value: 2, label: '周二' },
  { value: 3, label: '周三' },
  { value: 4, label: '周四' },
  { value: 5, label: '周五' },
  { value: 6, label: '周六' },
  { value: 7, label: '周日' },
]

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

// Generates 5-part cron expression: "minute hour day month weekday"
function computeCron(): string {
  switch (mode.value) {
    case 'minute':
      return `*/${minuteInterval.value} * * * *`
    case 'hour':
      return `${hourMinute.value} */${hourInterval.value} * * *`
    case 'daily':
      return `${selectedMinute.value} ${selectedHour.value} * * *`
    case 'workday':
      return `${selectedMinute.value} ${selectedHour.value} * * 1-5`
    case 'weekly': {
      const days = [...selectedWeekdays.value].sort((a, b) => a - b)
      const dayStr = days.length > 0 ? days.join(',') : '1'
      return `${selectedMinute.value} ${selectedHour.value} * * ${dayStr}`
    }
    case 'monthly': {
      const days = [...selectedMonthDays.value].sort((a, b) => a - b)
      const dayStr = days.length > 0 ? days.join(',') : '1'
      return `${selectedMinute.value} ${selectedHour.value} ${dayStr} * *`
    }
    case 'custom':
      return customExpr.value.trim()
  }
}

// Parses human-readable Chinese description
const humanReadable = computed(() => {
  const current = (mode.value === 'custom' ? customExpr.value : computeCron()).trim()
  const parts = current.split(/\s+/)
  if (parts.length !== 5) {
    return '格式错误：Cron 表达式必须由 5 个部分组成 (分 时 日 月 周)'
  }

  const [min, hr, day, mon, week] = parts

  if (min.startsWith('*/') && hr === '*' && day === '*' && mon === '*' && week === '*') {
    return `每隔 ${min.slice(2)} 分钟执行一次`
  }
  if (!min.includes('*') && hr.startsWith('*/') && day === '*' && mon === '*' && week === '*') {
    return `每隔 ${hr.slice(2)} 小时的第 ${min} 分钟执行一次`
  }
  if (day === '*' && mon === '*' && week === '*') {
    const h = Number(hr)
    const m = Number(min)
    if (!isNaN(h) && !isNaN(m)) {
      return `每天 ${pad2(h)}:${pad2(m)} 执行一次`
    }
  }
  if (day === '*' && mon === '*' && (week === '1-5' || week === '1,2,3,4,5')) {
    const h = Number(hr)
    const m = Number(min)
    if (!isNaN(h) && !isNaN(m)) {
      return `每周一至周五 (工作日) ${pad2(h)}:${pad2(m)} 执行`
    }
  }
  if (day === '*' && mon === '*' && week !== '*') {
    const dayNames = week.split(',').map((w) => {
      const map: Record<string, string> = {
        '1': '周一', '2': '周二', '3': '周三', '4': '周四', '5': '周五', '6': '周六', '7': '周日', '0': '周日',
      }
      return map[w] || `周${w}`
    })
    const h = Number(hr)
    const m = Number(min)
    if (!isNaN(h) && !isNaN(m)) {
      return `每周 (${dayNames.join('、')}) ${pad2(h)}:${pad2(m)} 执行`
    }
  }
  if (day !== '*' && mon === '*' && week === '*') {
    const h = Number(hr)
    const m = Number(min)
    if (!isNaN(h) && !isNaN(m)) {
      return `每月 ${day} 号 ${pad2(h)}:${pad2(m)} 执行`
    }
  }

  return `按表达式执行: ${current}`
})

// Reverse parse incoming cron expression
function parseIncoming(expr: string) {
  if (!expr || typeof expr !== 'string') return
  const trimmed = expr.trim()
  const parts = trimmed.split(/\s+/)
  if (parts.length !== 5) {
    mode.value = 'custom'
    customExpr.value = trimmed
    return
  }

  const [min, hr, day, mon, week] = parts

  // 1. Minute interval: */15 * * * *
  if (min.startsWith('*/') && hr === '*' && day === '*' && mon === '*' && week === '*') {
    const val = parseInt(min.slice(2), 10)
    if (!isNaN(val)) {
      mode.value = 'minute'
      minuteInterval.value = val
      return
    }
  }

  // 2. Hour interval: 0 */2 * * *
  if (!isNaN(Number(min)) && hr.startsWith('*/') && day === '*' && mon === '*' && week === '*') {
    const val = parseInt(hr.slice(2), 10)
    if (!isNaN(val)) {
      mode.value = 'hour'
      hourInterval.value = val
      hourMinute.value = Number(min)
      return
    }
  }

  // 3. Workday: 0 9 * * 1-5
  if (!isNaN(Number(min)) && !isNaN(Number(hr)) && day === '*' && mon === '*' && (week === '1-5' || week === '1,2,3,4,5')) {
    mode.value = 'workday'
    selectedHour.value = Number(hr)
    selectedMinute.value = Number(min)
    return
  }

  // 4. Daily: 0 9 * * *
  if (!isNaN(Number(min)) && !isNaN(Number(hr)) && day === '*' && mon === '*' && week === '*') {
    mode.value = 'daily'
    selectedHour.value = Number(hr)
    selectedMinute.value = Number(min)
    return
  }

  // 5. Weekly: 0 9 * * 1,3,5
  if (!isNaN(Number(min)) && !isNaN(Number(hr)) && day === '*' && mon === '*' && week !== '*') {
    const days = week.split(',').map((w) => parseInt(w, 10)).filter((n) => !isNaN(n) && n >= 1 && n <= 7)
    if (days.length > 0) {
      mode.value = 'weekly'
      selectedHour.value = Number(hr)
      selectedMinute.value = Number(min)
      selectedWeekdays.value = days
      return
    }
  }

  // 6. Monthly: 0 9 1,15 * *
  if (!isNaN(Number(min)) && !isNaN(Number(hr)) && day !== '*' && mon === '*' && week === '*') {
    const days = day.split(',').map((d) => parseInt(d, 10)).filter((n) => !isNaN(n) && n >= 1 && n <= 31)
    if (days.length > 0) {
      mode.value = 'monthly'
      selectedHour.value = Number(hr)
      selectedMinute.value = Number(min)
      selectedMonthDays.value = days
      return
    }
  }

  // Fallback to custom
  mode.value = 'custom'
  customExpr.value = trimmed
}

// Watch changes to internal state and emit update
function syncOutput() {
  const generated = computeCron()
  if (generated !== props.modelValue) {
    emit('update:modelValue', generated)
  }
}

watch([mode, minuteInterval, hourInterval, hourMinute, selectedHour, selectedMinute, selectedWeekdays, selectedMonthDays], () => {
  if (mode.value !== 'custom') {
    customExpr.value = computeCron()
  }
  syncOutput()
}, { deep: true })

watch(customExpr, (newVal) => {
  if (mode.value === 'custom') {
    syncOutput()
  }
})

watch(
  () => props.modelValue,
  (val) => {
    if (val && val.trim() !== computeCron().trim()) {
      parseIncoming(val)
    }
  },
  { immediate: true },
)

function onManualInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  customExpr.value = val
  if (mode.value !== 'custom') {
    mode.value = 'custom'
  }
  emit('update:modelValue', val)
}

function toggleWeekday(val: number) {
  const idx = selectedWeekdays.value.indexOf(val)
  if (idx >= 0) {
    if (selectedWeekdays.value.length > 1) {
      selectedWeekdays.value.splice(idx, 1)
    }
  } else {
    selectedWeekdays.value.push(val)
  }
}
</script>

<template>
  <div class="cron-builder space-y-3 p-3 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs" data-test="cron-builder">
    <!-- Row 1: Mode Select & Raw Expression Input -->
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 items-center">
      <div class="flex flex-col gap-1.5">
        <label class="font-medium text-[var(--color-text)]">快速生成模式</label>
        <UiSelect v-model="mode" :disabled="disabled" block data-test="cron-mode-select">
          <option value="daily">每天固定时间</option>
          <option value="workday">工作日 (周一至周五)</option>
          <option value="weekly">每周指定天</option>
          <option value="monthly">每月固定日期</option>
          <option value="minute">按分钟循环</option>
          <option value="hour">按小时循环</option>
          <option value="custom">高级 / 自定义表达式</option>
        </UiSelect>
      </div>

      <div class="flex flex-col gap-1.5">
        <label :for="id" class="font-medium text-[var(--color-text)]">
          Cron 表达式 (分 时 日 月 周)
        </label>
        <UiInput
          :id="id"
          :model-value="computeCron()"
          :disabled="disabled"
          class="font-mono text-xs"
          placeholder="0 9 * * *"
          data-test="cron-input"
          block
          @input="onManualInput"
        />
      </div>
    </div>

    <!-- Mode visual controls -->
    <div v-if="mode !== 'custom'" class="cron-controls pt-2 border-t border-[var(--color-border)]/60">
      <!-- 1. Minute mode -->
      <div v-if="mode === 'minute'" class="flex items-center gap-2">
        <span class="text-[var(--color-text-muted)]">每隔</span>
        <UiSelect v-model.number="minuteInterval" :disabled="disabled" class="w-32" data-test="cron-minute-select">
          <option :value="1">1 分钟</option>
          <option :value="5">5 分钟</option>
          <option :value="10">10 分钟</option>
          <option :value="15">15 分钟</option>
          <option :value="30">30 分钟</option>
          <option :value="45">45 分钟</option>
        </UiSelect>
        <span class="text-[var(--color-text-muted)]">执行一次</span>
      </div>

      <!-- 2. Hour mode -->
      <div v-else-if="mode === 'hour'" class="flex items-center gap-2 flex-wrap">
        <span class="text-[var(--color-text-muted)]">每隔</span>
        <UiSelect v-model.number="hourInterval" :disabled="disabled" class="w-28" data-test="cron-hour-select">
          <option :value="1">1 小时</option>
          <option :value="2">2 小时</option>
          <option :value="3">3 小时</option>
          <option :value="4">4 小时</option>
          <option :value="6">6 小时</option>
          <option :value="12">12 小时</option>
        </UiSelect>
        <span class="text-[var(--color-text-muted)]">的第</span>
        <UiSelect v-model.number="hourMinute" :disabled="disabled" class="w-24">
          <option v-for="m in [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]" :key="m" :value="m">{{ pad2(m) }} 分</option>
        </UiSelect>
        <span class="text-[var(--color-text-muted)]">执行</span>
      </div>

      <!-- 3. Daily / Workday mode -->
      <div v-else-if="mode === 'daily' || mode === 'workday'" class="flex items-center gap-2">
        <span class="text-[var(--color-text-muted)]">执行时刻：</span>
        <UiSelect v-model.number="selectedHour" :disabled="disabled" class="w-24" data-test="cron-hour-picker">
          <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ pad2(h - 1) }} 时</option>
        </UiSelect>
        <span class="text-[var(--color-text-muted)]">:</span>
        <UiSelect v-model.number="selectedMinute" :disabled="disabled" class="w-24" data-test="cron-minute-picker">
          <option v-for="m in [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]" :key="m" :value="m">{{ pad2(m) }} 分</option>
        </UiSelect>
      </div>

      <!-- 4. Weekly mode -->
      <div v-else-if="mode === 'weekly'" class="space-y-2">
        <div class="flex items-center gap-1.5 flex-wrap">
          <button
            v-for="d in weekdaysList"
            :key="d.value"
            type="button"
            class="px-2.5 py-1 rounded text-xs transition-colors border"
            :class="selectedWeekdays.includes(d.value)
              ? 'bg-[var(--color-primary,#2563eb)] text-white border-[var(--color-primary,#2563eb)]'
              : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
            :disabled="disabled"
            @click="toggleWeekday(d.value)"
          >
            {{ d.label }}
          </button>
        </div>
        <div class="flex items-center gap-2 pt-1">
          <span class="text-[var(--color-text-muted)]">时刻：</span>
          <UiSelect v-model.number="selectedHour" :disabled="disabled" class="w-24">
            <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ pad2(h - 1) }} 时</option>
          </UiSelect>
          <span class="text-[var(--color-text-muted)]">:</span>
          <UiSelect v-model.number="selectedMinute" :disabled="disabled" class="w-24">
            <option v-for="m in [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]" :key="m" :value="m">{{ pad2(m) }} 分</option>
          </UiSelect>
        </div>
      </div>

      <!-- 5. Monthly mode -->
      <div v-else-if="mode === 'monthly'" class="space-y-2">
        <div class="flex items-center gap-2">
          <span class="text-[var(--color-text-muted)]">每月：</span>
          <UiSelect v-model.number="selectedMonthDays[0]" :disabled="disabled" class="w-28">
            <option v-for="d in 31" :key="d" :value="d">{{ d }} 号</option>
          </UiSelect>
          <span class="text-[var(--color-text-muted)]">时刻：</span>
          <UiSelect v-model.number="selectedHour" :disabled="disabled" class="w-24">
            <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ pad2(h - 1) }} 时</option>
          </UiSelect>
          <span class="text-[var(--color-text-muted)]">:</span>
          <UiSelect v-model.number="selectedMinute" :disabled="disabled" class="w-24">
            <option v-for="m in [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]" :key="m" :value="m">{{ pad2(m) }} 分</option>
          </UiSelect>
        </div>
      </div>
    </div>

    <!-- Human readable explanation text -->
    <div class="flex items-center gap-1.5 text-[11px] text-[var(--color-text-muted)] pt-1">
      <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6" class="shrink-0 text-[var(--color-primary,#2563eb)]" aria-hidden="true">
        <circle cx="8" cy="8" r="6.5" />
        <polyline points="8 4 8 8 10.5 10.5" />
      </svg>
      <span class="font-medium text-[var(--color-text)]" data-test="cron-explanation">
        {{ humanReadable }}
      </span>
    </div>
  </div>
</template>
