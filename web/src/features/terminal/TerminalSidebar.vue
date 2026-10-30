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
    await terminalStore.deleteServer(deletingServer.value.id)
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
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn create-server-btn"
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

    <div v-if="!terminalStore.isSupported" class="platform-warning flex items-center gap-1.5" role="alert">
      <svg class="shrink-0 text-amber-500" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M8 2l6.5 11.5H1.5L8 2z" />
        <line x1="8" y1="6" x2="8" y2="9.5" />
        <circle cx="8" cy="11.5" r="0.75" fill="currentColor" />
      </svg>
      <span>该功能仅在 Linux 服务器环境中支持，当前宿主暂不支持本地终端。</span>
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
          <div class="server-card-actions">
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
                    <button
                    type="button"
                    class="btn-connect"
                    title="打开终端连接"
                    @click.stop="onConnect(srv.id)"
                  >
                    连接
                  </button>
              </div>
              <div class="server-sub-row">
                <span class="server-sub">
                  {{ srv.username }}@{{ srv.host }}:{{ srv.port }}
                </span>
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
  min-height: 0;
  flex: 1;
  overflow: hidden;
  color: var(--color-text);
}

.sidebar-actions {
  display: flex;
  gap: 6px;
  padding: 0 12px 8px 12px;
}

.action-btn,
.create-server-btn {
  flex: 1;
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 26px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius, 4px);
  padding: 0 8px;
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
  white-space: nowrap;
}

.action-btn:hover,
.create-server-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
  color: var(--color-text);
}

.search {
  margin: 0 12px 8px 12px;
  padding: 4px 10px;
  height: 28px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.12s ease-out;
}

.search:focus {
  border-color: var(--color-accent, #4a8ad6);
}

.search::placeholder {
  color: var(--color-text-muted);
}

.platform-warning {
  margin: 0 12px 8px 12px;
  padding: 6px 8px;
  background: color-mix(in srgb, var(--color-warning, #d9a441) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--color-warning, #d9a441) 30%, transparent);
  border-radius: var(--radius, 4px);
  font-size: 11px;
  color: var(--color-warning, #d9a441);
  line-height: 1.4;
}

.server-list-container {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px 16px 8px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.server-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  color: var(--color-text-muted);
  padding: 0 6px;
  letter-spacing: 0.3px;
}

.count-tag {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  padding: 0 4px;
  border-radius: var(--radius, 4px);
  font-size: 10px;
  font-weight: normal;
  font-variant-numeric: tabular-nums;
}

.remote-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.server-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 6px 8px;
  border-radius: var(--radius, 4px);
  border-left: 2px solid transparent;
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out;
  user-select: none;
}

.server-card:hover {
  background: var(--color-surface-muted);
}

.server-card.active {
  border-left-color: var(--color-accent, #4a8ad6);
  background: var(--color-surface-muted);
  font-weight: 500;
}

.server-card.disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.server-card-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.server-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.server-name-row .btn-connect {
  margin-left: auto;
}

.server-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.server-sub-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.server-sub-row .server-sub {
  flex: 1;
  min-width: 0;
}

.server-sub {
  font-size: 11px;
  color: var(--color-text-muted);
  font-family: var(--font-mono, monospace);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.badge {
  font-size: 10px;
  padding: 0 4px;
  height: 16px;
  line-height: 16px;
  border-radius: 3px;
  font-weight: 500;
  border: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.local-badge {
  color: var(--color-success, #44a282);
  border-color: color-mix(in srgb, var(--color-success, #44a282) 30%, transparent);
  background: color-mix(in srgb, var(--color-success, #44a282) 10%, transparent);
}

.badge.password,
.badge.key {
  color: var(--color-text-muted);
}

.server-card-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.12s ease-out;
}

.server-card:hover .server-card-actions,
.server-card:focus-within .server-card-actions {
  opacity: 1;
}

.btn-connect {
  padding: 1px 6px;
  height: 20px;
  background: var(--color-surface);
  color: var(--color-text);
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  font-size: 11px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
  flex-shrink: 0;
}

.btn-connect:hover:not(:disabled) {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
  color: var(--color-accent, #4a8ad6);
}

.btn-connect:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-icon {
  background: transparent;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 2px;
  width: 18px;
  height: 18px;
  border-radius: var(--radius, 4px);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color 0.12s ease-out;
}

.btn-icon:hover {
  color: var(--color-text);
}

.btn-icon.danger:hover {
  color: var(--color-danger, #e5566f);
}

.action-icon {
  width: 12px;
  height: 12px;
}

.empty-state {
  padding: 16px 8px;
  text-align: center;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
}

.empty-tip {
  font-size: 11px;
  color: var(--color-text-muted);
  margin: 0 0 6px 0;
}

.btn-empty-add {
  background: transparent;
  color: var(--color-accent, #4a8ad6);
  border: none;
  font-size: 11px;
  cursor: pointer;
  padding: 2px 4px;
}

.btn-empty-add:hover {
  text-decoration: underline;
}

.dialog-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(10, 12, 16, 0.75);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.confirm-modal {
  background: var(--color-surface);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius, 4px);
  padding: 16px;
  max-width: 360px;
  width: 100%;
}

.confirm-modal h3 {
  margin: 0 0 8px 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text);
}

.confirm-modal p {
  margin: 0 0 16px 0;
  font-size: 12px;
  color: var(--color-text-muted);
  line-height: 1.5;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.btn-cancel {
  padding: 4px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 12px;
  cursor: pointer;
}

.btn-danger {
  padding: 4px 10px;
  border: 1px solid var(--color-danger, #e5566f);
  border-radius: var(--radius, 4px);
  background: var(--color-danger, #e5566f);
  color: #fff;
  font-size: 12px;
  cursor: pointer;
}

.btn-danger:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
