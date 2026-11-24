<script setup lang="ts">
// Collaborative Orchestrator Engine settings panel.
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

const stepPresets: StepPreset[] = [
  { value: 10, label: '10步（轻量）' },
  { value: 20, label: '20步（推荐）' },
  { value: 30, label: '30步（标准）' },
  { value: 50, label: '50步（长流程）' },
  { value: -1, label: '不限制' },
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
  <div class="engine-panel space-y-5" v-if="localConfig">
    <!-- Header -->
    <div class="panel-header flex items-center justify-between pb-4 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h2 class="text-base font-semibold tracking-tight text-foreground">协同引擎 (Collaborative Orchestrator Engine)</h2>
          <span class="text-2xs px-1.5 py-0.5 rounded border border-border bg-surface-muted text-muted-foreground font-mono">Team Brain</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">多 Agent 协同流程的顶层指挥中枢，负责目标拆解、角色分派、异步等待与综合验收。</p>
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

    <!-- Section 1: Effective Orchestration Controls -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3.5">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">流程调度与步数预算</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Orchestration Budget</span>
      </div>

      <EngineStepControl
        v-model="localConfig.maxSteps"
        label="协同任务最大编排步数"
        hint="控制单个协同任务生命周期中允许中控大脑发起调度的最大轮次上限。设为 -1 表示不限制中控调度步数。"
        input-id="orchestrator-max-steps-input"
        :presets="stepPresets"
      />
    </div>

    <!-- Section 2: Collaboration Tools Summary -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3.5">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">中枢协同调度工具集</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Workflow Tools</span>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-mono text-2xs text-accent font-semibold flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
            flow_dispatch_role
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">
            向团队架构师、开发工程师、代码审查等专家角色分派明确的子任务与约束。
          </p>
        </div>

        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-mono text-2xs text-accent font-semibold flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-blue-500"></span>
            flow_wait_session
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">
            显式挂起流程等待指定专家子会话执行完毕，并自动提取其产出的文件或测试结果。
          </p>
        </div>

        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-mono text-2xs text-accent font-semibold flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
            flow_ask_user
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">
            在协同遇到架构歧义或高风险技术选型时，主动向操作者提问并挂起等待决策。
          </p>
        </div>

        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-mono text-2xs text-accent font-semibold flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-purple-500"></span>
            flow_complete_task
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">
            汇总各个专家成员的阶段性交付物，输出综合完成总结并将协同任务标记为归档。
          </p>
        </div>
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
        <p>• <strong>中枢代码巡检（只读代码树）</strong>：允许中控大脑直接查阅 Git 工作区代码与 Diff，以量化评估各角色交付物质量。</p>
        <p>• <strong>动态多中枢竞争与投票</strong>：引入副中控或评审委员会机制把关复杂技术方案。</p>
      </div>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground font-mono">正在加载协同引擎配置...</div>
</template>
