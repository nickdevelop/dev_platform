<script setup lang="ts">
// ChannelChatDrawer: inspect and interact with an assistant-mode channel's
// dedicated BOT conversations (`source=bot&channelId=...`), isolated from
// Quick Assistant history.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Position, VideoPause } from '@element-plus/icons-vue'
import type { BotChannel } from '../../../api/bot'
import {
  listChats,
  sendChatMessage,
  stopChat,
  updateChat,
  type Chat,
  type ChatDelta,
  type ChatMessage,
  type ChatSnapshot,
  type ChatTurn,
} from '../../../api/chats'
import { describeApiError } from '../../../api/client'
import {
  applyDelta as applyDeltaToEntries,
  createChatStream,
  entriesFromMessages,
  MessageView,
  settleStreaming,
  upsertMessage,
  type ChatEntry,
  type ChatStream,
  type ChatStreamStore,
  type StreamStatus,
} from '../../chat'
import { useBotStore } from '../../../stores/bot'
import { useModelsStore } from '../../../stores/models'

const props = defineProps<{
  channel: BotChannel
}>()

const emit = defineEmits<{
  close: []
}>()

const botStore = useBotStore()
const modelsStore = useModelsStore()

const channelChats = ref<Chat[]>([])
const selectedChatId = ref<number>(props.channel.chatId || 0)
const chat = ref<Chat | null>(null)
const messages = ref<ChatEntry[]>([])
const running = ref(false)
const sending = ref(false)
const resetting = ref(false)
const streamStatusMap = ref<Record<number, StreamStatus>>({})
const draftText = ref('')
const selectedModelId = ref<number>(0)
const scrollContainer = ref<HTMLElement | null>(null)

const isCurrentChannelChat = computed(
  () => selectedChatId.value > 0 && selectedChatId.value === props.channel.chatId,
)

const streamStatus = computed<StreamStatus>(() => {
  return streamStatusMap.value[selectedChatId.value] ?? 'idle'
})

const usableTextModels = computed(() => {
  const list: Array<{ id: number; label: string; serviceName: string }> = []
  for (const svc of modelsStore.services) {
    for (const m of svc.models ?? []) {
      if (m.usable && m.supportsText) {
        list.push({
          id: m.id,
          label: m.name || m.modelId,
          serviceName: svc.name,
        })
      }
    }
  }
  return list
})

function scrollToBottom() {
  void nextTick(() => {
    const el = scrollContainer.value
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  })
}

async function loadChannelChats() {
  try {
    const res = await listChats(30, 0, undefined, {
      source: 'bot',
      channelId: props.channel.id,
    })
    channelChats.value = res.chats || []
    if (props.channel.chatId > 0) {
      selectedChatId.value = props.channel.chatId
    } else if (channelChats.value.length > 0 && selectedChatId.value === 0) {
      selectedChatId.value = channelChats.value[0].id
    }
  } catch {
    // Best-effort history loading.
  }
}

const storeAdapter: ChatStreamStore = {
  get streamStatus() {
    return streamStatusMap.value
  },
  set streamStatus(val: Record<number, StreamStatus>) {
    streamStatusMap.value = val
  },
  applySnapshot(snapshot: ChatSnapshot) {
    chat.value = snapshot.conversation
    messages.value = entriesFromMessages(snapshot.messages)
    running.value = snapshot.running
    if (snapshot.conversation.modelId) {
      selectedModelId.value = snapshot.conversation.modelId
    }
    scrollToBottom()
  },
  applyDelta(chatId: number, delta: ChatDelta) {
    if (chatId !== selectedChatId.value) return
    messages.value = applyDeltaToEntries(messages.value, delta)
    scrollToBottom()
  },
  applyMessageUpdate(chatId: number, message: ChatMessage) {
    if (chatId !== selectedChatId.value) return
    messages.value = upsertMessage(messages.value, message)
    if (message.status !== 'pending') {
      running.value = false
    }
    scrollToBottom()
  },
  applyConversation(c: Chat) {
    if (c.id !== selectedChatId.value) return
    chat.value = c
    running.value = c.running
    if (c.modelId) {
      selectedModelId.value = c.modelId
    }
  },
  applyTurn(chatId: number, turn: ChatTurn) {
    if (chatId !== selectedChatId.value) return
    if (turn.phase === 'started') {
      running.value = true
      return
    }
    running.value = false
    messages.value = settleStreaming(messages.value)
    scrollToBottom()
  },
  handleClosed(chatId: number) {
    if (chatId !== selectedChatId.value) return
    running.value = false
  },
}

const stream: ChatStream = createChatStream(storeAdapter)

watch(
  () => props.channel.chatId,
  (chatId) => {
    if (chatId > 0) {
      selectedChatId.value = chatId
    }
    void loadChannelChats()
  },
)

watch(
  selectedChatId,
  (chatId) => {
    stream.close()
    messages.value = []
    chat.value = null
    running.value = false
    selectedModelId.value = props.channel.modelId || 0
    if (chatId > 0) {
      stream.open(chatId)
    }
  },
  { immediate: true },
)

onMounted(() => {
  if (modelsStore.services.length === 0) {
    void modelsStore.load()
  }
  void loadChannelChats()
})

onBeforeUnmount(() => {
  stream.close()
})

async function handleModelChange(modelId: number) {
  selectedModelId.value = modelId
  if (!selectedChatId.value || !modelId) return
  try {
    const updated = await updateChat(selectedChatId.value, { modelId })
    chat.value = updated
  } catch (err: unknown) {
    ElMessage.error(describeApiError(err, '切换模型失败'))
  }
}

async function handleResetContext() {
  resetting.value = true
  try {
    const updated = await botStore.resetContext(props.channel.id)
    if (updated) {
      selectedChatId.value = updated.chatId
      await loadChannelChats()
      ElMessage.success('已创建新的渠道 BOT 对话')
    }
  } finally {
    resetting.value = false
  }
}

async function handleSend() {
  const text = draftText.value.trim()
  if (!text || running.value || sending.value) return
  let targetId = selectedChatId.value
  if (!targetId) {
    const updated = await botStore.resetContext(props.channel.id)
    if (!updated || !updated.chatId) {
      ElMessage.warning('尚未能建立渠道对话，请稍后再试')
      return
    }
    targetId = updated.chatId
    selectedChatId.value = targetId
    await loadChannelChats()
  }
  const modelId =
    selectedModelId.value ||
    chat.value?.modelId ||
    props.channel.modelId ||
    usableTextModels.value[0]?.id ||
    0
  if (!modelId) {
    ElMessage.warning('请先选择或配置可用模型')
    return
  }
  sending.value = true
  try {
    const updated = await sendChatMessage(targetId, {
      text,
      modelId,
    })
    chat.value = updated
    draftText.value = ''
    running.value = true
  } catch (err: unknown) {
    ElMessage.error(describeApiError(err, '发送消息失败'))
  } finally {
    sending.value = false
  }
}

async function handleStop() {
  if (!selectedChatId.value) return
  try {
    await stopChat(selectedChatId.value)
  } catch (err: unknown) {
    ElMessage.error(describeApiError(err, '停止回复失败'))
  }
}
</script>

<template>
  <el-drawer
    :model-value="true"
    size="660px"
    :destroy-on-close="true"
    data-test="channel-chat-drawer"
    @close="emit('close')"
  >
    <template #header>
      <div class="drawer-head">
        <div class="drawer-title-row">
          <span class="drawer-title">渠道对话 · {{ channel.name }}</span>
          <span class="entity-chip entity-chip--assistant">BOT 专属对话</span>
          <el-tag
            :type="channel.status === 'connected' ? 'success' : 'info'"
            size="small"
          >
            {{ channel.status === 'connected' ? '已连接' : '离线' }}
          </el-tag>
          <el-button
            size="small"
            :icon="Plus"
            :loading="resetting"
            class="ml-auto"
            data-test="drawer-new-chat"
            @click="handleResetContext"
          >
            新建对话
          </el-button>
        </div>
        <div class="drawer-meta-row">
          <p class="drawer-subtitle">
            此抽屉仅展示当前消息渠道产生的 BOT 对话，与快捷助手历史隔离。
          </p>
          <el-select
            v-if="channelChats.length > 0"
            v-model="selectedChatId"
            size="small"
            class="drawer-history-select"
            placeholder="切换历史 BOT 对话"
          >
            <el-option
              v-for="item in channelChats"
              :key="item.id"
              :label="`#${item.id} · ${item.title || '新对话'}${item.id === channel.chatId ? '（当前）' : ''}`"
              :value="item.id"
            />
          </el-select>
        </div>
      </div>
    </template>

    <div class="drawer-layout">
      <div ref="scrollContainer" class="drawer-messages">
        <div v-if="messages.length === 0" class="drawer-empty">
          <p class="drawer-empty-title">
            {{ selectedChatId > 0 ? '当前对话暂无消息' : '尚未开启首轮 BOT 对话' }}
          </p>
          <p class="drawer-empty-desc">
            外部用户发来首条消息时会自动绑定专属 BOT 对话；你也可以直接在下方发送测试消息。
          </p>
        </div>
        <MessageView
          v-else
          :messages="messages"
          :capabilities="null"
          :stream-status="streamStatus"
          :running="running"
          display="chat"
          log-label="渠道 BOT 对话记录"
          :can-edit-user-messages="false"
          :can-regenerate-entry-id="null"
        />
      </div>

      <div class="drawer-composer">
        <div class="drawer-composer-toolbar">
          <el-select
            :model-value="selectedModelId || undefined"
            placeholder="跟随 AI 助手模型"
            size="small"
            class="drawer-model-select"
            @change="(v) => handleModelChange(Number(v) || 0)"
          >
            <el-option
              v-for="m in usableTextModels"
              :key="m.id"
              :label="m.label"
              :value="m.id"
            />
          </el-select>
          <span class="drawer-shortcut">
            {{
              isCurrentChannelChat || selectedChatId === 0
                ? 'Enter 发送 · Shift+Enter 换行'
                : '当前查看为历史 BOT 对话'
            }}
          </span>
        </div>

        <el-input
          v-model="draftText"
          type="textarea"
          :rows="3"
          resize="none"
          placeholder="输入消息或指令测试当前渠道绑定的 AI 助手…"
          data-test="channel-chat-input"
          @keydown.enter.exact.prevent="handleSend"
        />

        <div class="drawer-composer-actions">
          <el-button
            v-if="running"
            type="danger"
            :icon="VideoPause"
            @click="handleStop"
          >
            停止生成
          </el-button>
          <el-button
            v-else
            type="primary"
            :loading="sending"
            :disabled="!draftText.trim()"
            :icon="Position"
            data-test="channel-chat-send"
            @click="handleSend"
          >
            发送
          </el-button>
        </div>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped>
.drawer-head {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.drawer-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.drawer-title {
  font-size: var(--text-size-14);
  font-weight: 600;
  color: var(--color-text);
}

.drawer-meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}

.drawer-subtitle {
  margin: 0;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.drawer-history-select {
  width: 220px;
}

.drawer-layout {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  gap: 12px;
}

.drawer-messages {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.drawer-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 220px;
  text-align: center;
  padding: 24px;
}

.drawer-empty-title {
  margin: 0;
  font-size: var(--text-size-13);
  font-weight: 600;
  color: var(--color-text);
}

.drawer-empty-desc {
  margin: 6px 0 0;
  max-width: 380px;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
  line-height: 1.6;
}

.drawer-composer {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
}

.drawer-composer-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.drawer-model-select {
  width: 220px;
}

.drawer-shortcut {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.drawer-composer-actions {
  display: flex;
  justify-content: flex-end;
}
</style>
