// Package inspect performs read-only ESXi discovery and conservative matching.
package inspect

import "time"

// Certificate is retrieved without sending any login information.
type Certificate struct {
	Address    string    `json:"address"`
	Subject    string    `json:"subject"`
	Issuer     string    `json:"issuer"`
	SHA256     string    `json:"sha256"`
	NotBefore  time.Time `json:"notBefore"`
	NotAfter   time.Time `json:"notAfter"`
	Trusted    bool      `json:"trusted"`
	TrustError string    `json:"trustError,omitempty"`
	ValidTime  bool      `json:"validTime"`
}

type Request struct {
	Address      string `json:"address"`
	User         string `json:"user"`
	Password     string `json:"password"`
	Pin          string `json:"pin"`
	AllowExpired bool   `json:"allowExpired"` // only after explicit user confirmation, with an exact pin
}

type Controller struct {
	Device       string `json:"device"`
	Model        string `json:"model"`
	Driver       string `json:"driver"`
	Status       string `json:"status"`
	PCI          string `json:"pci"`
	Vendor       string `json:"vendor"`
	DeviceID     string `json:"deviceId"`
	SubVendor    string `json:"subVendor"`
	SubDevice    string `json:"subDevice"`
	CatalogModel string `json:"catalogModel,omitempty"`
	ProductURL   string `json:"productUrl,omitempty"`
	OEM          string `json:"oem,omitempty"`
	OEMReason    string `json:"oemReason,omitempty"`
	Match        string `json:"match"` // catalog, oem, unknown
}

type Datastore struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Accessible bool     `json:"accessible"`
	Free       int64    `json:"free"`
	Extents    []string `json:"extents,omitempty"` // canonical VMFS device names
	VMs        []string `json:"vms,omitempty"`     // visible datastore inventory associations
}

type VIB struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Vendor  string `json:"vendor,omitempty"`
}

type Firewall struct {
	Known                  bool     `json:"known"`
	Present                bool     `json:"present"`
	Enabled                bool     `json:"enabled"`
	AllIPs                 *bool    `json:"allIps,omitempty"`
	UserControllable       *bool    `json:"userControllable,omitempty"`
	IPListUserConfigurable *bool    `json:"ipListUserConfigurable,omitempty"`
	AllowedIPs             []string `json:"allowedIps,omitempty"`
	Ports                  []string `json:"ports,omitempty"`
}

type Check struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status"` // ok, warn, unknown, info
	Detail string `json:"detail,omitempty"`
}

type Host struct {
	ScannedAt         time.Time    `json:"scannedAt"`
	Address           string       `json:"address"`
	DisplayHost       string       `json:"displayHost"`
	Name              string       `json:"name"`
	Version           string       `json:"version"`
	Build             string       `json:"build"`
	FullName          string       `json:"fullName"`
	Vendor            string       `json:"vendor"`
	Model             string       `json:"model"`
	SecureBoot        *bool        `json:"secureBoot,omitempty"`
	Controllers       []Controller `json:"controllers"`
	Datastores        []Datastore  `json:"datastores"`
	Checks            []Check      `json:"checks"`
	Firewall          Firewall     `json:"firewall"`
	VIBInventoryKnown bool         `json:"vibInventoryKnown"`
	VIBs              []VIB        `json:"vibs,omitempty"`
	LWAServiceKnown   bool         `json:"lwaServiceKnown"`
	LWAServiceRunning bool         `json:"lwaServiceRunning"`
	LWAVersion        string       `json:"lwaVersion,omitempty"`
	Candidate         *Candidate   `json:"candidate,omitempty"`
	CandidateReason   string       `json:"candidateReason,omitempty"`
	Warnings          []string     `json:"warnings"`
}

type Candidate struct {
	Model       string `json:"model"`
	Version     string `json:"version"`
	ProductURL  string `json:"productUrl"`
	DownloadURL string `json:"downloadUrl"`
	ReadmeURL   string `json:"readmeUrl"`
	DocURL      string `json:"docUrl"`
	Provenance  string `json:"provenance"`
}
