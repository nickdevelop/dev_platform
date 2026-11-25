<script setup lang="ts">
// Usage and quota statistics panel for agent backends that support usage reporting (e.g. agy).
// Capability-driven: mounted only when agent.supportsUsage is true.
//
// Element Plus migration: `el-button` for the refresh action and the raw-output
// toggle, `el-tag` (state track) for the remaining-quota chip, and failures are
// reported once through `ElMessage` instead of an inline alert banner. The
// quota bars stay ours — they are display surfaces, painted with the semantic
// tokens rather than the raw Tailwind palette.
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { describeApiError } from '../../api/client'
import { getAgentUsage, type AgentUsageReport } from '../../api/agents'

const props = defineProps<{ backend: string }>()

const report = ref<AgentUsageReport | null>(null)
const loading = ref(false)
const showRaw = ref(false)

async function load() {
  loading.value = true
  try {
    report.value = await getAgentUsage(props.backend)
  } catch (caught) {
    ElMessage.error(`获取使用量信息失败：${describeApiError(caught)}`)
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
  if (fraction >= 0.5) return 'bg-platform-success'
  if (fraction >= 0.2) return 'bg-platform-warning'
  return 'bg-platform-danger'
}

function tagType(fraction: number): 'success' | 'warning' | 'danger' {
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
    class="agent-usage-panel p-4 rounded-md bg-platform-surface-muted/40 border border-platform-border/80 space-y-4"
    :data-test="`agent-usage-panel-${props.backend}`"
  >
    <!-- Header -->
    <div class="flex items-center justify-between pb-2 border-b border-platform-border/40 flex-wrap gap-2">
      <div>
        <div class="flex items-center gap-2">
          <strong class="text-xs font-semibold text-platform-text">使用量与配额查询</strong>
          <span v-if="report?.updatedAt" class="text-2xs text-platform-text-muted tabular-nums">
            上次更新：{{ formatTime(report.updatedAt) }}
          </span>
        </div>
        <p class="text-2xs text-platform-text-muted mt-0.5">
          {{ report?.description || '查看此 Agent 后端的实时配额余量与使用限制' }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <el-button
          size="small"
          :disabled="loading"
          :loading="loading"
          data-test="refresh-usage-btn"
          @click="load"
        >
          {{ loading ? '查询中...' : '刷新使用量' }}
        </el-button>
      </div>
    </div>

    <!-- Initial loading skeleton -->
    <div v-if="loading && !report" class="py-6 text-center text-xs text-platform-text-muted">
      正在查询 {{ props.backend }} 最新使用量信息…
    </div>

    <!-- Usage groups -->
    <div v-else-if="report && report.groups && report.groups.length > 0" class="space-y-4">
      <div
        v-for="group in report.groups"
        :key="group.name"
        class="group-card p-3.5 rounded bg-platform-bg/60 border border-platform-border/70 space-y-3"
        :data-test="`usage-group-${group.name}`"
      >
        <div class="flex items-center justify-between flex-wrap gap-1">
          <span class="text-xs font-semibold text-platform-text">{{ group.name }}</span>
          <span v-if="group.description" class="text-2xs text-platform-text-muted">{{ group.description }}</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div
            v-for="bucket in group.buckets"
            :key="bucket.id"
            class="bucket-item p-2.5 rounded bg-platform-surface/60 border border-platform-border/60 space-y-2"
          >
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-platform-text">{{ bucket.name }}</span>
              <el-tag :type="tagType(bucket.remainingFraction)" size="small">
                剩余 {{ formatPercent(bucket.remainingFraction) }}
              </el-tag>
            </div>

            <!-- Progress bar -->
            <div class="w-full bg-platform-surface-muted rounded-full h-1.5 overflow-hidden">
              <div
                class="h-full rounded-full transition-all duration-300"
                :class="barColorClass(bucket.remainingFraction)"
                :style="{ width: `${Math.min(100, Math.max(0, bucket.remainingFraction * 100))}%` }"
              ></div>
            </div>

            <!-- Footer / details -->
            <div class="flex items-center justify-between text-2xs text-platform-text-muted pt-0.5 flex-wrap gap-1">
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
    <div v-else-if="report && report.rawOutput" class="p-3 bg-platform-bg/60 rounded border border-platform-border/70">
      <pre class="font-mono text-xs text-platform-text whitespace-pre-wrap overflow-x-auto">{{ report.rawOutput }}</pre>
    </div>

    <!-- Raw text output accordion when groups are displayed -->
    <div v-if="report?.rawOutput && report.groups && report.groups.length > 0" class="pt-1">
      <el-button size="small" link type="info" data-test="usage-raw-toggle" @click="showRaw = !showRaw">
        {{ showRaw ? '收起终端原始输出' : '展开查看终端原始输出 (/usage)' }}
      </el-button>
      <div v-if="showRaw" class="mt-2 p-3 bg-platform-bg/60 rounded border border-platform-border/70">
        <pre class="font-mono text-2xs text-platform-text-muted whitespace-pre-wrap overflow-x-auto">{{ report.rawOutput }}</pre>
      </div>
    </div>
  </div>
</template>
