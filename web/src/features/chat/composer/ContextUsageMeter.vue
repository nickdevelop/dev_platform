<script setup lang="ts">
import { computed } from 'vue'
import { formatTokens } from '../../../shared/format'
import type { ContextUsage } from '../../../shared/contextUsage'

const props = defineProps<{
  usage?: ContextUsage
  contextDetail?: string
}>()

const visible = computed(() => {
  if (!props.usage) return false
  return (props.usage.used > 0 || props.usage.limit !== undefined) && props.usage.percent !== undefined
})

const percentage = computed(() => {
  const p = props.usage?.percent ?? 0
  return Math.max(0, Math.min(100, p))
})

const isWarning = computed(() => percentage.value >= 80)
const isDanger = computed(() => percentage.value >= 95)

// SVG 240-degree arc parameters (from 150° to 390°)
const size = 16
const strokeWidth = 2
const radius = (size - strokeWidth) / 2
const cx = size / 2
const cy = size / 2

const startAngle = 150
const endAngle = 390
const arcDegrees = endAngle - startAngle
const circumference = radius * ((arcDegrees * Math.PI) / 180)

const startRad = (startAngle * Math.PI) / 180
const endRad = (endAngle * Math.PI) / 180
const x1 = cx + radius * Math.cos(startRad)
const y1 = cy + radius * Math.sin(startRad)
const x2 = cx + radius * Math.cos(endRad)
const y2 = cy + radius * Math.sin(endRad)

const pathData = `M ${x1} ${y1} A ${radius} ${radius} 0 1 1 ${x2} ${y2}`

const strokeDashoffset = computed(() => {
  const fillLength = (percentage.value / 100) * circumference
  return Math.max(0, circumference - fillLength)
})

const tooltipText = computed(() => {
  if (!props.usage) return ''
  const u = props.usage
  const usedText = `${u.estimated ? '~' : ''}${formatTokens(u.used)}`
  const limitText = u.limit !== undefined ? formatTokens(u.limit) : ''
  if (limitText) {
    return `${usedText} / ${limitText} ${percentage.value}%`
  }
  return `${usedText} ${percentage.value}%`
})
</script>

<template>
  <div
    v-if="visible"
    class="context-meter inline-flex items-center justify-center w-[28px] h-[28px] rounded-[6px] border transition-colors select-none text-xs leading-none cursor-default"
    :class="[
      isDanger
        ? 'danger bg-[var(--color-danger-soft,#fee2e2)] border-[var(--color-danger-border,#fca5a5)] text-[var(--color-danger,#b91c1c)]'
        : isWarning
        ? 'warning bg-[var(--color-warning-soft,#fffbeb)] border-[var(--color-warning-border,#fcd34d)] text-[var(--color-warning,#92400e)]'
        : 'bg-platform-surface-muted border-platform-border text-platform-text-muted',
    ]"
    role="progressbar"
    :aria-valuenow="percentage"
    aria-valuemin="0"
    aria-valuemax="100"
    :title="tooltipText"
    data-test="context-usage"
  >
    <div class="meter-gauge shrink-0" aria-hidden="true">
      <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
        <path
          class="meter-bg stroke-[var(--color-border)]"
          :d="pathData"
          fill="none"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
        />
        <path
          class="meter-fill transition-all duration-300 ease-out"
          :class="[
            isDanger
              ? 'stroke-[var(--color-danger)]'
              : isWarning
              ? 'stroke-[var(--color-warning)]'
              : 'stroke-[var(--color-primary)]',
          ]"
          :d="pathData"
          fill="none"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
          :stroke-dasharray="circumference"
          :stroke-dashoffset="strokeDashoffset"
        />
      </svg>
    </div>
  </div>
</template>

<style scoped>
.context-meter {
  box-sizing: border-box;
  width: 28px;
  height: 28px;
}
</style>
