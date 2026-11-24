<script setup lang="ts">
// Builtin Coding Agent Engine settings panel.
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
const localConfig = ref(enginesStore.settings?.coder ? { ...enginesStore.settings.coder } : null)

watch(
  () => enginesStore.settings?.coder,
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
  { value: 30, label: '30步（深度）' },
  { value: 50, label: '50步（复杂）' },
  { value: -1, label: '不限制' },
]

async function save() {
  if (!localConfig.value || !enginesStore.settings) return
  const full = {
    ...enginesStore.settings,
    coder: { ...localConfig.value },
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
          <h2 class="text-base font-semibold tracking-tight text-foreground">会话引擎 (Session Coder Engine)</h2>
          <span class="text-2xs px-1.5 py-0.5 rounded border border-border bg-surface-muted text-muted-foreground font-mono">Workspace Coder</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">直连用户工程工作区、自动读取/修改代码并执行终端验证的内置智能编程 Agent 引擎。</p>
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

    <!-- Section 1: Effective Execution Controls -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3.5">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">执行控制与步数预算</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Turn Step Budget</span>
      </div>

      <EngineStepControl
        v-model="localConfig.maxSteps"
        label="单回合最大执行步数"
        hint="单次会话轮次中允许编码 Agent 连续执行工具调用的最大步数上限。设为 -1 表示不作硬性循环限制。"
        input-id="coder-max-steps-input"
        :presets="stepPresets"
      />
    </div>

    <!-- Section 2: AGENTS.md Hierarchy -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3.5">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">规范注入与上下文分层体系</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">AGENTS.md Layering</span>
      </div>

      <p class="text-xs text-muted-foreground leading-normal">
        系统在每轮会话发起前，将自动遵循以下优先级分层组装 System Prompt，确保项目架构规约与个人偏好严格落地：
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5 pt-1">
        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-medium text-foreground flex items-center gap-1.5">
            <span class="text-accent font-mono text-2xs">01</span>
            平台通用规范 (Platform AGENTS.md)
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">读取平台数据目录 <code>dataDir/AGENTS.md</code>，对全平台内置会话兜底生效。</p>
        </div>

        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-medium text-foreground flex items-center gap-1.5">
            <span class="text-accent font-mono text-2xs">02</span>
            个人偏好规范 (User AGENTS.md)
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">读取当前用户家目录 <code>~/.agents/AGENTS.md</code>，承载个人编码与交互偏好。</p>
        </div>

        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-medium text-foreground flex items-center gap-1.5">
            <span class="text-accent font-mono text-2xs">03</span>
            工程约定规范 (Project AGENTS.md)
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">读取工作区根目录 <code>&lt;project&gt;/AGENTS.md</code>，定义工程架构与质量门禁。</p>
        </div>

        <div class="p-3 rounded bg-background/80 border border-border/60 text-xs space-y-1">
          <div class="font-medium text-foreground flex items-center gap-1.5">
            <span class="text-accent font-mono text-2xs">04</span>
            扩展技能套件 (&lt;available_skills&gt;)
          </div>
          <p class="text-muted-foreground text-2xs leading-normal">自动探测并注入项目中的可用 SKILL 技能描述，指导模型按需激活外部能力。</p>
        </div>
      </div>
    </div>

    <!-- Section 3: Workspace Tools Summary -->
    <div class="panel-section bg-surface/50 border border-border/80 rounded p-4 space-y-3">
      <div class="section-head flex items-center justify-between pb-2 border-b border-border/40">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
          <h3 class="text-xs font-semibold text-foreground">工作空间内置标准工具集</h3>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Workspace Tools</span>
      </div>
      <p class="text-xs text-muted-foreground leading-relaxed">
        内置标准工具已在工作树隔离环境中开箱即用：
        <span class="inline-flex flex-wrap gap-1 mt-1 font-mono text-2xs">
          <span class="px-1.5 py-0.5 rounded border border-border bg-surface text-foreground">read_file</span>
          <span class="px-1.5 py-0.5 rounded border border-border bg-surface text-foreground">write_file</span>
          <span class="px-1.5 py-0.5 rounded border border-border bg-surface text-foreground">edit_file</span>
          <span class="px-1.5 py-0.5 rounded border border-border bg-surface text-foreground">bash</span>
          <span class="px-1.5 py-0.5 rounded border border-border bg-surface text-foreground">git_status</span>
          <span class="px-1.5 py-0.5 rounded border border-border bg-surface text-foreground">git_diff</span>
        </span>
      </p>
    </div>

    <!-- Section 4: Extensibility Slots -->
    <div class="panel-section bg-surface/25 border border-dashed border-border/70 rounded p-4 space-y-2.5">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold text-foreground">预留扩展能力</span>
          <span class="text-2xs px-1.5 py-0.2 rounded border border-border/60 bg-surface-muted text-muted-foreground font-mono">TODO / 待实装</span>
        </div>
        <span class="text-2xs text-muted-foreground font-mono">Roadmap</span>
      </div>
      <div class="space-y-1.5 text-xs text-muted-foreground leading-relaxed">
        <p>• <strong>沙箱容器执行隔离</strong>：将高危 Bash 执行隔离至轻量 Docker 或 MicroVM 运行时中，保障宿主安全。</p>
        <p>• <strong>编码 MCP 插件</strong>：为编码 Agent 挂载数据库连接器、Swagger 接口解析等外部专业能力。</p>
      </div>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground font-mono">正在加载会话引擎配置...</div>
</template>
