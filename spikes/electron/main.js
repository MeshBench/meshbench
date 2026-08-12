// Electron shell. One BrowserWindow, node integration on so the renderer can
// read the fixture from disk directly, which is what a Go backend would
// otherwise be doing over a socket.
const { app, BrowserWindow } = require('electron')
const path = require('path')

function create() {
  const win = new BrowserWindow({
    width: 1500, height: 900,
    backgroundColor: '#0f1215',
    title: 'MeshBench - Plan (Electron spike)',
    webPreferences: { nodeIntegration: true, contextIsolation: false },
  })
  win.setMenuBarVisibility(false)
  win.loadFile(path.join(__dirname, 'index.html'))
}

app.whenReady().then(create)
app.on('window-all-closed', () => app.quit())
