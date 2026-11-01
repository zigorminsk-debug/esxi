package inspect

import (
	"strings"
	"testing"
)

func TestConservativeCatalog(t *testing.T) {
	known := []struct{ dev, sub, model string }{{"10e2", "4010", "MegaRAID 9560-8i"}, {"10e2", "4020", "MegaRAID 9580-8i8e"}, {"10e6", "40d8", "MegaRAID 9524-8i"}, {"10e6", "40da", "MegaRAID 9520-2M2"}}
	for _, k := range known {
		c := Controller{Vendor: "1000", DeviceID: k.dev, SubVendor: "1000", SubDevice: k.sub, Device: "vmhba0", Driver: "lsi_mr3", Status: "online"}
		CatalogMatch(&c)
		if c.Match != "catalog" || c.CatalogModel != k.model || ChooseCandidate("8.0.3", []Controller{c}) == nil {
			t.Fatalf("expected candidate for %+v", c)
		}
		for _, ver := range []string{"8.0.0", "8.0", "8.1.0", "7.0.3", "9.0.0", "unknown"} {
			if ChooseCandidate(ver, []Controller{c}) != nil {
				t.Fatalf("version %s incorrectly accepted", ver)
			}
		}
		for _, ver := range []string{"8.0.1", "8.0.2", "8.0.3"} {
			if ChooseCandidate(ver, []Controller{c}) == nil {
				t.Fatalf("version %s incorrectly rejected", ver)
			}
		}
		for _, status := range []string{"", "offline", "unknown", "unbound"} {
			x := c
			x.Status = status
			if ChooseCandidate("8.0.3", []Controller{x}) != nil {
				t.Fatalf("status %q accepted", status)
			}
		}
		x := c
		x.Driver = ""
		if ChooseCandidate("8.0.3", []Controller{x}) != nil {
			t.Fatal("missing driver accepted")
		}
		x = c
		x.Device = ""
		if ChooseCandidate("8.0.3", []Controller{x}) != nil {
			t.Fatal("missing HBA accepted")
		}
	}
	for _, c := range []Controller{{Vendor: "1000", DeviceID: "10e2", SubVendor: "1028", SubDevice: "4010"}, {Vendor: "1000", DeviceID: "10e2", SubVendor: "1000", SubDevice: "ffff"}, {Vendor: "1234", DeviceID: "10e2", SubVendor: "1000", SubDevice: "4010"}} {
		CatalogMatch(&c)
		if c.Match == "catalog" {
			t.Fatalf("incorrect match %+v", c)
		}
	}
}

func TestCandidateReasonsAreSpecific(t *testing.T) {
	c := Controller{Vendor: "1000", DeviceID: "10e2", SubVendor: "1000", SubDevice: "4010", CatalogModel: "MegaRAID 9560-8i", Match: "catalog", Status: "unbound"}
	if r := CandidateReason("8.0.0", []Controller{c}); !strings.Contains(r, "U1") {
		t.Fatalf("version reason missing: %s", r)
	}
	if r := CandidateReason("8.0.3", []Controller{c}); !strings.Contains(r, "unbound") || !strings.Contains(r, "драйвер") {
		t.Fatalf("driver reason missing: %s", r)
	}
	oem := Controller{Match: "oem", SubVendor: "1028", SubDevice: "1234"}
	if r := CandidateReason("8.0.3", []Controller{oem}); !strings.Contains(r, "OEM") || !strings.Contains(r, "1028") {
		t.Fatalf("OEM reason missing: %s", r)
	}
}

func TestSharedSmartPQIDriverDoesNotClaimHPE(t *testing.T) {
	c := Controller{Model: "Microchip RAID", Driver: "smartpqi", SubVendor: "9005"}
	CatalogMatch(&c)
	if c.OEM != "" || c.Match == "oem" {
		t.Fatalf("shared driver misclassified: %+v", c)
	}
	c = Controller{Model: "Smart Array P408", Driver: "smartpqi"}
	CatalogMatch(&c)
	if c.OEM != "HPE Smart Array" {
		t.Fatalf("model-based classification lost: %+v", c)
	}
}
