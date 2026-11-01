(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const state = { cert: null, certAddress: '', host: null, bundle: null, report: null, busy: false, scale: 1 };
  const hide = (id, value) => { $(id).hidden = value; };
  const set = (id, text) => { $(id).textContent = text || '—'; };
  function toast(text) { $('toast').textContent = String(text); $('toast').hidden = false; clearTimeout(toast.timer); toast.timer = setTimeout(() => { $('toast').hidden = true; }, 6000); }
  function message(err) { return typeof err === 'string' ? err : (err && err.message) || 'Не удалось выполнить действие'; }
  function el(tag, cls, text) { const x = document.createElement(tag); if (cls) x.className = cls; if (text !== undefined) x.textContent = text; return x; }
  function normalizePin(s) { return s.replace(/[:\s-]/g, '').toUpperCase(); }
  async function copy(text) {
    try { if (window.goClipboard) { await window.goClipboard(text); } else { await navigator.clipboard.writeText(text); } toast('Скопировано'); }
    catch (_) { toast('Не удалось скопировать. Выделите текст вручную.'); }
  }
  async function job(start, label) {
    if (state.busy) return null;
    state.busy = true; $('busyText').textContent = label; hide('busy', false);
    try {
      const id = await start();
      if (!id) return null;
      for (;;) {
        const result = await window.goPoll(id);
        if (result.state === 'done') return result.data;
        if (result.state === 'error') throw new Error(result.error || 'Ошибка');
        await new Promise(resolve => setTimeout(resolve, 300));
      }
    } finally { state.busy = false; hide('busy', true); }
  }
  function navigate(id) {
    document.querySelectorAll('[data-nav]').forEach(b => b.classList.toggle('active', b.dataset.nav === id));
    $(id).scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
  document.querySelectorAll('[data-nav]').forEach(b => b.addEventListener('click', () => navigate(b.dataset.nav)));
  document.querySelectorAll('[data-link]').forEach(b => b.addEventListener('click', async () => { try { await window.goOpenLink(b.dataset.link); } catch (e) { toast(message(e)); } }));
  $('showPassword').addEventListener('click', () => { const input = $('password'); input.type = input.type === 'password' ? 'text' : 'password'; $('showPassword').setAttribute('aria-label', input.type === 'password' ? 'Показать пароль' : 'Скрыть пароль'); });
  function fontScale(step) { state.scale = Math.max(.9, Math.min(1.7, Math.round((state.scale + step) * 10) / 10)); document.documentElement.style.setProperty('--scale', state.scale); try { localStorage.setItem('raid-assistant-font', String(state.scale)); } catch (_) {} }
  try { const n = Number(localStorage.getItem('raid-assistant-font')); if (n >= .9 && n <= 1.7) state.scale = n; } catch (_) {}
  fontScale(0);
  $('fontPlus').addEventListener('click', () => fontScale(.1)); $('fontMinus').addEventListener('click', () => fontScale(-.1));
  $('copyCert').addEventListener('click', () => { if (state.cert) copy(state.cert.sha256); });
  $('address').addEventListener('input', () => { if (state.certAddress !== $('address').value.trim()) { state.cert = null; hide('certPanel', true); } });
  $('connectionForm').addEventListener('submit', async event => {
    event.preventDefault();
    const address = $('address').value.trim();
    try {
      state.cert = null; hide('certPanel', true);
      const cert = await job(() => window.goCertStart(address), 'Получаем сертификат ESXi…');
      if (!cert) return;
      state.cert = cert; state.certAddress = address;
      set('certFingerprint', cert.sha256); $('confirmedPin').value = '';
      const expiry = new Date(cert.notAfter);
      $('certExpiry').textContent = Number.isNaN(expiry.getTime()) ? '' : `Действует до ${expiry.toLocaleDateString('ru-RU')}`;
      $('certTitle').textContent = cert.trusted ? 'Сертификат доверен системой' : 'Нужно проверить сертификат';
      $('certMessage').textContent = cert.trusted ? 'Системная проверка сертификата и имени хоста прошла. Теперь можно отправить учётные данные по TLS.' : 'Центр сертификации или имя не подтверждены. Сверьте отпечаток через независимый доверенный канал.';
      hide('pinArea', !!cert.trusted);
      $('scanButton').disabled = !cert.validTime;
      if (!cert.validTime) $('certMessage').textContent = 'Срок действия сертификата истёк или ещё не начался. Замените сертификат и проверьте время на хосте.';
      hide('certPanel', false);
    } catch (e) { toast(message(e)); }
  });
  $('scanButton').addEventListener('click', async () => {
    if (!state.cert || state.certAddress !== $('address').value.trim()) { toast('Сначала заново проверьте сертификат'); return; }
    if (!$('username').value.trim() || !$('password').value) { toast('Введите логин и пароль ESXi'); return; }
    let pin = '';
    if (!state.cert.trusted) {
      pin = normalizePin($('confirmedPin').value);
      if (pin.length !== 64 || pin !== normalizePin(state.cert.sha256)) { toast('Введите полный отпечаток, независимо проверенный на самом сервере'); return; }
    }
    const request = { address: state.certAddress, user: $('username').value.trim(), password: $('password').value, pin };
    try {
      state.host = null; state.bundle = null; state.report = null;
      hide('diagnosticEmpty', false); hide('diagnosticResult', true);
      hide('packageEmpty', false); hide('packageResult', true);
      hide('reportEmpty', false); hide('reportResult', true);
      const promise = job(() => window.goScanStart(request), 'Проверяем ESXi и контроллеры…');
      $('password').value = '';
      const host = await promise;
      if (!host) return;
      state.host = host; state.bundle = null;
      renderHost(host); renderPackage(host); populateDatastores(host); await renderReport(); navigate('diagnostics');
    } catch (e) { toast(message(e)); }
    finally { $('password').value = ''; request.password = ''; }
  });
  function renderHost(host) {
    hide('diagnosticEmpty', true); hide('diagnosticResult', false);
    set('hostName', host.name || host.displayHost); set('hostAddress', host.address); set('hostVersion', `${host.version || 'Неизвестно'} · ${host.build || 'build неизвестен'}`); set('hostFullName', host.fullName || 'Версия по ESXi API'); set('hostModel', `${host.vendor || ''} ${host.model || ''}`.trim());
    const controllers = host.controllers || [];
    set('adapterCount', `${controllers.length} адаптер${controllers.length === 1 ? '' : 'ов'}`);
    const rows = $('controllerRows'); rows.replaceChildren();
    if (!controllers.length) { const tr = el('tr'); const td = el('td', '', 'Адаптеры не получены. Проверьте права доступа и PCI ID вручную.'); td.colSpan = 4; tr.append(td); rows.append(tr); }
    controllers.forEach(c => {
      const tr = el('tr'); const model = el('td'); model.append(el('strong', '', c.catalogModel || c.model || 'Неизвестный адаптер'), el('small', '', `${c.device || 'HBA нет'} · ${c.status || 'статус неизвестен'}`));
      const driver = el('td', '', c.driver || 'Нет данных'); const ids = el('td'); ids.append(el('strong', '', c.pci || 'PCI неизвестен'), el('small', '', c.vendor ? `${c.vendor}:${c.deviceId} / ${c.subVendor}:${c.subDevice}` : 'PCI ID не получены'));
      const match = el('td'); match.append(el('span', `tag ${c.match === 'catalog' ? 'tag-blue' : 'tag-amber'}`, c.match === 'catalog' ? 'СОВПАДЕНИЕ' : c.match === 'oem' ? 'OEM: ПРОВЕРИТЬ' : 'НЕТ ДАННЫХ'));
      tr.append(model, driver, ids, match); rows.append(tr);
    });
    const cards = $('checks'); cards.replaceChildren();
    (host.checks || []).forEach(check => { const card = el('div', 'check-card'), label = el('div', 'check-label', check.label), val = el('div', 'check-value'); val.append(el('span', `status-dot ${['ok','warn','unknown','info'].includes(check.status) ? check.status : 'unknown'}`), el('span', '', check.value)); card.append(label, val, el('p', '', check.detail || '')); cards.append(card); });
    const warnings = host.warnings || []; hide('hostWarnings', warnings.length === 0); $('hostWarnings').textContent = warnings.join(' ');
  }
  function renderPackage(host) {
    hide('packageEmpty', true); hide('packageResult', false); $('consent').checked = false; $('downloadButton').disabled = true; hide('bundleInfo', true);
    if (!host.candidate) { set('noMatchReason', (host.candidateReason || 'Для этого ESXi или полного PCI ID в каталоге нет точного совпадения.') + ' LWM не заменяет LSA для управления.'); hide('packageNoMatch', false); hide('packageMatch', true); return; }
    hide('packageNoMatch', true); hide('packageMatch', false);
    set('packageTitle', 'LSA для ESXi 8'); set('packageSubtitle', 'Light Weight Agent для доступа к управлению через ESXi'); set('packageModel', host.candidate.model); set('packageVersion', host.candidate.version);
  }
  $('consent').addEventListener('change', () => { $('downloadButton').disabled = !$('consent').checked; });
  function renderBundle(data) {
    state.bundle = data;
    const box = $('bundleInfo'); box.replaceChildren();
    box.append(el('strong', '', 'Файл подготовлен'), el('p', '', `Архив: ${data.archiveName}`));
    const hash = el('p'); hash.append(el('span', '', 'SHA-256 архива: '), el('code', '', data.sha256)); box.append(hash);
    box.append(el('p', '', `VIB: ${data.vibName || 'не найден'}`), el('p', '', `Папка / файл: ${data.vibPath || data.archivePath}`), el('p', '', data.notice));
    hide('bundleInfo', false);
  }
  $('downloadButton').addEventListener('click', async () => {
    try { const data = await job(() => window.goDownloadStart($('consent').checked), 'Скачиваем и проверяем пакет…'); if (data) { renderBundle(data); await renderReport(); toast('Пакет скачан и подготовлен'); } }
    catch (e) { toast(message(e)); }
  });
  $('importButton').addEventListener('click', async () => {
    try { const data = await job(() => window.goImportStart(), 'Проверяем выбранный файл…'); if (data) { renderBundle(data); await renderReport(); toast('Файл обработан'); } }
    catch (e) { toast(message(e)); }
  });
  function populateDatastores(host) {
    const select = $('datastore'); select.replaceChildren(); const placeholder = el('option', '', 'Выберите доступный datastore'); placeholder.value = ''; select.append(placeholder);
    (host.datastores || []).filter(d => d.accessible).forEach(d => { const o = el('option', '', `${d.name} · ${d.type || 'тип неизвестен'} · свободно ${(d.free / 1073741824).toFixed(1)} ГиБ`); o.value = d.name; select.append(o); });
  }
  function options() { return { datastore: $('datastore').value, accessIp: $('accessIp').value.trim() }; }
  async function renderReport() {
    if (!state.host) return;
    const r = await window.goReport(options()); state.report = r;
    hide('reportEmpty', true); hide('reportResult', false);
    const steps = $('steps'); steps.replaceChildren();
    (r.steps || []).forEach((s, index) => {
      const item = el('article', 'step'), num = el('div', 'step-num', String(index + 1).padStart(2, '0')), body = el('div');
      body.append(el('h3', '', s.title), el('p', '', s.body));
      (s.commands || []).forEach(command => { const block = el('div', 'command'), code = el('code', '', command), button = el('button', '', 'Копировать'); button.type = 'button'; button.addEventListener('click', () => copy(command)); block.append(code, button); body.append(block); });
      item.append(num, body); steps.append(item);
    });
  }
  $('refreshReport').addEventListener('click', () => renderReport().then(() => toast('Инструкция обновлена')).catch(e => toast(message(e))));
  $('datastore').addEventListener('change', () => renderReport().catch(e => toast(message(e))));
  $('copyReport').addEventListener('click', () => renderReport().then(() => copy(state.report.markdown)).catch(e => toast(message(e))));
  $('saveReport').addEventListener('click', async () => { try { const file = await window.goSave(options()); toast(`Инструкция сохранена: ${file}`); } catch (e) { toast(message(e)); } });
  $('openFolder').addEventListener('click', async () => { try { await window.goOpenFolder(); } catch (e) { toast(message(e)); } });
  if (window.RAID_DEMO) hide('demoBanner', false);
})();
