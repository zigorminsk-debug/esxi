package raid

import (
	"strings"
	"testing"
)

const storSample = `{"Controllers":[{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{"Basics":{"Controller":0,"Model":"MegaRAID 9560"},"Status":{"Controller Status":"Optimal"},"VD LIST":[{"DG/VD":"0/0","TYPE":"RAID1","State":"Optl","Size":"1 TB"}],"PD LIST":[{"EID:Slt":"252:0","State":"Onln","DG":"0","Size":"1 TB"},{"EID:Slt":"252:1","State":"Dgrd","DG":"0"}],"VD0 Properties":{"SCSI NAA Id":"600e00040010f26027ad7d3730ecb7ad"}}}]}`

func TestStorCLIAndHPE(t *testing.T) {
	r, e := Parse([]byte(storSample), "import")
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Controllers) != 1 || len(r.Volumes) != 1 || len(r.Disks) != 2 || r.Severity != "warning" || r.Volumes[0].NAA != "naa.600e00040010f26027ad7d3730ecb7ad" {
		t.Fatalf("%+v", r)
	}
	h := `HPE Smart Array P408i-a SR Gen10 in Slot 0 (Embedded)
 Array A (SAS, Unused Space: 0 MB)
 logicaldrive 1 (279.37 GB, RAID 1, OK)
 physicaldrive 1I:3:1 (port 1I:box 3:bay 1, SAS, 300 GB, Predictive Failure)
 physicaldrive 1I:3:2 (port 1I:box 3:bay 2, SAS, 300 GB, OK)
 Logical Drive: 1
   Status: OK
   Unique Identifier: 600508B1001C3289B48DE18BC32B6963
`
	r, e = Parse([]byte(h), "import")
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Volumes) != 1 || len(r.Disks) != 2 || r.Severity != "warning" || r.Volumes[0].NAA != "naa.600508b1001c3289b48de18bc32b6963" {
		t.Fatalf("%+v", r)
	}
}
func TestFailureUnknownDuplicatesAndMalformed(t *testing.T) {
	cases := []string{`{"Controllers":[{"Command Status":{"Status":"Failure"}}]}`, `{"Controllers":[]}`, `{"Controllers":[],"Controllers":[]}`, `{"Controllers":[]} garbage`, `garbage`, strings.Repeat("x", MaxReportBytes+1)}
	for _, input := range cases {
		if _, e := Parse([]byte(input), "import"); e == nil {
			t.Fatalf("accepted %s", input[:min(len(input), 100)])
		}
	}
	ok := strings.Replace(storSample, `"State":"Dgrd"`, `"State":"Onln"`, 1)
	r, e := Parse([]byte(ok), "import")
	if e != nil {
		t.Fatal(e)
	}
	if r.Severity != "unknown" {
		t.Fatalf("partial reports must not be globally green: %s", r.Severity)
	}
	if Severity("something-new") != "unknown" || Severity("") != "unknown" {
		t.Fatal("unknown states labelled healthy")
	}
}
func TestNormalizeNAA(t *testing.T) {
	for _, s := range []string{"naa.1234", "naa.0000000000000000", "naa.600e00040010f26027ad7d3730ecb7ag", "disk 600e00040010f26027ad7d3730ecb7ad"} {
		if NormalizeNAA(s) != "" {
			t.Fatal(s)
		}
	}
	if NormalizeNAA("0x600E00040010F26027AD7D3730ECB7AD") != "naa.600e00040010f26027ad7d3730ecb7ad" {
		t.Fatal("NAA")
	}
}

func TestConflictingAliasesAndDetailedPhysicalDrive(t *testing.T) {
	if _, err := Parse([]byte(`{"Controllers":[{"Command Status":{"Status":"Success","status":"Failure"},"Response Data":{}}]}`), "import"); err == nil {
		t.Fatal("case-insensitive conflicting JSON keys accepted")
	}
	input := `HPE Smart Array P408i-a in Slot 0
 array A
 logicaldrive 1 (279 GB, RAID 1, OK)
 Logical Drive: 1
   Status: OK
   Unique Identifier: 600508B1001C3289B48DE18BC32B6963
   physicaldrive 1I:3:1
     Status: Failed
     Size: 300 GB
     Model: Test disk
 unrelated header:
   Status: OK
`
	r, err := Parse([]byte(input), "import")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Volumes) != 1 || r.Volumes[0].State != "OK" || len(r.Disks) != 1 || r.Disks[0].Severity != "critical" || r.Severity != "critical" {
		t.Fatalf("physical status was lost or assigned to logical volume: %+v", r)
	}
}

func TestRepeatedObjectsKeepWorstStateAndConflictingNAAIsNotMapped(t *testing.T) {
	input := `{"Controllers":[{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{
      "A":[{"DG/VD":"0/0","State":"Optl","NAA":"600e00040010f26027ad7d3730ecb7ad"},{"EID:Slt":"1:1","State":"Onln"}],
      "B":[{"DG/VD":"0/0","State":"Failed","NAA":"600e00040010f26027ad7d3730ecb7ae"},{"EID:Slt":"1:1","State":"Onln","Status":"Failed"}]
    }}]}`
	r, err := Parse([]byte(input), "import")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Volumes) != 1 || r.Volumes[0].NAA != "" || r.Volumes[0].Severity != "critical" || len(r.Disks) != 1 || r.Disks[0].Severity != "critical" || r.Severity != "critical" {
		t.Fatalf("conflicting snapshot: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "разные NAA") {
		t.Fatal("NAA conflict was not disclosed")
	}
}

func TestEnrichDetailsWorstStateAndUnambiguousIdentity(t *testing.T) {
	const n1 = "600e00040010f26027ad7d3730ecb7ad"
	const n2 = "600e00040010f26027ad7d3730ecb7ae"
	base := &Snapshot{Controllers: []Controller{{ID: "0"}}, Volumes: []Volume{{Controller: "0", ID: "1", NAA: NormalizeNAA(n1), Severity: "ok"}}, Disks: []Disk{{Controller: "0", ID: "1I:1:1", State: "OK", Severity: "ok"}}}
	details := &Snapshot{Controllers: []Controller{{ID: "0"}}, Volumes: []Volume{{Controller: "0", ID: "1", NAA: NormalizeNAA(n2), State: "Failed", Severity: "critical"}}, Disks: []Disk{{Controller: "0", ID: "1I:1:1", State: "Failed", Severity: "critical"}}}
	EnrichDetails(base, details)
	if base.Severity != "critical" || base.Volumes[0].NAA != "" || base.Disks[0].Severity != "critical" {
		t.Fatalf("bad merge: %+v", base)
	}
	base2 := &Snapshot{Controllers: []Controller{{ID: "0"}, {ID: "0"}}}
	EnrichDetails(base2, details)
	if len(base2.Volumes) != 0 || len(base2.Disks) != 0 {
		t.Fatal("ambiguous controller merged")
	}
}
