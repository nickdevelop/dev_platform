<script setup lang="ts">
// The centered chevron at the foot of a clipped output block. Folded, it rides
// on the last visible row over a fade that hints at the rows hidden below (▼);
// expanded, it sits below the full content so the user can fold it back (▲).
// Both the plain-text output and the edit diff share it, so a long tool body
// always offers the same affordance.
defineProps<{ collapsed: boolean }>()
defineEmits<{ toggle: [] }>()
</script>

<template>
  <div class="expand-indicator" :class="{ 'is-collapsed': collapsed }" data-test="output-more">
    <button
      type="button"
      class="expand-button"
      data-test="output-more-toggle"
      :aria-expanded="(!collapsed).toString()"
      :aria-label="collapsed ? '展开全部内容' : '收起内容'"
      :title="collapsed ? '展开全部内容' : '收起内容'"
      @click="$emit('toggle')"
    >
      {{ collapsed ? '▼' : '▲' }}
    </button>
  </div>
</template>

<style scoped>
.expand-indicator {
  display: flex;
  justify-content: center;
  margin-top: 0.2rem;
}

/* Folded: the chevron overlays the last rendered row, over a gradient that
   fades the clipped text into the block's background. The overlay itself never
   swallows clicks; only the button does. */
.expand-indicator.is-collapsed {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 2.4em;
  margin-top: 0;
  align-items: flex-end;
  padding-bottom: 0.1em;
  background: linear-gradient(to bottom, transparent, var(--color-surface-muted) 72%);
  pointer-events: none;
}

.expand-button {
  pointer-events: auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 2.2rem;
  padding: 0.05rem 0.5rem;
  border: none;
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  font: inherit;
  font-size: 0.8rem;
  line-height: 1.4;
}

.expand-button:hover {
  color: var(--color-text);
}
</style>
