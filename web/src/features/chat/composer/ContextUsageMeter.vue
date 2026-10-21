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
  const limitText = u.limit !== undefined ? ` / ${formatTokens(u.limit)}` : ''
  let text = `${percentage.value}% · ${usedText}${limitText}`
  if (props.contextDetail) {
    text += ` (${props.contextDetail})`
  }
  if (percentage.value >= 80) {
    text += ' (接近上下文上限，可新建会话或输入 /compact 释放空间)'
  }
  return text
})
</script>

<template>
  <div
    v-if="visible"
    class="context-meter"
    :class="{ warning: isWarning, danger: isDanger }"
    role="progressbar"
    :aria-valuenow="percentage"
    aria-valuemin="0"
    aria-valuemax="100"
    :title="tooltipText"
    data-test="context-usage"
  >
    <div class="meter-gauge" aria-hidden="true">
      <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
        <path
          class="meter-bg"
          :d="pathData"
          fill="none"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
        />
        <path
          class="meter-fill"
          :d="pathData"
          fill="none"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
          :stroke-dasharray="circumference"
          :stroke-dashoffset="strokeDashoffset"
        />
      </svg>
    </div>
    <span class="meter-percent">{{ percentage }}%</span>
  </div>
</template>

<style scoped>
.context-meter {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  height: 28px;
  box-sizing: border-box;
  padding: 0 0.5rem;
  border-radius: 6px;
  background: var(--color-surface-muted, #f1f5f9);
  border: 1px solid var(--color-border, #e2e8f0);
  cursor: default;
  user-select: none;
  font-size: 0.76rem;
  line-height: 1;
  color: var(--color-text-muted, #64748b);
  transition: all 0.15s ease;
}

.meter-gauge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.meter-bg {
  stroke: var(--color-border-strong, #cbd5e1);
}

.meter-fill {
  stroke: var(--color-primary, #3b82f6);
  transition: stroke-dashoffset 0.25s ease;
}

.meter-percent {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}

.context-meter.warning {
  border-color: #f59e0b;
  background: rgba(245, 158, 11, 0.08);
  color: #d97706;
}

.context-meter.warning .meter-fill {
  stroke: #f59e0b;
}

.context-meter.danger {
  border-color: #ef4444;
  background: rgba(239, 68, 68, 0.08);
  color: #dc2626;
}

.context-meter.danger .meter-fill {
  stroke: #ef4444;
}
</style>
