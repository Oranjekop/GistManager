const { app, BrowserWindow, clipboard, ipcMain, Menu, shell } = require('electron')
const { spawn } = require('node:child_process')
const net = require('node:net')
const path = require('node:path')

const isDev = Boolean(process.env.ELECTRON_RENDERER_URL)
const backendPort = 3000
let backendProcess = null

function waitForPort(port, timeoutMs = 15000) {
  const start = Date.now()

  return new Promise((resolve, reject) => {
    const tryConnect = () => {
      const socket = net.connect(port, '127.0.0.1')

      socket.once('connect', () => {
        socket.end()
        resolve()
      })

      socket.once('error', () => {
        socket.destroy()

        if (Date.now() - start >= timeoutMs) {
          reject(new Error(`Timed out waiting for port ${port}.`))
          return
        }

        setTimeout(tryConnect, 250)
      })
    }

    tryConnect()
  })
}

function startBackend() {
  const serverEntry = path.join(__dirname, '..', 'server', 'index.js')
  const args = isDev ? ['--watch', serverEntry] : [serverEntry]

  backendProcess = spawn(process.execPath, args, {
    cwd: path.join(__dirname, '..'),
    env: {
      ...process.env,
      PORT: String(backendPort)
    },
    stdio: 'inherit'
  })

  backendProcess.on('exit', () => {
    backendProcess = null
  })

  return waitForPort(backendPort)
}

async function createWindow() {
  await startBackend()

  const window = new BrowserWindow({
    width: 1600,
    height: 980,
    minWidth: 1200,
    minHeight: 760,
    backgroundColor: '#0b1220',
    title: 'Gist管理器',
    icon: path.join(__dirname, '..', 'build', 'icon.png'),
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false
    }
  })

  window.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  if (isDev) {
    await window.loadURL(process.env.ELECTRON_RENDERER_URL)
    window.webContents.openDevTools({ mode: 'detach' })
  } else {
    await window.loadFile(path.join(__dirname, '..', 'client', 'dist', 'index.html'))
  }
}

app.whenReady().then(() => {
  Menu.setApplicationMenu(null)

  ipcMain.handle('open-external', async (_, url) => {
    if (!url) {
      return false
    }

    await shell.openExternal(url)
    return true
  })

  ipcMain.handle('copy-text', async (_, text) => {
    if (!text) {
      return false
    }

    clipboard.writeText(text)
    return true
  })

  createWindow().catch((error) => {
    console.error(error)
    app.quit()
  })

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow().catch((error) => {
        console.error(error)
      })
    }
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})

app.on('before-quit', () => {
  if (backendProcess) {
    backendProcess.kill()
  }
})
