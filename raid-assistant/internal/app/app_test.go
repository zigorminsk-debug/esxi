package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	return &App{jobs: make(map[string]Job), outputDir: t.TempDir()}
}
func waitForJob(t *testing.T, a *App, id string) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := a.Poll(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State != "running" {
			return j
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job timeout")
	return Job{}
}
func TestJobsAreRemovedAfterPoll(t *testing.T) {
	a := newTestApp(t)
	id, err := a.start(func() (any, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	j := waitForJob(t, a, id)
	if j.State != "done" || j.Data != "ok" {
		t.Fatalf("job: %+v", j)
	}
	if _, err = a.Poll(id); err == nil {
		t.Fatal("job was not removed")
	}
	id, err = a.start(func() (any, error) { return nil, errors.New("expected fixture error") })
	if err != nil {
		t.Fatal(err)
	}
	j = waitForJob(t, a, id)
	if j.State != "error" || !strings.Contains(j.Error, "fixture") {
		t.Fatalf("job error: %+v", j)
	}
}
func TestGuardsAndLinkAllowlist(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.Report(ReportOptions{}); err == nil {
		t.Fatal("report without host")
	}
	if _, err := a.StartDownload(false); err == nil {
		t.Fatal("download without license")
	}
	if _, err := a.StartDownload(true); err == nil {
		t.Fatal("download without candidate")
	}
	if _, err := a.StartImport("file.zip"); err == nil {
		t.Fatal("import without candidate")
	}
	if _, err := a.Link("https://evil.example/"); err == nil {
		t.Fatal("arbitrary link allowed")
	}
	a.host = &inspect.Host{Address: "192.0.2.10:443", DisplayHost: "192.0.2.10", Candidate: &inspect.Candidate{ProductURL: "https://www.broadcom.com/model", DownloadURL: "https://docs.broadcom.com/terms", ReadmeURL: "https://docs.broadcom.com/readme"}}
	link, err := a.Link("management")
	if err != nil || link != "https://192.0.2.10:35557" {
		t.Fatalf("management link %q %v", link, err)
	}
	if _, err = a.Report(ReportOptions{AccessIP: "0.0.0.0/0"}); err == nil {
		t.Fatal("worldwide firewall CIDR accepted")
	}
	if _, err = a.Report(ReportOptions{AccessIP: "192.0.2.10; reboot"}); err == nil {
		t.Fatal("injected IP accepted")
	}
}
func TestImportAndSaveAreLocalAndCredentialFree(t *testing.T) {
	a := newTestApp(t)
	a.host = &inspect.Host{Address: "192.0.2.10:443", DisplayHost: "192.0.2.10", Name: "fixture", Candidate: &inspect.Candidate{Model: "fixture"}}
	source := filepath.Join(t.TempDir(), "test_lwmd.vib")
	if err := os.WriteFile(source, []byte("!<arch>\nfixture"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := a.StartImport(source)
	if err != nil {
		t.Fatal(err)
	}
	j := waitForJob(t, a, id)
	if j.State != "done" {
		t.Fatalf("import %+v", j)
	}
	filename, err := a.SaveReport(ReportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "192.0.2.10") {
		t.Fatal("report does not reference host")
	}
	if strings.Contains(strings.ToLower(string(content)), "password") {
		t.Fatal("report contains credential field")
	}
	if filepath.Dir(filename) != a.outputDir {
		t.Fatal("report escaped output directory")
	}
}

func TestExtensionLinksAreFixedAndAvailableBeforeLogin(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"ext-storcli", "ext-storcli2", "ext-hpe", "ext-dell", "ext-depot", "ext-vgpu", "ext-cdp"} {
		link, err := a.Link(kind)
		if err != nil || !strings.HasPrefix(link, "https://") {
			t.Fatalf("%s: %s %v", kind, link, err)
		}
	}
	if _, err := a.Link("https://attacker.invalid/"); err == nil {
		t.Fatal("arbitrary link was allowed")
	}
}
