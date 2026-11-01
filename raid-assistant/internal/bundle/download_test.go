package bundle

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testClient(f roundTripFunc) *http.Client                             { c := officialClient(); c.Transport = f; return c }
func response(code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}
}

func TestOfficialDownloadConsentAndSuccess(t *testing.T) {
	vib := fakeVIB("download fixture")
	archive := testZip(t, zipEntry{"esxi8x/lwmd.vib", vib, 0})
	calls := 0
	c := testClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != OfficialFileURL || req.Header.Get("Authorization") != "" || req.URL.User != nil {
			t.Fatalf("unexpected request %v", req.URL)
		}
		return response(200, archive), nil
	})
	dir := filepath.Join(t.TempDir(), "results")
	if _, err := downloadOfficial(context.Background(), false, dir, c, OfficialFileURL, sumBytes(archive)); err == nil || calls != 0 {
		t.Fatal("downloaded without license consent")
	}
	r, err := downloadOfficial(context.Background(), true, dir, c, OfficialFileURL, sumBytes(archive))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || r.HashStatus != "pinned" || r.VibSHA256 != sumBytes(vib) || r.ArchiveName != OfficialFileName {
		t.Fatalf("bad download result %+v calls=%d", r, calls)
	}
	if data, err := os.ReadFile(r.ArchivePath); err != nil || !bytes.Equal(data, archive) {
		t.Fatalf("archive not saved: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temporary file remains: %s", e.Name())
		}
	}
}

func TestOfficialDownloadRejectsRedirectAndBadChecksum(t *testing.T) {
	archive := testZip(t, zipEntry{"lwmd.vib", fakeVIB("a"), 0})
	t.Run("redirect", func(t *testing.T) {
		calls := 0
		c := testClient(func(req *http.Request) (*http.Response, error) {
			calls++
			r := response(302, nil)
			r.Header.Set("Location", "https://attacker.example/steal")
			return r, nil
		})
		if _, err := downloadOfficial(context.Background(), true, t.TempDir(), c, OfficialFileURL, sumBytes(archive)); err == nil || calls != 1 {
			t.Fatalf("redirect followed: calls %d err %v", calls, err)
		}
	})
	t.Run("digest", func(t *testing.T) {
		c := testClient(func(req *http.Request) (*http.Response, error) { return response(200, archive), nil })
		dir := t.TempDir()
		if _, err := downloadOfficial(context.Background(), true, dir, c, OfficialFileURL, strings.Repeat("0", 64)); err == nil {
			t.Fatal("wrong digest accepted")
		}
		files, _ := os.ReadDir(dir)
		if len(files) != 0 {
			t.Fatalf("left files on bad hash: %v", files)
		}
	})
	t.Run("login page", func(t *testing.T) {
		c := testClient(func(req *http.Request) (*http.Response, error) { return response(403, []byte("sign in")), nil })
		if _, err := downloadOfficial(context.Background(), true, t.TempDir(), c, OfficialFileURL, sumBytes(archive)); err == nil {
			t.Fatal("HTTP error accepted")
		}
	})
}
