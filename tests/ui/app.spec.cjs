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
 await page.getByRole('button',{name:'Получить план настройки'}).click();await expect(page.locator('.plan-step')).toHaveCount(6);await expect(page.locator('.network-detail').first()).toContainText('NIC 2');
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
 await page.goto('/#gallery');await expect(page.locator('.screenshot-card')).toHaveCount(11);
 await page.locator('.screenshot-card').first().locator('img').scrollIntoViewIfNeeded();
 expect(await page.locator('.screenshot-card').first().locator('img').evaluate(img=>img.complete&&img.naturalWidth>0)).toBeTruthy();
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
