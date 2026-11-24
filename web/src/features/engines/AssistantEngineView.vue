<script setup lang="ts">
// Assistant Chat Engine settings panel.
// Conforms to design-taste-frontend (Data Terminal / App Screens) rules:
// - Rules over shadows, 1px accent active indicators
// - Tabular numerical alignments
// - Unified EngineStepControl integration with unlimited support
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
  { value: 3, label: '3步（极简）' },
  { value: 5, label: '5步（推荐）' },
  { value: 10, label: '10步（探索）' },
  { value: 20, label: '20步' },
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
  <div class="engine-panel space-y-5" v-if="localConfig">
    <!-- Header -->
    <div class="panel-header flex items-center justify-between pb-4 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h2 class="text-base font-semibold tracking-tight text-foreground">助手引擎 (Assistant Engine)</h2>
          <span class="text-2xs px-1.5 py-0.5 rounded border border-border bg-surface-muted text-muted-foreground font-mono">Builtin</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">负责平台日常对话、项目元数据检索与提示词库管理等对话助手的运行时引擎行为。</p>
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
    </div>

    <!-- Section 1: Loop Control & Step Budget -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3.5">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">对话循环与步数预算</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">ReAct Loop Budget</span>
      </div>

      <EngineStepControl
        v-model="localConfig.maxSteps"
        label="单轮最大工具调用步数"
        hint="防止模型在连续调用平台内部工具时陷入死循环。达到步数上限后将强制结束工具调用并输出最终文本答复。"
        input-id="assistant-max-steps-input"
        :presets="stepPresets"
      />
    </div>

    <!-- Section 2: Platform Capability Tools -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3.5">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">平台能力与工具箱</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Platform Toolkits</span>
      </div>

      <div class="space-y-2">
        <label class="flex items-start gap-3 p-3 rounded bg-background/80 border border-border/60 hover:border-border-strong cursor-pointer transition-colors">
          <input
            type="checkbox"
            v-model="localConfig.enablePlatformTools"
            class="mt-0.5 rounded border-border text-accent focus:ring-0"
          />
          <div class="space-y-0.5">
            <div class="text-xs font-medium text-foreground">开启平台管理权限（平台管家模式）</div>
            <p class="text-2xs text-muted-foreground leading-normal">
              允许对话助手通过内置工具读取并操作平台自身资源（如检索项目拓扑、提示词库读写、角色库查询等）。
            </p>
          </div>
        </label>
      </div>
    </div>

    <!-- Section 3: Extensibility Slots -->
    <div class="panel-section bg-surface/25 border border-dashed border-border/70 rounded p-4 space-y-2.5">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold text-foreground">预留扩展能力</span>
          <span class="text-2xs px-1.5 py-0.2 rounded border border-border/60 bg-surface-muted text-muted-foreground font-mono">TODO / 待实装</span>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Roadmap</span>
      </div>
      <div class="space-y-1.5 text-xs text-muted-foreground leading-relaxed">
        <p>• <strong>细粒度工具权限白名单</strong>：支持按助手实例独立勾选开放的平台函数（如仅允许只读类工具，禁止写操作）。</p>
        <p>• <strong>联网搜索集成</strong>：预留外部搜索引擎适配器，为日常对话助手补充最新互联网知识检索。</p>
        <p>• <strong>Model Context Protocol (MCP)</strong>：支持通过标准 MCP 协议动态为助手挂载第三方数据源与自定义工具。</p>
      </div>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground font-mono">正在加载助手引擎配置...</div>
</template>
