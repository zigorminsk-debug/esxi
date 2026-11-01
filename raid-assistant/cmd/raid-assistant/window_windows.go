//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	w := webview2.NewWithOptions(webview2.WebViewOptions{Debug: false, DataPath: dataPath, AutoFocus: true, WindowOptions: webview2.WindowOptions{Title: "ESXi RAID Assistant", Width: 1360, Height: 900, Center: true}})
	if w == nil {
		alert("Не удалось открыть окно. Установите Microsoft Edge WebView2 Runtime и повторите запуск.")
		return errors.New("WebView2 недоступен")
	}
	defer w.Destroy()
	w.SetSize(760, 600, webview2.HintMin)
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
		"goClipboard": copyToClipboard,
	}
	for name, fn := range bindings {
		if err := w.Bind(name, fn); err != nil {
			return err
		}
	}
	// CI can exercise the real WebView2 + native RPC bridge without ESXi.
	// No file is written and no network connection is attempted in this mode.
	smoke := os.Getenv("ESXI_RAID_ASSISTANT_SELFTEST") == "1"
	smokeOK := false
	if smoke {
		if err := w.Bind("goSmokeReady", func(valid bool) {
			smokeOK = valid
			w.Terminate()
		}); err != nil {
			return err
		}
		w.Init(`window.addEventListener('DOMContentLoaded', async function(){
		  let invalidRejected = false;
		  try { await window.goCertStart('https://u:p@invalid.example'); } catch (_) { invalidRejected = true; }
		  await window.goSmokeReady(invalidRejected && !!document.querySelector('#connectionForm') && !window.RAID_DEMO);
		});`)
	}
	w.SetHtml(frontend.HTML(false))
	w.Run()
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

func chooseFile(owner uintptr) (string, error) {
	buffer := make([]uint16, 32768)
	filter := utf16.Encode([]rune("ZIP или VIB (*.zip;*.vib)\x00*.zip;*.vib\x00Все файлы\x00*.*\x00\x00"))
	title, _ := syscall.UTF16PtrFromString("Выберите скачанный пакет LSA для ESXi 8")
	d := openFileName{StructSize: uint32(unsafe.Sizeof(openFileName{})), Owner: owner, Filter: &filter[0], FilterIndex: 1, File: &buffer[0], MaxFile: uint32(len(buffer)), Title: title, Flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000008 | 0x00000004}
	ret, _, _ := getOpenFileName.Call(uintptr(unsafe.Pointer(&d)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(buffer)
	runtime.KeepAlive(title)
	if ret == 0 {
		return "", nil
	} // cancel; no path is sent from web content
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
