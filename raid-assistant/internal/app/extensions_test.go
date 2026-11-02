package app

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/raid"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/remoteread"
)

func TestRaidImportRequiresHostConfirmationAndInvalidates(t *testing.T) {
	a := newTestApp(t)
	const n = "naa.600e00040010f26027ad7d3730ecb7ad"
	a.host = &inspect.Host{DisplayHost: "192.0.2.10", Datastores: []inspect.Datastore{{Name: "ds", Extents: []string{n}, VMs: []string{"vm"}}}}
	filename := filepath.Join(t.TempDir(), "report.json")
	data := `{"Controllers":[{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{"VD LIST":[{"DG/VD":"0/0","State":"Dgrd","NAA":"600e00040010f26027ad7d3730ecb7ad"}]}}]}`
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := a.StartRaidImport(filename)
	if err != nil {
		t.Fatal(err)
	}
	if j := waitForJob(t, a, id); j.State != "done" {
		t.Fatalf("import: %+v", j)
	}
	if _, err := a.Correlate(false); err == nil {
		t.Fatal("unconfirmed import linked")
	}
	links, err := a.Correlate(true)
	if err != nil || len(links.Links) != 1 || links.Links[0].Datastore != "ds" {
		t.Fatalf("links: %+v %v", links, err)
	}
	history, err := a.History()
	if err != nil || len(history) != 1 || history[0].Severity != "warning" {
		t.Fatalf("history: %+v %v", history, err)
	}
	a.Invalidate()
	if _, err := a.Correlate(true); err == nil {
		t.Fatal("stale host linked")
	}
}

func TestMonitoringPortAlertAndCancellation(t *testing.T) {
	a := newTestApp(t)
	defer a.Close()
	a.host = &inspect.Host{DisplayHost: "192.0.2.10"}
	a.extra.monitorProbe = func(ctx context.Context, host string, port int) inspect.PortProbe { return inspect.PortProbe{} }
	notified := make(chan string, 1)
	a.SetNotifier(func(title, message string) { notified <- message })
	if _, err := a.StartMonitor(MonitorOptions{Mode: "port", Interval: 2}); err == nil {
		t.Fatal("short interval accepted")
	}
	if _, err := a.StartMonitor(MonitorOptions{Mode: "port", Interval: 60, Notify: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-notified:
		if !strings.Contains(msg, "35557") {
			t.Fatal(msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("alert timeout")
	}
	m := a.Monitor()
	if !m.Running || m.Sequence != 1 || m.AlertSequence != 1 || m.Severity != "warning" {
		t.Fatalf("monitor: %+v", m)
	}
	events, err := a.History()
	if err != nil || len(events) != 1 || events[0].Type != "Мониторинг port" {
		t.Fatalf("history %+v %v", events, err)
	}
	if stopped := a.StopMonitor(); stopped.Running {
		t.Fatal("monitor did not stop")
	}
}

func TestSSHMonitorStopDoesNotAllowOverlappingReads(t *testing.T) {
	a := newTestApp(t)
	defer a.Close()
	a.host = &inspect.Host{DisplayHost: "192.0.2.10"}
	a.extra.key = &remoteread.Key{Fingerprint: "SHA256:test"}
	started, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	a.extra.monitorRead = func(ctx context.Context, host string, req remoteread.Request) (*raid.Snapshot, error) {
		close(started)
		<-ctx.Done()
		<-release
		return nil, ctx.Err()
	}
	req := SSHOptions{Request: remoteread.Request{User: "root", Password: "never-save-this", Fingerprint: "SHA256:test", Kind: "storcli"}, Consent: true}
	if _, err := a.StartMonitor(MonitorOptions{Mode: "ssh", Interval: 60, SSHOptions: req}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor read did not start")
	}
	a.StopMonitor()
	if _, err := a.StartSSHRead(req); err == nil || !strings.Contains(err.Error(), "ещё выполняется") {
		t.Fatalf("overlapping read accepted: %v", err)
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.mu.Lock()
		active := a.extra.sshInFlight
		a.mu.Unlock()
		if !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SSH gate was not released")
		}
		time.Sleep(time.Millisecond)
	}
	entries, err := os.ReadDir(a.extra.configDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		b, _ := os.ReadFile(filepath.Join(a.extra.configDir, entry.Name()))
		if strings.Contains(string(b), "never-save-this") {
			t.Fatal("password saved")
		}
	}
}

func TestCustomScreenshotImportReplaceClearAndReject(t *testing.T) {
	a := newTestApp(t)
	p := filepath.Join(t.TempDir(), "screen.png")
	writePNG := func(c color.Color) {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		im := image.NewRGBA(image.Rect(0, 0, 2, 2))
		im.Set(0, 0, c)
		if err = png.Encode(f, im); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(color.RGBA{R: 255, A: 255})
	one, err := a.ImportScreenshot(p)
	if err != nil || !one.Custom || !strings.HasPrefix(one.Data, "data:image/png;base64,") {
		t.Fatalf("image %+v %v", one, err)
	}
	writePNG(color.RGBA{G: 255, A: 255})
	two, err := a.ImportScreenshot(p)
	if err != nil || two.Data == one.Data {
		t.Fatalf("replace %v", err)
	}
	loaded, err := a.LoadScreenshot()
	if err != nil || loaded.Data != two.Data {
		t.Fatalf("load %v", err)
	}
	if err = a.ClearScreenshot(); err != nil {
		t.Fatal(err)
	}
	loaded, err = a.LoadScreenshot()
	if err != nil || loaded.Custom {
		t.Fatalf("clear %+v %v", loaded, err)
	}
	if err = os.WriteFile(p, []byte(`<svg onload="alert(1)"></svg>`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ImportScreenshot(p); err == nil {
		t.Fatal("SVG accepted")
	}
}

func TestInvalidScanClearsPreviousHostAndMonitorSample(t *testing.T) {
	a := newTestApp(t)
	a.host = &inspect.Host{DisplayHost: "192.0.2.10"}
	a.extra.snapshot = &raid.Snapshot{Source: "SSH"}
	a.extra.monitor = MonitorInfo{Running: true, Mode: "ssh", Sequence: 2, AlertSequence: 1, Snapshot: a.extra.snapshot}
	if _, err := a.StartScan(inspect.Request{}); err == nil {
		t.Fatal("invalid scan accepted")
	}
	if a.host != nil || a.extra.snapshot != nil {
		t.Fatal("old host or snapshot remained")
	}
	m := a.Monitor()
	if m.Running || m.Snapshot != nil || m.Sequence != 0 || m.AlertSequence != 0 {
		t.Fatalf("old sample remained: %+v", m)
	}
}

func TestFlatWindowsMonitorRPCDecodesSSHFields(t *testing.T) {
	var opts MonitorOptions
	payload := []byte(`{"mode":"ssh","interval":300,"notify":true,"user":"root","password":"fixture","fingerprint":"SHA256:test","kind":"storcli","consent":true}`)
	if err := json.Unmarshal(payload, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.Mode != "ssh" || opts.User != "root" || opts.Password != "fixture" || opts.Kind != "storcli" || opts.Fingerprint != "SHA256:test" || !opts.Consent {
		t.Fatalf("RPC fields lost: %+v", opts)
	}
}
