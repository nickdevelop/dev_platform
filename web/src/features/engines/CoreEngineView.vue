<script setup lang="ts">
// Runtime Core & Extensions settings panel.
// Conforms to design-taste-frontend app-screens rules.
import { ref, watch } from 'vue'
import { useEnginesStore } from '../../stores/engines'
import { UiButton } from '../../shared/ui'

const enginesStore = useEnginesStore()

const savedSuccess = ref(false)
const localConfig = ref(enginesStore.settings?.core ? { ...enginesStore.settings.core } : null)

watch(
  () => enginesStore.settings?.core,
  (val) => {
    if (val && !localConfig.value) {
      localConfig.value = { ...val }
    }
  },
  { immediate: true },
)

async function save() {
  if (!localConfig.value || !enginesStore.settings) return
  const full = {
    ...enginesStore.settings,
    core: { ...localConfig.value },
  }
  const ok = await enginesStore.saveSettings(full)
  if (ok) {
    savedSuccess.value = true
    setTimeout(() => {
      savedSuccess.value = false
    }, 2500)
  }
}
</script>

<template>
  <div class="engine-panel space-y-6" v-if="localConfig">
    <!-- Header -->
    <div class="panel-header border-b border-border/50 pb-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold tracking-tight text-foreground">运行时核心 (Engine Core)</h2>
          <p class="text-xs text-muted-foreground mt-0.5">底层统一 Agent ReAct 循环内核运行策略、调试日志与未来全局扩展插槽。</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="savedSuccess" class="text-xs text-emerald-600 font-medium">配置已保存</span>
          <UiButton size="sm" :loading="enginesStore.saving" @click="save">保存配置</UiButton>
        </div>
      </div>
    </div>

    <!-- Section 1: Effective Debugging & Tracing -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-4">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">ReAct 循环调试与追踪</div>
      <div>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" v-model="localConfig.enableDebugLog" class="rounded text-primary focus:ring-0" />
          <span class="text-xs text-foreground font-medium">开启全链路 Agent ReAct 详细追踪（记录单步 Step、ToolCall 耗时与 Token 开销）</span>
        </label>
        <p class="text-2xs text-muted-foreground mt-1 pl-6">用于在出现模型生成异常或工具执行死锁时进行端到端回溯排查。</p>
      </div>
    </div>

    <!-- Section 2: Reserved Extensibility (TODO) -->
    <div class="panel-section bg-surface/30 border border-dashed border-border/60 rounded-lg p-4 space-y-3 opacity-80">
      <div class="flex items-center gap-2">
        <span class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">全局能力扩展生态</span>
        <span class="text-2xs px-1.5 py-0.5 bg-muted text-muted-foreground rounded font-mono">TODO / 待实装</span>
      </div>
      <div class="space-y-2 text-xs text-muted-foreground leading-relaxed">
        <p>• <strong>网页搜索服务 (Web Search API)</strong>：预留 Tavily / SearXNG 适配器插槽，为各类 Agent 提供实时网络检索能力。</p>
        <p>• <strong>Model Context Protocol (MCP) 总线</strong>：预留通用 MCP Client 注册中心，支持跨形态统一挂载外部 MCP 服务的标准工具。</p>
      </div>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground">正在加载核心配置...</div>
</template>
