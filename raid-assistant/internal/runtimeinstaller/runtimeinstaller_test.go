package runtimeinstaller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRuntimeFlowAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		initial, after                                string
		detectErr, downloadErr, verifyErr, installErr error
		allow                                         bool
		wantStage                                     string
		want                                          []string
	}{
		{"already installed: no network", "140.0.0.1", "", nil, nil, nil, nil, false, "", []string{"detect"}},
		{"missing: download verify ask install then redetect", "", "140.0.0.1", nil, nil, nil, nil, true, "", []string{"detect", "download", "verify", "confirm", "install", "detect", "cleanup"}},
		{"decline install after automatic download", "", "", nil, nil, nil, nil, false, "cancelled", []string{"detect", "download", "verify", "confirm", "cleanup"}},
		{"unknown detection is not absent", "", "", errors.New("access denied"), nil, nil, nil, true, "detect", []string{"detect"}},
		{"network failure never executes", "", "", nil, errors.New("offline"), nil, nil, true, "download", []string{"detect", "download", "cleanup"}},
		{"untrusted signer never executes", "", "", nil, nil, errors.New("bad signature"), nil, true, "verify", []string{"detect", "download", "verify", "cleanup"}},
		{"installation failed", "", "", nil, nil, nil, errors.New("failure"), true, "install", []string{"detect", "download", "verify", "confirm", "install", "detect", "cleanup"}},
		{"install exit success but still absent", "", "", nil, nil, nil, nil, true, "postcheck", []string{"detect", "download", "verify", "confirm", "install", "detect", "cleanup"}},
		{"concurrent installation wins despite installer error", "", "140.0.0.1", nil, nil, nil, errors.New("already installed"), true, "", []string{"detect", "download", "verify", "confirm", "install", "detect", "cleanup"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			counter := 0
			ops := Operations{
				Detect: func() (string, error) {
					calls = append(calls, "detect")
					counter++
					if counter == 1 {
						return tc.initial, tc.detectErr
					}
					return tc.after, nil
				},
				Download: func(context.Context) (string, func(), error) {
					calls = append(calls, "download")
					return "test-setup.exe", func() { calls = append(calls, "cleanup") }, tc.downloadErr
				},
				Verify: func(path string) error {
					if path != "test-setup.exe" {
						t.Fatal(path)
					}
					calls = append(calls, "verify")
					return tc.verifyErr
				},
				Confirm: func() bool { calls = append(calls, "confirm"); return tc.allow },
				Install: func(_ context.Context, path string) error {
					if path != "test-setup.exe" {
						t.Fatal(path)
					}
					calls = append(calls, "install")
					return tc.installErr
				},
				Progress: func(_ string, work func() error) error { return work() },
			}
			version, err := Ensure(context.Background(), ops)
			if tc.wantStage == "" && (err != nil || version == "") {
				t.Fatalf("version %q error %v", version, err)
			}
			if tc.wantStage != "" {
				var f *Failure
				if !errors.As(err, &f) || f.Stage != tc.wantStage {
					t.Fatalf("expected stage %s: %v", tc.wantStage, err)
				}
			}
			if strings.Join(calls, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v want %v", calls, tc.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(req *http.Request, status int, body io.Reader) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(body), Request: req, ContentLength: -1}
}
func TestDownloadMicrosoftOnlyAndCleanup(t *testing.T) {
	good := []byte("MZ\x00\x01fake for test")
	root := t.TempDir()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "go.microsoft.com" {
			r := response(req, 302, bytes.NewReader(nil))
			r.Header.Set("Location", "https://msedge.sf.dl.delivery.mp.microsoft.com/test.exe")
			return r, nil
		}
		return response(req, 200, bytes.NewReader(good)), nil
	})}
	path, cleanup, err := download(context.Background(), client, BootstrapURL, root)
	if err != nil {
		t.Fatal(err)
	}
	if b, e := os.ReadFile(path); e != nil || !bytes.Equal(b, good) {
		t.Fatalf("bad download: %v", e)
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temp folder remains", err)
	}
}
func TestRejectUntrustedRedirectBadContentAndOversize(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   func(*http.Request) *http.Response
	}{
		{"redirect outside Microsoft", func(req *http.Request) *http.Response {
			r := response(req, 302, strings.NewReader(""))
			r.Header.Set("Location", "https://microsoft.com.attacker.test/evil.exe")
			return r
		}},
		{"redirect to HTTP", func(req *http.Request) *http.Response {
			r := response(req, 302, strings.NewReader(""))
			r.Header.Set("Location", "http://go.microsoft.com/test.exe")
			return r
		}},
		{"HTML", func(req *http.Request) *http.Response {
			return response(req, 200, strings.NewReader("<html>not an exe</html>"))
		}},
		{"bad status", func(req *http.Request) *http.Response { return response(req, 500, strings.NewReader("MZ")) }},
		{"stream exceeds limit", func(req *http.Request) *http.Response {
			return response(req, 200, io.MultiReader(strings.NewReader("MZ"), io.LimitReader(zeroReader{}, maxInstallerBytes)))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) { return tc.do(req), nil })}
			_, cleanup, err := download(context.Background(), client, BootstrapURL, root)
			if cleanup != nil {
				cleanup()
			}
			if err == nil {
				t.Fatal("accepted bad installer")
			}
			entries, e := os.ReadDir(root)
			if e != nil || len(entries) != 0 {
				t.Fatalf("temporary files left: %v %v", entries, e)
			}
		})
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func TestMicrosoftHostValidation(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://go.microsoft.com/fwlink": true, "https://MICROSOFT.COM:443/file": true, "https://msedge.sf.dl.delivery.mp.microsoft.com/test": true,
		"http://microsoft.com/x": false, "https://microsoft.com:8443/x": false, "https://microsoft.com@evil.test/x": false,
		"https://evil-microsoft.com/x": false, "https://microsoft.com.evil.test/x": false, "https://user:password@microsoft.com/x": false,
	} {
		u, e := url.Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		if got := allowedMicrosoftURL(u); got != want {
			t.Errorf("%q = %v", raw, got)
		}
	}
}
