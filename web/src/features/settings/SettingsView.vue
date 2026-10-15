<script setup lang="ts">
// Settings center layout: a secondary navigation column plus the active
// section. Reached from the category rail; the project sidebar is hidden
// while it is open (AppShell checks route.meta.settings).
import { RouterLink, RouterView } from 'vue-router'

const links = [
  { name: 'settings-appearance', label: '外观' },
  { name: 'settings-models', label: '模型' },
  { name: 'settings-agents', label: 'Agent' },
]
</script>

<template>
  <div class="settings-view">
    <nav class="settings-nav" aria-label="设置分类">
      <RouterLink
        v-for="link in links"
        :key="link.name"
        :to="{ name: link.name }"
        class="settings-link"
      >
        {{ link.label }}
      </RouterLink>
    </nav>
    <div class="settings-content">
      <RouterView />
    </div>
  </div>
</template>

<style scoped>
.settings-view {
  display: grid;
  grid-template-columns: 180px 1fr;
  gap: calc(var(--space-unit) * 3);
  height: 100%;
  min-height: 0;
}

.settings-nav {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.settings-link {
  padding: calc(var(--space-unit) * 1.2) calc(var(--space-unit) * 2);
  border-radius: var(--radius);
  color: var(--color-text-muted);
  text-decoration: none;
  font-size: 1rem;
}

.settings-link:hover {
  background: var(--color-surface-muted);
}

.settings-link.router-link-active {
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-weight: 600;
}

.settings-content {
  min-width: 0;
  overflow: auto;
}
</style>
