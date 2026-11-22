// Preload: the only bridge between the connect screen and the main
// process. The web app itself gets nothing — it is just loaded from the
// server and needs no desktop APIs.
const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('orbitDesktop', {
  connect: (url) => ipcRenderer.invoke('connect', url),
  getServer: () => ipcRenderer.invoke('getServer'),
})
