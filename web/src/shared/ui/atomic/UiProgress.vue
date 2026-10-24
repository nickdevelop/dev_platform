<script setup lang="ts">
import { computed } from 'vue'
import UiTooltip from './UiTooltip.vue'

const props = withDefaults(
  defineProps<{
    value: number
    max?: number
    variant?: 'primary' | 'success' | 'warning' | 'danger' | 'auto'
    size?: 'sm' | 'md' | 'lg'
    showText?: boolean
    text?: string
    tooltip?: string
    tooltipPlacement?: 'top' | 'bottom' | 'left' | 'right'
  }>(),
  {
    max: 100,
    variant: 'auto',
    size: 'md',
    showText: false,
    text: '',
    tooltip: '',
    tooltipPlacement: 'top',
  },
)

const percentage = computed(() => {
  if (props.max <= 0) return 0
  const pct = Math.round((props.value / props.max) * 100)
  return Math.max(0, Math.min(100, pct))
})

const resolvedVariant = computed(() => {
  if (props.variant !== 'auto') return props.variant
  if (percentage.value >= 90) return 'danger'
  if (percentage.value >= 75) return 'warning'
  return 'primary'
})

const barColorClass = computed(() => {
  switch (resolvedVariant.value) {
    case 'danger':
      return 'bg-red-500'
    case 'warning':
      return 'bg-amber-500'
    case 'success':
      return 'bg-emerald-500'
    case 'primary':
    default:
      return 'bg-blue-600'
  }
})

const heightClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-1.5'
    case 'lg':
      return 'h-3'
    case 'md':
    default:
      return 'h-2'
  }
})

const displayText = computed(() => {
  if (props.text) return props.text
  return `${percentage.value}%`
})
</script>

<template>
  <UiTooltip
    :content="tooltip"
    :placement="tooltipPlacement"
    :disabled="!tooltip && !$slots.tooltip"
  >
    <div class="inline-flex items-center gap-2 w-full select-none">
      <div
        class="w-full bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden shrink"
        :class="heightClass"
      >
        <div
          class="h-full rounded-full transition-all duration-300 ease-out"
          :class="barColorClass"
          :style="{ width: `${percentage}%` }"
          role="progressbar"
          :aria-valuenow="value"
          :aria-valuemin="0"
          :aria-valuemax="max"
        />
      </div>

      <span
        v-if="showText"
        class="text-xs font-mono text-slate-500 dark:text-slate-400 shrink-0"
      >
        {{ displayText }}
      </span>
    </div>

    <template v-if="$slots.tooltip" #content>
      <slot name="tooltip" />
    </template>
  </UiTooltip>
</template>
