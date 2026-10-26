<script setup lang="ts">
// Floating entry point for the quick assistant. The toolbar is a horizontal,
// square-button toolbar with default semi-transparency that fully reveals on hover.
// The toolbar and the panel positions are completely decoupled: dragging the toolbar
// leaves the panel where it is, and the panel header allows dragging the dialog
// anywhere within the viewport.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useDraggablePosition, useDraggablePanel } from './useDraggablePosition'
import QuickAssistantPanel from './QuickAssistantPanel.vue'
import { useChatsStore } from '../../../stores/chats'
import { useQuickAssistantStore } from '../../../stores/quickAssistant'
import type { ChatGroup } from '../../../api/chats'

const chats = useChatsStore()
const quick = useQuickAssistantStore()

const open = ref(false)
const toolbarRef = ref<HTMLElement | null>(null)
const {
  dragging,
  buttonStyle,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  consumeClickSuppression,
} = useDraggablePosition(toolbarRef)

const {
  panelStyle,
  draggingPanel,
  onPanelPointerDown,
  onPanelPointerMove,
  onPanelPointerUp,
  onPanelPointerCancel,
} = useDraggablePanel()

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

function toggle() {
  if (consumeClickSuppression()) return
  if (!open.value && activeAssistant.value) {
    quick.openAssistant(activeAssistant.value)
  }
  open.value = !open.value
}

function selectAssistant(assistant: ChatGroup) {
  if (consumeClickSuppression()) return
  if (open.value && quick.currentAssistant?.id === assistant.id) {
    open.value = false
    return
  }
  quick.openAssistant(assistant)
  open.value = true
}

function close() {
  open.value = false
}

watch(open, (value, previous) => {
  if (value || !previous) return
  void nextTick(() => toolbarRef.value?.querySelector<HTMLElement>('button')?.focus())
})
</script>

<template>
  <div class="quick-assistant-launcher">
    <!-- 横向漂浮工具栏（方形按钮组，默认半透明，悬浮完全显示） -->
    <div
      ref="toolbarRef"
      class="quick-assistant-toolbar"
      :class="{
        dragging,
        open,
        'has-multiple': floatingAssistants.length > 1,
      }"
      :style="buttonStyle"
      role="toolbar"
      aria-label="快捷助手工具栏"
      data-test="quick-assistant-toolbar"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerCancel"
    >
      <!-- 多助手时的各个方形按钮 -->
      <template v-if="floatingAssistants.length > 1">
        <button
          v-for="(assistant, index) in floatingAssistants"
          :key="assistant.id"
          type="button"
          class="quick-assistant-button square-btn"
          :class="{ active: quick.currentAssistant?.id === assistant.id && open }"
          :aria-label="`助手 · ${assistant.name}`"
          :title="`助手 · ${assistant.name}（点击新建对话）`"
          :data-test="index === 0 ? 'quick-assistant-button' : undefined"
          @click="selectAssistant(assistant)"
        >
          <span class="assistant-icon-text">{{ getAssistantIcon(assistant) }}</span>
        </button>
      </template>

      <!-- 单助手或默认快捷助手按钮 -->
      <button
        v-else
        type="button"
        class="quick-assistant-button square-btn"
        :class="{ active: open }"
        :aria-label="activeAssistant ? `助手 · ${activeAssistant.name}` : '快捷助手'"
        :title="activeAssistant ? `助手 · ${activeAssistant.name}（点击打开/关闭对话）` : '快捷助手'"
        aria-controls="quick-assistant-popover"
        :aria-expanded="open.toString()"
        data-test="quick-assistant-button"
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
    </div>

    <!-- 弹窗：位置与工具栏完全解绑，支持随意拖拽 -->
    <div
      v-if="open && panelStyle"
      id="quick-assistant-popover"
      class="quick-assistant-popover"
      :class="{ 'dragging-panel': draggingPanel }"
      :style="panelStyle"
      data-test="quick-assistant-popover"
    >
      <QuickAssistantPanel
        @close="close"
        @header-pointerdown="onPanelPointerDown"
        @header-pointermove="onPanelPointerMove"
        @header-pointerup="onPanelPointerUp"
        @header-pointercancel="onPanelPointerCancel"
      />
    </div>
  </div>
</template>

<style scoped>
.quick-assistant-toolbar {
  position: fixed;
  top: 50px;
  right: 50px;
  z-index: 61;
  display: inline-flex;
  flex-direction: row;
  align-items: center;
  gap: 4px;
  padding: 4px;
  border-radius: var(--radius, 6px);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.16);
  opacity: 0.55;
  backdrop-filter: blur(8px);
  transition: opacity 0.2s ease, box-shadow 0.15s ease, transform 0.15s ease;
  user-select: none;
  touch-action: none;
}

.quick-assistant-toolbar:hover,
.quick-assistant-toolbar.dragging,
.quick-assistant-toolbar.open {
  opacity: 1;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.26);
}

.quick-assistant-toolbar.dragging,
.quick-assistant-toolbar.dragging .quick-assistant-button {
  cursor: grabbing;
  transform: scale(1.02);
}

.quick-assistant-button.square-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--radius, 4px);
  background: var(--color-primary);
  color: #fff;
  cursor: pointer;
  user-select: none;
  touch-action: none;
  transition: transform 0.15s ease, filter 0.15s ease, box-shadow 0.15s ease;
}

.quick-assistant-button.square-btn:hover {
  transform: scale(1.05);
  filter: brightness(1.08);
}

.quick-assistant-button.square-btn.active {
  box-shadow: 0 0 0 2px var(--color-surface), 0 0 0 4px var(--color-primary);
}

.assistant-icon-text {
  font-size: 1.1rem;
  font-weight: 600;
  line-height: 1;
}

.quick-assistant-icon {
  width: 18px;
  height: 18px;
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

.quick-assistant-popover.dragging-panel {
  user-select: none;
}
</style>
