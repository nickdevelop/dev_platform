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
    class="context-meter inline-flex items-center gap-1.5 h-7 px-2 rounded border transition-colors select-none text-xs leading-none cursor-default"
    :class="[
      isDanger
        ? 'danger bg-red-50 dark:bg-red-950/40 border-red-200 dark:border-red-800 text-red-600 dark:text-red-400'
        : isWarning
        ? 'warning bg-amber-50 dark:bg-amber-950/40 border-amber-200 dark:border-amber-800 text-amber-600 dark:text-amber-400'
        : 'bg-slate-100 dark:bg-slate-800/60 border-slate-200 dark:border-slate-700 text-slate-500 dark:text-slate-400',
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
          class="meter-bg stroke-slate-200 dark:stroke-slate-700"
          :d="pathData"
          fill="none"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
        />
        <path
          class="meter-fill transition-all duration-300 ease-out"
          :class="[
            isDanger
              ? 'stroke-red-500'
              : isWarning
              ? 'stroke-amber-500'
              : 'stroke-blue-600 dark:stroke-blue-400',
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
    <span class="meter-percent font-mono text-[11px] font-medium">{{ percentage }}%</span>
  </div>
</template>
