<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useTerminalStore } from '../../stores/terminal'
import { useAuthStore } from '../../stores/auth'
import type { TerminalServer } from '../../api/terminal'
import ServerDialog from './ServerDialog.vue'

const terminalStore = useTerminalStore()
const authStore = useAuthStore()

const searchKeyword = ref('')
const showDialog = ref(false)
const editingServer = ref<TerminalServer | null>(null)
const deletingServer = ref<TerminalServer | null>(null)
const isDeleting = ref(false)

onMounted(async () => {
  await Promise.all([terminalStore.loadStatus(), terminalStore.loadServers()])
})

const filteredServers = computed(() => {
  const q = searchKeyword.value.trim().toLowerCase()
  if (!q) return terminalStore.servers
  return terminalStore.servers.filter(
    (s) =>
      s.name.toLowerCase().includes(q) ||
      s.host.toLowerCase().includes(q) ||
      s.username.toLowerCase().includes(q),
  )
})

const localServer = computed(() =>
  filteredServers.value.find((s) => s.is_local || s.id === 0),
)

const remoteServers = computed(() =>
  filteredServers.value.filter((s) => !s.is_local && s.id !== 0),
)

function openCreateDialog() {
  editingServer.value = null
  showDialog.value = true
}

function openEditDialog(server: TerminalServer) {
  editingServer.value = server
  showDialog.value = true
}

function openDeleteConfirm(server: TerminalServer) {
  deletingServer.value = server
}

async function confirmDelete() {
  if (!deletingServer.value) return
  isDeleting.value = true
  try {
    await terminalStore.deleteServer(authStore.csrfToken, deletingServer.value.id)
    deletingServer.value = null
  } finally {
    isDeleting.value = false
  }
}

function onConnect(serverId: number) {
  terminalStore.selectServer(serverId)
  terminalStore.openTerminal(serverId)
}
</script>

<template>
  <div class="terminal-sidebar" aria-label="服务器列表">
    <div class="sidebar-head">
      <h2>服务器</h2>
    </div>

    <div class="sidebar-actions">
      <button
        type="button"
        class="create-server-btn"
        data-test="create-server-btn"
        @click="openCreateDialog"
      >
        + 新增服务器
      </button>
    </div>

    <input
      v-model="searchKeyword"
      type="search"
      class="search"
      placeholder="搜索服务器名称或主机"
      aria-label="搜索服务器名称或主机"
    />

    <div v-if="!terminalStore.isSupported" class="platform-warning" role="alert">
      <span>⚠️ 该功能仅在 Linux 服务器环境中支持，当前宿主暂不支持本地终端。</span>
    </div>

    <div class="server-list-container">
      <!-- 1. Local Server -->
      <div v-if="localServer" class="server-section">
        <div class="section-title">本地环境</div>
        <div
          class="server-card local-card"
          :class="{
            active: terminalStore.activeServerId === 0,
            disabled: !terminalStore.isSupported,
          }"
          @click="terminalStore.selectServer(0)"
        >
          <div class="server-card-main">
            <div class="server-name-row">
              <span class="server-name">{{ localServer.name }}</span>
              <span class="badge local-badge">本机</span>
            </div>
            <div class="server-sub">
              {{ terminalStore.isSupported ? '内置 PTY 终端 Shell' : '当前系统不支持' }}
            </div>
          </div>
          <button
            type="button"
            class="btn-connect"
            :disabled="!terminalStore.isSupported"
            title="连接本机终端"
            @click.stop="onConnect(0)"
          >
            连接
          </button>
        </div>
      </div>

      <!-- 2. Remote Third-party Servers -->
      <div class="server-section">
        <div class="section-title">
          <span>第三方服务器</span>
          <span class="count-tag">{{ remoteServers.length }}</span>
        </div>

        <div v-if="remoteServers.length === 0" class="empty-state">
          <p class="empty-tip">暂无第三方服务器</p>
          <button type="button" class="btn-empty-add" @click="openCreateDialog">
            点击添加远程服务器
          </button>
        </div>

        <div v-else class="remote-list">
          <div
            v-for="srv in remoteServers"
            :key="srv.id"
            class="server-card"
            :class="{ active: terminalStore.activeServerId === srv.id }"
            @click="terminalStore.selectServer(srv.id)"
          >
            <div class="server-card-main">
              <div class="server-name-row">
                <span class="server-name">{{ srv.name }}</span>
                <span class="badge" :class="srv.auth_type">
                  {{ srv.auth_type === 'key' ? '私钥' : '密码' }}
                </span>
              </div>
              <div class="server-sub">
                {{ srv.username }}@{{ srv.host }}:{{ srv.port }}
              </div>
            </div>

            <div class="server-card-actions">
              <button
                type="button"
                class="btn-connect"
                title="打开终端连接"
                @click.stop="onConnect(srv.id)"
              >
                连接
              </button>
              <button
                type="button"
                class="btn-icon"
                title="编辑"
                @click.stop="openEditDialog(srv)"
              >
                <svg viewBox="0 0 24 24" class="action-icon">
                  <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                  <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
              <button
                type="button"
                class="btn-icon danger"
                title="删除"
                @click.stop="openDeleteConfirm(srv)"
              >
                <svg viewBox="0 0 24 24" class="action-icon">
                  <polyline points="3 6 5 6 21 6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Add/Edit Server Dialog -->
    <ServerDialog
      v-if="showDialog"
      :server="editingServer"
      @close="showDialog = false"
      @saved="showDialog = false"
    />

    <!-- Delete Confirm Modal -->
    <div v-if="deletingServer" class="dialog-backdrop">
      <div class="confirm-modal" role="dialog" aria-modal="true">
        <h3>确认删除服务器</h3>
        <p>确定要删除服务器「{{ deletingServer.name }}」吗？保存的凭据也将一并移除。</p>
        <div class="modal-actions">
          <button type="button" class="btn-cancel" @click="deletingServer = null">
            取消
          </button>
          <button
            type="button"
            class="btn-danger"
            :disabled="isDeleting"
            @click="confirmDelete"
          >
            {{ isDeleting ? '删除中…' : '删除' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.terminal-sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: calc(var(--space-unit, 8px) * 1.5);
  box-sizing: border-box;
  background: var(--color-surface, #1e1e24);
  color: var(--color-text, #fff);
  overflow-y: auto;
}

.sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
  margin-bottom: 8px;
}

.sidebar-head h2 {
  font-size: 15px;
  font-weight: 600;
  margin: 0;
}

.sidebar-actions {
  display: flex;
  margin-bottom: 10px;
}

.create-server-btn {
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 6px 12px;
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  border-radius: var(--radius, 6px);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: opacity 0.15s;
}

.create-server-btn:hover {
  opacity: 0.9;
}

.search {
  width: 100%;
  padding: 6px 10px;
  background: var(--color-surface-muted, #272730);
  border: 1px solid var(--color-border, #383844);
  border-radius: 6px;
  color: var(--color-text, #fff);
  font-size: 13px;
  box-sizing: border-box;
  margin-bottom: 12px;
}

.search:focus {
  outline: none;
  border-color: var(--color-primary, #3b82f6);
}

.platform-warning {
  padding: 8px 10px;
  margin-bottom: 12px;
  background: rgba(234, 179, 8, 0.1);
  border: 1px solid rgba(234, 179, 8, 0.3);
  border-radius: 6px;
  font-size: 12px;
  color: #facc15;
  line-height: 1.4;
}

.server-list-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.server-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-muted, #9ca3af);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.count-tag {
  background: var(--color-surface-muted, #373842);
  padding: 1px 6px;
  border-radius: 10px;
  font-size: 11px;
}

.server-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--color-surface-muted, #272832);
  border: 1px solid var(--color-border, #3a3b48);
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}

.server-card:hover {
  background: var(--color-bg, #2e2f3d);
  border-color: var(--color-border-strong, #4f5062);
}

.server-card.active {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.08);
}

.server-card.disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.server-card-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.server-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.server-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text, #fff);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.server-sub {
  font-size: 11px;
  color: var(--color-text-muted, #9ca3af);
  font-family: var(--font-mono, monospace);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: 500;
}

.local-badge {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.badge.password {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
}

.badge.key {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
}

.btn-connect {
  padding: 4px 10px;
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: opacity 0.15s;
  flex-shrink: 0;
}

.btn-connect:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-connect:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.server-card-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.btn-icon {
  background: transparent;
  border: none;
  color: var(--color-text-muted, #9ca3af);
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color 0.15s, background 0.15s;
}

.btn-icon:hover {
  color: var(--color-text, #fff);
  background: rgba(255, 255, 255, 0.08);
}

.btn-icon.danger:hover {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.1);
}

.action-icon {
  width: 14px;
  height: 14px;
}

.empty-state {
  padding: 16px 12px;
  text-align: center;
  background: var(--color-surface-muted, #272832);
  border: 1px dashed var(--color-border, #3a3b48);
  border-radius: 8px;
}

.empty-tip {
  font-size: 12px;
  color: var(--color-text-muted, #9ca3af);
  margin: 0 0 8px 0;
}

.btn-empty-add {
  background: transparent;
  color: var(--color-primary, #3b82f6);
  border: none;
  font-size: 12px;
  cursor: pointer;
  padding: 0;
  text-decoration: underline;
}

.dialog-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.confirm-modal {
  background: var(--color-surface, #20212a);
  border: 1px solid var(--color-border, #3a3b48);
  border-radius: 8px;
  padding: 20px;
  max-width: 360px;
  width: 90%;
  color: #fff;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.5);
}

.confirm-modal h3 {
  margin: 0 0 10px 0;
  font-size: 15px;
}

.confirm-modal p {
  font-size: 13px;
  color: var(--color-text-muted, #d1d5db);
  line-height: 1.5;
  margin: 0 0 18px 0;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.btn-cancel {
  padding: 6px 12px;
  background: transparent;
  color: var(--color-text-muted, #a1a1aa);
  border: 1px solid var(--color-border, #3f3f46);
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
}

.btn-danger {
  padding: 6px 14px;
  background: #dc2626;
  color: #fff;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
}

.btn-danger:disabled {
  opacity: 0.6;
}
</style>
