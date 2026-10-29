/* ESXi Atlas: deterministic educational capacity model, not an ESXi benchmark.
 * CPU GHz is a comparative capacity proxy, RAM GiB, disks decimal GB, network Mbit/s.
 * This module never connects to a server or executes a command.
 */
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.ESXiModel = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';
  const ROLES = {
    DC: { name: 'DC · DNS / AD', cpu: 2, ram: 4, ssd: 80, hdd: 0, users: 40, cpuUser: .012, ramUser: .012, iopsUser: 2, mbpsUser: .08, color: '#7dd3fc', note: 'DNS и время должны быть доступны до запуска зависимых ВМ. Разместите второй DC на другом оборудовании.' },
    SQL: { name: 'SQL Server', cpu: 4, ram: 24, ssd: 300, hdd: 0, users: 30, cpuUser: .075, ramUser: .08, iopsUser: 45, mbpsUser: .4, color: '#c4b5fd', note: 'Отдельные виртуальные диски данных, журнала и tempdb; политика backup согласуется с SQL. Проверьте лицензирование SQL.' },
    '1C': { name: 'Сервер 1С', cpu: 4, ram: 12, ssd: 120, hdd: 0, users: 25, cpuUser: .085, ramUser: .09, iopsUser: 15, mbpsUser: .3, color: '#fcd34d', note: 'Профиль зависит от конфигурации и фоновых заданий. Для баз на SQL нагрузка дисков учитывается на SQL, а не дублируется здесь.' },
    File: { name: 'Файловый сервер', cpu: 2, ram: 4, ssd: 60, hdd: 1000, users: 30, cpuUser: .012, ramUser: .025, iopsUser: 8, mbpsUser: 1.5, color: '#6ee7b7', note: 'Данные на HDD допустимы для архива; для активных профилей и случайного доступа нужен SSD. Теневые копии не заменяют backup.' },
    RDP: { name: 'RDP / RDS', cpu: 8, ram: 24, ssd: 160, hdd: 0, users: 20, cpuUser: .11, ramUser: .8, iopsUser: 12, mbpsUser: 1.2, color: '#fda4af', note: 'Нужны лицензии RDS и Windows. Профилируйте память на пользователя; не публикуйте TCP/UDP 3389 напрямую в интернет.' },
    Bitrix: { name: 'Bitrix / Web', cpu: 4, ram: 8, ssd: 120, hdd: 0, users: 100, cpuUser: .018, ramUser: .025, iopsUser: 4, mbpsUser: .3, color: '#5eead4', note: 'Здесь «пользователи» — активные посетители. Наружу публикуется reverse proxy/WAF через firewall; БД остаётся во внутренней сети.' },
    TP: { name: 'ТП · техподдержка', cpu: 2, ram: 4, ssd: 80, hdd: 100, users: 10, cpuUser: .025, ramUser: .08, iopsUser: 5, mbpsUser: .3, color: '#a5b4fc', note: 'ТП принято как сервис техподдержки. Название и профиль можно изменить; уточните реальные требования вашего приложения.' }
  };
  const LIMITS = { sockets: [1,2], cores: [10,50], ghz: [2,6], ram: [12,1024], ssdCount: [1,5], ssdSize: [512,2048], hddCount: [1,5], hddSize: [1000,32000], nics: [1,5], nicGbps: [1,25], ssdIops: [1000,300000], hddIops: [50,500] };
  const MODES = {
    isolated: { name: 'Своя сеть + свой интернет', short: 'Изолированная', description: 'Отдельный VLAN, отдельная зона firewall/VRF и свой WAN. Межсегментный доступ запрещён по умолчанию.' },
    shared: { name: 'Общая сеть + общий интернет', short: 'Общая', description: 'Общий VLAN 10 и WAN 1. Общая сеть не означает доверенную сеть: правила доступа всё равно нужны.' },
    split: { name: 'Общая LAN + свой интернет', short: 'Общая LAN / свой WAN', description: 'NIC 1: общая LAN без шлюза. NIC 2: отдельный transit VLAN в свою VRF со шлюзом. Провайдеры разделяются на upstream switch.' }
  };
  const clamp = (x, a, b) => Math.min(b, Math.max(a, x));
  const round = (x, n = 1) => Number(x.toFixed(n));
  function newVM(role = 'DC', index = 1) {
    const r = ROLES[role] || ROLES.DC;
    return { id: 'vm-' + index + '-' + role.toLowerCase(), name: role.toUpperCase() + '-' + String(index).padStart(2,'0'), role, cpu: r.cpu, ram: r.ram, ssd: r.ssd, hdd: r.hdd, users: r.users, mode: 'shared', enabled: true, os: role==='Bitrix'?'linux':'windows' };
  }
  function defaultConfig() {
    return { host: { sockets: 2, cores: 16, ghz: 2.6, ram: 128, ssdCount: 2, ssdSize: 1024, hddCount: 2, hddSize: 4000, nics: 3, nicGbps: 10, ssdIops: 20000, hddIops: 180 }, vms: ['DC','SQL','1C','File','RDP','Bitrix','TP'].map(newVM), vcenter: true, router: true };
  }
  function validate(config) {
    const errors = [];
    if (!config || typeof config !== 'object') return ['Некорректная конфигурация.'];
    for (const [key, [min,max]] of Object.entries(LIMITS)) {
      const v = config.host?.[key];
      if (typeof v !== 'number' || !Number.isFinite(v) || v < min || v > max || (key !== 'ghz' && !Number.isInteger(v))) errors.push(`${key}: допустимо ${min}–${max}${key === 'ghz' ? ' GHz' : ''}.`);
    }
    if (!Array.isArray(config.vms) || config.vms.length > 40) errors.push('Число ВМ должно быть от 0 до 40.');
    const ids = new Set();
    for (const v of config.vms || []) {
      if (!v || !ROLES[v.role] || !MODES[v.mode]) { errors.push('Неизвестная роль или режим сети.'); continue; }
      if (typeof v.id !== 'string' || !/^[a-zA-Z0-9_-]{1,64}$/.test(v.id) || ids.has(v.id)) errors.push('Идентификатор ВМ должен быть уникальным, до 64 букв/цифр/_/-.');
      ids.add(v.id);
      if (typeof v.name !== 'string' || !v.name.trim() || v.name.length > 63) errors.push('Имя ВМ: от 1 до 63 символов.');
      for (const [key,min,max] of [['cpu',1,100],['ram',1,1024],['ssd',20,65536],['hdd',0,160000],['users',1,10000]]) {
        const x = v[key];
        if (!Number.isInteger(x) || x < min || x > max) errors.push(`${v.name}: ${key} — целое ${min}–${max}.`);
      }
      if (typeof v.enabled !== 'boolean') errors.push(`${v.name}: некорректный флаг включения.`);
      if (v.os !== undefined && !['windows','linux'].includes(v.os)) errors.push(`${v.name}: ОС должна быть windows или linux.`);
    }
    if (typeof config.vcenter !== 'boolean' || typeof config.router !== 'boolean') errors.push('Некорректные параметры управляющих ВМ.');
    return [...new Set(errors)];
  }
  function topology(config) {
    const h = config.host;
    const wanSwitch = h.nics >= 3 ? 'vSwitch-WAN' : h.nics === 2 ? 'vSwitch-LAN' : 'vSwitch0';
    const vmSwitch = h.nics >= 2 ? 'vSwitch-LAN' : 'vSwitch0';
    const active = config.vms.filter(v => v.enabled);
    const rows = active.map(v => {
      const i = config.vms.indexOf(v);
      const lan = v.mode === 'isolated' ? 100 + i : 10;
      const wan = v.mode === 'shared' ? 1 : 200 + i;
      const nics = [{ label: 'NIC 1 · LAN', pg: `PG-LAN-${lan}`, vlan: lan, switch: vmSwitch, gateway: v.mode === 'split' ? 'Нет шлюза; при необходимости статические маршруты' : `10.${lan}.0.1 во внутреннем интерфейсе firewall/VRF`, address: `10.${lan}.0.${10+i}/24` }];
      if (v.mode === 'split') nics.push({ label: 'NIC 2 · WAN transit', pg: `PG-WAN-${wan}`, vlan: wan, switch: wanSwitch, gateway: `10.${wan}.0.1 во внутренней VRF firewall`, address: `10.${wan}.0.${10+i}/24` });
      return { ...v, lan, wan, nics, internet: v.mode === 'shared' ? 'WAN-1 · общий' : `WAN-${wan} · выделенный` };
    });
    return { rows, wanCount: new Set(rows.map(v=>v.wan)).size, lanCount: new Set(rows.map(v=>v.lan)).size, vmSwitch, wanSwitch, wanUplink: h.nics >= 3 ? 'vmnic2' : h.nics === 2 ? 'vmnic1' : 'vmnic0' };
  }
  function calculate(config, tick = 20, scenario = 'office') {
    const errors = validate(config);
    if (errors.length) return { errors };
    const h = config.host, net = topology(config);
    const physicalCores = h.sockets * h.cores;
    const capacity = physicalCores * h.ghz;
    const hostRam = Math.max(4, h.ram * .08);
    const usableRam = h.ram - hostRam;
    const ssdCap = h.ssdCount * h.ssdSize, hddCap = h.hddCount * h.hddSize;
    const ssdIops = h.ssdCount * h.ssdIops, hddIops = h.hddCount * h.hddIops;
    const t = clamp(Number(tick) || 0, 0, 60);
    const factors = { office: .65 + .22 * Math.sin(t / 7), peak: 1.15 + .15 * Math.sin(t / 5), backup: .85 + .12 * Math.sin(t / 6), night: .18 + .06 * Math.sin(t / 9) };
    const factor = factors[scenario] || factors.office;
    const backup = scenario === 'backup' ? 1 : 0;
    let items = config.vms.filter(v=>v.enabled).map(v => {
      const r = ROLES[v.role];
      const roleFactor = scenario === 'peak' && ['RDP','1C','SQL'].includes(v.role) ? 1.2 : 1;
      const demand = Math.max(.1, v.cpu * .2 + v.users * r.cpuUser * factor * roleFactor + backup * .35);
      const ram = Math.min(v.ram * 1.12, v.ram * .35 + v.users * r.ramUser * factor + backup * .4);
      const ioFactor = backup && ['File','SQL'].includes(v.role) ? 3 : 1;
      const iops = Math.max(5, v.users * r.iopsUser * factor * ioFactor + (v.role === 'SQL' ? 200 : 15));
      const ssdOps = v.role === 'File' ? iops * .15 : iops;
      const hddOps = v.role === 'File' ? iops * .85 : 0;
      const mbps = Math.max(1, v.users * r.mbpsUser * factor * (backup && v.role === 'File' ? 4 : 1));
      return { ...v, demand, ramActive: ram, iops, ssdOps, hddOps, mbps, cpuPct: demand / (v.cpu * h.ghz) * 100, ramPct: ram/v.ram*100 };
    });
    if (config.vcenter) items.push({ id:'vcsa', name:'VCSA-01 · Tiny', role:'vCenter', enabled:true, cpu:2, ram:14, ssd:579, hdd:0, users:0, mode:'shared', demand:.6 + .1*Math.sin(t/5), ramActive:11.5, iops:65, ssdOps:65, hddOps:0, mbps:8, cpuPct:10, ramPct:82, color:'#94a3b8' });
    if (config.router) items.push({ id:'router', name:'FW-01 · маршрутизатор', role:'Router', enabled:true, cpu:2, ram:2, ssd:20, hdd:0, users:0, mode:'shared', demand:.3+items.reduce((s,v)=>s+v.mbps,0)/1800, ramActive:1.2, iops:20, ssdOps:20, hddOps:0, mbps:0, cpuPct:10, ramPct:60, color:'#818cf8' });
    const totalDemand = items.reduce((s,v)=>s+v.demand,0);
    const totalRam = items.reduce((s,v)=>s+v.ram,0);
    const activeRam = items.reduce((s,v)=>s+v.ramActive,0);
    const allocatedCpu = items.reduce((s,v)=>s+v.cpu,0);
    const totalSsd = items.reduce((s,v)=>s+v.ssd,0), totalHdd = items.reduce((s,v)=>s+v.hdd,0);
    const totalSsdOps = items.reduce((s,v)=>s+v.ssdOps,0) + backup * h.ssdCount * 500;
    const totalHddOps = items.reduce((s,v)=>s+v.hddOps,0) + backup * h.hddCount * 100;
    const activeSsd = items.reduce((s,v)=>s+v.ssd*(v.role==='vCenter'?.18:.4),0)*(1+backup*.12);
    const activeHdd = totalHdd * .35 * (1+backup*.15);
    const totalMbps = items.reduce((s,v)=>s+v.mbps,0);
    const wanMpbs = Object.values(net.rows.reduce((acc,v)=>{acc[v.wan]=(acc[v.wan]||0)+v.mbps;return acc;},{}));
    const cpuPct = totalDemand/capacity*100;
    const ramPct = (activeRam+hostRam)/h.ram*100;
    const ssdPct = totalSsdOps/ssdIops*100, hddPct = totalHddOps/hddIops*100;
    const netPct = totalMbps/(h.nicGbps*1000*Math.max(1,h.nics-1))*100;
    const warnings = [];
    const warn = (severity, title, text, category) => warnings.push({severity,title,text,category});
    if (cpuPct > 80) warn('danger','CPU: пик выше 80%','Сократите параллельную нагрузку, уменьшите завышенные vCPU по фактической потребности или добавьте вычислительный ресурс. GHz разных поколений CPU не равнозначны.','cpu');
    else if (cpuPct > 65) warn('warn','CPU: запас уменьшается','Для пиков и обслуживания остаётся менее 35% модельной ёмкости. Проверьте CPU Ready и Co-Stop на реальном хосте.','cpu');
    if (allocatedCpu/physicalCores > 3) warn('warn','Высокое отношение vCPU:pCPU',`${allocatedCpu} vCPU на ${physicalCores} физических ядер. Это не жёсткий лимит ESXi: решение принимайте по Ready, готовности и рабочему профилю.`,'cpu');
    if (totalRam + hostRam > h.ram) warn('danger','Назначенная RAM превышает физическую',`ВМ назначено ${round(totalRam)} GiB; доступно после резерва ОС ${round(usableRam)} GiB. Для предсказуемой работы уменьшите размеры ВМ или добавьте RAM; не лечите дефицит лимитами памяти.`,'ram');
    else if (activeRam + hostRam > h.ram*.85) warn('warn','RAM: мало места для пиков','Модель активной памяти и резерв гипервизора занимают более 85%. Ballooning и host swapping — признаки дефицита, а не способ планового уплотнения.','ram');
    if (totalSsd > ssdCap*.8 || totalHdd > hddCap*.8) warn('danger','Диски: не соблюдён 20% резерв','Сумма виртуальных дисков превышает 80% ёмкости соответствующего пула. Thin не создаёт физическую ёмкость. Уменьшите диски, перенесите данные или расширьте хранилище; overhead, логи и снапшоты требуют дополнительного места.','storage');
    if (activeSsd > ssdCap*.8 || activeHdd > hddCap*.8) warn('danger','Прогноз заполнения datastore','Оценочные занятые данные с ростом при backup пересекают 80% пула. Это сценарная оценка, не фактическое потребление.','storage');
    if (ssdPct > 80 || hddPct > 80) warn('danger','Диски: очередь и задержка','Прогноз IOPS превышает 80% принятого в модели предела. Перенесите SQL/активные данные на SSD, разнесите нагрузку или используйте более быстрый массив. Значения IOPS задаются по измерениям.','storage');
    if (config.vms.some(v=>v.enabled && v.role==='SQL' && v.hdd>0)) warn('warn','SQL: проверьте размещение журнала','Для SQL предпочтителен SSD. Модель относит его активный I/O к SSD: если данные/журнал реально лежат на HDD, такой план непригоден без отдельного замера.','storage');
    if (netPct > 70 || Math.max(0,...wanMpbs) > h.nicGbps*1000*.7) warn('danger','Сеть: общий физический канал насыщен','VLAN и VRF изолируют трафик логически, но не увеличивают скорость общего адаптера. Разнесите WAN/LAN, повысьте скорость линка или ограничьте backup.','network');
    if (h.nics < 3) warn('warn','Недостаточно физических путей',h.nics===1?'Один NIC используется для управления, LAN и WAN через trunk. Независимого физического пути и отказоустойчивости нет.':'LAN и WAN используют один физический uplink через trunk. Для физического разделения нужны отдельные порты; management вынесен на vmnic0.','network');
    if (net.wanCount > Math.max(1,h.nics-2)) warn('info','Несколько WAN на общем uplink','Нужны поддерживаемый trunk/VLAN handoff от провайдеров и отдельная VRF/зона на firewall для каждого WAN. Иначе добавьте NIC или внешний маршрутизатор. Несколько шлюзов в одной подсети не обеспечивают изоляцию.','network');
    if (new Set([...net.rows.map(v=>v.lan), ...net.rows.filter(v=>v.wan!==1).map(v=>v.wan)]).size + 1 > 10) warn('warn','Проверьте лимит vNIC маршрутизатора','Схема предполагает много интерфейсов firewall. Проверьте лимиты конкретного устройства; при необходимости используйте внешний firewall или несколько маршрутизаторов. Модель не разворачивает их автоматически.','network');
    if (config.vcenter && 14 + hostRam > h.ram * .85) warn('danger','VCSA Tiny: недостаточный резерв RAM','Для vCenter 8 Tiny требуется 14 GiB RAM и 579 GB диска по умолчанию, плюс ресурсы гипервизора и рабочих ВМ. Проверьте всю сумму назначенной RAM; разместите vCenter вне стенда или увеличьте ресурсы.','ram');
    if (!config.vms.some(v=>v.enabled && v.role==='DC')) warn('info','Нет контроллера домена','Для доменной лаборатории нужны DNS/AD и корректное время. Не включайте зависимости от домена до проверки DNS.','network');
    if (config.router && net.wanCount > 1) warn('info','Нужна настройка firewall вне ESXi','ВМ FW-01 учтена в ресурсах, но её ПО не устанавливается стендом. Настройте VRF, NAT и deny-by-default вручную по документации выбранного firewall.','network');
    warn('info','Это расчёт, а не гарантия производительности','Нет эмуляции планировщика ESXi, NUMA, RAID, контроллеров, кэша и драйверов. Пулы дисков — сумма отдельных устройств, не общий datastore и не отказоустойчивый RAID. Проверьте проект нагрузочным тестом.','model');
    const recommendations = items.filter(v=>ROLES[v.role]).map(v=>{
      const cpu = clamp(Math.ceil(Math.max(2,v.demand/(h.ghz*.65))),2,physicalCores);
      const ram = Math.max(2,Math.ceil(v.ramActive/.7/2)*2);
      return {id:v.id,name:v.name,cpu,ram,note:cpu>v.cpu?'На пике стоит проверить увеличение vCPU.':cpu<v.cpu?'Кандидат на уменьшение vCPU после проверки в рабочем окне.':'Текущий размер допустим для этой модели.',action:ram>v.ram?'Увеличить RAM после проверки working set.':ram<v.ram?'Проверить право на уменьшение RAM; оставить запас приложениям.':'Сохранить RAM и наблюдать пики.'};
    });
    return { errors:[], tick:t, scenario, physicalCores, capacity, hostRam, usableRam, allocatedCpu, totalRam, activeRam, totalSsd, totalHdd, ssdCap, hddCap, activeSsd, activeHdd, totalSsdOps, totalHddOps, totalMbps, items, net, recommendations, warnings,
      metrics:{cpu:round(cpuPct),ram:round(ramPct),ssd:round(ssdPct),hdd:round(hddPct),network:round(netPct),storage:round(Math.max(totalSsd/ssdCap,totalHdd/hddCap)*100)},
      score: warnings.some(w=>w.severity==='danger')?'Перегрузка':warnings.some(w=>w.severity==='warn')?'Нужен запас':'Есть резерв' };
  }
  function optimize(config) {
    const c = JSON.parse(JSON.stringify(config));
    const base = calculate(c,30,'peak');
    if (base.errors?.length) return c;
    const active = c.vms.filter(v=>v.enabled);
    const fixedRam = (c.vcenter?14:0)+(c.router?2:0);
    const budget = Math.max(1,c.host.ram-base.hostRam-fixedRam);
    const sum = active.reduce((s,v)=>s+v.ram,0);
    for (const v of active) {
      const r = base.recommendations.find(x=>x.id===v.id);
      v.cpu = r.cpu;
      // This is a starting configuration, not a promise of fitting arbitrary workloads.
      v.ram = Math.max(2,Math.min(ROLES[v.role].ram,r.ram));
    }
    const newSum=active.reduce((s,v)=>s+v.ram,0);
    if(newSum>budget) for(const v of active) v.ram=Math.max(1,Math.floor(v.ram*budget/newSum));
    return c;
  }
  function commandCatalog() {
    return [
      {id:'version',title:'Версия и модель сервера',shell:'ESXi SSH',risk:'read',text:'vmware -vl\nesxcli hardware platform get',note:'Подключитесь по SSH к нужному ESXi. Команды только читают сведения.'},
      {id:'network',title:'Проверка физических и виртуальных сетей',shell:'ESXi SSH',risk:'read',text:'esxcli network nic list\nesxcli network vswitch standard list\nesxcli network ip interface ipv4 get\nesxcli network vm list',note:'Запишите имена vmnic и текущий management uplink до любых изменений.'},
      {id:'storage',title:'Datastore и подключения дисков',shell:'ESXi SSH',risk:'read',text:'df -h\nesxcli storage filesystem list\nesxcli storage core device list',note:'df -h показывает файловые системы, а не свободное место внутри каждого VMFS. Его проверяйте в Host Client.'},
      {id:'vm-list',title:'Инвентарь и питание ВМ',shell:'ESXi SSH',risk:'read',text:'vim-cmd vmsvc/getallvms\nvim-cmd vmsvc/get.summary VMID',note:'Замените VMID числом из первой команды. Не копируйте VMID буквально.'},
      {id:'logs',title:'Журналы хоста и управления',shell:'ESXi SSH',risk:'read',text:'tail -n 100 /var/log/vmkernel.log\ntail -n 100 /var/log/hostd.log',note:'Перед отправкой в мессенджер удалите IP, имена пользователей и другие чувствительные данные.'},
      {id:'snapshot',title:'Поиск ВМ, требующих консолидации',shell:'PowerCLI · Windows',risk:'read',text:'# Выполнять только после проверки $conn.SessionId\nGet-VM | Where-Object {$_.ExtensionData.Runtime.ConsolidationNeeded} |\n  Select-Object Name, PowerState, @{N="ConsolidationNeeded";E={$_.ExtensionData.Runtime.ConsolidationNeeded}}',note:'Требуется установленный VMware PowerCLI и подключение к нужному vCenter. Команда ничего не удаляет.'},
      {id:'snapshot-files',title:'Проверка дерева снапшотов',shell:'PowerCLI · Windows',risk:'read',text:'Get-VM -Name "VM_NAME" | Get-Snapshot |\n  Select-Object VM, Name, Created, SizeGB, Quiesced',note:'Замените VM_NAME. Пустой список не исключает необходимость консолидации.'},
      {id:'ntp',title:'Проверка времени ESXi',shell:'ESXi SSH',risk:'read',text:'esxcli system time get\nesxcli network firewall ruleset list | grep -i ntp\nesxcli system ntp get',note:'Проверьте также доступность NTP и фактическую синхронизацию; вывод времени сам по себе её не доказывает.'},
      {id:'portgroup',title:'Шаблон: порт-группа VLAN на существующем switch',shell:'ESXi SSH',risk:'change',text:'esxcli network vswitch standard portgroup add --portgroup-name="PG-LAN-100" --vswitch-name="vSwitch-LAN"\nesxcli network vswitch standard portgroup set --portgroup-name="PG-LAN-100" --vlan-id=100',note:'Сначала создайте vSwitch-LAN и проверьте uplink. VLAN 100 должен существовать на физическом switch. Это шаблон, а не готовая конфигурация вашего сервера.'},
      {id:'backup',title:'Проверка задач и событий ВМ',shell:'PowerCLI · Windows',risk:'read',text:'Get-VIEvent -Entity (Get-VM -Name "VM_NAME") -MaxSamples 50 |\n  Select-Object CreatedTime, FullFormattedMessage',note:'Замените VM_NAME. События могут содержать внутренние адреса: проверьте текст перед пересылкой.'}
    ];
  }

  // The personal runbook is a worked example, not a discovered configuration.
  // In particular, firewall gateways, addresses, host uplinks and DNS must be
  // confirmed by the operator before they are used on a real infrastructure.
  function defaultOS(v) { return v.os || (v.role === 'Bitrix' ? 'linux' : 'windows'); }
  function osProfile(v) {
    return defaultOS(v) === 'linux'
      ? {os:'linux', label:'Ubuntu Server 22.04 LTS (пример)', family:'Linux', version:'Ubuntu Linux (64-bit); выберите 22.04, если доступна отдельно', iso:'ISO Ubuntu Server 22.04 LTS, имя файла зависит от загрузки'}
      : {os:'windows', label:'Windows Server 2022 Desktop Experience (пример)', family:'Windows', version:'Microsoft Windows Server 2022 (64-bit)', iso:'ISO Windows Server 2022, имя файла зависит от загрузки'};
  }
  function guestHostname(name, os) {
    const max = os === 'windows' ? 15 : 63;
    return name.length <= max && /^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$/.test(name) && !/^\d+$/.test(name) ? name : '';
  }
  function dnsExample(v, rows) {
    if (v.role === 'DC' && defaultOS(v) === 'windows') {
      return {address:v.nics[0].address.split('/')[0], note:'После установки DNS/AD DS — пример собственного DNS. Для существующего домена и до запуска службы укажите согласованный доступный DNS.'};
    }
    const dc = rows.find(x => x.role === 'DC' && defaultOS(x) === 'windows' && x.lan === v.lan);
    return dc
      ? {address:dc.nics[0].address.split('/')[0], note:`Пример: ${dc.name}, если на нём установлен DNS и проверена доступность. Для домена используйте внутренний DNS, не публичный.`}
      : {address:'', note:'Модель не знает ваш DNS. Впишите адрес доступного внутреннего DNS; если DC в другой сети, сначала согласуйте маршрут и правила доступа.'};
  }
  function roleNotes(v, os) {
    const windows = os === 'windows';
    const notes = {
      DC: windows
        ? ['После настройки IP установите AD DS и DNS через Server Manager → Add Roles and Features; доменное имя, режим леса и пароль восстановления задавайте по отдельному проекту.', 'Проверьте DNS A/PTR, время, резервную копию AD и наличие второго DC.']
        : ['Microsoft AD DS не устанавливается через Server Manager в Ubuntu. Выберите Windows для этого примера либо отдельно проектируйте совместимый каталог и DNS.'],
      SQL: windows
        ? ['SQL Server устанавливается отдельно. Разделите системный диск, данные, журнал и tempdb согласно проекту, настройте резервное копирование и тест восстановления.']
        : ['Для SQL Server on Linux проверьте поддерживаемость точной версии ОС и настройте диски, память и backup по документации продукта.'],
      '1C': ['Установите сервер 1С и подключение к SQL отдельно; проверьте лицензирование, имя SQL и правила доступа между VLAN.'],
      File: windows
        ? ['На диске данных создайте общие папки; отдельно настройте разрешения NTFS, share ACL и восстановление файла из backup.']
        : ['На смонтированном диске данных настройте файловый сервис и права по документации выбранного продукта; проверьте восстановление файла.'],
      RDP: windows
        ? ['Для RDS проверьте лицензии и доступ через VPN или RD Gateway; не публикуйте 3389 напрямую в интернет.']
        : ['Windows RDS не является ролью Ubuntu; для удалённого доступа выберите отдельное поддерживаемое решение и защищённый доступ.'],
      Bitrix: ['Установите поддерживаемое окружение Bitrix отдельно. Публикуйте сайт через защищённый reverse proxy; не открывайте БД напрямую в интернет.'],
      TP: ['Уточните назначение ТП и установите соответствующий продукт отдельно; проверьте его зависимости и backup.']
    };
    return notes[v.role] || [];
  }
  function runbook(config, result = calculate(config, 30, 'peak')) {
    if (result.errors?.length) return {errors:result.errors};
    const rows = result.net.rows;
    const portGroups = [...new Map(rows.flatMap(v => v.nics).map(n => [n.pg + '@' + n.switch, n])).values()]
      .sort((a,b) => a.pg.localeCompare(b.pg));
    const hasTransit = rows.some(v => v.mode === 'split');
    const preparation = [
      'Host Client → Networking → Physical NICs: запишите имя, MAC, Link и скорость каждого физического адаптера. По кабелям и настройкам коммутатора определите, какой vmnic управляет ESXi, а какой действительно свободен. Модельные vmnic0/vmnic1/vmnic2 — не обнаруженные факты.',
      'До изменений проверьте доступ к BMC/физической консоли и резервную копию. Не отсоединяйте единственный management uplink и не меняйте его VLAN вслепую.',
      result.net.vmSwitch === 'vSwitch0'
        ? 'При одном физическом NIC модель предлагает использовать существующий vSwitch0. В Networking → Virtual switches проверьте его uplink и management; не создавайте второй vSwitch0 и не переносите управление. Все сети на этом порту разделяются только логически.'
        : `Для LAN: Networking → Virtual switches → Add standard virtual switch. В vSwitch Name введите ${result.net.vmSwitch}; в Uplink 1 выберите проверенный свободный физический порт. MTU 1500 оставьте только если он согласован по всему пути → Add. Не выбирайте занятый management uplink.`,
      hasTransit && result.net.wanSwitch !== result.net.vmSwitch
        ? `Для transit: повторите Add standard virtual switch, введите vSwitch Name = ${result.net.wanSwitch} и выберите другой проверенный свободный uplink → Add. Если свободного порта нет, сначала пересмотрите проект, не отсоединяйте management.`
        : hasTransit
          ? `Для transit модель использует тот же ${result.net.wanSwitch}: второй vSwitch и второй uplink не создаются. VLAN разделяют трафик логически, но не дают отдельный физический канал.`
          : 'Для выбранных режимов отдельная PG-WAN к рабочей ВМ не требуется: внешний WAN подключается и настраивается на firewall по его проекту.',
      'На физическом коммутаторе согласуйте порт и разрешённые VLAN с сетевым администратором. Для нескольких тегированных VLAN нужен правильно настроенный trunk; при одном uplink пропускная способность и отказ остаются общими.',
      'Host Client → Networking → Port groups → Add port group: введите Name, VLAN ID и Virtual switch из таблицы ниже → Add. Повторите для каждой строки. VLAN 0 означает нетегающий порт; не выбирайте 4095 для обычной гостевой сети.',
      'Проверьте security-политики и правила firewall: обычным ВМ не включайте Promiscuous mode, MAC address changes и Forged transmits без обоснования. Для distributed switch используйте New Distributed Port Group и согласованные uplinks.',
      'Storage → Datastores → нужный datastore → Datastore browser → Create directory → Upload: загрузите лицензированный ISO гостевой ОС и дождитесь окончания. В vCenter: datastore → Files → Upload Files.'
    ];
    const vms = rows.map(v => {
      const os = osProfile(v), lan = v.nics[0], wan = v.nics[1], dns = dnsExample(v, rows);
      const nics = v.nics.map((n, i) => ({
        label:n.label, pg:n.pg, vlan:n.vlan, switch:n.switch,
        ip:n.address.split('/')[0], subnet:`10.${n.vlan}.0.0/24`, prefix:24, mask:'255.255.255.0',
        gateway:n.gateway.startsWith('Нет') ? '' : n.gateway.split(' ')[0],
        dns:i === 0 ? dns.address : '',
        dnsDescription:i === 0 ? (dns.address || 'Укажите свой DNS') : 'Не добавлять второй DNS без проекта',
      }));
      const hostname = guestHostname(v.name, os.os);
      const wizard = [
        ['Name', v.name],
        ['Compatibility', 'ESXi 8.0 virtual machine либо версия, поддерживаемая всеми целевыми хостами'],
        ['Guest OS family', os.family], ['Guest OS version', os.version],
        ['Select storage', 'Существующий SSD-datastore с достаточным свободным местом'],
        ['CPU', `${v.cpu} vCPU`], ['Memory', `${v.ram} GiB`],
        ['Hard disk 1', `${v.ssd} GB: новый виртуальный системный диск`],
        ['Hard disk 2', v.hdd > 0 ? `${v.hdd} GB: добавить после установки ОС на проверенный datastore данных` : 'В модели не задан'],
        ['Network adapter 1', `${lan.pg}; VMXNET3; Connect at power on`],
        ['Network adapter 2', wan ? `${wan.pg}; VMXNET3; Connect at power on` : 'Не добавляется для выбранного режима'],
        ['CD/DVD drive', `Datastore ISO file → ${os.iso}; Connect at power on`],
        ['VM Options → Boot Options', 'UEFI/BIOS и Secure Boot по требованиям выбранной ОС; до установки']
      ];
      const creation = [
        'Войдите в Host Client по защищённому адресу ESXi → Virtual Machines → Create / Register VM → Create a new virtual machine → Next.',
        `На Select a name and guest OS введите Name = ${v.name}; выберите Compatibility, Guest OS family и Guest OS version по таблице ниже → Next. Имя файла ISO само по себе ОС не устанавливает.`,
        'На Select storage выберите проверенный datastore системного диска → Next. Убедитесь, что он реально существует и имеет свободное место с запасом.',
        `На Customize settings → Virtual Hardware введите CPU = ${v.cpu}, Memory = ${v.ram} GiB и Hard disk 1 = ${v.ssd} GB. Thin или Thick выбирайте по политике хранения; Thin требует контроля заполнения.`,
        `В Network Adapter 1 выберите ${lan.pg}, раскройте настройки, укажите VMXNET3 (если поддерживается гостевой ОС) и Connect at power on.${wan ? ` Нажмите Add network adapter, для второго выберите ${wan.pg}, VMXNET3 и Connect at power on.` : ''}`,
        'В CD/DVD Drive выберите Datastore ISO file → Browse → ранее загруженный ISO гостевой ОС → Select и отметьте Connect at power on.',
        'На VM Options → Boot Options проверьте совместимый с гостевой ОС BIOS/UEFI и Secure Boot. Изменение режима после установки может нарушить загрузку.',
        'Next → Ready to complete: проверьте значения → Finish. ВМ появится в Virtual Machines; ОС внутри ещё не установлена.'
      ];
      const install = os.os === 'windows' ? [
        'Выберите ВМ → Power on → Console → Open browser console / Launch Web Console. Если появится Press any key to boot from CD/DVD, нажмите любую клавишу.',
        'В Windows Setup выберите язык, время и клавиатуру → Next → Install now. Если требуется, введите лицензию своей редакции.',
        'Выберите свою лицензированную редакцию Windows Server 2022 с пометкой Desktop Experience для графического интерфейса → Next → примите лицензию.',
        `Выберите Custom: Install Windows only (advanced). Убедитесь, что выбран НОВЫЙ пустой системный виртуальный диск примерно ${v.ssd} GB. На существующей ВМ не форматируйте данные; на новом диске выберите Unallocated Space → Next.`,
        'Дождитесь установки и перезагрузки. Не нажимайте повторно загрузку с ISO; задайте уникальный пароль Administrator. Отправьте Ctrl+Alt+Delete через меню консоли и войдите.',
        `Server Manager → Local Server → Computer name → Change: задайте уникальное допустимое имя${hostname ? `, например ${hostname}` : ' (до 15 символов для Windows; имя ВМ из проекта может не подходить)'} и перезагрузите гостя.`,
        'В Host Client выберите работающую ВМ → Actions → Guest OS → Install VMware Tools. В гостевой ОС откройте смонтированный DVD → setup64.exe → Complete → Finish, затем перезагрузите.',
        'После установки Tools отключите установочный ISO: Edit settings → CD/DVD Drive → снимите подключение/Connect at power on. Если сетевого драйвера VMXNET3 нет, установите совместимые VMware Tools из ISO или проверьте поддержку ОС; не меняйте рабочий адаптер без записи его IP и MAC.'
      ] : [
        'Выберите ВМ → Power on → Console → Open browser console. В меню ISO выберите Try or Install Ubuntu Server.',
        'Выберите язык, клавиатуру и обычную установку Ubuntu Server. В Network connections выберите интерфейс по MAC из настроек ВМ → Edit IPv4 → Manual.',
        'Для каждого интерфейса введите Subnet, Address, Gateway и Name servers из таблицы NIC ниже. У общей LAN в режиме двух NIC поле Gateway оставьте пустым; на transit укажите один шлюз. Сохраните → Done.',
        'Прокси и зеркало укажите по правилам своей сети; при отсутствии доступа к интернету используйте доступный локальный источник пакетов.',
        `В Guided storage выберите только НОВЫЙ пустой системный виртуальный диск примерно ${v.ssd} GB → Done → проверьте итог → Continue. Не форматируйте существующий диск с данными.`,
        `В Profile setup задайте пользователя, пароль и уникальное допустимое имя сервера${hostname ? `, например ${hostname.toLowerCase()}` : ' (имя ВМ из проекта может не подходить)'}.`,
        'При необходимости выберите Install OpenSSH server; не открывайте SSH в интернет. Дождитесь установки → Reboot Now, затем отключите ISO в настройках CD/DVD.',
        'Установите open-vm-tools из доступного доверенного репозитория: sudo apt update, затем sudo apt install open-vm-tools. Если сети пока нет, подключите локальный источник пакетов; проверьте службу open-vm-tools.'
      ];
      const disks = v.hdd > 0 ? [
        `После установки ОС: выключите гостя в согласованное окно → Edit settings → Add hard disk → New standard hard disk. Укажите ${v.hdd} GB и проверенный datastore данных; Save.`,
        os.os === 'windows'
          ? 'В гостевой Windows откройте Disk Management (diskmgmt.msc). Сверьте размер и идентификатор НОВОГО пустого диска → Online → Initialize GPT → New Simple Volume → NTFS → назначьте букву, например D:. Не инициализируйте существующий диск.'
          : 'В Ubuntu выполните lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINTS. Сверьте идентификатор НОВОГО пустого диска. Создавайте раздел, файловую систему и запись fstab только по утверждённому плану; форматирование стирает данные.',
        'Проверьте свободное место, размещение данных и backup. Модель не объединяет физические SSD/HDD в один datastore или RAID.'
      ] : ['В модели дополнительный диск HDD не задан. Для SQL и других сервисов отдельно спланируйте VMDK data/log/tempdb и хранилище.'];
      const adapterSteps = [
        'В Host Client: Virtual Machines → нужная ВМ → Edit settings → Virtual Hardware. В vCenter: ВМ → Edit Settings → Virtual Hardware.',
        `Раскройте Network Adapter 1: Network/Port group = ${lan.pg}, Adapter Type = VMXNET3, Connect at power on = включено. Запишите MAC Address из этого адаптера.`,
        wan ? `Выберите Add network adapter (в vCenter: Add New Device → Network Adapter). Для Network Adapter 2 выберите ${wan.pg}, VMXNET3 и Connect at power on. Запишите второй MAC.` : 'Для выбранного режима достаточно одного адаптера. Не добавляйте второй без сетевого проекта.',
        'Save/OK. Включите ВМ. Если подключение изменяется у работающей ВМ, проверяйте статус Connected и совместимость hot-add; при сомнениях завершите работу гостя в согласованное окно.',
        'По MAC сопоставьте каждый сетевой адаптер vSphere с интерфейсом внутри гостевой ОС. Имена Ethernet и ens160 могут отличаться; не выбирайте их наугад.'
      ];
      const guestNetwork = os.os === 'windows' ? [
        'Откройте консоль ВМ и Windows PowerShell: Get-NetAdapter | Format-Table Name, MacAddress, Status. Сопоставьте MAC с Network Adapter 1/2 в vSphere; запишите имена Ethernet.',
        'Win+R → ncpa.cpl → Enter. Правой кнопкой по нужному Ethernet → Properties → Internet Protocol Version 4 (TCP/IPv4) → Properties → Use the following IP address.',
        `Для NIC 1 введите IP address = ${nics[0].ip}, Subnet mask = ${nics[0].mask}, Default gateway = ${nics[0].gateway || 'оставить пустым'} (пример из модели).`,
        nics[0].dns ? `В Preferred DNS server введите ${nics[0].dns} только после проверки доступности DNS; ${dns.note}` : `В Preferred DNS server укажите свой доступный DNS. ${dns.note}`,
        ...(wan ? [`Повторите Properties → IPv4 для NIC 2: IP address = ${nics[1].ip}, Subnet mask = ${nics[1].mask}, Default gateway = ${nics[1].gateway}. Не задавайте второй default gateway на NIC 1.`, 'На transit-адаптере не добавляйте второй несогласованный DNS; в Advanced → DNS отключите регистрацию адреса transit в DNS домена, если это соответствует проекту. Не включайте Network Bridge или Internet Connection Sharing.'] : []),
        'Нажмите OK → Close. Проверьте настройки из консоли; IP-адрес гостя задаётся в Windows, а не в поле VLAN ESXi.'
      ] : [
        'В консоли Ubuntu выполните ip -br link: сопоставьте MAC с Network Adapter 1/2 в vSphere. Имена ens160/ens192 здесь лишь примеры.',
        'Если ОС ещё устанавливается: Network connections → нужный интерфейс → Edit IPv4 → Manual. Заполните поля Subnet, Address, Gateway и Name servers из таблицы NIC; затем Save → Done.',
        'Для уже установленной Ubuntu проверьте sudo netplan get и ls /etc/netplan. Изменяйте только конфигурацию, которая действительно управляет этими интерфейсами: несколько YAML объединяются, а cloud-init может переписать настройки. Сохраните резервную копию.',
        'YAML ниже — пример, не готовый конфиг: замените LAN_INTERFACE/WAN_INTERFACE реальными именами, задайте работающий DNS и учтите действующий renderer. Не создавайте второй конкурирующий файл для тех же интерфейсов.',
        'Имея доступ к консоли, выполните sudo netplan generate, затем sudo netplan try. Подтвердите изменения только после проверки связи. При ошибке верните резервную копию файла; не запускайте netplan apply после отката.'
      ];
      const yaml = [
        '# Пример: заменить имена интерфейсов и проверить DNS, адреса и маршруты',
        'network:', '  version: 2', '  renderer: networkd', '  ethernets:',
        '    LAN_INTERFACE:', '      dhcp4: false', `      addresses: [${nics[0].ip}/24]`
      ];
      if (nics[0].gateway) yaml.push('      routes:', '        - to: default', `          via: ${nics[0].gateway}`);
      if (nics[0].dns) yaml.push('      nameservers:', `        addresses: [${nics[0].dns}]`);
      else yaml.push('      # Добавьте nameservers с адресом вашего действующего DNS');
      if (wan) yaml.push('    WAN_INTERFACE:', '      dhcp4: false', `      addresses: [${nics[1].ip}/24]`, '      routes:', '        - to: default', `          via: ${nics[1].gateway}`);
      const checks = os.os === 'windows' ? [
        'Get-NetAdapter | Format-Table Name, MacAddress, Status',
        'Get-NetIPConfiguration',
        'Get-NetRoute -DestinationPrefix 0.0.0.0/0',
        `Test-NetConnection ${nics[0].gateway || nics[1].gateway} # пример шлюза; ICMP может быть запрещён`
      ] : [
        'ip -br link', 'ip -br address', 'ip route show default', 'resolvectl status',
        `ping -c 3 ${nics[0].gateway || nics[1].gateway} # пример шлюза; ICMP может быть запрещён`
      ];
      return {id:v.id, name:v.name, role:v.role, mode:v.mode, os, wizard, creation, install, disks, adapterSteps, guestNetwork, nics, dns, linux:yaml.join('\n'), checks, roleNext:roleNotes(v,os.os)};
    });
    const zones = rows.map(v => ({
      id:v.id, name:v.name, mode:v.mode, wan:v.internet,
      lan:v.mode === 'split' ? 'Общая LAN, у ВМ нет default gateway на NIC 1' : `${v.nics[0].pg}: ${v.nics[0].gateway.split(' ')[0]}/24 — пример LAN-интерфейса firewall`,
      transit:v.mode === 'split' ? `${v.nics[1].pg}: ${v.nics[1].gateway.split(' ')[0]}/24 — пример transit-интерфейса firewall` : 'На рабочей ВМ transit NIC нет; отдельный WAN-интерфейс firewall — по проекту',
    }));
    return {preparation, portGroups, vms, zones};
  }
  function runbookMarkdown(config, result = calculate(config, 30, 'peak'), {vmId} = {}) {
    let rb = runbook(config, result); if (rb.errors) return '';
    if (vmId !== undefined) {
      const selected = rb.vms.filter(v => v.id === vmId);
      if (!selected.length) return '';
      const groups = new Set(selected.flatMap(v => v.nics.map(n => n.pg + '@' + n.switch)));
      rb = {...rb, vms:selected, zones:rb.zones.filter(z => z.id === vmId), portGroups:rb.portGroups.filter(n => groups.has(n.pg + '@' + n.switch))};
    }
    const cell = x => String(x ?? '').replaceAll('|','\\|').replace(/[\r\n]+/g,' ');
    const L = ['', '## 5. Пошаговый план: гостевые ВМ и сетевые карты', '',
      '> IP, VLAN, DNS, datastore, uplink и названия ISO — учебные примеры. Проверьте их до внедрения. Снимки в приложении показаны рядом с шагами; ссылки на источники ниже.',
      '', '### 5.1. Подготовьте ISO и сети'];
    rb.preparation.forEach((step,i) => L.push(`${i+1}. ${step}`));
    L.push('', '| Port group | VLAN примера | vSwitch примера | Назначение |', '|---|---:|---|---|');
    for (const n of rb.portGroups) L.push(`| ${cell(n.pg)} | ${n.vlan} | ${cell(n.switch)} | ${cell(n.label)} |`);
    const pushSteps = items => items.forEach((x,i) => L.push(`${i+1}. ${x}`));
    for (const [i,v] of rb.vms.entries()) {
      L.push('', `### 5.${i+2}. ${v.name} · ${v.role} · ${MODES[v.mode].name}`, '', `ОС в примере: ${v.os.label}.`, '', '#### 1. Создайте ВМ');
      pushSteps(v.creation);
      L.push('', '| Поле | Пример значения |', '|---|---|');
      for (const [k,val] of v.wizard) L.push(`| ${cell(k)} | ${cell(val)} |`);
      L.push('', '#### 2. Установите гостевую ОС'); pushSteps(v.install);
      L.push('', '#### 3. Дополнительный диск'); pushSteps(v.disks);
      L.push('', '#### 4. Сетевые карты в vSphere'); pushSteps(v.adapterSteps);
      L.push('', '| Адаптер | Port group | VLAN | Адрес/маска примера | Шлюз примера | DNS |', '|---|---|---:|---|---|---|');
      for (const n of v.nics) L.push(`| ${cell(n.label)} | ${cell(n.pg)} | ${n.vlan} | ${n.ip}/${n.prefix} (${n.mask}) | ${n.gateway||'не указывать'} | ${cell(n.dnsDescription)} |`);
      L.push('', `DNS: ${v.dns.note}`, '', '#### 5. Настройте IP внутри гостевой ОС'); pushSteps(v.guestNetwork);
      if (v.os.os === 'linux') L.push('', 'Netplan: только пример, замените имена интерфейсов и DNS:', '```yaml', v.linux, '```');
      L.push('', '#### 6. Проверьте из гостевой ОС', v.os.os === 'windows' ? '```powershell' : '```bash', ...v.checks, '```',
        'Проверьте DNS своим доменным именем, доступ к нужным сервисам, отсутствие доступа в запрещённые VLAN и внешний адрес через согласованный способ. Общее L2-соединение split-ВМ не является полной изоляцией.',
        '', '#### Дополнительно по роли');
      for (const x of v.roleNext) L.push('- '+x);
    }
    L.push('', `### 5.${rb.vms.length+2}. Firewall и подключение к провайдерам`,
      'Ни vSwitch, ни port group не создают интернет, NAT, маршруты или изоляцию L3. Укажите свои интерфейсы, адреса провайдеров, default route, NAT и ACL на выбранном firewall.',
      '', '| ВМ | Условный WAN | LAN на firewall | Transit на firewall |', '|---|---|---|---|');
    for (const z of rb.zones) L.push(`| ${cell(z.name)} | ${cell(z.wan)} | ${cell(z.lan)} | ${cell(z.transit)} |`);
    L.push('', 'Для каждого WAN требуется собственная согласованная политика маршрутизации; запретите нежелательный доступ между зонами. Конкретные поля зависят от модели firewall.',
      '', '### Источники реальных снимков',
      '- ESXi 8 Host Client, создание ВМ: https://www.mobinhost.com/mag/how-to-create-virtual-machine-on-vmware-esxi/',
      '- Host Client, port group и добавление сетевого адаптера: https://support.us.ovhcloud.com/hc/en-us/articles/360007000479-How-to-use-VLANs-with-multiple-VMware-ESXi-hosts',
      '- Host Client, загрузка ISO: https://support.us.ovhcloud.com/hc/en-us/articles/360003263859-How-to-create-a-VM-in-VMware-ESXi',
      '- vSphere Client, тип адаптера и MAC: https://www.nakivo.com/blog/locating-vmware-vms-by-ip-or-mac-address/',
      '- vSphere Client, ISO и аппаратные настройки: https://woshub.com/create-virtual-machine-vmware/',
      '- Windows Server 2022: https://suay.site/?p=1421 и https://petri.com/install-windows-server-2022/',
      '- Windows Server 2022 IPv4: https://serverdecode.com/assign-static-ip-address-windows-server/',
      '- Ubuntu Server 22.04: https://ahelpme.com/linux/ubuntu/installation-of-base-ubuntu-server-22-04-lts/',
      '', 'В Markdown включены ссылки, а не сами картинки; офлайн-скриншоты и их подписи находятся в приложении.', '');
    return L.join('\n');
  }

  function planMarkdown(config, result = calculate(config,30,'peak')) {
    if(result.errors?.length) return '# Ошибки\n'+result.errors.join('\n');
    const lines=['# ESXi Atlas · план лабораторного стенда','',`Создан: ${new Date().toISOString().slice(0,10)}`,'','> Расчётная модель, не benchmark. Проверьте совместимость, лицензии, отказоустойчивость и реальные пики. Приложение не выполняет команды на ESXi.','',
      '## 1. Сервер',`- CPU: ${config.host.sockets} сокет(а) × ${config.host.cores} ядер, ${config.host.ghz} GHz (условная частота модели). HT не считается физическим ядром.`,`- RAM: ${config.host.ram} GiB; резерв гипервизора в модели: ${round(result.hostRam)} GiB.`,`- SSD: ${config.host.ssdCount} × ${config.host.ssdSize} GB; HDD: ${config.host.hddCount} × ${config.host.hddSize} GB.`,`- LAN: ${config.host.nics} × ${config.host.nicGbps} Gbit/s. Пулы — отдельные устройства, не RAID и не объединённый datastore.`,'',
      '## 2. Оценка пиковой нагрузки',`CPU demand: ${result.metrics.cpu}% условной ёмкости; активная RAM с резервом: ${result.metrics.ram}%; SSD IOPS: ${result.metrics.ssd}%; HDD IOPS: ${result.metrics.hdd}%; сеть: ${result.metrics.network}%.`,`Назначено: ${result.allocatedCpu} vCPU, ${round(result.totalRam)} GiB RAM, ${result.totalSsd} GB SSD, ${result.totalHdd} GB HDD. Thin требует контроля физического заполнения.`,'',
      '## 3. Распределение ВМ','| ВМ | Роль | vCPU | RAM GiB | SSD GB | HDD GB | Пользователи | Сеть |','|---|---|---:|---:|---:|---:|---:|---|'];
    for(const v of result.items) lines.push(`| ${v.name.replaceAll('|','/')} | ${v.role} | ${v.cpu} | ${v.ram} | ${v.ssd} | ${v.hdd} | ${v.users} | ${MODES[v.mode]?.name||'Management'} |`);
    lines.push('','## 4. Сети и интернет');
    for(const v of result.net.rows){lines.push(`### ${v.name}`,`- ${MODES[v.mode].description}`,`- LAN VLAN ${v.lan}; интернет: ${v.internet}.`);for(const n of v.nics)lines.push(`- ${n.label}: ${n.pg}, VLAN ${n.vlan}, ${n.switch}, VMXNET3; адрес примера ${n.address}; шлюз: ${n.gateway}.`);}
    lines.push('',`Условный пример management: vSwitch0 / vmnic0, PG-MGMT и VMkernel vmk0. Проверьте реальные имена и VLAN; модель не обнаруживает фактические порты. VM LAN: ${result.net.vmSwitch}. Если используется transit для split: ${result.net.wanSwitch} / ${result.net.wanUplink}.`,'PG-WAN для split — внутренний транзит до firewall, а не публичный IP провайдера на сервере. У isolated-ВМ второго transit-адаптера нет; её WAN-номер — логический идентификатор, не VLAN. Для каждого выделенного WAN настройте маршрут, NAT и правила на firewall. В общей LAN у split-ВМ шлюза нет; запретите нежелательный транзит между зонами.',
      'Одинаковый VLAN ID на разных изолированных коммутаторах не объединяет сети автоматически. Один физический uplink может нести несколько VLAN, но остаётся общей полосой и точкой отказа. Несколько default gateway в одной плоской подсети — не решение.',
      runbookMarkdown(config,result), '## 6. Предупреждения');
    for(const w of result.warnings)lines.push(`- **${w.title}.** ${w.text}`);
    lines.push('','## 7. Проверка после настройки','- DC: DNS A/PTR, AD replication, время; SQL: backup/restore и задержки диска; 1С: тестовый вход и регламентные задания; RDP: сессии через защищённый шлюз; Bitrix: HTTPS/WAF; File: ACL и восстановление файла.', '- Провести замер рабочего пика, зафиксировать CPU Ready/RAM ballooning/datastore latency/IOPS/throughput. Не использовать этот расчёт как результат измерения.', '- Проверить отказ uplink, восстановление backup и консолидацию на тестовой ВМ в согласованное окно. Не удалять VMDK вручную.');
    return lines.join('\n');
  }
  return { ROLES, LIMITS, MODES, defaultConfig, newVM, validate, topology, calculate, optimize, commandCatalog, planMarkdown, runbook, runbookMarkdown, round, clamp };
});
