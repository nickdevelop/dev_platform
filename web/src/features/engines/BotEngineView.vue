<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ChatLineRound, Plus } from '@element-plus/icons-vue'
import type {
  BotChannel,
  CreateBotChannelInput,
  UpdateBotChannelInput,
} from '../../api/bot'
import type { BotEngineConfig } from '../../api/engines'
import { formatListTime } from '../../shared/format'
import ListEmptyState from '../../shared/components/ListEmptyState.vue'
import { useBotStore } from '../../stores/bot'
import { useEnginesStore } from '../../stores/engines'
import BotChannelChatDrawer from './BotChannelChatDrawer.vue'
import BotChannelDialog from './BotChannelDialog.vue'
import BotQrLoginDialog from './BotQrLoginDialog.vue'
import EngineStepControl, { type StepPreset } from './EngineStepControl.vue'

const enginesStore = useEnginesStore()
const botStore = useBotStore()

const localConfig = ref<BotEngineConfig | null>(null)
const channelDialogVisible = ref(false)
const editingChannel = ref<BotChannel | null>(null)
const savingChannel = ref(false)
const qrChannel = ref<BotChannel | null>(null)
const chatDrawerChannelId = ref<number | null>(null)

const stepPresets: StepPreset[] = [
  { value: 6, label: '6 步' },
  { value: 12, label: '12 步' },
  { value: 20, label: '20 步' },
  { value: 30, label: '30 步' },
  { value: -1, label: '不限制' },
]

watch(
  () => enginesStore.settings?.bot,
  (val) => {
    if (val && !localConfig.value) {
      localConfig.value = {
        ...val,
        enabledTools: [...(val.enabledTools ?? [])],
      }
    }
  },
  { immediate: true },
)

const chatDrawerChannel = computed(() =>
  chatDrawerChannelId.value === null
    ? null
    : botStore.channels.find((c) => c.id === chatDrawerChannelId.value) ?? null,
)

function descriptorFor(channelType: string) {
  return botStore.descriptors.find((d) => d.type === channelType)
}

function descriptorLabel(channelType: string): string {
  return (
    descriptorFor(channelType)?.label ||
    (channelType === 'wechat' ? '个人微信' : channelType)
  )
}

function asChannel(row: unknown): BotChannel {
  return row as BotChannel
}

function statusTagType(
  status: string,
): 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'connected':
      return 'success'
    case 'waiting_qr':
    case 'scanned':
      return 'warning'
    case 'expired':
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'connected':
      return '已连接'
    case 'waiting_qr':
      return '等待扫码'
    case 'scanned':
      return '待手机确认'
    case 'expired':
      return '凭证已过期'
    case 'error':
      return '连接异常'
    case 'disconnected':
      return '已停用'
    default:
      return '未登录'
  }
}

function selectAllTools() {
  if (!localConfig.value) return
  localConfig.value.enabledTools = botStore.tools.map((t) => t.name)
}

function clearAllTools() {
  if (!localConfig.value) return
  localConfig.value.enabledTools = []
}

async function saveConfig() {
  if (!localConfig.value || !enginesStore.settings) return
  const full = {
    ...enginesStore.settings,
    bot: {
      ...localConfig.value,
      enabledTools: [...localConfig.value.enabledTools],
    },
  }
  const ok = await enginesStore.saveSettings(full)
  if (ok) {
    ElMessage.success('Bot 引擎配置已保存')
  } else {
    ElMessage.error(enginesStore.error || '保存 Bot 引擎配置失败')
  }
}

function openCreateDialog() {
  editingChannel.value = null
  channelDialogVisible.value = true
}

function openEditDialog(channel: BotChannel) {
  editingChannel.value = channel
  channelDialogVisible.value = true
}

async function handleSaveChannel(
  payload: CreateBotChannelInput | UpdateBotChannelInput,
) {
  savingChannel.value = true
  try {
    if (editingChannel.value) {
      const updated = await botStore.updateChannel(
        editingChannel.value.id,
        payload as UpdateBotChannelInput,
      )
      if (updated) {
        ElMessage.success(`渠道「${updated.name}」已更新`)
        channelDialogVisible.value = false
      } else {
        ElMessage.error(botStore.error || '更新渠道失败')
      }
    } else {
      const created = await botStore.createChannel(
        payload as CreateBotChannelInput,
      )
      if (created) {
        channelDialogVisible.value = false
        ElMessage.success(`已为「${created.name}」创建专属对话，请扫码建立连接`)
        if (descriptorFor(created.channelType)?.supportsQrLogin !== false) {
          qrChannel.value = created
        }
      } else {
        ElMessage.error(botStore.error || '创建渠道失败')
      }
    }
  } finally {
    savingChannel.value = false
  }
}

async function toggleChannel(channel: BotChannel, enable: boolean) {
  const ok = enable
    ? await botStore.startChannel(channel.id)
    : await botStore.stopChannel(channel.id)
  if (ok) {
    ElMessage.success(
      enable ? `渠道「${channel.name}」已启动` : `渠道「${channel.name}」已停用`,
    )
  } else {
    ElMessage.error(botStore.error || '切换渠道运行状态失败')
  }
}

function openQrDialog(channel: BotChannel) {
  qrChannel.value = channel
}

function openChatDrawer(channel: BotChannel) {
  chatDrawerChannelId.value = channel.id
}

async function confirmDeleteChannel(channel: BotChannel) {
  try {
    await ElMessageBox.confirm(
      `确定删除渠道「${channel.name}」及其绑定的专属 Bot 对话吗？此操作不可撤销。`,
      '删除 Bot 渠道',
      {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }

  const ok = await botStore.removeChannel(channel.id)
  if (ok) {
    ElMessage.success(`渠道「${channel.name}」已删除`)
    if (chatDrawerChannelId.value === channel.id) {
      chatDrawerChannelId.value = null
    }
  } else {
    ElMessage.error(botStore.error || '删除渠道失败')
  }
}

onMounted(() => {
  void botStore.loadAll()
})

onBeforeUnmount(() => {
  botStore.unsubscribeBus()
})
</script>

<template>
  <div v-if="localConfig" class="bot-engine-page">
    <header class="settings-page-head settings-page-head-split">
      <div>
        <div class="bot-title-row">
          <h1 class="settings-page-title">Bot 引擎</h1>
          <span class="entity-chip entity-chip--wechat">Channel ReAct Engine</span>
        </div>
        <p class="settings-page-desc">
          基于平台现有 ReAct 循环的外部 IM 执行底座，携带平台查询与会话调度工具集，为个人微信等渠道提供统一平台操作支撑。
        </p>
      </div>
      <el-button
        type="primary"
        :loading="enginesStore.saving"
        data-test="save-bot-engine"
        @click="saveConfig"
      >
        保存配置
      </el-button>
    </header>

    <!-- 01 ReAct Loop & Channel Delivery -->
    <section class="settings-panel">
      <div class="settings-panel-head">
        <h2 class="settings-panel-title">01 · 执行控制与步数预算</h2>
        <p class="settings-panel-desc">
          控制 ReAct 引擎的单轮最大工具调用步数及外部 IM 消息投递行为，不绑定固定工程或后端 Agent。
        </p>
      </div>

      <div class="bot-form-stack">
        <EngineStepControl
          v-model="localConfig.maxSteps"
          label="单轮最大工具调用步数"
          hint="限制 Bot 引擎在单次外部消息回复中连续调用平台工具的 ReAct 轮次上限；设为 -1 表示不限制。"
          input-id="bot-max-steps-input"
          :presets="stepPresets"
        />

        <div class="settings-switch-row">
          <div>
            <div class="settings-switch-title">发送「正在输入」状态提示</div>
            <div class="settings-switch-desc">
              ReAct 工具调用与推理期间，定期向微信客户端发送 Typing 心跳指示。
            </div>
          </div>
          <el-switch v-model="localConfig.typingIndicator" />
        </div>

        <div class="settings-switch-row">
          <div>
            <div class="settings-switch-title">Watched 会话完成自动推送</div>
            <div class="settings-switch-desc">
              当通过 Bot 发起的编码会话完成或异常时，自动向来源渠道用户推送执行摘要并同步到专属对话。
            </div>
          </div>
          <el-switch v-model="localConfig.autoNotifySession" />
        </div>

        <div class="bot-reply-length-row">
          <div>
            <div class="settings-switch-title">单条回复分片上限（字符）</div>
            <div class="settings-switch-desc">
              超长文本回复按段落边界切分投递，适配外部 IM 单条长度限制。
            </div>
          </div>
          <el-input-number
            v-model="localConfig.maxReplyLength"
            :min="500"
            :max="4000"
            :step="200"
          />
        </div>
      </div>
    </section>

    <!-- 02 Tool Permissions -->
    <section class="settings-panel">
      <div class="settings-panel-head settings-panel-head-split">
        <div>
          <h2 class="settings-panel-title">02 · Bot 引擎携带工具集</h2>
          <p class="settings-panel-desc">
            与其他引擎的区别在于所携带的平台操作工具集；勾选允许 Bot ReAct 引擎在对话中调用的能力。
          </p>
        </div>
        <div class="bot-inline-actions">
          <el-button size="small" @click="selectAllTools">全选</el-button>
          <el-button size="small" @click="clearAllTools">清空</el-button>
        </div>
      </div>

      <el-checkbox-group v-model="localConfig.enabledTools" class="bot-tools-grid">
        <div
          v-for="tool in botStore.tools"
          :key="tool.name"
          class="bot-tool-card"
        >
          <div class="bot-tool-card-top">
            <el-checkbox :value="tool.name">{{ tool.label }}</el-checkbox>
            <el-tag
              v-if="tool.dangerous"
              type="warning"
              size="small"
            >
              高权限
            </el-tag>
          </div>
          <div class="bot-tool-meta">
            <code class="bot-tool-name">{{ tool.name }}</code>
            <span class="bot-tool-cat">{{ tool.category }}</span>
          </div>
          <p class="bot-tool-desc">{{ tool.description }}</p>
        </div>
      </el-checkbox-group>
    </section>

    <!-- 03 Channels: Connection + Dedicated Bot Chat -->
    <section class="settings-panel">
      <div class="settings-panel-head settings-panel-head-split">
        <div>
          <h2 class="settings-panel-title">03 · Bot 渠道管理</h2>
          <p class="settings-panel-desc">
            新建渠道同时完成两件事：① 与外部渠道（如微信扫码）建立连接；② 基于 Bot 引擎创建一个渠道专属对话。
          </p>
        </div>
        <el-button
          type="primary"
          :icon="Plus"
          data-test="add-bot-channel"
          @click="openCreateDialog"
        >
          新建渠道
        </el-button>
      </div>

      <el-table
        :data="botStore.channels"
        v-loading="botStore.loading"
        class="w-full"
        data-test="bot-channels-table"
      >
        <el-table-column label="渠道名称" min-width="140">
          <template #default="{ row }">
            <div class="bot-name-cell">
              <span class="bot-name-primary">{{ row.name }}</span>
              <span v-if="row.accountId" class="bot-name-sub">
                {{ row.accountId }}
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="协议" width="80">
          <template #default="{ row }">
            <span class="entity-chip entity-chip--wechat">
              {{ descriptorLabel(row.channelType) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="连接状态" width="95">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="专属对话" width="110">
          <template #default="{ row }">
            <el-button
              v-if="row.chatId > 0"
              link
              size="small"
              type="primary"
              @click="openChatDrawer(asChannel(row))"
            >
              Chat #{{ row.chatId }}
            </el-button>
            <span v-else class="bot-time-text">—</span>
          </template>
        </el-table-column>

        <el-table-column label="最近连接" width="125">
          <template #default="{ row }">
            <span class="bot-time-text">
              {{ row.lastConnectedAt ? formatListTime(row.lastConnectedAt) : '—' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="启用" width="65" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              size="small"
              :disabled="!row.hasCredentials && !row.enabled"
              @change="(val) => toggleChannel(asChannel(row), Boolean(val))"
            />
          </template>
        </el-table-column>

        <el-table-column label="操作" width="225" fixed="right" align="right">
          <template #default="{ row }">
            <el-button
              link
              size="small"
              type="primary"
              data-test="bot-channel-qr"
              @click="openQrDialog(asChannel(row))"
            >
              {{ row.hasCredentials ? '重连' : '扫码连接' }}
            </el-button>
            <el-button
              link
              size="small"
              type="primary"
              data-test="bot-channel-chat"
              @click="openChatDrawer(asChannel(row))"
            >
              对话
            </el-button>
            <el-button
              link
              size="small"
              data-test="bot-channel-edit"
              @click="openEditDialog(asChannel(row))"
            >
              编辑
            </el-button>
            <el-button
              link
              size="small"
              type="danger"
              data-test="bot-channel-delete"
              @click="confirmDeleteChannel(asChannel(row))"
            >
              删除
            </el-button>
          </template>
        </el-table-column>

        <template #empty>
          <ListEmptyState
            :state="botStore.loading ? 'loading' : 'empty'"
            :icon="ChatLineRound"
            title="还没有接入任何 Bot 渠道"
            hint="新建渠道后将自动创建基于 Bot 引擎的专属对话，并引导微信扫码建立连接。"
          >
            <el-button
              type="primary"
              :icon="Plus"
              @click="openCreateDialog"
            >
              新建渠道
            </el-button>
          </ListEmptyState>
        </template>
      </el-table>
    </section>

    <BotChannelDialog
      v-if="channelDialogVisible"
      :channel="editingChannel"
      :saving="savingChannel"
      :descriptors="botStore.descriptors"
      @save="handleSaveChannel"
      @cancel="channelDialogVisible = false"
    />

    <BotQrLoginDialog
      v-if="qrChannel"
      :channel="qrChannel"
      @close="qrChannel = null"
      @connected="qrChannel = null"
    />

    <BotChannelChatDrawer
      v-if="chatDrawerChannel"
      :channel="chatDrawerChannel"
      @close="chatDrawerChannelId = null"
    />
  </div>
</template>

<style scoped>
.bot-engine-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.bot-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.bot-form-stack {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.bot-inline-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bot-tools-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  width: 100%;
}

@media (max-width: 900px) {
  .bot-tools-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

.bot-tool-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.bot-tool-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.bot-tool-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.bot-tool-name {
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-primary);
}

.bot-tool-cat {
  font-size: var(--text-size-10);
  color: var(--color-text-muted);
}

.bot-tool-desc {
  margin: 0;
  font-size: var(--text-size-11);
  line-height: 1.5;
  color: var(--color-text-muted);
  white-space: normal;
}

.bot-reply-length-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.bot-name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.bot-name-primary {
  font-size: var(--text-size-12);
  font-weight: 500;
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bot-name-sub {
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bot-time-text {
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}
</style>
