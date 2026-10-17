<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ScheduleType, Task, TaskRun } from '../../api/tasks'
import { useSessionsStore } from '../../stores/sessions'
import { useTasksStore } from '../../stores/tasks'
import SessionChatPanel from '../chat/SessionChatPanel.vue'
import { extractExecutionPrompt } from './promptExtractor'

const props = defineProps<{
  task: Task
  taskRuns: TaskRun[]
  creatingSession?: boolean
  triggering?: boolean
}>()

const emit = defineEmits<{
  (e: 'createSession'): void
  (e: 'triggerExecution'): void
  (e: 'actionError', msg: string): void
}>()

const tasksStore = useTasksStore()
const sessionsStore = useSessionsStore()

const transactionalStage = ref<1 | 2 | 3>(1)
const extractingPrompt = ref(false)
const extractNotice = ref('')
const executionPromptInput = ref('')
const stage2ScheduleType = ref<ScheduleType>('none')
const stage2ScheduledAt = ref('')
const stage2CronExpr = ref('0 * * * *')
const savingStage2 = ref(false)
const showPromptDetails = ref(false)
const viewingSessionId = ref<string | null>(null)

const cronPresets = [
  { label: '每小时整点', expr: '0 * * * *' },
  { label: '工作日早 09:00', expr: '0 9 * * 1-5' },
  { label: '每天凌晨 02:00', expr: '0 2 * * *' },
  { label: '每周一凌晨 03:00', expr: '0 3 * * 1' },
]

function formatDateForInput(isoString?: string): string {
  if (!isoString) {
    const d = new Date(Date.now() + 3600 * 1000)
    d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
    return d.toISOString().slice(0, 16)
  }
  const d = new Date(isoString)
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
  return d.toISOString().slice(0, 16)
}

function syncTaskToStageState(t: Task | null) {
  if (!t) return
  executionPromptInput.value = t.executionPrompt || ''
  stage2ScheduleType.value = t.scheduleEnabled ? t.scheduleType || 'none' : 'none'
  stage2CronExpr.value = t.cronExpr || '0 * * * *'
  stage2ScheduledAt.value = formatDateForInput(t.scheduledAt)
  extractNotice.value = ''

  if (t.isConfirmed || t.status === 'ready_for_execution' || t.status === 'executing' || t.status === 'completed') {
    transactionalStage.value = 3
  } else {
    transactionalStage.value = 1
  }
}

watch(
  () => props.task.id,
  () => {
    viewingSessionId.value = props.task.activeSessionId || null
    syncTaskToStageState(props.task)
  },
  { immediate: true },
)

watch(
  () => props.taskRuns,
  (runs) => {
    if (runs.length > 0 && !viewingSessionId.value) {
      viewingSessionId.value = runs[0].sessionId
    }
  },
  { immediate: true },
)

function switchTransactionalStage(stage: 1 | 2 | 3) {
  transactionalStage.value = stage
  if (stage === 1) {
    viewingSessionId.value = props.task.activeSessionId || null
  } else if (stage === 3 && props.taskRuns.length > 0) {
    viewingSessionId.value = props.taskRuns[0].sessionId
  }
}

async function extractAndGoToStage2() {
  if (!props.task) return
  extractingPrompt.value = true
  emit('actionError', '')

  try {
    if (props.task.activeSessionId) {
      await sessionsStore.loadHistory(props.task.projectId, props.task.activeSessionId)
      const messages = sessionsStore.messagesFor(props.task.activeSessionId)
      const extracted = extractExecutionPrompt(messages, {
        title: props.task.title,
        description: props.task.description,
      })

      if (extracted) {
        executionPromptInput.value = extracted
        extractNotice.value = '✓ 已从分析会话中自动识别并提取执行方案，请核对与微调'
      } else {
        executionPromptInput.value = `【执行方案】${props.task.title}\n需求与目标：${props.task.description || ''}\n1. `
        extractNotice.value = '会话中暂未检测到完整的 Agent 输出方案，已生成初始骨架，可直接补充编写'
      }
    } else {
      executionPromptInput.value = `【执行方案】${props.task.title}\n需求与目标：${props.task.description || ''}\n1. `
      extractNotice.value = '尚未启动会话，已基于任务需求生成初始执行方案模板'
    }

    transactionalStage.value = 2
  } catch (err: any) {
    emit('actionError', err.message || '提取执行方案失败')
  } finally {
    extractingPrompt.value = false
  }
}

async function reExtractPrompt() {
  if (!props.task || !props.task.activeSessionId) return
  extractingPrompt.value = true
  try {
    await sessionsStore.loadHistory(props.task.projectId, props.task.activeSessionId)
    const messages = sessionsStore.messagesFor(props.task.activeSessionId)
    const extracted = extractExecutionPrompt(messages, {
      title: props.task.title,
      description: props.task.description,
    })
    if (extracted) {
      executionPromptInput.value = extracted
      extractNotice.value = '✓ 已重新从分析会话中提取最新执行方案'
    } else {
      extractNotice.value = '会话中暂未找到新的 Agent 方案输出'
    }
  } catch (err: any) {
    emit('actionError', err.message || '重新提取失败')
  } finally {
    extractingPrompt.value = false
  }
}

async function confirmStage2AndAdvance() {
  if (!props.task) return
  const prompt = executionPromptInput.value.trim()
  if (!prompt) {
    emit('actionError', '请填写或确认可直接执行的方案提示词')
    return
  }

  savingStage2.value = true
  emit('actionError', '')

  try {
    await tasksStore.saveExecutionPrompt(props.task.id, prompt)

    const scheduleEnabled = stage2ScheduleType.value !== 'none'
    let scheduledAtIso: string | undefined = undefined
    if (stage2ScheduleType.value === 'once' && stage2ScheduledAt.value) {
      scheduledAtIso = new Date(stage2ScheduledAt.value).toISOString()
    }

    await tasksStore.updateSchedule(props.task.id, {
      scheduleType: scheduleEnabled ? stage2ScheduleType.value : 'none',
      scheduleEnabled,
      cronExpr: stage2ScheduleType.value === 'cron' ? stage2CronExpr.value.trim() : undefined,
      scheduledAt: scheduledAtIso,
    })

    transactionalStage.value = 3
    if (props.taskRuns.length > 0) {
      viewingSessionId.value = props.taskRuns[0].sessionId
    }
  } catch (err: any) {
    emit('actionError', err.message || '确认并就绪失败')
  } finally {
    savingStage2.value = false
  }
}

function switchViewingSession(sid: string) {
  viewingSessionId.value = sid
}
</script>

<template>
  <div class="transactional-flow-workspace">
    <!-- Stage Stepper Bar -->
    <div class="stage-stepper-bar">
      <div
        class="stage-step-item"
        :class="{
          active: transactionalStage === 1,
          completed: task.isConfirmed || transactionalStage > 1,
        }"
        @click="switchTransactionalStage(1)"
      >
        <div class="step-num">
          <span v-if="task.isConfirmed || transactionalStage > 1">✓</span>
          <span v-else>1</span>
        </div>
        <div class="step-label">
          <span class="step-title-text">任务分析与澄清</span>
          <span class="step-status-sub">{{ task.activeSessionId ? '对话分析中' : '待启动' }}</span>
        </div>
      </div>

      <div class="stage-step-arrow">➔</div>

      <div
        class="stage-step-item"
        :class="{
          active: transactionalStage === 2,
          completed: task.isConfirmed || transactionalStage > 2,
        }"
        @click="switchTransactionalStage(2)"
      >
        <div class="step-num">
          <span v-if="task.isConfirmed || transactionalStage > 2">✓</span>
          <span v-else>2</span>
        </div>
        <div class="step-label">
          <span class="step-title-text">方案确认与就绪</span>
          <span class="step-status-sub">{{ task.isConfirmed ? '已就绪' : '待确认' }}</span>
        </div>
      </div>

      <div class="stage-step-arrow">➔</div>

      <div
        class="stage-step-item"
        :class="{
          active: transactionalStage === 3,
          completed: task.status === 'completed',
        }"
        @click="switchTransactionalStage(3)"
      >
        <div class="step-num">
          <span v-if="task.status === 'completed'">✓</span>
          <span v-else>3</span>
        </div>
        <div class="step-label">
          <span class="step-title-text">任务执行与监控</span>
          <span class="step-status-sub">
            {{ task.isConfirmed ? `${taskRuns.length} 次运行` : '未就绪' }}
          </span>
        </div>
      </div>
    </div>

    <!-- STAGE 1: Task Analysis Dedicated View -->
    <section v-if="transactionalStage === 1" class="stage-view stage-analysis-view">
      <div class="stage-banner">
        <span class="banner-badge">💡 步骤 1 · 任务分析与澄清</span>
        <p class="banner-desc">
          Agent 正在对任务需求进行分析，重点识别信息歧义、执行过程不确定点与潜在风险。在对话确认完毕后，点击右下方按钮系统将<strong>自动识别并提炼执行方案</strong>，无缝切换至下一步。
        </p>
      </div>

      <!-- Analysis Chat Session Container -->
      <div class="analysis-session-wrap">
        <div v-if="!task.activeSessionId" class="no-session-card">
          <div class="no-session-content">
            <h4>尚未启动任务分析会话</h4>
            <p>
              已为此任务配置了 <strong>{{ task.backend }}</strong> Agent 与
              <strong>{{ task.model }}</strong> 模型。
              点击下方按钮拉起【任务分析】会话，Agent 将对任务进行分析并识别歧义与不确定点。
            </p>
            <button
              type="button"
              class="btn-launch-session"
              :disabled="creatingSession"
              @click="emit('createSession')"
            >
              {{ creatingSession ? '正在创建会话...' : '🚀 创建并启动任务会话' }}
            </button>
          </div>
        </div>

        <div v-else class="embedded-chat-panel full-height">
          <SessionChatPanel
            :key="task.activeSessionId"
            :project-id="task.projectId"
            :session-id="task.activeSessionId"
          />
        </div>
      </div>

      <!-- Floating / Bottom Extraction Action Bar -->
      <div class="stage-bottom-bar">
        <div class="bottom-bar-info">
          <span class="tip-icon">✨</span>
          <span>分析对话完成后，点击右侧按钮系统将自动提取 Agent 总结的执行方案并进入下一步确认。</span>
        </div>
        <button
          type="button"
          class="btn-extract-next"
          :disabled="extractingPrompt"
          @click="extractAndGoToStage2"
        >
          {{ extractingPrompt ? '正在提取方案...' : '🪄 提取方案并进入下一步 ➔' }}
        </button>
      </div>
    </section>

    <!-- STAGE 2: Plan Confirmation & Ready Dedicated View -->
    <section v-else-if="transactionalStage === 2" class="stage-view stage-confirm-view">
      <div class="stage-confirm-container">
        <!-- Notice banner -->
        <div class="confirmation-notice">
          <div class="notice-badge">⚠️ 核心免打断要求</div>
          <p class="notice-text">
            在【任务分析】阶段，Agent 需分析任务需求，识别信息是否有歧义、执行过程是否存在不确定点与潜在风险，全面与用户确认清楚，并输出可直接执行的执行计划，作为第二步执行的提示词。后续独立执行阶段将不再向用户提问。
          </p>
        </div>

        <!-- Execution Prompt Box -->
        <div class="execution-prompt-box">
          <div class="prompt-box-header">
            <div class="prompt-box-title-wrap">
              <span class="prompt-box-title">📝 提炼第二步执行提示词（Execution Prompt）</span>
              <button
                type="button"
                class="btn-re-extract"
                :disabled="extractingPrompt"
                @click="reExtractPrompt"
              >
                {{ extractingPrompt ? '正在提取...' : '🔄 从分析会话重新提取' }}
              </button>
            </div>
            <span class="prompt-box-hint">
              任务分析完成后，请在此确认可直接执行的方案提示词，确认后任务即刻就绪。
            </span>
          </div>

          <div v-if="extractNotice" class="extract-notice-bar" role="status">
            {{ extractNotice }}
          </div>

          <textarea
            v-model="executionPromptInput"
            rows="9"
            placeholder="例如：请按以下已确认方案执行部署：1.拉取镜像 2.重启容器 3.健康检查..."
            class="execution-prompt-textarea"
          ></textarea>
        </div>

        <!-- Integrated Schedule Configuration -->
        <div class="schedule-config-card">
          <div class="card-head">
            <span class="card-head-title">⏰ 调度与触发方式</span>
            <span class="card-head-sub">配置任务就绪后的执行策略（支持立即手动执行、定时单次或周期循环）</span>
          </div>

          <div class="schedule-options">
            <label class="schedule-opt-item" :class="{ selected: stage2ScheduleType === 'none' }">
              <input v-model="stage2ScheduleType" type="radio" value="none" />
              <div class="opt-content">
                <strong>仅手动触发</strong>
                <span>随时在执行界面点击【▶ 立即执行】拉起独立会话</span>
              </div>
            </label>

            <label class="schedule-opt-item" :class="{ selected: stage2ScheduleType === 'once' }">
              <input v-model="stage2ScheduleType" type="radio" value="once" />
              <div class="opt-content">
                <strong>定时单次执行</strong>
                <span>在指定的未来时间到达时，由系统自动执行一次</span>
              </div>
            </label>

            <label class="schedule-opt-item" :class="{ selected: stage2ScheduleType === 'cron' }">
              <input v-model="stage2ScheduleType" type="radio" value="cron" />
              <div class="opt-content">
                <strong>周期循环调度 (Cron)</strong>
                <span>按设定的 Cron 表达式周期性自动启动独立 Agent 会话执行</span>
              </div>
            </label>
          </div>

          <!-- Schedule inputs depending on type -->
          <div v-if="stage2ScheduleType === 'once'" class="schedule-input-row">
            <label class="input-label" for="stage2-datetime">执行时刻：</label>
            <input
              id="stage2-datetime"
              v-model="stage2ScheduledAt"
              type="datetime-local"
              class="datetime-input"
            />
          </div>

          <div v-else-if="stage2ScheduleType === 'cron'" class="schedule-input-row">
            <label class="input-label" for="stage2-cron">Cron 表达式：</label>
            <input
              id="stage2-cron"
              v-model="stage2CronExpr"
              type="text"
              placeholder="0 * * * *"
              class="cron-input"
            />
            <div class="preset-chips">
              <button
                v-for="p in cronPresets"
                :key="p.expr"
                type="button"
                class="preset-chip"
                @click="stage2CronExpr = p.expr"
              >
                {{ p.label }}
              </button>
            </div>
          </div>
        </div>

        <!-- Bottom Confirmation Actions -->
        <div class="confirm-actions-bar">
          <button
            type="button"
            class="btn-secondary"
            @click="switchTransactionalStage(1)"
          >
            ⬅ 返回分析会话修改
          </button>
          <button
            type="button"
            class="btn-save-prompt btn-confirm-ready btn-primary"
            :disabled="savingStage2"
            @click="confirmStage2AndAdvance"
          >
            {{ savingStage2 ? '正在保存就绪...' : '✓ 确认执行方案并就绪（进入执行） ➔' }}
          </button>
        </div>
      </div>
    </section>

    <!-- STAGE 3: Execution & Monitoring Dedicated View -->
    <section v-else class="stage-view stage-execution-view">
      <!-- Ready & Control Panel -->
      <div class="execution-control-panel">
        <div class="control-left">
          <div class="ready-status-badge">
            <span class="status-indicator"></span>
            <strong>任务已就绪</strong>
          </div>

          <div v-if="task.scheduleEnabled" class="schedule-status-chip">
            <span class="chip-icon">⏰</span>
            <span v-if="task.scheduleType === 'cron'">周期调度：<code>{{ task.cronExpr }}</code></span>
            <span v-else>定时执行</span>
          </div>

          <div class="plan-summary-toggle" @click="showPromptDetails = !showPromptDetails">
            <span>{{ showPromptDetails ? '▼ 收起执行方案提示词' : '▶ 查看当前执行方案提示词' }}</span>
          </div>
        </div>

        <div class="control-right">
          <button
            type="button"
            class="btn-secondary btn-sm"
            @click="switchTransactionalStage(2)"
            title="重新核对或微调执行提示词"
          >
            📝 调整方案提示词
          </button>
          <button
            type="button"
            class="btn-secondary btn-sm"
            @click="switchTransactionalStage(1)"
            title="回看任务分析主会话"
          >
            💬 查看分析记录
          </button>
          <button
            type="button"
            class="btn-trigger btn-lg"
            :disabled="triggering"
            @click="emit('triggerExecution')"
            title="立即启动一次独立执行会话"
          >
            {{ triggering ? '正在启动...' : '▶ 立即执行一次' }}
          </button>
        </div>
      </div>

      <!-- Collapsible Prompt Preview -->
      <div v-if="showPromptDetails" class="prompt-preview-drawer">
        <div class="drawer-header">
          <strong>执行方案提示词（每次独立执行依据）：</strong>
        </div>
        <pre class="drawer-content">{{ task.executionPrompt || '（未设置）' }}</pre>
      </div>

      <!-- Execution Runs Section -->
      <div class="runs-workbench-wrap">
        <div class="runs-header-bar">
          <div class="runs-header-title">
            <h4>独立执行记录（共 {{ taskRuns.length }} 次）</h4>
            <span class="runs-sub-hint">事务型任务每次执行均新启动独立 Agent 会话免打断执行</span>
          </div>

          <div v-if="taskRuns.length > 0" class="runs-tab-scroll">
            <button
              v-for="r in taskRuns"
              :key="r.id"
              type="button"
              class="run-tab-chip"
              :class="{
                active: viewingSessionId === r.sessionId,
                success: r.status === 'success',
                failed: r.status === 'failed',
              }"
              @click="switchViewingSession(r.sessionId)"
            >
              <span class="run-trig">[{{ r.triggerType === 'cron' ? '周期' : r.triggerType === 'once' ? '定时' : '手动' }}]</span>
              <span class="run-time">{{ new Date(r.triggeredAt).toLocaleTimeString() }}</span>
              <span class="run-badge">{{ r.status }}</span>
            </button>
          </div>
        </div>

        <!-- Execution Session Display -->
        <div v-if="taskRuns.length === 0" class="empty-runs-card">
          <div class="empty-runs-icon">🚀</div>
          <h3>等待首次执行</h3>
          <p>任务方案已确认就绪，点击上方【▶ 立即执行一次】启动免打断运行，或等待设定的定时/周期调度自动触发。</p>
        </div>

        <div v-else-if="viewingSessionId" class="embedded-chat-panel full-height">
          <SessionChatPanel
            :key="viewingSessionId"
            :project-id="task.projectId"
            :session-id="viewingSessionId"
          />
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
@import './transactional-flow.css';
</style>
