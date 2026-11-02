(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const state = { certAddress: '', connecting: false, host: null, bundle: null, report: null, snapshot: null, sshKey: null, components: [], profiles: [], monitorSeq: 0, alertSeq: 0, busy: false, scale: 1 };
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
  // The native bridge should respond immediately. A stuck RPC must not leave
  // the user staring at a spinner forever; actual network jobs have their own
  // longer deadline and are polled separately.
  async function rpc(call, timeoutMs = 12000) {
    let timer;
    try {
      return await Promise.race([
        Promise.resolve().then(call),
        new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Нет ответа от модуля программы. Перезапустите приложение и повторите действие.')), timeoutMs); })
      ]);
    } finally { clearTimeout(timer); }
  }
  async function job(start, label, timeoutMs = 360000, startTimeoutMs = 0) {
    if (state.busy) return null;
    state.busy = true; $('busyText').textContent = label; hide('busy', false);
    try {
      // Native file choosers legitimately wait for the user; only connection
      // RPC starts get a short bridge timeout. Network job time starts after
      // the chooser/start call returns.
      const id = startTimeoutMs ? await rpc(start, startTimeoutMs) : await start();
      if (!id) return null;
      const deadline = Date.now() + timeoutMs;
      while (Date.now() < deadline) {
        const result = await rpc(() => window.goPoll(id));
        if (!result || !['running', 'done', 'error'].includes(result.state)) throw new Error('Модуль программы вернул некорректный ответ. Повторите попытку.');
        if (result.state === 'done') return result.data;
        if (result.state === 'error') throw new Error(result.error || 'Ошибка');
        await new Promise(resolve => setTimeout(resolve, 300));
      }
      throw new Error('Время ожидания истекло. Проверьте доступность ESXi и попробуйте ещё раз.');
    } finally { state.busy = false; hide('busy', true); }
  }
  function connectionStatus(text, kind = '', reveal = false) {
    const box = $('connectionStatus'); box.textContent = text; box.className = `connection-status ${kind}`.trim();
    if (reveal) box.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }
  function sshStatus(text, kind = '') {
    const box = $('sshStatus'); box.textContent = text; box.className = `connection-status ${kind}`.trim();
  }
  function confirmUnverified(reason, unverified) {
    return new Promise(resolve => {
      const dialog = $('securityDialog'), cancel = $('securityCancel'), proceed = $('securityContinue');
      $('securityDialogTitle').textContent = unverified ? 'Сервер не подтверждён' : 'Проверьте срок сертификата';
      $('securityDialogMessage').textContent = reason;
      const previous = document.activeElement;
      const finish = accepted => {
        hide('securityDialog', true); cancel.removeEventListener('click', no); proceed.removeEventListener('click', yes); document.removeEventListener('keydown', key);
        if (previous && previous.focus) previous.focus(); resolve(accepted);
      };
      const no = () => finish(false), yes = () => finish(true), key = event => {
        if (event.key === 'Escape') { event.preventDefault(); finish(false); }
        else if (event.key === 'Tab') { event.preventDefault(); (document.activeElement === cancel ? proceed : cancel).focus(); }
      };
      cancel.addEventListener('click', no); proceed.addEventListener('click', yes); document.addEventListener('keydown', key);
      hide('securityDialog', false); cancel.focus();
    });
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
  function resetResults() {
    state.host = null; state.bundle = null; state.report = null; state.snapshot = null; state.sshKey = null; state.monitorSeq = 0; state.alertSeq = 0;
    $('sshPassword').value = ''; $('sshPin').value = ''; $('sshConsent').checked = false; sshStatus('Для чтения проверьте ключ и заполните поля выше.'); hide('sshKeyPanel', true); hide('raidResult', true); hide('diagnosticWizard', true); $('relationsResult').replaceChildren();
    hide('diagnosticEmpty', false); hide('diagnosticResult', true);
    hide('packageEmpty', false); hide('packageResult', true);
    hide('reportEmpty', false); hide('reportResult', true);
  }
  $('address').addEventListener('input', () => {
    if (state.certAddress && state.certAddress !== $('address').value.trim()) {
      state.certAddress = ''; $('confirmedPin').value = '';
      // Do not leave a previous host's package or commands next to a new IP.
      if (state.host) { resetResults(); if (window.goInvalidate) rpc(() => window.goInvalidate()).then(() => { refreshMonitor(); loadComponents(); }).catch(() => {}); }
      connectionStatus('Адрес изменён. Введите пароль и подключитесь к новому хосту.');
    }
  });
  $('connectionForm').addEventListener('submit', async event => {
    event.preventDefault();
    if (state.connecting || state.busy) return;
    const address = $('address').value.trim(), user = $('username').value.trim();
    let password = $('password').value;
    if (!address || !user || !password) {
      connectionStatus('Введите IP-адрес ESXi, логин и пароль.', 'error', true);
      return;
    }
    const manualPin = normalizePin($('confirmedPin').value);
    if (manualPin && !/^[A-F0-9]{64}$/.test(manualPin)) {
      connectionStatus('SHA-256 должен содержать 64 шестнадцатеричных символа, либо оставьте поле пустым.', 'error', true);
      return;
    }
    state.connecting = true; $('connectButton').disabled = true;
    let request = null, scanStarted = false;
    try {
      if (typeof window.goCertStart !== 'function' || typeof window.goScanStart !== 'function' || typeof window.goPoll !== 'function' || typeof window.goInvalidate !== 'function') {
        throw new Error('Модуль подключения недоступен. Перезапустите приложение и попробуйте ещё раз.');
      }
      resetResults();
      connectionStatus('Подключаемся к ESXi: проверяем адрес и защищённое соединение…');
      // Clear even a previous connection to the same host before a new attempt.
      await rpc(() => window.goInvalidate());
      const cert = await job(() => window.goCertStart(address), 'Подключаемся к ESXi…', 20000, 12000);
      if (!cert || typeof cert.sha256 !== 'string' || !/^[A-F0-9]{64}$/.test(normalizePin(cert.sha256)) || typeof cert.trusted !== 'boolean' || typeof cert.validTime !== 'boolean') {
        throw new Error('Не удалось получить данные защищённого соединения. Проверьте адрес ESXi.');
      }
      if (address !== $('address').value.trim()) throw new Error('Адрес изменился во время подключения. Попробуйте ещё раз.');
      const observedPin = normalizePin(cert.sha256);
      if (manualPin && manualPin !== observedPin) throw new Error('SHA-256 не совпадает с проверенным вами отпечатком. Пароль не отправлен.');
      const unverified = !cert.trusted && !manualPin;
      if (unverified || !cert.validTime) {
        const reason = [unverified ? 'Подлинность сервера не подтверждена. У ESXi часто используется самоподписанный сертификат.' : '',
          !cert.validTime ? 'Срок действия сертификата истёк или ещё не начался. Проверьте часы компьютера и сервера.' : ''].filter(Boolean).join('\n\n');
        connectionStatus('Ожидаем вашего решения: продолжить или отменить подключение.', 'warning');
        const accepted = await confirmUnverified(reason, unverified);
        if (!accepted) {
          $('password').value = '';
          connectionStatus('Подключение отменено. Пароль не отправлен.', 'warning', true);
          return;
        }
      }
      if (address !== $('address').value.trim()) throw new Error('Адрес изменился во время подключения. Попробуйте ещё раз.');
      // Observed pins prevent the certificate changing between preflight and
      // login, but without an independent check they do not authenticate it.
      const pin = manualPin || ((!cert.trusted || !cert.validTime) ? observedPin : '');
      request = { address, user, password, pin, allowExpired: !cert.validTime };
      connectionStatus('Входим в ESXi и читаем данные хоста…');
      scanStarted = true;
      const pending = job(() => {
        // The bridge serializes arguments synchronously before returning.
        try { return window.goScanStart(request); }
        finally { $('password').value = ''; request.password = ''; password = ''; }
      }, 'Входим в ESXi и читаем данные хоста…', 90000, 12000);
      const host = await pending;
      if (!host) throw new Error('Подключение не завершено. Попробуйте ещё раз.');
      if (address !== $('address').value.trim()) throw new Error('Адрес изменился во время подключения. Попробуйте ещё раз.');
      state.certAddress = address; state.host = host; state.bundle = null;
      renderHost(host); renderPackage(host); populateDatastores(host);
      const warning = unverified ? 'Подлинность этого сервера не подтверждена. Используйте только доверенную сеть.' : (!cert.validTime ? 'Подключение выполнено с истёкшим или ещё не действующим сертификатом.' : '');
      $('connectionWarning').textContent = warning; hide('connectionWarning', !warning);
      connectionStatus(warning ? 'Подключено. ' + warning : 'Подключено. Данные ESXi получены.', warning ? 'warning' : 'success');
      $('sshUser').value = user; navigate('diagnostics');
      // The host result is already visible even if an auxiliary panel fails.
      try { await rpc(() => Promise.all([renderReport(), renderDiagnostics(), loadComponents(), loadHistory()])); }
      catch (_) { toast('Подключение выполнено, но не все дополнительные данные удалось загрузить.'); }
    } catch (e) {
      if (scanStarted && window.goInvalidate) rpc(() => window.goInvalidate()).catch(() => {});
      connectionStatus(message(e), 'error', true);
    } finally {
      $('password').value = ''; password = ''; if (request) request.password = '';
      state.connecting = false; $('connectButton').disabled = false;
    }
  });
  function renderHost(host) {
    hide('diagnosticEmpty', true); hide('diagnosticResult', false);
    set('hostName', host.name || host.displayHost); set('hostAddress', host.address); set('hostVersion', `${host.version || 'Неизвестно'} · ${host.build || 'build неизвестен'}`); set('hostFullName', host.fullName || 'Версия по ESXi API'); set('hostModel', `${host.vendor || ''} ${host.model || ''}`.trim());
    const controllers = host.controllers || [];
    set('adapterCount', `${controllers.length} адаптер${controllers.length === 1 ? '' : 'ов'}`);
    const rows = $('controllerRows'); rows.replaceChildren();
    if (!controllers.length) { const tr = el('tr'); const td = el('td', '', 'Адаптеры не получены. Проверьте права доступа и PCI ID вручную.'); td.colSpan = 4; tr.append(td); rows.append(tr); }
    controllers.forEach(c => {
      const tr = el('tr'); const model = el('td');
      const apiStatus = !c.status || c.status.toLowerCase() === 'unknown' ? 'статус API неизвестен' : `статус API: ${c.status}`;
      model.append(el('strong', '', c.catalogModel || c.model || 'Неизвестный адаптер'), el('small', '', `${c.device || 'HBA нет'} · ${apiStatus}`));
      if (c.match === 'identified') model.append(el('small', '', 'Модель по PCI ID; здоровье RAID не проверено'));
      if (c.oem) model.append(el('small', '', `${c.oem}: ${c.oemReason || 'сверьте производителя'}`));
      const driver = el('td', '', c.driver || 'Нет данных'); const ids = el('td'); ids.append(el('strong', '', c.pci || 'PCI неизвестен'), el('small', '', c.vendor ? `${c.vendor}:${c.deviceId} / ${c.subVendor}:${c.subDevice}` : 'PCI ID не получены'));
      const match = el('td'); match.append(el('span', `tag ${c.match === 'catalog' ? 'tag-blue' : 'tag-amber'}`, c.match === 'catalog' ? 'ПАКЕТ-КАНДИДАТ' : c.match === 'identified' ? 'МОДЕЛЬ ПО PCI' : c.match === 'oem' ? 'OEM: ПРОВЕРИТЬ' : 'НЕТ ДАННЫХ'));
      tr.append(model, driver, ids, match); rows.append(tr);
    });
    const cards = $('checks'); cards.replaceChildren();
    (host.checks || []).forEach(check => { const card = el('div', 'check-card'), label = el('div', 'check-label', check.label), val = el('div', 'check-value'); val.append(el('span', `status-dot ${['ok','warn','unknown','info'].includes(check.status) ? check.status : 'unknown'}`), el('span', '', check.value)); card.append(label, val, el('p', '', check.detail || '')); cards.append(card); });
    const warnings = host.warnings || []; hide('hostWarnings', warnings.length === 0); $('hostWarnings').textContent = warnings.join(' ');
  }
  function renderPackage(host) {
    hide('packageEmpty', true); hide('packageResult', false); $('consent').checked = false; $('downloadButton').disabled = true; hide('bundleInfo', true);
    const has9361 = (host.controllers || []).some(c => c.match === 'identified' && c.catalogModel === 'MegaRAID SAS 9361-8i');
    hide('help9361', !has9361);
    if (has9361) {
      // Presence of a VIB name alone proves neither the path nor compatibility.
      // A failed/partial API read must never be described as absent software.
      const utilities = (host.vibs || []).filter(v => ['storcli','vmware-storcli64','vmware-storcli'].includes((v.name || '').toLowerCase()));
      const inventory = !host.vibInventoryKnown ? 'Список VIB не получен: наличие StorCLI неизвестно.' : utilities.length ? `В списке VIB найдены: ${utilities.slice(0,4).map(v => `${v.name} ${v.version || ''}`.trim()).join(', ')}. Путь и совместимость не проверены.` : 'В прочитанном списке VIB имена storcli / vmware-storcli64 / vmware-storcli не найдены. Это не исключает другую сборку или ручную установку.';
      set('storcli9361Inventory', inventory);
    }
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

  // Host profiles contain address and optional login, never passwords or pins.
  async function loadProfiles() {
    try {
      state.profiles = await window.goProfiles() || [];
      const select = $('hostProfiles'); const selected = select.value; select.replaceChildren();
      const first = el('option', '', 'Выберите или введите новый адрес'); first.value = ''; select.append(first);
      state.profiles.forEach(p => { const o = el('option', '', `${p.name} · ${p.address}`); o.value = p.address; select.append(o); });
      if (state.profiles.some(p => p.address === selected)) select.value = selected;
    } catch (e) { toast(message(e)); }
  }
  $('hostProfiles').addEventListener('change', () => {
    const p = state.profiles.find(x => x.address === $('hostProfiles').value); if (!p) return;
    $('address').value = p.address; $('address').dispatchEvent(new Event('input')); $('username').value = p.user || ''; $('password').value = ''; $('profileName').value = p.name;
  });
  $('saveProfile').addEventListener('click', async () => {
    try { state.profiles = await window.goProfileSave({ name:$('profileName').value.trim(), address:$('address').value.trim(), user:$('username').value.trim() }); await loadProfiles(); toast('Хост сохранён без пароля'); }
    catch(e) { toast(message(e)); }
  });
  $('deleteProfile').addEventListener('click', async () => {
    const address = $('hostProfiles').value; if (!address) { toast('Выберите хост из списка'); return; }
    if (!confirm('Удалить выбранный хост из списка? История проверок останется.')) return;
    try { await window.goProfileDelete(address); await loadProfiles(); toast('Хост удалён из списка'); } catch(e) { toast(message(e)); }
  });

  async function renderDiagnostics() {
    if (!state.host) return;
    const list = await window.goDiagnostics(); const root = $('diagnosticSteps'); root.replaceChildren();
    list.forEach(d => { const row=el('article','diag-row'); row.append(el('h4','',d.title),el('p','',`Факт: ${d.fact}`),el('p','',`Что проверить: ${d.next}`));
      (d.commands||[]).forEach(command => { const block=el('div','command'), code=el('code','',command), btn=el('button','','Копировать');btn.type='button';btn.addEventListener('click',()=>copy(command));block.append(code,btn);row.append(block); });root.append(row); });
    hide('diagnosticWizard',false);
  }
  $('probePort').addEventListener('click', async () => {
    try { const data=await job(()=>window.goPortStart(),'Проверяем TCP и TLS без учётных данных…'); if(data){await renderDiagnostics();await loadHistory();toast('Проверка порта завершена');} }
    catch(e){toast(message(e));}
  });
  $('copy9361').addEventListener('click',()=>copy('/opt/storcli/bin/storcli64 show nolog'));
  $('read9361').addEventListener('click',()=>{ $('sshKind').value='storcli64'; $('sshDetails').open=true; navigate('raid'); });
  $('copyStorcli').addEventListener('click',()=>copy('/opt/lsi/storcli/storcli /call show all nolog J'));
  $('copyHpe').addEventListener('click',()=>copy('/opt/smartstorageadmin/ssacli/bin/ssacli ctrl all show config'));
  $('raidImport').addEventListener('click', async () => {
    try { const data=await job(()=>window.goRaidImportStart(),'Читаем выбранный отчёт…'); if(data){renderRaid(data);await loadHistory();toast('Отчёт загружен');} }
    catch(e){toast(message(e));}
  });
  $('sshKeyButton').addEventListener('click', async () => {
    try { state.sshKey=null;hide('sshKeyPanel',true);$('sshPin').value='';$('sshConsent').checked=false;sshStatus('Получаем SSH-ключ без отправки пароля…');
      const data=await job(()=>window.goSSHKeyStart(),'Получаем SSH host key без пароля…'); if(!data){sshStatus('Получение ключа не выполнено.');return;}state.sshKey=data;set('sshKeyAlgorithm',data.algorithm);set('sshKeyFingerprint',data.fingerprint);hide('sshKeyPanel',false);sshStatus('Ключ получен. Сверьте его независимо и заполните SSH-логин и пароль.');
    }catch(e){sshStatus(message(e),'error');toast(message(e));}
  });
  function sshOptions() {
    const pin=$('sshPin').value.trim();
    if(!state.host) throw new Error('Сначала проверьте хост ESXi');
    if(!state.sshKey || pin!==state.sshKey.fingerprint) throw new Error('Введите SSH host key, проверенный независимо');
    if(!$('sshConsent').checked) throw new Error('Для SSH нужно ваше явное разрешение');
    if(!$('sshUser').value.trim() || !$('sshPassword').value) throw new Error('Введите логин и пароль SSH');
    return {user:$('sshUser').value.trim(),password:$('sshPassword').value,fingerprint:pin,kind:$('sshKind').value,consent:true};
  }
  $('sshRead').addEventListener('click',async()=>{
    let request;try {request=sshOptions();sshStatus('Проверяем SSH-ключ и читаем RAID…');const pending=job(()=>window.goSSHReadStart(request),'Читаем RAID по SSH…');$('sshPassword').value='';const data=await pending;if(data){renderRaid(data);await loadHistory();sshStatus('Снимок RAID получен. Состояние и предупреждения показаны ниже.','success');toast('Снимок RAID получен');}else{sshStatus('Чтение RAID не выполнено.');}}
    catch(e){sshStatus(message(e),'error');toast(message(e));}finally{$('sshPassword').value='';if(request)request.password='';}
  });
  function severityLabel(value){return ({critical:'КРИТИЧНО',warning:'ВНИМАНИЕ',unknown:'НЕПОЛНЫЕ ДАННЫЕ',ok:'НАЙДЕНО',info:'ИНФОРМАЦИЯ'})[value]||'НЕИЗВЕСТНО';}
  function renderRaid(r){
    state.snapshot=r;hide('raidResult',false);$('reportHostConfirmed').checked=false;$('relationsResult').replaceChildren();
    const date=new Date(r.collectedAt);$('raidMeta').textContent=`${r.source} · ${r.format} · ${Number.isNaN(date.getTime())?'время неизвестно':date.toLocaleString('ru-RU')}`;
    const severity=$('raidSeverity');severity.textContent=severityLabel(r.severity);severity.className=`tag ${r.severity==='critical'||r.severity==='warning'?'tag-amber':''}`;
    $('raidWarnings').textContent=(r.warnings||[]).join(' ');
    const root=$('raidTables');root.replaceChildren();
    const groups=[['Контроллеры',r.controllers||[],['ID','Модель','Состояние'], x=>[x.id,x.model,x.state]],['Логические диски / RAID',r.volumes||[],['Контроллер / ID','Уровень · размер','Состояние / NAA'],x=>[`${x.controller} / ${x.id}`,`${x.level||'—'} · ${x.size||'—'}`,`${x.state||'нет данных'}${x.naa?' · '+x.naa:''}`]],['Физические диски',r.disks||[],['Контроллер / слот','Группа · модель','Состояние'],x=>[`${x.controller} / ${x.id}`,`${x.group||'—'} · ${x.model||'—'}`,x.state||'нет данных']]];
    groups.forEach(([title,items,head,values])=>{const section=el('div','result-table');section.append(el('h4','',`${title} · ${items.length}`));if(!items.length){section.append(el('p','subtle','В этом отчёте данные не распознаны. Это не означает, что объектов нет.'));root.append(section);return;}const wrap=el('div','table-wrap'),table=el('table'),thead=el('thead'),tr=el('tr'),body=el('tbody');head.forEach(x=>tr.append(el('th','',x)));thead.append(tr);table.append(thead);items.forEach(x=>{const row=el('tr');values(x).forEach(v=>row.append(el('td','',v||'—')));body.append(row)});table.append(body);wrap.append(table);section.append(wrap);root.append(section);});
  }
  $('correlate').addEventListener('click',async()=>{
    try {const result=await window.goCorrelate($('reportHostConfirmed').checked);const root=$('relationsResult');root.replaceChildren();root.append(el('p','',result.note));if(!(result.links||[]).length){root.append(el('strong','','Надёжных совпадений не найдено. Это не означает отсутствие связи.'));return;}result.links.forEach(link=>{const box=el('div','relation-item');box.append(el('strong','',`Контроллер ${link.controller} · RAID ${link.volume} → ${link.datastore}`),el('p','',`NAA: ${link.naa}`),el('p','',`Видимые ВМ с инвентарной связью: ${(link.vms||[]).join(', ')||'не получены / нет видимых'}`));root.append(box)});}
    catch(e){toast(message(e));}
  });

  async function loadComponents(){
    try {state.components=await window.goComponents()||[];renderComponents();}catch(e){$('componentSummary').textContent=message(e);}
  }
  function renderComponents(){
    const search=$('componentSearch').value.toLowerCase().trim(),filter=$('componentFilter').value;
    const list=state.components.filter(x=>(filter==='all'||x.presence===filter)&&`${x.name} ${x.category} ${x.purpose} ${x.requirements}`.toLowerCase().includes(search));
    $('componentSummary').textContent=`Показано: ${list.length} из ${state.components.length}. «Не найдено» относится только к перечисленным именам VIB, а не ко всей функции или версии.`;
    const root=$('componentGrid');root.replaceChildren();
    list.forEach(item=>{const card=el('article','extension-card'),tag=el('span','tag tag-blue',item.category);card.append(tag,el('h3','',item.name),el('p','',item.purpose));
      card.append(el('p','component-delivery',`Распространение: ${item.delivery}`),el('p','component-req',`Условия: ${item.requirements}`));
      const labels={unknown:'Наличие VIB неизвестно',found:'VIB найден в API', 'not-seen':'Указанные имена VIB не найдены в прочитанном списке','not-vib':'Не проверяется как отдельный VIB'};
      const found=(item.found||[]).map(x=>`${x.name} ${x.version||''}`).join(', ');card.append(el('p','component-found',`${labels[item.presence]||labels.unknown}${found?': '+found:''}`));
      const link=el('button','text-button','Официальный источник ↗');link.type='button';link.addEventListener('click',()=>window.goOpenLink('component:'+item.id).catch(e=>toast(message(e))));card.append(link);root.append(card);});
  }
  $('componentSearch').addEventListener('input',renderComponents);$('componentFilter').addEventListener('change',renderComponents);

  async function loadHistory(){try {const history=await window.goHistory()||[],root=$('historyList');root.replaceChildren();if(!history.length){root.append(el('p','subtle','История пуста.'));return;}history.forEach(e=>{const row=el('div','history-item'),date=new Date(e.at);row.append(el('time','',Number.isNaN(date.getTime())?'—':date.toLocaleString('ru-RU')));const body=el('div');body.append(el('strong','',`${e.host} · ${e.type}`),el('p','',`${severityLabel(e.severity)} · ${e.summary}`));row.append(body);root.append(row);});}catch(e){toast(message(e));}}
  $('clearHistory').addEventListener('click',async()=>{if(!confirm('Очистить локальную историю проверок?'))return;try{await window.goHistoryClear();await loadHistory();toast('История очищена');}catch(e){toast(message(e));}});
  async function refreshMonitor(){
    try{const m=await window.goMonitor();$('monitorStart').disabled=!!m.running;$('monitorStop').disabled=!m.running;
      const text=m.running?`${m.message||'Ожидаем результат'}${m.lastAt?' · '+new Date(m.lastAt).toLocaleString('ru-RU'):''}`:'Мониторинг остановлен.';
      $('monitorStatus').textContent=text;$('monitorStatus').className='monitor-status '+(m.severity||'unknown');
      if(m.sequence && m.sequence!==state.monitorSeq){state.monitorSeq=m.sequence;if(m.mode==='ssh'){if(m.snapshot)renderRaid(m.snapshot);else{state.snapshot=null;hide('raidResult',true);}}await loadHistory();}
      if(m.alertSequence>state.alertSeq){state.alertSeq=m.alertSequence;toast('Мониторинг: '+m.message);}
    }catch(_){/* Bridge is unavailable during window shutdown. */}
  }
  $('monitorStart').addEventListener('click',async()=>{
    let request;try{const mode=$('monitorMode').value;const opts={mode,interval:Number($('monitorInterval').value),notify:$('monitorNotify').checked};if(mode==='ssh'){request=sshOptions();Object.assign(opts,request);}const pending=window.goMonitorStart(opts);$('sshPassword').value='';state.monitorSeq=0;state.alertSeq=0;await pending;await refreshMonitor();toast('Мониторинг запущен');}
    catch(e){toast(message(e));}finally{$('sshPassword').value='';if(request)request.password='';}
  });
  $('monitorStop').addEventListener('click',async()=>{try{await window.goMonitorStop();await refreshMonitor();toast('Мониторинг остановлен');}catch(e){toast(message(e));}});
  setInterval(refreshMonitor,3000);
  $('checkUpdate').addEventListener('click',async()=>{try{const r=await job(()=>window.goUpdateStart(),'Проверяем релизы GitHub…');if(!r)return;$('updateStatus').textContent=r.latest?(r.available?`Доступна версия ${r.latest}${r.prerelease?' (предварительная)':''}; текущая ${r.current}. `:`Текущая версия ${r.current}; более новая среди найденных релизов не обнаружена. `)+r.note:`Релиз RAID Assistant не найден. ${r.note}`;}catch(e){$('updateStatus').textContent=message(e);}});
  const defaultShot=$('datastoreShot').src;
  async function showCustomShot(){try{const shot=await window.goScreenshotLoad();if(shot&&shot.custom&&shot.data){$('datastoreShot').src=shot.data;$('shotCaption').textContent='Ваш снимок Host Client, сохранённый локально.';}}catch(e){toast(message(e));}}
  $('importShot').addEventListener('click',async()=>{try{const shot=await window.goScreenshotImport();if(shot&&shot.data){$('datastoreShot').src=shot.data;$('shotCaption').textContent='Ваш снимок Host Client, сохранённый локально.';toast('Снимок сохранён на этом ПК');}}catch(e){toast(message(e));}});
  $('clearShot').addEventListener('click',async()=>{try{await window.goScreenshotClear();$('datastoreShot').src=defaultShot;$('shotCaption').replaceChildren(document.createTextNode('Host Client → Storage → Datastore browser → Upload; в источнике загружается ISO, а не VIB. '));const b=el('button','text-button','Источник OVHcloud ↗');b.type='button';b.addEventListener('click',()=>window.goOpenLink('screenshot-source').catch(e=>toast(message(e))));$('shotCaption').append(b);toast('Возвращён справочный снимок');}catch(e){toast(message(e));}});
  $('shareTelegram').addEventListener('click',async()=>{try{await renderReport();const text=state.report.markdown.slice(0,1400);if(!confirm('В Telegram будут переданы первые 1400 символов инструкции, включая возможные адреса и названия. Продолжить?'))return;await window.goShareTelegram(text);}catch(e){toast(message(e));}});
  loadProfiles();loadComponents();loadHistory();showCustomShot();refreshMonitor();

  if (window.RAID_DEMO) hide('demoBanner', false);
})();
