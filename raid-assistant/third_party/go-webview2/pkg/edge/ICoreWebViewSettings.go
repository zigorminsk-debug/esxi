package edge

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ICoreWebviewSettings is the merged settings class

type _ICoreWebViewSettingsVtbl struct {
	_IUnknownVtbl
	GetIsScriptEnabled                  ComProc
	PutIsScriptEnabled                  ComProc
	GetIsWebMessageEnabled              ComProc
	PutIsWebMessageEnabled              ComProc
	GetAreDefaultScriptDialogsEnabled   ComProc
	PutAreDefaultScriptDialogsEnabled   ComProc
	GetIsStatusBarEnabled               ComProc
	PutIsStatusBarEnabled               ComProc
	GetAreDevToolsEnabled               ComProc
	PutAreDevToolsEnabled               ComProc
	GetAreDefaultContextMenusEnabled    ComProc
	PutAreDefaultContextMenusEnabled    ComProc
	GetAreHostObjectsAllowed            ComProc
	PutAreHostObjectsAllowed            ComProc
	GetIsZoomControlEnabled             ComProc
	PutIsZoomControlEnabled             ComProc
	GetIsBuiltInErrorPageEnabled        ComProc
	PutIsBuiltInErrorPageEnabled        ComProc
	GetUserAgent                        ComProc
	PutUserAgent                        ComProc
	GetAreBrowserAcceleratorKeysEnabled ComProc
	PutAreBrowserAcceleratorKeysEnabled ComProc
	GetIsPasswordAutosaveEnabled        ComProc
	PutIsPasswordAutosaveEnabled        ComProc
	GetIsGeneralAutofillEnabled         ComProc
	PutIsGeneralAutofillEnabled         ComProc
	GetIsPinchZoomEnabled               ComProc
	PutIsPinchZoomEnabled               ComProc
	GetIsSwipeNavigationEnabled         ComProc
	PutIsSwipeNavigationEnabled         ComProc
}

type ICoreWebViewSettings struct {
	vtbl *_ICoreWebViewSettingsVtbl
}

// DisableAutofill uses the actual Settings4 COM interface, rather than
// calling newer vtable slots on the Settings1 pointer. Never persist form data
// or show password-save prompts in this credential-handling application.
func (i *ICoreWebViewSettings) DisableAutofill() error {
	guid := NewGUID("cb56846c-4168-4d53-b04f-03b6d6796ff2")
	var settings4 *ICoreWebViewSettings
	hr, _, _ := i.vtbl.QueryInterface.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(guid)), uintptr(unsafe.Pointer(&settings4)))
	if err := checkHRESULT("QueryInterface Settings4", hr); err != nil {
		return err
	}
	if settings4 == nil {
		return fmt.Errorf("QueryInterface Settings4: empty COM result")
	}
	defer settings4.vtbl.Release.Call(uintptr(unsafe.Pointer(settings4)))
	if hr, _, _ = settings4.vtbl.PutIsPasswordAutosaveEnabled.Call(uintptr(unsafe.Pointer(settings4)), 0); hresultFailed(hr) {
		return checkHRESULT("put_IsPasswordAutosaveEnabled", hr)
	}
	if hr, _, _ = settings4.vtbl.PutIsGeneralAutofillEnabled.Call(uintptr(unsafe.Pointer(settings4)), 0); hresultFailed(hr) {
		return checkHRESULT("put_IsGeneralAutofillEnabled", hr)
	}
	var enabled int32
	if hr, _, _ = settings4.vtbl.GetIsPasswordAutosaveEnabled.Call(uintptr(unsafe.Pointer(settings4)), uintptr(unsafe.Pointer(&enabled))); hresultFailed(hr) {
		return checkHRESULT("get_IsPasswordAutosaveEnabled", hr)
	}
	if enabled != 0 {
		return fmt.Errorf("password autosave is still enabled")
	}
	if hr, _, _ = settings4.vtbl.GetIsGeneralAutofillEnabled.Call(uintptr(unsafe.Pointer(settings4)), uintptr(unsafe.Pointer(&enabled))); hresultFailed(hr) {
		return checkHRESULT("get_IsGeneralAutofillEnabled", hr)
	}
	if enabled != 0 {
		return fmt.Errorf("general autofill is still enabled")
	}
	return nil
}

// Release balances the reference returned by get_Settings.
func (i *ICoreWebViewSettings) Release() {
	i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
}

func (i *ICoreWebViewSettings) AddRef() uintptr {
	r, _, _ := i.vtbl.AddRef.Call()
	return r
}

func (i *ICoreWebViewSettings) GetIsScriptEnabled() (bool, error) {
	var err error
	var isScriptEnabled bool
	_, _, err = i.vtbl.GetIsScriptEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&isScriptEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return isScriptEnabled, nil
}

func (i *ICoreWebViewSettings) PutIsScriptEnabled(isScriptEnabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsScriptEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(isScriptEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetIsWebMessageEnabled() (bool, error) {
	var err error
	var isWebMessageEnabled bool
	_, _, err = i.vtbl.GetIsWebMessageEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&isWebMessageEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return isWebMessageEnabled, nil
}

func (i *ICoreWebViewSettings) PutIsWebMessageEnabled(isWebMessageEnabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsWebMessageEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(isWebMessageEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetAreDefaultScriptDialogsEnabled() (bool, error) {
	var err error
	var areDefaultScriptDialogsEnabled bool
	_, _, err = i.vtbl.GetAreDefaultScriptDialogsEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&areDefaultScriptDialogsEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return areDefaultScriptDialogsEnabled, nil
}

func (i *ICoreWebViewSettings) PutAreDefaultScriptDialogsEnabled(areDefaultScriptDialogsEnabled bool) error {
	var err error

	_, _, err = i.vtbl.PutAreDefaultScriptDialogsEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(areDefaultScriptDialogsEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetIsStatusBarEnabled() (bool, error) {
	var err error
	var isStatusBarEnabled bool
	_, _, err = i.vtbl.GetIsStatusBarEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&isStatusBarEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return isStatusBarEnabled, nil
}

func (i *ICoreWebViewSettings) PutIsStatusBarEnabled(isStatusBarEnabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsStatusBarEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(isStatusBarEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetAreDevToolsEnabled() (bool, error) {
	var err error
	var areDevToolsEnabled bool
	_, _, err = i.vtbl.GetAreDevToolsEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&areDevToolsEnabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return areDevToolsEnabled, nil
}

func (i *ICoreWebViewSettings) PutAreDevToolsEnabled(areDevToolsEnabled bool) error {
	hr, _, _ := i.vtbl.PutAreDevToolsEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(areDevToolsEnabled)),
	)
	return checkHRESULT("put_AreDevToolsEnabled", hr)
}

func (i *ICoreWebViewSettings) GetAreDefaultContextMenusEnabled() (bool, error) {
	var err error
	var enabled bool
	_, _, err = i.vtbl.GetAreDefaultContextMenusEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return enabled, nil
}

func (i *ICoreWebViewSettings) PutAreDefaultContextMenusEnabled(enabled bool) error {
	hr, _, _ := i.vtbl.PutAreDefaultContextMenusEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(enabled)),
	)
	return checkHRESULT("put_AreDefaultContextMenusEnabled", hr)
}

func (i *ICoreWebViewSettings) GetAreHostObjectsAllowed() (bool, error) {
	var err error
	var allowed bool
	_, _, err = i.vtbl.GetAreHostObjectsAllowed.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&allowed)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return allowed, nil
}

func (i *ICoreWebViewSettings) PutAreHostObjectsAllowed(allowed bool) error {
	var err error

	_, _, err = i.vtbl.PutAreHostObjectsAllowed.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(allowed)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetIsZoomControlEnabled() (bool, error) {
	var err error
	var enabled bool
	_, _, err = i.vtbl.GetIsZoomControlEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return enabled, nil
}

func (i *ICoreWebViewSettings) PutIsZoomControlEnabled(enabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsZoomControlEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetIsBuiltInErrorPageEnabled() (bool, error) {
	var err error
	var enabled bool
	_, _, err = i.vtbl.GetIsBuiltInErrorPageEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return enabled, nil
}

func (i *ICoreWebViewSettings) PutIsBuiltInErrorPageEnabled(enabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsBuiltInErrorPageEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetUserAgent() (string, error) {
	var err error
	// Create *uint16 to hold result
	var _userAgent *uint16
	_, _, err = i.vtbl.GetUserAgent.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(_userAgent)),
	)
	if err != windows.ERROR_SUCCESS {
		return "", err
	} // Get result and cleanup
	userAgent := windows.UTF16PtrToString(_userAgent)
	windows.CoTaskMemFree(unsafe.Pointer(_userAgent))
	return userAgent, nil
}

func (i *ICoreWebViewSettings) PutUserAgent(userAgent string) error {
	var err error
	// Convert string 'userAgent' to *uint16
	_userAgent, err := windows.UTF16PtrFromString(userAgent)
	if err != nil {
		return err
	}

	_, _, err = i.vtbl.PutUserAgent.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(_userAgent)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetAreBrowserAcceleratorKeysEnabled() (bool, error) {
	var err error
	var enabled bool
	_, _, err = i.vtbl.GetAreBrowserAcceleratorKeysEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return enabled, nil
}

func (i *ICoreWebViewSettings) PutAreBrowserAcceleratorKeysEnabled(enabled bool) error {
	var err error

	_, _, err = i.vtbl.PutAreBrowserAcceleratorKeysEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetIsPinchZoomEnabled() (bool, error) {
	var err error
	var enabled bool
	_, _, err = i.vtbl.GetIsPinchZoomEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return enabled, nil
}

func (i *ICoreWebViewSettings) PutIsPinchZoomEnabled(enabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsPinchZoomEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebViewSettings) GetIsSwipeNavigationEnabled() (bool, error) {
	var err error
	var enabled bool
	_, _, err = i.vtbl.GetIsSwipeNavigationEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return enabled, nil
}

func (i *ICoreWebViewSettings) PutIsSwipeNavigationEnabled(enabled bool) error {
	var err error

	_, _, err = i.vtbl.PutIsSwipeNavigationEnabled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(enabled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}
