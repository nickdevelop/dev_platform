<script setup lang="ts">
import { getCurrentInstance, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useProjectTerminalStore, type ProjectTerminalWindow } from '../../stores/projectTerminal'
import { useTerminalStore } from '../../stores/terminal'
import XtermView from './XtermView.vue'

const store = useProjectTerminalStore()
const terminalStore = useTerminalStore()
const vm = getCurrentInstance()
let router: ReturnType<typeof useRouter> | null = null
try {
  router = useRouter()
} catch {
  router = vm?.appContext.config.globalProperties.$router || null
}
if (!router && vm?.appContext.config.globalProperties.$router) {
  router = vm.appContext.config.globalProperties.$router
}

// Dragging state for individual floating windows
const draggingId = ref<string | null>(null)
let dragStartX = 0
let dragStartY = 0
let initialLeft = 0
let initialTop = 0

// Dropdown menu state
const openMenuWindowId = ref<string | null>(null)

onMounted(async () => {
  await Promise.all([terminalStore.loadStatus(), terminalStore.loadServers()])
  window.addEventListener('pointerdown', onGlobalPointerDown)
})

function onGlobalPointerDown(e: PointerEvent) {
  const target = e.target as HTMLElement | null
  if (!target?.closest('.new-tab-menu') && !target?.closest('.new-tab-btn')) {
    openMenuWindowId.value = null
  }
}

function onPointerDownHeader(e: PointerEvent, instance: ProjectTerminalWindow) {
  // Bring to front on any interaction
  store.bringToFront(instance.id)

  // Only start drag on primary button and ignore control buttons/tabs/menu
  if (e.button !== 0) return
  const target = e.target as HTMLElement | null
  if (target?.closest('button') || target?.closest('.modal-tab-item') || target?.closest('.new-tab-menu')) return

  draggingId.value = instance.id
  dragStartX = e.clientX
  dragStartY = e.clientY
  initialLeft = instance.left
  initialTop = instance.top

  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp)
  window.addEventListener('pointercancel', onPointerUp)
}

function onPointerMove(e: PointerEvent) {
  if (!draggingId.value) return
  const inst = store.windows.find((i) => i.id === draggingId.value)
  if (!inst) return

  const dx = e.clientX - dragStartX
  const dy = e.clientY - dragStartY

  const maxLeft = Math.max(0, window.innerWidth - 120)
  const maxTop = Math.max(0, window.innerHeight - 50)

  const newLeft = Math.min(Math.max(0, initialLeft + dx), maxLeft)
  const newTop = Math.min(Math.max(0, initialTop + dy), maxTop)

  store.updatePosition(inst.id, newLeft, newTop)
}

function onPointerUp() {
  if (!draggingId.value) return
  draggingId.value = null
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerUp)
}

onBeforeUnmount(() => {
  onPointerUp()
  window.removeEventListener('pointerdown', onGlobalPointerDown)
})

function onWindowPointerDown(instance: ProjectTerminalWindow) {
  store.bringToFront(instance.id)
}

function handleMinimize(id: string) {
  store.minimize(id)
}

function handleClose(id: string) {
  store.closeTerminal(id)
}

function toggleNewTabMenu(windowId: string) {
  if (openMenuWindowId.value === windowId) {
    openMenuWindowId.value = null
  } else {
    openMenuWindowId.value = windowId
    void terminalStore.loadServers()
  }
}

function createLocalTab(windowId: string) {
  openMenuWindowId.value = null
  store.openLocalTab(undefined, undefined, windowId)
}

function createServerTab(serverId: number, serverName: string, windowId: string) {
  openMenuWindowId.value = null
  store.openServerTab(serverId, serverName, windowId)
}

function goToSettings() {
  openMenuWindowId.value = null
  if (router?.push) {
    void router.push({ name: 'settings-terminal' })
  }
}
</script>

<template>
  <Teleport to="body">
    <!-- Render each terminal window independently -->
    <template v-for="instance in store.windows" :key="instance.id">
      <div
        v-show="!instance.isMinimized"
        class="project-terminal-window fixed flex flex-col bg-[#121316] text-platform-text rounded border border-white/15 shadow-2xl overflow-hidden select-none"
        :class="{ 'is-dragging': draggingId === instance.id }"
        :style="{
          left: `${instance.left}px`,
          top: `${instance.top}px`,
          zIndex: instance.zIndex,
          width: '800px',
          height: '500px',
          maxWidth: '95vw',
          maxHeight: '90vh',
        }"
        data-test="project-terminal-modal"
        :data-terminal-id="instance.id"
        @pointerdown="onWindowPointerDown(instance)"
      >
        <!-- Draggable title bar / tab strip -->
        <div
          class="terminal-title-bar flex items-center justify-between px-2 py-1.5 bg-[#1a1b20] border-b border-white/10 shrink-0 cursor-grab active:cursor-grabbing min-h-[38px] relative z-20"
          data-test="terminal-title-bar"
          @pointerdown="onPointerDownHeader($event, instance)"
        >
          <!-- Left: Main Window renders Tabs; Detached Window renders Title -->
          <div class="flex items-center min-w-0 flex-1 mr-2 overflow-visible relative">
            <!-- Terminal branding icon -->
            <span class="terminal-icon inline-flex items-center justify-center text-platform-primary shrink-0 mr-2 ml-1 pointer-events-none">
              <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M3 4.5l3.5 3.5L3 11.5M8.5 11.5h4" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
            </span>

            <!-- Main Window: Tabs Strip -->
            <template v-if="instance.isMain">
              <div class="terminal-tabs-container flex items-center gap-1 overflow-x-auto overflow-y-hidden max-w-[calc(100%-40px)] min-w-0 flex-shrink">
                <div
                  v-for="tab in store.tabs.filter((t) => t.windowId === instance.id)"
                  :key="tab.id"
                  class="modal-tab-item flex items-center gap-1.5 px-2.5 py-1 rounded text-xs transition-colors cursor-pointer shrink-0 border"
                  :class="[
                    tab.id === instance.activeTabId
                      ? 'bg-[#121316] text-white border-white/20 font-medium'
                      : 'bg-white/5 text-slate-400 hover:text-slate-200 hover:bg-white/10 border-transparent',
                  ]"
                  data-test="modal-tab-item"
                  :title="tab.title"
                  @click.stop="store.setActiveTab(instance.id, tab.id)"
                >
                  <!-- Status dot -->
                  <span
                    class="status-dot w-1.5 h-1.5 rounded-full shrink-0"
                    :class="{
                      'bg-emerald-500 shadow-[0_0_4px_#10b981]': tab.status === 'connected',
                      'bg-amber-400 shadow-[0_0_4px_#f59e0b]': tab.status === 'connecting',
                      'bg-red-500 shadow-[0_0_4px_#ef4444]': tab.status === 'error',
                      'bg-slate-500': tab.status === 'disconnected',
                    }"
                  />
                  <!-- Title text -->
                  <span class="tab-title truncate max-w-[150px]">{{ tab.title }}</span>

                  <!-- Detach button (pop-out to standalone window) -->
                  <button
                    type="button"
                    class="tab-btn detach-btn inline-flex items-center justify-center w-4 h-4 rounded text-slate-400 hover:text-white hover:bg-white/15 transition-colors cursor-pointer"
                    title="分离为单独窗口"
                    aria-label="分离为单独窗口"
                    data-test="detach-tab-btn"
                    @pointerdown.stop
                    @click.stop="store.detachTab(tab.id)"
                  >
                    <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M10 2h4v4M14 2L8.5 7.5M6 4H3a1 1 0 0 0-1 1v8a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1v-3" />
                    </svg>
                  </button>

                  <!-- Close tab button -->
                  <button
                    type="button"
                    class="tab-btn close-tab-btn inline-flex items-center justify-center w-4 h-4 rounded text-slate-400 hover:text-white hover:bg-red-500/80 transition-colors cursor-pointer"
                    title="关闭标签"
                    aria-label="关闭标签"
                    data-test="close-tab-btn"
                    @pointerdown.stop
                    @click.stop="store.closeTab(tab.id)"
                  >
                    <svg viewBox="0 0 16 16" width="9" height="9" fill="none" stroke="currentColor" stroke-width="2">
                      <line x1="4" y1="4" x2="12" y2="12" stroke-linecap="round" />
                      <line x1="12" y1="4" x2="4" y2="12" stroke-linecap="round" />
                    </svg>
                  </button>
                </div>
              </div>

              <!-- '+' Add Tab Button with Dropdown -->
              <div class="relative inline-block ml-1 shrink-0 overflow-visible">
                <button
                  type="button"
                  class="new-tab-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-400 hover:text-white hover:bg-white/10 transition-colors cursor-pointer"
                  title="新建终端连接"
                  aria-label="新建终端连接"
                  data-test="new-terminal-tab-btn"
                  @pointerdown.stop
                  @click.stop="toggleNewTabMenu(instance.id)"
                >
                  <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                    <line x1="8" y1="3" x2="8" y2="13" />
                    <line x1="3" y1="8" x2="13" y2="8" />
                  </svg>
                </button>

                <!-- Dropdown Popover -->
                <div
                  v-if="openMenuWindowId === instance.id"
                  class="new-tab-menu absolute left-0 top-full mt-1.5 w-60 bg-[#1a1b20] border border-white/20 rounded-md shadow-2xl py-1 z-50 text-xs backdrop-blur-md"
                  data-test="new-tab-dropdown"
                  @pointerdown.stop
                >
                  <button
                    type="button"
                    class="w-full text-left px-3 py-1.5 flex items-center gap-2 hover:bg-white/10 text-white cursor-pointer transition-colors"
                    data-test="menu-item-local"
                    @click.stop="createLocalTab(instance.id)"
                  >
                    <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="text-emerald-400">
                      <rect x="2" y="3" width="12" height="10" rx="1.5" />
                      <path d="M5 7l2 1.5-2 1.5M9 10h2" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                    <span>新建本地终端 (Localhost)</span>
                  </button>

                  <div class="border-t border-white/10 my-1" />
                  <div class="px-3 py-1 text-3xs font-medium text-slate-400 uppercase tracking-wider">
                    第三方服务器
                  </div>

                  <template v-if="terminalStore.servers.filter((s) => !s.is_local && s.id !== 0).length > 0">
                    <button
                      v-for="server in terminalStore.servers.filter((s) => !s.is_local && s.id !== 0)"
                      :key="server.id"
                      type="button"
                      class="w-full text-left px-3 py-1.5 flex items-center justify-between hover:bg-white/10 text-slate-200 cursor-pointer transition-colors"
                      data-test="menu-item-server"
                      @click.stop="createServerTab(server.id, server.name, instance.id)"
                    >
                      <span class="truncate font-medium">{{ server.name }}</span>
                      <span class="text-3xs text-slate-400 font-mono">{{ server.host }}:{{ server.port }}</span>
                    </button>
                  </template>
                  <div v-else class="px-3 py-1.5 text-slate-500 text-3xs italic">
                    暂无配置的第三方服务器
                  </div>

                  <div class="border-t border-white/10 my-1" />
                  <button
                    type="button"
                    class="w-full text-left px-3 py-1.5 flex items-center gap-2 hover:bg-white/10 text-platform-primary cursor-pointer transition-colors"
                    data-test="menu-item-manage"
                    @click.stop="goToSettings"
                  >
                    <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                      <circle cx="8" cy="8" r="2.5" />
                      <path d="M8 1.5v1.5M8 13v1.5M1.5 8h1.5M13 8h1.5M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M3.4 12.6l1.1-1.1M11.5 4.5l1.1-1.1" stroke-linecap="round" />
                    </svg>
                    <span>管理服务器...</span>
                  </button>
                </div>
              </div>
            </template>

            <!-- Detached Window: Single Tab Title -->
            <template v-else>
              <span class="font-medium text-xs text-white truncate pointer-events-none mr-2">
                终端 · {{ instance.projectName }}
              </span>
              <span
                v-if="instance.cwd"
                class="path-badge text-3xs font-mono px-1.5 py-0.5 rounded bg-white/5 text-slate-400 border border-white/10 truncate max-w-[360px] pointer-events-none"
                :title="instance.cwd"
              >
                {{ instance.cwd }}
              </span>
            </template>
          </div>

          <!-- Window action buttons -->
          <div class="flex items-center gap-1 shrink-0">
            <!-- If detached: Attach back to Main Tab Strip button -->
            <button
              v-if="!instance.isMain"
              type="button"
              class="window-btn attach-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-400 hover:text-white hover:bg-white/10 transition-colors cursor-pointer"
              title="附加回主窗口标签页"
              aria-label="附加到主窗口"
              data-test="terminal-attach-btn"
              @click.stop="store.attachTab(instance.activeTabId)"
            >
              <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 6V2h-4M14 2L8.5 7.5M2 10v3a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V9" />
              </svg>
            </button>

            <!-- Minimize button -->
            <button
              type="button"
              class="window-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-400 hover:text-white hover:bg-white/10 transition-colors cursor-pointer"
              title="最小化到快捷栏"
              aria-label="最小化终端"
              data-test="terminal-minimize-btn"
              @click.stop="handleMinimize(instance.id)"
            >
              <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                <line x1="3" y1="8" x2="13" y2="8" stroke-linecap="round" />
              </svg>
            </button>

            <!-- Close button -->
            <button
              type="button"
              class="window-btn close-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-400 hover:text-white hover:bg-red-500/80 transition-colors cursor-pointer"
              title="关闭并退出终端"
              aria-label="关闭终端"
              data-test="terminal-close-btn"
              @click.stop="handleClose(instance.id)"
            >
              <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="4" y1="4" x2="12" y2="12" stroke-linecap="round" />
                <line x1="12" y1="4" x2="4" y2="12" stroke-linecap="round" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Terminal body: Renders XtermView for each tab in this window -->
        <div class="terminal-window-body flex-1 min-h-0 bg-[#121316] relative overflow-hidden select-text">
          <template v-for="tab in store.tabs.filter((t) => t.windowId === instance.id)" :key="tab.id">
            <div
              v-show="tab.id === instance.activeTabId"
              class="w-full h-full"
              :data-tab-content="tab.id"
            >
              <XtermView
                :tab-id="tab.id"
                :server-id="tab.serverId"
                :cwd="tab.cwd"
                :active="!instance.isMinimized && tab.id === instance.activeTabId"
                :visible="!instance.isMinimized && tab.id === instance.activeTabId"
                style="width: 100%; height: 100%;"
              />
            </div>
          </template>
        </div>
      </div>
    </template>
  </Teleport>
</template>

<style scoped>
.project-terminal-window {
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.55), 0 2px 8px rgba(0, 0, 0, 0.35);
  transition: box-shadow 0.15s ease;
}

.project-terminal-window.is-dragging {
  opacity: 0.96;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.65);
}

.terminal-tabs-container {
  scrollbar-width: none;
}
.terminal-tabs-container::-webkit-scrollbar {
  display: none;
}
</style>
