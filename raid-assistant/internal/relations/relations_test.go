package relations

import (
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/raid"
	"testing"
)

func TestExactUniqueNAAMappingOnly(t *testing.T) {
	const n = "naa.600e00040010f26027ad7d3730ecb7ad"
	h := &inspect.Host{Datastores: []inspect.Datastore{{Name: "a", Extents: []string{n}, VMs: []string{"vm1"}}, {Name: "partial", Extents: []string{"naa.600e00040010f260"}}}}
	r := &raid.Snapshot{Source: "import", Volumes: []raid.Volume{{ID: "0/0", NAA: n}}}
	if _, e := Correlate(h, r, false); e == nil {
		t.Fatal("unconfirmed import")
	}
	got, e := Correlate(h, r, true)
	if e != nil || len(got.Links) != 1 || got.Links[0].Datastore != "a" || got.Links[0].VMs[0] != "vm1" {
		t.Fatalf("%+v %v", got, e)
	}
	r.Volumes = append(r.Volumes, raid.Volume{ID: "1/1", NAA: n})
	got, e = Correlate(h, r, true)
	if e != nil || len(got.Links) != 0 {
		t.Fatal("ambiguous ID linked")
	}
}

func TestAmbiguousDatastoreExtentIsNotLinked(t *testing.T) {
	const n = "naa.600e00040010f26027ad7d3730ecb7ad"
	h := &inspect.Host{Datastores: []inspect.Datastore{{Name: "a", Extents: []string{n}}, {Name: "b", Extents: []string{n}}}}
	r := &raid.Snapshot{Source: "SSH", Volumes: []raid.Volume{{ID: "0/0", NAA: n}}}
	got, err := Correlate(h, r, false)
	if err != nil || len(got.Links) != 0 {
		t.Fatalf("ambiguous stores linked: %+v %v", got, err)
	}
	h.Datastores = h.Datastores[:1]
	h.Datastores[0].Extents = []string{n, n}
	got, err = Correlate(h, r, false)
	if err != nil || len(got.Links) != 1 {
		t.Fatalf("duplicate within one store rejected: %+v %v", got, err)
	}
}
