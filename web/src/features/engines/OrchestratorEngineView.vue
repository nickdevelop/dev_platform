<script setup lang="ts">
// Collaborative Orchestrator Engine settings panel.
// Conforms to design-taste-frontend app-screens rules.
import { ref, watch } from 'vue'
import { useEnginesStore } from '../../stores/engines'
import { UiButton, UiSelect } from '../../shared/ui'

const enginesStore = useEnginesStore()

const savedSuccess = ref(false)
const localConfig = ref(enginesStore.settings?.orchestrator ? { ...enginesStore.settings.orchestrator } : null)

watch(
  () => enginesStore.settings?.orchestrator,
  (val) => {
    if (val && !localConfig.value) {
      localConfig.value = { ...val }
    }
  },
  { immediate: true },
)

const maxStepsOptions = [
  { value: '10', label: '10 步（小型流程）' },
  { value: '20', label: '20 步（标准默认）' },
  { value: '30', label: '30 步（复杂工程协同）' },
  { value: '50', label: '50 步（大型长流程）' },
]

async function save() {
  if (!localConfig.value || !enginesStore.settings) return
  const full = {
    ...enginesStore.settings,
    orchestrator: { ...localConfig.value },
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
          <h2 class="text-lg font-semibold tracking-tight text-foreground">协同中控大脑配置 (Collaborative Orchestrator)</h2>
          <p class="text-xs text-muted-foreground mt-0.5">多 Agent 协同流程的顶层指挥中枢，负责目标拆解、角色分派、异步等待与综合验收。</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="savedSuccess" class="text-xs text-emerald-600 font-medium">配置已保存</span>
          <UiButton size="sm" :loading="enginesStore.saving" @click="save">保存配置</UiButton>
        </div>
      </div>
    </div>

    <!-- Section 1: Effective Orchestration Controls -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-4">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">流程调度步数控制</div>
      <div class="max-w-xs">
        <label class="block text-xs font-medium text-foreground mb-1">协同任务最大编排步数</label>
        <UiSelect
          :model-value="String(localConfig.maxSteps)"
          :options="maxStepsOptions"
          @update:model-value="localConfig.maxSteps = Number($event)"
        />
        <p class="text-2xs text-muted-foreground mt-1">控制一个协同任务生命周期中允许中控大脑发起调度的最大轮次上限。</p>
      </div>
    </div>

    <!-- Section 2: Orchestration Prompt -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-3">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">中枢工作准则提示词 (System Prompt)</div>
      <textarea
        v-model="localConfig.systemPrompt"
        rows="4"
        class="w-full text-xs font-mono bg-background border border-border/80 rounded-md p-2.5 focus:outline-none focus:border-primary text-foreground leading-relaxed resize-y"
        placeholder="输入中控大脑全局工作准则..."
      ></textarea>
      <p class="text-2xs text-muted-foreground">中控大脑在调度时默认遵循的工作守则与角色协同规范（在具体任务或团队中亦可覆盖定制）。</p>
    </div>

    <!-- Section 3: Collaboration Tools Summary -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-2">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">中枢协同调度工具集</div>
      <div class="text-xs text-muted-foreground leading-relaxed">
        • <span class="font-mono text-foreground font-medium">flow_dispatch_role</span>：向团队架构师、开发工程师等专家分派具体子任务；<br>
        • <span class="font-mono text-foreground font-medium">flow_wait_session</span>：显式等待目标角色会话执行完毕并读取交付物；<br>
        • <span class="font-mono text-foreground font-medium">flow_ask_user</span>：向用户提问并挂起流程等待决策确认；<br>
        • <span class="font-mono text-foreground font-medium">flow_complete_task</span>：输出最终完成总结并标记协同任务成功。
      </div>
    </div>

    <!-- Section 4: Reserved Extensibility (TODO) -->
    <div class="panel-section bg-surface/30 border border-dashed border-border/60 rounded-lg p-4 space-y-3 opacity-80">
      <div class="flex items-center gap-2">
        <span class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">预留扩展能力</span>
        <span class="text-2xs px-1.5 py-0.5 bg-muted text-muted-foreground rounded font-mono">TODO / 待实装</span>
      </div>
      <p class="text-xs text-muted-foreground leading-relaxed">
        • <strong>中枢代码巡检（只读代码树）</strong>：允许中控大脑直接查阅 Git 工作区代码以评估各角色交付物质量。<br>
        • <strong>动态多中枢竞争与投票</strong>：引入副中控或评审委员会机制把关复杂变更。
      </p>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground">正在加载中控大脑配置...</div>
</template>
