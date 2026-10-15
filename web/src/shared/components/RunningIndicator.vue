<script setup lang="ts">
// Animated marker for a session whose turn is running. It replaces the plain
// 「运行中」 text on session rows and tabs: a small spinner before the session
// name. The state is carried in the accessible name/title only, so the marker
// stays compact while assistive technology still announces it.
withDefaults(defineProps<{ label?: string }>(), { label: '运行中' })
</script>

<template>
  <span
    class="running-indicator"
    role="img"
    :aria-label="label"
    :title="label"
    data-test="running-indicator"
  >
    <span class="spinner" aria-hidden="true"></span>
  </span>
</template>

<style scoped>
.running-indicator {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
}

/* A classic ring spinner: the neutral track is visible in both themes while the
   rotating head marks motion. */
.spinner {
  width: 0.75rem;
  height: 0.75rem;
  border: 2px solid var(--color-border-strong);
  border-top-color: var(--color-primary);
  border-radius: 999px;
  animation: running-spin 0.8s linear infinite;
}

@keyframes running-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .spinner {
    animation-duration: 2s;
  }
}
</style>
