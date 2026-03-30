const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('gistDesktop', {
  isElectron: true,
  request(action, payload = {}) {
    return ipcRenderer.invoke('gist-api', {
      action,
      ...payload
    })
  },
  openExternal(url) {
    return ipcRenderer.invoke('open-external', url)
  },
  copyText(text) {
    return ipcRenderer.invoke('copy-text', text)
  }
})
