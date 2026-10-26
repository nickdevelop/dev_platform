<script setup lang="ts">
// Conversation surface used inside the floating quick-assistant popover. It
// reuses the pure-chat renderer and composer but keeps an isolated store and
// stream, so the main Chat page can stay open on another conversation.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { ImageAttachment } from '../../../api/messageTypes'
import type { ChatEntry } from '../state/entry'
import { useModelsStore } from '../../../stores/models'
import { useQuickAssistantStore } from '../../../stores/quickAssistant'
import { useBottomFollow } from '../../../shared/scrollFollow'
import MessageView from '../rendering/MessageView.vue'
import ChatComposer from '../pure/ChatComposer.vue'

const emit = defineEmits<{
  close: []
  headerPointerdown: [event: PointerEvent]
  headerPointermove: [event: PointerEvent]
  headerPointerup: [event: PointerEvent]
  headerPointercancel: [event: PointerEvent]
}>()

const quick = useQuickAssistantStore()
const models = useModelsStore()

const text = ref('')
const images = ref<ImageAttachment[]>([])
const notice = ref('')
const scroll = ref<HTMLElement | null>(null)

const usableModels = computed(() =>
  (models.services ?? []).flatMap((service) =>
    (service.models ?? [])
      .filter((model) => model.usable && model.supportsText)
      .map((model) => ({
        id: model.id,
        name: model.name || model.modelId,
        supportsImage: model.supportsImage,
      })),
  ),
)
const recentChats = computed(() => (Array.isArray(quick.recentChats) ? quick.recentChats : []))
const hasModels = computed(() => usableModels.value.length > 0)
const selectedModel = computed(() =>
  usableModels.value.find((model) => model.id === quick.modelId) ?? null,
)
const supportsImage = computed(() => selectedModel.value?.supportsImage === true)
const modelAvailable = computed(() => quick.modelId !== null && selectedModel.value !== null)
const invalidModel = computed(() => hasModels.value && !modelAvailable.value)
const messages = computed<ChatEntry[]>(() => quick.messages)

const signature = computed(() => {
  let open = 0
  for (const entry of messages.value) if (entry.streaming) open += entry.text.length
  return `${quick.chatId ?? 'new'}:${messages.value.length}:${open}`
})
const follow = useBottomFollow(scroll, signature)

const streamBanner = computed(() => {
  if (quick.streamStatus === 'reconnecting') return '连接中断，正在重连…'
  if (quick.streamStatus === 'error') return '事件流已断开，无法接收新消息。'
  return ''
})
const streamBroken = computed(
  () => quick.streamStatus === 'reconnecting' || quick.streamStatus === 'error',
)

const currentAssistant = computed(() => quick.currentAssistant)
const assistantName = computed(() => currentAssistant.value?.name || '快捷助手')
const assistantIcon = computed(() => {
  if (currentAssistant.value?.icon && currentAssistant.value.icon.trim()) {
    return currentAssistant.value.icon.trim()
  }
  const trimmed = assistantName.value.trim()
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : '助'
})

function ensureModel() {
  if (
    currentAssistant.value?.defaultModelId &&
    usableModels.value.some((model) => model.id === currentAssistant.value?.defaultModelId)
  ) {
    quick.modelId = currentAssistant.value.defaultModelId
    return
  }
  if (quick.modelId !== null && usableModels.value.some((model) => model.id === quick.modelId)) return
  // Keep a persisted conversation model when the model list simply has not
  // loaded (or failed to load); only fall back when choices are available.
  if (quick.modelId !== null && usableModels.value.length === 0) return
  quick.modelId = usableModels.value[0]?.id ?? null
}

async function focusComposer() {
  await nextTick()
  const input = document.getElementById('quick-assistant-message') as HTMLElement | null
  input?.focus()
}

onMounted(async () => {
  if (models.services.length === 0) await models.load()
  await quick.restoreOrOpenLatest()
  ensureModel()
  await nextTick()
  follow.scrollToBottom(false)
  await focusComposer()
})

onBeforeUnmount(() => {
  quick.close()
})

watch(usableModels, () => {
  ensureModel()
})

watch(
  () => messages.value.length,
  () => {
    if (follow.pinned.value) void nextTick(() => follow.scrollToBottom(false))
  },
)

async function onModelChange(value: number) {
  notice.value = ''
  const ok = await quick.setModel(value)
  if (!ok) notice.value = quick.actionError || '模型切换失败，请重试。'
}

async function submit() {
  if (quick.running) {
    await quick.stop()
    return
  }
  notice.value = ''
  const content = text.value.trim()
  if (content === '' && images.value.length === 0) return
  if (!modelAvailable.value || quick.modelId === null) {
    notice.value = '当前模型不可用，请先选择模型。'
    return
  }
  const ok = await quick.send(content, images.value, quick.modelId)
  if (!ok) return
  text.value = ''
  images.value = []
  await nextTick()
  follow.scrollToBottom(false)
}

async function startNewChat() {
  if (quick.running) return
  quick.newChat()
  text.value = ''
  images.value = []
  notice.value = ''
  ensureModel()
  await nextTick()
  follow.scrollToBottom(false)
  await focusComposer()
}

const currentHistoryId = computed(() => {
  if (quick.chatId !== null && recentChats.value.some((c) => c.id === quick.chatId)) {
    return quick.chatId
  }
  return ''
})

async function onSelectHistory(event: Event) {
  const target = event.target as HTMLSelectElement
  const val = Number(target.value)
  if (!val || Number.isNaN(val) || val === quick.chatId) return
  notice.value = ''
  text.value = ''
  images.value = []
  await quick.openChat(val)
  ensureModel()
  await nextTick()
  follow.scrollToBottom(false)
  await focusComposer()
}

function reconnect() {
  quick.reconnect()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  event.stopPropagation()
  emit('close')
}
</script>

<template>
  <section
    class="quick-assistant"
    role="dialog"
    aria-label="快捷助手"
    data-test="quick-assistant-panel"
    @keydown="onKeydown"
  >
    <header
      class="quick-assistant-header"
      @pointerdown="emit('headerPointerdown', $event)"
      @pointermove="emit('headerPointermove', $event)"
      @pointerup="emit('headerPointerup', $event)"
      @pointercancel="emit('headerPointercancel', $event)"
    >
      <div class="quick-assistant-heading">
        <h2>
          <span class="assistant-mini-badge">{{ assistantIcon }}</span>
          {{ assistantName }}
        </h2>
        <span class="quick-assistant-subtitle">对话保存在 Chat · {{ assistantName }}</span>
      </div>
      <div class="quick-assistant-actions">
        <div class="history-select-wrapper" @pointerdown.stop>
          <select
            class="header-action history-select"
            data-test="quick-assistant-history-select"
            :value="currentHistoryId"
            :disabled="quick.running || quick.loading || recentChats.length === 0"
            :title="currentHistoryId ? '选择最近对话' : (recentChats.length === 0 ? '暂无历史对话' : '选择最近对话')"
            aria-label="最近对话"
            @change="onSelectHistory"
          >
            <option v-if="recentChats.length === 0" value="" disabled>
              暂无历史对话
            </option>
            <option v-else value="" :disabled="quick.chatId !== null">
              最近对话…
            </option>
            <option
              v-for="item in recentChats"
              :key="item.id"
              :value="item.id"
              :title="item.title || '新对话'"
            >
              {{ item.title || '新对话' }}
            </option>
          </select>
        </div>
        <button
          type="button"
          class="header-action"
          data-test="quick-assistant-new"
          :disabled="quick.running || quick.loading"
          :title="quick.running ? '当前回复生成中，请先停止' : '开始新对话'"
          @click="startNewChat"
        >
          新对话
        </button>
        <button
          type="button"
          class="header-action close"
          aria-label="关闭快捷助手"
          title="关闭"
          data-test="quick-assistant-close"
          @click="emit('close')"
        >
          ✕
        </button>
      </div>
    </header>

    <p v-if="quick.error" class="quick-assistant-error" role="alert" data-test="quick-assistant-error">
      {{ quick.error }}
    </p>

    <div ref="scroll" class="quick-assistant-scroll" data-test="quick-assistant-scroll" @scroll.passive="follow.onScroll">
      <div v-if="quick.chatId === null && !quick.loading" class="quick-assistant-welcome">
        <h3>你好，我是{{ assistantName }}</h3>
        <p>有什么可以帮你？对话会自动保存到 Chat 中。</p>
        <p v-if="!hasModels" class="quick-assistant-warning">
          尚未配置可用模型，请先到「设置 → 模型」添加并启用模型。
        </p>
      </div>

      <p v-else-if="quick.loading && quick.chatId === null" class="quick-assistant-note" role="status">
        正在加载对话…
      </p>

      <p v-else-if="quick.chatId !== null && !quick.chat" class="quick-assistant-note" role="status">
        正在加载对话…
      </p>

      <MessageView
        v-else
        :messages="messages"
        :capabilities="null"
        :stream-status="quick.streamStatus"
        display="chat"
        log-label="快捷助手消息"
        :can-edit-user-messages="false"
        :can-regenerate-entry-id="null"
      />

      <p v-if="quick.truncated" class="quick-assistant-note" data-test="quick-assistant-truncated">
        仅显示最近的消息记录。
      </p>

      <button
        v-if="streamBroken"
        type="button"
        class="quick-assistant-reconnect"
        data-test="quick-assistant-reconnect"
        @click="reconnect"
      >
        重新连接事件流
      </button>
    </div>

    <p v-if="streamBanner" class="quick-assistant-note stream-banner" role="status">
      {{ streamBanner }}
    </p>
    <p v-if="notice" class="quick-assistant-error" role="alert">{{ notice }}</p>
    <p v-else-if="invalidModel" class="quick-assistant-error" role="alert">
      当前模型不可用，请选择其他模型。
    </p>
    <p v-if="quick.actionError" class="quick-assistant-error" role="alert">{{ quick.actionError }}</p>
    <p v-if="quick.sendError" class="quick-assistant-error" role="alert" data-test="quick-assistant-send-error">
      {{ quick.sendError }}
    </p>

    <ChatComposer
      :text="text"
      :images="images"
      :model-id="quick.modelId"
      :running="quick.running"
      :disabled="quick.loading || (!modelAvailable && hasModels)"
      :has-models="hasModels"
      :supports-image="supportsImage"
      compact
      input-id="quick-assistant-message"
      placeholder="给快捷助手发送消息"
      :model-fallback-label="quick.chat?.modelName ?? ''"
      @update:text="text = $event"
      @update:images="images = $event"
      @update:model-id="onModelChange"
      @send="submit"
      @stop="quick.stop()"
    />
  </section>
</template>

<style scoped>
.quick-assistant {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  height: 100%;
  min-height: 0;
  padding: 0.75rem;
  box-sizing: border-box;
  background: var(--color-surface);
  color: var(--color-text);
}

.quick-assistant-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
  cursor: grab;
  user-select: none;
  touch-action: none;
}

.quick-assistant-header:active {
  cursor: grabbing;
}

.quick-assistant-heading {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.quick-assistant-heading h2 {
  margin: 0;
  font-size: 1rem;
  display: flex;
  align-items: center;
}

.assistant-mini-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  background: var(--color-primary-soft, rgba(59, 130, 246, 0.12));
  color: var(--color-primary);
  font-size: 0.8rem;
  font-weight: 600;
  margin-right: 6px;
  flex-shrink: 0;
}

.quick-assistant-subtitle {
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.quick-assistant-actions {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex-shrink: 0;
}

.header-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  padding: 0 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.header-action:hover:not(:disabled) {
  background: var(--color-surface-muted);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.header-action:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.header-action.close {
  padding: 0 0.45rem;
  color: var(--color-text-muted);
}

.history-select-wrapper {
  display: inline-flex;
  align-items: center;
}

.history-select {
  max-width: 130px;
  outline: none;
  appearance: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23888888' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 6px center;
  background-size: 10px;
  padding-right: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
  overflow: hidden;
}

.history-select:focus {
  border-color: var(--color-primary);
}

.quick-assistant-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

.quick-assistant-welcome {
  margin: 2.5rem 0.5rem 0;
  color: var(--color-text-muted);
  text-align: center;
}

.quick-assistant-welcome h3 {
  margin: 0 0 0.4rem;
  color: var(--color-text);
  font-size: 1rem;
}

.quick-assistant-welcome p {
  margin: 0 0 0.4rem;
  font-size: 0.85rem;
  line-height: 1.5;
}

.quick-assistant-warning {
  color: var(--color-warning, #b45309);
}

.quick-assistant-note,
.quick-assistant-error {
  margin: 0;
  font-size: 0.8rem;
}

.quick-assistant-note {
  color: var(--color-text-muted);
  padding: 0.15rem 0.25rem;
}

.quick-assistant-error {
  color: var(--color-danger);
}

.stream-banner {
  text-align: center;
}

.quick-assistant-reconnect {
  display: block;
  margin: 0.5rem auto;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text);
  padding: 0.25rem 0.75rem;
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}
</style>
