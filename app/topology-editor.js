/* Local topology editor. Lines and animation are illustrative, not packet emulation. */
(() => {
'use strict';
const T=window.ESXiTopology, W=T.WORLD;
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const $=(s,root=document)=>root.querySelector(s), $$=(s,root=document)=>[...root.querySelectorAll(s)];
const iconPaths={
 host:'<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01M11 7.5h7M11 16.5h7"/>',
 vm:'<rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 21h8M12 18v3M7 9h4M7 12h8"/>',
 vrouter:'<rect x="3" y="6" width="18" height="12" rx="3"/><path d="M7 10h10M14 8l3 2-3 2M17 14H7M10 12l-3 2 3 2"/>',
 router:'<rect x="3" y="7" width="18" height="11" rx="3"/><path d="M7 13h10M14 10l3 3-3 3M8 4v3M16 4v3"/>',
 provider:'<path d="M7 18h11a4 4 0 0 0 .5-8A6.5 6.5 0 0 0 6 9.5 4.5 4.5 0 0 0 7 18z"/><path d="M9 14h6M12 11v6"/>',
 switch:'<rect x="3" y="6" width="18" height="12" rx="2"/><path d="M7 10h2M11 10h2M15 10h2M7 14h2M11 14h2M15 14h2"/>',
 link:'<path d="M10 13a4 4 0 0 0 6 .5l3-3a4 4 0 0 0-5.7-5.7l-1.7 1.7M14 11a4 4 0 0 0-6-.5l-3 3a4 4 0 0 0 5.7 5.7l1.7-1.7"/>',
 play:'<path d="m8 5 11 7-11 7z"/>',pause:'<path d="M9 5v14M15 5v14"/>',
 trash:'<path d="M4 6h16M9 6V3h6v3M6 6l1 14h10l1-14M10 10v6M14 10v6"/>',
 download:'<path d="M12 3v11m-4-4 4 4 4-4M4 17v4h16v-4"/>',
 plus:'<path d="M12 5v14M5 12h14"/>',settings:'<path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4"/><circle cx="12" cy="12" r="4"/>',
 expand:'<path d="M9 4H4v5M15 4h5v5M4 15v5h5M20 15v5h-5"/>',
 refresh:'<path d="M4 10a8 8 0 1 1 1 7M4 4v6h6"/>',close:'<path d="M6 6l12 12M18 6 6 18"/>'
};
const icon=name=>`<svg viewBox="0 0 24 24" aria-hidden="true" class="topo-icon">${iconPaths[name]||iconPaths.vm}</svg>`;
let active=null;
function mount(root,api){
  if(active)active.destroy();
  let graph=T.copy(api.getTopology()),selected=null,draft=null,dirty=false,connecting=false,source=null;
  let fontRatio=(Number(document.documentElement.dataset.fontScale)||100)/100;
  let zoom=Math.min(2,(window.innerWidth<700?.65:.85)*fontRatio),playing=false,animation=0,started=0,geometries=[],drag=null,busy=false,confirmCallback=null;
  let uidCounter=0;
  const uid=prefix=>prefix+'-'+Date.now().toString(36)+'-'+(++uidCounter).toString(36)+'-'+Math.random().toString(36).slice(2,7);
  root.innerHTML=`<section class="topology-app" aria-label="Интерактивная карта сети">
    <div class="topo-intro"><div><div class="topo-kicker"><span class="topo-live-dot"></span> ИНТЕРАКТИВНАЯ ЛАБОРАТОРИЯ</div><h2>Карта инфраструктуры</h2><p>Разместите устройства, задайте их интерфейсы и соедините линии. Нажмите на устройство или линию для настройки.</p></div><div class="topo-intro-actions"><button class="button" type="button" data-topo-action="from-project">${icon('refresh')} Из проекта стенда</button><button class="button" type="button" data-topo-action="example">${icon('refresh')} Пример</button><button class="button" type="button" data-topo-action="clear">Очистить</button></div></div>
    <div class="topo-disclaimer"><strong>Учебная визуализация.</strong> Движение точек по линиям не проверяет IP, маршрутизацию, VLAN, NAT или доступность; это не эмулятор PNETLab и не реальный трафик. Адреса примера условные. Схема не изменяет ваши серверы.</div>
    <div class="topo-editor-shell">
      <aside class="topo-palette" aria-label="Набор инструментов"><div class="topo-panel-caption">ИНСТРУМЕНТЫ <span>01</span></div><h3>Добавить на карту</h3><p>Нажмите или перетащите на поле.</p><div class="topo-tool-list">${Object.entries(T.TYPES).map(([type,t])=>`<button type="button" class="topo-tool" draggable="true" data-topo-add="${type}" title="Добавить: ${esc(t.label)}"><span class="topo-tool-icon" style="--device:${t.color}">${icon(type)}</span><span>${esc(t.label)}</span><span class="topo-tool-plus">+</span></button>`).join('')}</div><div class="topo-tool-divider"></div><button type="button" class="topo-connect-button" data-topo-action="connect">${icon('link')} <span>Соединить устройства</span></button><p class="topo-tip">Выберите два устройства, затем карты на обоих концах.</p><button type="button" class="topo-remove-button" data-topo-action="delete" disabled>${icon('trash')} Удалить элемент</button></aside>
      <div class="topo-stage"><div class="topo-stage-head"><div><div class="topo-panel-caption">СХЕМА <span>02</span></div><input class="topo-title" data-topo-title aria-label="Название схемы" maxlength="80" value="${esc(graph.title)}"><div class="topo-count" data-topo-count></div></div><div class="topo-stage-actions"><button class="topo-stage-btn primary" type="button" data-topo-action="play">${icon('play')} <span>Запустить движение</span></button><button class="topo-stage-btn" type="button" data-topo-action="gif">${icon('download')} GIF</button><button class="topo-stage-btn" type="button" data-topo-action="pdf">${icon('download')} PDF</button></div></div>
        <div class="topo-canvas-toolbar"><div class="topo-canvas-hint" data-topo-hint>Перемещайте устройства. Прокручивайте поле двумя пальцами или полосами прокрутки.</div><div class="topo-zoom"><button type="button" data-topo-action="zoom-out" aria-label="Уменьшить масштаб">−</button><span data-topo-zoom>85%</span><button type="button" data-topo-action="zoom-in" aria-label="Увеличить масштаб">+</button><button type="button" data-topo-action="fit" title="Показать всю схему" aria-label="Показать всю схему">${icon('expand')} <span>Вся схема</span></button></div></div>
        <div class="topo-viewport" tabindex="0" aria-label="Поле схемы: можно прокручивать и перетаскивать устройства"><div class="topo-world-holder"><div class="topo-world"><svg class="topo-links" width="${W.width}" height="${W.height}" viewBox="0 0 ${W.width} ${W.height}" aria-label="Линии связи"></svg><div class="topo-nodes"></div></div></div></div>
        <div class="topo-stage-foot"><div class="topo-legend"><span><i class="to"></i> туда</span><span><i class="back"></i> обратно</span><span><i class="off"></i> связь выключена</span></div><div data-topo-state aria-live="polite">Движение остановлено</div></div><div class="topo-export-status" data-topo-export-status role="status" aria-live="polite"></div>
      </div>
      <aside class="topo-inspector" aria-label="Настройки выбранного элемента"><div class="topo-panel-caption">НАСТРОЙКИ <span>03</span></div><div data-topo-inspector></div></aside>
    </div>
    <div class="topo-warnings" data-topo-warnings></div>
    <details class="topo-help"><summary>Как работать с картой и сохранять проект</summary><ol><li>Добавьте физический сервер и задайте процессоры, ядра, память и диски. Добавьте виртуальные серверы и роутеры на ESXi; в их настройках выберите физический сервер.</li><li>Добавьте провайдера, внешний роутер и при необходимости коммутатор. В настройках устройства добавьте сетевые карты и заполните IPv4/префикс, шлюз, MAC, VLAN и port group.</li><li>Выберите «Соединить устройства», нажмите два устройства и укажите по одному свободному интерфейсу на каждом конце. Для общего сегмента используйте коммутатор.</li><li>Нажмите «Запустить движение»: две точки показывают условное направление туда и обратно на включённых линиях. Это не проверка связности, маршрутов или настройки ESXi.</li><li>GIF — зацикленная анимация всей схемы, PDF — схема и список настроек. Эти файлы не редактируются. Для увеличения надписей на поле используйте масштаб «+». Для переноса и дальнейшего редактирования используйте кнопку «Сохранить проект» над вкладками: карта включена в JSON вместе с расчётным стендом.</li></ol><p>Импорт из проекта создаёт отдельный снимок серверов и ВМ без линий; дальнейшие изменения карты и расчётного стенда не синхронизируются. Один интерфейс может участвовать только в одной линии.</p></details>
    <dialog class="topo-dialog" data-topo-dialog><div data-topo-dialog-content></div></dialog>
  </section>`;
  const app=$('.topology-app',root),viewport=$('.topo-viewport',root),holder=$('.topo-world-holder',root),world=$('.topo-world',root),svg=$('.topo-links',root),nodes=$('.topo-nodes',root),inspector=$('[data-topo-inspector]',root),dialog=$('[data-topo-dialog]',root),dialogContent=$('[data-topo-dialog-content]',root);
  function notify(text){api.toast(text);}
  function setZoom(next){zoom=Math.max(.2,Math.min(2,Math.round(next*100)/100));holder.style.width=(W.width*zoom)+'px';holder.style.height=(W.height*zoom)+'px';world.style.transform=`scale(${zoom})`;$('[data-topo-zoom]',root).textContent=Math.round(zoom*100)+'%';}
  function fit(){const b=T.bounds(graph,45),z=Math.min(1.3,(viewport.clientWidth-24)/b.width,(viewport.clientHeight-24)/b.height);setZoom(z);viewport.scrollLeft=Math.max(0,b.x*zoom-12);viewport.scrollTop=Math.max(0,b.y*zoom-12);}
  function save(){api.saveTopology(T.copy(graph));}
  function setDirty(value){dirty=value;const el=$('[data-topo-dirty]',root);if(el)el.textContent=dirty?'Есть несохранённые изменения':'Настройки сохранены';const saveButton=$('[data-topo-save]',root);if(saveButton)saveButton.disabled=!dirty;}
  function refreshCounts(){
    $('[data-topo-count]',root).textContent=`${graph.devices.length} устройств · ${graph.links.length} связей · сохраняется локально`;
    const del=$('[data-topo-action="delete"]',root);del.disabled=!selected;
    const hint=$('[data-topo-hint]',root);
    hint.textContent=connecting?(source?`Выбрано: ${graph.devices.find(d=>d.id===source)?.name}. Теперь выберите второе устройство.`:'Выберите первое устройство на карте.'):'Перемещайте устройства. Прокручивайте поле двумя пальцами или полосами прокрутки.';
    const connect=$('[data-topo-action="connect"]',root);connect.classList.toggle('active',connecting);$('span',connect).textContent=connecting?'Отменить соединение':'Соединить устройства';
    const play=$('[data-topo-action="play"]',root);play.innerHTML=icon(playing?'pause':'play')+' <span>'+(playing?'Остановить движение':'Запустить движение')+'</span>';
    $('[data-topo-state]',root).textContent=playing?'Условное движение включено':'Движение остановлено';
  }
  function renderWarnings(){const list=T.warnings(graph),el=$('[data-topo-warnings]',root);el.innerHTML=list.length?`<strong>Проверьте ресурсы и размещение</strong><ul>${list.map(x=>`<li>${esc(x)}</li>`).join('')}</ul>`:'';el.hidden=!list.length;}
  function labelFor(end){const d=graph.devices.find(x=>x.id===end.device),n=d?.nics.find(x=>x.id===end.nic);return n?n.name+(n.address?' · '+n.address:''):'';}
  function renderLinks(){
    geometries=[];
    svg.innerHTML=graph.links.map(l=>{
      const a=graph.devices.find(d=>d.id===l.a.device),b=graph.devices.find(d=>d.id===l.b.device);if(!a||!b)return '';
      const p=T.geometry(a,b);geometries.push({...p,id:l.id,active:l.active});
      const mid=(t,axis)=>axis==='x'?p.x1+(p.x2-p.x1)*t:p.y1+(p.y2-p.y1)*t;
      return `<g class="topo-link ${l.active?'':'is-off'} ${selected?.kind==='link'&&selected.id===l.id?'is-selected':''}"><line class="topo-link-line" x1="${p.x1}" y1="${p.y1}" x2="${p.x2}" y2="${p.y2}"/><line class="topo-link-hit" data-link-id="${esc(l.id)}" role="button" tabindex="0" aria-label="Настроить линию ${esc(a.name)} — ${esc(b.name)}" x1="${p.x1}" y1="${p.y1}" x2="${p.x2}" y2="${p.y2}"/><text x="${mid(.15,'x')}" y="${mid(.15,'y')-8}" class="topo-link-label">${esc(labelFor(l.a))}</text><text x="${mid(.85,'x')}" y="${mid(.85,'y')+17}" text-anchor="end" class="topo-link-label">${esc(labelFor(l.b))}</text><circle class="topo-packet topo-forward" data-forward="${esc(l.id)}" r="5"/><circle class="topo-packet topo-return" data-return="${esc(l.id)}" r="5"/></g>`;
    }).join('');
    app.classList.toggle('is-playing',playing);
    if(playing)movePackets(performance.now());
  }
  function summary(d){const r=d.resources;if(d.type==='host')return `${r.cpu} CPU × ${r.cores} яд. · ${r.ram} GiB`;if(d.type==='vm'||d.type==='vrouter')return `${r.vcpu} vCPU · ${r.ram} GiB RAM`;return d.model||T.TYPES[d.type].short;}
  function renderNodes(){nodes.innerHTML=graph.devices.map(d=>{
    const type=T.TYPES[d.type],host=graph.devices.find(x=>x.id===d.hostId),addressed=d.nics.find(n=>n.address);
    return `<div class="topo-node ${selected?.kind==='device'&&selected.id===d.id?'is-selected':''} ${source===d.id?'is-source':''}" data-node-id="${esc(d.id)}" role="button" tabindex="0" aria-label="${esc(type.label+': '+d.name)}" style="left:${d.x}px;top:${d.y}px;--device:${type.color}"><div class="topo-node-main"><span class="topo-node-icon">${icon(d.type)}</span><span class="topo-node-heading"><small>${esc(type.short)}</small><strong title="${esc(d.name)}">${esc(d.name)}</strong></span><button type="button" class="topo-node-settings" data-node-settings="${esc(d.id)}" title="Настройки ${esc(d.name)}" aria-label="Настройки ${esc(d.name)}">${icon('settings')}</button></div><div class="topo-node-info">${esc(summary(d))}</div><div class="topo-node-host">${host?'На '+esc(host.name):esc(d.type==='vm'||d.type==='vrouter'?'Физический сервер не выбран':d.model)}</div><div class="topo-node-nics"><span></span>${addressed?esc(addressed.name+' · '+addressed.address):d.nics.length+' сетевых интерфейсов'}</div></div>`;
  }).join('');}
  function renderWorld(){renderNodes();renderLinks();refreshCounts();renderWarnings();}
  function field(label,key,value,options={}){
    const {type='text',min,max,step,maxlength=120,placeholder=''}=options;
    const val=typeof value==='number'&&!Number.isFinite(value)?'':value??'';
    return `<label class="topo-field"><span>${label}</span><input type="${type}" data-topo-field="${key}" value="${esc(val)}" ${min!==undefined?`min="${min}"`:''} ${max!==undefined?`max="${max}"`:''} ${step!==undefined?`step="${step}"`:''} maxlength="${maxlength}" placeholder="${esc(placeholder)}"></label>`;
  }
  function resourceField(label,key,value,min,max,step=1){const val=typeof value==='number'&&Number.isFinite(value)?value:'';return `<label class="topo-field"><span>${label}</span><input type="number" inputmode="decimal" data-topo-resource="${key}" value="${esc(val)}" min="${min}" max="${max}" step="${step}"></label>`;}
  function nicField(n,key,label,placeholder=''){return `<label class="topo-field"><span>${label}</span><input type="text" data-topo-nic="${esc(n.id)}" data-topo-nic-field="${key}" value="${esc(n[key])}" maxlength="${key==='portGroup'?63:key==='name'?48:40}" placeholder="${esc(placeholder)}" ${['address','gateway','vlan'].includes(key)?'inputmode="decimal"':''}></label>`;}
  function deviceForm(d){
    const type=T.TYPES[d.type],hosts=graph.devices.filter(x=>x.type==='host'),r=d.resources;
    let resources='';
    if(d.type==='host')resources=`<div class="topo-form-section"><h4>Ресурсы физического сервера</h4><div class="topo-field-grid">${resourceField('CPU · сокеты','cpu',r.cpu,1,8)}${resourceField('Ядер на CPU','cores',r.cores,1,256)}${resourceField('Частота, GHz','ghz',r.ghz,.5,8,.1)}${resourceField('RAM, GiB','ram',r.ram,1,65536)}${resourceField('SSD · количество','ssdCount',r.ssdCount,0,128)}${resourceField('SSD · каждый, GB','ssdSize',r.ssdSize,0,1000000)}${resourceField('HDD · количество','hddCount',r.hddCount,0,128)}${resourceField('HDD · каждый, GB','hddSize',r.hddSize,0,1000000)}</div></div>`;
    if(d.type==='vm'||d.type==='vrouter')resources=`<div class="topo-form-section"><h4>Ресурсы виртуальной машины</h4><div class="topo-field-grid">${resourceField('vCPU · всего','vcpu',r.vcpu,1,1024)}${resourceField('Ядер на v-сокет','coresPerSocket',r.coresPerSocket,1,256)}${resourceField('RAM, GiB','ram',r.ram,1,65536)}${resourceField('SSD, GB','ssd',r.ssd,0,1000000)}${resourceField('HDD, GB','hdd',r.hdd,0,1000000)}</div><p>vCPU должно делиться на число ядер на виртуальный сокет.</p></div>`;
    const placement=['vm','vrouter','switch'].includes(d.type)?`<label class="topo-field"><span>На каком физическом сервере</span><select data-topo-field="hostId"><option value="">${d.type==='switch'?'Внешний / не выбран':'Не выбран'}</option>${hosts.map(h=>`<option value="${esc(h.id)}" ${d.hostId===h.id?'selected':''}>${esc(h.name)}</option>`).join('')}</select></label>`:'';
    const nicCards=d.nics.map((n,i)=>{
      const attached=graph.links.filter(l=>(l.a.device===d.id&&l.a.nic===n.id)||(l.b.device===d.id&&l.b.nic===n.id)).length;
      return `<details class="topo-nic" ${i===0?'open':''}><summary><span>${esc(n.name)}</span><small>${attached?'Подключена':'Свободна'}</small></summary><div class="topo-nic-body">${nicField(n,'name','Название интерфейса','vNIC 1')}${nicField(n,'address','IPv4 / префикс','10.10.0.10/24')}${nicField(n,'gateway','Шлюз IPv4','10.10.0.1')}${nicField(n,'mac','MAC-адрес','00:11:22:33:44:55')}${nicField(n,'vlan','VLAN ID','0–4094')}${nicField(n,'portGroup','Port group','PG-LAN-10')}<button type="button" class="topo-inline-danger" data-topo-remove-nic="${esc(n.id)}">${icon('trash')} Удалить карту${attached?' и её связь':''}</button></div></details>`;
    }).join('');
    return `<div class="topo-inspector-head"><span class="topo-selected-icon" style="--device:${type.color}">${icon(d.type)}</span><div><h3>Устройство</h3><p>${esc(type.label)}</p></div></div><form data-topo-device-form novalidate><div class="topo-form-section">${field('Имя устройства','name',d.name,{maxlength:63})}${field('Модель / ОС / назначение','model',d.model,{maxlength:120})}${placement}<label class="topo-field"><span>Примечание</span><textarea data-topo-field="notes" maxlength="300" rows="2" placeholder="Назначение или важные детали">${esc(d.notes)}</textarea></label></div>${resources}<div class="topo-form-section"><div class="topo-section-row"><h4>Сетевые карты <span>${d.nics.length}</span></h4><button type="button" class="topo-add-nic" data-topo-action="add-nic">${icon('plus')} Добавить</button></div><p>IP указывайте вместе с префиксом. Шлюзы на нескольких NIC могут конфликтовать.${d.type==='host'?' Управляющий IP ESXi назначается VMkernel vmk, а не физическому uplink vmnic.':''}</p>${nicCards||'<p class="topo-empty-nic">Нет сетевых карт. Добавьте интерфейс.</p>'}</div><div class="topo-form-error" data-topo-form-error role="alert"></div><div class="topo-form-footer"><span data-topo-dirty>${dirty?'Есть несохранённые изменения':'Настройки сохранены'}</span><button type="submit" class="button primary" data-topo-save ${dirty?'':'disabled'}>Сохранить настройки</button><button type="button" class="button" data-topo-action="discard" ${dirty?'':'disabled'}>Отменить изменения</button><button type="button" class="topo-inline-danger" data-topo-action="delete">${icon('trash')} Удалить устройство</button></div></form>`;
  }
  function availableNICs(d,linkId){
    const busyNics=new Set();for(const l of graph.links)if(l.id!==linkId)for(const e of [l.a,l.b])if(e.device===d.id)busyNics.add(e.nic);
    return d.nics.map(n=>`<option value="${esc(n.id)}" ${busyNics.has(n.id)?'disabled':''}>${esc(n.name+(n.address?' · '+n.address:''))}${busyNics.has(n.id)?' — занята':''}</option>`).join('');
  }
  function linkForm(l){
    const a=graph.devices.find(d=>d.id===l.a.device),b=graph.devices.find(d=>d.id===l.b.device);
    const nicSelect=(device,end)=>`<label class="topo-field"><span>${esc(device.name)} · интерфейс</span><select data-topo-link-end="${end}">${availableNICs(device,l.id)}</select></label>`;
    return `<div class="topo-inspector-head"><span class="topo-selected-icon link-icon">${icon('link')}</span><div><h3>Линия связи</h3><p>${esc(a.name)} ↔ ${esc(b.name)}</p></div></div><form data-topo-link-edit novalidate><div class="topo-form-section">${field('Название линии','name',l.name,{maxlength:60,placeholder:'Например, WAN 1'})}${nicSelect(a,'a')}${nicSelect(b,'b')}<label class="topo-field"><span>Скорость, Gbit/s</span><input type="number" data-topo-link-speed min="0.01" max="400" step="0.01" value="${l.speed}"></label><label class="topo-checkbox"><input type="checkbox" data-topo-link-active ${l.active?'checked':''}> Линия включена</label><p>Точки показываются только на включённых линиях. Скорость — подпись, а не измеренная пропускная способность.</p></div><div class="topo-form-error" data-topo-form-error role="alert"></div><div class="topo-form-footer"><span data-topo-dirty>${dirty?'Есть несохранённые изменения':'Настройки сохранены'}</span><button type="submit" class="button primary" data-topo-save ${dirty?'':'disabled'}>Сохранить настройки</button><button type="button" class="button" data-topo-action="discard" ${dirty?'':'disabled'}>Отменить изменения</button><button type="button" class="topo-inline-danger" data-topo-action="delete">${icon('trash')} Удалить линию</button></div></form>`;
  }
  function renderInspector(){
    if(!selected){inspector.innerHTML=`<div class="topo-inspector-empty"><div class="topo-empty-orbit">${icon('settings')}</div><h3>Выберите элемент</h3><p>Нажмите на устройство или линию, чтобы открыть их параметры, сетевые карты и адреса.</p><div class="topo-inspector-hint"><strong>Начните со схемы</strong><span>Добавляйте устройства слева, а затем соединяйте их по интерфейсам.</span></div></div>`;return;}
    inspector.innerHTML=selected.kind==='device'?deviceForm(draft):linkForm(draft);
    if(selected.kind==='link')for(const end of ['a','b']){const el=$(`[data-topo-link-end="${end}"]`,inspector);if(el)el.value=draft[end].nic;}
  }
  function openDialog(html){dialogContent.innerHTML=html;if(!dialog.open)dialog.showModal();}
  function closeDialog(){if(dialog.open)dialog.close();confirmCallback=null;}
  function confirm(title,text,label,callback,danger=false){
    confirmCallback=callback;
    openDialog(`<div class="topo-dialog-body"><div class="topo-dialog-head"><h3>${esc(title)}</h3><button type="button" data-topo-dialog-close aria-label="Закрыть">${icon('close')}</button></div><p>${esc(text)}</p><div class="topo-dialog-actions"><button type="button" class="button" data-topo-dialog-close>Отмена</button><button type="button" class="button ${danger?'danger':'primary'}" data-topo-confirm>${esc(label)}</button></div></div>`);
  }
  function choose(kind,id,scroll=false,force=false){
    if(dirty&&!force&&selected?.kind===kind&&selected?.id===id){if(scroll&&window.innerWidth<1200)inspector.scrollIntoView({behavior:'smooth',block:'start'});return;}
    if(dirty&&!force){
      confirm('Несохранённые настройки','Сохранить настройки выбранного элемента или отказаться от изменений перед переходом.','Не сохранять',()=>{setDirty(false);choose(kind,id,scroll,true);},true);return;
    }
    const item=kind==='device'?graph.devices.find(d=>d.id===id):graph.links.find(l=>l.id===id);if(!item)return;
    selected={kind,id};draft=T.copy(item);dirty=false;renderWorld();renderInspector();
    if(scroll&&window.innerWidth<1200)inspector.scrollIntoView({behavior:'smooth',block:'start'});
  }
  function clearSelection(){selected=null;draft=null;dirty=false;renderWorld();renderInspector();}
  function showErrors(errors){const el=dialog.open?$('[data-topo-dialog-error]',dialogContent):$('[data-topo-form-error]',inspector);if(el)el.textContent=errors.slice(0,3).join(' ');notify('Проверьте значения в настройках.');}
  function commitDraft(){
    if(!selected||!dirty)return true;
    const next=T.copy(graph);let removedLinks=0;
    if(selected.kind==='device'){
      const index=next.devices.findIndex(d=>d.id===selected.id);if(index<0)return false;
      next.devices[index]=T.copy(draft);const ids=new Set(draft.nics.map(n=>n.id));
      const before=next.links.length;next.links=next.links.filter(l=>!(l.a.device===selected.id&&!ids.has(l.a.nic))&&!(l.b.device===selected.id&&!ids.has(l.b.nic)));removedLinks=before-next.links.length;
    }else{const index=next.links.findIndex(l=>l.id===selected.id);if(index<0)return false;next.links[index]=T.copy(draft);}
    const errors=T.validate(next);if(errors.length){showErrors(errors);return false;}
    const finish=()=>{graph=next;save();dirty=false;draft=T.copy(selected.kind==='device'?graph.devices.find(d=>d.id===selected.id):graph.links.find(l=>l.id===selected.id));renderWorld();renderInspector();notify('Настройки сохранены на устройстве.');};
    if(removedLinks){confirm('Удалить сетевую карту?',`Вместе с удалёнными картами будут удалены связи: ${removedLinks}. Остальные изменения также сохранятся.`, 'Сохранить и удалить',finish,true);return false;}
    finish();return true;
  }
  function requestDelete(){
    if(!selected)return;
    const sel={...selected};
    if(sel.kind==='device'){
      const d=graph.devices.find(x=>x.id===sel.id),links=graph.links.filter(l=>l.a.device===sel.id||l.b.device===sel.id).length,guests=graph.devices.filter(x=>x.hostId===sel.id).length;
      confirm('Удалить устройство?',`«${d.name}» будет удалено вместе со связями (${links}). Размещённые на нём устройства (${guests}) останутся на карте, но физический сервер у них будет снят.`, 'Удалить устройство',()=>{graph=T.removeDevice(graph,sel.id);save();clearSelection();notify('Устройство удалено.');},true);
    }else confirm('Удалить линию связи?','Интерфейсы останутся в настройках устройств и снова станут свободными.','Удалить линию',()=>{graph.links=graph.links.filter(l=>l.id!==sel.id);save();clearSelection();notify('Линия удалена.');},true);
  }
  function findFreePosition(x,y){
    const clamp=(v,max)=>Math.round(Math.max(0,Math.min(max,v)));
    const base={x:clamp(x,W.width-W.nodeWidth),y:clamp(y,W.height-W.nodeHeight)};
    for(let radius=0;radius<=12;radius++){
      const offsets=radius===0?[[0,0]]:[[0,radius*145],[radius*225,0],[-radius*225,0],[0,-radius*145],[radius*225,radius*145],[-radius*225,radius*145],[radius*225,-radius*145],[-radius*225,-radius*145]];
      for(const [dx,dy] of offsets){
        const p={x:clamp(base.x+dx,W.width-W.nodeWidth),y:clamp(base.y+dy,W.height-W.nodeHeight)};
        if(graph.devices.every(d=>p.x+W.nodeWidth+14<=d.x||d.x+W.nodeWidth+14<=p.x||p.y+W.nodeHeight+14<=d.y||d.y+W.nodeHeight+14<=p.y))return p;
      }
    }
    return base;
  }
  function newDevice(type,x,y){
    if(!T.TYPES[type])return;if(graph.devices.length>=T.LIMITS.devices){notify(`Максимум ${T.LIMITS.devices} устройств.`);return;}
    if(dirty){notify('Сначала сохраните или отмените настройки выбранного элемента.');return;}
    const pos=findFreePosition(x,y),index=graph.devices.filter(d=>d.type===type).length+1,d=T.makeDevice(type,uid('node'),pos.x,pos.y,index);
    graph.devices.push(d);save();choose('device',d.id);
    const left=d.x*zoom,right=(d.x+W.nodeWidth)*zoom,top=d.y*zoom,bottom=(d.y+W.nodeHeight)*zoom;
    if(left<viewport.scrollLeft)viewport.scrollLeft=Math.max(0,left-15);else if(right>viewport.scrollLeft+viewport.clientWidth)viewport.scrollLeft=right-viewport.clientWidth+15;
    if(top<viewport.scrollTop)viewport.scrollTop=Math.max(0,top-15);else if(bottom>viewport.scrollTop+viewport.clientHeight)viewport.scrollTop=bottom-viewport.clientHeight+15;
    notify('Устройство добавлено. Откройте его настройки справа или ниже.');
  }
  function addAtCenter(type){const x=(viewport.scrollLeft+viewport.clientWidth/2)/zoom-W.nodeWidth/2,y=(viewport.scrollTop+viewport.clientHeight/2)/zoom-W.nodeHeight/2;newDevice(type,x,y);}
  function availableOptions(device){
    const used=new Set();for(const l of graph.links)for(const e of [l.a,l.b])if(e.device===device.id)used.add(e.nic);
    return device.nics.filter(n=>!used.has(n.id)).map(n=>`<option value="${esc(n.id)}">${esc(n.name+(n.address?' · '+n.address:''))}</option>`).join('')+(device.nics.length<T.LIMITS.nics?'<option value="__new">+ Создать новую сетевую карту</option>':'');
  }
  function linkDialog(a,b){
    if(graph.links.length>=T.LIMITS.links){notify(`Максимум ${T.LIMITS.links} связей.`);return;}
    const optionsA=availableOptions(a),optionsB=availableOptions(b);
    openDialog(`<div class="topo-dialog-body"><div class="topo-dialog-head"><div><div class="topo-kicker">СОЗДАНИЕ СВЯЗИ</div><h3>Подключить устройства</h3></div><button type="button" data-topo-dialog-close aria-label="Закрыть">${icon('close')}</button></div><p>Одна карта может быть подключена к одной линии. Для общего сегмента добавьте коммутатор.</p><form data-topo-create-link novalidate><label class="topo-field"><span>${esc(a.name)} · интерфейс</span><select name="nic-a">${optionsA||'<option value="">Нет свободных карт</option>'}</select></label><label class="topo-field"><span>${esc(b.name)} · интерфейс</span><select name="nic-b">${optionsB||'<option value="">Нет свободных карт</option>'}</select></label><div class="topo-field-grid"><label class="topo-field"><span>Название связи</span><input name="name" maxlength="60" placeholder="LAN / WAN"></label><label class="topo-field"><span>Скорость, Gbit/s</span><input name="speed" type="number" min="0.01" max="400" step="0.01" value="1"></label></div><div class="topo-form-error" data-topo-dialog-error role="alert"></div><div class="topo-dialog-actions"><button type="button" class="button" data-topo-dialog-close>Отмена</button><button type="submit" class="button primary">${icon('link')} Создать линию</button></div></form></div>`);
    const form=$('[data-topo-create-link]',dialogContent);
    form.addEventListener('submit',e=>{
      e.preventDefault();const next=T.copy(graph),left=next.devices.find(d=>d.id===a.id),right=next.devices.find(d=>d.id===b.id);
      const getNIC=(d,value)=>{if(value==='__new'){const n=T.makeNIC(d);d.nics.push(n);return n.id;}return value;};
      const anic=getNIC(left,form.elements['nic-a'].value),bnic=getNIC(right,form.elements['nic-b'].value);
      const link=T.makeLink(uid('link'),left.id,anic,right.id,bnic,form.elements.namedItem('name').value.trim(),Number(form.elements.namedItem('speed').value));next.links.push(link);
      const errors=T.validate(next);if(errors.length){showErrors(errors);return;}
      graph=next;save();connecting=false;source=null;closeDialog();choose('link',link.id);notify('Линия создана. IP и другие параметры задаются в настройках сетевых карт.');
    });
  }
  function selectOnMap(id){
    if(connecting){
      if(dirty){notify('Сначала сохраните настройки выбранного элемента.');return;}
      if(!source){source=id;renderWorld();notify('Теперь выберите второе устройство.');return;}
      if(source===id){notify('Выберите другое устройство для второго конца линии.');return;}
      const a=graph.devices.find(d=>d.id===source),b=graph.devices.find(d=>d.id===id);source=null;connecting=false;renderWorld();if(a&&b)linkDialog(a,b);return;
    }
    choose('device',id);
  }
  function movePackets(now){
    if(!playing)return;
    const phase=((now-started)/2400+.06)%1;
    for(const p of geometries){
      const f=$(`[data-forward="${p.id}"]`,svg),b=$(`[data-return="${p.id}"]`,svg);if(!f||!b||!p.active)continue;
      const dx=p.x2-p.x1,dy=p.y2-p.y1,len=Math.hypot(dx,dy)||1,nx=-dy/len*4,ny=dx/len*4;
      f.setAttribute('cx',p.x1+dx*phase+nx);f.setAttribute('cy',p.y1+dy*phase+ny);
      b.setAttribute('cx',p.x1+dx*(1-phase)-nx);b.setAttribute('cy',p.y1+dy*(1-phase)-ny);
    }
  }
  function animate(now){if(!playing)return;movePackets(now);animation=requestAnimationFrame(animate);}
  function togglePlay(){
    if(!playing&&!graph.links.some(l=>l.active)){notify('Добавьте хотя бы одну включённую линию связи.');return;}
    playing=!playing;app.classList.toggle('is-playing',playing);
    if(playing){started=performance.now();animation=requestAnimationFrame(animate);}else cancelAnimationFrame(animation);
    refreshCounts();
  }
  async function exportFile(kind){
    if(busy)return;if(dirty){notify('Сначала сохраните или отмените настройки выбранного элемента.');return;}
    const errors=T.validate(graph);if(errors.length){notify('Проверьте схему перед экспортом.');return;}
    busy=true;$$('[data-topo-action="gif"],[data-topo-action="pdf"]',root).forEach(b=>b.disabled=true);
    const status=$('[data-topo-export-status]',root);status.textContent=kind==='gif'?'Создаём зацикленную анимацию…':'Создаём PDF…';
    try{
      const snapshot=T.copy(graph);
      const blob=kind==='gif'?await window.TopologyExport.gif(snapshot,(i,total)=>{status.textContent=`Создаём GIF: ${i} / ${total} кадров…`;}):await window.TopologyExport.pdf(snapshot,text=>{status.textContent=text;});
      const native=await api.saveBlob('esxi-atlas-topology.'+kind,blob);
      status.textContent=native?'Выберите место сохранения файла.':`${kind.toUpperCase()} подготовлен и сохранён.`;
      notify(native?'Выберите место сохранения файла.':`${kind.toUpperCase()} сохранён.`);
    }catch(error){status.textContent='';notify('Не удалось сохранить: '+error.message);}
    finally{busy=false;$$('[data-topo-action="gif"],[data-topo-action="pdf"]',root).forEach(b=>b.disabled=false);}
  }
  function replaceGraph(next,message){playing=false;cancelAnimationFrame(animation);app.classList.remove('is-playing');connecting=false;source=null;graph=next;save();clearSelection();setZoom((window.innerWidth<700?.65:.85)*fontRatio);viewport.scrollLeft=0;viewport.scrollTop=0;notify(message);}
  root.addEventListener('click',e=>{
    const settings=e.target.closest('[data-node-settings]');if(settings){e.stopPropagation();choose('device',settings.dataset.nodeSettings,true);return;}
    const line=e.target.closest('[data-link-id]');if(line){choose('link',line.dataset.linkId,true);return;}
    const add=e.target.closest('[data-topo-add]');if(add){addAtCenter(add.dataset.topoAdd);return;}
    const close=e.target.closest('[data-topo-dialog-close]');if(close){closeDialog();return;}
    const confirmed=e.target.closest('[data-topo-confirm]');if(confirmed){const fn=confirmCallback;closeDialog();fn?.();return;}
    const removeNic=e.target.closest('[data-topo-remove-nic]');if(removeNic&&selected?.kind==='device'){draft.nics=draft.nics.filter(n=>n.id!==removeNic.dataset.topoRemoveNic);dirty=true;renderInspector();return;}
    const button=e.target.closest('[data-topo-action]');if(!button)return;
    const action=button.dataset.topoAction;
    if(action==='connect'){if(dirty){notify('Сначала сохраните или отмените настройки.');return;}connecting=!connecting;source=null;renderWorld();return;}
    if(action==='delete'){requestDelete();return;}
    if(action==='play'){togglePlay();return;}
    if(action==='zoom-in'){setZoom(zoom+.1);return;}if(action==='zoom-out'){setZoom(zoom-.1);return;}if(action==='fit'){fit();return;}
    if(action==='gif'||action==='pdf'){exportFile(action);return;}
    if(action==='add-nic'&&selected?.kind==='device'){if(draft.nics.length>=T.LIMITS.nics){notify(`Максимум ${T.LIMITS.nics} карт на устройстве.`);return;}draft.nics.push(T.makeNIC(draft));dirty=true;renderInspector();return;}
    if(action==='discard'&&selected){draft=T.copy(selected.kind==='device'?graph.devices.find(d=>d.id===selected.id):graph.links.find(l=>l.id===selected.id));dirty=false;renderInspector();return;}
    if(action==='clear')confirm('Очистить карту?','Все устройства и линии этой карты будут удалены. Расчётный стенд и учебник останутся без изменений. Сначала сохраните проект JSON, если схема нужна позже.','Очистить карту',()=>replaceGraph({schema:1,title:'Новая схема',devices:[],links:[]},'Карта очищена.'),true);
    if(action==='example')confirm('Открыть учебный пример?','Текущая карта будет заменена примером. Сохраните проект JSON, чтобы не потерять её.','Открыть пример',()=>replaceGraph(T.demo(),'Открыт учебный пример.'));
    if(action==='from-project')confirm('Перенести сервер и ВМ из стенда?','Текущая карта будет заменена снимком расчётного проекта. Линии не создаются автоматически. Позже изменения карты и стенда не синхронизируются.','Перенести',()=>{try{replaceGraph(api.fromProject(),'Сервер и ВМ перенесены. Добавьте линии и провайдеров.');}catch(err){notify('Не удалось перенести проект: '+err.message);}});
  });
  root.addEventListener('submit',e=>{if(e.target.matches('[data-topo-device-form],[data-topo-link-edit]')){e.preventDefault();commitDraft();}});
  root.addEventListener('input',e=>{
    const el=e.target;if(!selected||!draft)return;
    if(el.dataset.topoField){draft[el.dataset.topoField]=el.value;setDirty(true);}
    if(el.dataset.topoResource){draft.resources[el.dataset.topoResource]=el.value===''?null:Number(el.value);setDirty(true);}
    if(el.dataset.topoNic&&el.dataset.topoNicField){const nic=draft.nics.find(n=>n.id===el.dataset.topoNic);if(nic){nic[el.dataset.topoNicField]=el.value.trim();setDirty(true);}}
    if(el.dataset.topoLinkEnd){draft[el.dataset.topoLinkEnd].nic=el.value;setDirty(true);}
    if(el.hasAttribute('data-topo-link-speed')){draft.speed=el.value===''?null:Number(el.value);setDirty(true);}
    if(el.hasAttribute('data-topo-link-active')){draft.active=el.checked;setDirty(true);}
    const cancel=$('[data-topo-action="discard"]',inspector);if(cancel)cancel.disabled=!dirty;
  });
  root.addEventListener('change',e=>{
    const el=e.target;
    if(el.hasAttribute('data-topo-title')){const title=el.value.trim();if(!title||title.length>80){el.value=graph.title;notify('Название схемы: от 1 до 80 символов.');return;}graph.title=title;save();notify('Название схемы сохранено.');}
  });
  root.addEventListener('dragstart',e=>{const btn=e.target.closest('[data-topo-add]');if(btn&&e.dataTransfer){e.dataTransfer.setData('text/plain','atlas:'+btn.dataset.topoAdd);e.dataTransfer.effectAllowed='copy';}});
  viewport.addEventListener('dragover',e=>{if(e.dataTransfer?.types.includes('text/plain')){e.preventDefault();e.dataTransfer.dropEffect='copy';}});
  viewport.addEventListener('drop',e=>{const data=e.dataTransfer?.getData('text/plain')||'';if(!data.startsWith('atlas:'))return;e.preventDefault();const rect=viewport.getBoundingClientRect(),x=(e.clientX-rect.left+viewport.scrollLeft)/zoom-W.nodeWidth/2,y=(e.clientY-rect.top+viewport.scrollTop)/zoom-W.nodeHeight/2;newDevice(data.slice(6),x,y);});
  world.addEventListener('pointerdown',e=>{
    const node=e.target.closest('[data-node-id]');if(!node||e.target.closest('[data-node-settings]')||(e.pointerType==='mouse'&&e.button!==0))return;
    const id=node.dataset.nodeId,d=graph.devices.find(x=>x.id===id);if(!d)return;
    if(dirty&&selected?.id!==id){choose('device',id);return;}
    drag={id,node,pointer:e.pointerId,startX:e.clientX,startY:e.clientY,x:d.x,y:d.y,moved:false};
    node.setPointerCapture(e.pointerId);e.preventDefault();
  });
  world.addEventListener('pointermove',e=>{
    if(!drag||drag.pointer!==e.pointerId||connecting)return;
    const dx=(e.clientX-drag.startX)/zoom,dy=(e.clientY-drag.startY)/zoom;if(Math.hypot(dx,dy)>4)drag.moved=true;if(!drag.moved)return;
    const d=graph.devices.find(x=>x.id===drag.id);d.x=Math.round(Math.max(0,Math.min(W.width-W.nodeWidth,drag.x+dx)));d.y=Math.round(Math.max(0,Math.min(W.height-W.nodeHeight,drag.y+dy)));
    drag.node.style.left=d.x+'px';drag.node.style.top=d.y+'px';if(selected?.kind==='device'&&selected.id===d.id&&draft){draft.x=d.x;draft.y=d.y;}renderLinks();
  });
  function endPointer(e){
    if(!drag||drag.pointer!==e.pointerId)return;const info=drag;drag=null;
    if(info.moved&&!connecting){save();if(dirty&&selected?.kind==='device'&&selected.id===info.id)renderWorld();else choose('device',info.id);notify('Положение сохранено.');}else selectOnMap(info.id);
  }
  world.addEventListener('pointerup',endPointer);
  world.addEventListener('pointercancel',e=>{if(drag&&drag.pointer===e.pointerId){if(drag.moved)save();drag=null;renderWorld();}});
  world.addEventListener('keydown',e=>{const node=e.target.closest('[data-node-id]'),line=e.target.closest('[data-link-id]');if((node||line)&&(e.key==='Enter'||e.key===' ')){e.preventDefault();if(node)selectOnMap(node.dataset.nodeId);else choose('link',line.dataset.linkId,true);}});
  const keyHandler=e=>{if(!root.isConnected||dialog.open||['INPUT','TEXTAREA','SELECT'].includes(document.activeElement?.tagName))return;if((e.key==='Delete'||e.key==='Backspace')&&selected){e.preventDefault();requestDelete();}if(e.key==='Escape'&&connecting){connecting=false;source=null;renderWorld();}};
  document.addEventListener('keydown',keyHandler);
  dialog.addEventListener('click',e=>{if(e.target===dialog)closeDialog();});
  dialog.addEventListener('close',()=>{confirmCallback=null;});
  setZoom(zoom);renderWorld();renderInspector();
  active={flush(){return !dirty||commitDraft();},setFontScale(value){const next=Math.max(1,Math.min(2,Number(value)/100||1));const cx=(viewport.scrollLeft+viewport.clientWidth/2)/zoom,cy=(viewport.scrollTop+viewport.clientHeight/2)/zoom;setZoom(zoom*next/fontRatio);fontRatio=next;viewport.scrollLeft=Math.max(0,cx*zoom-viewport.clientWidth/2);viewport.scrollTop=Math.max(0,cy*zoom-viewport.clientHeight/2);},destroy(){playing=false;cancelAnimationFrame(animation);document.removeEventListener('keydown',keyHandler);if(dirty&&selected){const next=T.copy(graph);if(selected.kind==='device'){const i=next.devices.findIndex(d=>d.id===selected.id);if(i>=0)next.devices[i]=T.copy(draft);}else{const i=next.links.findIndex(l=>l.id===selected.id);if(i>=0)next.links[i]=T.copy(draft);}if(!T.validate(next).length){graph=next;save();}else notify('Некорректные или неподтверждённые изменения элемента не сохранены.');}if(active===this)active=null;}};
  return active;
}
window.TopologyEditor={mount,unmount(){active?.destroy();},flush(){return active?active.flush():true;},setFontScale(value){active?.setFontScale(value);}};
})();
