//go:build windows

package edge

import "fmt"

// HRESULT is a signed 32-bit value even when passed in a 64-bit register.
// COM methods do not use the Win32 thread's GetLastError as their result.
type HRESULTError struct {
	Operation string
	Code      uint32
}

func (e *HRESULTError) Error() string {
	return fmt.Sprintf("%s: HRESULT 0x%08X", e.Operation, e.Code)
}

func hresultFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func checkHRESULT(operation string, hr uintptr) error {
	if hresultFailed(hr) {
		return &HRESULTError{Operation: operation, Code: uint32(hr)}
	}
	return nil
}
