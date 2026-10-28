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
    return { id: 'vm-' + index + '-' + role.toLowerCase(), name: role.toUpperCase() + '-' + String(index).padStart(2,'0'), role, cpu: r.cpu, ram: r.ram, ssd: r.ssd, hdd: r.hdd, users: r.users, mode: 'shared', enabled: true };
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
  function planMarkdown(config, result = calculate(config,30,'peak')) {
    if(result.errors?.length) return '# Ошибки\n'+result.errors.join('\n');
    const lines=['# ESXi Atlas · план лабораторного стенда','',`Создан: ${new Date().toISOString().slice(0,10)}`,'','> Расчётная модель, не benchmark. Проверьте совместимость, лицензии, отказоустойчивость и реальные пики. Приложение не выполняет команды на ESXi.','',
      '## 1. Сервер',`- CPU: ${config.host.sockets} сокет(а) × ${config.host.cores} ядер, ${config.host.ghz} GHz (условная частота модели). HT не считается физическим ядром.`,`- RAM: ${config.host.ram} GiB; резерв гипервизора в модели: ${round(result.hostRam)} GiB.`,`- SSD: ${config.host.ssdCount} × ${config.host.ssdSize} GB; HDD: ${config.host.hddCount} × ${config.host.hddSize} GB.`,`- LAN: ${config.host.nics} × ${config.host.nicGbps} Gbit/s. Пулы — отдельные устройства, не RAID и не объединённый datastore.`,'',
      '## 2. Оценка пиковой нагрузки',`CPU demand: ${result.metrics.cpu}% условной ёмкости; активная RAM с резервом: ${result.metrics.ram}%; SSD IOPS: ${result.metrics.ssd}%; HDD IOPS: ${result.metrics.hdd}%; сеть: ${result.metrics.network}%.`,`Назначено: ${result.allocatedCpu} vCPU, ${round(result.totalRam)} GiB RAM, ${result.totalSsd} GB SSD, ${result.totalHdd} GB HDD. Thin требует контроля физического заполнения.`,'',
      '## 3. Распределение ВМ','| ВМ | Роль | vCPU | RAM GiB | SSD GB | HDD GB | Пользователи | Сеть |','|---|---|---:|---:|---:|---:|---:|---|'];
    for(const v of result.items) lines.push(`| ${v.name.replaceAll('|','/')} | ${v.role} | ${v.cpu} | ${v.ram} | ${v.ssd} | ${v.hdd} | ${v.users} | ${MODES[v.mode]?.name||'Management'} |`);
    lines.push('','## 4. Сети и интернет');
    for(const v of result.net.rows){lines.push(`### ${v.name}`,`- ${MODES[v.mode].description}`,`- LAN VLAN ${v.lan}; интернет: ${v.internet}.`);for(const n of v.nics)lines.push(`- ${n.label}: ${n.pg}, VLAN ${n.vlan}, ${n.switch}, VMXNET3; адрес примера ${n.address}; шлюз: ${n.gateway}.`);}
    lines.push('',`Management: vSwitch0 / vmnic0, PG-MGMT, отдельный management VLAN (укажите свой), VMkernel vmk0. VM LAN: ${result.net.vmSwitch}. WAN: ${result.net.wanSwitch} / ${result.net.wanUplink}.`,'WAN VLAN — транзитные сегменты до VRF firewall, а не подключение серверов напрямую к провайдеру. Для каждого выделенного WAN: своя VRF, маршрут по умолчанию, NAT и правила. В общей LAN у split-ВМ шлюза нет. Запретите transit→LAN и LAN→WAN transit, кроме необходимых разрешений.',
      'Одинаковый VLAN ID на разных изолированных коммутаторах не объединяет сети автоматически. Один физический uplink может нести несколько VLAN, но остаётся общей полосой и точкой отказа. Несколько default gateway в одной плоской подсети — не решение.',
      '', '## 5. Порядок настройки ESXi 8 / vCenter 8',
      '1. Проверить сервер, контроллеры и NIC по Broadcom Compatibility Guide; подготовить legal ISO ESXi 8 и VCSA 8 compatible build, backup и доступ к консоли.',
      '2. Настроить BIOS/UEFI, виртуализацию, контроллер и отдельный boot-диск. Установка может уничтожить данные выбранного диска.',
      '3. Установить ESXi, задать root password, настроить management IP/VLAN/DNS/NTP; проверить вход в Host Client и доступ к консоли восстановления.',
      '4. Создать VMFS-datastore на проверенном свободном устройстве. Не удалять неизвестные разделы. Настроить резерв места и мониторинг.',
      '5. Создать стандартные vSwitch и порт-группы по плану. Сначала подготовить физический trunk; management переносить только с консольным доступом.',
      '6. Развернуть VCSA 8 в два этапа; проверить прямую/обратную DNS-запись, время, SSO и лицензии. Для Tiny заложить 2 vCPU / 14 GiB / 579 GB default storage.',
      '7. Создать Datacenter и Cluster, добавить хост, назначить лицензии и роли. Не открывать management/SSH в интернет.',
      '8. Создать ВМ, установить гостевые ОС и VMware Tools, настроить диски и VMXNET3 по таблицам. Сначала DC/DNS, затем SQL, 1С, файловые и прикладные службы.',
      '9. Настроить firewall/VRF/NAT и правила только необходимых потоков. Проверить связность каждой пары сетей и отсутствие нежелательного транзита.',
      '10. Настроить backup ВМ и отдельно SQL/AD, репликацию/второй DC по требованиям, оповещения, журнал сроков паролей и сертификатов. Выполнить тест восстановления.',
      '', '## 6. Интерфейс vSphere',
      'Хост → Configure → Networking → Virtual switches: проверить switch/uplink; Add Networking → Virtual Machine Port Group for a Standard Switch: имя и VLAN из таблицы.',
      'ВМ → Edit Settings → Add Network Adapter → VMXNET3 → нужная порт-группа. Для split добавить второй адаптер; внутри гостевой ОС убрать default gateway на общей LAN.',
      'ВМ → Edit Settings → CPU/RAM: начать с таблицы; Shares relative priority, Reservation guarantee, Limit верхняя граница. Не выставлять произвольные MHz limits. Не резервировать всю RAM хоста.',
      'Названия пунктов зависят от update-релиза. Реальные скриншоты с источниками и импорт снимков вашего стенда доступны в приложении.',
      '', '## 7. Предупреждения');
    for(const w of result.warnings)lines.push(`- **${w.title}.** ${w.text}`);
    lines.push('','## 8. Проверка после настройки','- DC: DNS A/PTR, AD replication, время; SQL: backup/restore и задержки диска; 1С: тестовый вход и регламентные задания; RDP: сессии через защищённый шлюз; Bitrix: HTTPS/WAF; File: ACL и восстановление файла.', '- Провести замер рабочего пика, зафиксировать CPU Ready/RAM ballooning/datastore latency/IOPS/throughput. Не использовать этот расчёт как результат измерения.', '- Проверить отказ uplink, восстановление backup и консолидацию на тестовой ВМ в согласованное окно. Не удалять VMDK вручную.');
    return lines.join('\n');
  }
  return { ROLES, LIMITS, MODES, defaultConfig, newVM, validate, topology, calculate, optimize, commandCatalog, planMarkdown, round, clamp };
});
