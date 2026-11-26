<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Position, VideoPause } from '@element-plus/icons-vue'
import type { BotChannel } from '../../api/bot'
import {
  sendChatMessage,
  stopChat,
  updateChat,
  type Chat,
  type ChatDelta,
  type ChatMessage,
  type ChatSnapshot,
  type ChatTurn,
} from '../../api/chats'
import { describeApiError } from '../../api/client'
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
} from '../chat'
import { useModelsStore } from '../../stores/models'

const props = defineProps<{
  channel: BotChannel
}>()

const emit = defineEmits<{
  close: []
}>()

const modelsStore = useModelsStore()

const chat = ref<Chat | null>(null)
const messages = ref<ChatEntry[]>([])
const running = ref(false)
const sending = ref(false)
const streamStatusMap = ref<Record<number, StreamStatus>>({})
const draftText = ref('')
const selectedModelId = ref<number>(0)
const scrollContainer = ref<HTMLElement | null>(null)

const streamStatus = computed<StreamStatus>(() => {
  return streamStatusMap.value[props.channel.chatId] ?? 'idle'
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
    if (chatId !== props.channel.chatId) return
    messages.value = applyDeltaToEntries(messages.value, delta)
    scrollToBottom()
  },
  applyMessageUpdate(chatId: number, message: ChatMessage) {
    if (chatId !== props.channel.chatId) return
    messages.value = upsertMessage(messages.value, message)
    if (message.status !== 'pending') {
      running.value = false
    }
    scrollToBottom()
  },
  applyConversation(c: Chat) {
    if (c.id !== props.channel.chatId) return
    chat.value = c
    running.value = c.running
    if (c.modelId) {
      selectedModelId.value = c.modelId
    }
  },
  applyTurn(chatId: number, turn: ChatTurn) {
    if (chatId !== props.channel.chatId) return
    if (turn.phase === 'started') {
      running.value = true
      return
    }
    running.value = false
    messages.value = settleStreaming(messages.value)
    scrollToBottom()
  },
  handleClosed(chatId: number) {
    if (chatId !== props.channel.chatId) return
    running.value = false
  },
}

const stream: ChatStream = createChatStream(storeAdapter)

watch(
  () => props.channel.chatId,
  (chatId) => {
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
})

onBeforeUnmount(() => {
  stream.close()
})

async function handleModelChange(modelId: number) {
  selectedModelId.value = modelId
  if (!props.channel.chatId || !modelId) return
  try {
    const updated = await updateChat(props.channel.chatId, { modelId })
    chat.value = updated
  } catch (err: unknown) {
    ElMessage.error(describeApiError(err, '切换模型失败'))
  }
}

async function handleSend() {
  const text = draftText.value.trim()
  if (!text || running.value || sending.value) return
  if (!props.channel.chatId) {
    ElMessage.warning('该渠道尚未绑定对话会话')
    return
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
    const updated = await sendChatMessage(props.channel.chatId, {
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
  if (!props.channel.chatId) return
  try {
    await stopChat(props.channel.chatId)
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
    data-test="bot-channel-chat-drawer"
    @close="emit('close')"
  >
    <template #header>
      <div class="drawer-head">
        <div class="drawer-title-row">
          <span class="drawer-title">Bot 对话 · {{ channel.name }}</span>
          <span class="entity-chip entity-chip--wechat">微信通道</span>
          <el-tag
            :type="channel.status === 'connected' ? 'success' : 'info'"
            size="small"
          >
            {{ channel.status === 'connected' ? '已连接' : '离线' }}
          </el-tag>
        </div>
        <p class="drawer-subtitle">
          微信端来往消息、平台工具调用与当前抽屉实时共享同一对话（Chat #{{ channel.chatId }}）。
        </p>
      </div>
    </template>

    <div class="drawer-layout">
      <div ref="scrollContainer" class="drawer-messages">
        <div v-if="messages.length === 0" class="drawer-empty">
          <p class="drawer-empty-title">暂无对话消息</p>
          <p class="drawer-empty-desc">
            可在下方直接发送指令测试 Bot 工具（如查询工程、创建会话），或使用个人微信向该 Bot 发消息。
          </p>
        </div>
        <MessageView
          v-else
          :messages="messages"
          :capabilities="null"
          :stream-status="streamStatus"
          :running="running"
          display="chat"
          log-label="Bot 对话记录"
          :can-edit-user-messages="false"
          :can-regenerate-entry-id="null"
        />
      </div>

      <div class="drawer-composer">
        <div class="drawer-composer-toolbar">
          <el-select
            :model-value="selectedModelId || undefined"
            placeholder="选择模型"
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
          <span class="drawer-shortcut">Enter 发送 · Shift+Enter 换行</span>
        </div>

        <el-input
          v-model="draftText"
          type="textarea"
          :rows="3"
          resize="none"
          placeholder="输入消息与 Bot 渠道对话…"
          data-test="bot-chat-input"
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
            data-test="bot-chat-send"
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
  gap: 4px;
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

.drawer-subtitle {
  margin: 0;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
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
