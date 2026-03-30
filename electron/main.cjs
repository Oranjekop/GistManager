const { app, BrowserWindow, clipboard, ipcMain, Menu, shell } = require('electron')
const path = require('node:path')
const gistService = require('./gist-service.cjs')

const isDev = Boolean(process.env.ELECTRON_RENDERER_URL)

async function createWindow() {
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

  ipcMain.handle('gist-api', async (_, request = {}) => {
    const { action, token, gistId, payload, params } = request

    try {
      switch (action) {
        case 'session':
          return await gistService.getSession(token)
        case 'list-gists':
          return await gistService.listGists(token, params)
        case 'get-gist':
          return await gistService.getGist(token, gistId)
        case 'create-gist':
          return await gistService.createGist(token, payload)
        case 'update-gist':
          return await gistService.updateGist(token, gistId, payload)
        case 'delete-gist':
          return await gistService.deleteGist(token, gistId)
        default:
          throw new Error(`未知客户端操作：${action || 'empty'}`)
      }
    } catch (error) {
      throw new Error(error.message || '客户端请求失败。')
    }
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
