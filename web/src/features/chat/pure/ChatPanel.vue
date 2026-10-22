<script setup lang="ts">
// Pure-chat conversation pane. It is the counterpart of SessionChatPanel but
// carries only chat concerns: transcript, stream status, model, composer,
// rename/delete, stop, regenerate and edit-and-resend. No projects, agent
// backends, tools, approvals, file tree or activity pane are involved.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import type { ImageAttachment } from '../../../api/messageTypes'
import type { ChatEntry } from '../state/entry'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { useBottomFollow } from '../../../shared/scrollFollow'
import AppDialog from '../../../shared/components/AppDialog.vue'
import MessageView from '../rendering/MessageView.vue'
import ChatComposer from './ChatComposer.vue'
import { reconnectChatStream } from './useChatStream'

const props = defineProps<{ chatId?: number }>()

const chats = useChatsStore()
const models = useModelsStore()
const router = useRouter()

const text = ref('')
const images = ref<ImageAttachment[]>([])
const modelId = ref<number | null>(null)
const editingMessageId = ref<number | null>(null)
const minModelNotice = ref('')

const renameOpen = ref(false)
const renameTitle = ref('')
const renameError = ref('')
const deleteOpen = ref(false)
const deleting = ref(false)

const scroll = ref<HTMLElement | null>(null)

const active = computed(() => chats.active)
const activeID = computed(() => props.chatId ?? null)
const messages = computed<ChatEntry[]>(() =>
  activeID.value === null ? [] : chats.messagesFor(activeID.value),
)
const running = computed(() => (activeID.value === null ? false : chats.runningFor(activeID.value)))
const streamStatus = computed(() =>
  activeID.value === null ? 'idle' : chats.statusFor(activeID.value),
)
const truncated = computed(() =>
  activeID.value === null ? false : chats.truncatedFor(activeID.value),
)
const streamBroken = computed(
  () => streamStatus.value === 'reconnecting' || streamStatus.value === 'error',
)
const isEditing = computed(() => editingMessageId.value !== null)

const usableModels = computed(() =>
  models.services.flatMap((service) =>
    service.models
      .filter((model) => model.usable && model.supportsText)
      .map((model) => ({
        id: model.id,
        name: model.name || model.modelId,
        supportsImage: model.supportsImage,
      })),
  ),
)
const hasModels = computed(() => usableModels.value.length > 0)
const selectedModel = computed(() => usableModels.value.find((model) => model.id === modelId.value) ?? null)
const supportsImage = computed(() => selectedModel.value?.supportsImage === true)
const modelAvailable = computed(() => modelId.value !== null && selectedModel.value !== null)
const invalidModel = computed(() => hasModels.value && !modelAvailable.value)
const modelFallbackLabel = computed(() => active.value?.modelName ?? '')

const signature = computed(() => {
  let open = 0
  for (const entry of messages.value) if (entry.streaming) open += entry.text.length
  return `${activeID.value ?? 'new'}:${messages.value.length}:${open}`
})
const follow = useBottomFollow(scroll, signature)
const onScroll = follow.onScroll
const jump = follow.showJump

const lastAssistantEntryId = computed(() => {
  const id = chats.lastAssistantMessageId
  return id === null ? null : `msg:${id}`
})

const streamBanner = computed(() => {
  if (streamStatus.value === 'reconnecting') return '连接中断，正在重连…'
  if (streamStatus.value === 'error') return '事件流已断开，无法接收新消息。'
  return ''
})

function ensureDraftModel() {
  if (props.chatId !== undefined) return
  if (modelId.value !== null && usableModels.value.some((model) => model.id === modelId.value)) return
  const stored = chats.draftModelId
  if (stored !== null && usableModels.value.some((model) => model.id === stored)) {
    modelId.value = stored
    return
  }
  modelId.value = usableModels.value[0]?.id ?? null
}

async function bootstrap() {
  editingMessageId.value = null
  text.value = ''
  images.value = []
  minModelNotice.value = ''
  chats.clearErrors()
  if (props.chatId === undefined) chats.startNewChat()
  if (models.services.length === 0) await models.load()
  if (props.chatId === undefined) {
    ensureDraftModel()
    return
  }
  await chats.openChat(props.chatId)
  modelId.value = chats.active?.modelId ?? null
  await nextTick()
  follow.scrollToBottom(false)
}

onMounted(() => {
  void bootstrap()
})

onBeforeUnmount(() => {
  // The router replaces a keyed chat panel when switching conversations. If
  // the next panel has already made its conversation active, leave that stream
  // alone; only release the stream this panel actually owned.
  if (props.chatId === undefined) {
    if (chats.activeId === null) chats.closeActive()
    return
  }
  if (chats.activeId === props.chatId) chats.closeActive()
})

watch(
  () => props.chatId,
  () => {
    void bootstrap()
  },
)

watch(usableModels, () => {
  if (props.chatId === undefined) ensureDraftModel()
})

watch(
  () => chats.draftNonce,
  () => {
    if (props.chatId !== undefined) return
    editingMessageId.value = null
    text.value = ''
    images.value = []
  },
)

watch(
  () => chats.active?.modelId,
  (value) => {
    if (props.chatId === undefined) return
    if (value !== undefined && value !== null) modelId.value = value
  },
)

watch(
  () => chats.closedReason,
  (reason) => {
    if (reason !== 'deleted') return
    if (props.chatId !== undefined) void router.push({ name: 'chat' })
  },
)

watch(
  () => messages.value.length,
  () => {
    if (follow.pinned.value) void nextTick(() => follow.scrollToBottom(false))
  },
)

async function onModelChange(value: number) {
  const previous = modelId.value
  modelId.value = value
  minModelNotice.value = ''
  if (props.chatId === undefined) {
    chats.draftModelId = value
    return
  }
  const ok = await chats.setModel(value)
  if (!ok) {
    modelId.value = previous
    minModelNotice.value = chats.actionError || '模型切换失败，请重试。'
  }
}

async function submit() {
  if (running.value) {
    await chats.stop()
    return
  }
  minModelNotice.value = ''
  const content = text.value.trim()
  if (content === '' && images.value.length === 0) return
  if (!modelAvailable.value || modelId.value === null) {
    minModelNotice.value = '当前模型不可用，请先选择模型。'
    return
  }

  if (editingMessageId.value !== null) {
    const ok = await chats.editAndResend(editingMessageId.value, content, images.value)
    if (!ok) return
    editingMessageId.value = null
    text.value = ''
    images.value = []
    return
  }

  if (props.chatId === undefined) {
    const chat = await chats.createAndSend(content, images.value, modelId.value)
    if (!chat) return
    text.value = ''
    images.value = []
    chats.draftModelId = modelId.value
    await router.push({ name: 'chat-detail', params: { chatId: String(chat.id) } })
    return
  }

  const ok = await chats.sendMessage(content, images.value, modelId.value)
  if (!ok) return
  text.value = ''
  images.value = []
  await nextTick()
  follow.scrollToBottom(false)
}

function beginEdit(entry: ChatEntry) {
  const match = /^msg:(\d+)$/.exec(entry.id)
  if (!match || entry.role !== 'user') return
  editingMessageId.value = Number(match[1])
  text.value = entry.text
  images.value = entry.images ? [...entry.images] : []
  minModelNotice.value = ''
  void nextTick(() => follow.scrollToBottom(false))
}

function cancelEdit() {
  editingMessageId.value = null
  text.value = ''
  images.value = []
}

async function regenerate() {
  editingMessageId.value = null
  await chats.regenerate()
  await nextTick()
  follow.scrollToBottom(false)
}

function openRename() {
  renameTitle.value = active.value?.title ?? ''
  renameError.value = ''
  renameOpen.value = true
}

async function submitRename() {
  const value = renameTitle.value.trim()
  if (value === '') {
    renameError.value = '标题不能为空。'
    return
  }
  const ok = await chats.rename(value)
  if (!ok) {
    renameError.value = chats.actionError || '重命名失败。'
    return
  }
  renameOpen.value = false
}

async function confirmDelete() {
  if (deleting.value) return
  deleting.value = true
  const ok = await chats.removeActive()
  deleting.value = false
  if (!ok) return
  deleteOpen.value = false
  await router.push({ name: 'chat' })
  await chats.loadChats()
}

function reconnect() {
  if (props.chatId === undefined) return
  reconnectChatStream(chats, props.chatId)
}
</script>

<template>
  <section class="pure-chat" aria-label="Chat 对话" data-test="pure-chat">
    <header v-if="activeID !== null && active" class="pure-chat-header">
      <div class="pure-chat-title-row">
        <h2 class="pure-chat-title">{{ active?.title || '未命名对话' }}</h2>
        <span v-if="active?.modelName" class="pure-chat-model-name">{{ active.modelName }}</span>
      </div>
      <div class="pure-chat-header-actions">
        <button type="button" class="header-action" data-test="chat-rename-btn" @click="openRename">
          重命名
        </button>
        <button type="button" class="header-action danger" data-test="chat-delete-btn" @click="deleteOpen = true">
          删除
        </button>
      </div>
    </header>

    <p v-if="chats.loadError" class="pure-chat-error" role="alert" data-test="chat-load-error">
      {{ chats.loadError }}
      <RouterLink :to="{ name: 'chat' }">返回对话列表</RouterLink>
    </p>

    <template v-else>
      <div
        class="pure-chat-scroll"
        data-test="chat-scroll"
        ref="scroll"
        @scroll.passive="onScroll"
      >
        <div v-if="activeID === null" class="chat-welcome">
          <h2>开始新的对话</h2>
          <p>选择一个模型，然后输入消息。对话会保存在平台中，刷新后可继续。</p>
          <p v-if="!hasModels" class="chat-welcome-warning">
            尚未配置可用模型，请先到
            <RouterLink :to="{ name: 'settings-models' }">设置 → 模型管理</RouterLink>
            添加并启用模型。
          </p>
        </div>

        <p v-else-if="!active" class="history-note" role="status">正在加载对话…</p>

        <MessageView
          v-else
          :messages="messages"
          :capabilities="null"
          :stream-status="streamStatus"
          display="chat"
          log-label="Chat 消息"
          :can-edit-user-messages="!running"
          :can-regenerate-entry-id="lastAssistantEntryId"
          @edit="beginEdit"
          @regenerate="regenerate"
        />

        <p v-if="truncated" class="history-note" data-test="chat-truncated">
          仅显示最近的消息记录。
        </p>

        <button
          v-if="streamBroken"
          type="button"
          class="reconnect-btn"
          data-test="chat-reconnect"
          @click="reconnect"
        >
          重新连接事件流
        </button>
      </div>

      <button v-if="jump" type="button" class="jump-latest" data-test="chat-jump-latest" @click="follow.scrollToBottom(true)">
        ↓ 滚动到最新消息
      </button>

      <p v-if="streamBanner" class="stream-banner" role="status" data-test="chat-stream-status">
        {{ streamBanner }}
      </p>
      <p v-if="minModelNotice" class="chat-action-error" role="alert">{{ minModelNotice }}</p>
      <p v-else-if="invalidModel" class="chat-action-error" role="alert">
        当前模型不可用，请选择其他模型。
      </p>
      <p v-if="chats.actionError" class="chat-action-error" role="alert">{{ chats.actionError }}</p>
      <p v-if="chats.sendError" class="chat-action-error" role="alert" data-test="chat-send-error">
        {{ chats.sendError }}
      </p>

      <ChatComposer
        :text="text"
        :images="images"
        :model-id="modelId"
        :running="running"
        :disabled="!modelAvailable && hasModels"
        :has-models="hasModels"
        :supports-image="supportsImage"
        :editing="isEditing"
        :model-fallback-label="modelFallbackLabel"
        @update:text="text = $event"
        @update:images="images = $event"
        @update:model-id="onModelChange"
        @send="submit"
        @stop="chats.stop()"
        @cancel-edit="cancelEdit"
      />
    </template>

    <AppDialog v-if="renameOpen" title="重命名对话" @close="renameOpen = false">
      <form class="chat-dialog-form" @submit.prevent="submitRename">
        <p v-if="renameError" class="chat-dialog-error" role="alert">{{ renameError }}</p>
        <label for="chat-title">标题</label>
        <input id="chat-title" v-model="renameTitle" type="text" maxlength="120" autofocus data-test="chat-title-input" />
        <div class="chat-dialog-actions">
          <button type="button" class="secondary" @click="renameOpen = false">取消</button>
          <button type="submit" class="primary">保存</button>
        </div>
      </form>
    </AppDialog>

    <AppDialog v-if="deleteOpen" title="删除对话" @close="deleteOpen = false">
      <div class="chat-dialog-form">
        <p>删除后，该对话及其全部消息将从平台中移除，且无法恢复。</p>
        <div class="chat-dialog-actions">
          <button type="button" class="secondary" @click="deleteOpen = false">取消</button>
          <button type="button" class="danger" :disabled="deleting" data-test="chat-delete-confirm" @click="confirmDelete">
            {{ deleting ? '删除中…' : '删除' }}
          </button>
        </div>
      </div>
    </AppDialog>
  </section>
</template>

<style scoped>
.pure-chat {
  display: flex;
  flex-direction: column;
  padding: 0 1rem 1rem;
  gap: 0.5rem;
  height: 100%;
  min-height: 0;
  position: relative;
  box-sizing: border-box;
}

.pure-chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.pure-chat-title-row {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  min-width: 0;
}

.pure-chat-title {
  margin: 0;
  font-size: 1rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pure-chat-model-name {
  color: var(--color-text-muted);
  font-size: 0.75rem;
  flex-shrink: 0;
}

.pure-chat-header-actions {
  display: flex;
  gap: 0.35rem;
  flex-shrink: 0;
}

.header-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  padding: 0 0.65rem;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.header-action:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.header-action.danger {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.header-action.danger:hover {
  background: var(--color-danger-soft);
  border-color: var(--color-danger);
  color: var(--color-danger);
}

.pure-chat-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0;
}

.chat-welcome {
  max-width: 34rem;
  margin: 12vh auto 0;
  text-align: center;
  color: var(--color-text-muted);
}

.chat-welcome h2 {
  color: var(--color-text);
  margin-bottom: 0.4rem;
}

.chat-welcome-warning {
  margin-top: 1rem;
  color: var(--color-warning, #b45309);
}

.pure-chat-error,
.chat-action-error,
.stream-banner,
.history-note {
  margin: 0;
  padding: 0.4rem 0.9rem;
  font-size: 0.82rem;
}

.pure-chat-error,
.chat-action-error {
  color: var(--color-danger);
}

.stream-banner,
.history-note {
  color: var(--color-text-muted);
}

.reconnect-btn,
.jump-latest {
  margin: 0.4rem auto;
  display: block;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text);
  padding: 0.25rem 0.75rem;
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}

.jump-latest {
  position: absolute;
  right: 1rem;
  bottom: 8.5rem;
  z-index: 5;
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
}

.chat-dialog-form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.chat-dialog-form label {
  font-size: 0.85rem;
}

.chat-dialog-form input {
  padding: 0.45rem 0.6rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  font: inherit;
}

.chat-dialog-error {
  margin: 0;
  color: var(--color-danger);
  font-size: 0.82rem;
}

.chat-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.chat-dialog-actions button {
  padding: 0.4rem 0.8rem;
  border-radius: 6px;
  border: 1px solid var(--color-border);
  font: inherit;
  cursor: pointer;
}

.chat-dialog-actions .primary {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
}

.chat-dialog-actions .danger {
  background: var(--color-danger);
  border-color: var(--color-danger);
  color: #fff;
}
</style>
