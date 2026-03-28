const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('gistDesktop', {
  apiBase: 'http://127.0.0.1:3000/api',
  isElectron: true,
  openExternal(url) {
    return ipcRenderer.invoke('open-external', url)
  },
  copyText(text) {
    return ipcRenderer.invoke('copy-text', text)
  }
})
