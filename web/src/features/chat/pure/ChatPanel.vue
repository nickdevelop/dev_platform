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
import { useOpenTabsStore } from '../../../stores/openTabs'
import { useBottomFollow } from '../../../shared/scrollFollow'
import { UiDialog, UiButton, UiInput, UiAlert, UiEmpty } from '../../../shared/ui'
import MessageView from '../rendering/MessageView.vue'
import ChatComposer from './ChatComposer.vue'
import { reconnectChatStream } from './useChatStream'

const props = defineProps<{ chatId?: number }>()

const chats = useChatsStore()
const models = useModelsStore()
const openTabs = useOpenTabsStore()
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
  if (
    chats.selectedGroup?.defaultModelId &&
    usableModels.value.some((m) => m.id === chats.selectedGroup?.defaultModelId)
  ) {
    modelId.value = chats.selectedGroup.defaultModelId
    return
  }
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
  if (props.chatId === undefined && chats.selectedGroupId === null && chats.groups.length > 0) {
    chats.selectedGroupId = chats.groups[0].id
  }
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
    const targetGroupId =
      chats.selectedGroupId !== null
        ? chats.selectedGroupId
        : chats.groups[0]?.id !== undefined
          ? chats.groups[0].id
          : undefined
    const chat = await chats.createAndSend(content, images.value, modelId.value, targetGroupId)
    if (!chat) return
    text.value = ''
    images.value = []
    chats.draftModelId = modelId.value
    openTabs.promoteNewChat(chat.id, chat.title)
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
  if (props.chatId !== undefined) {
    openTabs.updateChatTitle(props.chatId, value)
  }
  renameOpen.value = false
}

async function confirmDelete() {
  if (deleting.value) return
  deleting.value = true
  const deletedId = props.chatId
  const ok = await chats.removeActive()
  deleting.value = false
  if (!ok) return
  deleteOpen.value = false
  if (deletedId !== undefined) {
    openTabs.closeTabByKey(`chat:${deletedId}`)
  }
  await router.push({ name: 'chat' })
  await chats.loadChats()
}

function reconnect() {
  if (props.chatId === undefined) return
  reconnectChatStream(chats, props.chatId)
}
</script>

<template>
  <section class="pure-chat flex flex-col h-full min-h-0 relative box-border" aria-label="Chat 对话" data-test="pure-chat">
    <header v-if="activeID !== null && active" class="pure-chat-header flex items-center justify-between gap-3 pb-2 border-b border-slate-200 dark:border-slate-800 shrink-0">
      <div class="pure-chat-title-row flex items-baseline gap-2.5 min-w-0">
        <h2 class="pure-chat-title m-0 text-base font-semibold truncate text-slate-800 dark:text-slate-100">{{ active?.title || '未命名对话' }}</h2>
        <span v-if="active?.modelName" class="pure-chat-model-name text-xs text-slate-400 font-mono shrink-0">{{ active.modelName }}</span>
      </div>
      <div class="pure-chat-header-actions flex items-center gap-1.5 shrink-0">
        <UiButton size="sm" variant="secondary" class="header-action" data-test="chat-rename-btn" @click="openRename">
          重命名
        </UiButton>
        <UiButton size="sm" variant="danger" class="header-action danger" data-test="chat-delete-btn" @click="deleteOpen = true">
          删除
        </UiButton>
      </div>
    </header>

    <UiAlert v-if="chats.loadError" variant="error" class="pure-chat-error" data-test="chat-load-error">
      {{ chats.loadError }}
      <RouterLink :to="{ name: 'chat' }" class="text-blue-600 underline ml-2">返回对话列表</RouterLink>
    </UiAlert>

    <template v-else>
      <div
        class="pure-chat-scroll flex-1 min-h-0 overflow-y-auto overflow-x-hidden"
        data-test="chat-scroll"
        ref="scroll"
        @scroll.passive="onScroll"
      >
        <div v-if="activeID === null" class="chat-welcome max-w-lg mx-auto mt-[12vh] text-center text-slate-500">
          <UiEmpty
            :title="chats.selectedGroup ? `与「${chats.selectedGroup.name}」开始新对话` : '开始新的对话'"
            :description="chats.selectedGroup ? '输入消息即可开始交流，助手的系统提示词将自动注入到模型上下文中。' : '请从左侧选择一个助手发起对话。'"
            :icon="chats.selectedGroup?.icon || '💬'"
          >
            <template v-if="!hasModels" #description>
              <p class="chat-welcome-warning text-xs text-amber-600 dark:text-amber-400">
                尚未配置可用模型，请先到
                <RouterLink :to="{ name: 'settings-models' }" class="underline font-medium">设置 → 模型管理</RouterLink>
                添加并启用模型。
              </p>
            </template>
          </UiEmpty>
        </div>

        <p v-else-if="!active" class="history-note text-xs text-slate-400 p-3 text-center" role="status">正在加载对话…</p>

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

        <p v-if="truncated" class="history-note text-xs text-slate-400 p-2 text-center" data-test="chat-truncated">
          仅显示最近的消息记录。
        </p>

        <UiButton
          v-if="streamBroken"
          size="sm"
          variant="secondary"
          class="reconnect-btn mx-auto my-2 block"
          data-test="chat-reconnect"
          @click="reconnect"
        >
          重新连接事件流
        </UiButton>
      </div>

      <UiButton
        v-if="jump"
        size="sm"
        variant="secondary"
        class="jump-latest absolute right-4 bottom-32 z-10 shadow-md"
        data-test="chat-jump-latest"
        @click="follow.scrollToBottom(true)"
      >
        ↓ 滚动到最新消息
      </UiButton>

      <UiAlert v-if="streamBanner" variant="warning" class="stream-banner" role="status" data-test="chat-stream-status">
        {{ streamBanner }}
      </UiAlert>
      <UiAlert v-if="minModelNotice" variant="error" class="chat-action-error">
        {{ minModelNotice }}
      </UiAlert>
      <UiAlert v-else-if="invalidModel" variant="error" class="chat-action-error">
        当前模型不可用，请选择其他模型。
      </UiAlert>
      <UiAlert v-if="chats.actionError" variant="error" class="chat-action-error">
        {{ chats.actionError }}
      </UiAlert>
      <UiAlert v-if="chats.sendError" variant="error" class="chat-action-error" data-test="chat-send-error">
        {{ chats.sendError }}
      </UiAlert>

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

    <UiDialog v-if="renameOpen" title="重命名对话" width-class="w-full max-w-md" @close="renameOpen = false">
      <form class="chat-dialog-form space-y-4" @submit.prevent="submitRename">
        <UiAlert v-if="renameError" variant="error" class="chat-dialog-error">
          {{ renameError }}
        </UiAlert>
        <div class="space-y-1.5">
          <label for="chat-title" class="text-xs font-semibold text-slate-700 dark:text-slate-300">标题</label>
          <UiInput id="chat-title" v-model="renameTitle" maxlength="120" autofocus data-test="chat-title-input" block />
        </div>
        <div class="chat-dialog-actions flex items-center justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
          <UiButton type="button" variant="secondary" class="secondary" @click="renameOpen = false">取消</UiButton>
          <UiButton type="submit" variant="primary" class="primary">保存</UiButton>
        </div>
      </form>
    </UiDialog>

    <UiDialog v-if="deleteOpen" title="删除对话" width-class="w-full max-w-md" @close="deleteOpen = false">
      <div class="chat-dialog-form space-y-4">
        <p class="text-xs text-slate-600 dark:text-slate-300 leading-relaxed">删除后，该对话及其全部消息将从平台中移除，且无法恢复。</p>
        <div class="chat-dialog-actions flex items-center justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
          <UiButton type="button" variant="secondary" class="secondary" @click="deleteOpen = false">取消</UiButton>
          <UiButton
            type="button"
            variant="danger"
            class="danger"
            :disabled="deleting"
            :loading="deleting"
            data-test="chat-delete-confirm"
            @click="confirmDelete"
          >
            {{ deleting ? '删除中…' : '删除' }}
          </UiButton>
        </div>
      </div>
    </UiDialog>
  </section>
</template>

<style scoped>
.pure-chat {
  padding: calc(var(--space-unit) * 1);
}
</style>