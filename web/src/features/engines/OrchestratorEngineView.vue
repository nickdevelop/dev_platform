<script setup lang="ts">
// Collaborative Orchestrator Engine settings panel.
// Standard app typography scale (text-xl header, text-sm section, text-xs body).
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
  { value: 10, label: '10 步' },
  { value: 20, label: '20 步' },
  { value: 30, label: '30 步' },
  { value: 50, label: '50 步' },
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
  <div class="engine-panel space-y-6" v-if="localConfig">
    <!-- Header -->
    <header class="panel-header flex items-center justify-between pb-4 border-b border-border/70">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-xl font-semibold tracking-tight text-foreground">协同引擎</h1>
          <span class="text-xs px-2 py-0.5 rounded border border-border bg-surface-muted text-muted-foreground font-mono">Team Orchestrator</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">
          多 Agent 协作流程的顶层调度中枢，负责目标拆解、角色分派、异步等待与综合验收。
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

    <!-- Section 1: Effective Orchestration Controls -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-4" aria-labelledby="orchestrator-budget-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="orchestrator-budget-title" class="text-sm font-semibold text-foreground">流程调度与步数预算</h2>
      </div>

      <EngineStepControl
        v-model="localConfig.maxSteps"
        label="协同任务最大编排步数"
        hint="控制单个协同任务生命周期中允许中控大脑发起调度的最大轮次上限。设为 -1 表示不限制中控调度步数。"
        input-id="orchestrator-max-steps-input"
        :presets="stepPresets"
      />
    </section>

    <!-- Section 2: Collaboration Tools Summary (Pipeline Moment) -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-4" aria-labelledby="orchestrator-tools-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="orchestrator-tools-title" class="text-sm font-semibold text-foreground">中枢协同调度工具集</h2>
      </div>

      <p class="text-xs text-muted-foreground leading-relaxed">
        中枢大脑通过以下 4 项标准编排工具驱动团队成员间的工作流流转：
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
        <div class="p-4 rounded border border-border/60 bg-background/60 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <code class="font-mono text-xs text-accent font-medium">flow_dispatch_role</code>
            <span class="text-xs font-mono text-muted-foreground px-1.5 py-0.5 rounded bg-surface border border-border">01 · 分派</span>
          </div>
          <p class="text-xs text-muted-foreground leading-relaxed">
            向架构师、开发工程师、审查员等角色分配明确的子任务目标与工作区上下文约束。
          </p>
        </div>

        <div class="p-4 rounded border border-border/60 bg-background/60 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <code class="font-mono text-xs text-accent font-medium">flow_wait_session</code>
            <span class="text-xs font-mono text-muted-foreground px-1.5 py-0.5 rounded bg-surface border border-border">02 · 等待</span>
          </div>
          <p class="text-xs text-muted-foreground leading-relaxed">
            显式挂起流程等待指定子会话完成，并在恢复时自动汇总其生成的文件与测试结果。
          </p>
        </div>

        <div class="p-4 rounded border border-border/60 bg-background/60 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <code class="font-mono text-xs text-accent font-medium">flow_ask_user</code>
            <span class="text-xs font-mono text-muted-foreground px-1.5 py-0.5 rounded bg-surface border border-border">03 · 对齐</span>
          </div>
          <p class="text-xs text-muted-foreground leading-relaxed">
            在协同遇到架构歧义、高危破坏性操作或重大技术选型时，主动向操作者提问。
          </p>
        </div>

        <div class="p-4 rounded border border-border/60 bg-background/60 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <code class="font-mono text-xs text-accent font-medium">flow_complete_task</code>
            <span class="text-xs font-mono text-muted-foreground px-1.5 py-0.5 rounded bg-surface border border-border">04 · 验收</span>
          </div>
          <p class="text-xs text-muted-foreground leading-relaxed">
            核对并归纳各成员的阶段性交付物，输出综合完成报告并将协同任务归档。
          </p>
        </div>
      </div>
    </section>

    <!-- Section 3: Extensibility Slots -->
    <section class="panel-section bg-surface/20 border border-border/60 rounded-lg p-5 space-y-3" aria-labelledby="orchestrator-ext-title">
      <div class="section-head pb-2 border-b border-border/40 flex items-center justify-between">
        <h2 id="orchestrator-ext-title" class="text-sm font-semibold text-foreground">规划中扩展能力</h2>
        <span class="text-xs text-muted-foreground font-mono">规划中</span>
      </div>
      <div class="space-y-2 text-xs text-muted-foreground leading-relaxed">
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">中枢只读代码巡检</strong>：允许编排中枢直接查阅 Git 工作区代码树与 Diff 记录，量化校验交付物。</p>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">动态多中枢竞争与投票</strong>：引入副中枢与评审仲裁机制，复杂技术方案由多视角独立打分。</p>
        </div>
      </div>
    </section>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground font-mono">正在加载协同引擎配置...</div>
</template>
