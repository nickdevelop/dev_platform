<script setup lang="ts">
// Leftmost category rail. Only the project category is available; the chat
// category is reserved for a later release and is shown disabled. The footer
// holds the authorized-roots settings and logout.
const emit = defineEmits<{
  'open-roots': []
  logout: []
}>()
</script>

<template>
  <nav class="category-rail" aria-label="大类">
    <button type="button" class="category active" title="项目" aria-current="page">项目</button>
    <button type="button" class="category reserved" title="Chat（预留）" disabled>Chat</button>
    <div class="spacer"></div>
    <button type="button" class="footer-btn" title="授权工作区" @click="emit('open-roots')">设置</button>
    <button type="button" class="footer-btn" title="退出登录" @click="emit('logout')">退出</button>
  </nav>
</template>

<style scoped>
.category-rail {
  background: #1e2330;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 12px 0;
  gap: 8px;
}

.category,
.footer-btn {
  width: 44px;
  padding: 6px 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: #aab2c5;
  font-size: 12px;
  cursor: pointer;
}

.category.active {
  background: #3b4560;
  color: #fff;
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
