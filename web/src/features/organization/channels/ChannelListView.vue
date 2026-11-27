<script setup lang="ts">
// ChannelListView: message channels under the organization center.
//
// Conforms to UI Spec v2 3-pane list layout (`.list-page-header` +
// `.list-page-panel` with `ListToolbar`, `el-table`, `ListEmptyState` and
// `el-pagination`). Each channel operates in single-channel context mode and
// binds either an AI Assistant (`assistant`) or a Project + Role + Builtin
// Agent (`agent`).
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ChatLineRound, Plus } from '@element-plus/icons-vue'
import type {
  BotChannel,
  CreateBotChannelInput,
  UpdateBotChannelInput,
} from '../../../api/bot'
import ListToolbar from '../../../shared/components/ListToolbar.vue'
import ListEmptyState from '../../../shared/components/ListEmptyState.vue'
import { formatListTime } from '../../../shared/format'
import { useBotStore } from '../../../stores/bot'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { useProjectsStore } from '../../../stores/projects'
import { usePromptsStore } from '../../../stores/prompts'
import { useAgentsStore } from '../../../stores/agents'
import ChannelEditDialog from './ChannelEditDialog.vue'
import ChannelQrLoginDialog from './ChannelQrLoginDialog.vue'
import ChannelChatDrawer from './ChannelChatDrawer.vue'

const route = useRoute()
const router = useRouter()
const botStore = useBotStore()
const chatsStore = useChatsStore()
const modelsStore = useModelsStore()
const projectsStore = useProjectsStore()
const promptsStore = usePromptsStore()
const agentsStore = useAgentsStore()

const keywordDraft = ref('')
const keyword = ref('')
const modeFilter = ref<'all' | 'assistant' | 'agent'>('all')

const showDialog = ref(false)
const editingChannel = ref<BotChannel | null>(null)
const savingChannel = ref(false)
const qrChannel = ref<BotChannel | null>(null)
const chatDrawerChannelId = ref<number | null>(null)

const chatDrawerChannel = computed(() =>
  chatDrawerChannelId.value === null
    ? null
    : (botStore.channels.find((c) => c.id === chatDrawerChannelId.value) ?? null),
)

const modeFilterOptions = computed(() => [
  { label: `全部 (${botStore.channels.length})`, value: 'all' },
  {
    label: `AI 助手 (${botStore.channels.filter((c) => c.targetMode !== 'agent').length})`,
    value: 'assistant',
  },
  {
    label: `内置 Agent (${botStore.channels.filter((c) => c.targetMode === 'agent').length})`,
    value: 'agent',
  },
])

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

function targetModeLabel(mode?: string): string {
  return mode === 'agent' ? '内置 Agent' : 'AI 助手'
}

function assistantNameFor(channel: BotChannel): string {
  if (!channel.chatGroupId) return '未绑定助手'
  const group = chatsStore.groupById(channel.chatGroupId)
  return group ? group.name : `助手 #${channel.chatGroupId}`
}

function projectNameFor(channel: BotChannel): string {
  if (!channel.defaultProjectId) return '未绑定项目'
  const proj = projectsStore.byId(channel.defaultProjectId)
  return proj ? proj.name : `项目 #${channel.defaultProjectId}`
}

function rolePromptNameFor(channel: BotChannel): string {
  if (!channel.promptId) return ''
  const p = promptsStore.byId(channel.promptId)
  return p ? p.name : `角色 #${channel.promptId}`
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
      return '待确认'
    case 'expired':
      return '已过期'
    case 'error':
      return '异常'
    case 'disconnected':
      return '已停用'
    default:
      return '未登录'
  }
}

function syncRouteState() {
  if (route.query.action === 'new') {
    editingChannel.value = null
    showDialog.value = true
  } else if (route.query.edit) {
    const target = botStore.channels.find((c) => c.id === Number(route.query.edit))
    if (target) {
      editingChannel.value = target
      showDialog.value = true
    }
  } else if (showDialog.value) {
    showDialog.value = false
    editingChannel.value = null
  }
}

watch(
  () => [route.query, botStore.channels],
  () => {
    syncRouteState()
  },
  { deep: true },
)

watch(
  () => botStore.error,
  (msg) => {
    if (msg) {
      ElMessage.error(msg)
    }
  },
)

onMounted(async () => {
  await Promise.all([
    botStore.loadAll(),
    chatsStore.groups.length === 0 ? chatsStore.loadGroups() : Promise.resolve(),
    projectsStore.projects.length === 0 ? projectsStore.load() : Promise.resolve(),
    promptsStore.prompts.length === 0 ? promptsStore.loadPrompts() : Promise.resolve(),
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
    agentsStore.backends.length === 0 ? agentsStore.loadBackends() : Promise.resolve(),
  ])
  syncRouteState()
})

onBeforeUnmount(() => {
  botStore.unsubscribeBus()
})

function applySearch() {
  keyword.value = keywordDraft.value.trim()
}

function resetSearch() {
  keywordDraft.value = ''
  keyword.value = ''
  modeFilter.value = 'all'
}

const filteredChannels = computed(() => {
  let list = botStore.channels
  if (modeFilter.value === 'assistant') {
    list = list.filter((c) => c.targetMode !== 'agent')
  } else if (modeFilter.value === 'agent') {
    list = list.filter((c) => c.targetMode === 'agent')
  }
  const q = keyword.value.trim().toLowerCase()
  if (q) {
    list = list.filter((c) => {
      const targetText =
        c.targetMode === 'agent'
          ? `${projectNameFor(c)} ${rolePromptNameFor(c)}`.toLowerCase()
          : assistantNameFor(c).toLowerCase()
      return (
        c.name.toLowerCase().includes(q) ||
        (c.accountId && c.accountId.toLowerCase().includes(q)) ||
        targetText.includes(q)
      )
    })
  }
  return list
})

const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]
const totalRows = computed(() => filteredChannels.value.length)
const pagedChannels = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredChannels.value.slice(start, start + pageSize.value)
})

watch([keyword, modeFilter], () => {
  page.value = 1
})

watch([totalRows, pageSize], () => {
  const lastPage = Math.max(1, Math.ceil(totalRows.value / pageSize.value))
  if (page.value > lastPage) page.value = lastPage
})

const emptyState = computed<'loading' | 'filtered' | 'empty'>(() => {
  if (botStore.loading) return 'loading'
  return keyword.value ? 'filtered' : 'empty'
})

const emptyTitle = computed(() =>
  emptyState.value === 'filtered' ? '没有匹配的消息渠道' : '当前暂无消息渠道',
)

const emptyHint = computed(() =>
  emptyState.value === 'filtered'
    ? `关键词「${keyword.value}」没有命中任何渠道名称、账号或绑定服务。`
    : '每个消息渠道通过单渠道专属上下文运行，可绑定「AI 助手」或「项目 + 角色 + 内置 Agent」。',
)

function openCreate() {
  editingChannel.value = null
  showDialog.value = true
  void router.replace({ query: { action: 'new' } })
}

function openEdit(channel: BotChannel) {
  editingChannel.value = channel
  showDialog.value = true
  void router.replace({ query: { edit: String(channel.id) } })
}

function closeEditor() {
  showDialog.value = false
  editingChannel.value = null
  void router.replace({ query: {} })
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
        closeEditor()
      }
    } else {
      const created = await botStore.createChannel(
        payload as CreateBotChannelInput,
      )
      if (created) {
        closeEditor()
        ElMessage.success(`渠道「${created.name}」已创建，请扫码建立连接`)
        if (descriptorFor(created.channelType)?.supportsQrLogin !== false) {
          qrChannel.value = created
        }
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
      enable ? `渠道「${channel.name}」已启用` : `渠道「${channel.name}」已停用`,
    )
  }
}

function openQrDialog(channel: BotChannel) {
  qrChannel.value = channel
}

function openContextView(channel: BotChannel) {
  if (channel.targetMode === 'agent') {
    if (!channel.defaultProjectId || !channel.activeSessionId) {
      ElMessage.info('该渠道尚未生成内置 Agent 会话，外部首条消息到达时将自动创建。')
      return
    }
    void router.push({
      name: 'session',
      params: {
        projectId: String(channel.defaultProjectId),
        sessionId: channel.activeSessionId,
      },
    })
    return
  }
  chatDrawerChannelId.value = channel.id
}

async function confirmResetContext(channel: BotChannel) {
  try {
    await ElMessageBox.confirm(
      `将为渠道「${channel.name}」新建专属${channel.targetMode === 'agent' ? '编码会话' : ' BOT 对话'}并重置当前单渠道上下文，确认继续吗？`,
      '新建渠道会话',
      {
        type: 'info',
        confirmButtonText: '新建',
        cancelButtonText: '取消',
      },
    )
  } catch {
    return
  }
  const updated = await botStore.resetContext(channel.id)
  if (updated) {
    ElMessage.success(`已为「${channel.name}」开启新会话`)
  }
}

async function confirmDeleteChannel(channel: BotChannel) {
  try {
    await ElMessageBox.confirm(
      `删除后消息渠道「${channel.name}」将断开外部连接并解除上下文绑定，此操作不可逆。`,
      `删除消息渠道「${channel.name}」？`,
      {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        confirmButtonType: 'danger',
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
  }
}
</script>

<template>
  <div
    class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden"
    data-test="channel-list"
  >
    <header class="list-page-header">
      <h1 class="list-page-title">消息渠道</h1>
      <p class="list-page-desc">
        外部 IM 渠道接入与差异化 AI 服务绑定，每个渠道独立维护单一专属对话或内置 Agent 自动放行会话。
      </p>
    </header>

    <div class="list-page-panel" data-test="channel-panel">
      <div class="list-panel-toolbar">
        <ListToolbar
          v-model:keyword="keywordDraft"
          placeholder="搜索渠道…"
          hint="搜索渠道名称、绑定助手或目标项目…"
          search-test="btn-search-channel"
          reset-test="btn-reset-channel"
          @search="applySearch"
          @reset="resetSearch"
        >
          <template #actions>
            <el-button
              type="primary"
              :icon="Plus"
              data-test="btn-create-channel"
              @click="openCreate"
            >
              新建渠道
            </el-button>
          </template>

          <template #filters>
            <el-select
              v-model="modeFilter"
              class="list-page-filter"
              data-test="select-mode-filter"
              aria-label="按绑定模式筛选"
            >
              <el-option
                v-for="option in modeFilterOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </template>
        </ListToolbar>
      </div>

      <div class="list-panel-table">
        <el-table
          :data="pagedChannels"
          row-key="id"
          height="100%"
          data-test="channel-table"
        >
          <el-table-column label="渠道名称" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <div class="channel-name-cell">
                <div class="channel-title-line">
                  <span class="font-medium text-foreground truncate">{{ row.name }}</span>
                  <span class="id-chip font-mono">#{{ row.id }}</span>
                </div>
                <span v-if="row.accountId" class="channel-sub font-mono">
                  {{ row.accountId }}
                </span>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="协议" width="96">
            <template #default="{ row }">
              <span class="entity-chip entity-chip--wechat">
                {{ descriptorLabel(row.channelType) }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="绑定模式" width="112">
            <template #default="{ row }">
              <span
                class="entity-chip"
                :class="
                  row.targetMode === 'agent'
                    ? 'entity-chip--agent'
                    : 'entity-chip--assistant'
                "
              >
                {{ targetModeLabel(row.targetMode) }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="关联服务与当前上下文" min-width="240" show-overflow-tooltip>
            <template #default="{ row }">
              <div v-if="row.targetMode === 'agent'" class="channel-binding-cell">
                <span class="text-foreground truncate">
                  {{ projectNameFor(asChannel(row)) }}
                  <span v-if="rolePromptNameFor(asChannel(row))" class="text-platform-text-muted">
                    · {{ rolePromptNameFor(asChannel(row)) }}
                  </span>
                </span>
                <span class="channel-sub font-mono">
                  {{
                    row.activeSessionId
                      ? `会话 ${row.activeSessionId}`
                      : '首条消息自动创建会话（自动放行）'
                  }}
                </span>
              </div>
              <div v-else class="channel-binding-cell">
                <span class="text-foreground truncate">
                  {{ assistantNameFor(asChannel(row)) }}
                </span>
                <span class="channel-sub font-mono">
                  {{
                    row.chatId > 0
                      ? `BOT 对话 #${row.chatId}`
                      : '首条消息自动创建专属对话'
                  }}
                </span>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="连接状态" width="96">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">
                {{ statusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="最近连接" width="128">
            <template #default="{ row }">
              <span class="text-platform-text-muted">
                {{ row.lastConnectedAt ? formatListTime(row.lastConnectedAt) : '—' }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="启用" width="68" align="center">
            <template #default="{ row }">
              <el-switch
                size="small"
                :model-value="row.enabled"
                :disabled="!row.hasCredentials && !row.enabled"
                :data-test="`switch-channel-${row.id}`"
                @change="(val) => toggleChannel(asChannel(row), Boolean(val))"
              />
            </template>
          </el-table-column>

          <el-table-column label="操作" width="256" fixed="right" align="right">
            <template #default="{ row }">
              <div class="inline-flex items-center gap-1">
                <el-button
                  link
                  size="small"
                  type="primary"
                  :data-test="`btn-qr-channel-${row.id}`"
                  @click="openQrDialog(asChannel(row))"
                >
                  {{ row.hasCredentials ? '重连' : '扫码' }}
                </el-button>
                <el-button
                  link
                  size="small"
                  type="primary"
                  :data-test="`btn-context-channel-${row.id}`"
                  @click="openContextView(asChannel(row))"
                >
                  {{ row.targetMode === 'agent' ? '会话' : '对话' }}
                </el-button>
                <el-button
                  link
                  size="small"
                  :data-test="`btn-new-context-channel-${row.id}`"
                  @click="confirmResetContext(asChannel(row))"
                >
                  新会话
                </el-button>
                <el-button
                  link
                  size="small"
                  :data-test="`btn-edit-channel-${row.id}`"
                  @click="openEdit(asChannel(row))"
                >
                  编辑
                </el-button>
                <el-button
                  link
                  size="small"
                  type="danger"
                  :data-test="`btn-delete-channel-${row.id}`"
                  @click="confirmDeleteChannel(asChannel(row))"
                >
                  删除
                </el-button>
              </div>
            </template>
          </el-table-column>

          <template #empty>
            <ListEmptyState
              :state="emptyState"
              :icon="ChatLineRound"
              :title="emptyTitle"
              :hint="emptyHint"
              @reset="resetSearch"
            >
              <el-button
                type="primary"
                :icon="Plus"
                data-test="btn-new-channel"
                @click="openCreate"
              >
                新建渠道
              </el-button>
            </ListEmptyState>
          </template>
        </el-table>
      </div>

      <div class="list-panel-footer" data-test="channel-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="totalRows"
          :page-sizes="pageSizes"
          size="small"
          background
          layout="total, sizes, prev, pager, next"
        />
      </div>
    </div>

    <ChannelEditDialog
      v-if="showDialog"
      :channel="editingChannel"
      :saving="savingChannel"
      :descriptors="botStore.descriptors"
      @save="handleSaveChannel"
      @cancel="closeEditor"
    />

    <ChannelQrLoginDialog
      v-if="qrChannel"
      :channel="qrChannel"
      @close="qrChannel = null"
      @connected="qrChannel = null"
    />

    <ChannelChatDrawer
      v-if="chatDrawerChannel"
      :channel="chatDrawerChannel"
      @close="chatDrawerChannelId = null"
    />
  </div>
</template>

<style scoped>
.channel-name-cell,
.channel-binding-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.channel-title-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.channel-sub {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.id-chip {
  flex: none;
  padding: 0 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  font-size: var(--text-size-10);
  line-height: 16px;
  color: var(--color-text-muted);
}
</style>
