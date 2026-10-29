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
  /* 20px is deliberate: the rail gives the conversation back every pixel it can
     and the buttons carry their own caption (icon on top, vertical text below).
     The shell overrides --chat-rail-width to resize both rails together. */
  flex: 0 0 var(--chat-rail-width, 20px);
  width: var(--chat-rail-width, 20px);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 4px 0;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  min-height: 0;
}

.rail-items {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

/* The rail's own caption sits at the very bottom, pushed there by margin-top:
   auto, so it never competes with the buttons for the top of the rail. The
   narrow width stacks the two characters without writing-mode, which keeps the
   column centred on the rail's axis instead of clinging to its right edge. */
.rail-label {
  margin-top: auto;
  margin-bottom: 2px;
  width: 12px;
  color: var(--color-text-muted);
  font-size: 0.6rem;
  line-height: 1.5;
  text-align: center;
  word-break: break-all;
  overflow-wrap: anywhere;
  user-select: none;
}

/* Slot content belongs to SessionChatPanel, so :slotted is required for these
   styles to reach the buttons rendered there. The button is a vertical stack:
   icon on top, a slim vertical-rl caption below it, both on the same centre
   line and with square corners to match the narrow rail. */
.chat-side-rail :slotted(.rail-btn) {
  position: relative;
  width: 16px;
  padding: 3px 0 4px;
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  border: 1px solid transparent;
  border-radius: 0;
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
  width: 14px;
  height: 14px;
  flex: none;
  stroke: currentColor;
  fill: none;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* The caption under the icon, kept out of the accessible name (the button's
   aria-label already names the action). Deliberately NOT writing-mode: in
   vertical-rl the block axis is horizontal, so a single line hugs the box's
   right edge and cannot be centred on the icon. A width of one em with
   word-break stacks the characters one per line, and text-align centres that
   column on the same axis as the icon above it. */
.chat-side-rail :slotted(.rail-text) {
  width: 1em;
  color: inherit;
  font-size: 10px;
  line-height: 1.35;
  letter-spacing: 0;
  text-align: center;
  word-break: break-all;
  overflow-wrap: anywhere;
  user-select: none;
}
</style>
