package main

import (
	"log"
	"runtime"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/app"
)

func main() {
	// Win32 and WebView2 require their owning thread to stay fixed.
	runtime.LockOSThread()
	a, err := app.New()
	if err != nil {
		log.Fatal(err)
	}
	if err := runWindow(a); err != nil {
		log.Fatal(err)
	}
}
