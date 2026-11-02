//go:build windows

package main

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/runtimeinstaller"
	"golang.org/x/sys/windows"
)

func TestRuntimeVersionParsing(t *testing.T) {
	for v, want := range map[string]bool{"140.0.7339.0": true, "109.0.0.1": true, "": false, "1.2.3": false, "1.2.3.4.5": false, "1.2.x.4": false, "1.2.3.-4": false, "4294967296.0.0.0": false} {
		if got := runtimeVersionValid(v); got != want {
			t.Errorf("%s = %v", v, got)
		}
	}
}
func TestMicrosoftSignerIdentity(t *testing.T) {
	for _, tc := range []struct {
		name pkix.Name
		want bool
	}{
		{pkix.Name{CommonName: "Microsoft Corporation", Organization: []string{"Microsoft Corporation"}}, true},
		{pkix.Name{CommonName: "Microsoft Corporation", Organization: []string{"Evil"}}, false},
		{pkix.Name{CommonName: "Evil, O=Microsoft Corporation", Organization: []string{"Evil"}}, false},
		{pkix.Name{CommonName: "Microsoft Corporation", Organization: []string{"Microsoft Corporation", "Evil"}}, false},
		{pkix.Name{CommonName: "Other", Organization: []string{"Microsoft Corporation"}}, false},
	} {
		if got := isMicrosoftSigner(&x509.Certificate{Subject: tc.name}); got != tc.want {
			t.Errorf("%+v = %v", tc.name, got)
		}
	}
}
func TestSignatureRejectsUnsignedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.exe")
	if err := os.WriteFile(path, []byte("MZnot an authentic executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyMicrosoftSetup(path); err == nil {
		t.Fatal("accepted unsigned executable")
	}
}
func TestRealOfficialBootstrapperSignature(t *testing.T) {
	if os.Getenv("RAID_TEST_REAL_BOOTSTRAPPER") != "1" {
		t.Skip("official network check disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	path, cleanup, err := runtimeinstaller.DownloadOfficial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := verifyMicrosoftSetup(path); err != nil {
		t.Fatalf("official bootstrapper failed signature verification: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8192 {
		t.Fatal("installer unexpectedly small")
	}
	data[4096] ^= 1
	altered := filepath.Join(t.TempDir(), "modified.exe")
	if err := os.WriteFile(altered, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyMicrosoftSetup(altered); err == nil {
		t.Fatal("modified bootstrapper passed signature verification")
	}
	if os.Getenv("RAID_TEST_REAL_INSTALLER") == "1" {
		// This GitHub-hosted runner already has the Runtime. Exercise the real
		// Microsoft process and our wait/exit path, but do not claim that a
		// clean machine without WebView2 has been tested.
		err := installMicrosoftSetup(ctx, path)
		if err != nil && !strings.HasPrefix(err.Error(), "installer exit code") {
			t.Fatalf("could not launch or wait for the installer: %v", err)
		}
		version, detectErr := detectRuntime()
		if detectErr != nil || version == "" {
			t.Fatalf("Runtime not detected after test installer: %q %v", version, detectErr)
		}
		t.Logf("Microsoft installer exited (error=%v), installed Runtime %s remains available", err, version)
	}
}
func TestNativeProgressAndEmbeddedIcon(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		t.Fatal(err)
	}
	icon, _, _ := user32.NewProc("LoadImageW").Call(uintptr(instance), 1, 1, 0, 0, 0x40|0x8000)
	if icon == 0 {
		t.Fatal("application icon resource 1 missing")
	}
	want := errors.New("work result")
	err := runtimeProgress("download", func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("progress did not return worker error: %v", err)
	}
}
func TestStartupMessageIncludesHRESULTButNotPrivatePath(t *testing.T) {
	text := webviewFailureText("123.4.5.6", &edge.HRESULTError{Operation: "get_Settings", Code: 0x80070005})
	if !strings.Contains(text, "0x80070005") || !strings.Contains(text, "get_Settings") || !strings.Contains(text, "123.4.5.6") {
		t.Fatal(text)
	}
	text = webviewFailureText("1.2.3.4", errors.New(`WebView2Loader: C:\Users\PrivateUser\secret`))
	if strings.Contains(text, "PrivateUser") || strings.Contains(text, "secret") {
		t.Fatal("message exposed private path")
	}
}
