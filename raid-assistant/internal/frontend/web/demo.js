// Preview-only mock bridge. The Windows executable does not embed this script.
// No network access, credential storage, download or ESXi change occurs here.
(() => {
  window.RAID_DEMO = true;
  const catalog = /*__DEMO_CATALOG__*/ [];
  const jobs = new Map(); let sequence = 0; let current = null; let imported = null; let snapshot = null;
  let monitoring = {running:false,sequence:0,alertSequence:0}; let history = [];
  function remember(type,severity,summary){ history.unshift({at:new Date().toISOString(),host:current?.displayHost || 'Демо',type,severity,summary}); }
  const start = data => { const id = `demo-${++sequence}`; jobs.set(id, { state:'done', data }); return Promise.resolve(id); };
  window.goPoll = async id => { const result = jobs.get(id); if (!result) throw new Error('Демо-задача не найдена'); jobs.delete(id); return result; };
  window.goCertStart = async address => start({address, subject:'CN=demo', issuer:'Demo', sha256:'A1'.repeat(32), notAfter:'2030-01-01T00:00:00Z', trusted:true, validTime:true});
  window.goScanStart = async request => {
    // Only the example address is retained, never credentials.
    current = {
      address:request.address, displayHost:request.address.split(':')[0], name:'esxi-lab-01', version:'8.0.3', build:'24022510', fullName:'VMware ESXi 8.0 Update 3', vendor:'Broadcom lab', model:'Демонстрационный сервер',
      controllers:[{device:'vmhba0', model:'MegaRAID 9560-8i', driver:'lsi_mr3', status:'online', pci:'0000:18:00.0', vendor:'1000', deviceId:'10e2', subVendor:'1000', subDevice:'4010', catalogModel:'MegaRAID 9560-8i', match:'catalog'}, {device:'vmhba1', model:'SATA AHCI Controller', driver:'vmw_ahci', status:'online', pci:'0000:00:17.0', vendor:'8086', deviceId:'a282', subVendor:'1028', subDevice:'07a1', match:'unknown'}],
      datastores:[{name:'datastore1', type:'VMFS', accessible:true, free:420000000000,extents:['naa.600e00040010f26027ad7d3730ecb7ad'],vms:['DEMO-VM']}],
      vibInventoryKnown:true,vibs:[{name:'esx-base',version:'8.0.3-demo'},{name:'tools-light',version:'demo'},{name:'lsi-mr3',version:'demo'}],
      checks:[
        {label:'Версия ESXi',value:'8.0.3 · build 24022510',status:'ok',detail:'Сценарий установки подготовлен для ESXi 8.x.'},
        {label:'Каталог контроллеров',value:'Есть точное совпадение PCI ID',status:'ok',detail:'Не подтверждает firmware, подпись или состояние RAID.'},
        {label:'Secure Boot и подпись',value:'Не проверено',status:'unknown',detail:'Проверьте на хосте перед установкой.'},
        {label:'Firmware и здоровье RAID',value:'Не проверено',status:'unknown',detail:'Проверьте в утилите контроллера.'},
        {label:'Bootbank и место под VIB',value:'Не проверено',status:'unknown',detail:'Место в datastore не означает место в bootbank.'},
        {label:'Режим обслуживания',value:'Не включён',status:'info',detail:'Подготовьте окно обслуживания.'},
        {label:'SSH на хосте',value:'Остановлен',status:'info',detail:'Приложение не включает SSH.'},
        {label:'Порт 35557 с этого ПК',value:'TCP недоступен',status:'unknown',detail:'Причина может быть в службе, firewall или маршрутизации.'},
        {label:'Установленный LWA',value:'Не удалось проверить',status:'unknown',detail:'Проверьте вручную список VIB.'}
      ], warnings:[], candidate:{model:'MegaRAID 9560-8i', version:'008.018.012.000 (MR 7.38)', productUrl:'https://www.broadcom.com/products/storage/raid-controllers/megaraid-9560-8i', readmeUrl:'https://docs.broadcom.com/docs/1232745888', downloadUrl:'https://docs.broadcom.com/docs/1232745889'}
    }; imported = null; snapshot=null; remember('ESXi API','info','Вымышленные данные демонстрации'); return start(current);
  };
  const fakeBundle = () => ({archiveName:'008.018.012.000_MR_7.38_LSA_ESXi.zip (демо)', archivePath:'C:\\Users\\Example\\Downloads', sha256:'b11f8b5cf11d918c652dfc3b6a6caa231df63ddf2aab17ceaf074cf9d72679a5', hashStatus:'pinned', vibName:'BCM_bootbank_lwmd_008.018.012.000-01.vib', vibPath:'C:\\Users\\Example\\Downloads\\BCM_bootbank_lwmd_008.018.012.000-01.vib', vibSha256:'c52600fc5722eefb0ae1e66fe80f9a80deca0c714426475617af82788e564c67', notice:'Демонстрация: файл на самом деле не скачан. В приложении SHA-256 сверяется с ранее зафиксированной копией.'});
  window.goDownloadStart = async accepted => { if (!accepted) throw new Error('Сначала примите условия Broadcom'); imported = fakeBundle(); return start(imported); };
  window.goImportStart = async () => { imported = fakeBundle(); return start(imported); };
  window.goReport = async opts => {
    if (!current) throw new Error('Сначала проверьте ESXi');
    const steps = [
      {title:'Проверьте исходные данные',body:`Демонстрационный хост ${current.displayHost}, ESXi 8.0.3, контроллер MegaRAID 9560-8i. Firmware, Secure Boot и здоровье RAID не проверены.`,commands:['vmware -vl','esxcli storage core adapter list','esxcli hardware pci list']},
      {title:'Подготовьте сервер и резервные копии',body:'Проверьте резервные копии, состояние RAID и совместимость OEM. Запланируйте окно обслуживания.'},
      {title:'Сверьте пакет и условия Broadcom',body:'Каталог Broadcom указывает LSA для ESXi. Сверьте README, firmware и подпись. LWM не заменяет LSA.'}
    ];
    if (!imported) steps.push({title:'Получите и проверьте VIB',body:'Скачайте архив из официального источника и выберите его в приложении. Это демонстрация.'});
    else if (!opts.datastore) steps.push({title:'Выберите доступный datastore',body:'Выберите хранилище для формирования команд.'});
    else {
      const path=`'/vmfs/volumes/${opts.datastore}/BCM_bootbank_lwmd_008.018.012.000-01.vib'`;
      steps.push({title:'Проверьте файл и выполните dry-run',body:'Сверьте SHA-256 и источник. При ошибках подписи или совместимости остановитесь.',commands:[`sha256sum ${path}`,`esxcli software vib install -v ${path} --dry-run`]});
      steps.push({title:'Установите вручную в согласованное окно',body:'Только после проверки и успешного dry-run. Перезагрузка выполняется вручную.',commands:[`esxcli software vib install -v ${path}`,'esxcli daemon info get -s lwmd']});
      steps.push({title:'Откройте веб-интерфейс',body:`После проверки службы и firewall откройте https://${current.displayHost}:35557. Не публикуйте порт в Интернет.`});
    }
    return {title:'Инструкция по LSA / LWA для ESXi',host:current.displayHost,steps,markdown:'# ДЕМО: инструкция для ESXi\n\n'+steps.map((s,i)=>`## ${i+1}. ${s.title}\n\n${s.body}\n\n${(s.commands||[]).join('\n')}`).join('\n\n')};
  };
  window.goSave = async () => { throw new Error('В демо-режиме файл не сохраняется'); };
  window.goOpenFolder = async () => { throw new Error('В демо-режиме папка не создаётся'); };
  window.goOpenLink = async () => { throw new Error('В демо-режиме внешние ссылки отключены'); };
  window.goClipboard = async text => navigator.clipboard.writeText(text);

  window.goInvalidate = async()=>{current=null;imported=null;snapshot=null;monitoring={running:false,sequence:0,alertSequence:0};};
  window.goProfiles=async()=>{try{return JSON.parse(localStorage.getItem('raid-demo-hosts')||'[]');}catch(_){return []}};
  window.goProfileSave=async p=>{const list=await window.goProfiles();const x={name:p.name||p.address,address:p.address,user:p.user};const i=list.findIndex(v=>v.address===p.address);if(i<0)list.push(x);else list[i]=x;localStorage.setItem('raid-demo-hosts',JSON.stringify(list));return list;};
  window.goProfileDelete=async address=>{const list=(await window.goProfiles()).filter(p=>p.address!==address);localStorage.setItem('raid-demo-hosts',JSON.stringify(list));return list;};
  window.goComponents=async()=>catalog.map(x=>{const v=(current?.vibs||[]).filter(v=>(x.vibNames||[]).some(n=>n.toLowerCase()===v.name.toLowerCase()));return {...x,presence:!x.vibNames?.length?'not-vib':!current?'unknown':v.length?'found':'not-seen',found:v};});
  window.goDiagnostics=async()=>{if(!current)throw new Error('Сначала проверьте ESXi');return [
    {title:'1. Установлен ли LWA?',fact:'ДЕМО: состояние VIB не подтверждено.',next:'Сверьте README и совместимость.',commands:['esxcli software vib list | grep -i lwmd'],status:'unknown'},
    {title:'2. Состояние lwmd',fact:'ДЕМО: служба не найдена.',next:'Проверьте службу.',commands:['esxcli daemon info get -s lwmd'],status:'unknown'},
    {title:'3. Правило firewall',fact:'ДЕМО: нет данных.',next:'Ограничьте доступ доверенными IP.',status:'unknown'},
    {title:'4. TCP и маршрут с этого ПК',fact:'ДЕМО: проверка не выполнялась.',next:'Причина не установлена.',status:'unknown'},
    {title:'5. TLS и веб-ответ',fact:'ДЕМО: сертификат не получен.',next:'Не вводите пароль на неподтверждённой странице.',status:'unknown'}];};
  window.goPortStart=async()=>{remember('TCP/TLS 35557','info','ДЕМО: подключения нет');return start({tcp:false,tls:false,observation:'ДЕМО'});};
  function fakeRaid(source){return {source,format:'ДЕМОНСТРАЦИЯ',collectedAt:new Date().toISOString(),severity:'warning',warnings:['Вымышленные данные: проверка реального сервера не выполнялась.'],controllers:[{id:'0',model:'Demo MegaRAID',state:'Optimal',severity:'ok'}],volumes:[{controller:'0',id:'0/0',level:'RAID1',size:'1 TB',state:'Dgrd',naa:'naa.600e00040010f26027ad7d3730ecb7ad',severity:'warning'}],disks:[{controller:'0',id:'252:0',group:'0',state:'Onln',severity:'ok'},{controller:'0',id:'252:1',group:'0',state:'Failed',severity:'critical'}]};}
  window.goRaidImportStart=async()=>{snapshot=fakeRaid('Импорт');remember('RAID импорт','warning','ДЕМО-отчёт');return start(snapshot)};
  window.goSSHKeyStart=async()=>{if(!current)throw new Error('Сначала проверьте ESXi');return start({algorithm:'ssh-ed25519',fingerprint:'SHA256:DEMO_ONLY_NOT_A_REAL_HOST_KEY'});};
  window.goSSHReadStart=async req=>{if(!req.consent||req.fingerprint!=='SHA256:DEMO_ONLY_NOT_A_REAL_HOST_KEY')throw new Error('Сначала проверьте SSH-ключ');snapshot=fakeRaid('SSH');remember('RAID SSH','warning','ДЕМО SSH');return start(snapshot);};
  window.goCorrelate=async confirmed=>{if(!current||!snapshot)throw new Error('Сначала проверьте хост и отчёт');if(snapshot.source==='Импорт'&&!confirmed)throw new Error('Подтвердите принадлежность отчёта хосту');return {note:'ДЕМО: только совпадение NAA. Связь ВМ является инвентарной.',links:[{controller:'0',volume:'0/0',naa:'naa.600e00040010f26027ad7d3730ecb7ad',datastore:'datastore1',vms:['DEMO-VM']}]};};
  window.goMonitorStart=async opts=>{if(!current)throw new Error('Сначала проверьте ESXi');if(opts.interval<60||opts.interval>3600)throw new Error('Интервал 60–3600 секунд');monitoring={running:true,mode:opts.mode,interval:opts.interval,sequence:1,alertSequence:1,lastAt:new Date().toISOString(),severity:'warning',message:'ДЕМО: предупреждение, реальный мониторинг не запущен',snapshot:opts.mode==='ssh'?fakeRaid('SSH'):null};remember('Мониторинг','warning','Демонстрационное предупреждение');return monitoring;};
  window.goMonitor=async()=>monitoring;
  window.goMonitorStop=async()=>{monitoring={...monitoring,running:false,snapshot:null,sequence:0,alertSequence:0};return monitoring;};
  window.goHistory=async()=>history;
  window.goHistoryClear=async()=>{history=[];};
  window.goUpdateStart=async()=>start({current:'0.2.0',latest:'0.2.0',available:false,note:'ДЕМО: GitHub не запрашивался.'});
  window.goScreenshotLoad=async()=>null;
  window.goScreenshotImport=async()=>{throw new Error('В демо-режиме файл не импортируется');};
  window.goScreenshotClear=async()=>{};
  window.goShareTelegram=async()=>{throw new Error('В демо-режиме внешние ссылки отключены');};

})();
