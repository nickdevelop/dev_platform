<script setup lang="ts">
// Terminal and SSH Server settings view.
// Redesigned with design-taste-frontend app-screens guidelines (framed-sheet,
// flat surfaces, zero resting shadows, tabular figures for ports, sentence-case
// 12px table headers, pale status pills, and consistent top error alerts).
import { computed, onMounted, ref } from 'vue'
import { useTerminalStore } from '../../stores/terminal'
import { useProjectTerminalStore } from '../../stores/projectTerminal'
import { type TerminalServer, testServerConnection } from '../../api/terminal'
import ServerDialog from '../terminal/ServerDialog.vue'
import {
  UiButton,
  UiBadge,
  UiAlert,
  UiEmpty,
  UiConfirmDialog,
  UiSpinner,
} from '../../shared/ui'

const terminalStore = useTerminalStore()
const projectTerminal = useProjectTerminalStore()

const searchKeyword = ref('')
const loading = ref(false)
const errorMessage = ref('')

const showDialog = ref(false)
const editingServer = ref<TerminalServer | null>(null)
const pendingDelete = ref<TerminalServer | null>(null)
const isDeleting = ref(false)
const deleteError = ref('')

// Connection test state
const testingServerId = ref<number | null>(null)
const testResults = ref<Record<number, { ok: boolean; message: string }>>({})

async function fetchData() {
  loading.value = true
  errorMessage.value = ''
  try {
    await Promise.all([terminalStore.loadStatus(), terminalStore.loadServers()])
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : '加载终端服务与配置失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void fetchData()
})

function clearError() {
  errorMessage.value = ''
  terminalStore.error = null
}

const localServer = computed(() =>
  terminalStore.servers.find((s) => s.is_local || s.id === 0),
)

const remoteServers = computed(() => {
  const remotes = terminalStore.servers.filter((s) => !s.is_local && s.id !== 0)
  const q = searchKeyword.value.trim().toLowerCase()
  if (!q) return remotes
  return remotes.filter(
    (s) =>
      s.name.toLowerCase().includes(q) ||
      s.host.toLowerCase().includes(q) ||
      s.username.toLowerCase().includes(q),
  )
})

function openCreateDialog() {
  editingServer.value = null
  showDialog.value = true
}

function openEditDialog(server: TerminalServer) {
  editingServer.value = server
  showDialog.value = true
}

function handleDelete(server: TerminalServer) {
  deleteError.value = ''
  pendingDelete.value = server
}

async function confirmDelete() {
  if (!pendingDelete.value || isDeleting.value) return
  const target = pendingDelete.value
  isDeleting.value = true
  deleteError.value = ''
  try {
    await terminalStore.deleteServer(target.id)
    pendingDelete.value = null
  } catch (err: unknown) {
    deleteError.value = err instanceof Error ? err.message : '删除服务器失败'
  } finally {
    isDeleting.value = false
  }
}

async function onTestServer(server: TerminalServer) {
  testingServerId.value = server.id
  delete testResults.value[server.id]
  try {
    const res = await testServerConnection({
      host: server.host,
      port: server.port,
      username: server.username,
      auth_type: server.auth_type === 'key' ? 'key' : 'password',
      server_id: server.id,
    })
    testResults.value[server.id] = {
      ok: res.ok,
      message: res.ok ? '连接测试成功' : res.message || '连接失败',
    }
  } catch (err: unknown) {
    testResults.value[server.id] = {
      ok: false,
      message: err instanceof Error ? err.message : '连接超时或握手失败',
    }
  } finally {
    testingServerId.value = null
  }
}

function openTerminalForServer(server: TerminalServer) {
  if (server.is_local || server.id === 0) {
    projectTerminal.openLocalTab()
  } else {
    projectTerminal.openServerTab(server.id, server.name)
  }
}

async function onDialogSaved() {
  showDialog.value = false
  await terminalStore.loadServers()
}
</script>

<template>
  <section class="terminal-settings-view w-full space-y-6" aria-label="终端服务器设置">
    <!-- Header -->
    <header class="flex items-center justify-between pb-4 border-b border-slate-200 dark:border-slate-800 gap-4 flex-wrap">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-semibold text-slate-900 dark:text-slate-100 tracking-tight">终端服务器设置</h1>
          <UiBadge variant="default" size="sm">Terminal & SSH</UiBadge>
        </div>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-1">
          管理本地终端环境与远程 SSH 服务器连接配置，支持快速发起终端会话及服务连通性测试。
        </p>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <UiButton variant="secondary" size="sm" :disabled="loading" @click="fetchData">
          刷新
        </UiButton>
        <UiButton variant="primary" size="sm" data-test="create-server-btn" @click="openCreateDialog">
          + 新增服务器
        </UiButton>
      </div>
    </header>

    <!-- Top Error Alert: consistent with ProxySettings and ModelSettings -->
    <UiAlert
      v-if="errorMessage || terminalStore.error"
      variant="error"
      closable
      @close="clearError"
    >
      {{ errorMessage || terminalStore.error }}
    </UiAlert>

    <!-- Section 1: Local Server Environment -->
    <section class="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-800 p-4 space-y-3" aria-label="本地宿主环境">
      <div class="flex items-center justify-between flex-wrap gap-2">
        <div class="flex items-center gap-2.5">
          <span class="font-semibold text-sm text-slate-900 dark:text-slate-100">本地宿主环境</span>
          <UiBadge variant="default" size="sm">Localhost</UiBadge>
          <span
            class="inline-flex items-center gap-1.5 text-2xs px-2 py-0.5 rounded-full"
            :class="terminalStore.isSupported
              ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
              : 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'"
          >
            <span
              class="w-1.5 h-1.5 rounded-full"
              :class="terminalStore.isSupported ? 'bg-emerald-500' : 'bg-amber-500'"
            />
            {{ terminalStore.isSupported ? 'PTY 引擎就绪' : '不支持本地 PTY' }}
          </span>
        </div>
        <UiButton
          variant="secondary"
          size="sm"
          :disabled="!terminalStore.isSupported"
          data-test="open-local-term-btn"
          @click="openTerminalForServer(localServer || { id: 0, name: '本机 (Localhost)', host: 'localhost', port: 0, username: 'local', auth_type: 'local', is_local: true, has_secret: false })"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="mr-1.5 inline">
            <path d="M3 4.5l3.5 3.5L3 11.5M8.5 11.5h4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          打开本机终端
        </UiButton>
      </div>

      <UiAlert
        v-if="!terminalStore.isSupported"
        variant="warning"
        class="text-xs"
      >
        {{ terminalStore.status?.message || '当前宿主非 Linux 环境，本地 PTY 终端暂不支持；您仍可添加并连接远程 SSH 服务器。' }}
      </UiAlert>
      <p v-else class="text-xs text-slate-500 dark:text-slate-400">
        本地终端会话直接连接宿主系统 Shell 进程，支持实时交互、命令执行及工作区路径自动同步。
      </p>
    </section>

    <!-- Section 2: Remote SSH Servers (Table List View) -->
    <section class="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-800 p-4 space-y-4" aria-label="远程 SSH 服务器列表">
      <div class="flex items-center justify-between flex-wrap gap-3 pb-3 border-b border-slate-100 dark:border-slate-800">
        <div>
          <h2 class="text-sm font-semibold text-slate-900 dark:text-slate-100">远程 SSH 服务器</h2>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
            配置用于远程维护、跨机部署与脚本调试的第三方 SSH 节点。
          </p>
        </div>
        <div class="flex items-center gap-2">
          <input
            v-model="searchKeyword"
            type="search"
            placeholder="搜索服务器名称/主机/用户…"
            class="px-2.5 py-1 text-xs bg-slate-50 dark:bg-slate-800/80 border border-slate-200 dark:border-slate-700 rounded text-slate-800 dark:text-slate-200 placeholder-slate-400 focus:outline-none focus:border-blue-500"
          />
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading && terminalStore.servers.length === 0" class="py-12 flex flex-col items-center justify-center gap-3 text-slate-400">
        <UiSpinner size="lg" />
        <span class="text-xs">正在加载终端服务器列表…</span>
      </div>

      <!-- Empty State: No remote servers -->
      <div
        v-else-if="terminalStore.servers.filter((s) => !s.is_local && s.id !== 0).length === 0"
        class="py-12"
      >
        <UiEmpty
          title="暂无配置的远程 SSH 服务器"
          description="添加第三方 Linux 服务器后，可直接在平台内通过 SSH 发起独立或多标签终端会话。"
        >
          <UiButton variant="primary" size="sm" class="mt-2" data-test="create-server-btn" @click="openCreateDialog">
            添加第一个远程服务器
          </UiButton>
        </UiEmpty>
      </div>

      <!-- Empty State: Filter matched nothing -->
      <div
        v-else-if="remoteServers.length === 0"
        class="py-8 text-center text-xs text-slate-400"
      >
        未找到匹配「{{ searchKeyword }}」的服务器
      </div>

      <!-- Structured Table List -->
      <div v-else class="overflow-x-auto rounded border border-slate-100 dark:border-slate-800" data-test="server-list">
        <table class="w-full text-left text-xs border-collapse">
          <thead class="bg-slate-50 dark:bg-slate-800/50 text-slate-500 dark:text-slate-400 text-2xs font-medium border-b border-slate-100 dark:border-slate-800">
            <tr>
              <th class="py-2.5 px-3">服务器名称</th>
              <th class="py-2.5 px-3">主机与端口</th>
              <th class="py-2.5 px-3">登录用户</th>
              <th class="py-2.5 px-3">认证方式</th>
              <th class="py-2.5 px-3">连通性状态</th>
              <th class="py-2.5 px-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800 text-slate-700 dark:text-slate-300">
            <tr
              v-for="server in remoteServers"
              :key="server.id"
              class="server-card hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition-colors"
              data-test="server-card"
              :data-server-id="server.id"
            >
              <td class="py-2.5 px-3 font-medium text-slate-900 dark:text-slate-100">
                <div class="flex items-center gap-2">
                  <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8" class="text-slate-400 shrink-0">
                    <rect x="2" y="3" width="12" height="10" rx="1.5" />
                    <path d="M5 7l2 1.5-2 1.5M9 10h2" stroke-linecap="round" stroke-linejoin="round" />
                  </svg>
                  <span class="truncate max-w-[180px]" :title="server.name">{{ server.name }}</span>
                </div>
              </td>

              <td class="py-2.5 px-3 font-mono text-2xs text-slate-600 dark:text-slate-400">
                <code class="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800/80 text-blue-600 dark:text-blue-400 tabular-nums">
                  {{ server.host }}:{{ server.port }}
                </code>
              </td>

              <td class="py-2.5 px-3 font-mono text-2xs text-slate-700 dark:text-slate-300">
                {{ server.username }}
              </td>

              <td class="py-2.5 px-3">
                <UiBadge :variant="server.auth_type === 'key' ? 'info' : 'default'" size="sm">
                  {{ server.auth_type === 'key' ? '私钥认证' : '密码认证' }}
                </UiBadge>
              </td>

              <td class="py-2.5 px-3">
                <div v-if="testingServerId === server.id" class="inline-flex items-center gap-1.5 text-2xs text-slate-500">
                  <UiSpinner size="sm" />
                  <span>正在检测连通性…</span>
                </div>
                <div v-else-if="testResults[server.id]" class="inline-flex items-center gap-1.5 text-2xs">
                  <span
                    class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full"
                    :class="testResults[server.id].ok
                      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
                      : 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'"
                  >
                    <span class="w-1.5 h-1.5 rounded-full" :class="testResults[server.id].ok ? 'bg-emerald-500' : 'bg-red-500'" />
                    {{ testResults[server.id].message }}
                  </span>
                </div>
                <span v-else class="text-3xs text-slate-400 select-none">
                  未测试
                </span>
              </td>

              <td class="py-2.5 px-3 text-right">
                <div class="inline-flex items-center justify-end gap-1">
                  <UiButton
                    variant="secondary"
                    size="sm"
                    data-test="open-server-term-btn"
                    @click="openTerminalForServer(server)"
                  >
                    连接终端
                  </UiButton>
                  <UiButton
                    variant="ghost"
                    size="sm"
                    :disabled="testingServerId === server.id"
                    data-test="test-server-btn"
                    @click="onTestServer(server)"
                  >
                    <UiSpinner v-if="testingServerId === server.id" size="sm" class="mr-1 inline" />
                    测试
                  </UiButton>
                  <UiButton
                    variant="ghost"
                    size="sm"
                    data-test="edit-server-btn"
                    @click="openEditDialog(server)"
                  >
                    编辑
                  </UiButton>
                  <UiButton
                    variant="ghost"
                    size="sm"
                    class="text-red-600 hover:text-red-700 dark:text-red-400"
                    data-test="delete-server-btn"
                    @click="handleDelete(server)"
                  >
                    删除
                  </UiButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Dialogs -->
    <ServerDialog
      v-if="showDialog"
      :server="editingServer"
      @close="showDialog = false"
      @saved="onDialogSaved"
    />

    <UiConfirmDialog
      v-if="pendingDelete"
      title="删除服务器"
      :message="`确定要删除服务器「${pendingDelete.name}」吗？`"
      detail="删除后，相关连接凭证将立即清除，已打开的会话将在关闭后不可再连接。"
      confirm-label="删除"
      :loading="isDeleting"
      :error="deleteError"
      @close="pendingDelete = null"
      @confirm="confirmDelete"
    />
  </section>
</template>

<style scoped>
.terminal-settings-view {
  min-height: 100%;
}
</style>