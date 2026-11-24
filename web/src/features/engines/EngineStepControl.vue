<script setup lang="ts">
// EngineStepControl: Unified step-limit control component across engine panels.
// Unslopped per craft guidelines:
// - 4px grid rhythm (h-8 32px controls, gap-2)
// - Standard app typography scale (text-xs / text-sm, 12px leading-relaxed hint)
// - Quiet chrome, zero resting shadows, 1px accent active indicator
// - Tabular figures, quiet status metadata, no colored pill badges
// - Subtle calibrated budget gauge visual
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

function onInputChange(e: Event) {
  const target = e.target as HTMLInputElement
  const raw = Number.parseInt(target.value, 10)
  if (Number.isNaN(raw)) {
    emit('update:modelValue', -1)
  } else if (raw <= 0 && raw !== -1) {
    emit('update:modelValue', -1)
  } else {
    emit('update:modelValue', raw)
  }
}

function selectPreset(val: number) {
  emit('update:modelValue', val)
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

    <!-- Calibrated Budget Gauge (Tactile Moment) -->
    <div class="budget-gauge-track w-full h-1 bg-surface-muted rounded-full overflow-hidden flex items-center">
      <div
        class="budget-gauge-bar h-full transition-all duration-200"
        :class="isUnlimited ? 'bg-muted-foreground/40 w-full' : 'bg-accent'"
        :style="{ width: isUnlimited ? '100%' : `${budgetPercent}%` }"
      ></div>
    </div>

    <div class="flex items-center gap-2 flex-wrap">
      <div class="step-input-wrapper inline-flex items-center gap-1.5">
        <input
          :id="inputId"
          type="number"
          min="-1"
          step="1"
          :value="modelValue"
          class="step-num-input w-24 h-8 px-2.5 text-xs font-mono rounded border border-border bg-background text-foreground focus:outline-none focus:border-accent tabular-nums transition-colors"
          placeholder="步数"
          @input="onInputChange"
        />
        <span class="text-xs text-muted-foreground select-none font-mono">步</span>
      </div>

      <div class="preset-group flex items-center gap-1 flex-wrap" role="group" aria-label="预设步数选择">
        <button
          v-for="p in presets"
          :key="p.value"
          type="button"
          class="preset-btn px-2.5 h-8 text-xs font-mono rounded border transition-colors cursor-pointer select-none tabular-nums"
          :class="
            modelValue === p.value
              ? 'active border-accent bg-accent/10 text-accent font-medium'
              : 'border-border/70 bg-surface/60 text-muted-foreground hover:text-foreground hover:border-border hover:bg-surface'
          "
          :aria-pressed="modelValue === p.value"
          @click="selectPreset(p.value)"
        >
          {{ p.label }}
        </button>
      </div>
    </div>

    <p v-if="hint" class="text-xs text-muted-foreground leading-relaxed">
      {{ hint }}
    </p>
  </div>
</template>

<style scoped>
.step-num-input::-webkit-outer-spin-button,
.step-num-input::-webkit-inner-spin-button {
  margin: 0;
}
.step-num-input {
  -moz-appearance: textfield;
}

.preset-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  white-space: nowrap;
}
</style>
