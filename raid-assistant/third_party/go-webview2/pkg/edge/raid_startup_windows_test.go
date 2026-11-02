//go:build windows

package edge

import (
	"errors"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"
)

// A successful COM call can leave an unrelated Win32 last-error. Conversely
// a failed COM call can leave ERROR_SUCCESS. Verify the HRESULT, not errno.
func TestSettingsHRESULTInsteadOfLastError(t *testing.T) {
	lastError := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError")
	var hr uintptr
	var stale uintptr = 5
	settings := &ICoreWebViewSettings{}
	get := windows.NewCallback(func(_ uintptr, out **ICoreWebViewSettings) uintptr {
		*out = settings
		lastError.Call(stale)
		return hr
	})
	set := windows.NewCallback(func(_, _ uintptr) uintptr {
		lastError.Call(stale)
		return hr
	})
	web := &ICoreWebView2{vtbl: &iCoreWebView2Vtbl{GetSettings: ComProc(get)}}
	settings.vtbl = &_ICoreWebViewSettingsVtbl{PutAreDefaultContextMenusEnabled: ComProc(set), PutAreDevToolsEnabled: ComProc(set)}
	if raw, _, errno := ComProc(set).Call(0, 0); raw != 0 || !errors.Is(errno, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("fixture did not leave the intended last error: HRESULT %x errno %v", raw, errno)
	}
	if got, err := web.GetSettings(); err != nil || got != settings {
		t.Fatalf("stale last-error broke get_Settings: %v", err)
	}
	if err := settings.PutAreDefaultContextMenusEnabled(false); err != nil {
		t.Fatalf("stale last-error broke menu setter: %v", err)
	}
	if err := settings.PutAreDevToolsEnabled(false); err != nil {
		t.Fatalf("stale last-error broke devtools setter: %v", err)
	}
	hr, stale = 0x80004005, 0
	for _, call := range []func() error{
		func() error { _, err := web.GetSettings(); return err },
		func() error { return settings.PutAreDefaultContextMenusEnabled(false) },
		func() error { return settings.PutAreDevToolsEnabled(false) },
	} {
		var native *HRESULTError
		if err := call(); !errors.As(err, &native) || native.Code != 0x80004005 {
			t.Fatalf("COM failure not propagated: %v", err)
		}
	}
	hr = 1 // S_FALSE is still a successful HRESULT
	if _, err := web.GetSettings(); err != nil {
		t.Fatalf("S_FALSE treated as failure: %v", err)
	}
}

func TestSecuritySettingsFailClosed(t *testing.T) {
	var failGeneral, forceEnabled bool
	var releases int
	var four *ICoreWebViewSettings
	query := windows.NewCallback(func(_ uintptr, _ *windows.GUID, out **ICoreWebViewSettings) uintptr { *out = four; return 0 })
	release := windows.NewCallback(func(_ uintptr) uintptr { releases++; return 1 })
	passwordPut := windows.NewCallback(func(_, value uintptr) uintptr {
		if value != 0 {
			t.Error("password autosave enabled")
		}
		return 0
	})
	generalPut := windows.NewCallback(func(_, value uintptr) uintptr {
		if value != 0 {
			t.Error("autofill enabled")
		}
		if failGeneral {
			return 0x80004005
		}
		return 0
	})
	get := windows.NewCallback(func(_ uintptr, enabled *int32) uintptr {
		if forceEnabled {
			*enabled = 1
		} else {
			*enabled = 0
		}
		return 0
	})
	four = &ICoreWebViewSettings{vtbl: &_ICoreWebViewSettingsVtbl{_IUnknownVtbl: _IUnknownVtbl{Release: ComProc(release)}, PutIsPasswordAutosaveEnabled: ComProc(passwordPut), PutIsGeneralAutofillEnabled: ComProc(generalPut), GetIsPasswordAutosaveEnabled: ComProc(get), GetIsGeneralAutofillEnabled: ComProc(get)}}
	base := &ICoreWebViewSettings{vtbl: &_ICoreWebViewSettingsVtbl{_IUnknownVtbl: _IUnknownVtbl{QueryInterface: ComProc(query)}}}
	if err := base.DisableAutofill(); err != nil {
		t.Fatal(err)
	}
	failGeneral = true
	if err := base.DisableAutofill(); err == nil {
		t.Fatal("failed setter did not stop startup")
	}
	failGeneral = false
	forceEnabled = true
	if err := base.DisableAutofill(); err == nil {
		t.Fatal("enabled security setting did not stop startup")
	}
	if releases != 3 {
		t.Fatalf("Settings4 references not released: %d", releases)
	}
}

func TestCallbacksInterpret32BitFailureAndNilObjects(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Chromium)
	}{
		{"environment failure", func(c *Chromium) { c.EnvironmentCompleted(uintptr(0x80070005), nil) }},
		{"controller failure", func(c *Chromium) { c.CreateCoreWebView2ControllerCompleted(uintptr(0x80004005), nil) }},
		{"environment success without object", func(c *Chromium) { c.EnvironmentCompleted(0, nil) }},
		{"controller success without object", func(c *Chromium) { c.CreateCoreWebView2ControllerCompleted(0, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewChromium()
			tc.call(c)
			if c.initErr == nil || atomic.LoadUintptr(&c.inited) == 0 || atomic.LoadUint32(&c.initAborted) == 0 {
				t.Fatalf("error not recorded: %v", c.initErr)
			}
			c.EnvironmentCompleted(0, nil)
			if c.initErr == nil {
				t.Fatal("late callback cleared error")
			}
		})
	}
}

func TestPermissionReadUsesActualKindPointer(t *testing.T) {
	var state uintptr
	get := windows.NewCallback(func(_ uintptr, kind *CoreWebView2PermissionKind) uintptr {
		*kind = CoreWebView2PermissionKindClipboardRead
		return 0
	})
	put := windows.NewCallback(func(_, value uintptr) uintptr { state = value; return 0 })
	args := &iCoreWebView2PermissionRequestedEventArgs{vtbl: &iCoreWebView2PermissionRequestedEventArgsVtbl{GetPermissionKind: ComProc(get), PutState: ComProc(put)}}
	c := NewChromium()
	c.SetPermission(CoreWebView2PermissionKindClipboardRead, CoreWebView2PermissionStateDeny)
	c.PermissionRequested(nil, args)
	if state != uintptr(CoreWebView2PermissionStateDeny) {
		t.Fatalf("clipboard read not denied: %d", state)
	}
}
