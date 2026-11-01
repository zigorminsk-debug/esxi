package inspect

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		input, host, address string
		bad                  bool
	}{
		{"192.168.1.20", "192.168.1.20", "192.168.1.20:443", false},
		{" https://esxi.example:8443/ ", "esxi.example", "esxi.example:8443", false},
		{"2001:db8::1", "2001:db8::1", "[2001:db8::1]:443", false},
		{"[2001:db8::1]:8443", "2001:db8::1", "[2001:db8::1]:8443", false},
		{"", "", "", true}, {"http://host", "", "", true}, {"https://user:pass@host", "", "", true},
		{"host/path", "", "", true}, {"host?token=x", "", "", true}, {"host#fragment", "", "", true},
		{"host:0", "", "", true}, {"host:65536", "", "", true}, {"host:bad", "", "", true},
		{"host\nother", "", "", true}, {"host..example", "", "", true}, {"[fe80::1%eth0]", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			h, a, e := ParseAddress(tt.input)
			if tt.bad {
				if e == nil {
					t.Fatalf("accepted %q as %q", tt.input, a)
				}
				return
			}
			if e != nil || h != tt.host || a != tt.address {
				t.Fatalf("got %q %q %v", h, a, e)
			}
		})
	}
	if got := ManagementURL("2001:db8::1"); got != "https://[2001:db8::1]:35557" {
		t.Fatal(got)
	}
}

func TestTLSPreflightAndPin(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "https://")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cert, err := PeekCertificate(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("preflight sent an HTTP request")
	}
	if cert.Trusted || !cert.ValidTime || len(cert.SHA256) != 64 {
		t.Fatalf("unexpected certificate: %+v", cert)
	}
	check := func(pin string, expect bool) {
		cfg, e := TLSConfig(pin)
		if e != nil {
			t.Fatal(e)
		}
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg}}
		r, e := client.Get(server.URL)
		if r != nil {
			r.Body.Close()
		}
		if (e == nil) != expect {
			t.Fatalf("pin %q: success=%v error=%v", pin, e == nil, e)
		}
	}
	check("", false)
	check(strings.Repeat("0", 64), false)
	check(strings.ToLower(cert.SHA256), true)
	if requests.Load() != 1 {
		t.Fatalf("a failed TLS handshake reached the application: %d", requests.Load())
	}
	if _, err := TLSConfig("not-a-pin"); err == nil {
		t.Fatal("invalid pin accepted")
	}
}

func TestValidateCredentials(t *testing.T) {
	valid := Request{Address: "192.0.2.2", User: "root", Password: "fake-only"}
	if err := ValidateCredentials(valid); err != nil {
		t.Fatal(err)
	}
	for _, req := range []Request{{Address: valid.Address, Password: "x"}, {Address: valid.Address, User: "root"}, {Address: "https://root:bad@host", User: "root", Password: "x"}, {Address: valid.Address, User: "root", Password: "x", Pin: "not a fingerprint"}} {
		if err := ValidateCredentials(req); err == nil {
			t.Fatalf("invalid credentials accepted: %+v", req)
		}
	}
}

func TestPinnedExpiredCertificateIsRejected(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "expired-fixture"}, NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("expired certificate reached HTTP handler") }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	defer server.Close()
	cert, err := PeekCertificate(context.Background(), strings.TrimPrefix(server.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	if cert.ValidTime {
		t.Fatal("expired certificate appeared valid")
	}
	cfg, err := TLSConfig(cert.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg}}
	r, err := client.Get(server.URL)
	if r != nil {
		r.Body.Close()
	}
	if err == nil {
		t.Fatal("expired pinned certificate accepted")
	}
}
