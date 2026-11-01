//go:build windows

package edge

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

type raidNavigationArgsVtbl struct {
	_IUnknownVtbl
	GetURI             ComProc
	GetIsUserInitiated ComProc
	GetIsRedirected    ComProc
	GetRequestHeaders  ComProc
	GetCancel          ComProc
	PutCancel          ComProc
	GetNavigationID    ComProc
}
type raidNavigationArgs struct{ vtbl *raidNavigationArgsVtbl }
type raidNavigationHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}
type raidNavigationHandler struct {
	vtbl *raidNavigationHandlerVtbl
	impl *Chromium
}

func raidQuery(h *raidNavigationHandler, id, out uintptr) uintptr {
	return h.impl.QueryInterface(id, out)
}
func raidAddRef(h *raidNavigationHandler) uintptr  { return h.impl.AddRef() }
func raidRelease(h *raidNavigationHandler) uintptr { return h.impl.Release() }
func raidInvoke(h *raidNavigationHandler, _ *ICoreWebView2, a *raidNavigationArgs) uintptr {
	var source *uint16
	hr, _, _ := a.vtbl.GetURI.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&source)))
	uri := ""
	if source != nil {
		uri = w32.Utf16PtrToString(source)
		windows.CoTaskMemFree(unsafe.Pointer(source))
	}
	lower := strings.ToLower(uri)
	allowed := hr == 0 && lower == "about:blank"
	// NavigateToString is invoked only by the native application with its
	// embedded HTML. Its navigation-start URI is not necessarily the final
	// origin; permit one locally initiated non-network navigation, never a
	// HTTP/file/other external destination. The RPC bridge still checks the
	// final WebMessageReceived source independently.
	var initiated int32
	hri, _, _ := a.vtbl.GetIsUserInitiated.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&initiated)))
	localURI := lower == "" || strings.HasPrefix(lower, "about:") || strings.HasPrefix(lower, "data:text/html")
	if !allowed && hr == 0 && hri == 0 && initiated != 0 && localURI && atomic.CompareAndSwapUint32(&h.impl.pendingLocalNavigation, 1, 0) {
		allowed = true
	}
	if allowed && lower == "about:blank" {
		atomic.StoreUint32(&h.impl.pendingLocalNavigation, 0)
	}
	RaidTrace(fmt.Sprintf("navigation allowed=%v initiated=%v URI=%.60q", allowed, initiated != 0, uri))
	if !allowed {
		_, _, _ = a.vtbl.PutCancel.Call(uintptr(unsafe.Pointer(a)), 1)
	}
	return 0
}

var raidNavigationHandlerFunctions = raidNavigationHandlerVtbl{_IUnknownVtbl{NewComProc(raidQuery), NewComProc(raidAddRef), NewComProc(raidRelease)}, NewComProc(raidInvoke)}

func newRaidNavigationHandler(c *Chromium) *raidNavigationHandler {
	return &raidNavigationHandler{vtbl: &raidNavigationHandlerFunctions, impl: c}
}
