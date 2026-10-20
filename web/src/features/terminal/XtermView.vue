<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from '@xterm/addon-fit'
import 'xterm/css/xterm.css'
import { buildTerminalWSUrl } from '../../api/terminal'
import { useTerminalStore } from '../../stores/terminal'

const props = defineProps<{
  tabId: string
  serverId: number
  active: boolean
}>()

const terminalStore = useTerminalStore()
const containerRef = ref<HTMLDivElement | null>(null)

let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null
const isConnected = ref(false)
const connectionError = ref<string | null>(null)

// xterm measures the cell size once at open() and never re-measures when a
// webfont swaps in: neither CharSizeService nor the DOM renderer's WidthCache
// listen to document.fonts. The bundled faces use font-display: swap, so a
// terminal mounted while they are still pending measures with the fallback
// chain — in Chinese locales that ends at Noto Sans Mono CJK (advance 0.5em)
// — while rows later paint with Google Sans Code (advance ~0.615em). Rows
// then exceed their allotted cells and the renderer's per-row overflow:hidden
// clips the tail glyphs (CJK halved, trailing words cut). Waiting for the
// faces BEFORE open keeps measure and render on one set of metrics; a late
// loadingdone event resyncs the renderer the same way for anything still
// racing after open.
const FONT_STACK = 'var(--font-mono, "SF Mono", Monaco, Consolas, "Liberation Mono", monospace)'

let disposed = false

async function waitForTerminalFonts(): Promise<void> {
  try {
    // Literal families: the stacks from --font-mono in style.css. Google Sans
    // Code sizes the ASCII cells; Noto Sans SC paints glyphs that fall through
    // it (CJK), so both must be resolved before the first measurement.
    await document.fonts.load('13px "Google Sans Code"')
    await document.fonts.load('13px "Noto Sans SC"', '汉字')
    await document.fonts.ready
  } catch {
    // No font API (unusual browsers/jsdom): render on the fallback as before.
  }
}

function resyncRendererMetrics(): void {
  if (!term) return
  // RenderService.handleDevicePixelRatioChange re-measures the cell size with
  // the currently resolved fonts, clears the renderer's width cache, resets
  // the default letter spacing and repaints every row — the full self-heal
  // path xterm itself runs on DPR changes, applied here to font swaps. Fit
  // afterwards so the grid adopts the real cell size, then tell the PTY.
  const core = (term as unknown as { _core?: { _renderService?: { handleDevicePixelRatioChange?: () => void } } })._core
  core?._renderService?.handleDevicePixelRatioChange?.()
  handleFit()
  sendResize()
}

function onFontLoadingDone(): void {
  if (disposed || !term) return
  resyncRendererMetrics()
}

function connect() {
  if (!containerRef.value) return

  cleanupWS()
  terminalStore.updateTabStatus(props.tabId, 'connecting')
  connectionError.value = null

  const cols = term?.cols ?? 80
  const rows = term?.rows ?? 24
  const wsUrl = buildTerminalWSUrl(props.serverId, cols, rows)

  try {
    ws = new WebSocket(wsUrl)
    ws.binaryType = 'arraybuffer'
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : 'WebSocket 创建失败'
    connectionError.value = msg
    terminalStore.updateTabStatus(props.tabId, 'error', msg)
    term?.writeln(`\r\n\x1b[31m[连接异常] ${msg}\x1b[0m`)
    return
  }

  ws.onopen = () => {
    isConnected.value = true
    terminalStore.updateTabStatus(props.tabId, 'connected')
    sendResize()
    if (props.active) {
      term?.focus()
    }
  }

  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      term?.write(new Uint8Array(event.data))
    } else if (typeof event.data === 'string') {
      term?.write(event.data)
    }
  }

  ws.onerror = () => {
    const msg = '连接发生错误'
    connectionError.value = msg
    terminalStore.updateTabStatus(props.tabId, 'error', msg)
  }

  ws.onclose = (event) => {
    isConnected.value = false
    const reason = event.reason || (event.wasClean ? '已正常退出' : '连接中断')
    terminalStore.updateTabStatus(props.tabId, 'disconnected', reason)
    term?.writeln(`\r\n\x1b[33m[终端已断开: ${reason}]\x1b[0m`)
  }
}

function sendResize() {
  if (!ws || ws.readyState !== WebSocket.OPEN || !term) return
  ws.send(
    JSON.stringify({
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    }),
  )
}

function handleFit() {
  if (!fitAddon || !term || !containerRef.value) return
  if (containerRef.value.clientWidth === 0 || containerRef.value.clientHeight === 0) return
  try {
    fitAddon.fit()
    sendResize()
  } catch {
    // Ignore fit errors on detached DOM
  }
}

function cleanupWS() {
  if (ws) {
    ws.onopen = null
    ws.onmessage = null
    ws.onerror = null
    ws.onclose = null
    if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
      ws.close()
    }
    ws = null
  }
}

onMounted(() => {
  if (!containerRef.value) return
  void mountTerminal()
})

// Async because the webfont gate may yield: the PTY/connect flow only starts
// once the fonts are resolved (see waitForTerminalFonts above).
async function mountTerminal(): Promise<void> {
  await waitForTerminalFonts()
  if (disposed || !containerRef.value) return

  term = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: FONT_STACK,
    theme: {
      background: '#121316',
      foreground: '#e4e4e7',
      cursor: '#38bdf8',
      cursorAccent: '#121316',
      selectionBackground: 'rgba(56, 189, 248, 0.3)',
      black: '#27272a',
      red: '#ef4444',
      green: '#22c55e',
      yellow: '#eab308',
      blue: '#3b82f6',
      magenta: '#d946ef',
      cyan: '#06b6d4',
      white: '#f4f4f5',
      brightBlack: '#71717a',
      brightRed: '#f87171',
      brightGreen: '#4ade80',
      brightYellow: '#facc15',
      brightBlue: '#60a5fa',
      brightMagenta: '#e879f9',
      brightCyan: '#22d3ee',
      brightWhite: '#ffffff',
    },
    convertEol: true,
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(containerRef.value)

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'input', data }))
    }
  })

  // Watch size changes
  resizeObserver = new ResizeObserver(() => {
    if (props.active) {
      handleFit()
    }
  })
  resizeObserver.observe(containerRef.value)

  // Webfont faces can still resolve after open (page-local faces, user agent
  // font changes): resync renderer metrics so clipped-glyph regressions from a
  // swap never survive.
  document.fonts?.addEventListener?.('loadingdone', onFontLoadingDone)

  connect()

  void nextTick(() => {
    handleFit()
    if (props.active) {
      term?.focus()
    }
  })
}

watch(
  () => props.active,
  (isActive) => {
    if (isActive) {
      void nextTick(() => {
        handleFit()
        term?.focus()
      })
    }
  },
)

onBeforeUnmount(() => {
  disposed = true
  document.fonts?.removeEventListener?.('loadingdone', onFontLoadingDone)
  if (resizeObserver && containerRef.value) {
    resizeObserver.unobserve(containerRef.value)
    resizeObserver.disconnect()
  }
  cleanupWS()
  if (term) {
    term.dispose()
    term = null
  }
  fitAddon = null
})
</script>

<template>
  <div class="xterm-wrapper" :class="{ 'is-active': active }">
    <div ref="containerRef" class="xterm-container" />
    <div v-if="!isConnected" class="terminal-bar">
      <span class="disconnect-notice">
        {{ connectionError ? `连接异常: ${connectionError}` : '终端会话已断开' }}
      </span>
      <button type="button" class="btn-reconnect" @click="connect">
        重新连接
      </button>
    </div>
  </div>
</template>

<style scoped>
.xterm-wrapper {
  position: relative;
  width: 100%;
  height: 100%;
  background: #121316;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  /* The inset lives here, NOT on .xterm-container: FitAddon measures the .xterm
     element and its container but only subtracts the .xterm element's own padding
     — container padding would inflate the proposed cols/rows so the fitted grid
     spills under the viewport scrollbar and past the bottom edge. Measuring a
     padding-free box keeps the last column clear of the scrollbar. */
  padding: 8px 12px;
  box-sizing: border-box;
}

.xterm-container {
  flex: 1;
  min-height: 0;
  width: 100%;
  height: 100%;
}

.xterm-container :deep(.xterm) {
  height: 100%;
}

/* Theme xterm's built-in viewport scrollbar: a slim translucent thumb over a
   transparent track reads as a floating overlay instead of the wide, opaque
   native scrollbar that parked on top of the shell output. */
.xterm-container :deep(.xterm-viewport) {
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.18) transparent;
}

.xterm-container :deep(.xterm-viewport)::-webkit-scrollbar {
  width: 8px;
}

.xterm-container :deep(.xterm-viewport)::-webkit-scrollbar-track {
  background: transparent;
}

.xterm-container :deep(.xterm-viewport)::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.18);
  border-radius: 4px;
}

.xterm-container :deep(.xterm-viewport)::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}

.terminal-bar {
  position: absolute;
  top: 8px;
  right: 16px;
  display: flex;
  align-items: center;
  gap: 10px;
  background: rgba(30, 32, 38, 0.9);
  backdrop-filter: blur(4px);
  border: 1px solid var(--color-border, #3f3f46);
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 12px;
  color: #fbbf24;
  z-index: 10;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.disconnect-notice {
  font-family: var(--font-family);
}

.btn-reconnect {
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  border-radius: 4px;
  padding: 3px 8px;
  font-size: 12px;
  cursor: pointer;
  transition: opacity 0.15s;
}

.btn-reconnect:hover {
  opacity: 0.9;
}
</style>
