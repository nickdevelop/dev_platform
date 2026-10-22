<script setup lang="ts">
// Middle column of the pure-chat area: conversation history and a single
// "new chat" action. Titles are filtered locally; the list is small enough
// (latest 200) that a server-side search would add a request per keystroke for
// no benefit.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useChatsStore } from '../../../stores/chats'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'

const chats = useChatsStore()
const router = useRouter()
const route = useRoute()
const search = ref('')

const activeId = computed<number | null>(() => {
  const raw = route.params.chatId
  if (raw === undefined) return null
  const value = Number(raw)
  return Number.isFinite(value) && value > 0 ? value : null
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (q === '') return chats.chats
  return chats.chats.filter((chat) => chat.title.toLowerCase().includes(q))
})

onMounted(() => {
  void chats.loadChats()
})

function startNewChat() {
  chats.startNewChat()
  if (route.name !== 'chat') void router.push({ name: 'chat' })
}

function openChat(id: number) {
  if (id === activeId.value) return
  void router.push({ name: 'chat-detail', params: { chatId: String(id) } })
}
</script>

<template>
  <div class="chat-sidebar">
    <div class="sidebar-actions">
      <button type="button" class="new-chat-btn" data-test="chat-new-btn" @click="startNewChat">
        + 新对话
      </button>
    </div>

    <input
      v-model="search"
      type="search"
      class="search"
      placeholder="搜索对话标题"
      aria-label="搜索对话标题"
    />

    <p v-if="chats.listError" class="list-error" role="alert">{{ chats.listError }}</p>

    <ul class="chat-list" data-test="chat-list">
      <li v-for="chat in filtered" :key="chat.id">
        <button
          type="button"
          class="chat-item"
          :class="{ active: chat.id === activeId }"
          :aria-current="chat.id === activeId ? 'page' : undefined"
          :data-chat-id="chat.id"
          @click="openChat(chat.id)"
        >
          <span class="chat-item-title">{{ chat.title || '未命名对话' }}</span>
          <RunningIndicator v-if="chat.running" label="生成中" class="chat-item-running" />
        </button>
      </li>
    </ul>

    <p v-if="!chats.loading && filtered.length === 0" class="empty">
      {{ search.trim() ? '没有匹配的对话。' : '还没有对话，点击「新对话」开始。' }}
    </p>
  </div>
</template>

<style scoped>
.chat-sidebar {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.sidebar-actions {
  display: flex;
  padding: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.333);
}

.new-chat-btn {
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  font: inherit;
  font-size: 0.857rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}

.new-chat-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.search {
  margin: 0 calc(var(--space-unit) * 2.000) calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.000) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.search:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-info-soft);
}

.search::placeholder {
  color: var(--color-text-muted);
}

.chat-list {
  list-style: none;
  margin: 0;
  padding: 0 calc(var(--space-unit) * 2.000) calc(var(--space-unit) * 2.000);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
  overflow-y: auto;
  min-height: 0;
  flex: 1;
  box-sizing: border-box;
}

.chat-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: calc(var(--space-unit) * 0.5);
  padding: calc(var(--space-unit) * 1.0) calc(var(--space-unit) * 1.333);
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: 0.88rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
  box-sizing: border-box;
}

.chat-item:hover {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
}

.chat-item.active {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
  box-shadow: 0 0 8px var(--color-primary-soft, rgba(59, 130, 246, 0.2));
  font-weight: 600;
}

.chat-item-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.88rem;
}

.chat-item-running {
  flex-shrink: 0;
}

.empty,
.list-error {
  margin: 0 calc(var(--space-unit) * 2.000);
  padding: calc(var(--space-unit) * 1.000) 0;
  color: var(--color-text-muted);
  font-size: 0.857rem;
}

.list-error {
  color: var(--color-danger);
}
</style>
