const { app, BrowserWindow, ipcMain, clipboard, shell, session, dialog } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
const { pathToFileURL } = require('node:url');
const { autoUpdater } = require('electron-updater');
let win = null, downloaded = false, checking = false, updateInfo = null;
const allowedHosts = new Set(['github.com','api.github.com','techdocs.broadcom.com','knowledge.broadcom.com','claudiokuenzler.com','vcloud-lab.com','www.starwindsoftware.com','t.me','wa.me','www.nakivo.com']);
const singleInstance = app.requestSingleInstanceLock();
if (!singleInstance) app.quit();
app.setAppUserModelId('ai.arena.esxiatlas');
app.disableHardwareAcceleration();
function send(status){if(win&&!win.isDestroyed())win.webContents.send('atlas:update-status',status);}
function isTrustedSender(event){return win && event.sender === win.webContents && event.senderFrame === win.webContents.mainFrame && event.sender.getURL().startsWith(pathToFileURL(path.join(__dirname,'../app/index.html')).href);}
function trusted(handler){return (event,...args)=>{if(!isTrustedSender(event))throw new Error('Untrusted IPC sender');return handler(...args);};}
function safeExternal(value){let u;try{u=new URL(value);}catch{throw new Error('Некорректная ссылка');}if(u.protocol!=='https:'||!allowedHosts.has(u.hostname)||u.username||u.password||u.port)throw new Error('Домен не входит в список приложения');return u.href;}
function preferenceFile(){return path.join(app.getPath('userData'),'preferences.json');}
function autoCheck(){try{return JSON.parse(fs.readFileSync(preferenceFile(),'utf8')).autoUpdate!==false;}catch{return true;}}
async function checkUpdates(){
 if(!app.isPackaged)return {available:false,message:'Режим разработки: обновления проверяются только в установленном приложении.'};
 if(process.platform!=='win32')return {available:false,message:'Автообновление этого пакета предназначено для Windows.'};
 if(checking)return {available:false,message:'Проверка уже выполняется.'};
 if(downloaded)return {available:true,downloaded:true,version:updateInfo?.version,message:'Обновление загружено и проверено. Подтвердите перезапуск.'};
 checking=true;send({available:false,message:'Проверяем GitHub Releases…'});
 try{const r=await autoUpdater.checkForUpdates();const available=!!r?.isUpdateAvailable;return {available,downloaded:false,version:r?.updateInfo?.version,message:available?'Новая версия найдена. Нажмите «Скачать и установить».':'У вас актуальная опубликованная версия.'};}
 catch(error){send({available:false,message:'Проверка не удалась: '+error.message});throw error;}finally{checking=false;}
}
function createWindow(){
 win=new BrowserWindow({width:1440,height:960,minWidth:390,minHeight:620,backgroundColor:'#f5f7fa',title:'ESXi Atlas',icon:path.join(__dirname,'resources/icon.png'),autoHideMenuBar:true,show:false,webPreferences:{preload:path.join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true,webSecurity:true,allowRunningInsecureContent:false,spellcheck:false}});
 win.webContents.setWindowOpenHandler(({url})=>{try{shell.openExternal(safeExternal(url));}catch{}return {action:'deny'};});
 win.webContents.on('will-navigate',(event,url)=>{if(url!==pathToFileURL(path.join(__dirname,'../app/index.html')).href)event.preventDefault();});
 win.webContents.on('will-attach-webview',event=>event.preventDefault());
 win.webContents.session.setPermissionRequestHandler((_wc,_permission,callback)=>callback(false));
 win.loadFile(path.join(__dirname,'../app/index.html'));
 win.once('ready-to-show',()=>{win.show();if(autoCheck())setTimeout(()=>checkUpdates().catch(()=>{}),2500);});
 win.on('closed',()=>{win=null;});
}
if(singleInstance){
 app.whenReady().then(()=>{
  session.defaultSession.setPermissionRequestHandler((_wc,_permission,callback)=>callback(false));
  ipcMain.handle('atlas:copy',trusted(text=>{if(typeof text!=='string'||text.length>200000)throw new Error('Invalid clipboard text');clipboard.writeText(text);return true;}));
  ipcMain.handle('atlas:external',trusted(async url=>{await shell.openExternal(safeExternal(url));return true;}));
  ipcMain.handle('atlas:update-check',trusted(checkUpdates));
  ipcMain.handle('atlas:auto-update',trusted(enabled=>{fs.writeFileSync(preferenceFile(),JSON.stringify({autoUpdate:enabled}),{mode:0o600});return true;}));
  ipcMain.handle('atlas:update-install',trusted(async()=>{
   if(!downloaded){
    if(!updateInfo)return {message:'Сначала нажмите «Проверить». Установка без проверки не выполняется.'};
    const approval=await dialog.showMessageBox(win,{type:'question',buttons:['Отмена','Скачать обновление'],defaultId:0,cancelId:0,title:'Загрузка обновления',message:'Скачать новую версию ESXi Atlas?',detail:'Перед установкой проверяются SHA-512, Authenticode и публичный сертификат. Перезапуск потребуется подтвердить отдельно.'});
    if(approval.response!==1)return {message:'Загрузка отменена.'};
    autoUpdater.downloadUpdate().catch(error=>send({available:false,message:'Ошибка загрузки: '+error.message}));return {message:'Загрузка началась. Дождитесь проверки пакета.'};
   }
   const choice=await dialog.showMessageBox(win,{type:'question',buttons:['Отмена','Перезапустить и установить'],defaultId:0,cancelId:0,title:'Обновление ESXi Atlas',message:'Установить проверенное обновление?',detail:'Приложение закроется. Проект и прогресс сохраняются в данных пользователя.'});
   if(choice.response===1)setImmediate(()=>autoUpdater.quitAndInstall(false,true));return {message:choice.response===1?'Перезапуск…':'Установка отменена.'};
  }));
  autoUpdater.autoDownload=false;autoUpdater.autoInstallOnAppQuit=false;autoUpdater.allowPrerelease=false;autoUpdater.disableDifferentialDownload=true;
  // Override legacy verifier: fail closed on PowerShell errors and pin the exact certificate, not only its CN.
  autoUpdater.verifyUpdateCodeSignature=async(publisherNames,file)=>{
   const config=await autoUpdater.configOnDisk.value;
   const names=Array.isArray(config?.publisherName)?config.publisherName:[config?.publisherName];
   if(!names.includes('ESXi Atlas Code Signing'))return 'Updater publisher identity is missing or unexpected';
   const pin=process.env.ATLAS_WINDOWS_CERT_SHA256||(()=>{try{return fs.readFileSync(path.join(process.resourcesPath,'signing/windows-certificate.sha256'),'utf8').trim();}catch{return '';}})();
   if(!/^[a-fA-F0-9]{64}$/.test(pin))return 'Windows certificate pin is not configured';
   const {execFile}=require('node:child_process');
   const script='$ErrorActionPreference="Stop"; $s=Get-AuthenticodeSignature -LiteralPath $args[0]; $sha=[BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($s.SignerCertificate.RawData)).Replace("-",""); [pscustomobject]@{Status=[int]$s.Status;Thumbprint=$sha;Subject=$s.SignerCertificate.Subject;Path=$s.Path}|ConvertTo-Json -Compress';
   try{const output=await new Promise((resolve,reject)=>execFile('powershell.exe',['-NoProfile','-NonInteractive','-Command',script,file],{windowsHide:true,timeout:20000},(err,stdout,stderr)=>err||stderr?reject(err||new Error(stderr)):resolve(stdout)));
    const status=JSON.parse(output);if(status.Status!==0||status.Thumbprint?.toLowerCase()!==pin.toLowerCase()||path.normalize(status.Path)!==path.normalize(file))return 'Signature, certificate pin or path mismatch';
    return null;
   }catch(error){return 'Signature verification failed: '+error.message;}
  };
  autoUpdater.on('update-available',info=>{updateInfo=info;send({available:true,downloaded:false,version:info.version,message:'Доступна версия '+info.version+'. Требуется ваше подтверждение загрузки.'});});
  autoUpdater.on('update-not-available',()=>send({available:false,message:'Обновлений нет.'}));
  autoUpdater.on('download-progress',p=>send({available:true,downloaded:false,message:'Загрузка обновления: '+Math.round(p.percent)+'%'}));
  autoUpdater.on('update-downloaded',info=>{downloaded=true;updateInfo=info;send({available:true,downloaded:true,version:info.version,message:'Обновление проверено. Подтвердите перезапуск для установки.'});});
  autoUpdater.on('error',error=>{downloaded=false;send({available:false,message:'Обновление не установлено: '+error.message});});
  createWindow();
  app.on('activate',()=>{if(BrowserWindow.getAllWindows().length===0)createWindow();});
 });
 app.on('second-instance',()=>{if(win){if(win.isMinimized())win.restore();win.focus();}});
 app.on('window-all-closed',()=>{if(process.platform!=='darwin')app.quit();});
}
