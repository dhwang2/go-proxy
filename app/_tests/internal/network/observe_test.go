package network

import "testing"

// status probes the public IPv4 only for a host whose interfaces carry a
// private IPv4 and no public one; these two answers make that decision.
func TestObservationFamilyScopes(t *testing.T) {
	nat := Observation{Addresses: []Address{
		{Family: "ipv4", Scope: "loopback", Address: "127.0.0.1/8"},
		{Family: "ipv4", Scope: "private", Address: "10.0.0.2/32"},
		{Family: "ipv6", Scope: "link_local", Address: "fe80::1/64"},
		{Family: "ipv6", Scope: "global", Address: "2001:db8::1/128"},
	}}
	if !nat.HasFamily("ipv4") || nat.HasGlobal("ipv4") {
		t.Fatal("a NATed IPv4 host was not recognised")
	}
	if !nat.HasGlobal("ipv6") {
		t.Fatal("a global IPv6 address was missed")
	}
	v6only := Observation{Addresses: []Address{{Family: "ipv4", Scope: "loopback", Address: "127.0.0.1/8"}}}
	if v6only.HasFamily("ipv4") {
		t.Fatal("loopback counted as an IPv4 address to probe for")
	}
}
