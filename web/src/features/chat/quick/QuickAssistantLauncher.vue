<script setup lang="ts">
// Floating entry point for the quick assistant. The button is draggable and its
// position is remembered; the popover opens over the current page without a
// route change and sizes itself to 480x600 whenever the viewport allows.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { placePanel, useDraggablePosition } from './useDraggablePosition'
import QuickAssistantPanel from './QuickAssistantPanel.vue'
import { useChatsStore } from '../../../stores/chats'
import { useQuickAssistantStore } from '../../../stores/quickAssistant'
import type { ChatGroup } from '../../../api/chats'

const chats = useChatsStore()
const quick = useQuickAssistantStore()

const open = ref(false)
const buttonRef = ref<HTMLButtonElement | null>(null)
const {
  position,
  buttonSize,
  viewport,
  dragging,
  buttonStyle,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  consumeClickSuppression,
} = useDraggablePosition(buttonRef)

onMounted(() => {
  if (chats.groups.length === 0) {
    void chats.loadGroups()
  }
})

const floatingAssistants = computed(() => chats.groups.filter((g) => g.showInFloat))
const activeAssistant = computed(() => {
  if (quick.currentAssistant) return quick.currentAssistant
  return floatingAssistants.value[0] ?? chats.groups[0] ?? null
})

function getAssistantIcon(assistant?: ChatGroup | null): string {
  if (!assistant) return ''
  if (assistant.icon && assistant.icon.trim()) return assistant.icon.trim()
  const trimmed = assistant.name?.trim() ?? ''
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : '助'
}

const panelStyle = computed(() => {
  if (!position.value) return undefined
  const placed = placePanel(
    {
      left: position.value.left,
      top: position.value.top,
      width: buttonSize.value.width,
      height: buttonSize.value.height,
    },
    viewport.value,
  )
  return {
    left: `${placed.left}px`,
    top: `${placed.top}px`,
    width: `${placed.width}px`,
    height: `${placed.height}px`,
  }
})

function toggle() {
  if (consumeClickSuppression()) return
  if (!open.value && activeAssistant.value) {
    quick.openAssistant(activeAssistant.value)
  }
  open.value = !open.value
}

function selectAssistant(assistant: ChatGroup) {
  if (consumeClickSuppression()) return
  quick.openAssistant(assistant)
  open.value = true
}

function close() {
  open.value = false
}

watch(open, (value, previous) => {
  if (value || !previous) return
  void nextTick(() => buttonRef.value?.focus())
})
</script>

<template>
  <div class="quick-assistant-launcher">
    <!-- 多助手浮窗按钮组 -->
    <div
      v-if="floatingAssistants.length > 1"
      class="multi-assistant-bar"
      :style="buttonStyle"
    >
      <button
        v-for="(assistant, index) in floatingAssistants"
        :key="assistant.id"
        :ref="index === 0 ? (el) => { buttonRef = el as HTMLButtonElement } : undefined"
        type="button"
        class="quick-assistant-button multi-item"
        :class="{ dragging, active: quick.currentAssistant?.id === assistant.id && open }"
        :aria-label="`助手 · ${assistant.name}`"
        :title="`助手 · ${assistant.name}（点击新建对话）`"
        :data-test="index === 0 ? 'quick-assistant-button' : undefined"
        @pointerdown="index === 0 ? onPointerDown($event) : undefined"
        @pointermove="index === 0 ? onPointerMove($event) : undefined"
        @pointerup="index === 0 ? onPointerUp($event) : undefined"
        @pointercancel="index === 0 ? onPointerCancel($event) : undefined"
        @click="selectAssistant(assistant)"
      >
        <span class="assistant-icon-text">{{ getAssistantIcon(assistant) }}</span>
      </button>
    </div>

    <!-- 单个助手或默认快捷助手按钮 -->
    <button
      v-else
      ref="buttonRef"
      type="button"
      class="quick-assistant-button"
      :class="{ dragging }"
      :style="buttonStyle"
      :aria-label="activeAssistant ? `助手 · ${activeAssistant.name}` : '快捷助手'"
      :title="activeAssistant ? `助手 · ${activeAssistant.name}（点击新建对话）` : '快捷助手'"
      aria-controls="quick-assistant-popover"
      :aria-expanded="open.toString()"
      data-test="quick-assistant-button"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerCancel"
      @click="toggle"
    >
      <span v-if="activeAssistant" class="assistant-icon-text">
        {{ getAssistantIcon(activeAssistant) }}
      </span>
      <svg v-else class="quick-assistant-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path
          d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </button>

    <div
      v-if="open && panelStyle"
      id="quick-assistant-popover"
      class="quick-assistant-popover"
      :style="panelStyle"
      data-test="quick-assistant-popover"
    >
      <QuickAssistantPanel @close="close" />
    </div>
  </div>
</template>

<style scoped>
.multi-assistant-bar {
  position: fixed;
  top: 50px;
  right: 50px;
  z-index: 61;
  display: inline-flex;
  flex-direction: column;
  gap: 8px;
  user-select: none;
}

.quick-assistant-button {
  position: fixed;
  top: 50px;
  right: 50px;
  z-index: 61;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  border: 1px solid var(--color-primary);
  border-radius: 50%;
  background: var(--color-primary);
  color: #fff;
  cursor: grab;
  user-select: none;
  touch-action: none;
  box-shadow: 0 6px 20px rgba(15, 23, 42, 0.22);
  transition: transform 0.15s ease, box-shadow 0.15s ease, background-color 0.15s ease;
}

.multi-assistant-bar .quick-assistant-button {
  position: static;
}

.quick-assistant-button:hover {
  transform: scale(1.06);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.3);
}

.quick-assistant-button.dragging {
  cursor: grabbing;
  transform: scale(1.08);
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.35);
}

.assistant-icon-text {
  font-size: 1.25rem;
  font-weight: 600;
  line-height: 1;
}

.quick-assistant-icon {
  width: 22px;
  height: 22px;
}

.quick-assistant-popover {
  position: fixed;
  z-index: 62;
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.28);
  border-radius: calc(var(--radius) * 1.5);
  overflow: hidden;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
}
</style>
