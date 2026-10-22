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
  gap: 0.5rem;
  padding: 0.5rem 0.75rem 1rem;
  min-height: 0;
}

.sidebar-actions {
  display: flex;
}

.new-chat-btn {
  width: 100%;
  padding: 0.45rem 0.75rem;
  border: 1px solid var(--color-primary);
  border-radius: 8px;
  background: var(--color-primary);
  color: var(--color-surface);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}

.search {
  width: 100%;
  box-sizing: border-box;
}

.chat-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  overflow-y: auto;
  min-height: 0;
}

.chat-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem;
  padding: 0.45rem 0.55rem;
  border: 1px solid transparent;
  border-radius: 7px;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.chat-item:hover {
  background: var(--color-surface-muted);
}

.chat-item.active {
  border-color: var(--color-primary);
  background: var(--color-info-soft);
}

.chat-item-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.85rem;
}

.chat-item-running {
  flex-shrink: 0;
}

.empty,
.list-error {
  margin: 0.25rem 0;
  color: var(--color-text-muted);
  font-size: 0.82rem;
}

.list-error {
  color: var(--color-danger);
}
</style>
