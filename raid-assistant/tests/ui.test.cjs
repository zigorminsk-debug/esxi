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
