<script setup lang="ts">
// Assistant Chat Engine settings panel.
// Conforms to design-taste-frontend app-screens rules.
import { ref, watch } from 'vue'
import { useEnginesStore } from '../../stores/engines'
import { UiButton, UiSelect } from '../../shared/ui'

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

const maxStepsOptions = [
  { value: '3', label: '3 轮（极简）' },
  { value: '5', label: '5 轮（推荐默认）' },
  { value: '10', label: '10 轮（深入探索）' },
  { value: '15', label: '15 轮' },
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
    <div class="panel-header border-b border-border/50 pb-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold tracking-tight text-foreground">助手引擎配置 (Assistant Engine)</h2>
          <p class="text-xs text-muted-foreground mt-0.5">负责平台日常对话、项目元数据检索与提示词库管理等对话助手的运行时引擎行为。</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="savedSuccess" class="text-xs text-emerald-600 font-medium">配置已保存</span>
          <UiButton size="sm" :loading="enginesStore.saving" @click="save">保存配置</UiButton>
        </div>
      </div>
    </div>

    <!-- Section 1: Effective Parameters -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-4">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">对话循环控制</div>
      <div class="max-w-xs">
        <label class="block text-xs font-medium text-foreground mb-1">单轮最大工具调用步数</label>
        <UiSelect
          :model-value="String(localConfig.maxSteps)"
          :options="maxStepsOptions"
          @update:model-value="localConfig.maxSteps = Number($event)"
        />
        <p class="text-2xs text-muted-foreground mt-1">防止模型陷入连续工具调用死循环的硬性上限（超出后强制输出文本答复）。</p>
      </div>
    </div>

    <!-- Section 3: Toolkits Mount -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-3">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">平台能力与工具箱</div>
      <div class="space-y-2">
        <label class="flex items-start gap-2.5 p-2.5 rounded bg-background border border-border/40 cursor-pointer">
          <input type="checkbox" v-model="localConfig.enablePlatformTools" class="mt-0.5 rounded text-primary focus:ring-0" />
          <div class="text-xs">
            <span class="font-medium text-foreground">开启平台管理权限（平台管家模式）</span>
            <p class="text-muted-foreground text-2xs mt-0.5">允许助手通过工具调用操作平台自身资源（如检索项目清单、查询/新增提示词、查看角色库等）。</p>
          </div>
        </label>
      </div>
    </div>

    <!-- Section 4: Reserved Extensibility (TODO) -->
    <div class="panel-section bg-surface/30 border border-dashed border-border/60 rounded-lg p-4 space-y-3 opacity-80">
      <div class="flex items-center gap-2">
        <span class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">预留扩展能力</span>
        <span class="text-2xs px-1.5 py-0.5 bg-muted text-muted-foreground rounded font-mono">TODO / 待实装</span>
      </div>
      <p class="text-xs text-muted-foreground leading-relaxed">
        • <strong>细粒度工具权限白名单</strong>：支持按会话分组独立勾选启用的平台函数（如仅允许只读类工具，禁止写操作）。<br>
        • <strong>联网搜索集成</strong>：支持对接外部搜索引擎为对话助手补充实时网络知识。<br>
        • <strong>Model Context Protocol (MCP)</strong>：支持通过标准 MCP 协议动态挂载自定义后端工具。
      </p>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground">正在加载助手配置...</div>
</template>
