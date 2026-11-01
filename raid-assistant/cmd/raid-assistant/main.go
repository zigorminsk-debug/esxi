package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/app"
)

func selfTestTrace(stage string) {
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

func main() {
	selfTestTrace("main started")
	// Win32 and WebView2 require their owning thread to stay fixed.
	runtime.LockOSThread()
	a, err := app.New()
	selfTestTrace("session initialized")
	if err != nil {
		log.Fatal(err)
	}
	err = runWindow(a)
	a.Close() // stop monitoring and release any in-memory SSH credentials
	if err != nil {
		log.Fatal(err)
	}
}
