<script setup lang="ts">
// EngineStepControl: Unified step-limit control component across engine panels.
// Element Plus migration: the number field is `el-input-number` (controls off,
// clamped at -1) and the preset chips are an `el-segmented` — the library owns
// the selection affordance now. The budget gauge stays: it is our own display
// surface, not a control, and it only paints platform tokens.
// -1 keeps its "unlimited" meaning: a cleared field or any non-positive value
// falls back to -1, exactly like the hand-rolled input it replaced.
import { computed } from 'vue'

export interface StepPreset {
  value: number
  label: string
}

const props = withDefaults(
  defineProps<{
    modelValue: number
    label: string
    hint?: string
    inputId?: string
    presets?: StepPreset[]
  }>(),
  {
    inputId: 'engine-step-input',
    hint: '',
    presets: () => [
      { value: 5, label: '5步' },
      { value: 10, label: '10步' },
      { value: 20, label: '20步' },
      { value: 30, label: '30步' },
      { value: -1, label: '不限制' },
    ],
  },
)

const emit = defineEmits<{
  'update:modelValue': [val: number]
}>()

const isUnlimited = computed(() => props.modelValue === -1 || props.modelValue < 0)

// Visual indicator percentage: capped at 50 steps for progress visualization
const budgetPercent = computed(() => {
  if (isUnlimited.value) return 100
  return Math.min(100, Math.max(4, (props.modelValue / 50) * 100))
})

// el-input-number binds `number | null`; the panel contract is a plain number
// where anything non-positive (or a cleared field) means "unlimited" (-1).
const inputValue = computed<number | null>({
  get: () => props.modelValue,
  set: (value) => {
    if (value === null || Number.isNaN(value) || value <= 0) {
      emit('update:modelValue', -1)
    } else {
      emit('update:modelValue', Math.trunc(value))
    }
  },
})

// The segmented preset only lights up when the current value actually is one
// of the presets; a custom number (e.g. 42) leaves the selection empty rather
// than lying about which chip is active.
const presetSelection = computed(() =>
  props.presets.some((preset) => preset.value === props.modelValue)
    ? props.modelValue
    : undefined,
)

function onPresetChange(value: number | string | boolean) {
  emit('update:modelValue', Number(value))
}
</script>

<template>
  <div class="engine-step-control space-y-3">
    <div class="flex items-center justify-between gap-2">
      <label :for="inputId" class="text-xs font-medium text-foreground select-none">
        {{ label }}
      </label>
      <span class="text-xs font-mono text-muted-foreground tabular-nums flex items-center gap-1.5">
        当前设定:
        <span
          class="font-mono font-medium px-2 py-0.5 rounded text-xs tabular-nums border bg-surface-muted text-foreground border-border"
        >
          {{ isUnlimited ? '不限制 (-1)' : `${modelValue} 步` }}
        </span>
      </span>
    </div>

    <!-- Calibrated Budget Gauge (display-only, not a control) -->
    <div class="budget-gauge-track w-full h-1 bg-surface-muted rounded-full overflow-hidden flex items-center">
      <div
        class="budget-gauge-bar h-full transition-all duration-200"
        :class="isUnlimited ? 'bg-muted-foreground/40 w-full' : 'bg-accent'"
        :style="{ width: isUnlimited ? '100%' : `${budgetPercent}%` }"
      ></div>
    </div>

    <div class="flex items-center gap-2 flex-wrap">
      <div class="step-input-wrapper inline-flex items-center gap-1.5">
        <el-input-number
          :id="inputId"
          v-model="inputValue"
          :min="-1"
          :step="1"
          :controls="false"
          class="step-num-input w-24"
          placeholder="步数"
          data-test="engine-step-number"
        />
        <span class="text-xs text-muted-foreground select-none font-mono">步</span>
      </div>

      <el-segmented
        :model-value="presetSelection"
        :options="presets"
        class="preset-group"
        role="group"
        aria-label="预设步数选择"
        data-test="engine-step-presets"
        @change="onPresetChange"
      />
    </div>

    <p v-if="hint" class="text-xs text-muted-foreground leading-relaxed">
      {{ hint }}
    </p>
  </div>
</template>

<style scoped>
.step-num-input :deep(input) {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
