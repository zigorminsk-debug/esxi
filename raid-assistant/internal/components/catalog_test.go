package components

import (
	"net/url"
	"strings"
	"testing"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
)

func TestCatalogPresenceAndFixedOfficialSources(t *testing.T) {
	items := List(nil)
	if len(items) < 20 {
		t.Fatalf("catalog unexpectedly short: %d", len(items))
	}
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || ids[item.ID] {
			t.Fatalf("invalid or duplicate ID: %+v", item)
		}
		ids[item.ID] = true
		u, err := url.Parse(item.Source)
		if err != nil || u.Scheme != "https" || u.Host == "" || item.Purpose == "" || item.Requirements == "" || Link(item.ID) != item.Source {
			t.Fatalf("unusable item: %+v", item)
		}
		if len(item.VIBNames) > 0 && item.Presence != "unknown" {
			t.Fatalf("missing inventory cannot prove absence: %+v", item)
		}
	}
	h := &inspect.Host{VIBInventoryKnown: true, VIBs: []inspect.VIB{{Name: "BNXTNET", Version: "test"}, {Name: "vmware-hbr-agent", Version: "test"}}}
	found := map[string]Item{}
	for _, item := range List(h) {
		found[item.ID] = item
	}
	if found["bnxt"].Presence != "found" || len(found["bnxt"].Found) != 1 || found["replication"].Presence != "found" || found["lsa"].Presence != "not-seen" || found["snmp"].Presence != "not-vib" {
		t.Fatalf("wrong presence: %+v", found)
	}
	if !strings.Contains(Link("snmp"), "/snmp-and-vsphere/") || Link("https://evil.invalid") != "" {
		t.Fatal("source allowlist invalid")
	}
}
