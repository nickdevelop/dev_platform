<script setup lang="ts">
// Session Coder Engine settings panel.
// Standard app typography scale (text-xl header, text-sm section, text-xs body).
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
  <div class="engine-panel space-y-6" v-if="localConfig">
    <!-- Header -->
    <header class="panel-header flex items-center justify-between pb-4 border-b border-border/70">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-xl font-semibold tracking-tight text-foreground">会话引擎</h1>
          <span class="text-xs px-2 py-0.5 rounded border border-border bg-surface-muted text-muted-foreground font-mono">Workspace Coder</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">
          直连工程工作区代码树，负责自主读取、编辑工程源码并驱动终端验证的内置编码 Agent 引擎。
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

    <!-- Section 1: Execution Step Budget -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-4" aria-labelledby="coder-budget-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="coder-budget-title" class="text-sm font-semibold text-foreground">执行控制与步数预算</h2>
      </div>

      <EngineStepControl
        v-model="localConfig.maxSteps"
        label="单回合最大执行步数"
        hint="单次会话轮次中允许编码 Agent 连续执行工具调用的最大步数上限。设为 -1 表示不作硬性循环限制。"
        input-id="coder-max-steps-input"
        :presets="stepPresets"
      />
    </section>

    <!-- Section 2: AGENTS.md Hierarchy Cascade (Moment) -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-4" aria-labelledby="coder-rules-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="coder-rules-title" class="text-sm font-semibold text-foreground">规范注入与上下文分层体系</h2>
      </div>

      <p class="text-xs text-muted-foreground leading-relaxed">
        系统在每轮会话发起前，按由浅入深的优先级分层装配 System Prompt，确保平台级约束与项目级工程标准严格执行：
      </p>

      <div class="cascade-flow space-y-2.5 pt-1">
        <div class="cascade-item flex items-start gap-3 p-3.5 rounded border border-border/60 bg-background/60">
          <span class="font-mono text-xs text-accent font-medium px-2 py-0.5 rounded bg-surface border border-border tabular-nums">01</span>
          <div class="flex-1 min-w-0 space-y-1">
            <div class="flex items-center justify-between gap-2">
              <strong class="text-xs font-medium text-foreground">平台通用规范</strong>
              <code class="text-xs font-mono text-muted-foreground">dataDir/AGENTS.md</code>
            </div>
            <p class="text-xs text-muted-foreground leading-relaxed">
              由平台系统运维部署，对平台所有内置会话底座生效，承载全局安全边界与系统级指令。
            </p>
          </div>
        </div>

        <div class="cascade-item flex items-start gap-3 p-3.5 rounded border border-border/60 bg-background/60">
          <span class="font-mono text-xs text-accent font-medium px-2 py-0.5 rounded bg-surface border border-border tabular-nums">02</span>
          <div class="flex-1 min-w-0 space-y-1">
            <div class="flex items-center justify-between gap-2">
              <strong class="text-xs font-medium text-foreground">个人偏好规范</strong>
              <code class="text-xs font-mono text-muted-foreground">~/.agents/AGENTS.md</code>
            </div>
            <p class="text-xs text-muted-foreground leading-relaxed">
              读取当前宿主操作者家目录，承载开发者个人的编码风格、语言习惯与交互约束。
            </p>
          </div>
        </div>

        <div class="cascade-item flex items-start gap-3 p-3.5 rounded border border-border/60 bg-background/60">
          <span class="font-mono text-xs text-accent font-medium px-2 py-0.5 rounded bg-surface border border-border tabular-nums">03</span>
          <div class="flex-1 min-w-0 space-y-1">
            <div class="flex items-center justify-between gap-2">
              <strong class="text-xs font-medium text-foreground">工程约定规范</strong>
              <code class="text-xs font-mono text-muted-foreground">&lt;project&gt;/AGENTS.md</code>
            </div>
            <p class="text-xs text-muted-foreground leading-relaxed">
              位于当前项目代码根目录，定义工程分层架构、测试守卫、提交规范与质量门禁。
            </p>
          </div>
        </div>

        <div class="cascade-item flex items-start gap-3 p-3.5 rounded border border-border/60 bg-background/60">
          <span class="font-mono text-xs text-accent font-medium px-2 py-0.5 rounded bg-surface border border-border tabular-nums">04</span>
          <div class="flex-1 min-w-0 space-y-1">
            <div class="flex items-center justify-between gap-2">
              <strong class="text-xs font-medium text-foreground">扩展技能套件</strong>
              <code class="text-xs font-mono text-muted-foreground">&lt;available_skills&gt;</code>
            </div>
            <p class="text-xs text-muted-foreground leading-relaxed">
              根据项目上下文动态探测并注入 SKILL 元数据，指导模型按需激活外部专业能力。
            </p>
          </div>
        </div>
      </div>
    </section>

    <!-- Section 3: Workspace Tools Summary -->
    <section class="panel-section bg-surface/40 border border-border/80 rounded-lg p-5 space-y-3.5" aria-labelledby="coder-tools-title">
      <div class="section-head pb-2 border-b border-border/40">
        <h2 id="coder-tools-title" class="text-sm font-semibold text-foreground">工作空间内置标准工具集</h2>
      </div>
      <p class="text-xs text-muted-foreground leading-relaxed">
        会话编码引擎在工作树隔离环境中开箱即用以下 6 项标准工具：
      </p>
      <div class="grid grid-cols-2 sm:grid-cols-3 gap-2.5 pt-1 text-xs">
        <div class="p-3 rounded border border-border/70 bg-background/60 flex items-center justify-between">
          <span class="font-mono text-foreground font-medium">read_file</span>
          <span class="text-muted-foreground">读文件</span>
        </div>
        <div class="p-3 rounded border border-border/70 bg-background/60 flex items-center justify-between">
          <span class="font-mono text-foreground font-medium">write_file</span>
          <span class="text-muted-foreground">写文件</span>
        </div>
        <div class="p-3 rounded border border-border/70 bg-background/60 flex items-center justify-between">
          <span class="font-mono text-foreground font-medium">edit_file</span>
          <span class="text-muted-foreground">精准替换</span>
        </div>
        <div class="p-3 rounded border border-border/70 bg-background/60 flex items-center justify-between">
          <span class="font-mono text-foreground font-medium">bash</span>
          <span class="text-muted-foreground">终端执行</span>
        </div>
        <div class="p-3 rounded border border-border/70 bg-background/60 flex items-center justify-between">
          <span class="font-mono text-foreground font-medium">git_status</span>
          <span class="text-muted-foreground">改动检视</span>
        </div>
        <div class="p-3 rounded border border-border/70 bg-background/60 flex items-center justify-between">
          <span class="font-mono text-foreground font-medium">git_diff</span>
          <span class="text-muted-foreground">增量差异</span>
        </div>
      </div>
    </section>

    <!-- Section 4: Extensibility Slots -->
    <section class="panel-section bg-surface/20 border border-border/60 rounded-lg p-5 space-y-3" aria-labelledby="coder-ext-title">
      <div class="section-head pb-2 border-b border-border/40 flex items-center justify-between">
        <h2 id="coder-ext-title" class="text-sm font-semibold text-foreground">规划中扩展能力</h2>
        <span class="text-xs text-muted-foreground font-mono">规划中</span>
      </div>
      <div class="space-y-2 text-xs text-muted-foreground leading-relaxed">
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">沙箱容器执行隔离</strong>：将高危 Bash 命令调度至轻量 Docker 或 MicroVM 运行时中，保障宿主数据安全。</p>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-accent font-mono text-xs select-none">•</span>
          <p><strong class="text-foreground font-medium">编码 MCP 插件</strong>：为编码 Agent 挂载数据库连接器、OpenAPI 接口解析等外部专业能力。</p>
        </div>
      </div>
    </section>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground font-mono">正在加载会话引擎配置...</div>
</template>
