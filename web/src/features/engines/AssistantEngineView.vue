<script setup lang="ts">
// Assistant Chat Engine settings panel.
// Standard app typography scale (text-xl header, text-sm section, text-xs body).
import { ref, watch } from 'vue'
import { useEnginesStore } from '../../stores/engines'
import { UiButton } from '../../shared/ui'
import EngineStepControl, { type StepPreset } from './EngineStepControl.vue'

const enginesStore = useEnginesStore()

const savedSuccess = ref(false)
const localConfig = ref(enginesStore.settings?.steward ? { ...enginesStore.settings.steward } : null)

watch(
  () => enginesStore.settings?.steward,
  (val) => {
    if (val && !localConfig.value) {
      localConfig.value = { ...val }
    }
  },
  { immediate: true },
)

const stepPresets: StepPreset[] = [
  { value: 3, label: '3 步' },
  { value: 5, label: '5 步' },
  { value: 10, label: '10 步' },
  { value: 20, label: '20 步' },
  { value: -1, label: '不限制' },
]

async function save() {
  if (!localConfig.value || !enginesStore.settings) return
  const full = {
    ...enginesStore.settings,
    steward: { ...localConfig.value },
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
    <header class="panel-header flex items-center justify-between pb-4 border-b border-border/70">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-xl font-semibold tracking-tight text-foreground">助手引擎</h1>
          <span class="text-xs px-2 py-0.5 rounded border border-border bg-surface-muted text-muted-foreground font-mono">Platform Steward</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">
          负责平台日常对话、项目元数据检索与提示词库管理等对话助手的运行时引擎行为。
        </p>
      </div>
      <div class="flex items-center gap-2.5">
        <span v-if="savedSuccess" class="text-xs text-emerald-600 dark:text-emerald-400 font-medium flex items-center gap-1 select-none">
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <polyline points="20 6 9 17 4 12" />
          </svg>
          配置已保存
        </span>
        <UiButton size="sm" :loading="enginesStore.saving" @click="save">保存配置</UiButton>
      </div>
    </header>

    <!-- Section 1: Loop Control & Step Budget -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-4" aria-labelledby="assistant-budget-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="assistant-budget-title" class="text-sm font-semibold text-foreground">对话循环与步数预算</h2>
      </div>

      <EngineStepControl
        v-model="localConfig.maxSteps"
        label="单轮最大工具调用步数"
        hint="防止模型在连续调用平台内部工具时陷入死循环。达到步数上限后将强制结束工具调用并输出最终文本答复。"
        input-id="assistant-max-steps-input"
        :presets="stepPresets"
      />
    </section>

    <!-- Section 2: Platform Capability Tools -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-4" aria-labelledby="assistant-tools-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="assistant-tools-title" class="text-sm font-semibold text-foreground">平台能力与工具权限</h2>
      </div>

      <div class="space-y-2">
        <label class="flex items-start gap-3 p-3.5 rounded border border-border/70 bg-background/60 hover:border-border transition-colors cursor-pointer select-none">
          <input
            type="checkbox"
            v-model="localConfig.enablePlatformTools"
            class="mt-0.5 rounded border-border text-accent focus:ring-0 focus:ring-offset-0 cursor-pointer"
          />
          <div class="space-y-1">
            <div class="text-xs font-medium text-foreground">开启平台管理权限（平台管家模式）</div>
            <p class="text-xs text-muted-foreground leading-relaxed">
              允许对话助手调用内置系统级工具读取并管理平台资源（包含检索项目拓扑结构、提示词库读写、角色库查询等）。
            </p>
          </div>
        </label>
      </div>
    </section>

    <!-- Section 3: Extensibility Slots -->
    <section class="panel-section bg-surface/20 border border-border/60 rounded-lg p-5 space-y-3" aria-labelledby="assistant-ext-title">
      <div class="section-head pb-2 border-b border-border/40 flex items-center justify-between">
        <h2 id="assistant-ext-title" class="text-sm font-semibold text-foreground">规划中扩展能力</h2>
        <span class="text-xs text-muted-foreground font-mono">规划中</span>
      </div>
      <div class="space-y-2 text-xs text-muted-foreground leading-relaxed">
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">细粒度工具权限白名单</strong>：支持按助手实例独立勾选开放的平台函数（如仅允许只读类工具，禁止写操作）。</p>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">联网搜索集成</strong>：预留外部搜索引擎适配器，为日常对话助手补充最新互联网知识检索。</p>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">Model Context Protocol (MCP)</strong>：支持通过标准 MCP 协议动态为助手挂载第三方数据源与自定义工具。</p>
        </div>
      </div>
    </section>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground font-mono">正在加载助手引擎配置...</div>
</template>
