<script setup lang="ts">
// Engines central wrapper view hosting the active engine sub-section.
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import { useEnginesStore } from '../../stores/engines'

const enginesStore = useEnginesStore()

onMounted(() => {
  if (!enginesStore.settings) {
    void enginesStore.loadSettings()
  }
})
</script>

<template>
  <div class="engines-view-container">
    <div class="engines-view-canvas">
      <RouterView v-slot="{ Component }">
        <transition name="engines-fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </RouterView>
    </div>
  </div>
</template>

<style scoped>
.engines-view-container {
  min-width: 0;
  width: 100%;
  min-height: 100%;
  padding: 24px 32px 64px;
  background: var(--color-bg);
}

.engines-view-canvas {
  max-width: 1080px;
  margin: 0 auto;
  width: 100%;
}

@media (max-width: 768px) {
  .engines-view-container {
    padding: 16px 16px 40px;
  }
}

.engines-fade-enter-active,
.engines-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.engines-fade-enter-from {
  opacity: 0;
  transform: translateY(4px);
}
</style>
