const test=require('node:test');
const assert=require('node:assert/strict');
const T=require('../shared/topology.cjs');
const M=require('../shared/model.cjs');

test('Demo contains physical and virtual hosts, both router types, provider and real endpoint assignments',()=>{
 const g=T.demo();assert.deepEqual(T.validate(g),[]);
 for(const type of ['host','vm','vrouter','router','provider','switch'])assert.ok(g.devices.some(d=>d.type===type),type);
 const host=g.devices.find(d=>d.type==='host');
 assert.ok(g.devices.filter(d=>d.type==='vm'||d.type==='vrouter').every(d=>d.hostId===host.id));
 assert.equal(g.links.length,6);assert.ok(g.links.every(l=>l.active));
 assert.ok(g.devices.some(d=>d.nics.some(n=>n.address==='203.0.113.1/30')));
});

test('IPv4/prefix, gateway, MAC and VLAN are checked before a graph is accepted',()=>{
 for(const s of ['10.1.2.3/24','0.0.0.0/0','255.255.255.255/32',''])assert.equal(T.validCIDR(s),true,s);
 for(const s of ['10.1.2.3','10.1.2.3/33','999.1.2.3/24','01.2.3.4/24','1.2.3/24','example.com/24'])assert.equal(T.validCIDR(s),false,s);
 assert.ok(T.validMAC('00:11:22:aa:BB:ff'));assert.equal(T.validMAC('00-11-22-aa-bb-ff'),false);
 const g=T.demo(),nic=g.devices[0].nics[0];nic.address='300.2.3.4/24';nic.gateway='example.com';nic.mac='broken';nic.vlan='4095';
 const errors=T.validate(g).join(' ');assert.match(errors,/IPv4/);assert.match(errors,/шлюза/);assert.match(errors,/MAC/);assert.match(errors,/VLAN/);
});

test('Each point-to-point line uses distinct available NICs; invalid endpoints and self links are rejected',()=>{
 const g=T.demo();const existing=g.links[0];
 g.links.push(T.makeLink('duplicate',existing.a.device,existing.a.nic,g.devices[3].id,'nic-2'));
 assert.match(T.validate(g).join(' '),/одной карте|Одна сетевая карта/);
 const self=T.demo();self.links.push(T.makeLink('self','demo-host','nic-2','demo-host','nic-3'));
 assert.match(T.validate(self).join(' '),/два разных/);
 const missing=T.demo();missing.links[0].a.nic='not-a-nic';assert.match(T.validate(missing).join(' '),/отсутствующую/);
});

test('NIC names and IDs are unique per device and new interfaces avoid conflicts',()=>{
 const d=T.makeDevice('vm','new-vm',20,20);const n=T.makeNIC(d);assert.equal(n.name,'vNIC 3');d.nics.push(n);
 assert.equal(T.makeNIC(d).name,'vNIC 4');
 const g={schema:1,title:'Test',devices:[d],links:[]};d.nics[1].name=d.nics[0].name;
 assert.match(T.validate(g).join(' '),/уникальны/);
});

test('Resource validation and warnings distinguish overcommit from an invalid configuration',()=>{
 const g=T.demo(),host=g.devices.find(d=>d.type==='host'),guest=g.devices.find(d=>d.type==='vm');
 guest.resources.vcpu=8;guest.resources.coresPerSocket=3;assert.match(T.validate(g).join(' '),/делиться/);
 guest.resources.coresPerSocket=2;guest.resources.ram=256;guest.resources.ssd=9000;guest.resources.vcpu=80;
 assert.deepEqual(T.validate(g),[]);const warnings=T.warnings(g).join(' ');assert.match(warnings,/памяти/);assert.match(warnings,/подписка/);assert.match(warnings,/ёмкость/);
 host.resources.cpu=0;assert.match(T.validate(g).join(' '),/ресурсы физического/);
});

test('Removing a host deletes its lines but preserves guests and explicitly unassigns placement',()=>{
 const g=T.demo(),out=T.removeDevice(g,'demo-host');
 assert.ok(g.devices.some(d=>d.id==='demo-host'));assert.equal(g.links.length,6);
 assert.ok(!out.devices.some(d=>d.id==='demo-host'));assert.ok(out.devices.some(d=>d.id==='demo-dc'));
 assert.equal(out.devices.find(d=>d.id==='demo-dc').hostId,'');assert.equal(out.devices.find(d=>d.id==='demo-firewall').hostId,'');
 assert.equal(out.links.length,5);assert.deepEqual(T.validate(out),[]);assert.match(T.warnings(out).join(' '),/не выбран/);
});

test('Removing a NIC also removes only its own connection',()=>{
 const g=T.demo(),out=T.removeNIC(g,'demo-switch','nic-2');
 assert.equal(g.links.length,6);assert.equal(out.links.length,5);assert.equal(out.devices.find(d=>d.id==='demo-switch').nics.length,3);
 assert.deepEqual(T.validate(out),[]);
});

test('Import from capacity project is a separate snapshot and preserves modeled NICs without inventing links',()=>{
 const c=M.defaultConfig();c.host.ram=256;c.vms[0].mode='split';c.vms[1].mode='isolated';c.vms[2].enabled=false;
 const snapshot=T.fromConfig(c,M);assert.deepEqual(T.validate(snapshot),[]);assert.equal(snapshot.links.length,0);
 const host=snapshot.devices.find(d=>d.type==='host');assert.equal(host.resources.ram,256);
 const first=snapshot.devices.find(d=>d.name===c.vms[0].name);assert.equal(first.hostId,host.id);assert.equal(first.resources.vcpu,c.vms[0].cpu);
 assert.equal(first.nics.length,2);assert.equal(first.nics[1].portGroup,'PG-WAN-200');assert.equal(first.nics[1].vlan,'200');
 assert.equal(snapshot.devices.find(d=>d.name===c.vms[1].name).nics.length,1);
 assert.equal(snapshot.devices.some(d=>d.name===c.vms[2].name),false);
 assert.ok(snapshot.devices.some(d=>d.type==='vrouter'));assert.ok(snapshot.devices.some(d=>d.name.includes('VCSA')));
 snapshot.devices[0].resources.ram=64;assert.equal(c.host.ram,256);
});

test('Forty-VM project maps fit the bounded workspace and old OS defaults remain correct',()=>{
 const c=M.defaultConfig();c.vms=Array.from({length:40},(_,i)=>({...M.newVM(i===0?'Bitrix':'TP',i),mode:'split'}));delete c.vms[0].os;
 const g=T.fromConfig(c,M);assert.equal(g.devices.length,43);assert.deepEqual(T.validate(g),[]);
 assert.match(g.devices.find(d=>d.name===c.vms[0].name).model,/Ubuntu/);
 assert.ok(g.devices.every(d=>d.x+T.WORLD.nodeWidth<=T.WORLD.width&&d.y+T.WORLD.nodeHeight<=T.WORLD.height));
});

test('Invalid imported graph cannot contain dangling placement, duplicate IDs or oversized objects',()=>{
 const g=T.demo();g.devices.find(d=>d.type==='vm').hostId='missing-host';assert.match(T.validate(g).join(' '),/физический сервер отсутствует/);
 const dup=T.demo();dup.devices[1].id=dup.devices[0].id;assert.match(T.validate(dup).join(' '),/идентификатор/);
 assert.ok(T.validate({schema:2,title:'x',devices:[],links:[]}).length);
 assert.ok(T.validate({schema:1,title:'x',devices:Array(81).fill(null),links:[]}).length);
});

test('Diagram endpoints and bounds are deterministic and include every device',()=>{
 const g=T.demo(),a=g.devices[0],b=g.devices[1],p=T.geometry(a,b),bounds=T.bounds(g);
 assert.equal(p.x1,a.x+T.WORLD.nodeWidth);assert.equal(p.x2,b.x);
 for(const d of g.devices){assert.ok(d.x>=bounds.x&&d.y>=bounds.y);assert.ok(d.x+T.WORLD.nodeWidth<=bounds.x+bounds.width);assert.ok(d.y+T.WORLD.nodeHeight<=bounds.y+bounds.height);}
});
