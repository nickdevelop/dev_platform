<script setup lang="ts">
// The settings center's own middle column: the category links ride in the
// shell's sidebar in place of the project/task tree, so every area of the
// workbench shares one three-column shell (rail / column / tabbed pane).
// Selecting a category navigates the pane's child route.
import { RouterLink } from 'vue-router'

const links = [
  { name: 'settings-appearance', label: '外观' },
  { name: 'settings-models', label: '模型' },
  { name: 'settings-agents', label: 'Agent' },
]
</script>

<template>
  <nav class="settings-nav" data-test="settings-nav" aria-label="设置分类">
    <div class="sidebar-head">
      <h2>设置</h2>
    </div>
    <div class="settings-links">
      <RouterLink v-for="link in links" :key="link.name" :to="{ name: link.name }" class="settings-link">
        {{ link.label }}
      </RouterLink>
    </div>
  </nav>
</template>

<style scoped>
.settings-nav {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}

.sidebar-head {
  padding: calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 1.333);
}

.sidebar-head h2 {
  font-size: 1.071rem;
  margin: 0;
}

.settings-links {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
  padding: 0 calc(var(--space-unit) * 1.667);
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
</style>

