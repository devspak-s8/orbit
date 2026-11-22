// ORBIT desktop — Electron main process. A native shell for any
// self-hosted ORBIT instance: point it at your server URL, get an app
// window with tray, native menus, and reconnect-on-launch.
//
// Design notes: settings are plain JSON in userData (no dependency), the
// connect screen is a local HTML file, and the server is validated from
// the main process (healthz) before the window ever loads it.
const { app, BrowserWindow, Tray, Menu, dialog, ipcMain, nativeImage, shell } = require('electron')
const path = require('node:path')
const fs = require('node:fs')

const APP_NAME = 'ORBIT'
const settingsPath = () => path.join(app.getPath('userData'), 'settings.json')

function loadSettings() {
  try {
    return JSON.parse(fs.readFileSync(settingsPath(), 'utf8'))
  } catch {
    return {}
  }
}

function saveSettings(settings) {
  fs.mkdirSync(path.dirname(settingsPath()), { recursive: true })
  fs.writeFileSync(settingsPath(), JSON.stringify(settings, null, 2))
}

let mainWindow = null
let tray = null
let connectReplyHandler = null

function createWindow() {
  const icon = nativeImage.createFromPath(path.join(__dirname, 'assets', 'icon.png'))
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 960,
    minHeight: 600,
    backgroundColor: '#0a0a0a',
    icon,
    title: APP_NAME,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  mainWindow.setMenuBarVisibility(true)

  const { serverUrl } = loadSettings()
  if (serverUrl) {
    mainWindow.loadURL(serverUrl).catch(() => showConnect())
  } else {
    showConnect()
  }

  // ORBIT is a self-hosted app: external links open in the system browser,
  // not a new Electron window.
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  // If the server is gone (dev stack stopped), fall back to the connect
  // screen instead of a dead error page.
  mainWindow.webContents.on('did-fail-load', (_event, code, _desc, url) => {
    if (url.startsWith('file://') || code === -3) return // aborted, not an outage
    showConnect()
  })

  mainWindow.on('close', (event) => {
    // Close hides to tray; Quit from the tray (or Cmd/Ctrl+Q) really quits.
    if (!app.isQuitting) {
      event.preventDefault()
      mainWindow.hide()
    }
  })
}

function showConnect() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.loadFile(path.join(__dirname, 'connect.html'))
  }
}

function createTray() {
  const icon = nativeImage.createFromPath(path.join(__dirname, 'assets', 'icon.png'))
  tray = new Tray(icon)
  tray.setToolTip(APP_NAME)
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: 'Open ORBIT', click: () => (mainWindow ? (mainWindow.show(), mainWindow.focus()) : createWindow()) },
      { label: 'Change server…', click: () => changeServer() },
      { type: 'separator' },
      {
        label: 'Quit',
        click: () => {
          app.isQuitting = true
          app.quit()
        },
      },
    ]),
  )
  tray.on('click', () => (mainWindow ? (mainWindow.isVisible() ? mainWindow.focus() : mainWindow.show()) : createWindow()))
}

function changeServer() {
  saveSettings({})
  showConnect()
  if (mainWindow) {
    mainWindow.show()
    mainWindow.focus()
  }
}

function applyMenu() {
  const template = [
    {
      label: 'File',
      submenu: [
        { label: 'Change server…', click: () => changeServer() },
        { type: 'separator' },
        { label: 'Quit', accelerator: 'CmdOrCtrl+Q', click: () => { app.isQuitting = true; app.quit() } },
      ],
    },
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'forceReload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'resetZoom' },
        { role: 'zoomIn' },
        { role: 'zoomOut' },
        { role: 'togglefullscreen' },
      ],
    },
    {
      label: 'Help',
      submenu: [
        {
          label: 'About ORBIT',
          click: () =>
            dialog.showMessageBox({
              type: 'info',
              title: 'About ORBIT',
              message: `ORBIT ${app.getVersion()}`,
              detail: 'The open-source, self-hosted alternative to the subscription stack.\nhttps://github.com/devspak-s8/orbit',
            }),
        },
      ],
    },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

// The connect screen asks the main process to validate and remember a
// server URL. Validation happens here (not in the renderer) so the
// window never loads an unverified address.
ipcMain.handle('connect', async (_event, rawUrl) => {
  let url = String(rawUrl || '').trim().replace(/\/+$/, '')
  if (!/^https?:\/\/.+/.test(url)) {
    return { ok: false, error: 'The URL must start with http:// or https://' }
  }
  try {
    const res = await fetch(url + '/healthz', { signal: AbortSignal.timeout(5000) })
    if (!res.ok) {
      return { ok: false, error: `${url} answered ${res.status} on /healthz — is this an ORBIT server?` }
    }
    const body = await res.json().catch(() => null)
    if (!body || body.status !== 'ok') {
      return { ok: false, error: `${url} is reachable but does not look like an ORBIT server` }
    }
  } catch (err) {
    return { ok: false, error: `Could not reach ${url}/healthz — ${err.name === 'TimeoutError' ? 'timed out' : 'connection failed'}` }
  }
  saveSettings({ serverUrl: url })
  if (mainWindow && !mainWindow.isDestroyed()) {
    await mainWindow.loadURL(url)
  }
  return { ok: true }
})

ipcMain.handle('getServer', () => loadSettings().serverUrl ?? '')

const singleInstance = app.requestSingleInstanceLock()
if (!singleInstance) {
  app.quit()
} else {
  app.on('second-instance', () => {
    if (mainWindow) {
      mainWindow.show()
      mainWindow.focus()
    }
  })

  app.whenReady().then(() => {
    applyMenu()
    createWindow()
    createTray()

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) createWindow()
      else if (mainWindow) mainWindow.show()
    })
  })

  app.on('window-all-closed', () => {
    // Tray keeps the app alive on all platforms.
  })
}
