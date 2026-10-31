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
  <div class="terminal-sidebar sidebar-panel" aria-label="服务器列表">
    <!-- Zone A: Dedicated actions row -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn create-server-btn"
        data-test="create-server-btn"
        title="添加新的远程 SSH 服务器连接"
        @click="openCreateDialog"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新增服务器</span>
      </button>
    </div>

    <!-- Zone B: Search input with inset magnifier -->
    <div class="sidebar-search-bar">
      <svg class="sidebar-search-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="7" cy="7" r="4.5" />
        <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
      </svg>
      <input
        v-model="searchKeyword"
        type="search"
        class="search sidebar-search-input"
        placeholder="搜索服务器名称或主机"
        aria-label="搜索服务器名称或主机"
      />
      <button
        v-if="searchKeyword"
        type="button"
        class="sidebar-search-clear"
        title="清空搜索"
        aria-label="清空搜索"
        @click="searchKeyword = ''"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="4" y1="4" x2="12" y2="12" />
          <line x1="12" y1="4" x2="4" y2="12" />
        </svg>
      </button>
    </div>

    <div v-if="!terminalStore.isSupported" class="platform-warning flex items-center gap-1.5" role="alert">
      <svg class="shrink-0 text-amber-500" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M8 2l6.5 11.5H1.5L8 2z" />
        <line x1="8" y1="6" x2="8" y2="9.5" />
        <circle cx="8" cy="11.5" r="0.75" fill="currentColor" />
      </svg>
      <span>该功能仅在 Linux 服务器环境中支持，当前宿主暂不支持本地终端。</span>
    </div>

    <!-- Zone C: Server list container -->
    <div class="server-list-container sidebar-list-container">
      <!-- 1. Local Server -->
      <div v-if="localServer" class="server-section">
        <div class="section-title">本地环境</div>
        <div
          class="server-card local-card sidebar-card group"
          :class="{
            active: terminalStore.activeServerId === 0,
            disabled: !terminalStore.isSupported,
          }"
          role="button"
          tabindex="0"
          @click="terminalStore.selectServer(0)"
          @keydown.enter.prevent="terminalStore.selectServer(0)"
          @keydown.space.prevent="terminalStore.selectServer(0)"
        >
          <!-- Row 1: Name + Badge + Connect button -->
          <div class="sidebar-card-row-top">
            <div class="flex items-center gap-1.5 min-w-0 flex-1">
              <span class="server-name sidebar-card-title" :title="localServer.name">{{ localServer.name }}</span>
              <span class="badge local-badge sidebar-card-badge">本机</span>
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

          <!-- Row 2: Subtext / Status -->
          <div class="sidebar-card-row-bottom">
            <span class="server-sub sidebar-card-meta">
              {{ terminalStore.isSupported ? '内置 PTY 终端 Shell' : '当前系统不支持' }}
            </span>
          </div>
        </div>
      </div>

      <!-- 2. Remote Third-party Servers -->
      <div class="server-section">
        <div class="section-title">
          <span>第三方服务器</span>
          <span class="count-tag">{{ remoteServers.length }}</span>
        </div>

        <div v-if="remoteServers.length === 0" class="empty-state sidebar-empty">
          <p class="empty-tip">暂无第三方服务器</p>
          <button type="button" class="btn-empty-add" @click="openCreateDialog">
            点击添加远程服务器
          </button>
        </div>

        <div v-else class="remote-list">
          <div
            v-for="srv in remoteServers"
            :key="srv.id"
            class="server-card sidebar-card group"
            :class="{ active: terminalStore.activeServerId === srv.id }"
            role="button"
            tabindex="0"
            @click="terminalStore.selectServer(srv.id)"
            @keydown.enter.prevent="terminalStore.selectServer(srv.id)"
            @keydown.space.prevent="terminalStore.selectServer(srv.id)"
          >
            <!-- Row 1: Server Name + Auth Badge + Connect button -->
            <div class="sidebar-card-row-top">
              <div class="flex items-center gap-1.5 min-w-0 flex-1">
                <span class="server-name sidebar-card-title" :title="srv.name">{{ srv.name }}</span>
                <span class="badge sidebar-card-badge" :class="srv.auth_type">
                  {{ srv.auth_type === 'key' ? '私钥' : '密码' }}
                </span>
              </div>
              <button
                type="button"
                class="btn-connect"
                title="打开终端连接"
                @click.stop="onConnect(srv.id)"
              >
                连接
              </button>
            </div>

            <!-- Row 2: Host string + Hover Actions (Edit/Delete) -->
            <div class="sidebar-card-row-bottom">
              <span class="server-sub sidebar-card-meta" :title="`${srv.username}@${srv.host}:${srv.port}`">
                {{ srv.username }}@{{ srv.host }}:{{ srv.port }}
              </span>
              <div class="server-card-actions sidebar-card-actions" @click.stop>
                <button
                  type="button"
                  class="btn-icon sidebar-icon-btn"
                  title="编辑服务器配置"
                  :aria-label="`编辑服务器「${srv.name}」`"
                  @click.stop="openEditDialog(srv)"
                >
                  <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                    <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
                  </svg>
                </button>
                <button
                  type="button"
                  class="btn-icon danger sidebar-icon-btn"
                  title="删除服务器配置"
                  :aria-label="`删除服务器「${srv.name}」`"
                  @click.stop="openDeleteConfirm(srv)"
                >
                  <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                    <path d="M3 4h10M6 4V2.5h4V4M4.5 4v9.5h7V4" />
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
  padding: 0 4px;
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
  gap: 3px;
}

.server-card.disabled {
  opacity: 0.5;
  cursor: not-allowed;
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
  padding: 0;
  width: 18px;
  height: 18px;
  border-radius: var(--radius, 4px);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: color 0.12s ease-out, background-color 0.12s ease-out;
}

.btn-icon:hover {
  color: var(--color-text);
  background: var(--color-surface);
}

.btn-icon.danger:hover {
  color: var(--color-danger, #e5566f);
  background: var(--color-danger-soft, #fee2e2);
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
