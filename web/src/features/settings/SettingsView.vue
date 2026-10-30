<script setup lang="ts">
// Settings center pane wrapper: with category links living in the shell's
// middle column (SettingsNav.vue), this component hosts the active section
// inside a framed canvas following the app-screens guidelines (framed-sheet style,
// medium density, 24px pad, clean section rhythm, zero resting shadows).
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="settings-view-container">
    <div class="settings-view-canvas">
      <RouterView v-slot="{ Component }">
        <transition name="settings-fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </RouterView>
    </div>
  </div>
</template>

<style scoped>
.settings-view-container {
  min-width: 0;
  width: 100%;
  min-height: 100%;
  padding: 24px 32px 64px;
  background: var(--color-bg);
}

.settings-view-canvas {
  max-width: 1080px;
  margin: 0 auto;
  width: 100%;
}

@media (max-width: 768px) {
  .settings-view-container {
    padding: 16px 16px 40px;
  }
}

.settings-fade-enter-active,
.settings-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.settings-fade-enter-from {
  opacity: 0;
  transform: translateY(4px);
}

.settings-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
