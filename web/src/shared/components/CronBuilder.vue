<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { UiSelect, UiInput } from '../ui'
import {
  buildCron,
  calculateNextRuns,
  createDefaultCronStates,
  describeCron,
  FIELD_CONFIGS,
  formatDateTime,
  parseCron,
  type CronFieldStates,
  type FieldState,
  type FieldType,
} from '../utils/cronParser'

const props = withDefaults(
  defineProps<{
    modelValue?: string
    disabled?: boolean
    id?: string
  }>(),
  {
    modelValue: '0 0 9 * * ? *',
    disabled: false,
    id: 'cron-expr',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

export type ActiveTab = 'preset' | 'second' | 'minute' | 'hour' | 'day' | 'month' | 'week' | 'year'
export type PresetMode = 'daily' | 'workday' | 'weekly' | 'monthly' | 'minute' | 'hour' | 'custom'

const activeTab = ref<ActiveTab>('preset')
const presetMode = ref<PresetMode>('daily')

// Minute interval preset
const minuteInterval = ref<number>(15)

// Hour interval preset
const hourInterval = ref<number>(1)
const hourMinute = ref<number>(0)

// Fixed time: hour & minute & second preset
const selectedHour = ref<number>(9)
const selectedMinute = ref<number>(0)
const selectedSecond = ref<number>(0)

// Weekly preset
const selectedWeekdays = ref<number[]>([1])

// Monthly preset
const selectedMonthDays = ref<number[]>([1])

// Full 7-field state
const states = ref<CronFieldStates>(createDefaultCronStates())

// Manual raw expression
const manualExpr = ref<string>(props.modelValue || '0 0 9 * * ? *')
const isManualEditing = ref(false)
const showNextRuns = ref(true)

const currentYear = new Date().getFullYear()
const yearsList = Array.from({ length: 11 }, (_, i) => currentYear + i)

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

// Compute the active cron expression string
const currentCronExpr = computed<string>(() => {
  if (isManualEditing.value || presetMode.value === 'custom') {
    return manualExpr.value.trim()
  }
  return buildCron(states.value)
})

// Human-readable Chinese translation
const humanReadable = computed(() => {
  return describeCron(currentCronExpr.value)
})

// Next 10 run times
const nextRuns = computed(() => {
  try {
    return calculateNextRuns(currentCronExpr.value, 10)
  } catch {
    return []
  }
})

// Sync states from preset selections
function applyPresetToStates() {
  const next = createDefaultCronStates()
  switch (presetMode.value) {
    case 'minute':
      next.second = { type: 'specific', specific: [0] }
      next.minute = { type: 'step', start: 0, step: minuteInterval.value, specific: [] }
      next.hour = { type: 'all', specific: [] }
      next.day = { type: 'all', specific: [] }
      next.month = { type: 'all', specific: [] }
      next.week = { type: 'none', specific: [] }
      next.year = { type: 'all', specific: [] }
      break
    case 'hour':
      next.second = { type: 'specific', specific: [0] }
      next.minute = { type: 'specific', specific: [hourMinute.value] }
      next.hour = { type: 'step', start: 0, step: hourInterval.value, specific: [] }
      next.day = { type: 'all', specific: [] }
      next.month = { type: 'all', specific: [] }
      next.week = { type: 'none', specific: [] }
      next.year = { type: 'all', specific: [] }
      break
    case 'daily':
      next.second = { type: 'specific', specific: [selectedSecond.value] }
      next.minute = { type: 'specific', specific: [selectedMinute.value] }
      next.hour = { type: 'specific', specific: [selectedHour.value] }
      next.day = { type: 'all', specific: [] }
      next.month = { type: 'all', specific: [] }
      next.week = { type: 'none', specific: [] }
      next.year = { type: 'all', specific: [] }
      break
    case 'workday':
      next.second = { type: 'specific', specific: [selectedSecond.value] }
      next.minute = { type: 'specific', specific: [selectedMinute.value] }
      next.hour = { type: 'specific', specific: [selectedHour.value] }
      next.day = { type: 'all', specific: [] }
      next.month = { type: 'all', specific: [] }
      next.week = { type: 'range', start: 1, end: 5, specific: [] }
      next.year = { type: 'all', specific: [] }
      break
    case 'weekly': {
      const days = [...selectedWeekdays.value].sort((a, b) => a - b)
      next.second = { type: 'specific', specific: [selectedSecond.value] }
      next.minute = { type: 'specific', specific: [selectedMinute.value] }
      next.hour = { type: 'specific', specific: [selectedHour.value] }
      next.day = { type: 'none', specific: [] }
      next.month = { type: 'all', specific: [] }
      next.week = { type: 'specific', specific: days.length > 0 ? days : [1] }
      next.year = { type: 'all', specific: [] }
      break
    }
    case 'monthly': {
      const days = [...selectedMonthDays.value].sort((a, b) => a - b)
      next.second = { type: 'specific', specific: [selectedSecond.value] }
      next.minute = { type: 'specific', specific: [selectedMinute.value] }
      next.hour = { type: 'specific', specific: [selectedHour.value] }
      next.day = { type: 'specific', specific: days.length > 0 ? days : [1] }
      next.month = { type: 'all', specific: [] }
      next.week = { type: 'none', specific: [] }
      next.year = { type: 'all', specific: [] }
      break
    }
    case 'custom':
      break
  }
  states.value = next
}

// Reverse parse incoming expression into states and detect presets
function parseIncoming(expr: string) {
  if (!expr || typeof expr !== 'string') return
  const trimmed = expr.trim()
  const parsed = parseCron(trimmed)
  if (!parsed) {
    presetMode.value = 'custom'
    manualExpr.value = trimmed
    return
  }

  states.value = parsed
  manualExpr.value = trimmed

  // Check if matches known presets
  const { second, minute, hour, day, month, week, year } = parsed

  const isStandardYear = year.type === 'all'
  const isStandardMonth = month.type === 'all'
  const isStandardZeroSecond = second.type === 'specific' && second.specific.length === 1 && second.specific[0] === 0

  if (
    isStandardYear &&
    isStandardMonth &&
    isStandardZeroSecond &&
    minute.type === 'step' &&
    hour.type === 'all' &&
    day.type === 'all'
  ) {
    presetMode.value = 'minute'
    minuteInterval.value = minute.step ?? 15
    return
  }

  if (
    isStandardYear &&
    isStandardMonth &&
    isStandardZeroSecond &&
    minute.type === 'specific' &&
    hour.type === 'step' &&
    day.type === 'all'
  ) {
    presetMode.value = 'hour'
    hourInterval.value = hour.step ?? 1
    hourMinute.value = minute.specific[0] ?? 0
    return
  }

  if (
    isStandardYear &&
    isStandardMonth &&
    isStandardZeroSecond &&
    minute.type === 'specific' &&
    minute.specific.length === 1 &&
    hour.type === 'specific' &&
    hour.specific.length === 1 &&
    day.type === 'all' &&
    week.type === 'range' &&
    week.start === 1 &&
    week.end === 5
  ) {
    presetMode.value = 'workday'
    selectedSecond.value = 0
    selectedMinute.value = minute.specific[0] ?? 0
    selectedHour.value = hour.specific[0] ?? 9
    return
  }

  if (
    isStandardYear &&
    isStandardMonth &&
    isStandardZeroSecond &&
    minute.type === 'specific' &&
    minute.specific.length === 1 &&
    hour.type === 'specific' &&
    hour.specific.length === 1 &&
    day.type === 'all' &&
    (week.type === 'none' || week.type === 'all')
  ) {
    presetMode.value = 'daily'
    selectedSecond.value = 0
    selectedMinute.value = minute.specific[0] ?? 0
    selectedHour.value = hour.specific[0] ?? 9
    return
  }

  if (
    isStandardYear &&
    isStandardMonth &&
    isStandardZeroSecond &&
    minute.type === 'specific' &&
    hour.type === 'specific' &&
    (day.type === 'none' || day.type === 'all') &&
    week.type === 'specific' &&
    week.specific.length > 0
  ) {
    presetMode.value = 'weekly'
    selectedSecond.value = 0
    selectedMinute.value = minute.specific[0] ?? 0
    selectedHour.value = hour.specific[0] ?? 9
    selectedWeekdays.value = [...week.specific]
    return
  }

  if (
    isStandardYear &&
    isStandardMonth &&
    isStandardZeroSecond &&
    minute.type === 'specific' &&
    hour.type === 'specific' &&
    day.type === 'specific' &&
    day.specific.length > 0 &&
    (week.type === 'none' || week.type === 'all')
  ) {
    presetMode.value = 'monthly'
    selectedSecond.value = 0
    selectedMinute.value = minute.specific[0] ?? 0
    selectedHour.value = hour.specific[0] ?? 9
    selectedMonthDays.value = [...day.specific]
    return
  }

  // Fallback to custom mode if doesn't match standard preset
  presetMode.value = 'custom'
}

function emitUpdate() {
  const expr = currentCronExpr.value
  if (expr !== props.modelValue) {
    emit('update:modelValue', expr)
  }
}

watch(
  () => props.modelValue,
  (val) => {
    if (val && val.trim() !== currentCronExpr.value.trim()) {
      parseIncoming(val)
    }
  },
  { immediate: true },
)

watch(
  [presetMode, minuteInterval, hourInterval, hourMinute, selectedHour, selectedMinute, selectedSecond, selectedWeekdays, selectedMonthDays],
  () => {
    if (presetMode.value !== 'custom') {
      isManualEditing.value = false
      applyPresetToStates()
    }
    emitUpdate()
  },
  { deep: true },
)

watch(
  states,
  () => {
    if (!isManualEditing.value && presetMode.value !== 'custom') {
      manualExpr.value = buildCron(states.value)
    }
    emitUpdate()
  },
  { deep: true },
)

function onManualInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  isManualEditing.value = true
  manualExpr.value = val
  parseIncoming(val)
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

function toggleSpecific(field: FieldState, val: number) {
  isManualEditing.value = false
  if (field.type !== 'specific') {
    field.type = 'specific'
    field.specific = [val]
    return
  }
  const idx = field.specific.indexOf(val)
  if (idx >= 0) {
    if (field.specific.length > 1) {
      field.specific.splice(idx, 1)
    }
  } else {
    field.specific.push(val)
    field.specific.sort((a, b) => a - b)
  }
}

function onPresetModeChange(val: PresetMode) {
  presetMode.value = val
  if (val !== 'custom') {
    isManualEditing.value = false
    applyPresetToStates()
  }
}

// When day specifies specific/range/step, week should be 'none' (?), and vice versa
watch(
  () => states.value.day.type,
  (dayType) => {
    if (activeTab.value === 'day' && dayType !== 'none') {
      states.value.week.type = 'none'
    }
  },
)

watch(
  () => states.value.week.type,
  (weekType) => {
    if (activeTab.value === 'week' && weekType !== 'none') {
      states.value.day.type = 'none'
    }
  },
)
</script>

<template>
  <div class="cron-builder space-y-3 p-3 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs" data-test="cron-builder">
    <!-- Row 1: Mode Select & Raw Expression Input -->
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 items-center">
      <div class="flex flex-col gap-1.5">
        <label class="font-medium text-[var(--color-text)]">快速生成模式</label>
        <UiSelect
          :model-value="presetMode"
          :disabled="disabled"
          block
          data-test="cron-mode-select"
          @update:model-value="(val) => onPresetModeChange(val as PresetMode)"
        >
          <option value="daily">每天固定时间</option>
          <option value="workday">工作日 (周一至周五)</option>
          <option value="weekly">每周指定天</option>
          <option value="monthly">每月固定日期</option>
          <option value="minute">按分钟循环</option>
          <option value="hour">按小时循环</option>
          <option value="custom">高级 / 7 字段自定义 (秒、分、时、日、月、周、年)</option>
        </UiSelect>
      </div>

      <div class="flex flex-col gap-1.5">
        <label :for="id" class="font-medium text-[var(--color-text)] flex items-center justify-between">
          <span>Cron 表达式 (秒 分 时 日 月 周 年)</span>
          <span class="text-[10px] text-[var(--color-text-muted)] font-normal">支持直接粘贴反解析</span>
        </label>
        <UiInput
          :id="id"
          :model-value="currentCronExpr"
          :disabled="disabled"
          class="font-mono text-xs"
          placeholder="0 0 9 * * ? *"
          data-test="cron-input"
          block
          @input="onManualInput"
        />
      </div>
    </div>

    <!-- Navigation Tabs: Fast Preset vs 7 individual Fields -->
    <div class="cron-tabs-header flex items-center gap-1 border-b border-[var(--color-border)] pb-1.5 overflow-x-auto text-[11px]">
      <button
        type="button"
        class="tab-btn px-2.5 py-1 rounded transition-colors font-medium shrink-0"
        :class="activeTab === 'preset' ? 'bg-[var(--color-primary)] text-white' : 'text-[var(--color-text-muted)] hover:bg-[var(--color-surface)]'"
        @click="activeTab = 'preset'"
      >
        常用预设
      </button>
      <span class="text-[var(--color-border)]">|</span>
      <button
        v-for="t in [
          { key: 'second', label: '秒' },
          { key: 'minute', label: '分' },
          { key: 'hour', label: '时' },
          { key: 'day', label: '日' },
          { key: 'month', label: '月' },
          { key: 'week', label: '周' },
          { key: 'year', label: '年' },
        ]"
        :key="t.key"
        type="button"
        class="tab-btn px-2 py-0.5 rounded transition-colors shrink-0"
        :class="activeTab === t.key ? 'bg-[var(--color-primary)] text-white font-medium' : 'text-[var(--color-text)] hover:bg-[var(--color-surface)]'"
        @click="activeTab = t.key as ActiveTab"
      >
        {{ t.label }}
      </button>
    </div>

    <!-- Tab 0: Fast Presets -->
    <div v-if="activeTab === 'preset'" class="cron-controls">
      <!-- 1. Minute mode -->
      <div v-if="presetMode === 'minute'" class="flex items-center gap-2">
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
      <div v-else-if="presetMode === 'hour'" class="flex items-center gap-2 flex-wrap">
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
      <div v-else-if="presetMode === 'daily' || presetMode === 'workday'" class="flex items-center gap-2">
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
      <div v-else-if="presetMode === 'weekly'" class="space-y-2">
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
      <div v-else-if="presetMode === 'monthly'" class="space-y-2">
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

      <!-- 6. Custom helper hint -->
      <div v-else class="text-[11px] text-[var(--color-text-muted)]">
        当前为自定义表达式，可点击上方「秒、分、时、日、月、周、年」标签页直接编辑字段规则，或直接修改表达式。
      </div>
    </div>

    <!-- Tab: Second (0-59) -->
    <div v-else-if="activeTab === 'second'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.second.type" type="radio" value="all" />
          <span>每秒 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.second.type" type="radio" value="step" />
          <span>循环从</span>
          <input v-model.number="states.second.start" type="number" min="0" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="0" />
          <span>秒开始，每隔</span>
          <input v-model.number="states.second.step" type="number" min="1" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="5" />
          <span>秒执行</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.second.type" type="radio" value="range" />
          <span>周期从</span>
          <input v-model.number="states.second.start" type="number" min="0" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="0" />
          <span>到</span>
          <input v-model.number="states.second.end" type="number" min="0" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="30" />
          <span>秒</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.second.type" type="radio" value="specific" />
          <span>指定秒数</span>
        </label>
      </div>

      <div v-if="states.second.type === 'specific'" class="grid grid-cols-10 sm:grid-cols-12 gap-1 pt-1 max-h-36 overflow-y-auto">
        <button
          v-for="s in 60"
          :key="s - 1"
          type="button"
          class="px-1 py-0.5 text-[11px] rounded border text-center transition-colors"
          :class="states.second.specific.includes(s - 1)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.second, s - 1)"
        >
          {{ pad2(s - 1) }}
        </button>
      </div>
    </div>

    <!-- Tab: Minute (0-59) -->
    <div v-else-if="activeTab === 'minute'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.minute.type" type="radio" value="all" />
          <span>每分钟 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.minute.type" type="radio" value="step" />
          <span>循环从</span>
          <input v-model.number="states.minute.start" type="number" min="0" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="0" />
          <span>分开始，每隔</span>
          <input v-model.number="states.minute.step" type="number" min="1" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="5" />
          <span>分执行</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.minute.type" type="radio" value="range" />
          <span>周期从</span>
          <input v-model.number="states.minute.start" type="number" min="0" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="0" />
          <span>到</span>
          <input v-model.number="states.minute.end" type="number" min="0" max="59" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="30" />
          <span>分</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.minute.type" type="radio" value="specific" />
          <span>指定分钟</span>
        </label>
      </div>

      <div v-if="states.minute.type === 'specific'" class="grid grid-cols-10 sm:grid-cols-12 gap-1 pt-1 max-h-36 overflow-y-auto">
        <button
          v-for="m in 60"
          :key="m - 1"
          type="button"
          class="px-1 py-0.5 text-[11px] rounded border text-center transition-colors"
          :class="states.minute.specific.includes(m - 1)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.minute, m - 1)"
        >
          {{ pad2(m - 1) }}
        </button>
      </div>
    </div>

    <!-- Tab: Hour (0-23) -->
    <div v-else-if="activeTab === 'hour'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.hour.type" type="radio" value="all" />
          <span>每小时 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.hour.type" type="radio" value="step" />
          <span>循环从</span>
          <input v-model.number="states.hour.start" type="number" min="0" max="23" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="0" />
          <span>点开始，每隔</span>
          <input v-model.number="states.hour.step" type="number" min="1" max="23" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="2" />
          <span>小时执行</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.hour.type" type="radio" value="range" />
          <span>周期从</span>
          <input v-model.number="states.hour.start" type="number" min="0" max="23" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="9" />
          <span>到</span>
          <input v-model.number="states.hour.end" type="number" min="0" max="23" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="18" />
          <span>点</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.hour.type" type="radio" value="specific" />
          <span>指定小时</span>
        </label>
      </div>

      <div v-if="states.hour.type === 'specific'" class="grid grid-cols-8 sm:grid-cols-12 gap-1 pt-1 max-h-36 overflow-y-auto">
        <button
          v-for="h in 24"
          :key="h - 1"
          type="button"
          class="px-1 py-0.5 text-[11px] rounded border text-center transition-colors"
          :class="states.hour.specific.includes(h - 1)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.hour, h - 1)"
        >
          {{ pad2(h - 1) }}
        </button>
      </div>
    </div>

    <!-- Tab: Day (1-31) -->
    <div v-else-if="activeTab === 'day'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.day.type" type="radio" value="all" />
          <span>每天 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.day.type" type="radio" value="none" />
          <span>不指定 (?)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.day.type" type="radio" value="step" />
          <span>从</span>
          <input v-model.number="states.day.start" type="number" min="1" max="31" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="1" />
          <span>号开始，每隔</span>
          <input v-model.number="states.day.step" type="number" min="1" max="31" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="1" />
          <span>天执行</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.day.type" type="radio" value="specific" />
          <span>指定日期</span>
        </label>
      </div>

      <div v-if="states.day.type === 'specific'" class="grid grid-cols-8 sm:grid-cols-11 gap-1 pt-1 max-h-36 overflow-y-auto">
        <button
          v-for="d in 31"
          :key="d"
          type="button"
          class="px-1 py-0.5 text-[11px] rounded border text-center transition-colors"
          :class="states.day.specific.includes(d)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.day, d)"
        >
          {{ pad2(d) }}
        </button>
      </div>
    </div>

    <!-- Tab: Month (1-12) -->
    <div v-else-if="activeTab === 'month'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.month.type" type="radio" value="all" />
          <span>每月 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.month.type" type="radio" value="step" />
          <span>从</span>
          <input v-model.number="states.month.start" type="number" min="1" max="12" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="1" />
          <span>月开始，每隔</span>
          <input v-model.number="states.month.step" type="number" min="1" max="12" class="w-14 px-1 py-0.5 border rounded bg-[var(--color-surface)]" placeholder="1" />
          <span>个月</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.month.type" type="radio" value="specific" />
          <span>指定月份</span>
        </label>
      </div>

      <div v-if="states.month.type === 'specific'" class="grid grid-cols-6 sm:grid-cols-12 gap-1 pt-1">
        <button
          v-for="m in 12"
          :key="m"
          type="button"
          class="px-1 py-0.5 text-[11px] rounded border text-center transition-colors"
          :class="states.month.specific.includes(m)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.month, m)"
        >
          {{ m }}月
        </button>
      </div>
    </div>

    <!-- Tab: Week (1-7) -->
    <div v-else-if="activeTab === 'week'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.week.type" type="radio" value="all" />
          <span>每周 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.week.type" type="radio" value="none" />
          <span>不指定 (?)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.week.type" type="radio" value="range" />
          <span>周期从</span>
          <select v-model.number="states.week.start" class="px-1 py-0.5 border rounded bg-[var(--color-surface)]">
            <option v-for="d in weekdaysList" :key="d.value" :value="d.value">{{ d.label }}</option>
          </select>
          <span>到</span>
          <select v-model.number="states.week.end" class="px-1 py-0.5 border rounded bg-[var(--color-surface)]">
            <option v-for="d in weekdaysList" :key="d.value" :value="d.value">{{ d.label }}</option>
          </select>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.week.type" type="radio" value="specific" />
          <span>指定星期</span>
        </label>
      </div>

      <div v-if="states.week.type === 'specific'" class="flex items-center gap-1.5 flex-wrap pt-1">
        <button
          v-for="d in weekdaysList"
          :key="d.value"
          type="button"
          class="px-2.5 py-1 text-[11px] rounded border transition-colors"
          :class="states.week.specific.includes(d.value)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.week, d.value)"
        >
          {{ d.label }}
        </button>
      </div>
    </div>

    <!-- Tab: Year (Optional, e.g. 2026-2035) -->
    <div v-else-if="activeTab === 'year'" class="space-y-2">
      <div class="flex items-center gap-4 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.year.type" type="radio" value="all" />
          <span>每年 (*)</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.year.type" type="radio" value="range" />
          <span>周期从</span>
          <input v-model.number="states.year.start" type="number" :min="currentYear" :max="currentYear + 20" class="w-18 px-1 py-0.5 border rounded bg-[var(--color-surface)]" :placeholder="String(currentYear)" />
          <span>到</span>
          <input v-model.number="states.year.end" type="number" :min="currentYear" :max="currentYear + 20" class="w-18 px-1 py-0.5 border rounded bg-[var(--color-surface)]" :placeholder="String(currentYear + 5)" />
          <span>年</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input v-model="states.year.type" type="radio" value="specific" />
          <span>指定年份</span>
        </label>
      </div>

      <div v-if="states.year.type === 'specific'" class="flex items-center gap-1.5 flex-wrap pt-1">
        <button
          v-for="y in yearsList"
          :key="y"
          type="button"
          class="px-2 py-0.5 text-[11px] rounded border transition-colors"
          :class="states.year.specific.includes(y)
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)]'
            : 'bg-[var(--color-surface)] text-[var(--color-text)] border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]'"
          @click="toggleSpecific(states.year, y)"
        >
          {{ y }}
        </button>
      </div>
    </div>

    <!-- Human readable explanation text -->
    <div class="flex items-center justify-between gap-2 pt-2 border-t border-[var(--color-border)]/60">
      <div class="flex items-center gap-1.5 text-[11px] text-[var(--color-text-muted)] min-w-0">
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6" class="shrink-0 text-[var(--color-primary,#2563eb)]" aria-hidden="true">
          <circle cx="8" cy="8" r="6.5" />
          <polyline points="8 4 8 8 10.5 10.5" />
        </svg>
        <span class="font-medium text-[var(--color-text)] truncate" data-test="cron-explanation">
          {{ humanReadable }}
        </span>
      </div>

      <button
        type="button"
        class="text-[11px] text-[var(--color-primary)] hover:underline shrink-0"
        @click="showNextRuns = !showNextRuns"
      >
        {{ showNextRuns ? '收起后 10 次执行时间' : '查看后 10 次执行时间' }}
      </button>
    </div>

    <!-- Next 10 runs forecast panel -->
    <div v-if="showNextRuns" class="cron-next-runs bg-[var(--color-surface)] p-2.5 rounded border border-[var(--color-border)]/80 space-y-1.5" data-test="cron-next-runs">
      <div class="flex items-center justify-between text-[11px] font-medium text-[var(--color-text)]">
        <span>推断后 10 次执行时间：</span>
        <span class="text-[10px] text-[var(--color-text-muted)] font-normal">基于本地系统时区计算</span>
      </div>

      <ol v-if="nextRuns.length > 0" class="list-decimal list-inside grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-0.5 text-[11px] font-mono text-[var(--color-text-muted)]">
        <li v-for="(run, idx) in nextRuns" :key="idx" class="truncate" :data-test="`next-run-${idx}`">
          {{ formatDateTime(run) }}
        </li>
      </ol>
      <p v-else class="text-[11px] text-amber-500 py-1">
        当前表达式在未来 5 年内无法产生执行时刻，请检查字段配置（如已过期的年份或互斥的日/周配置）。
      </p>
    </div>
  </div>
</template>

<style scoped>
.tab-btn {
  user-select: none;
}
</style>
