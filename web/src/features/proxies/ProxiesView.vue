<script setup lang="ts">
// Proxies view: exposes local directories or port HTTP services via transparent
// contextpath proxies.
// Redesigned with design-taste-frontend app-screens guidelines (framed-sheet,
// flat surfaces, zero resting shadows, tabular figures, pale status pills).
import { ref, onMounted } from 'vue'
import { UiButton, UiBadge, UiEmpty, UiSpinner, UiAlert } from '../../shared/ui'
import ProxyEditDialog from './ProxyEditDialog.vue'
import {
  listProxies,
  deleteProxy,
  toggleProxy,
  type ServiceProxy,
} from '../../api/proxies'

const proxies = ref<ServiceProxy[]>([])
const loading = ref(false)
const errorMessage = ref('')

const isDialogOpen = ref(false)
const activeProxy = ref<ServiceProxy | null>(null)

const copiedId = ref<number | null>(null)

function getFullUrl(alias: string): string {
  const base = window.location.origin
  return `${base}/proxy/${alias}/`
}

async function copyUrl(proxy: ServiceProxy) {
  const url = getFullUrl(proxy.alias)
  try {
    await navigator.clipboard.writeText(url)
    copiedId.value = proxy.id
    setTimeout(() => {
      if (copiedId.value === proxy.id) {
        copiedId.value = null
      }
    }, 1500)
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = url
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
    copiedId.value = proxy.id
    setTimeout(() => {
      if (copiedId.value === proxy.id) {
        copiedId.value = null
      }
    }, 1500)
  }
}

function openUrl(proxy: ServiceProxy) {
  const url = getFullUrl(proxy.alias)
  window.open(url, '_blank')
}

async function fetchProxies() {
  loading.value = true
  errorMessage.value = ''
  try {
    const list = await listProxies()
    proxies.value = Array.isArray(list) ? list : []
  } catch (err: any) {
    errorMessage.value = err?.error || err?.message || '加载代理列表失败'
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  activeProxy.value = null
  isDialogOpen.value = true
}

function openEditDialog(proxy: ServiceProxy) {
  activeProxy.value = proxy
  isDialogOpen.value = true
}

async function handleToggle(proxy: ServiceProxy) {
  try {
    const updated = await toggleProxy(proxy.id)
    const idx = proxies.value.findIndex((p) => p.id === proxy.id)
    if (idx !== -1) {
      proxies.value[idx] = updated
    }
  } catch (err: any) {
    errorMessage.value = err?.error || err?.message || '切换代理状态失败'
  }
}

async function handleDelete(proxy: ServiceProxy) {
  if (!confirm(`确定要删除代理 "${proxy.name}" (alias: ${proxy.alias}) 吗？`)) {
    return
  }
  try {
    await deleteProxy(proxy.id)
    proxies.value = proxies.value.filter((p) => p.id !== proxy.id)
  } catch (err: any) {
    errorMessage.value = err?.error || err?.message || '删除代理失败'
  }
}

function onDialogSaved(saved: ServiceProxy) {
  isDialogOpen.value = false
  const idx = proxies.value.findIndex((p) => p.id === saved.id)
  if (idx !== -1) {
    proxies.value[idx] = saved
  } else {
    proxies.value.unshift(saved)
  }
}

onMounted(() => {
  fetchProxies()
})
</script>

<template>
  <section class="proxies-view w-full space-y-6" aria-label="服务代理管理">
    <!-- Header -->
    <header class="flex items-center justify-between pb-4 border-b border-slate-200 dark:border-slate-800 gap-4 flex-wrap">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-[22px] font-semibold text-slate-900 dark:text-slate-100 tracking-tight">服务代理与 Demo 预览</h1>
          <UiBadge variant="neutral" size="sm">ContextPath Proxy</UiBadge>
        </div>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-1">
          将服务器上的静态构建目录或本地端口 HTTP 服务以平台端口的独立二级路径（<code>/proxy/:alias/</code>）直接暴露访问，支持免登录分享与 WebSocket HMR 热更新。
        </p>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <UiButton variant="secondary" size="sm" :disabled="loading" @click="fetchProxies">
          刷新
        </UiButton>
        <UiButton variant="primary" size="sm" @click="openCreateDialog">
          + 新建代理
        </UiButton>
      </div>
    </header>

    <UiAlert v-if="errorMessage" variant="danger" dismissible @dismiss="errorMessage = ''">
      {{ errorMessage }}
    </UiAlert>

    <!-- Loading State -->
    <div v-if="loading && proxies.length === 0" class="py-16 flex flex-col items-center justify-center gap-3 text-slate-400 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800">
      <UiSpinner size="lg" />
      <span class="text-xs">正在加载服务代理列表…</span>
    </div>

    <!-- Empty State -->
    <div v-else-if="proxies.length === 0" class="py-12 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-lg">
      <UiEmpty
        title="暂无服务代理规则"
        description="您可以将服务器上生成的网页产物目录或本地运行的 Web 端口（如 Vite 5173、后端 8080）暴露为独立的 ContextPath，直接通过浏览器访问或分享。"
      >
        <UiButton variant="primary" size="sm" class="mt-2" @click="openCreateDialog">
          创建第一个代理
        </UiButton>
      </UiEmpty>
    </div>

    <!-- Proxies Cards List -->
    <div v-else class="flex flex-col gap-3.5">
      <article
        v-for="p in proxies"
        :key="p.id"
        class="proxy-card bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-lg p-5 space-y-3.5 hover:border-slate-300 dark:hover:border-slate-700 transition-colors"
      >
        <!-- Row 1: Title, Badges and Controls -->
        <div class="flex items-center justify-between gap-3 flex-wrap">
          <div class="flex items-center gap-2.5 min-w-0">
            <span class="font-semibold text-sm text-slate-900 dark:text-slate-100 truncate">
              {{ p.name }}
            </span>
            <UiBadge :variant="p.type === 'directory' ? 'neutral' : 'info'" size="sm">
              {{ p.type === 'directory' ? '静态目录' : '端口反代' }}
            </UiBadge>
            <template v-if="p.type === 'port'">
              <span
                class="inline-flex items-center gap-1.5 text-[11px] px-2 py-0.5 rounded-full"
                :class="p.online ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400'"
              >
                <span class="w-1.5 h-1.5 rounded-full" :class="p.online ? 'bg-emerald-500' : 'bg-slate-400'"></span>
                {{ p.online ? '服务监听中' : '端口未连接' }}
              </span>
            </template>
          </div>

          <div class="flex items-center gap-2.5 shrink-0">
            <label class="toggle-control inline-flex items-center cursor-pointer select-none text-xs text-slate-600 dark:text-slate-400">
              <input
                type="checkbox"
                :checked="p.enabled"
                class="sr-only peer"
                @change="handleToggle(p)"
              />
              <div class="w-8 h-4 bg-slate-200 peer-focus:outline-none rounded-full peer dark:bg-slate-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-transform dark:border-slate-600 peer-checked:bg-blue-600 relative"></div>
              <span class="ml-1.5 text-xs">{{ p.enabled ? '已启用' : '已停用' }}</span>
            </label>

            <UiButton variant="ghost" size="sm" @click="openEditDialog(p)">
              编辑
            </UiButton>
            <UiButton variant="ghost" size="sm" class="text-red-500 hover:text-red-600 dark:text-red-400" @click="handleDelete(p)">
              删除
            </UiButton>
          </div>
        </div>

        <!-- Row 2: Access ContextPath Link Box -->
        <div class="bg-slate-50 dark:bg-slate-800/50 rounded-md px-3.5 py-2.5 border border-slate-200/80 dark:border-slate-700/60 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2 min-w-0 text-xs text-slate-700 dark:text-slate-300">
            <span class="text-slate-400 select-none text-[11px]">ContextPath:</span>
            <code class="font-semibold text-blue-600 dark:text-blue-400 truncate">{{ getFullUrl(p.alias) }}</code>
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            <UiButton variant="secondary" size="xs" @click="copyUrl(p)">
              <svg v-if="copiedId === p.id" class="inline-block mr-1" viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="3 8 6 11 13 4" />
              </svg>
              <span>{{ copiedId === p.id ? '已复制' : '复制链接' }}</span>
            </UiButton>
            <UiButton variant="secondary" size="xs" :disabled="!p.enabled" @click="openUrl(p)">
              在浏览器打开 ↗
            </UiButton>
          </div>
        </div>

        <!-- Row 3: Target Details & Metadata -->
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between items-start gap-1 text-xs text-slate-500 dark:text-slate-400 pt-0.5">
          <div class="truncate max-w-lg flex items-center gap-1.5">
            <span class="text-slate-400">映射目标：</span>
            <code class="font-mono text-slate-700 dark:text-slate-300 text-[11px] truncate">
              {{ p.type === 'directory' ? p.target : `127.0.0.1:${p.target}` }}
            </code>
          </div>
          <div class="text-[11px] shrink-0 text-slate-400 tabular-nums">
            更新于 {{ new Date(p.updatedAt).toLocaleString() }}
          </div>
        </div>
      </article>
    </div>

    <!-- Create / Edit Dialog -->
    <ProxyEditDialog
      v-if="isDialogOpen"
      :proxy="activeProxy"
      @close="isDialogOpen = false"
      @saved="onDialogSaved"
    />
  </section>
</template>
