// Package runtimeinstaller checks for WebView2 and obtains Microsoft's online
// installer only when the Runtime is absent. It never touches an ESXi host.
package runtimeinstaller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Microsoft's official Evergreen Bootstrapper link. It is deliberately not
// user-configurable: a downloaded executable is verified before being run.
const BootstrapURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
const maxInstallerBytes int64 = 16 << 20

// Failure identifies the stage without incorporating credentials or a local
// user's path in the text shown on screen.
type Failure struct {
	Stage string
	Err   error
}

func (e *Failure) Error() string { return fmt.Sprintf("%s: %v", e.Stage, e.Err) }
func (e *Failure) Unwrap() error { return e.Err }

var ErrDeclined = errors.New("installation declined")

type Operations struct {
	Detect   func() (string, error)
	Download func(context.Context) (string, func(), error)
	Verify   func(string) error
	Confirm  func() bool
	Install  func(context.Context, string) error
	Progress func(stage string, work func() error) error
}

// Ensure automatically downloads the small installer only if detection says
// the runtime is absent. Installation requires a separate explicit consent.
// After running it, detection is repeated rather than trusting its exit code.
func Ensure(ctx context.Context, ops Operations) (string, error) {
	version, err := ops.Detect()
	if err != nil {
		return "", &Failure{Stage: "detect", Err: err}
	}
	if version != "" {
		return version, nil
	}
	var setup string
	cleanup := func() {}
	defer func() { cleanup() }()
	err = ops.Progress("download", func() error {
		var err error
		setup, cleanup, err = ops.Download(ctx)
		if cleanup == nil {
			cleanup = func() {}
		}
		if err != nil {
			return &Failure{Stage: "download", Err: err}
		}
		if err := ops.Verify(setup); err != nil {
			return &Failure{Stage: "verify", Err: err}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if !ops.Confirm() {
		return "", &Failure{Stage: "cancelled", Err: ErrDeclined}
	}
	installErr := ops.Progress("install", func() error { return ops.Install(ctx, setup) })
	// An installer can fail because another process has just installed it. The
	// registry check, not the installer's exit status, decides whether to run.
	version, err = ops.Detect()
	if err != nil {
		return "", &Failure{Stage: "postcheck", Err: err}
	}
	if version != "" {
		return version, nil
	}
	if installErr != nil {
		return "", &Failure{Stage: "install", Err: installErr}
	}
	return "", &Failure{Stage: "postcheck", Err: errors.New("runtime not detected after setup")}
}

func allowedMicrosoftURL(u *url.URL) bool {
	if u == nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "microsoft.com" || strings.HasSuffix(host, ".microsoft.com")
}

// DownloadOfficial saves the Bootstrapper in a new temporary folder. Redirects
// may only use Microsoft HTTPS hosts; no executable is run here.
func DownloadOfficial(ctx context.Context) (string, func(), error) {
	return download(ctx, &http.Client{Timeout: 2 * time.Minute}, BootstrapURL, "")
}

func download(ctx context.Context, client *http.Client, source, tempRoot string) (string, func(), error) {
	u, err := url.Parse(source)
	if err != nil || !allowedMicrosoftURL(u) {
		return "", nil, errors.New("invalid Microsoft HTTPS download URL")
	}
	cl := *client
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !allowedMicrosoftURL(req.URL) {
			return errors.New("untrusted installer redirect")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "ESXi-RAID-Assistant/0.2.2")
	resp, err := cl.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !allowedMicrosoftURL(resp.Request.URL) {
		return "", nil, fmt.Errorf("installer HTTP status %d or invalid source", resp.StatusCode)
	}
	if resp.ContentLength > maxInstallerBytes {
		return "", nil, errors.New("installer exceeds size limit")
	}
	dir, err := os.MkdirTemp(tempRoot, "esxi-raid-webview2-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	ok := false
	defer func() {
		if !ok {
			cleanup()
		}
	}()
	path := filepath.Join(dir, "MicrosoftEdgeWebview2Setup.exe")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxInstallerBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		return "", nil, copyErr
	}
	if closeErr != nil {
		return "", nil, closeErr
	}
	if n < 2 || n > maxInstallerBytes {
		return "", nil, errors.New("installer is empty or exceeds size limit")
	}
	f, err = os.Open(path)
	if err != nil {
		return "", nil, err
	}
	var magic [2]byte
	_, err = io.ReadFull(f, magic[:])
	_ = f.Close()
	if err != nil || string(magic[:]) != "MZ" {
		return "", nil, errors.New("download is not a Windows executable")
	}
	ok = true
	return path, cleanup, nil
}
