const { test, expect } = require('@playwright/test');
const fs=require('node:fs');
test('Home, course, search and progress work without JS errors',async({page})=>{
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/');await expect(page.locator('.hero')).toContainText('Настраивать осознанно');
 await page.locator('#search-trigger').click();await page.locator('#search-input').fill('снапшот');
 await page.locator('.search-result').first().click();await expect(page.locator('.article-header h1')).toBeVisible();
 await page.locator('#chapter-complete').check();await expect(page.locator('#chapter-status')).toHaveText('Изучено');
 await page.locator('#save-chapter').click();await page.goto('/#saved');await expect(page.locator('.course-card')).toHaveCount(1);
 await page.reload();await expect(page.locator('.course-card')).toHaveCount(1);expect(errors).toEqual([]);
});
test('Hardware edit, VM roles, simulation, scenario and personalized plan',async({page})=>{
 await page.goto('/#lab');await page.locator('#host-ram').fill('256');await expect(page.locator('#hardware-summary')).toContainText('256 GiB');
 await page.getByRole('tab',{name:'Виртуальные машины'}).click();await expect(page.locator('[data-vm-row]')).toHaveCount(7);
 await page.locator('[data-key="mode"]').first().selectOption('split');
 await page.getByRole('button',{name:'Запустить моделирование'}).click();await expect(page.locator('#simulation-toggle')).toContainText('Пауза');
 await page.locator('#scenario-select').selectOption('backup');await expect(page.locator('.results-metrics')).toBeVisible();
 await page.getByRole('button',{name:'Получить план настройки'}).click();await expect(page.getByRole('heading',{name:'Пошаговый план настройки'})).toBeVisible();await expect(page.locator('.plan-vm')).toHaveCount(7);await expect(page.locator('.plan-vm').first()).toContainText('NIC 2');await expect(page.locator('.plan-vm').first()).toContainText('ncpa.cpl');
 const download=page.waitForEvent('download');await page.getByRole('button',{name:'Скачать план .md'}).click();const d=await download;expect(d.suggestedFilename()).toBe('esxi-atlas-plan.md');
 const file=await d.path();expect(fs.readFileSync(file,'utf8')).toContain('VLAN 200');
});
test('Invalid hardware is rejected and cannot run',async({page})=>{
 await page.goto('/#lab');await page.locator('#host-cores').fill('99');await expect(page.locator('.field.invalid')).toHaveCount(1);
 await page.getByRole('tab',{name:'Виртуальные машины'}).click();await page.getByRole('button',{name:'Запустить моделирование'}).click();await expect(page.locator('#toast')).toContainText('Исправьте параметры');
 await expect(page.locator('.error-box')).toContainText('допустимо 10–50');
});
test('Project export/import, command copy and explicit share dialog',async({page,context})=>{
 await context.grantPermissions(['clipboard-read','clipboard-write']);await page.goto('/#lab');
 const download=page.waitForEvent('download');await page.getByRole('button',{name:'Сохранить проект'}).click();const d=await download;const file=await d.path();const data=JSON.parse(fs.readFileSync(file,'utf8'));expect(data.config.vms).toHaveLength(7);
 data.config.host.ram=512;await page.locator('#config-file-input').setInputFiles({name:'project.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(data))});await expect(page.locator('#host-ram')).toHaveValue('512');
 await page.goto('/#commands');await page.getByRole('button',{name:'Копировать команду Версия и модель сервера',exact:true}).click();await expect(page.locator('#toast')).toContainText('скопирована');
 expect(await page.evaluate(()=>navigator.clipboard.readText())).toContain('vmware -vl');
 await page.getByRole('button',{name:'Поделиться командой Версия и модель сервера',exact:true}).click();await expect(page.locator('#share-text')).toContainText('vmware -vl');await expect(page.getByRole('button',{name:'Telegram',exact:true})).toBeVisible();await page.getByRole('button',{name:'Отмена',exact:true}).click();
});
test('All real screenshots load, local image import is labelled',async({page})=>{
 await page.goto('/#gallery');await expect(page.locator('.screenshot-card')).toHaveCount(26);
 const images=page.locator('.screenshot-card img');await images.evaluateAll(imgs=>imgs.forEach(img=>img.loading='eager'));
 await expect.poll(()=>images.evaluateAll(imgs=>imgs.filter(img=>!img.complete||img.naturalWidth===0).map(img=>img.alt))).toEqual([]);
 await page.locator('[data-import-shot]').first().click();
 // File chooser receives an actual bundled screenshot, not a fabricated UI image.
 await page.locator('#screenshot-input').setInputFiles('app/assets/screenshots/esxi-boot.png');await expect(page.locator('.screenshot-card').first()).toContainText('Ваш снимок');
});
test('Responsive page has no horizontal overflow and mobile menu works',async({page,isMobile})=>{
 await page.goto('/');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
 if(isMobile){await page.locator('#menu-button').click();await expect(page.locator('#sidebar')).toHaveClass(/open/);await page.locator('.main-nav a[href="#course"]').click();await expect(page.locator('#sidebar')).not.toHaveClass(/open/);}
 await page.goto('/#chapter/wan');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
});
test('Every chapter opens and source images exist',async({page})=>{
 const C=require('../../shared/course.cjs');const errors=[];page.on('pageerror',e=>errors.push(e.message));
 for(const c of C.chapters){await page.goto('/#chapter/'+c.id);await expect(page.locator('.article-header h1')).toHaveText(c.title);expect(await page.locator('.article-block').textContent()).not.toContain('undefined');}
 expect(errors).toEqual([]);
});
test('Local project persists across navigation and reload',async({page,context})=>{
 await page.goto('/#lab');await page.locator('#host-ram').fill('64');await context.setOffline(true);
 await page.goto('/#commands').catch(()=>{});await context.setOffline(false);await page.goto('/#lab');await expect(page.locator('#host-ram')).toHaveValue('64');
});
test('Font size can be enlarged, persists and stays usable on mobile',async({page,isMobile})=>{
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/#lab');
 await page.locator('#font-size-button').click();
 await expect(page.locator('#app-dialog')).toContainText('Размер шрифта');
 await page.locator('#app-dialog [data-font-scale="150"]').click();
 await expect(page.locator('html')).toHaveAttribute('data-font-scale','150');
 await page.locator('#app-dialog [data-close-dialog]').first().click();
 const size=await page.locator('.check-line').first().evaluate(el=>parseFloat(getComputedStyle(el).fontSize));
 expect(size).toBeGreaterThanOrEqual(16);
 await page.reload();await expect(page.locator('html')).toHaveAttribute('data-font-scale','150');
 await page.goto('/#settings');await expect(page.locator('[data-font-range]')).toHaveValue('150');
 await page.locator('[data-font-scale="200"]').click();await expect(page.locator('html')).toHaveAttribute('data-font-scale','200');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
 await page.goto('/#chapter/wan');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
 expect(errors).toEqual([]);
});

test('Step-by-step plan follows selected VMs and opens sourced real screenshots',async({page,context})=>{
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await context.grantPermissions(['clipboard-read','clipboard-write']);
 await page.goto('/#lab');await page.getByRole('tab',{name:'Виртуальные машины'}).click();
 const rows=page.locator('[data-vm-row]');
 await rows.nth(0).locator('[data-key="mode"]').selectOption('split');
 await rows.nth(0).locator('[data-key="cpu"]').fill('5');
 await rows.nth(1).locator('[data-key="mode"]').selectOption('isolated');
 await rows.nth(1).locator('[data-key="os"]').selectOption('linux');
 await page.getByRole('tab',{name:'План настройки'}).click();
 await expect(page.getByRole('heading',{name:'Пошаговый план настройки'})).toBeVisible();
 const first=page.locator('#plan-vm-0');await expect(first).toHaveAttribute('open','');
 await expect(first).toContainText('5 vCPU');await expect(first).toContainText('PG-WAN-200');
 await expect(first).toContainText('10.200.0.10');await expect(first).toContainText('не задавать');
 await expect(first).toContainText('Создайте виртуальную машину');await expect(first).toContainText('Установите гостевую ОС');
 const pic=first.locator('[data-image="win2022-setup-edition.png"]');await expect(pic).toBeVisible();
 await pic.click();await expect(page.locator('#app-dialog img')).toHaveAttribute('src',/win2022-setup-edition/);
 await expect(page.locator('#app-dialog')).toContainText('Windows Server 2022');await page.locator('#app-dialog [data-close-dialog]').click();
 await first.locator('[data-import-shot="win2022-setup-edition.png"]').click();
 await page.locator('#screenshot-input').setInputFiles('app/assets/screenshots/esxi-boot.png');
 await expect(page.locator('#plan-vm-0').locator('figure').filter({hasText:'Windows Server 2022: выбор Desktop Experience'})).toContainText('Ваш снимок');
 await page.reload();await expect(page.locator('#plan-vm-0')).toContainText('5 vCPU');
 await expect(page.locator('#plan-vm-0').locator('[data-image="win2022-setup-edition.png"] img')).toHaveAttribute('src',/^data:image/);
 await page.locator('[data-plan-vm="1"]').click();await expect(page.locator('#plan-vm-1')).toHaveAttribute('open','');
 await expect(page.locator('#plan-vm-1')).toContainText('Ubuntu Server 22.04');
 await expect(page.locator('#plan-vm-1')).toContainText('PG-LAN-101');
 await expect(page.locator('#plan-vm-1')).not.toContainText('PG-WAN-201');
 await expect(page).toHaveURL(/#lab$/);
 const code=page.locator('#plan-vm-1 .code-block').last();await code.locator('.copy-code').click();
 expect(await page.evaluate(()=>navigator.clipboard.readText())).toContain('ip -br link');
 await page.locator('#plan-vm-1 [data-share-plan-vm]').click();await expect(page.locator('#app-dialog')).toContainText('Отправить коллеге');
 const single=await page.locator('#share-text').inputValue();expect(single).toContain('SQL-01');expect(single).not.toContain('### 5.2. DC-00');
 await page.locator('#app-dialog [data-close-dialog]').first().click();
 const event=page.waitForEvent('download');await page.getByRole('button',{name:'Скачать план .md'}).click();
 const d=await event,md=fs.readFileSync(await d.path(),'utf8');
 expect(md).toContain('5 vCPU');expect(md).toContain('PG-WAN-200');expect(md).toContain('Источники реальных снимков');
 expect(md).toContain('Ubuntu Server 22.04');expect(md).not.toContain('sudo netplan apply');
 expect(errors).toEqual([]);
});
test('Plan remains responsive at 200% font and gallery opens the plan',async({page})=>{
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/#settings');await page.locator('[data-font-scale="200"]').click();
 await page.goto('/#gallery');await page.locator('.screenshot-card').filter({hasText:'ESXi 8 Host Client: имя'}).getByText('К плану').click();
 await expect(page.getByRole('heading',{name:'Пошаговый план настройки'})).toBeVisible();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
 await page.locator('[data-plan-vm="1"]').click();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
 expect(errors).toEqual([]);
});

test('Large plan covers all forty VMs without page overflow',async({page})=>{
 const M=require('../../shared/model.cjs'),config=M.defaultConfig();
 config.vms=Array.from({length:40},(_,i)=>({...M.newVM('TP',i),mode:'split'}));
 await page.addInitScript(c=>{localStorage.setItem('atlas:config-v1',JSON.stringify(c));localStorage.setItem('atlas:lab-tab',JSON.stringify('plan'));},config);
 await page.goto('/#lab');await expect(page.locator('.plan-vm')).toHaveCount(40);
 await page.locator('[data-plan-vm="39"]').click();await expect(page.locator('#plan-vm-39')).toContainText('PG-WAN-239');
 await expect(page.locator('#plan-vm-39')).toContainText('10.239.0.49');
 await page.getByRole('button',{name:'Поделиться',exact:true}).click();await expect(page.locator('#toast')).toContainText('слишком длинный');
 await page.locator('#plan-vm-39 [data-share-plan-vm]').click();await expect(page.locator('#share-text')).toHaveValue(/TP-39/);
 await page.locator('#app-dialog [data-close-dialog]').first().click();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
});

test('Android export delegates plan and project to native save picker',async({page})=>{
 await page.addInitScript(()=>{
  localStorage.setItem('atlas:lab-tab',JSON.stringify('plan'));
  localStorage.setItem('atlas:prefs',JSON.stringify({autoUpdate:false}));
  window.AndroidBridge={saveTextFile:(name,text)=>{window.savedTextFile={name,text};return true;},copyText:()=>true,shareText:()=>{},openExternal:()=>{},checkUpdates:()=>{}};
 });
 await page.goto('/#lab');await page.getByRole('button',{name:'Скачать план .md'}).click();
 let saved=await page.evaluate(()=>window.savedTextFile);expect(saved.name).toBe('esxi-atlas-plan.md');expect(saved.text).toContain('Пошаговый план');
 await expect(page.locator('#toast')).toContainText('Выберите место сохранения плана');
 await page.getByRole('button',{name:'Сохранить проект'}).click();saved=await page.evaluate(()=>window.savedTextFile);
 expect(saved.name).toBe('esxi-atlas-project.json');expect(JSON.parse(saved.text).config.vms).toHaveLength(7);
});
