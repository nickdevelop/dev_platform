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
  overflow-y: auto;
  padding: 0 calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 2.0);
}

.settings-links {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.settings-link {
  display: flex;
  align-items: center;
  padding: calc(var(--space-unit) * 0.75) calc(var(--space-unit) * 1.0);
  border: 1px solid transparent;
  border-radius: var(--radius);
  color: var(--color-text);
  text-decoration: none;
  font-size: 0.929rem;
  font-weight: normal;
  transition: all 0.15s ease;
  user-select: none;
}

.settings-link:hover {
  border-color: var(--color-border);
  background: var(--color-surface-muted);
}

.settings-link.router-link-active {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
  box-shadow: 0 0 6px var(--color-primary-soft, rgba(59, 130, 246, 0.2));
  font-weight: 500;
  color: var(--color-text);
}
</style>

