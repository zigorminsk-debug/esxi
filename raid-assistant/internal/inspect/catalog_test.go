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

func TestCandidateReasonPrefersSmartArrayOverHPESubsystemAHCI(t *testing.T) {
	ahci := Controller{Model: "Intel AHCI", Device: "vmhba1", Vendor: "8086", DeviceID: "1d02", SubVendor: "103c", SubDevice: "18a9", Driver: "vmw_ahci"}
	p420 := Controller{Model: "Smart Array P420i", Device: "vmhba0", Vendor: "103c", DeviceID: "323b", SubVendor: "103c", SubDevice: "3354", Driver: "nhpsa"}
	CatalogMatch(&ahci)
	CatalogMatch(&p420)
	r := CandidateReason("8.0.3", []Controller{ahci, p420})
	if !strings.Contains(r, "P420i") || !strings.Contains(r, "3354") || strings.Contains(r, "18a9") || !strings.Contains(r, "совместимость") {
		t.Fatalf("incorrect controller or overconfident recommendation: %s", r)
	}
}

// The screenshot shows SAS3108 as "Invader", status unknown, and the complete
// 9361 subsystem tuple. Identification must never imply an MR 7.38 candidate.
func Test9361IdentificationDoesNotAuthorizeLSAArchive(t *testing.T) {
	c := Controller{Device: "vmhba2", Model: "MegaRAID SAS Invader Controller", Driver: "lsi_mr3", Status: "unknown", PCI: "0000:17:00.0", Vendor: "1000", DeviceID: "005D", SubVendor: "1000", SubDevice: "9361"}
	CatalogMatch(&c)
	if c.Match != "identified" || c.CatalogModel != "MegaRAID SAS 9361-8i" || c.ProductURL != SAS9361Downloads {
		t.Fatalf("incorrect identification: %+v", c)
	}
	for _, version := range []string{"", "7.0.3", "8.0.0", "8.0.1", "8.0.2", "8.0.3", "9.0.0"} {
		for _, status := range []string{"unknown", "online", "offline", ""} {
			x := c
			x.Status = status
			if candidate := ChooseCandidate(version, []Controller{x}); candidate != nil {
				t.Fatalf("identified-only model selected package for %q/%q: %+v", version, status, candidate)
			}
			r := CandidateReason(version, []Controller{x})
			for _, want := range []string{"9361-8i", "MR 7.38", "не подтверждён"} {
				if !strings.Contains(r, want) {
					t.Fatalf("missing %q for %q: %s", want, version, r)
				}
			}
		}
	}
}

func Test9361RequiresExactSubsystemAndDoesNotShadow95xx(t *testing.T) {
	base := Controller{Device: "vmhba2", Model: "MegaRAID SAS Invader Controller", Driver: "lsi_mr3", Status: "online", Vendor: "1000", DeviceID: "005d", SubVendor: "1000", SubDevice: "9361"}
	for _, mutation := range []func(*Controller){
		func(c *Controller) { c.Vendor = "1028" },
		func(c *Controller) { c.DeviceID = "005e" },
		func(c *Controller) { c.SubVendor = "1028" },
		func(c *Controller) { c.SubDevice = "9360" },
		func(c *Controller) { c.SubDevice = "" },
	} {
		c := base
		CatalogMatch(&c)
		mutation(&c)
		CatalogMatch(&c)
		if c.Match == "identified" || c.CatalogModel != "" || c.ProductURL != "" || ChooseCandidate("8.0.3", []Controller{c}) != nil {
			t.Fatalf("partial or mismatched PCI tuple accepted: %+v", c)
		}
	}
	CatalogMatch(&base)
	other := Controller{Device: "vmhba3", Driver: "lsi_mr3", Status: "online", Vendor: "1000", DeviceID: "10e2", SubVendor: "1000", SubDevice: "4010"}
	CatalogMatch(&other)
	candidate := ChooseCandidate("8.0.3", []Controller{base, other})
	if candidate == nil || candidate.Model != "MegaRAID 9560-8i" {
		t.Fatalf("95xx candidate lost or misattributed: %+v", candidate)
	}
}
