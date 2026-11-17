<script setup lang="ts">
// Usage and quota statistics panel for agent backends that support usage reporting (e.g. agy).
// Capability-driven: mounted only when agent.supportsUsage is true.
import { onMounted, ref } from 'vue'
import { describeApiError } from '../../api/client'
import { getAgentUsage, type AgentUsageReport } from '../../api/agents'
import { UiAlert, UiButton, UiBadge } from '../../shared/ui'

const props = defineProps<{ backend: string }>()

const report = ref<AgentUsageReport | null>(null)
const loading = ref(false)
const error = ref('')
const showRaw = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    report.value = await getAgentUsage(props.backend)
  } catch (caught) {
    error.value = describeApiError(caught)
  } finally {
    loading.value = false
  }
}

function formatTime(isoString?: string): string {
  if (!isoString) return ''
  const parsed = new Date(isoString)
  if (Number.isNaN(parsed.getTime())) return isoString
  return parsed.toLocaleString()
}

function formatPercent(fraction: number): string {
  const pct = Math.round(fraction * 100)
  return `${pct}%`
}

function barColorClass(fraction: number): string {
  if (fraction >= 0.5) return 'bg-emerald-500'
  if (fraction >= 0.2) return 'bg-amber-500'
  return 'bg-rose-500'
}

function badgeVariant(fraction: number): 'success' | 'warning' | 'danger' {
  if (fraction >= 0.5) return 'success'
  if (fraction >= 0.2) return 'warning'
  return 'danger'
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div
    class="agent-usage-panel p-4 rounded-md bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/80 space-y-4"
    :data-test="`agent-usage-panel-${props.backend}`"
  >
    <!-- Header -->
    <div class="flex items-center justify-between pb-2 border-b border-slate-200/80 dark:border-slate-700/60 flex-wrap gap-2">
      <div>
        <div class="flex items-center gap-2">
          <strong class="text-xs font-semibold text-slate-800 dark:text-slate-200">使用量与配额查询</strong>
          <span v-if="report?.updatedAt" class="text-2xs text-slate-400 tabular-nums">
            上次更新：{{ formatTime(report.updatedAt) }}
          </span>
        </div>
        <p class="text-2xs text-slate-400 mt-0.5">
          {{ report?.description || '查看此 Agent 后端的实时配额余量与使用限制' }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <UiButton
          size="sm"
          variant="secondary"
          :disabled="loading"
          :loading="loading"
          data-test="refresh-usage-btn"
          @click="load"
        >
          {{ loading ? '查询中...' : '刷新使用量' }}
        </UiButton>
      </div>
    </div>

    <!-- Error state -->
    <UiAlert v-if="error" variant="error" class="text-xs">
      <div class="flex items-center justify-between gap-2">
        <span>获取使用量信息失败：{{ error }}</span>
        <UiButton size="sm" variant="ghost" @click="load">重试</UiButton>
      </div>
    </UiAlert>

    <!-- Initial loading skeleton -->
    <div v-if="loading && !report" class="py-6 text-center text-xs text-slate-400">
      正在查询 {{ props.backend }} 最新使用量信息…
    </div>

    <!-- Usage groups -->
    <div v-else-if="report && report.groups && report.groups.length > 0" class="space-y-4">
      <div
        v-for="group in report.groups"
        :key="group.name"
        class="group-card p-3.5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3"
        :data-test="`usage-group-${group.name}`"
      >
        <div class="flex items-center justify-between flex-wrap gap-1">
          <span class="text-xs font-semibold text-slate-800 dark:text-slate-200">{{ group.name }}</span>
          <span v-if="group.description" class="text-2xs text-slate-400">{{ group.description }}</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div
            v-for="bucket in group.buckets"
            :key="bucket.id"
            class="bucket-item p-2.5 rounded bg-slate-50 dark:bg-slate-800/60 border border-slate-100 dark:border-slate-800 space-y-2"
          >
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-slate-700 dark:text-slate-300">{{ bucket.name }}</span>
              <UiBadge :variant="badgeVariant(bucket.remainingFraction)" size="sm">
                剩余 {{ formatPercent(bucket.remainingFraction) }}
              </UiBadge>
            </div>

            <!-- Progress bar -->
            <div class="w-full bg-slate-200 dark:bg-slate-700 rounded-full h-1.5 overflow-hidden">
              <div
                class="h-full rounded-full transition-all duration-300"
                :class="barColorClass(bucket.remainingFraction)"
                :style="{ width: `${Math.min(100, Math.max(0, bucket.remainingFraction * 100))}%` }"
              ></div>
            </div>

            <!-- Footer / details -->
            <div class="flex items-center justify-between text-2xs text-slate-400 pt-0.5 flex-wrap gap-1">
              <span v-if="bucket.description" class="truncate max-w-[200px]" :title="bucket.description">
                {{ bucket.description }}
              </span>
              <span v-if="bucket.resetTime" class="tabular-nums">
                重置时间：{{ formatTime(bucket.resetTime) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Fallback if groups are empty but raw output exists -->
    <div v-else-if="report && report.rawOutput" class="p-3 bg-white dark:bg-slate-900 rounded border border-slate-200 dark:border-slate-800">
      <pre class="font-mono text-xs text-slate-700 dark:text-slate-300 whitespace-pre-wrap overflow-x-auto">{{ report.rawOutput }}</pre>
    </div>

    <!-- Raw text output accordion when groups are displayed -->
    <div v-if="report?.rawOutput && report.groups && report.groups.length > 0" class="pt-1">
      <button
        type="button"
        class="text-2xs text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 cursor-pointer flex items-center gap-1 select-none"
        @click="showRaw = !showRaw"
      >
        <span>{{ showRaw ? '收起终端原始输出' : '展开查看终端原始输出 (/usage)' }}</span>
      </button>
      <div v-if="showRaw" class="mt-2 p-3 bg-white dark:bg-slate-900 rounded border border-slate-200 dark:border-slate-800">
        <pre class="font-mono text-2xs text-slate-600 dark:text-slate-400 whitespace-pre-wrap overflow-x-auto">{{ report.rawOutput }}</pre>
      </div>
    </div>
  </div>
</template>
