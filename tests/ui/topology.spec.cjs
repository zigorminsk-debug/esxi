const {test,expect}=require('@playwright/test');
const fs=require('node:fs');
const {parseGIF,decompressFrames}=require('gifuct-js');
const {PDFDocument,PDFName,PDFRawStream}=require('pdf-lib');
const M=require('../../shared/model.cjs');
const T=require('../../shared/topology.cjs');

async function openMap(page){
 await page.goto('/#lab');await page.getByRole('button',{name:'Карта сети',exact:true}).click();
 await expect(page.getByRole('heading',{name:'Карта инфраструктуры'})).toBeVisible();
}
async function currentGraph(page){return page.evaluate(()=>JSON.parse(localStorage.getItem('atlas:topology-v1')));}
async function confirm(page){await page.locator('[data-topo-confirm]').click();}

test('Demo map has all device types and genuinely moves markers both ways',async({page})=>{
 const errors=[];page.on('pageerror',e=>errors.push(e.message));await openMap(page);
 await expect(page.locator('.topo-node')).toHaveCount(7);await expect(page.locator('.topo-link')).toHaveCount(6);
 await expect(page.locator('.topo-disclaimer')).toContainText('не реальный трафик');
 for(const type of ['host','vm','vrouter','router','provider','switch'])await expect(page.locator(`[data-topo-add="${type}"]`)).toBeVisible();
 await page.locator('[data-topo-action="play"]').click();await expect(page.locator('[data-topo-state]')).toContainText('включено');
 const f=page.locator('[data-forward="demo-link-1"]'),r=page.locator('[data-return="demo-link-1"]');
 await expect.poll(()=>f.getAttribute('cx')).not.toBe(null);const firstF=Number(await f.getAttribute('cx')),firstR=Number(await r.getAttribute('cx'));
 await expect.poll(async()=>Math.abs(Number(await f.getAttribute('cx'))-firstF)).toBeGreaterThan(1);
 await expect.poll(async()=>Math.abs(Number(await r.getAttribute('cx'))-firstR)).toBeGreaterThan(1);
 await page.locator('[data-topo-action="play"]').click();await expect(page.locator('[data-topo-state]')).toContainText('остановлено');
 expect(errors).toEqual([]);
});

test('Settings edit hardware, placement, multiple NICs and validate IPv4; changes persist',async({page})=>{
 await openMap(page);await page.locator('[data-node-settings="demo-host"]').click();
 await page.locator('[data-topo-resource="cpu"]').fill('2');await page.locator('[data-topo-resource="cores"]').fill('24');
 await page.locator('[data-topo-resource="ram"]').fill('256');await page.locator('[data-topo-resource="ssdCount"]').fill('4');
 await page.locator('[data-topo-action="add-nic"]').click();
 await page.locator('.topo-nic').last().locator('summary').click();
 await page.locator('[data-topo-nic="nic-5"][data-topo-nic-field="address"]').fill('172.16.0.5/24');
 await page.locator('[data-topo-nic="nic-5"][data-topo-nic-field="vlan"]').fill('20');
 await page.locator('[data-topo-nic="nic-5"][data-topo-nic-field="portGroup"]').fill('PG-TEST');
 await page.locator('[data-topo-save]').click();let g=await currentGraph(page),host=g.devices.find(d=>d.id==='demo-host');
 expect(host.resources).toMatchObject({cpu:2,cores:24,ram:256,ssdCount:4});expect(host.nics[4]).toMatchObject({address:'172.16.0.5/24',vlan:'20',portGroup:'PG-TEST'});
 await page.locator('[data-node-settings="demo-dc"]').click();
 await page.locator('[data-topo-resource="vcpu"]').fill('8');await page.locator('[data-topo-resource="coresPerSocket"]').fill('2');
 await page.locator('[data-topo-field="hostId"]').selectOption('demo-host');
 await page.locator('[data-topo-nic="nic-1"][data-topo-nic-field="address"]').fill('999.0.0.1/24');
 await page.locator('[data-topo-save]').click();await expect(page.locator('[data-topo-form-error]')).toContainText('IPv4');
 await page.locator('[data-topo-nic="nic-1"][data-topo-nic-field="address"]').fill('10.20.0.11/24');
 await page.locator('[data-topo-nic="nic-1"][data-topo-nic-field="mac"]').fill('00:11:22:aa:bb:cc');
 await page.locator('[data-topo-save]').click();g=await currentGraph(page);expect(g.devices.find(d=>d.id==='demo-dc').nics[0].address).toBe('10.20.0.11/24');
 await page.reload();await expect(page.locator('.topo-node')).toHaveCount(7);await page.locator('[data-node-settings="demo-dc"]').click();
 await expect(page.locator('[data-topo-resource="vcpu"]')).toHaveValue('8');await expect(page.locator('[data-topo-field="hostId"]')).toHaveValue('demo-host');
 await expect(page.locator('[data-topo-nic="nic-1"][data-topo-nic-field="address"]')).toHaveValue('10.20.0.11/24');
});

test('New devices do not overlap, can be connected by NIC, disabled and deleted',async({page})=>{
 await openMap(page);await page.locator('[data-topo-action="clear"]').click();await confirm(page);await expect(page.locator('.topo-node')).toHaveCount(0);
 await page.locator('[data-topo-add="provider"]').click();await page.locator('[data-topo-add="router"]').click();await expect(page.locator('.topo-node')).toHaveCount(2);
 const pos=await page.locator('.topo-node').evaluateAll(ns=>ns.map(n=>[n.style.left,n.style.top]));expect(pos[0]).not.toEqual(pos[1]);
 await page.locator('[data-topo-action="connect"]').click();await page.locator('.topo-node').nth(0).click({position:{x:12,y:12}});await page.locator('.topo-node').nth(1).click({position:{x:12,y:12}});
 await expect(page.locator('[data-topo-dialog]')).toContainText('Подключить устройства');
 await page.locator('[data-topo-create-link] [name="name"]').fill('WAN test');await page.locator('[data-topo-create-link] [name="speed"]').fill('2.5');
 await page.locator('[data-topo-create-link] button[type="submit"]').click();await expect(page.locator('.topo-link')).toHaveCount(1);
 let g=await currentGraph(page);expect(g.links[0]).toMatchObject({name:'WAN test',speed:2.5,active:true});expect(g.links[0].a.nic).toBe('nic-1');expect(g.links[0].b.nic).toBe('nic-1');
 await page.locator('[data-topo-link-active]').uncheck();await page.locator('[data-topo-save]').click();await expect(page.locator('.topo-link')).toHaveClass(/is-off/);
 await page.locator('[data-topo-action="play"]').click();await expect(page.locator('#toast')).toContainText('включённую линию');
 await page.locator('.topo-remove-button').click();await confirm(page);await expect(page.locator('.topo-link')).toHaveCount(0);
 g=await currentGraph(page);expect(g.devices).toHaveLength(2);expect(g.links).toHaveLength(0);
});

test('Removing connected NIC and host is explicit; guests survive and become unassigned',async({page})=>{
 await openMap(page);await page.locator('[data-node-settings="demo-host"]').click();
 await page.locator('.topo-nic').last().locator('summary').click();await page.locator('[data-topo-remove-nic="nic-4"]').click();await page.locator('[data-topo-save]').click();
 await expect(page.locator('[data-topo-dialog]')).toContainText('удалены связи: 1');await confirm(page);
 let g=await currentGraph(page);expect(g.links).toHaveLength(5);expect(g.devices.find(d=>d.id==='demo-host').nics).toHaveLength(3);
 await page.locator('.topo-remove-button').click();await expect(page.locator('[data-topo-dialog]')).toContainText('останутся на карте');await confirm(page);
 g=await currentGraph(page);expect(g.devices.some(d=>d.id==='demo-host')).toBe(false);expect(g.devices.find(d=>d.id==='demo-dc').hostId).toBe('');
 expect(g.devices.find(d=>d.id==='demo-firewall').hostId).toBe('');await expect(page.locator('[data-topo-warnings]')).toContainText('не выбран');
});

test('Position dragging saves and dirty settings are not discarded by reselecting the same device',async({page,isMobile})=>{
 await openMap(page);await page.locator('[data-node-settings="demo-router"]').click();
 await page.locator('[data-topo-field="name"]').fill('Тестовый маршрутизатор');
 await page.locator('[data-node-settings="demo-router"]').click();await expect(page.locator('[data-topo-field="name"]')).toHaveValue('Тестовый маршрутизатор');
 await page.locator('[data-topo-save]').click();const before=(await currentGraph(page)).devices.find(d=>d.id==='demo-router');
 const node=page.locator('[data-node-id="demo-router"]');await node.scrollIntoViewIfNeeded();const box=await node.boundingBox();
 if(isMobile){
  const client=await page.context().newCDPSession(page);
  await client.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:box.x+35,y:box.y+30,id:0}]});
  await client.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:box.x+75,y:box.y+65,id:0}]});
  await client.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});await client.detach();
 }else{await page.mouse.move(box.x+35,box.y+30);await page.mouse.down();await page.mouse.move(box.x+85,box.y+65,{steps:6});await page.mouse.up();}
 await expect.poll(async()=>{const d=(await currentGraph(page)).devices.find(x=>x.id==='demo-router');return Math.abs(d.x-before.x)+Math.abs(d.y-before.y);}).toBeGreaterThan(20);
 await page.reload();await expect(page.locator('[data-node-id="demo-router"]')).toContainText('Тестовый маршрутизатор');
});

test('Full project JSON includes the map, accepts old projects and rejects invalid map atomically',async({page})=>{
 await openMap(page);await page.locator('[data-node-settings="demo-host"]').click();await page.locator('[data-topo-resource="ram"]').fill('256');
 const download=page.waitForEvent('download');await page.getByRole('button',{name:'Сохранить проект'}).click();const d=await download,data=JSON.parse(fs.readFileSync(await d.path(),'utf8'));
 expect(data.topology.devices.find(x=>x.id==='demo-host').resources.ram).toBe(256);expect(data.config.vms).toHaveLength(7);
 data.config.host.ram=512;data.topology.devices.find(x=>x.id==='demo-host').name='Хост из JSON';
 await page.locator('#config-file-input').setInputFiles({name:'project.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(data))});
 await expect(page.locator('[data-node-id="demo-host"]')).toContainText('Хост из JSON');expect((await currentGraph(page)).devices.find(x=>x.id==='demo-host').name).toBe('Хост из JSON');
 const invalid=JSON.parse(JSON.stringify(data));invalid.config.host.ram=128;invalid.topology.links[0].a.nic='missing';
 await page.locator('#config-file-input').setInputFiles({name:'invalid.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(invalid))});
 await expect(page.locator('#toast')).toContainText('Импорт отклонён');expect((await currentGraph(page)).devices.find(x=>x.id==='demo-host').name).toBe('Хост из JSON');
 await page.locator('#config-file-input').setInputFiles({name:'old.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify({schema:1,config:M.defaultConfig()}))});
 await expect(page.locator('#toast')).toContainText('карта не изменена');await expect(page.locator('[data-node-id="demo-host"]')).toContainText('Хост из JSON');
});

test('Project snapshot imports modeled servers and VM NICs without inventing lines',async({page})=>{
 await openMap(page);await page.locator('[data-topo-action="from-project"]').click();await confirm(page);
 await expect(page.locator('.topo-node')).toHaveCount(10);await expect(page.locator('.topo-link')).toHaveCount(0);
 const g=await currentGraph(page);expect(g.devices.find(d=>d.type==='host').resources.ram).toBe(M.defaultConfig().host.ram);expect(g.devices.filter(d=>d.type==='vm').length).toBe(8);
 await page.locator('[data-topo-action="gif"]').click();await expect(page.locator('#toast')).toContainText('хотя бы одну');
});

test('GIF contains 24 visibly distinct looping frames and PDF contains the diagram and device details',async({page})=>{
 test.setTimeout(120000);await openMap(page);
 let event=page.waitForEvent('download');await page.locator('[data-topo-action="gif"]').click();let file=await event,buf=fs.readFileSync(await file.path());
 expect(file.suggestedFilename()).toBe('esxi-atlas-topology.gif');expect(buf.subarray(0,6).toString()).toBe('GIF89a');
 const parsed=parseGIF(buf),frames=decompressFrames(parsed,true);expect(frames).toHaveLength(24);expect(frames[0].delay).toBe(100);
 expect(Buffer.compare(Buffer.from(frames[0].patch),Buffer.from(frames[6].patch))).not.toBe(0);
 expect(buf.includes(Buffer.from('NETSCAPE2.0'))).toBe(true);
 event=page.waitForEvent('download');await page.locator('[data-topo-action="pdf"]').click();file=await event;buf=fs.readFileSync(await file.path());
 expect(file.suggestedFilename()).toBe('esxi-atlas-topology.pdf');expect(buf.subarray(0,5).toString()).toBe('%PDF-');
 const pdf=await PDFDocument.load(buf);expect(pdf.getPageCount()).toBeGreaterThanOrEqual(2);
 const images=pdf.context.enumerateIndirectObjects().filter(([,v])=>v instanceof PDFRawStream&&v.dict.get(PDFName.of('Subtype'))?.toString()==='/Image');
 expect(images.length).toBe(pdf.getPageCount());
});

test('Android binary bridge receives actual GIF and PDF with correct types',async({page})=>{
 test.setTimeout(120000);
 await page.addInitScript(()=>{localStorage.setItem('atlas:prefs',JSON.stringify({autoUpdate:false}));window.binaryExports=[];window.AndroidBridge={saveBinaryFile:(name,mime,base64)=>{window.binaryExports.push({name,mime,base64});return true;},saveTextFile:()=>true,openExternal:()=>{},copyText:()=>true,shareText:()=>{},checkUpdates:()=>{}};});
 await openMap(page);await page.locator('[data-topo-action="gif"]').click();await expect.poll(()=>page.evaluate(()=>window.binaryExports.length)).toBe(1);
 await page.locator('[data-topo-action="pdf"]').click();await expect.poll(()=>page.evaluate(()=>window.binaryExports.length)).toBe(2);
 const result=await page.evaluate(()=>window.binaryExports);expect(result.map(x=>x.mime)).toEqual(['image/gif','application/pdf']);
 expect(decompressFrames(parseGIF(Buffer.from(result[0].base64,'base64')),true)).toHaveLength(24);
 expect((await PDFDocument.load(Buffer.from(result[1].base64,'base64'))).getPageCount()).toBeGreaterThanOrEqual(2);
 await expect(page.locator('[data-topo-export-status]')).toContainText('Выберите место');
});

test('Map and device settings stay within the screen at 200% font',async({page})=>{
 await page.goto('/#settings');await page.locator('[data-font-scale="200"]').click();await openMap(page);
 await page.locator('[data-node-settings="demo-host"]').click();
 expect(Number((await page.locator('[data-topo-zoom]').innerText()).replace('%',''))).toBeGreaterThanOrEqual(130);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
 await page.locator('[data-topo-action="fit"]').click();await expect(page.locator('[data-topo-zoom]')).toContainText('%');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
});
