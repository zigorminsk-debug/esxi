package app

import (
	"crypto/tls"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/vmware/govmomi/simulator"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
)

// End-to-end native job flow against a local simulator, never a real ESXi host.
func TestSimulatedConnectionJobs(t *testing.T) {
	m := simulator.ESX()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	m.Service.TLS = &tls.Config{}
	m.Service.Listen = &url.URL{Host: "127.0.0.1:0", User: url.UserPassword("fixture-user", "fixture-password")}
	s := m.Service.NewServer()
	defer s.Close()
	a := newTestApp(t)
	defer a.Close()

	id, err := a.StartCertificate(s.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	job := waitForJob(t, a, id)
	cert, ok := job.Data.(inspect.Certificate)
	if job.State != "done" || !ok || cert.Trusted || !cert.ValidTime {
		t.Fatalf("unexpected preflight: %+v", job)
	}
	req := inspect.Request{Address: s.URL.Host, User: "fixture-user", Password: "wrong-fixture-password", Pin: cert.SHA256}
	id, err = a.StartScan(req)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForJob(t, a, id)
	if job.State != "error" || !strings.Contains(job.Error, "логин, пароль") || strings.Contains(job.Error, req.Password) {
		t.Fatalf("unexpected login error: %+v", job)
	}
	if _, err := a.Diagnostics(); err == nil {
		t.Fatal("failed login retained a host")
	}

	req.Password = "fixture-password"
	id, err = a.StartScan(req)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForJob(t, a, id)
	host, ok := job.Data.(*inspect.Host)
	if job.State != "done" || !ok || host.Name == "" {
		t.Fatalf("host not returned: %+v", job)
	}
	result, err := json.Marshal(job)
	if err != nil || strings.Contains(string(result), req.Password) || strings.Contains(string(result), "wrong-fixture-password") {
		t.Fatal("credentials included in result")
	}
	a.Invalidate()
	if _, err := a.Diagnostics(); err == nil {
		t.Fatal("old host remained after invalidation")
	}
}
