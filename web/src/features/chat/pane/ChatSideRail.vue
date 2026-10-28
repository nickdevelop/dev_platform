<script setup lang="ts">
// A slim, always-visible rail beside the conversation. It owns only the rail
// shell and button styling; the chat panel supplies the actual controls through
// the slot and keeps every piece of pane state in useActivityPane. Keeping the
// component chat-local (not shared/ui) prevents a business-specific control from
// leaking into the provider-neutral UI library.
defineProps<{
  side: 'left' | 'right'
  // Short vertical caption shown at the bottom of the rail ("文件" / "预览").
  label: string
  // Full accessible name for the toolbar itself.
  ariaLabel: string
}>()
</script>

<template>
  <div
    class="chat-side-rail"
    :class="side"
    role="toolbar"
    :aria-label="ariaLabel"
    aria-orientation="vertical"
    :data-test="`chat-side-rail-${side}`"
  >
    <div class="rail-items">
      <slot />
    </div>
    <span class="rail-label" aria-hidden="true">{{ label }}</span>
  </div>
</template>

<style scoped>
.chat-side-rail {
  /* 34px is deliberate: two rails cost less than one old header button row and
     keep the conversation width when both panes are open. The shell can override
     --chat-rail-width to resize both rails together. */
  flex: 0 0 var(--chat-rail-width, 34px);
  width: var(--chat-rail-width, 34px);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 6px 0;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface);
  min-height: 0;
}

.rail-items {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.rail-label {
  margin-top: auto;
  margin-bottom: 2px;
  color: var(--color-text-muted);
  font-size: 0.6rem;
  line-height: 1;
  letter-spacing: 0.08em;
  writing-mode: vertical-rl;
  user-select: none;
}

/* Slot content belongs to SessionChatPanel, so :slotted is required for these
   styles to reach the buttons rendered there. */
.chat-side-rail :slotted(.rail-btn) {
  position: relative;
  width: 26px;
  height: 26px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.chat-side-rail :slotted(.rail-btn:hover) {
  background: var(--color-surface-muted, #f8fafc);
  color: var(--color-text);
}

.chat-side-rail :slotted(.rail-btn:focus-visible) {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-soft, #e8effd);
}

.chat-side-rail :slotted(.rail-btn.is-active) {
  background: var(--color-primary-soft, #e8effd);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.chat-side-rail :slotted(.rail-icon) {
  width: 16px;
  height: 16px;
  stroke: currentColor;
  fill: none;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.chat-side-rail :slotted(.rail-count) {
  position: absolute;
  top: -5px;
  right: -6px;
  min-width: 14px;
  height: 14px;
  padding: 0 3px;
  border: 1.5px solid var(--color-surface);
  border-radius: 999px;
  background: var(--color-primary);
  color: var(--color-on-primary, #fff);
  font-size: 0.56rem;
  font-weight: 700;
  line-height: 11px;
  text-align: center;
}
</style>
