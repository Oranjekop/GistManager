const { app, BrowserWindow } = require('electron')
const { autoUpdater } = require('electron-updater')
const fs = require('node:fs')
const path = require('node:path')

let initialized = false
let updateInfo = null
let state = {
  status: 'idle',
  currentVersion: app.getVersion(),
  availableVersion: '',
  percent: 0,
  transferred: 0,
  total: 0,
  message: '',
  channel: app.getVersion().includes('-') ? 'beta' : 'stable'
}

function configPath() {
  return path.join(app.getPath('userData'), 'updater.json')
}

function readChannel() {
  try {
    const config = JSON.parse(fs.readFileSync(configPath(), 'utf8'))
    return config.channel === 'beta' ? 'beta' : 'stable'
  } catch {
    return app.getVersion().includes('-') ? 'beta' : 'stable'
  }
}

function writeChannel(channel) {
  const target = configPath()
  fs.mkdirSync(path.dirname(target), { recursive: true })
  fs.writeFileSync(target, JSON.stringify({ channel }, null, 2), 'utf8')
}

function applyChannel(channel) {
  const isBeta = channel === 'beta'
  autoUpdater.allowPrerelease = isBeta
  autoUpdater.allowDowngrade = !isBeta && app.getVersion().includes('-')

  if (isBeta) {
    autoUpdater.setFeedURL({
      provider: 'generic',
      url: 'https://github.com/Oranjekop/GistManager/releases/download/pre-release',
      channel: 'beta'
    })
  } else {
    autoUpdater.setFeedURL({
      provider: 'github',
      owner: 'Oranjekop',
      repo: 'GistManager'
    })
  }
}

function broadcast() {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) {
      window.webContents.send('update-state-changed', state)
    }
  }
}

function setState(patch) {
  state = { ...state, ...patch }
  broadcast()
  return state
}

function initialize() {
  if (initialized) {
    return
  }
  initialized = true

  autoUpdater.autoDownload = false
  autoUpdater.autoInstallOnAppQuit = false
  const channel = readChannel()
  applyChannel(channel)
  state = { ...state, channel }

  autoUpdater.on('checking-for-update', () => {
    setState({ status: 'checking', message: '正在检查更新…', percent: 0 })
  })

  autoUpdater.on('update-available', (info) => {
    updateInfo = info
    setState({
      status: 'available',
      availableVersion: info.version,
      message: `发现新版本 ${info.version}。`
    })
  })

  autoUpdater.on('update-not-available', () => {
    updateInfo = null
    setState({
      status: 'not-available',
      availableVersion: '',
      message: '当前已是最新版本。',
      percent: 0
    })
  })

  autoUpdater.on('download-progress', (progress) => {
    setState({
      status: 'downloading',
      percent: Math.max(0, Math.min(100, progress.percent || 0)),
      transferred: progress.transferred || 0,
      total: progress.total || 0,
      message: `正在下载 ${Math.round(progress.percent || 0)}%`
    })
  })

  autoUpdater.on('update-downloaded', (info) => {
    updateInfo = info
    setState({
      status: 'downloaded',
      availableVersion: info.version,
      percent: 100,
      message: '更新已下载完成，可以重启安装。'
    })
  })

  autoUpdater.on('error', (error) => {
    setState({
      status: 'error',
      message: error?.message || '检查更新失败，请稍后重试。'
    })
  })
}

function getState() {
  return state
}

function setChannel(channel) {
  if (!['stable', 'beta'].includes(channel)) {
    throw new Error('未知的更新通道。')
  }

  writeChannel(channel)
  applyChannel(channel)
  updateInfo = null
  return setState({
    status: 'idle',
    channel,
    availableVersion: '',
    percent: 0,
    transferred: 0,
    total: 0,
    message: channel === 'beta' ? '已切换到测试版通道。' : '已切换到正式版通道。'
  })
}

async function checkForUpdates() {
  if (!app.isPackaged) {
    return setState({
      status: 'development',
      message: '开发环境不检查更新，请在安装版中使用此功能。'
    })
  }

  await autoUpdater.checkForUpdates()
  return state
}

async function downloadUpdate() {
  if (state.status !== 'available' || !updateInfo) {
    throw new Error('当前没有可下载的更新。')
  }

  setState({ status: 'downloading', percent: 0, message: '正在准备下载…' })
  await autoUpdater.downloadUpdate()
  return state
}

function installUpdate() {
  if (state.status !== 'downloaded') {
    throw new Error('更新尚未下载完成。')
  }

  setImmediate(() => autoUpdater.quitAndInstall(false, true))
  return true
}

module.exports = {
  checkForUpdates,
  downloadUpdate,
  getState,
  initialize,
  installUpdate,
  setChannel
}
