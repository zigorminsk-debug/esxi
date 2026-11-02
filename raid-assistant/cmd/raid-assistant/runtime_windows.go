//go:build windows

package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/runtimeinstaller"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runtimeRegistryPath = `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
const runtimeHelpURL = "https://developer.microsoft.com/microsoft-edge/webview2/"

// Microsoft documents the machine and current-user pv registry values as the
// Runtime-specific check. Unlike the loader's version API this does not mistake
// an Edge preview channel for the production WebView2 Runtime.
func detectRuntime() (string, error) {
	var firstErr error
	for _, location := range []struct {
		root registry.Key
		view uint32
	}{{registry.LOCAL_MACHINE, registry.WOW64_32KEY}, {registry.CURRENT_USER, 0}} {
		k, err := registry.OpenKey(location.root, runtimeRegistryPath, registry.QUERY_VALUE|location.view)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		pv, _, err := k.GetStringValue("pv")
		_ = k.Close()
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		pv = strings.TrimSpace(pv)
		if pv == "" || pv == "0.0.0.0" {
			continue
		}
		if !runtimeVersionValid(pv) {
			if firstErr == nil {
				firstErr = errors.New("invalid Runtime version in registry")
			}
			continue
		}
		return pv, nil
	}
	if firstErr != nil {
		return "", firstErr // unknown is not the same as absent
	}
	return "", nil
}

func runtimeVersionValid(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return false
			}
		}
		if _, err := strconv.ParseUint(p, 10, 32); err != nil {
			return false
		}
	}
	return true
}

var (
	wintrust       = windows.NewLazySystemDLL("wintrust.dll")
	providerData   = wintrust.NewProc("WTHelperProvDataFromStateData")
	providerSigner = wintrust.NewProc("WTHelperGetProvSignerFromChain")
	providerCert   = wintrust.NewProc("WTHelperGetProvCertFromChain")
)

// The first members of CRYPT_PROVIDER_CERT. The certificate belongs to the
// WinVerifyTrust state and is parsed before that state is closed.
type trustProviderCert struct {
	Size uint32
	Cert *windows.CertContext
}

func isMicrosoftSigner(cert *x509.Certificate) bool {
	return cert != nil && cert.Subject.CommonName == "Microsoft Corporation" &&
		len(cert.Subject.Organization) == 1 && cert.Subject.Organization[0] == "Microsoft Corporation"
}

func verifyMicrosoftSetup(path string) error {
	filePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("invalid installer path")
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: filePath}
	// Microsoft currently supplies a second, unrelated signature as well; ask
	// Windows to verify the primary (Microsoft) signature specifically.
	sig := windows.WinTrustSignatureSettings{Size: uint32(unsafe.Sizeof(windows.WinTrustSignatureSettings{})), Index: 0, Flags: 1 /* WSS_VERIFY_SPECIFIC */}
	data := windows.WinTrustData{
		Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_WHOLECHAIN, UnionChoice: windows.WTD_CHOICE_FILE,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file), StateAction: windows.WTD_STATEACTION_VERIFY,
		ProvFlags: windows.WTD_REVOCATION_CHECK_CHAIN_EXCLUDE_ROOT, SignatureSettings: &sig,
	}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	}()
	if err != nil {
		return fmt.Errorf("WinVerifyTrust: %w", err)
	}
	if data.StateData == 0 || sig.VerifiedSigIndex != 0 {
		return errors.New("missing primary signature verification state")
	}
	pd, _, _ := providerData.Call(uintptr(data.StateData))
	if pd == 0 {
		return errors.New("missing signature provider data")
	}
	signer, _, _ := providerSigner.Call(pd, 0, 0, 0)
	if signer == 0 {
		return errors.New("missing verified signer")
	}
	pc, _, _ := providerCert.Call(signer, 0)
	if pc == 0 {
		return errors.New("missing signer certificate")
	}
	certContext := (*trustProviderCert)(unsafe.Pointer(pc)).Cert
	if certContext == nil || certContext.EncodedCert == nil || certContext.Length == 0 || certContext.Length > 65536 {
		return errors.New("invalid signer certificate")
	}
	cert, err := x509.ParseCertificate(unsafe.Slice(certContext.EncodedCert, int(certContext.Length)))
	if err != nil || !isMicrosoftSigner(cert) {
		return errors.New("installer is not signed by Microsoft Corporation")
	}
	runtime.KeepAlive(filePath)
	runtime.KeepAlive(file)
	return nil
}

var (
	shell32        = windows.NewLazySystemDLL("shell32.dll")
	shellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Hwnd       uintptr
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   uintptr
	IDList     uintptr
	Class      *uint16
	ClassKey   uintptr
	HotKey     uint32
	Icon       uintptr
	Process    windows.Handle
}

// Run the standard Microsoft installer after consent. ShellExecuteEx lets
// Windows show a UAC prompt if needed; no shell interpolation is involved.
func installMicrosoftSetup(ctx context.Context, path string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, 2); err != nil && !errors.Is(err, syscall.Errno(1)) {
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	defer windows.CoUninitialize()
	filePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("invalid installer path")
	}
	verb, _ := windows.UTF16PtrFromString("open")
	args, _ := windows.UTF16PtrFromString("/silent /install")
	info := shellExecuteInfo{Size: uint32(unsafe.Sizeof(shellExecuteInfo{})), Mask: 0x40 | 0x100, File: filePath, Verb: verb, Parameters: args, Show: 1}
	ok, _, callErr := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(filePath)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(args)
	if ok == 0 {
		return fmt.Errorf("ShellExecuteEx: %w", callErr)
	}
	if info.Process == 0 {
		return errors.New("installer process handle unavailable")
	}
	defer windows.CloseHandle(info.Process)
	deadline := time.Now().Add(15 * time.Minute)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return errors.New("installer is still running")
		}
		state, err := windows.WaitForSingleObject(info.Process, 1000)
		if err != nil {
			return err
		}
		if state == uint32(windows.WAIT_TIMEOUT) {
			continue
		}
		if state != windows.WAIT_OBJECT_0 {
			return errors.New("unexpected installer wait status")
		}
		var code uint32
		if err := windows.GetExitCodeProcess(info.Process, &code); err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("installer exit code 0x%08X", code)
		}
		return nil
	}
}

func confirmRuntimeInstall() bool {
	text, _ := syscall.UTF16PtrFromString("WebView2 Runtime отсутствует. Официальный установщик Microsoft скачан и его подпись проверена.\n\nРазрешить установку на этот компьютер? Установщику понадобится интернет; Windows может запросить подтверждение.\n\nСервер ESXi изменяться не будет.")
	caption, _ := syscall.UTF16PtrFromString("ESXi RAID Assistant — установка WebView2")
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)), 0x4|0x20|0x100|0x10000)
	return r == 6 // IDYES; default is No
}

func runtimeFailureText(err error) string {
	var failure *runtimeinstaller.Failure
	if !errors.As(err, &failure) {
		return "Не удалось подготовить WebView2 Runtime. Перезапустите программу."
	}
	switch failure.Stage {
	case "detect":
		return "Не удалось определить, установлен ли WebView2 Runtime. Автоматическая загрузка не выполнялась. Проверьте доступ к реестру Windows и повторите запуск.\n\nКод: RUNTIME-DETECT"
	case "download":
		return "WebView2 Runtime не найден, и скачать установщик Microsoft не удалось. Проверьте интернет и настройки прокси или скачайте Evergreen Standalone Installer x64 на другом ПК:\n" + runtimeHelpURL + "\n\nКод: RUNTIME-DOWNLOAD"
	case "verify":
		return "Не удалось подтвердить действительную цифровую подпись Microsoft у скачанного установщика. Установка не запускалась. Проверьте дату, интернет и сертификаты Windows или скачайте установщик с официального сайта:\n" + runtimeHelpURL + "\n\nКод: RUNTIME-SIGNATURE"
	case "install", "postcheck":
		return "WebView2 Runtime пока не обнаружен после установки. Проверьте интернет, разрешение Windows и ограничения организации. Если установщик ещё работает, дождитесь окончания и запустите помощник снова.\n\nОфициальный установщик: " + runtimeHelpURL + "\nКод: RUNTIME-INSTALL"
	default:
		return "Установка WebView2 не завершена."
	}
}

func ensureRuntime() (string, error) {
	return runtimeinstaller.Ensure(context.Background(), runtimeinstaller.Operations{
		Detect: detectRuntime, Download: runtimeinstaller.DownloadOfficial, Verify: verifyMicrosoftSetup,
		Confirm: confirmRuntimeInstall, Install: installMicrosoftSetup, Progress: runtimeProgress,
	})
}

// Windows GUI binaries have no console. This small native progress window makes
// the automatic download visible while a worker performs network/installer IO.
type winMessage struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

func runtimeProgress(stage string, work func() error) error {
	windowText := "WebView2 Runtime не найден.\n\nСкачиваем и проверяем официальный установщик Microsoft…"
	if stage == "install" {
		windowText = "Устанавливаем WebView2 Runtime.\n\nПожалуйста, дождитесь окончания и подтвердите запрос Windows, если он появится."
	}
	class, _ := syscall.UTF16PtrFromString("STATIC")
	caption, _ := syscall.UTF16PtrFromString("ESXi RAID Assistant")
	label, _ := syscall.UTF16PtrFromString(windowText)
	create := user32.NewProc("CreateWindowExW")
	metrics := user32.NewProc("GetSystemMetrics")
	sx, _, _ := metrics.Call(0)
	sy, _, _ := metrics.Call(1)
	const width, height = 560, 150
	var x, y uintptr
	if sx > width {
		x = (sx - width) / 2
	}
	if sy > height {
		y = (sy - height) / 2
	}
	// A simple non-cancellable top-level STATIC window. Work is time-bounded;
	// no network operations or credentials go to ESXi at this stage.
	hwnd, _, _ := create.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), 0x80000000|0x00C00000|0x10000000,
		x, y, width, height, 0, 0, 0, 0)
	if hwnd == 0 {
		return work()
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	child, _, _ := create.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(label)), 0x40000000|0x10000000,
		22, 18, width-44, height-55, hwnd, 0, 0, 0)
	if child != 0 {
		font, _, _ := windows.NewLazySystemDLL("gdi32.dll").NewProc("GetStockObject").Call(17) // DEFAULT_GUI_FONT
		user32.NewProc("SendMessageW").Call(child, 0x0030, font, 1)                            // WM_SETFONT
	}
	user32.NewProc("UpdateWindow").Call(hwnd)
	thread, _, _ := kernel32.NewProc("GetCurrentThreadId").Call()
	const doneMessage = 0x8000 + 27
	done := make(chan error, 1)
	go func() {
		done <- work()
		user32.NewProc("PostThreadMessageW").Call(thread, doneMessage, 0, 0)
	}()
	var msg winMessage
	getMessage := user32.NewProc("GetMessageW")
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(uint32(r)) == -1 || msg.Message == doneMessage {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	return <-done
}
