package inspect

import (
	"strings"
	"testing"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func TestFirewallDetailsAreReportedWithoutInventingPermissions(t *testing.T) {
	no := false
	source := mo.HostSystem{Config: &types.HostConfigInfo{Firewall: &types.HostFirewallInfo{Ruleset: []types.HostFirewallRuleset{{Key: "lwmd", Enabled: true, UserControllable: &no, IpListUserConfigurable: &no, AllowedHosts: &types.HostFirewallRulesetIpList{AllIp: false, IpAddress: []string{"192.0.2.10"}, IpNetwork: []types.HostFirewallRulesetIpNetwork{{Network: "198.51.100.0", PrefixLength: 24}}}, Rule: []types.HostFirewallRule{{Direction: "inbound", Protocol: "tcp", Port: 35557}, {Direction: "outbound", Protocol: "tcp", Port: 465}}}}}}}
	host := &Host{}
	collectFirewall(source, host)
	if !host.Firewall.Known || !host.Firewall.Present || !host.Firewall.Enabled || host.Firewall.AllIPs == nil || *host.Firewall.AllIPs {
		t.Fatalf("incorrect status: %+v", host.Firewall)
	}
	if host.Firewall.UserControllable == nil || *host.Firewall.UserControllable || host.Firewall.IPListUserConfigurable == nil || *host.Firewall.IPListUserConfigurable {
		t.Fatalf("editability lost: %+v", host.Firewall)
	}
	if strings.Join(host.Firewall.AllowedIPs, ",") != "192.0.2.10,198.51.100.0/24" {
		t.Fatalf("bad IP list: %v", host.Firewall.AllowedIPs)
	}
	for _, p := range host.Firewall.Ports {
		if strings.Contains(p, "-0") {
			t.Fatalf("incorrect single port: %s", p)
		}
	}
	if !strings.Contains(strings.Join(host.Firewall.Ports, " "), "35557") {
		t.Fatalf("missing port: %v", host.Firewall.Ports)
	}
}
