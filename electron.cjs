const { app, BrowserWindow, shell } = require('electron');
const path = require('path');
app.whenReady().then(() => {
  const window = new BrowserWindow({ width: 1200, height: 850, webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true } });
  window.loadFile(path.join(__dirname, 'src/index.html'));
  window.webContents.setWindowOpenHandler(({url}) => { if (url.startsWith('https://help.mikrotik.com/')) shell.openExternal(url); return {action:'deny'}; });
});
app.on('window-all-closed', () => app.quit());
