//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/jchv/go-webview2"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/app"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/frontend"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	comdlg32        = syscall.NewLazyDLL("comdlg32.dll")
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	messageBox      = user32.NewProc("MessageBoxW")
	getOpenFileName = comdlg32.NewProc("GetOpenFileNameW")
	fileDialogError = comdlg32.NewProc("CommDlgExtendedError")
)

func alert(text string) {
	t, _ := syscall.UTF16PtrFromString(text)
	caption, _ := syscall.UTF16PtrFromString("ESXi RAID Assistant")
	messageBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(caption)), 0x10)
}

func runWindow(a *app.App) error {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	dataPath := filepath.Join(cache, "ESXi-RAID-Assistant", "WebView2")
	selfTestTrace("starting WebView2")
	w := webview2.NewWithOptions(webview2.WebViewOptions{Debug: false, DataPath: dataPath, AutoFocus: true, WindowOptions: webview2.WindowOptions{Title: "ESXi RAID Assistant", Width: 1360, Height: 900, Center: true}})
	selfTestTrace("WebView2 returned")
	if w == nil {
		selfTestTrace("WebView2 unavailable")
		if os.Getenv("ESXI_RAID_ASSISTANT_SELFTEST") != "1" {
			alert("Не удалось открыть окно или отключить автозаполнение. Установите либо обновите Microsoft Edge WebView2 Runtime и повторите запуск.")
		}
		return errors.New("WebView2 недоступен")
	}
	defer w.Destroy()
	w.SetSize(760, 600, webview2.HintMin)
	// Optional notifications are only enabled by the user when monitoring.
	// A single non-blocking native dialog is shown for a new warning state.
	var alertOpen atomic.Bool
	a.SetNotifier(func(title, text string) {
		if !alertOpen.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer alertOpen.Store(false)
			t, _ := syscall.UTF16PtrFromString(text)
			c, _ := syscall.UTF16PtrFromString(title)
			messageBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x30|0x10000|0x40000)
		}()
	})
	bindings := map[string]any{
		"goCertStart":     a.StartCertificate,
		"goScanStart":     a.StartScan,
		"goPoll":          a.Poll,
		"goDownloadStart": a.StartDownload,
		"goReport":        a.Report,
		"goSave":          a.SaveReport,
		"goImportStart": func() (string, error) {
			file, err := chooseFile(uintptr(w.Window()))
			if err != nil {
				return "", err
			}
			if file == "" {
				return "", nil
			}
			return a.StartImport(file)
		},
		"goOpenLink": func(kind string) error {
			u, err := a.Link(kind)
			if err != nil {
				return err
			}
			return openWindows(u)
		},
		"goOpenFolder": func() error {
			p, err := a.OutputDir()
			if err != nil {
				return err
			}
			return exec.Command("explorer.exe", p).Start()
		},
		"goClipboard":       copyToClipboard,
		"goInvalidate":      a.Invalidate,
		"goProfiles":        a.Profiles,
		"goProfileSave":     a.SaveProfile,
		"goProfileDelete":   a.DeleteProfile,
		"goComponents":      a.Components,
		"goDiagnostics":     a.Diagnostics,
		"goPortStart":       a.StartPortProbe,
		"goSSHKeyStart":     a.StartSSHKey,
		"goSSHReadStart":    a.StartSSHRead,
		"goCorrelate":       a.Correlate,
		"goMonitorStart":    a.StartMonitor,
		"goMonitor":         a.Monitor,
		"goMonitorStop":     a.StopMonitor,
		"goHistory":         a.History,
		"goHistoryClear":    a.ClearHistory,
		"goUpdateStart":     a.StartUpdate,
		"goScreenshotLoad":  a.LoadScreenshot,
		"goScreenshotClear": a.ClearScreenshot,
		"goScreenshotImport": func() (any, error) {
			file, err := chooseDataFile(uintptr(w.Window()), "image")
			if err != nil || file == "" {
				return nil, err
			}
			return a.ImportScreenshot(file)
		},
		"goRaidImportStart": func() (string, error) {
			file, err := chooseDataFile(uintptr(w.Window()), "report")
			if err != nil || file == "" {
				return "", err
			}
			return a.StartRaidImport(file)
		},
		"goShareTelegram": func(text string) error {
			if len(text) == 0 || len(text) > 6000 {
				return errors.New("Текст слишком большой для передачи")
			}
			// An explicitly clicked action and confirmation are required in the UI.
			target := "https://t.me/share/url?url=" + url.QueryEscape("https://github.com/zigorminsk-debug/esxi") + "&text=" + url.QueryEscape(text)
			return openWindows(target)
		},
	}
	for name, fn := range bindings {
		if err := w.Bind(name, fn); err != nil {
			return err
		}
	}
	// CI exercises the real WebView2 + native RPC bridge and a loopback-only
	// govmomi simulator. The fixture has fake credentials, not a real ESXi.
	smoke := os.Getenv("ESXI_RAID_ASSISTANT_SELFTEST") == "1"
	smokeOK := false
	if smoke {
		if err := w.Bind("goSmokeReady", func(valid bool, stage string) {
			smokeOK = valid
			selfTestTrace("smoke callback: " + stage)
			w.Terminate()
		}); err != nil {
			return err
		}
		endpointJSON, _ := json.Marshal(os.Getenv("ESXI_RAID_ASSISTANT_SELFTEST_ENDPOINT"))
		script := `window.addEventListener('DOMContentLoaded', async function(){
		  let valid = false, stage = 'validation';
		  try {
		    let invalidRejected = false, sshRejected = false, monitorRejected = false;
		    try { await window.goCertStart('https://u:p@invalid.example'); } catch (_) { invalidRejected = true; }
		    try { await window.goSSHReadStart({user:'test',password:'not-a-credential',kind:'storcli',consent:true}); } catch (_) { sshRejected = true; }
		    try { await window.goMonitorStart({mode:'ssh',interval:60}); } catch (_) { monitorRejected = true; }
		    stage = 'read components and history';
		    const catalog = await window.goComponents(), history = await window.goHistory();
		    const monitor = await window.goMonitor(), shot = await window.goScreenshotLoad();
		    stage = 'save profile';
		    const saved = await window.goProfileSave({name:'CI smoke',address:'192.0.2.111',user:'fixture'});
		    stage = 'read and delete profile';
		    const profiles = await window.goProfiles();
		    await window.goProfileDelete('192.0.2.111');
		    stage = 'validate results';
		    valid = invalidRejected && sshRejected && monitorRejected && Array.isArray(catalog) && catalog.length >= 20 &&
		      Array.isArray(history) && !monitor.running && shot && Array.isArray(saved) &&
		      profiles.some(p => p.address === '192.0.2.111:443' && p.user === 'fixture' && !('password' in p)) &&
		      !!document.querySelector('#connectionForm') && !!document.querySelector('#raid') && !!document.querySelector('#extensions') && !window.RAID_DEMO;
		    if (!valid) throw new Error('native bridge validation');
		    const endpoint = /*__SMOKE_ENDPOINT__*/;
		    if (!/^127\.0\.0\.1:\d+$/.test(endpoint)) throw new Error('loopback fixture not configured');
		    const field = (id, value) => { const e = document.getElementById(id); e.value = value; e.dispatchEvent(new Event('input', {bubbles:true})); };
		    const visible = id => { const e = document.getElementById(id); return e && !e.hidden; };
		    const wait = async (condition, what) => {
		      const until = Date.now() + 15000;
		      while (Date.now() < until) { if (condition()) return; await new Promise(r => setTimeout(r, 50)); }
		      throw new Error('timeout: ' + what);
		    };
		    const connect = () => document.getElementById('connectButton').click();
		    field('address', endpoint); field('username', 'fixture-user'); field('password', 'fixture-password');
		    stage = 'click connect and cancel untrusted warning'; connect();
		    await wait(() => visible('securityDialog'), 'untrusted warning');
		    if (!document.getElementById('securityDialog').textContent.includes('перехвачен')) throw new Error('warning not shown');
		    document.getElementById('securityCancel').click();
		    await wait(() => document.getElementById('connectionStatus').textContent.includes('отменено'), 'cancelled');
		    if (document.getElementById('password').value) throw new Error('password retained after cancellation');
		    stage = 'click connect with incorrect credentials'; field('password', 'wrong-fixture-password'); connect();
		    await wait(() => visible('securityDialog'), 'second warning'); document.getElementById('securityContinue').click();
		    await wait(() => document.getElementById('connectionStatus').textContent.includes('Вход в ESXi не выполнен'), 'login error');
		    if (document.getElementById('password').value) throw new Error('password retained after failed login');
		    stage = 'click connect with valid fixture credentials'; field('password', 'fixture-password'); connect();
		    await wait(() => visible('securityDialog'), 'third warning'); document.getElementById('securityContinue').click();
		    await wait(() => visible('diagnosticResult'), 'fixture inventory');
		    if (!document.getElementById('hostName').textContent || !visible('connectionWarning') || document.getElementById('password').value) throw new Error('inventory or security status missing');
		    await wait(() => !document.getElementById('connectButton').disabled, 'connection form ready');
		    stage = 'change host and show network error'; field('address', '127.0.0.1:1');
		    if (visible('diagnosticResult')) throw new Error('old host still visible');
		    field('password', 'fixture-password'); connect();
		    await wait(() => document.getElementById('connectionStatus').textContent.includes('Не удалось подключиться'), 'network error');
		    if (document.getElementById('password').value || visible('diagnosticResult')) throw new Error('failed connection retained data');
		    valid = true; stage = 'real WebView2 form + simulator passed';
		  } catch (e) { valid = false; stage += ': ' + String(e).slice(0, 200); }
		  await window.goSmokeReady(valid, stage);
		});`
		w.Init(strings.Replace(script, "/*__SMOKE_ENDPOINT__*/", string(endpointJSON), 1))
	}
	selfTestTrace("bindings registered")
	w.SetHtml(frontend.HTML(false))
	selfTestTrace("HTML navigation requested")
	w.Run()
	selfTestTrace("window loop ended")
	if smoke && !smokeOK {
		return errors.New("WebView2 self-test failed")
	}
	return nil
}

func openWindows(target string) error {
	// target is generated only from a fixed allowlist in app.Link, or from a
	// previously parsed ESXi address. No shell interpolation is involved.
	if err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target).Start(); err != nil {
		return errors.New("Не удалось открыть браузер")
	}
	return nil
}

type openFileName struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefaultExt      *uint16
	CustData        uintptr
	Hook            uintptr
	TemplateName    *uint16
	Reserved        unsafe.Pointer
	Reserved2       uint32
	FlagsEx         uint32
}

func chooseFile(owner uintptr) (string, error) { return chooseDataFile(owner, "package") }
func chooseDataFile(owner uintptr, kind string) (string, error) {
	buffer := make([]uint16, 32768)
	filterText, titleText := "ZIP или VIB (*.zip;*.vib)\x00*.zip;*.vib\x00Все файлы\x00*.*\x00\x00", "Выберите скачанный пакет LSA для ESXi 8"
	switch kind {
	case "report":
		filterText, titleText = "Отчёты JSON и текст (*.json;*.txt)\x00*.json;*.txt\x00Все файлы\x00*.*\x00\x00", "Выберите отчёт StorCLI или HPE SSACLI"
	case "image":
		filterText, titleText = "Изображения PNG и JPEG (*.png;*.jpg;*.jpeg)\x00*.png;*.jpg;*.jpeg\x00\x00", "Выберите свой скриншот Host Client"
	case "package":
	default:
		return "", errors.New("Неизвестный тип файла")
	}
	filter := utf16.Encode([]rune(filterText))
	title, _ := syscall.UTF16PtrFromString(titleText)
	d := openFileName{StructSize: uint32(unsafe.Sizeof(openFileName{})), Owner: owner, Filter: &filter[0], FilterIndex: 1, File: &buffer[0], MaxFile: uint32(len(buffer)), Title: title, Flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000008 | 0x00000004}
	ret, _, _ := getOpenFileName.Call(uintptr(unsafe.Pointer(&d)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(buffer)
	runtime.KeepAlive(title)
	if ret == 0 {
		if code, _, _ := fileDialogError.Call(); code != 0 {
			return "", errors.New("Не удалось открыть системный диалог выбора файла")
		}
		return "", nil // cancel; no path is sent from web content
	}
	return syscall.UTF16ToString(buffer), nil
}

// Copying is explicitly triggered by the user; credentials are never copied by
// the application. Windows owns the global allocation after SetClipboardData.
func copyToClipboard(text string) error {
	if len(text) > 1<<20 {
		return errors.New("Текст слишком большой для буфера обмена")
	}
	open := user32.NewProc("OpenClipboard")
	close := user32.NewProc("CloseClipboard")
	empty := user32.NewProc("EmptyClipboard")
	set := user32.NewProc("SetClipboardData")
	alloc := kernel32.NewProc("GlobalAlloc")
	lock := kernel32.NewProc("GlobalLock")
	unlock := kernel32.NewProc("GlobalUnlock")
	free := kernel32.NewProc("GlobalFree")
	ok, _, _ := open.Call(0)
	if ok == 0 {
		return errors.New("Буфер обмена занят")
	}
	defer close.Call()
	data := utf16.Encode([]rune(text + "\x00"))
	mem, _, _ := alloc.Call(0x0002, uintptr(len(data)*2)) // GMEM_MOVEABLE
	if mem == 0 {
		return errors.New("Не удалось выделить память для буфера")
	}
	ptr, _, _ := lock.Call(mem)
	if ptr == 0 {
		free.Call(mem)
		return errors.New("Не удалось открыть память буфера")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(data)), data)
	unlock.Call(mem)
	empty.Call()
	if r, _, _ := set.Call(13, mem); r == 0 {
		free.Call(mem)
		return errors.New("Не удалось записать буфер обмена")
	} // CF_UNICODETEXT
	return nil
}
