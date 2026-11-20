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

// Local shell configuration state
const customShellPath = ref('')
const customShellArgs = ref('')
const isSavingShell = ref(false)
const shellSuccessMsg = ref('')
const shellValidationMsg = ref<{ ok: boolean; message: string } | null>(null)
const isValidatingShell = ref(false)

async function fetchData() {
  loading.value = true
  errorMessage.value = ''
  try {
    await Promise.all([
      terminalStore.loadStatus(),
      terminalStore.loadServers(),
      terminalStore.loadLocalShell(),
    ])
    if (terminalStore.localShell) {
      customShellPath.value = terminalStore.localShell.path || ''
      customShellArgs.value = (terminalStore.localShell.args || []).join(' ')
    }
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : '加载终端服务与配置失败'
  } finally {
    loading.value = false
  }
}

async function onScanShells() {
  shellValidationMsg.value = null
  shellSuccessMsg.value = ''
  try {
    const list = await terminalStore.scanShells()
    if (list.length > 0) {
      shellSuccessMsg.value = `扫描完成，发现 ${list.length} 个可用本地终端`
    } else {
      shellValidationMsg.value = { ok: false, message: '未扫描到可用 Shell' }
    }
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : '扫描失败'
  }
}

function selectDiscoveredShell(shell: { path: string; args: string[] }) {
  customShellPath.value = shell.path
  customShellArgs.value = (shell.args || []).join(' ')
  shellValidationMsg.value = null
  shellSuccessMsg.value = ''
}

async function onValidateShell() {
  if (!customShellPath.value.trim()) {
    shellValidationMsg.value = { ok: false, message: '请先输入 Shell 路径' }
    return
  }
  isValidatingShell.value = true
  shellValidationMsg.value = null
  shellSuccessMsg.value = ''
  try {
    const res = await terminalStore.validateLocalShell(customShellPath.value.trim())
    shellValidationMsg.value = { ok: res.ok, message: res.message || '路径有效且可执行' }
  } catch (err: unknown) {
    shellValidationMsg.value = {
      ok: false,
      message: err instanceof Error ? err.message : '路径不可达或不可执行',
    }
  } finally {
    isValidatingShell.value = false
  }
}

async function onSaveShell() {
  if (!customShellPath.value.trim()) {
    shellValidationMsg.value = { ok: false, message: 'Shell 路径不能为空' }
    return
  }
  isSavingShell.value = true
  shellValidationMsg.value = null
  shellSuccessMsg.value = ''
  try {
    const args = customShellArgs.value.trim() ? customShellArgs.value.trim().split(/\s+/) : []
    await terminalStore.saveLocalShell({
      path: customShellPath.value.trim(),
      args,
    })
    shellSuccessMsg.value = '本地终端 Shell 设置已保存并即时生效'
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : '保存本地终端设置失败'
  } finally {
    isSavingShell.value = false
  }
}

async function onResetShell() {
  isSavingShell.value = true
  shellValidationMsg.value = null
  shellSuccessMsg.value = ''
  try {
    const reset = await terminalStore.resetLocalShell()
    customShellPath.value = reset.path || ''
    customShellArgs.value = (reset.args || []).join(' ')
    shellSuccessMsg.value = '已恢复系统默认 Shell 配置'
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : '恢复默认设置失败'
  } finally {
    isSavingShell.value = false
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
    <section class="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-800 p-4 space-y-4" aria-label="本地宿主环境">
      <div class="flex items-center justify-between flex-wrap gap-2 pb-3 border-b border-slate-100 dark:border-slate-800">
        <div class="flex items-center gap-2.5">
          <span class="font-semibold text-sm text-slate-900 dark:text-slate-100">本地宿主环境</span>
          <UiBadge variant="default" size="sm">Localhost</UiBadge>
          <span class="text-2xs font-mono px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400">
            {{ terminalStore.status?.os || 'unknown' }}
          </span>
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
        {{ terminalStore.status?.message || '当前操作系统环境不支持本地 PTY 终端；您仍可添加并连接远程 SSH 服务器。' }}
      </UiAlert>

      <!-- Supported: Local Shell Management -->
      <div v-else class="space-y-4">
        <!-- Effective Shell Summary -->
        <div class="flex items-center justify-between flex-wrap gap-2 text-xs bg-slate-50 dark:bg-slate-800/40 p-3 rounded border border-slate-100 dark:border-slate-800">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="text-slate-500 dark:text-slate-400">当前生效 Shell：</span>
              <span class="font-medium text-slate-800 dark:text-slate-200">
                {{ terminalStore.localShell?.name || '默认终端' }}
              </span>
              <UiBadge
                v-if="terminalStore.localShell?.is_recommended"
                variant="success"
                size="sm"
              >
                推荐
              </UiBadge>
              <UiBadge
                :variant="terminalStore.localShell?.is_custom ? 'info' : 'default'"
                size="sm"
              >
                {{ terminalStore.localShell?.is_custom ? '自定义配置' : '系统默认' }}
              </UiBadge>
            </div>
            <div class="font-mono text-2xs text-slate-500 dark:text-slate-400">
              <code>{{ terminalStore.localShell?.path || '-' }}</code>
              <span v-if="terminalStore.localShell?.args?.length" class="ml-2 text-slate-400">
                参数: {{ terminalStore.localShell.args.join(' ') }}
              </span>
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <UiButton
              variant="secondary"
              size="sm"
              :disabled="terminalStore.scanningShells"
              data-test="scan-shells-btn"
              @click="onScanShells"
            >
              <UiSpinner v-if="terminalStore.scanningShells" size="sm" class="mr-1 inline" />
              <svg v-else viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="mr-1 inline">
                <circle cx="8" cy="8" r="6" />
                <path d="M8 5v6M5 8h6" />
              </svg>
              扫描本地 Shell
            </UiButton>
          </div>
        </div>

        <!-- Discovered Shells Grid (after scan or if available) -->
        <div v-if="terminalStore.discoveredShells.length > 0" class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-slate-700 dark:text-slate-300">已探测到的本地 Shell 候选</span>
            <span class="text-2xs text-slate-400">点击卡片可快速填入并应用</span>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5">
            <div
              v-for="sh in terminalStore.discoveredShells"
              :key="sh.id + sh.path"
              class="p-2.5 rounded border transition-colors cursor-pointer text-left space-y-1 relative"
              :class="customShellPath === sh.path
                ? 'border-blue-500 bg-blue-50/40 dark:bg-blue-950/20'
                : 'border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:border-slate-300 dark:hover:border-slate-700'"
              @click="selectDiscoveredShell(sh)"
            >
              <div class="flex items-center justify-between gap-1.5">
                <span class="text-xs font-medium text-slate-800 dark:text-slate-200 truncate">{{ sh.name }}</span>
                <UiBadge v-if="sh.isRecommended" variant="success" size="sm">推荐</UiBadge>
              </div>
              <div class="text-2xs font-mono text-slate-500 dark:text-slate-400 truncate" :title="sh.path">
                {{ sh.path }}
              </div>
              <div v-if="sh.args?.length" class="text-3xs text-slate-400 truncate">
                参数: {{ sh.args.join(' ') }}
              </div>
            </div>
          </div>
        </div>

        <!-- Custom Shell Setting Form -->
        <div class="space-y-3 pt-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-slate-800 dark:text-slate-200">Shell 命令与路径配置</span>
            <span class="text-2xs text-slate-400">可从扫描列表选择，也可手动输入任意有效绝对路径</span>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div class="sm:col-span-2 space-y-1">
              <label class="block text-2xs font-medium text-slate-600 dark:text-slate-400">可执行文件绝对路径</label>
              <input
                v-model="customShellPath"
                type="text"
                placeholder="例如 C:\Program Files\Git\bin\bash.exe 或 /bin/bash"
                class="w-full px-2.5 py-1.5 text-xs bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded text-slate-800 dark:text-slate-200 placeholder-slate-400 focus:outline-none focus:border-blue-500 font-mono"
                data-test="shell-path-input"
              />
            </div>
            <div class="space-y-1">
              <label class="block text-2xs font-medium text-slate-600 dark:text-slate-400">启动参数（空格分隔）</label>
              <input
                v-model="customShellArgs"
                type="text"
                placeholder="例如 --login -i"
                class="w-full px-2.5 py-1.5 text-xs bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded text-slate-800 dark:text-slate-200 placeholder-slate-400 focus:outline-none focus:border-blue-500 font-mono"
                data-test="shell-args-input"
              />
            </div>
          </div>

          <!-- Actions & Feedback -->
          <div class="flex items-center justify-between flex-wrap gap-2 pt-1">
            <div class="flex items-center gap-2">
              <UiButton
                variant="primary"
                size="sm"
                :disabled="isSavingShell || !customShellPath.trim()"
                data-test="save-shell-btn"
                @click="onSaveShell"
              >
                <UiSpinner v-if="isSavingShell" size="sm" class="mr-1 inline" />
                保存配置
              </UiButton>
              <UiButton
                variant="secondary"
                size="sm"
                :disabled="isValidatingShell || !customShellPath.trim()"
                data-test="validate-shell-btn"
                @click="onValidateShell"
              >
                <UiSpinner v-if="isValidatingShell" size="sm" class="mr-1 inline" />
                校验路径
              </UiButton>
              <UiButton
                variant="ghost"
                size="sm"
                :disabled="isSavingShell || !terminalStore.localShell?.is_custom"
                data-test="reset-shell-btn"
                @click="onResetShell"
              >
                恢复默认
              </UiButton>
            </div>

            <div v-if="shellSuccessMsg" class="text-xs text-emerald-600 dark:text-emerald-400 inline-flex items-center gap-1">
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 8.5l3.5 3.5L13 5" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
              <span>{{ shellSuccessMsg }}</span>
            </div>
            <div v-else-if="shellValidationMsg" class="text-xs inline-flex items-center gap-1" :class="shellValidationMsg.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
              <span class="w-1.5 h-1.5 rounded-full" :class="shellValidationMsg.ok ? 'bg-emerald-500' : 'bg-red-500'" />
              <span>{{ shellValidationMsg.message }}</span>
            </div>
          </div>
        </div>
      </div>
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