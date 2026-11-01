// Package relations links vendor logical-drive reports to VMFS extents only by
// exact, unique NAA identifiers. It never infers a relationship by capacity,
// device number, controller order, VM name or datastore name.
package relations

import (
	"errors"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/raid"
)

type Link struct {
	Controller string   `json:"controller"`
	Volume     string   `json:"volume"`
	NAA        string   `json:"naa"`
	Datastore  string   `json:"datastore"`
	VMs        []string `json:"vms"`
}
type Result struct {
	Links []Link `json:"links"`
	Note  string `json:"note"`
}

func Correlate(host *inspect.Host, snapshot *raid.Snapshot, confirmed bool) (Result, error) {
	if host == nil || snapshot == nil {
		return Result{}, errors.New("Сначала проверьте ESXi и загрузите отчёт RAID")
	}
	if snapshot.Source != "SSH" && !confirmed {
		return Result{}, errors.New("Подтвердите, что отчёт относится к выбранному хосту и актуален")
	}
	out := Result{Links: []Link{}, Note: "Только уникальное точное совпадение NAA отчёта с VMFS extent. Имена ВМ — видимые связи инвентаря с datastore, а не перечень всех их дисков; NFS, vSAN, RDM и недоступные права могут отсутствовать. Отчёт может устареть."}
	counts := map[string]int{}
	for _, v := range snapshot.Volumes {
		if id := raid.NormalizeNAA(v.NAA); id != "" {
			counts[id]++
		}
	}
	// Multiple visible datastores sharing a disk ID are ambiguous, too. Count
	// each datastore once even if it exposes repeated extents on the same disk.
	stores := map[string]int{}
	for _, ds := range host.Datastores {
		seen := map[string]bool{}
		for _, extent := range ds.Extents {
			if id := raid.NormalizeNAA(extent); id != "" {
				seen[id] = true
			}
		}
		for id := range seen {
			stores[id]++
		}
	}
	for _, v := range snapshot.Volumes {
		id := raid.NormalizeNAA(v.NAA)
		if id == "" || counts[id] != 1 || stores[id] != 1 {
			continue
		}
		for _, ds := range host.Datastores {
			for _, extent := range ds.Extents {
				if raid.NormalizeNAA(extent) == id {
					out.Links = append(out.Links, Link{Controller: v.Controller, Volume: v.ID, NAA: id, Datastore: ds.Name, VMs: append([]string{}, ds.VMs...)})
					break
				}
			}
		}
	}
	return out, nil
}
