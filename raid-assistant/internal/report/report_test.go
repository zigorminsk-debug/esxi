package report

import (
	"strings"
	"testing"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/bundle"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
)

func fixtureHost() *inspect.Host {
	return &inspect.Host{Address: "192.0.2.10:443", DisplayHost: "192.0.2.10", Name: "esxi-test", Version: "8.0.3", Build: "123", Vendor: "example", Model: "example", Datastores: []inspect.Datastore{{Name: "Data O'Brien", Accessible: true}}, Candidate: &inspect.Candidate{Model: "MegaRAID 9560-8i", Version: "fixture", ProductURL: "https://www.broadcom.com/", ReadmeURL: "https://docs.broadcom.com/", DownloadURL: "https://docs.broadcom.com/"}}
}
func fixtureBundle() *bundle.Result {
	return &bundle.Result{ArchiveName: "archive.zip", SHA256: strings.Repeat("a", 64), VibName: "BCM_bootbank_lwmd_1.vib", VibSHA256: strings.Repeat("b", 64), VibPath: `C:\Users\test\Downloads\BCM_bootbank_lwmd_1.vib`, Notice: "Unverified fixture"}
}
func commands(r Report) []string {
	var result []string
	for _, s := range r.Steps {
		result = append(result, s.Commands...)
	}
	return result
}
func commandText(r Report) string { return strings.Join(commands(r), "\n") }

func TestReportStopsForUnknownPackageAndMissingBundle(t *testing.T) {
	h := fixtureHost()
	h.Candidate = nil
	h.CandidateReason = "OEM 1028:1 не подтверждён"
	r := Build(h, fixtureBundle(), "Data O'Brien", "192.0.2.1")
	if !strings.Contains(r.Markdown, h.CandidateReason) || strings.Contains(commandText(r), "vib install") || strings.Contains(commandText(r), "firewall ruleset set") {
		t.Fatal("unsafe commands for unknown candidate")
	}
	h.Candidate = fixtureHost().Candidate
	for _, b := range []*bundle.Result{nil, {VibName: "../lwmd.vib", VibSHA256: strings.Repeat("b", 64)}, {VibName: "lwmd.vib", VibSHA256: "not-a-hash"}} {
		r = Build(h, b, "Data O'Brien", "192.0.2.1")
		if strings.Contains(commandText(r), "vib install") {
			t.Fatalf("unsafe VIB path or hash: %+v", b)
		}
	}
}

func TestReportCommandsAreBoundedAndQuoted(t *testing.T) {
	h := fixtureHost()
	b := fixtureBundle()
	r := Build(h, b, "Data O'Brien", "192.0.2.129/24")
	all := commandText(r)
	for _, want := range []string{"/vmfs/volumes/Data O'\\''Brien/BCM_bootbank_lwmd_1.vib'", "--dry-run", "esxcli software vib signature verify", "-i '192.0.2.0/24'", "--allowed-all false", "esxcli daemon control start -s lwmd"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %s", want, all)
		}
	}
	if strings.Contains(all, "--no-sig-check") || strings.Contains(all, "--force") || strings.Contains(all, "allowed-all true") {
		t.Fatal("unsafe bypass was generated")
	}
	if strings.Index(all, "--allowed-all false") > strings.Index(all, "allowedip add") || strings.Index(all, "allowedip add") > strings.Index(all, "-e true") {
		t.Fatal("unsafe firewall ordering")
	}
	if !strings.Contains(r.Markdown, "https://192.0.2.10:35557") || !strings.Contains(r.Markdown, "перезагруз") {
		t.Fatal("missing port or reboot guidance")
	}
}

func TestNoCommandsForUnknownOrMaliciousDatastores(t *testing.T) {
	h := fixtureHost()
	b := fixtureBundle()
	if strings.Contains(commandText(Build(h, b, "not-found", "")), "vib install") {
		t.Fatal("unknown datastore accepted")
	}
	for _, ds := range []string{"../etc", "name/escape", "name\nmalicious", "bad`name"} {
		h.Datastores = append(h.Datastores, inspect.Datastore{Name: ds, Accessible: true})
		r := Build(h, b, ds, "")
		if strings.Contains(commandText(r), "vib install") {
			t.Fatalf("unsafe datastore %q accepted", ds)
		}
	}
	h.Datastores = []inspect.Datastore{{Name: "offline", Accessible: false}}
	if strings.Contains(commandText(Build(h, b, "offline", "")), "vib install") {
		t.Fatal("inaccessible datastore accepted")
	}
}
func TestAccessIP(t *testing.T) {
	for input, want := range map[string]string{"192.0.2.1": "192.0.2.1", "192.0.2.15/24": "192.0.2.0/24", "2001:db8::1/64": "2001:db8::/64"} {
		got, ok := NormalizeAccessIP(input)
		if !ok || got != want {
			t.Fatalf("%s -> %s %v", input, got, ok)
		}
	}
	for _, bad := range []string{"", "0.0.0.0/0", "::/0", "0.0.0.0", "224.0.0.1", "bad; reboot", "192.0.2.1/99"} {
		if _, ok := NormalizeAccessIP(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestFirewallCommandsSuppressedWhenAPIDisallowsChanges(t *testing.T) {
	h := fixtureHost()
	f := false
	h.Firewall = inspect.Firewall{Known: true, Present: true, UserControllable: &f, IPListUserConfigurable: &f}
	r := Build(h, fixtureBundle(), "Data O'Brien", "192.0.2.1")
	c := commandText(r)
	if strings.Contains(c, "--allowed-all false") || strings.Contains(c, "allowedip add") || strings.Contains(c, "-e true") {
		t.Fatalf("generated forbidden firewall modifications: %s", c)
	}
	if !strings.Contains(r.Markdown, "нельзя изменять") || !strings.Contains(r.Markdown, "не управляется пользователем") {
		t.Fatal("missing explanation of firewall restriction")
	}
}
