const { contextBridge, ipcRenderer } = require('electron');
ipcRenderer.on('atlas:update-status',(_event,data)=>window.dispatchEvent(new CustomEvent('atlas-update',{detail:data})));
const bounded = (value, max) => typeof value === 'string' && value.length <= max;
contextBridge.exposeInMainWorld('atlasDesktop', {
  copyText: text => { if (!bounded(text,200000)) throw new Error('Текст слишком большой'); return ipcRenderer.invoke('atlas:copy',text); },
  openExternal: url => { if (!bounded(url,8192)) throw new Error('Некорректная ссылка'); return ipcRenderer.invoke('atlas:external',url); },
  checkUpdates: () => ipcRenderer.invoke('atlas:update-check'),
  installUpdate: () => ipcRenderer.invoke('atlas:update-install'),
  setAutoUpdate: enabled => ipcRenderer.invoke('atlas:auto-update',enabled === true),
  onUpdate: callback => { const listener = (_event,data) => callback(data); ipcRenderer.on('atlas:update-status',listener); return () => ipcRenderer.removeListener('atlas:update-status',listener); }
});
