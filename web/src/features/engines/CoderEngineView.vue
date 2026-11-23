<script setup lang="ts">
// Builtin Coding Agent Engine settings panel.
// Conforms to design-taste-frontend app-screens rules.
import { ref, watch } from 'vue'
import { useEnginesStore } from '../../stores/engines'
import { UiButton, UiSelect } from '../../shared/ui'

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

const maxStepsOptions = [
  { value: '10', label: '10 步（轻量）' },
  { value: '20', label: '20 步（默认推荐）' },
  { value: '30', label: '30 步（深度修改）' },
  { value: '50', label: '50 步（复杂任务）' },
  { value: '100', label: '100 步（超长流程）' },
  { value: '-1', label: '不限制（-1）' },
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
    <div class="panel-header border-b border-border/50 pb-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold tracking-tight text-foreground">会话引擎配置 (Session Coder Engine)</h2>
          <p class="text-xs text-muted-foreground mt-0.5">直连用户工程工作区、自动读取/修改代码并执行终端命令的内置智能编程 Agent 引擎。</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="savedSuccess" class="text-xs text-emerald-600 font-medium">配置已保存</span>
          <UiButton size="sm" :loading="enginesStore.saving" @click="save">保存配置</UiButton>
        </div>
      </div>
    </div>

    <!-- Section 1: Effective Execution Controls -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-4">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">执行控制与步数预算</div>
      <div class="max-w-xs">
        <label class="block text-xs font-medium text-foreground mb-1">单回合最大执行步数</label>
        <UiSelect
          :model-value="String(localConfig.maxSteps)"
          :options="maxStepsOptions"
          @update:model-value="localConfig.maxSteps = Number($event)"
        />
        <p class="text-2xs text-muted-foreground mt-1">单次会话轮次中允许 Agent 连续调用工具的最大步数上限（防止死循环）。</p>
      </div>
    </div>

    <!-- Section 2: AGENTS.md Hierarchy -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-3">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">规范注入与上下文分层体系</div>
      <p class="text-xs text-muted-foreground leading-relaxed">
        系统在每轮会话发起前，将自动遵循以下优先级分层组装 System Prompt，确保项目规范与人设对齐：
      </p>
      <div class="space-y-2 pt-1">
        <div class="p-2.5 rounded bg-background border border-border/40 text-xs">
          <div class="font-medium text-foreground">1. 平台通用规范 (Platform AGENTS.md)</div>
          <p class="text-muted-foreground text-2xs mt-0.5">读取平台数据目录 <code>dataDir/AGENTS.md</code>，对平台内所有内置会话生效。</p>
        </div>
        <div class="p-2.5 rounded bg-background border border-border/40 text-xs">
          <div class="font-medium text-foreground">2. 个人偏好规范 (User AGENTS.md)</div>
          <p class="text-muted-foreground text-2xs mt-0.5">读取当前用户家目录 <code>~/.agents/AGENTS.md</code>，承载个人编码与语言偏好。</p>
        </div>
        <div class="p-2.5 rounded bg-background border border-border/40 text-xs">
          <div class="font-medium text-foreground">3. 工程约定规范 (Project AGENTS.md)</div>
          <p class="text-muted-foreground text-2xs mt-0.5">读取当前项目根目录下的 <code>&lt;projectDir&gt;/AGENTS.md</code>，定义工程架构与测试门禁。</p>
        </div>
        <div class="p-2.5 rounded bg-background border border-border/40 text-xs">
          <div class="font-medium text-foreground">4. 扩展技能套件 (&lt;available_skills&gt;)</div>
          <p class="text-muted-foreground text-2xs mt-0.5">自动探测并注入项目中的可用 SKILL 技能描述，指导模型按需激活外部能力。</p>
        </div>
      </div>
    </div>

    <!-- Section 3: Workspace Tools Summary -->
    <div class="panel-section bg-surface/50 border border-border/60 rounded-lg p-4 space-y-2">
      <div class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">工作空间内置工具集</div>
      <p class="text-xs text-muted-foreground">
        内置标准工具已在工作树隔离环境中开箱即用：<span class="font-mono text-foreground font-medium">read_file, write_file, edit_file, bash, git_status, git_diff</span>。
      </p>
    </div>

    <!-- Section 4: Reserved Extensibility (TODO) -->
    <div class="panel-section bg-surface/30 border border-dashed border-border/60 rounded-lg p-4 space-y-3 opacity-80">
      <div class="flex items-center gap-2">
        <span class="section-title text-xs font-semibold uppercase tracking-wider text-muted-foreground">预留扩展能力</span>
        <span class="text-2xs px-1.5 py-0.5 bg-muted text-muted-foreground rounded font-mono">TODO / 待实装</span>
      </div>
      <p class="text-xs text-muted-foreground leading-relaxed">
        • <strong>沙箱容器执行隔离</strong>：将高危 Bash 执行隔离至轻量 Docker 或 MicroVM 中。<br>
        • <strong>编码 MCP 插件</strong>：为编码 Agent 挂载数据库连接器、Swagger 接口解析等外部工具。
      </p>
    </div>
  </div>
  <div v-else class="py-12 text-center text-xs text-muted-foreground">正在加载编码 Agent 配置...</div>
</template>
