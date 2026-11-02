//go:build windows

package webview2

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func TestFailedStartupDestroysWindow(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		// Leave the locked test goroutine at exit, so its WM_QUIT and COM
		// apartment cannot affect another test on a reused OS thread.
		runtime.LockOSThread()
		const title = "ESXi RAID failure cleanup test"
		w, err := NewWithOptionsError(WebViewOptions{DataPath: "invalid\x00path", WindowOptions: WindowOptions{Title: title, Width: 320, Height: 200}})
		if w != nil || err == nil {
			done <- syscall.EINVAL
			return
		}
		name, _ := syscall.UTF16PtrFromString(title)
		hwnd, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(name)))
		if hwnd != 0 {
			done <- syscall.EBUSY
			return
		}
		done <- nil
	}()
	if err := <-done; err != nil {
		t.Fatalf("failed initialization left a window or returned success: %v", err)
	}
}
