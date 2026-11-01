//go:build windows

package edge

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/jchv/go-webview2/internal/raidguard"
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
	allowed := hr == 0 && uri == "about:blank"
	// WebView2 currently starts NavigateToString as a base64 data URI and
	// reports IsUserInitiated=false. Allow it only once, when its decoded
	// bytes exactly match the native application's embedded document.
	if !allowed && hr == 0 && atomic.LoadUint32(&h.impl.pendingLocalNavigation) == 1 && raidguard.IsEmbeddedDocument(uri, h.impl.localDocumentDigest) && atomic.CompareAndSwapUint32(&h.impl.pendingLocalNavigation, 1, 0) {
		h.impl.localDocumentURI = uri
		allowed = true
	}
	if allowed && uri == "about:blank" && atomic.CompareAndSwapUint32(&h.impl.pendingLocalNavigation, 1, 0) {
		h.impl.localDocumentURI = uri
	}
	RaidTrace(fmt.Sprintf("navigation allowed=%v URI=%.60q", allowed, uri))
	if !allowed {
		_, _, _ = a.vtbl.PutCancel.Call(uintptr(unsafe.Pointer(a)), 1)
	}
	return 0
}

var raidNavigationHandlerFunctions = raidNavigationHandlerVtbl{_IUnknownVtbl{NewComProc(raidQuery), NewComProc(raidAddRef), NewComProc(raidRelease)}, NewComProc(raidInvoke)}

func newRaidNavigationHandler(c *Chromium) *raidNavigationHandler {
	return &raidNavigationHandler{vtbl: &raidNavigationHandlerFunctions, impl: c}
}
