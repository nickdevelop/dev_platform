import { useEffect, useRef } from 'react'
import { Terminal } from 'xterm'
import { FitAddon } from '@xterm/addon-fit'
import 'xterm/css/xterm.css'
import { buildTerminalWSUrl } from '../../api/terminal'
import styles from './XtermPane.module.css'

interface XtermPaneProps {
  serverId: number
}

export function XtermPane({ serverId }: XtermPaneProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const socketRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    let disposed = false
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "'Google Sans Code', 'Noto Sans SC', monospace",
      theme: {
        background: '#121316',
        foreground: '#e4e4e7',
        cursor: '#38bdf8',
        selectionBackground: 'rgba(56, 189, 248, 0.3)',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)
    termRef.current = term
    fitRef.current = fit

    function sendResize() {
      const socket = socketRef.current
      if (!socket || socket.readyState !== WebSocket.OPEN) return
      socket.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    }

    function connect() {
      const socket = new WebSocket(buildTerminalWSUrl(serverId, term.cols, term.rows))
      socket.binaryType = 'arraybuffer'
      socketRef.current = socket
      socket.onopen = () => {
        fit.fit()
        sendResize()
        term.focus()
      }
      socket.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) term.write(new Uint8Array(event.data))
        else if (typeof event.data === 'string') term.write(event.data)
      }
      socket.onclose = () => {
        term.writeln('\r\n\x1b[33m[终端已断开]\x1b[0m')
      }
    }

    function handleData(data: string) {
      const socket = socketRef.current
      if (socket && socket.readyState === WebSocket.OPEN) socket.send(data)
    }

    const disposable = term.onData(handleData)
    const observer = new ResizeObserver(() => {
      try {
        fit.fit()
        sendResize()
      } catch {
        // Container may be detached during teardown.
      }
    })
    observer.observe(container)
    connect()

    return () => {
      disposed = true
      observer.disconnect()
      disposable.dispose()
      if (socketRef.current) {
        socketRef.current.onclose = null
        socketRef.current.close()
        socketRef.current = null
      }
      term.dispose()
      termRef.current = null
      fitRef.current = null
      void disposed
    }
  }, [serverId])

  return <div className={styles.container} ref={containerRef} data-test="xterm-container" />
}
