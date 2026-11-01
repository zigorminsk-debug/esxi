package inspect

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestPortProbeNoCredentialsOrRedirect(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials sent")
		}
		http.Redirect(w, r, "https://example.com", http.StatusFound)
	}))
	defer server.Close()
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := ProbePort(ctx, host, n)
	if !p.TCP || !p.TLS || p.Trusted || len(p.CertSHA256) != 64 || p.HTTPStatus != 302 || count.Load() != 1 {
		t.Fatalf("%+v count=%d", p, count.Load())
	}
	_ = tls.VersionTLS12
}
func TestTroubleshootUnknownNotInstalled(t *testing.T) {
	s := Troubleshoot(&Host{}, nil)
	if s[0].Status != "unknown" || s[1].Status != "unknown" || s[2].Status != "unknown" {
		t.Fatalf("%+v", s)
	}
}
