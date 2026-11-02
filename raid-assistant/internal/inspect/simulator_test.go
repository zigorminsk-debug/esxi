package inspect

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// This is an in-process govmomi simulator, not a real ESXi lab.
func TestReadOnlySimulator(t *testing.T) {
	m := simulator.ESX()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	hs := m.Map().AllReference("HostSystem")[0].(*simulator.HostSystem)
	if hs.Hardware == nil || hs.Config == nil || hs.Config.StorageDevice == nil {
		t.Fatal("simulator lacks host configuration")
	}
	secureBoot := true
	if hs.Capability == nil {
		hs.Capability = &types.HostCapability{}
	}
	hs.Capability.UefiSecureBoot = &secureBoot
	hs.Config.Product.Version = "8.0.3"
	hs.Config.Product.Build = "24022510"
	hs.Hardware.PciDevice = append(hs.Hardware.PciDevice, types.HostPciDevice{Id: "0000:18:00.0", ClassId: 0x0104, VendorId: 0x1000, DeviceId: 0x10e2, SubVendorId: 0x1000, SubDeviceId: 0x4010, DeviceName: "MegaRAID 9560-8i"})
	hs.Config.StorageDevice.HostBusAdapter = append(hs.Config.StorageDevice.HostBusAdapter, &types.HostBlockHba{HostHostBusAdapter: types.HostHostBusAdapter{Device: "vmhba99", Driver: "lsi_mr3", Status: "online", Pci: "0000:18:00.0", Model: "MegaRAID 9560-8i"}})
	// Explicit mock VMFS extent and VM inventory; simulator is not a real host.
	const mockNAA = "naa.600e00040010f26027ad7d3730ecb7ad"
	dsRefs := m.Map().AllReference("Datastore")
	vmRefs := m.Map().AllReference("VirtualMachine")
	if len(dsRefs) == 0 || len(vmRefs) == 0 {
		t.Fatal("simulator fixture lacks datastore/VM")
	}
	ds := dsRefs[0].(*simulator.Datastore)
	ds.Info = &types.VmfsDatastoreInfo{DatastoreInfo: *ds.Info.GetDatastoreInfo(), Vmfs: &types.HostVmfsVolume{Extent: []types.HostScsiDiskPartition{{DiskName: mockNAA, Partition: 1}}}}
	ds.Summary.Type = "VMFS"
	ds.Vm = []types.ManagedObjectReference{vmRefs[0].Reference()}
	m.Service.TLS = &tls.Config{}
	srv := m.Service.NewServer()
	defer srv.Close()
	var mu sync.Mutex
	var calls []string
	var executeBodies []string
	m.Map().Handler = func(_ *simulator.Context, method *simulator.Method) (mo.Reference, types.BaseMethodFault) {
		mu.Lock()
		calls = append(calls, method.Name)
		if method.Name == "ExecuteSoap" {
			b, _ := json.Marshal(method.Body)
			executeBodies = append(executeBodies, string(b))
		}
		mu.Unlock()
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cert, err := PeekCertificate(ctx, srv.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Address: srv.URL.Host, User: "user", Password: "pass", Pin: cert.SHA256}
	got, err := Scan(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name == "" || got.Version != "8.0.3" || got.Candidate == nil || got.Candidate.Model != "MegaRAID 9560-8i" {
		t.Fatalf("unexpected simulated host: %+v", got)
	}
	if got.SecureBoot == nil || !*got.SecureBoot {
		t.Fatal("firmware-reported Secure Boot status missing")
	}
	if len(got.Datastores) == 0 {
		t.Fatal("simulator datastore was not collected")
	}
	matched := false
	for _, ds := range got.Datastores {
		if len(ds.Extents) > 0 && ds.Extents[0] == mockNAA && len(ds.VMs) > 0 && ds.VMs[0] != "" {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("mock VMFS extent / VM association was not collected: %+v", got.Datastores)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), `"password"`) || strings.Contains(string(b), `"pass"`) {
		t.Fatal("result leaked credentials")
	}
	mu.Lock()
	defer mu.Unlock()
	allowed := map[string]bool{"RetrieveServiceContent": true, "Login": true, "Logout": true, "CreateContainerView": true, "DestroyView": true, "RetrieveProperties": true, "RetrievePropertiesEx": true, "ContinueRetrievePropertiesEx": true, "RetrieveManagedMethodExecuter": true, "RetrieveDynamicTypeManager": true, "ExecuteSoap": true}
	for _, call := range calls {
		if !allowed[call] {
			t.Errorf("unexpected SOAP operation: %s; all=%v", call, calls)
		}
	}
	for _, body := range executeBodies {
		var operation struct {
			Method   string
			Argument []struct{ Val string }
		}
		if err := json.Unmarshal([]byte(body), &operation); err != nil {
			t.Fatal(err)
		}
		switch operation.Method {
		case "vim.EsxCLI.software.vib.list", "vim.EsxCLI.software.acceptance.get":
		case "vim.CLIInfo.FetchCLIInfo":
			if len(operation.Argument) != 1 || (!strings.Contains(operation.Argument[0].Val, "vim.EsxCLI.software.vib") && !strings.Contains(operation.Argument[0].Val, "vim.EsxCLI.software.acceptance")) {
				t.Fatalf("unexpected CLI namespace %s", body)
			}
		default:
			t.Fatalf("unexpected privileged CLI operation %s", operation.Method)
		}
	}
	t.Logf("simulated read-only API calls: %v", calls)
}

func TestSimulatorRejectsVCenterBeforeLogin(t *testing.T) {
	m := simulator.VPX()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	m.Service.TLS = &tls.Config{}
	s := m.Service.NewServer()
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cert, err := PeekCertificate(ctx, s.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var calls []string
	m.Map().Handler = func(_ *simulator.Context, method *simulator.Method) (mo.Reference, types.BaseMethodFault) {
		mu.Lock()
		calls = append(calls, method.Name)
		mu.Unlock()
		return nil, nil
	}
	_, err = Scan(ctx, Request{Address: s.URL.Host, User: "user", Password: "pass", Pin: cert.SHA256})
	if err == nil || !strings.Contains(fmt.Sprint(err), "vCenter") {
		t.Fatalf("vCenter was not rejected: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, call := range calls {
		if call == "Login" {
			t.Fatal("credentials were sent to vCenter")
		}
	}
}

func TestSimulatorRejectsWrongPinBeforeAnySOAP(t *testing.T) {
	m := simulator.ESX()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	m.Service.TLS = &tls.Config{}
	server := m.Service.NewServer()
	defer server.Close()
	var mu sync.Mutex
	var calls []string
	m.Map().Handler = func(_ *simulator.Context, method *simulator.Method) (mo.Reference, types.BaseMethodFault) {
		mu.Lock()
		calls = append(calls, method.Name)
		mu.Unlock()
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Scan(ctx, Request{Address: server.URL.Host, User: "user", Password: "pass", Pin: strings.Repeat("0", 64)})
	if err == nil {
		t.Fatal("wrong pin was accepted")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("SOAP requests before certificate validation: %v", calls)
	}
}

func TestSimulatorLoginFailureAndUnreachableHost(t *testing.T) {
	m := simulator.ESX()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	m.Service.TLS = &tls.Config{}
	// Non-default credentials make the simulator reject incorrect logins.
	m.Service.Listen = &url.URL{Host: "127.0.0.1:0", User: url.UserPassword("fixture-user", "fixture-password")}
	s := m.Service.NewServer()
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cert, err := PeekCertificate(ctx, s.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Trusted {
		t.Fatal("simulator's self-signed certificate should not be system trusted")
	}
	_, err = Scan(ctx, Request{Address: s.URL.Host, User: "fixture-user", Password: "wrong-fixture-password", Pin: cert.SHA256})
	if err == nil || !strings.Contains(err.Error(), "логин, пароль") || strings.Contains(err.Error(), "wrong-fixture-password") {
		t.Fatalf("login error was unclear or leaked a password: %v", err)
	}
	_, err = PeekCertificate(ctx, "127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "IP-адрес") {
		t.Fatalf("unreachable host did not return a useful error: %v", err)
	}
}

// Simulate the PCI/HBA values shown in the user's screenshot; this is not a
// connection to a real ESXi host and does not validate RAID health or LWA.
func TestSimulatorIdentifies9361WithoutSuggestingPackage(t *testing.T) {
	m := simulator.ESX()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	hs := m.Map().AllReference("HostSystem")[0].(*simulator.HostSystem)
	hs.Config.Product.Version = "8.0.3"
	hs.Hardware.PciDevice = append(hs.Hardware.PciDevice, types.HostPciDevice{Id: "0000:17:00.0", ClassId: 0x0104, VendorId: 0x1000, DeviceId: 0x005d, SubVendorId: 0x1000, SubDeviceId: 0x9361 - 0x10000, DeviceName: "MegaRAID SAS Invader Controller"})
	hs.Config.StorageDevice.HostBusAdapter = append(hs.Config.StorageDevice.HostBusAdapter, &types.HostBlockHba{HostHostBusAdapter: types.HostHostBusAdapter{Device: "vmhba2", Driver: "lsi_mr3", Status: "unknown", Pci: "0000:17:00.0", Model: "MegaRAID SAS Invader Controller"}})
	m.Service.TLS = &tls.Config{}
	srv := m.Service.NewServer()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cert, err := PeekCertificate(ctx, srv.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	host, err := Scan(ctx, Request{Address: srv.URL.Host, User: "user", Password: "pass", Pin: cert.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	var found *Controller
	for i := range host.Controllers {
		if host.Controllers[i].PCI == "0000:17:00.0" {
			found = &host.Controllers[i]
		}
	}
	if found == nil || found.Match != "identified" || found.Status != "unknown" || found.CatalogModel != "MegaRAID SAS 9361-8i" {
		t.Fatalf("missing exact simulated adapter: %+v", host.Controllers)
	}
	if host.Candidate != nil || !strings.Contains(host.CandidateReason, "MR 7.38") {
		t.Fatalf("unsafe package candidate: %+v reason=%s", host.Candidate, host.CandidateReason)
	}
}
