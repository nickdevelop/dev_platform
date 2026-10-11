<script setup lang="ts">
// Leftmost category rail. Only the project category is available; the chat
// category is reserved for a later release and is shown disabled. The footer
// links to the settings center and logs out.
import { RouterLink } from 'vue-router'

const emit = defineEmits<{
  logout: []
}>()
</script>

<template>
  <nav class="category-rail" aria-label="大类">
    <button type="button" class="category active" title="项目" aria-current="page">项目</button>
    <button type="button" class="category reserved" title="Chat（预留）" disabled>Chat</button>
    <div class="spacer"></div>
    <RouterLink class="footer-btn settings" title="设置" :to="{ name: 'settings-appearance' }">
      设置
    </RouterLink>
    <button type="button" class="footer-btn logout" title="退出登录" @click="emit('logout')">退出</button>
  </nav>
</template>

<style scoped>
.category-rail {
  background: var(--rail-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: calc(var(--space-unit) * 2.000) 0;
  gap: calc(var(--space-unit) * 1.333);
}

.category,
.footer-btn {
  width: 44px;
  padding: calc(var(--space-unit) * 1.000) 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--rail-text);
  font-size: 0.857rem;
  cursor: pointer;
  text-align: center;
  text-decoration: none;
  display: block;
}

.category.active {
  background: var(--rail-active);
  color: var(--color-surface);
}

.category.reserved,
.category:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.spacer {
  flex: 1;
}
</style>
