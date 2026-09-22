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
  },
  security: {
    getState() {
      return ipcRenderer.invoke('security-state')
    },
    configure(options) {
      return ipcRenderer.invoke('security-configure', options)
    },
    lock() {
      return ipcRenderer.invoke('security-lock')
    },
    unlockWithPassword(password) {
      return ipcRenderer.invoke('security-unlock-password', password)
    },
    unlockWithBiometric() {
      return ipcRenderer.invoke('security-unlock-biometric')
    },
    loadToken() {
      return ipcRenderer.invoke('security-load-token')
    },
    saveToken(token) {
      return ipcRenderer.invoke('security-save-token', token)
    },
    clearToken() {
      return ipcRenderer.invoke('security-clear-token')
    }
  },
  updates: {
    getState() {
      return ipcRenderer.invoke('update-state')
    },
    check() {
      return ipcRenderer.invoke('update-check')
    },
    download() {
      return ipcRenderer.invoke('update-download')
    },
    install() {
      return ipcRenderer.invoke('update-install')
    },
    setChannel(channel) {
      return ipcRenderer.invoke('update-set-channel', channel)
    },
    onStateChanged(callback) {
      const listener = (_, state) => callback(state)
      ipcRenderer.on('update-state-changed', listener)
      return () => ipcRenderer.removeListener('update-state-changed', listener)
    }
  }
})
