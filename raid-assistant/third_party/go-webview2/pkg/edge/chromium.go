//go:build windows
// +build windows

package edge

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

// CI-only non-sensitive lifecycle trace; disabled in ordinary application use.
func RaidTrace(stage string) {
	if os.Getenv("ESXI_RAID_ASSISTANT_SELFTEST") != "1" {
		return
	}
	file := os.Getenv("ESXI_RAID_ASSISTANT_SELFTEST_TRACE")
	if file == "" {
		return
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err == nil {
		_, _ = fmt.Fprintln(f, stage)
		_ = f.Close()
	}
}

type Chromium struct {
	hwnd                   uintptr
	focusOnInit            bool
	controller             *ICoreWebView2Controller
	webview                *ICoreWebView2
	inited                 uintptr
	initAborted            uint32
	initErr                error
	envCompleted           *iCoreWebView2CreateCoreWebView2EnvironmentCompletedHandler
	controllerCompleted    *iCoreWebView2CreateCoreWebView2ControllerCompletedHandler
	webMessageReceived     *iCoreWebView2WebMessageReceivedEventHandler
	permissionRequested    *iCoreWebView2PermissionRequestedEventHandler
	webResourceRequested   *iCoreWebView2WebResourceRequestedEventHandler
	acceleratorKeyPressed  *ICoreWebView2AcceleratorKeyPressedEventHandler
	navigationCompleted    *ICoreWebView2NavigationCompletedEventHandler
	raidNavigationStarting *raidNavigationHandler

	environment *ICoreWebView2Environment

	// The only app-initiated NavigateToString is the embedded document. Its
	// NavigationStarting URI may differ from its eventual about:blank origin.
	pendingLocalNavigation uint32
	localDocumentDigest    [sha256.Size]byte
	localDocumentURI       string

	// Settings
	DataPath string

	// permissions
	permissions      map[CoreWebView2PermissionKind]CoreWebView2PermissionState
	globalPermission *CoreWebView2PermissionState

	// Callbacks
	MessageCallback              func(string)
	WebResourceRequestedCallback func(request *ICoreWebView2WebResourceRequest, args *ICoreWebView2WebResourceRequestedEventArgs)
	NavigationCompletedCallback  func(sender *ICoreWebView2, args *ICoreWebView2NavigationCompletedEventArgs)
	AcceleratorKeyCallback       func(uint) bool
}

func NewChromium() *Chromium {
	e := &Chromium{}
	/*
	 All these handlers are passed to native code through syscalls with 'uintptr(unsafe.Pointer(handler))' and we know
	 that a pointer to those will be kept in the native code. Furthermore these handlers als contain pointer to other Go
	 structs like the vtable.
	 This violates the unsafe.Pointer rule '(4) Conversion of a Pointer to a uintptr when calling syscall.Syscall.' because
	 theres no guarantee that Go doesn't move these objects.
	 AFAIK currently the Go runtime doesn't move HEAP objects, so we should be safe with these handlers. But they don't
	 guarantee it, because in the future Go might use a compacting GC.
	 There's a proposal to add a runtime.Pin function, to prevent moving pinned objects, which would allow to easily fix
	 this issue by just pinning the handlers. The https://go-review.googlesource.com/c/go/+/367296/ should land in Go 1.19.
	*/
	e.envCompleted = newICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler(e)
	e.controllerCompleted = newICoreWebView2CreateCoreWebView2ControllerCompletedHandler(e)
	e.webMessageReceived = newICoreWebView2WebMessageReceivedEventHandler(e)
	e.permissionRequested = newICoreWebView2PermissionRequestedEventHandler(e)
	e.webResourceRequested = newICoreWebView2WebResourceRequestedEventHandler(e)
	e.acceleratorKeyPressed = newICoreWebView2AcceleratorKeyPressedEventHandler(e)
	e.navigationCompleted = newICoreWebView2NavigationCompletedEventHandler(e)
	e.raidNavigationStarting = newRaidNavigationHandler(e)
	e.permissions = make(map[CoreWebView2PermissionKind]CoreWebView2PermissionState)

	return e
}

// Embed retains the upstream bool API; EmbedWithError preserves diagnostics.
func (e *Chromium) Embed(hwnd uintptr) bool { return e.EmbedWithError(hwnd) == nil }

func (e *Chromium) abortInit(err error) {
	e.initErr = err
	atomic.StoreUint32(&e.initAborted, 1)
	atomic.StoreUintptr(&e.inited, 1)
}

func (e *Chromium) EmbedWithError(hwnd uintptr) error {
	RaidTrace("Embed started")
	e.hwnd = hwnd
	if comInitializationError != nil {
		return comInitializationError
	}
	dataPath := e.DataPath
	if dataPath == "" {
		currentExePath := make([]uint16, windows.MAX_PATH)
		_, err := windows.GetModuleFileName(windows.Handle(0), &currentExePath[0], windows.MAX_PATH)
		if err != nil {
			return fmt.Errorf("GetModuleFileName: %w", err)
		}
		currentExeName := filepath.Base(windows.UTF16ToString(currentExePath))
		dataPath = filepath.Join(os.Getenv("AppData"), currentExeName)
	}
	dataPath16, err := windows.UTF16PtrFromString(dataPath)
	if err != nil {
		return errors.New("invalid WebView2 data folder")
	}
	RaidTrace("calling WebView2Loader")
	res, err := createCoreWebView2EnvironmentWithOptions(nil, dataPath16, 0, e.envCompleted)
	RaidTrace(fmt.Sprintf("WebView2Loader result=%08X", uint32(res)))
	if err != nil {
		return fmt.Errorf("WebView2Loader: %w", err)
	}
	if err := checkHRESULT("CreateCoreWebView2EnvironmentWithOptions", res); err != nil {
		return err
	}

	// GetMessageW otherwise waits forever if a broken runtime never invokes
	// its callback. A window timer wakes this same STA thread after 45s.
	const timerID = 0x7351
	timer, _, timerErr := w32.User32SetTimer.Call(hwnd, timerID, 45000, 0)
	if timer == 0 {
		e.abortInit(fmt.Errorf("SetTimer: %w", timerErr))
		return e.initErr
	}
	defer w32.User32KillTimer.Call(hwnd, timer)
	var msg w32.Msg
	for atomic.LoadUintptr(&e.inited) == 0 {
		r, _, getErr := w32.User32GetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 {
			e.abortInit(errors.New("window closed during WebView2 initialization"))
			break
		}
		if int32(uint32(r)) == -1 {
			e.abortInit(fmt.Errorf("GetMessageW: %w", getErr))
			break
		}
		if msg.Message == w32.WMTimer && msg.WParam == timer && uintptr(msg.Hwnd) == hwnd {
			e.abortInit(errors.New("WebView2 initialization timed out"))
			break
		}
		_, _, _ = w32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return e.initErr
}

// CloseFailedInitialization keeps handlers alive until process exit but
// releases any COM objects created before the failure, on the owning thread.
func (e *Chromium) CloseFailedInitialization() {
	atomic.StoreUint32(&e.initAborted, 1)
	if e.controller != nil {
		e.controller.vtbl.Close.Call(uintptr(unsafe.Pointer(e.controller)))
	}
	if e.webview != nil {
		e.webview.vtbl.Release.Call(uintptr(unsafe.Pointer(e.webview)))
		e.webview = nil
	}
	if e.controller != nil {
		e.controller.vtbl.Release.Call(uintptr(unsafe.Pointer(e.controller)))
		e.controller = nil
	}
	if e.environment != nil {
		e.environment.vtbl.Release.Call(uintptr(unsafe.Pointer(e.environment)))
		e.environment = nil
	}
}

func (e *Chromium) Navigate(url string) {
	_, _, _ = e.webview.vtbl.Navigate.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(url))),
	)
}

func (e *Chromium) NavigateToString(htmlContent string) {
	e.localDocumentDigest = sha256.Sum256([]byte(htmlContent))
	e.localDocumentURI = ""
	atomic.StoreUint32(&e.pendingLocalNavigation, 1)
	hr, _, _ := e.webview.vtbl.NavigateToString.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(htmlContent))),
	)
	if int32(hr) != 0 {
		atomic.StoreUint32(&e.pendingLocalNavigation, 0)
	}
}

func (e *Chromium) Init(script string) {
	_, _, _ = e.webview.vtbl.AddScriptToExecuteOnDocumentCreated.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(script))),
		0,
	)
}

func (e *Chromium) Eval(script string) {
	_script, err := windows.UTF16PtrFromString(script)
	if err != nil {
		log.Fatal(err)
	}

	_, _, _ = e.webview.vtbl.ExecuteScript.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(_script)),
		0,
	)
}

func (e *Chromium) Show() error {
	return e.controller.PutIsVisible(true)
}

func (e *Chromium) Hide() error {
	return e.controller.PutIsVisible(false)
}

func (e *Chromium) QueryInterface(_, _ uintptr) uintptr {
	return 0
}

func (e *Chromium) AddRef() uintptr {
	return 1
}

func (e *Chromium) Release() uintptr {
	return 1
}

func (e *Chromium) EnvironmentCompleted(res uintptr, env *ICoreWebView2Environment) uintptr {
	RaidTrace(fmt.Sprintf("environment callback result %08X", uint32(res)))
	if atomic.LoadUint32(&e.initAborted) != 0 {
		return 0
	}
	if err := checkHRESULT("WebView2 environment callback", res); err != nil {
		e.abortInit(err)
		return 0
	}
	if env == nil {
		e.abortInit(errors.New("WebView2 environment callback: empty COM result"))
		return 0
	}
	_, _, _ = env.vtbl.AddRef.Call(uintptr(unsafe.Pointer(env)))
	e.environment = env

	hr, _, _ := env.vtbl.CreateCoreWebView2Controller.Call(
		uintptr(unsafe.Pointer(env)), e.hwnd, uintptr(unsafe.Pointer(e.controllerCompleted)),
	)
	if err := checkHRESULT("CreateCoreWebView2Controller", hr); err != nil {
		e.abortInit(err)
	}
	return 0
}

func (e *Chromium) CreateCoreWebView2ControllerCompleted(res uintptr, controller *ICoreWebView2Controller) uintptr {
	RaidTrace(fmt.Sprintf("controller callback result %08X", uint32(res)))
	if atomic.LoadUint32(&e.initAborted) != 0 {
		return 0
	}
	if err := checkHRESULT("WebView2 controller callback", res); err != nil {
		e.abortInit(err)
		return 0
	}
	if controller == nil {
		e.abortInit(errors.New("WebView2 controller callback: empty COM result"))
		return 0
	}
	_, _, _ = controller.vtbl.AddRef.Call(uintptr(unsafe.Pointer(controller)))
	e.controller = controller

	hr, _, _ := controller.vtbl.GetCoreWebView2.Call(uintptr(unsafe.Pointer(controller)), uintptr(unsafe.Pointer(&e.webview)))
	if err := checkHRESULT("get_CoreWebView2", hr); err != nil {
		e.abortInit(err)
		return 0
	}
	if e.webview == nil {
		e.abortInit(errors.New("get_CoreWebView2: empty COM result"))
		return 0
	}
	// get_CoreWebView2 returns an owned reference. Require the navigation and
	// message handlers to be registered before any application HTML loads.
	var token _EventRegistrationToken
	for _, event := range []struct {
		name    string
		proc    ComProc
		handler unsafe.Pointer
	}{
		{"add_NavigationStarting", e.webview.vtbl.AddNavigationStarting, unsafe.Pointer(e.raidNavigationStarting)},
		{"add_FrameNavigationStarting", e.webview.vtbl.AddFrameNavigationStarting, unsafe.Pointer(e.raidNavigationStarting)},
		{"add_WebMessageReceived", e.webview.vtbl.AddWebMessageReceived, unsafe.Pointer(e.webMessageReceived)},
		{"add_PermissionRequested", e.webview.vtbl.AddPermissionRequested, unsafe.Pointer(e.permissionRequested)},
		{"add_WebResourceRequested", e.webview.vtbl.AddWebResourceRequested, unsafe.Pointer(e.webResourceRequested)},
		{"add_NavigationCompleted", e.webview.vtbl.AddNavigationCompleted, unsafe.Pointer(e.navigationCompleted)},
	} {
		hr, _, _ = event.proc.Call(uintptr(unsafe.Pointer(e.webview)), uintptr(event.handler), uintptr(unsafe.Pointer(&token)))
		if err := checkHRESULT(event.name, hr); err != nil {
			e.abortInit(err)
			return 0
		}
	}
	if err := e.controller.AddAcceleratorKeyPressed(e.acceleratorKeyPressed, &token); err != nil {
		e.abortInit(err)
		return 0
	}
	atomic.StoreUintptr(&e.inited, 1)
	if e.focusOnInit {
		e.Focus()
	}
	return 0
}

func (e *Chromium) MessageReceived(_ *ICoreWebView2, args *iCoreWebView2WebMessageReceivedEventArgs) uintptr {
	// Never expose the native bridge to a remote or local file document, even
	// if navigation is somehow triggered by the WebView runtime itself.
	var source *uint16
	hr, _, _ := args.vtbl.GetSource.Call(uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(&source)))
	sourceURI := ""
	if source != nil {
		sourceURI = w32.Utf16PtrToString(source)
		windows.CoTaskMemFree(unsafe.Pointer(source))
	}
	// WebView2 may report the documented about:blank origin or the exact
	// data URI validated during the trusted NavigateToString navigation.
	allowed := hr == 0 && (sourceURI == "about:blank" || (e.localDocumentURI != "" && sourceURI == e.localDocumentURI))
	RaidTrace(fmt.Sprintf("message received allowed=%v source=%.60q", allowed, sourceURI))
	if !allowed {
		return 0
	}
	var message *uint16
	hr, _, _ = args.vtbl.TryGetWebMessageAsString.Call(uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(&message)))
	if hr != 0 || message == nil {
		return 0
	}
	if e.MessageCallback != nil {
		e.MessageCallback(w32.Utf16PtrToString(message))
	}
	// Do not echo RPC messages: a login RPC may contain a password.
	windows.CoTaskMemFree(unsafe.Pointer(message))
	return 0
}

func (e *Chromium) SetPermission(kind CoreWebView2PermissionKind, state CoreWebView2PermissionState) {
	e.permissions[kind] = state
}

func (e *Chromium) SetGlobalPermission(state CoreWebView2PermissionState) {
	e.globalPermission = &state
}

func (e *Chromium) PermissionRequested(_ *ICoreWebView2, args *iCoreWebView2PermissionRequestedEventArgs) uintptr {
	var kind CoreWebView2PermissionKind
	hr, _, _ := args.vtbl.GetPermissionKind.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(&kind)),
	)
	if hresultFailed(hr) {
		// Fail closed rather than treating an unknown permission as approved.
		args.vtbl.PutState.Call(uintptr(unsafe.Pointer(args)), uintptr(CoreWebView2PermissionStateDeny))
		return 0
	}
	var result CoreWebView2PermissionState
	if e.globalPermission != nil {
		result = *e.globalPermission
	} else {
		var ok bool
		result, ok = e.permissions[kind]
		if !ok {
			result = CoreWebView2PermissionStateDefault
		}
	}
	_, _, _ = args.vtbl.PutState.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(result),
	)
	return 0
}

func (e *Chromium) WebResourceRequested(sender *ICoreWebView2, args *ICoreWebView2WebResourceRequestedEventArgs) uintptr {
	req, err := args.GetRequest()
	if err != nil {
		log.Fatal(err)
	}
	if e.WebResourceRequestedCallback != nil {
		e.WebResourceRequestedCallback(req, args)
	}
	return 0
}

func (e *Chromium) AddWebResourceRequestedFilter(filter string, ctx COREWEBVIEW2_WEB_RESOURCE_CONTEXT) {
	err := e.webview.AddWebResourceRequestedFilter(filter, ctx)
	if err != nil {
		log.Fatal(err)
	}
}

func (e *Chromium) Environment() *ICoreWebView2Environment {
	return e.environment
}

// AcceleratorKeyPressed is called when an accelerator key is pressed.
// If the AcceleratorKeyCallback method has been set, it will defer handling of the keypress
// to the callback. That callback returns a bool indicating if the event was handled.
func (e *Chromium) AcceleratorKeyPressed(sender *ICoreWebView2Controller, args *ICoreWebView2AcceleratorKeyPressedEventArgs) uintptr {
	if e.AcceleratorKeyCallback == nil {
		return 0
	}
	eventKind, _ := args.GetKeyEventKind()
	if eventKind == COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN ||
		eventKind == COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN {
		virtualKey, _ := args.GetVirtualKey()
		status, _ := args.GetPhysicalKeyStatus()
		if !status.WasKeyDown {
			_ = args.PutHandled(e.AcceleratorKeyCallback(virtualKey))
			return 0
		}
	}
	_ = args.PutHandled(false)
	return 0
}

func (e *Chromium) GetSettings() (*ICoreWebViewSettings, error) {
	return e.webview.GetSettings()
}

func (e *Chromium) GetController() *ICoreWebView2Controller {
	return e.controller
}

func boolToInt(input bool) int {
	if input {
		return 1
	}
	return 0
}

func (e *Chromium) NavigationCompleted(sender *ICoreWebView2, args *ICoreWebView2NavigationCompletedEventArgs) uintptr {
	if e.NavigationCompletedCallback != nil {
		e.NavigationCompletedCallback(sender, args)
	}
	return 0
}

func (e *Chromium) NotifyParentWindowPositionChanged() error {
	//It looks like the wndproc function is called before the controller initialization is complete.
	//Because of this the controller is nil
	if e.controller == nil {
		return nil
	}
	return e.controller.NotifyParentWindowPositionChanged()
}

func (e *Chromium) Focus() {
	if e.controller == nil {
		e.focusOnInit = true
		return
	}
	_ = e.controller.MoveFocus(COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
}
