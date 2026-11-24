<script setup lang="ts">
// EngineStepControl: Unified step-limit control component across engine panels.
// Adheres strictly to design-taste-frontend (Data Terminal / App Screens) rules:
// - Flat surfaces, zero resting shadows, 1px accent active indicators
// - Tabular figures for numerical fidelity
// - Immediate numerical input + responsive quick-preset buttons (including unlimited: -1)
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
  <div class="engine-step-control space-y-2.5">
    <div class="flex items-center justify-between gap-2">
      <label :for="inputId" class="text-xs font-medium text-foreground select-none">
        {{ label }}
      </label>
      <span class="text-2xs font-mono text-muted-foreground tabular-nums flex items-center gap-1.5">
        当前设定:
        <span
          class="font-semibold px-1.5 py-0.5 rounded text-2xs"
          :class="isUnlimited ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20' : 'bg-surface-muted text-foreground border border-border'"
        >
          {{ isUnlimited ? '不限制 (-1)' : `${modelValue} 步` }}
        </span>
      </span>
    </div>

    <div class="flex items-center gap-2.5 flex-wrap">
      <div class="step-input-wrapper inline-flex items-center gap-1.5">
        <input
          :id="inputId"
          type="number"
          min="-1"
          step="1"
          :value="modelValue"
          class="step-num-input w-24 h-7.5 px-2.5 text-xs font-mono rounded border border-border bg-background text-foreground focus:outline-none focus:border-accent tabular-nums transition-colors"
          placeholder="步数"
          @input="onInputChange"
        />
        <span class="text-2xs text-muted-foreground select-none">步</span>
      </div>

      <div class="preset-group flex items-center gap-1 flex-wrap" role="group" aria-label="预设步数选择">
        <button
          v-for="p in presets"
          :key="p.value"
          type="button"
          class="preset-btn px-2.5 h-7.5 text-xs rounded border transition-colors cursor-pointer select-none tabular-nums"
          :class="
            modelValue === p.value
              ? 'active border-accent bg-accent/10 text-accent font-semibold shadow-none'
              : 'border-border/80 bg-surface/80 text-muted-foreground hover:text-foreground hover:border-border-strong hover:bg-surface'
          "
          :aria-pressed="modelValue === p.value"
          @click="selectPreset(p.value)"
        >
          {{ p.label }}
        </button>
      </div>
    </div>

    <p v-if="hint" class="text-2xs text-muted-foreground leading-normal">
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

.preset-btn.active {
  position: relative;
}
</style>
