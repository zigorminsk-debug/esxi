/* Offline educational topology model. It does not emulate packets or configure ESXi. */
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.ESXiTopology = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';
  const WORLD = { width: 2200, height: 1800, nodeWidth: 202, nodeHeight: 120 };
  const LIMITS = { devices: 80, links: 160, nics: 24 };
  const TYPES = {
    host: { label: 'Физический сервер ESXi', short: 'ESXi · физический сервер', color: '#70a6ff', prefix: 'ESXi' },
    vm: { label: 'Виртуальный сервер', short: 'Виртуальная машина', color: '#7edab9', prefix: 'VM' },
    vrouter: { label: 'Роутер на ESXi', short: 'Виртуальный роутер', color: '#c6a2ff', prefix: 'vRouter' },
    router: { label: 'Внешний роутер', short: 'Внешний роутер', color: '#ffbf72', prefix: 'Router' },
    provider: { label: 'Провайдер', short: 'Интернет-провайдер', color: '#69d2e7', prefix: 'ISP' },
    switch: { label: 'Коммутатор / vSwitch', short: 'Коммутатор', color: '#f28fae', prefix: 'Switch' }
  };
  const copy = value => JSON.parse(JSON.stringify(value));
  const idOK = value => typeof value === 'string' && /^[a-zA-Z0-9_-]{1,64}$/.test(value);
  const shortText = (value, max, required = false) => typeof value === 'string' && value.length <= max && (!required || !!value.trim());
  const integer = (value, min, max) => Number.isInteger(value) && value >= min && value <= max;
  const number = (value, min, max) => typeof value === 'number' && Number.isFinite(value) && value >= min && value <= max;
  function validIPv4(ip) {
    if (typeof ip !== 'string' || !/^\d{1,3}(\.\d{1,3}){3}$/.test(ip)) return false;
    return ip.split('.').every(o => Number(o) <= 255 && (o === '0' || !o.startsWith('0')));
  }
  function validCIDR(value) {
    if (!value) return true;
    const m = /^(\d{1,3}(?:\.\d{1,3}){3})\/(\d{1,2})$/.exec(value);
    return !!m && validIPv4(m[1]) && Number(m[2]) <= 32;
  }
  function validMAC(value) { return !value || /^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(value); }
  function nicName(type, index) {
    if (type === 'host') return 'vmnic' + (index - 1);
    if (type === 'vm' || type === 'vrouter') return 'vNIC ' + index;
    if (type === 'switch') return 'port ' + index;
    return 'eth' + (index - 1);
  }
  function makeNIC(device, index = 1) {
    const used = new Set((device.nics || []).map(n => n.id));
    const names = new Set((device.nics || []).map(n => n.name.toLowerCase()));
    let i = Math.max(1, index);
    while (used.has('nic-' + i) || names.has(nicName(device.type, i).toLowerCase())) i++;
    return { id: 'nic-' + i, name: nicName(device.type, i), address: '', gateway: '', mac: '', vlan: '', portGroup: '' };
  }
  function makeDevice(type, id, x, y, index = 1) {
    if (!TYPES[type] || !idOK(id)) throw new Error('Неизвестный тип или идентификатор устройства.');
    const d = {
      id, type, name: TYPES[type].prefix + '-' + String(index).padStart(2, '0'),
      x: Math.round(Math.max(0, Math.min(WORLD.width - WORLD.nodeWidth, x))),
      y: Math.round(Math.max(0, Math.min(WORLD.height - WORLD.nodeHeight, y))),
      hostId: '', model: ({ host:'VMware ESXi 8', vm:'Гостевая ОС', vrouter:'Виртуальный firewall / роутер', router:'Маршрутизатор', provider:'Канал провайдера', switch:'Коммутатор / vSwitch' })[type],
      notes: '', resources: {}, nics: []
    };
    if (type === 'host') d.resources = { cpu: 2, cores: 16, ghz: 2.6, ram: 128, ssdCount: 2, ssdSize: 1024, hddCount: 2, hddSize: 4000 };
    if (type === 'vm') d.resources = { vcpu: 4, coresPerSocket: 1, ram: 16, ssd: 120, hdd: 0 };
    if (type === 'vrouter') d.resources = { vcpu: 2, coresPerSocket: 1, ram: 4, ssd: 32, hdd: 0 };
    const count = ({host:3, vm:2, vrouter:3, router:3, provider:1, switch:4})[type];
    for (let i = 1; i <= count; i++) d.nics.push(makeNIC(d, i));
    return d;
  }
  function makeLink(id, a, anic, b, bnic, name = '', speed = 1) {
    return { id, a: { device: a, nic: anic }, b: { device: b, nic: bnic }, name, speed, active: true };
  }
  function demo() {
    const provider = makeDevice('provider','demo-isp',55,240,1);
    const router = makeDevice('router','demo-router',315,240,1);
    const firewall = makeDevice('vrouter','demo-firewall',590,240,1);
    const sw = makeDevice('switch','demo-switch',865,240,1);
    const vm1 = makeDevice('vm','demo-dc',1145,90,1);
    const vm2 = makeDevice('vm','demo-app',1145,410,2);
    const host = makeDevice('host','demo-host',590,550,1);
    provider.name='Провайдер 1'; router.name='Внешний роутер'; firewall.name='vRouter'; sw.name='LAN-коммутатор';
    vm1.name='DC-01'; vm1.model='Windows Server · пример'; vm1.resources={vcpu:2,coresPerSocket:1,ram:4,ssd:80,hdd:0};
    vm2.name='APP-01'; vm2.model='Сервер приложений · пример'; vm2.resources={vcpu:4,coresPerSocket:1,ram:16,ssd:120,hdd:0};
    host.name='ESXi-01'; firewall.hostId=host.id; vm1.hostId=host.id; vm2.hostId=host.id;
    provider.nics[0].address='203.0.113.1/30';
    router.nics[0].address='203.0.113.2/30'; router.nics[0].gateway='203.0.113.1';
    router.nics[1].address='192.0.2.1/30'; firewall.nics[0].address='192.0.2.2/30'; firewall.nics[0].gateway='192.0.2.1';
    firewall.nics[1].address='10.10.0.1/24'; firewall.nics[1].portGroup='PG-LAN-10'; firewall.nics[1].vlan='10';
    vm1.nics[0].address='10.10.0.10/24'; vm1.nics[0].gateway='10.10.0.1'; vm1.nics[0].portGroup='PG-LAN-10'; vm1.nics[0].vlan='10';
    vm2.nics[0].address='10.10.0.11/24'; vm2.nics[0].gateway='10.10.0.1'; vm2.nics[0].portGroup='PG-LAN-10'; vm2.nics[0].vlan='10';
    const vmk=makeNIC(host);vmk.name='vmk0';vmk.address='10.10.0.2/24';vmk.gateway='10.10.0.1';vmk.portGroup='PG-MGMT';host.nics.push(vmk);
    host.notes='Адрес управления ESXi назначается VMkernel vmk0, а не физическому uplink vmnic. Линия показана логически: настройте vSwitch, port group и uplink отдельно.';
    const links = [
      makeLink('demo-link-1',provider.id,'nic-1',router.id,'nic-1','WAN',1),
      makeLink('demo-link-2',router.id,'nic-2',firewall.id,'nic-1','Транзит',1),
      makeLink('demo-link-3',firewall.id,'nic-2',sw.id,'nic-1','LAN',10),
      makeLink('demo-link-4',sw.id,'nic-2',vm1.id,'nic-1','LAN',10),
      makeLink('demo-link-5',sw.id,'nic-3',vm2.id,'nic-1','LAN',10),
      makeLink('demo-link-6',sw.id,'nic-4',host.id,'nic-4','Management · логически',1)
    ];
    return { schema:1, title:'Учебная схема', devices:[provider,router,firewall,sw,vm1,vm2,host], links };
  }
  function validate(graph) {
    const errors = [];
    if (!graph || typeof graph !== 'object' || graph.schema !== 1 || !shortText(graph.title, 80, true)) return ['Некорректная схема или версия формата.'];
    if (!Array.isArray(graph.devices) || graph.devices.length > LIMITS.devices) return ['Слишком много устройств или неверный список.'];
    if (!Array.isArray(graph.links) || graph.links.length > LIMITS.links) return ['Слишком много связей или неверный список.'];
    const devices = new Map();
    for (const d of graph.devices) {
      if (!d || !idOK(d.id) || devices.has(d.id) || !TYPES[d.type]) { errors.push('Некорректный или повторный идентификатор устройства.'); continue; }
      devices.set(d.id, d);
      if (!shortText(d.name,63,true) || !shortText(d.model,120) || !shortText(d.notes,300)) errors.push(`${d.name || d.id}: проверьте имя, модель и примечание.`);
      if (!integer(d.x,0,WORLD.width-WORLD.nodeWidth) || !integer(d.y,0,WORLD.height-WORLD.nodeHeight)) errors.push(`${d.name}: устройство находится за границей карты.`);
      if (typeof d.hostId !== 'string' || (d.hostId && !['vm','vrouter','switch'].includes(d.type))) errors.push(`${d.name}: неверное размещение.`);
      if (!d.resources || typeof d.resources !== 'object' || Array.isArray(d.resources)) errors.push(`${d.name}: неверные ресурсы.`);
      else if (d.type === 'host') {
        const r=d.resources;
        if (!integer(r.cpu,1,8)||!integer(r.cores,1,256)||!number(r.ghz,.5,8)||!integer(r.ram,1,65536)||!integer(r.ssdCount,0,128)||!integer(r.ssdSize,0,1000000)||!integer(r.hddCount,0,128)||!integer(r.hddSize,0,1000000)) errors.push(`${d.name}: неверные ресурсы физического сервера.`);
      } else if (d.type === 'vm' || d.type === 'vrouter') {
        const r=d.resources;
        if (!integer(r.vcpu,1,1024)||!integer(r.coresPerSocket,1,256)||r.vcpu%r.coresPerSocket!==0||!integer(r.ram,1,65536)||!integer(r.ssd,0,1000000)||!integer(r.hdd,0,1000000)) errors.push(`${d.name}: проверьте ресурсы; vCPU должно делиться на число ядер в сокете.`);
      }
      if (!Array.isArray(d.nics) || d.nics.length > LIMITS.nics) { errors.push(`${d.name}: неверный список сетевых карт.`); continue; }
      const ids = new Set(), names = new Set();
      for (const n of d.nics) {
        if (!n || !idOK(n.id) || ids.has(n.id)) { errors.push(`${d.name}: повторный или неверный ID сетевой карты.`); continue; }
        ids.add(n.id);
        if (!shortText(n.name,48,true) || names.has(n.name.toLowerCase())) errors.push(`${d.name}: имена сетевых карт должны быть уникальны.`);
        else names.add(n.name.toLowerCase());
        if (!shortText(n.address,40) || !validCIDR(n.address)) errors.push(`${d.name} / ${n.name}: IPv4 вводится как адрес/префикс, например 10.0.0.10/24.`);
        if (!shortText(n.gateway,15) || (n.gateway && !validIPv4(n.gateway))) errors.push(`${d.name} / ${n.name}: неверный IPv4 шлюза.`);
        if (!shortText(n.mac,17) || !validMAC(n.mac)) errors.push(`${d.name} / ${n.name}: MAC должен иметь вид 00:11:22:33:44:55.`);
        if (typeof n.vlan !== 'string' || (n.vlan !== '' && (!/^\d{1,4}$/.test(n.vlan) || Number(n.vlan)>4094))) errors.push(`${d.name} / ${n.name}: VLAN должен быть от 0 до 4094.`);
        if (!shortText(n.portGroup,63)) errors.push(`${d.name} / ${n.name}: слишком длинное имя port group.`);
      }
    }
    for (const d of graph.devices) if (d && d.hostId && devices.get(d.hostId)?.type !== 'host') errors.push(`${d.name}: выбранный физический сервер отсутствует.`);
    const linkIds = new Set(), occupied = new Set();
    for (const l of graph.links) {
      if (!l || !idOK(l.id) || linkIds.has(l.id)) { errors.push('Некорректный или повторный ID связи.'); continue; }
      linkIds.add(l.id);
      if (!shortText(l.name,60) || !number(l.speed,.01,400) || typeof l.active !== 'boolean') errors.push('Неверные настройки связи.');
      if (!l.a || !l.b || !idOK(l.a.device) || !idOK(l.b.device) || l.a.device === l.b.device || !idOK(l.a.nic) || !idOK(l.b.nic)) { errors.push('Связь должна соединять два разных устройства.'); continue; }
      for (const e of [l.a,l.b]) {
        const d=devices.get(e.device), key=e.device+'/'+e.nic;
        if (!d || !Array.isArray(d.nics) || !d.nics.some(n => n.id===e.nic)) errors.push('Связь ссылается на отсутствующую сетевую карту.');
        if (occupied.has(key)) errors.push('Одна сетевая карта не может быть подключена к нескольким линиям. Используйте коммутатор.');
        occupied.add(key);
      }
    }
    return [...new Set(errors)];
  }
  function removeDevice(graph, id) {
    const next=copy(graph);
    next.devices=next.devices.filter(d=>d.id!==id);
    next.links=next.links.filter(l=>l.a.device!==id && l.b.device!==id);
    for (const d of next.devices) if (d.hostId===id) d.hostId='';
    return next;
  }
  function removeNIC(graph, deviceId, nicId) {
    const next=copy(graph), device=next.devices.find(d=>d.id===deviceId);
    if (device) device.nics=device.nics.filter(n=>n.id!==nicId);
    next.links=next.links.filter(l=>!(l.a.device===deviceId && l.a.nic===nicId) && !(l.b.device===deviceId && l.b.nic===nicId));
    return next;
  }
  function warnings(graph) {
    const messages=[];
    for (const d of graph.devices.filter(d=>d.type==='vm'||d.type==='vrouter')) if (!d.hostId) messages.push(`${d.name}: не выбран физический сервер.`);
    for (const host of graph.devices.filter(d=>d.type==='host')) {
      const guests=graph.devices.filter(d=>(d.type==='vm'||d.type==='vrouter')&&d.hostId===host.id);
      const sum=key=>guests.reduce((n,d)=>n+d.resources[key],0), r=host.resources;
      if (sum('ram')>r.ram) messages.push(`${host.name}: назначено ${sum('ram')} GiB RAM при ${r.ram} GiB физической памяти; учтите также резерв ESXi.`);
      if (sum('vcpu')>r.cpu*r.cores) messages.push(`${host.name}: ${sum('vcpu')} vCPU на ${r.cpu*r.cores} физических ядер. Это допустимая подписка ресурсов, но проверьте реальную нагрузку.`);
      if (sum('ssd')>r.ssdCount*r.ssdSize || sum('hdd')>r.hddCount*r.hddSize) messages.push(`${host.name}: назначенная ёмкость превышает сумму дисков соответствующего типа. RAID, резерв и thin provisioning не учитываются.`);
    }
    return messages;
  }
  function fromConfig(config, model) {
    const host=makeDevice('host','project-host',35,60,1), h=config.host;
    host.name='ESXi-01';
    host.resources={cpu:h.sockets,cores:h.cores,ghz:h.ghz,ram:h.ram,ssdCount:h.ssdCount,ssdSize:h.ssdSize,hddCount:h.hddCount,hddSize:h.hddSize};
    host.nics=[]; for(let i=1;i<=h.nics;i++) host.nics.push(makeNIC(host,i));
    const guests=[];
    if(config.vcenter) guests.push({type:'vm', name:'vCenter · VCSA', model:'VCSA 8 Tiny · пример', cpu:2, ram:14, ssd:579, hdd:0, nets:[]});
    if(config.router) guests.push({type:'vrouter', name:'Firewall', model:'Виртуальный firewall · пример', cpu:2, ram:2, ssd:32, hdd:0, nets:[]});
    const rows=model.topology(config).rows;
    for(const v of config.vms.filter(v=>v.enabled)) {
      const net=rows.find(r=>r.id===v.id);
      guests.push({type:'vm', name:v.name, model:((v.os|| (v.role==='Bitrix'?'linux':'windows'))==='linux'?'Ubuntu Server 22.04':'Windows Server 2022')+' · '+v.role, cpu:v.cpu, ram:v.ram, ssd:v.ssd, hdd:v.hdd, nets:net?.nics||[]});
    }
    const devices=[host];
    guests.forEach((v,i)=>{
      const d=makeDevice(v.type,'project-vm-'+(i+1),310+(i%7)*265,60+Math.floor(i/7)*185,i+1);
      d.name=v.name; d.model=v.model; d.hostId=host.id; d.resources={vcpu:v.cpu,coresPerSocket:1,ram:v.ram,ssd:v.ssd,hdd:v.hdd};
      d.notes='Перенесено из расчётного стенда. Значения сетей — учебные примеры, их необходимо согласовать.';
      if(v.nets.length){d.nics=[];for(const n of v.nets){const nic=makeNIC(d);nic.address=n.address||'';nic.vlan=String(n.vlan);nic.portGroup=n.pg;d.nics.push(nic);}}
      devices.push(d);
    });
    return {schema:1,title:'Схема из проекта стенда',devices,links:[]};
  }
  function geometry(a,b) {
    const hw=WORLD.nodeWidth/2, hh=WORLD.nodeHeight/2;
    const ax=a.x+hw, ay=a.y+hh, bx=b.x+hw, by=b.y+hh;
    let dx=bx-ax, dy=by-ay;
    if (dx===0 && dy===0) dx=1;
    const factor=1/Math.max(Math.abs(dx)/hw,Math.abs(dy)/hh);
    return {x1:ax+dx*factor,y1:ay+dy*factor,x2:bx-dx*factor,y2:by-dy*factor};
  }
  function bounds(graph, padding=70) {
    if (!graph.devices.length) return {x:0,y:0,width:900,height:500};
    const minX=Math.min(...graph.devices.map(d=>d.x))-padding, minY=Math.min(...graph.devices.map(d=>d.y))-padding;
    const maxX=Math.max(...graph.devices.map(d=>d.x+WORLD.nodeWidth))+padding, maxY=Math.max(...graph.devices.map(d=>d.y+WORLD.nodeHeight))+padding;
    return {x:minX,y:minY,width:Math.max(300,maxX-minX),height:Math.max(200,maxY-minY)};
  }
  return { WORLD, LIMITS, TYPES, copy, validIPv4, validCIDR, validMAC, makeNIC, makeDevice, makeLink, demo, validate, removeDevice, removeNIC, warnings, fromConfig, geometry, bounds };
});
