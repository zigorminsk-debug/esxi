const test=require('node:test');
const assert=require('node:assert/strict');
const M=require('../shared/model.cjs');
const C=require('../shared/course.cjs');
const clone=x=>JSON.parse(JSON.stringify(x));
test('Default lab includes every requested role and valid capacity',()=>{
 const c=M.defaultConfig(),r=M.calculate(c);
 assert.deepEqual(r.errors,[]);assert.deepEqual(c.vms.map(v=>v.role),['DC','SQL','1C','File','RDP','Bitrix','TP']);
 assert.equal(r.physicalCores,32);assert.equal(r.totalRam,96);assert.equal(r.ssdCap,2048);assert.equal(r.hddCap,8000);assert.equal(r.items.length,9);
});
test('All requested hardware bounds are accepted and out-of-range values rejected',()=>{
 const c=M.defaultConfig();
 for(const [key,[min,max]]of Object.entries(M.LIMITS)){
  for(const value of [min,max]){const d=clone(c);d.host[key]=value;assert.deepEqual(M.validate(d),[],key+' '+value);}
  for(const value of [min-1,max+1,NaN,Infinity,'2']){const d=clone(c);d.host[key]=value;assert.ok(M.validate(d).length,key+' '+value);}
 }
});
test('Invalid input cannot produce plan or crash calculation',()=>{
 const c=M.defaultConfig();c.host.cores=999;assert.ok(M.calculate(c).errors.length);assert.match(M.planMarkdown(c),/Ошибки/);
 const d=M.defaultConfig();d.vms[0].id=d.vms[1].id;assert.ok(M.validate(d).some(e=>e.includes('уникальным')));
});
test('Three network modes have distinct L2 and routed WAN semantics',()=>{
 const c=M.defaultConfig();c.vms=c.vms.slice(0,3);c.vms.forEach((v,i)=>v.mode=['shared','isolated','split'][i]);const n=M.topology(c);
 assert.equal(n.wanCount,3);assert.equal(n.lanCount,2);assert.equal(n.rows[0].nics.length,1);assert.equal(n.rows[1].lan,101);assert.equal(n.rows[2].nics.length,2);assert.equal(n.rows[2].lan,10);assert.match(n.rows[2].nics[0].gateway,/Нет шлюза/);assert.equal(n.rows[2].nics[1].vlan,202);
 assert.equal(new Set(n.rows.map(v=>v.nics[0].vlan)).size,2);
});
test('Disabled VMs do not consume model resources',()=>{
 const c=M.defaultConfig();c.vms.forEach(v=>v.enabled=false);const r=M.calculate(c);assert.equal(r.items.length,2);assert.equal(r.totalRam,16);assert.equal(r.net.rows.length,0);
});
test('VCSA and firewall can be explicitly external',()=>{
 const c=M.defaultConfig();c.vcenter=false;c.router=false;const r=M.calculate(c);assert.equal(r.items.length,7);assert.equal(r.totalRam,80);
});
test('Scenarios are deterministic and backup increases storage demand',()=>{
 const c=M.defaultConfig();assert.deepEqual(M.calculate(c,25,'office'),M.calculate(c,25,'office'));
 assert.ok(M.calculate(c,25,'peak').metrics.cpu>M.calculate(c,25,'night').metrics.cpu);
 assert.ok(M.calculate(c,25,'backup').totalHddOps>M.calculate(c,25,'office').totalHddOps);
});
test('RAM overcommit, storage exhaustion, CPU and network saturation are flagged',()=>{
 const c=M.defaultConfig();c.host.ram=32;let r=M.calculate(c);assert.ok(r.warnings.some(w=>w.category==='ram'&&w.severity==='danger'));
 c.host.ram=128;c.vms.forEach(v=>{v.users=10000;v.ssd=20000;v.cpu=100;v.ram=100;});c.host.ssdSize=512;c.host.nicGbps=1;r=M.calculate(c,30,'peak');
 for(const category of ['cpu','ram','storage','network'])assert.ok(r.warnings.some(w=>w.category===category&&w.severity==='danger'),category);
});
test('Single NIC does not claim physical independence',()=>{
 const c=M.defaultConfig();c.host.nics=1;c.vms[0].mode='isolated';const r=M.calculate(c);assert.equal(r.net.wanUplink,'vmnic0');assert.ok(r.warnings.some(w=>w.title.includes('Недостаточно физических')));
});
test('Many isolated WANs warn about firewall interface limits',()=>{
 const c=M.defaultConfig();c.vms=Array.from({length:12},(_,i)=>({...M.newVM('TP',i+1),mode:'isolated'}));const r=M.calculate(c);assert.ok(r.warnings.some(w=>w.title.includes('vNIC')));
});
test('Optimizer preserves hardware, network and disks and produces valid sizes',()=>{
 const c=M.defaultConfig();c.vms[1].hdd=200;const o=M.optimize(c);assert.deepEqual(o.host,c.host);assert.deepEqual(o.vms.map(v=>v.mode),c.vms.map(v=>v.mode));assert.deepEqual(o.vms.map(v=>[v.ssd,v.hdd]),c.vms.map(v=>[v.ssd,v.hdd]));assert.deepEqual(M.validate(o),[]);
});
test('Personalized export includes every enabled VM and all NICs, with caveats',()=>{
 const c=M.defaultConfig();c.vms[0].name='DC-TEST';c.vms[1].mode='split';const md=M.planMarkdown(c);assert.match(md,/DC-TEST/);assert.match(md,/NIC 2/);assert.match(md,/VCSA/);assert.match(md,/Расчётная модель/);assert.match(md,/VLAN 201/);assert.doesNotMatch(md,/undefined|NaN/);
});
test('Commands are read-only except explicitly marked template; no credential placeholders',()=>{
 for(const cmd of M.commandCatalog()){assert.ok(['read','change'].includes(cmd.risk));assert.doesNotMatch(cmd.text,/password\s*[=:]\s*\S+|Remove-|rm\s+-rf|vmkfstools.*-[EUX]/i);}
 assert.equal(M.commandCatalog().filter(c=>c.risk==='change').length,1);
});
test('16 substantial chapters, unique IDs, real screenshot assets and source links',()=>{
 assert.equal(C.chapters.length,16);assert.equal(new Set(C.chapters.map(c=>c.id)).size,16);
 const fs=require('node:fs'),path=require('node:path');
 for(const ch of C.chapters){assert.ok(ch.blocks.length>4);assert.ok(ch.blocks.some(b=>b.type==='source'));for(const b of ch.blocks){if(b.type==='image'){assert.ok(fs.existsSync(path.join(__dirname,'../app/assets/screenshots',b.file)));assert.match(b.source,/^https:\/\//);assert.ok(b.version);}}}
});
test('No accidental non-Russian CJK text in original content',()=>{
 assert.doesNotMatch(JSON.stringify(C),/[\u3400-\u9fff]/);assert.doesNotMatch(M.planMarkdown(M.defaultConfig()),/[\u3400-\u9fff]/);
});

test('Optimizer never emits fractional or invalid VM resources on a tiny host',()=>{const c=M.defaultConfig();c.host.ram=12;c.host.cores=10;const o=M.optimize(c);assert.deepEqual(M.validate(o),[]);});
