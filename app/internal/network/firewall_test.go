package network

import (
	"reflect"
	"testing"

	"github.com/dhwang2/go-proxy/internal/store"
)

func TestDesiredFirewallRules(t *testing.T) {
	st := &store.Store{
		Config: &store.SingboxConfig{
			Inbounds: []store.Inbound{
				{Type: "vless", ListenPort: 443},
				{Type: "trojan", ListenPort: 8443},
				{Type: "tuic", ListenPort: 4433},
				{Type: "shadowsocks", ListenPort: 8388},
				{Type: "anytls", ListenPort: 10443},
			},
		},
		SnellConf: &store.SnellConfig{Values: map[string]string{
			"listen": "0.0.0.0:8444",
			"udp":    "true",
		}},
	}
	rules := desiredFirewallRules(st)

	tcp := collectPortsByProto(rules, "tcp")
	udp := collectPortsByProto(rules, "udp")

	expectedTCP := []int{22, 80, 443, 8388, 8443, 8444, 10443}
	expectedUDP := []int{4433, 8388, 8444}
	if !reflect.DeepEqual(tcp, expectedTCP) {
		t.Fatalf("unexpected tcp ports: got=%v want=%v", tcp, expectedTCP)
	}
	if !reflect.DeepEqual(udp, expectedUDP) {
		t.Fatalf("unexpected udp ports: got=%v want=%v", udp, expectedUDP)
	}
}

func TestJoinPorts(t *testing.T) {
	if got := joinPorts([]int{80, 443, 8443}); got != "80, 443, 8443" {
		t.Fatalf("unexpected joinPorts: %s", got)
	}
}
