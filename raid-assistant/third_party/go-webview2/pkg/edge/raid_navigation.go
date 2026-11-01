//go:build windows

package edge

import (
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
func raidInvoke(_ *raidNavigationHandler, _ *ICoreWebView2, a *raidNavigationArgs) uintptr {
	var source *uint16
	hr, _, _ := a.vtbl.GetURI.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&source)))
	allowed := hr == 0 && source != nil && w32.Utf16PtrToString(source) == "about:blank"
	if source != nil {
		windows.CoTaskMemFree(unsafe.Pointer(source))
	}
	if !allowed {
		_, _, _ = a.vtbl.PutCancel.Call(uintptr(unsafe.Pointer(a)), 1)
	}
	return 0
}

var raidNavigationHandlerFunctions = raidNavigationHandlerVtbl{_IUnknownVtbl{NewComProc(raidQuery), NewComProc(raidAddRef), NewComProc(raidRelease)}, NewComProc(raidInvoke)}

func newRaidNavigationHandler(c *Chromium) *raidNavigationHandler {
	return &raidNavigationHandler{vtbl: &raidNavigationHandlerFunctions, impl: c}
}
