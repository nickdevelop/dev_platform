import { contextBridge, ipcRenderer } from 'electron'
import { DesktopIpcApi, DesktopStatus, ServerConnection, TestConnectionResult } from '../shared/types'

const api: DesktopIpcApi = {
  getConnections: () => ipcRenderer.invoke('desktop:get-connections'),
  saveConnection: (connection) => ipcRenderer.invoke('desktop:save-connection', connection),
  removeConnection: (id) => ipcRenderer.invoke('desktop:remove-connection', id),
  switchConnection: (id) => ipcRenderer.invoke('desktop:switch-connection', id),
  getStatus: () => ipcRenderer.invoke('desktop:get-status'),
  testConnection: (url, token) => ipcRenderer.invoke('desktop:test-connection', url, token),
  openDirectoryDialog: (defaultPath) => ipcRenderer.invoke('desktop:open-directory-dialog', defaultPath),
  onStatusChanged: (callback) => {
    const subscription = (_event: Electron.IpcRendererEvent, status: DesktopStatus) => callback(status)
    ipcRenderer.on('desktop:status-changed', subscription)
    return () => {
      ipcRenderer.removeListener('desktop:status-changed', subscription)
    }
  },
}

contextBridge.exposeInMainWorld('desktopAPI', api)
