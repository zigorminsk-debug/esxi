const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { chromium } = require('playwright');
const { spawn } = require('node:child_process');

const base = process.env.RAID_TEST_BASE || 'http://127.0.0.1:3005/';
let browser, preview;
before(async () => {
  if (process.env.RAID_PREVIEW_EXECUTABLE) {
    preview = spawn(process.env.RAID_PREVIEW_EXECUTABLE, ['-listen', process.env.RAID_PREVIEW_LISTEN || '127.0.0.1:3005'], {stdio:['ignore','ignore','pipe']});
    await new Promise((resolve,reject)=>{ const timer=setTimeout(()=>reject(new Error('preview did not start')),15000); preview.on('error',reject); preview.on('exit',code=>reject(new Error(`preview exited ${code}`))); preview.stderr.on('data',chunk=>{if(chunk.toString().includes('listening on')){clearTimeout(timer);resolve();}}); });
  }
  browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined, args: ['--no-sandbox'] });
});
after(async () => { if (browser) await browser.close(); if (preview) preview.kill(); });
async function pageFor(viewport={width:1440,height:900}) {
  const p = await browser.newPage({viewport});
  p.setDefaultTimeout(5000);
  return p;
}
async function certificate(p) {
  await p.fill('#address','192.0.2.10'); await p.fill('#username','root'); await p.fill('#password','test-password-not-real');
  await p.click('#certButton'); await p.locator('#certPanel').waitFor({state:'visible'});
}
async function scan(p) {
  await certificate(p); await p.click('#scanButton'); await p.locator('#diagnosticResult').waitFor({state:'visible'});
}

test('demonstration workflow: consent, package, report and no credential network request', async () => {
 const p=await pageFor(); const pageErrors=[]; const requests=[];
 p.on('pageerror',e=>pageErrors.push(e.message));p.on('request',r=>requests.push({url:r.url(),post:r.postData()}));
 try {
  await p.goto(base);assert.match(await p.locator('#demoBanner').innerText(),/Демонстрационный/);
  await scan(p);
  assert.equal(await p.locator('#password').inputValue(),'');
  assert.equal(await p.locator('#hostName').innerText(),'esxi-lab-01');
  assert.match(await p.locator('#controllerRows').innerText(),/9560-8i/);
  assert.equal(await p.locator('#downloadButton').isDisabled(),true);
  await p.check('#consent');assert.equal(await p.locator('#downloadButton').isEnabled(),true);
  await p.click('#downloadButton');await p.locator('#bundleInfo').waitFor({state:'visible'});
  assert.match(await p.locator('#bundleInfo').innerText(),/на самом деле не скачан/);
  await p.selectOption('#datastore','datastore1');await p.fill('#accessIp','192.0.2.20');await p.click('#refreshReport');
  await p.getByText('Проверьте файл и выполните dry-run').waitFor();
  assert.match(await p.locator('#steps').innerText(),/192\.0\.2\.10:35557/);
  assert.equal(pageErrors.length,0, pageErrors.join('\n'));
  assert.equal(requests.some(r=>(r.url+(r.post||'')).includes('test-password-not-real')),false);
  assert.equal(requests.filter(r=>r.post).length,0);
  await p.fill('#address','192.0.2.11');
  assert.equal(await p.locator('#diagnosticResult').isVisible(),false);
  assert.equal(await p.locator('#reportResult').isVisible(),false);
  assert.equal(await p.locator('#certPanel').isVisible(),false);
 } finally {await p.close()}
});

test('untrusted certificate requires independently entered full matching pin', async () => {
 const p=await pageFor();try {
  await p.goto(base);
  await p.evaluate(() => {
   const oldPoll=window.goPoll;
   window.goCertStart=async()=> 'untrusted';
   window.goPoll=async id=>id==='untrusted'?{state:'done',data:{sha256:'AB'.repeat(32),notAfter:'2030-01-01T00:00:00Z',trusted:false,validTime:true}}:oldPoll(id);
  });
  await certificate(p);assert.equal(await p.locator('#pinArea').isVisible(),true);
  await p.fill('#confirmedPin','incorrect');await p.click('#scanButton');
  assert.match(await p.locator('#toast').innerText(),/полный отпечаток/);
  assert.equal(await p.locator('#diagnosticResult').isVisible(),false);
  await p.fill('#confirmedPin','AB'.repeat(32));await p.click('#scanButton');
  await p.locator('#diagnosticResult').waitFor({state:'visible'});assert.equal(await p.locator('#password').inputValue(),'');
 } finally {await p.close()}
});

test('OEM fallback explains the reason and untrusted data is text, never markup', async () => {
 const p=await pageFor();try {
  await p.goto(base);
  await p.evaluate(() => {const oldPoll=window.goPoll; window.goPoll=async id=>{const r=await oldPoll(id);if(r.data&&r.data.controllers){r.data.candidate=null;r.data.candidateReason='OEM subsystem 1028:1: сверяйте пакет у производителя';r.data.name='<img src=x onerror=alert(1)>'; }return r;};});
  await scan(p);assert.equal(await p.locator('#packageNoMatch').isVisible(),true);
  assert.match(await p.locator('#noMatchReason').innerText(),/OEM subsystem/);
  assert.equal(await p.locator('#hostName img').count(),0);
  assert.equal(await p.locator('#hostName').innerText(),'<img src=x onerror=alert(1)>');
  assert.equal(await p.locator('#packageMatch').isVisible(),false);
 } finally {await p.close()}
});

test('font controls persist and the interface fits a narrow window', async () => {
 const p=await pageFor({width:800,height:700});try {
  await p.goto(base);await p.evaluate(()=>localStorage.removeItem('raid-assistant-font'));await p.reload();
  const a=await p.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--scale'));
  await p.click('#fontPlus');await p.click('#fontPlus');const b=await p.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--scale'));
  assert.ok(Number(b)>Number(a));await p.reload();assert.equal((await p.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--scale'))).trim(),b.trim());
  const overflow=await p.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth);assert.ok(overflow<=1,`horizontal overflow ${overflow}`);
 } finally {await p.close()}
});

test('additional ESXi packages are clearly informational and link to fixed vendor topics', async () => {
 const p=await pageFor();try {
  await p.goto(base);
  await p.evaluate(()=>{window.openedKind='';window.goOpenLink=async kind=>{window.openedKind=kind}});
  await p.locator('[data-nav="extensions"]').click();
  const section=p.locator('#extensions');
  assert.match(await section.innerText(),/обзор, а не автоматическая установка/i);
  assert.match(await section.innerText(),/StorCLI/);
  assert.match(await section.innerText(),/SSACLI/);
  assert.match(await section.innerText(),/iDRAC/);
  assert.match(await section.innerText(),/vGPU/);
  await section.locator('.extension-card').filter({hasText:'HPE SSACLI'}).getByRole('button',{name:/Официальный источник/}).click();
  assert.equal(await p.evaluate(()=>window.openedKind),'component:ssacli');
 }finally{await p.close()}
});


test('RAID snapshot import requires host confirmation before exact-NAA relations',async()=>{
 const p=await pageFor();try{
  const errors=[];p.on('pageerror',e=>errors.push(e.message));await p.goto(base);await scan(p);
  await p.click('#raidImport');await p.locator('#raidResult').waitFor({state:'visible'});
  assert.match(await p.locator('#raidResult').innerText(),/Вымышленные данные/);
  assert.match(await p.locator('#raidTables').innerText(),/Failed/);
  await p.click('#correlate');assert.match(await p.locator('#toast').innerText(),/Подтвердите/);
  await p.check('#reportHostConfirmed');await p.click('#correlate');
  assert.match(await p.locator('#relationsResult').innerText(),/datastore1/);
  assert.match(await p.locator('#relationsResult').innerText(),/DEMO-VM/);
  assert.equal(errors.length,0,errors.join('\n'));
 }finally{await p.close()}
});

test('SSH requires matching host key and explicit consent; monitor keeps no password in page',async()=>{
 const p=await pageFor();try{
  const errors=[];p.on('pageerror',e=>errors.push(e.message));await p.goto(base);await scan(p);
  await p.locator('#sshDetails summary').click();await p.click('#sshKeyButton');await p.locator('#sshKeyPanel').waitFor({state:'visible'});
  await p.fill('#sshUser','root');await p.fill('#sshPassword','ssh-secret-test');await p.fill('#sshPin','SHA256:wrong');await p.check('#sshConsent');await p.click('#sshRead');
  assert.match(await p.locator('#toast').innerText(),/проверенный независимо/);
  await p.fill('#sshPin','SHA256:DEMO_ONLY_NOT_A_REAL_HOST_KEY');await p.fill('#sshPassword','ssh-secret-test');await p.click('#sshRead');
  await p.locator('#raidResult').waitFor({state:'visible'});assert.equal(await p.locator('#sshPassword').inputValue(),'');
  await p.fill('#sshPassword','ssh-secret-test');await p.selectOption('#monitorMode','ssh');await p.fill('#monitorInterval','60');await p.click('#monitorStart');
  assert.equal(await p.locator('#sshPassword').inputValue(),'');assert.equal(await p.locator('#monitorStop').isEnabled(),true);
  assert.match(await p.locator('#monitorStatus').innerText(),/ДЕМО/);assert.match(await p.locator('#historyList').innerText(),/Мониторинг/);
  assert.equal(await p.evaluate(()=>JSON.stringify(localStorage).includes('ssh-secret-test')),false);
  await p.click('#monitorStop');assert.match(await p.locator('#monitorStatus').innerText(),/остановлен/);assert.equal(errors.length,0,errors.join('\n'));
 }finally{await p.close()}
});

test('saved profiles do not persist passwords; catalog, wizard, and screenshots are usable',async()=>{
 const p=await pageFor();try{
  const errors=[];p.on('pageerror',e=>errors.push(e.message));await p.goto(base);
  await p.evaluate(()=>localStorage.removeItem('raid-demo-hosts'));await p.fill('#address','192.0.2.42');await p.fill('#username','operator');await p.fill('#password','do-not-store-me');await p.fill('#profileName','Lab42');await p.click('#saveProfile');
  assert.match(await p.locator('#hostProfiles').innerText(),/Lab42/);assert.equal(await p.evaluate(()=>localStorage.getItem('raid-demo-hosts').includes('do-not-store-me')),false);
  await p.reload();await p.selectOption('#hostProfiles','192.0.2.42');assert.equal(await p.locator('#username').inputValue(),'operator');assert.equal(await p.locator('#password').inputValue(),'');
  await p.fill('#password','another-not-real');await p.click('#certButton');await p.locator('#certPanel').waitFor({state:'visible'});await p.click('#scanButton');await p.locator('#diagnosticWizard').waitFor({state:'visible'});
  assert.match(await p.locator('#diagnosticSteps').innerText(),/Факт:/);await p.click('#probePort');
  await p.locator('#componentSearch').fill('NSX');assert.match(await p.locator('#componentGrid').innerText(),/NSX host modules/);assert.equal(await p.locator('#componentGrid .extension-card').count(),1);
  await p.locator('#componentSearch').fill('');await p.selectOption('#componentFilter','found');assert.match(await p.locator('#componentGrid').innerText(),/tools-light/);
  assert.ok(await p.locator('#datastoreShot').evaluate(img=>img.naturalWidth)>0);assert.ok(await p.locator('.screenshot-grid img').nth(1).evaluate(img=>img.naturalWidth)>0);
  assert.equal(errors.length,0,errors.join('\n'));
 }finally{await p.close()}
});

test('changing host stops monitoring and removes old RAID and catalog results',async()=>{
 const p=await pageFor();try{
  const errors=[];p.on('pageerror',e=>errors.push(e.message));await p.goto(base);await scan(p);
  await p.locator('#sshDetails summary').click();await p.click('#sshKeyButton');await p.locator('#sshKeyPanel').waitFor({state:'visible'});
  await p.fill('#sshUser','root');await p.fill('#sshPassword','temporary-secret');await p.fill('#sshPin','SHA256:DEMO_ONLY_NOT_A_REAL_HOST_KEY');await p.check('#sshConsent');
  await p.selectOption('#monitorMode','ssh');await p.click('#monitorStart');await p.locator('#raidResult').waitFor({state:'visible'});
  await p.selectOption('#componentFilter','found');assert.ok(await p.locator('#componentGrid .extension-card').count()>0);
  await p.fill('#address','192.0.2.99');
  await p.waitForFunction(()=>document.querySelector('#raidResult').hidden && document.querySelector('#monitorStatus').textContent.includes('остановлен') && document.querySelector('#componentGrid').children.length===0);
  await p.waitForTimeout(3200);assert.equal(await p.locator('#raidResult').isHidden(),true);
  assert.equal(errors.length,0,errors.join('\n'));
 }finally{await p.close()}
});

test('Windows minimum size at enlarged type has no page-level horizontal overflow',async()=>{
 const p=await pageFor({width:760,height:600});try{
  await p.goto(base);await p.evaluate(()=>localStorage.removeItem('raid-assistant-font'));await p.reload();
  for(let i=0;i<7;i++)await p.click('#fontPlus');
  await scan(p);await p.click('#raidImport');await p.locator('#raidResult').waitFor({state:'visible'});
  const measure=await p.evaluate(()=>({w:document.documentElement.scrollWidth,inner:innerWidth,scale:getComputedStyle(document.documentElement).getPropertyValue('--scale')}));
  assert.ok(measure.w<=measure.inner+2,JSON.stringify(measure));assert.equal(measure.scale.trim(),'1.7');
 }finally{await p.close()}
});
