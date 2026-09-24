package store

import (
	"testing"
)

func TestFirewallConfigNormalize(t *testing.T) {
	cfg := &FirewallConfig{
		Ports: []FirewallPort{
			{Proto: "udp", Port: 5353},
			{Proto: "tcp", Port: 443},
			{Proto: "udp", Port: 5353},
			{Proto: "", Port: 8443},
			{Proto: "invalid", Port: 8443},
			{Proto: "udp", Port: 0},
		},
	}

	cfg.Normalize()

	if len(cfg.Ports) != 3 {
		t.Fatalf("len(cfg.Ports) = %d, want 3", len(cfg.Ports))
	}
	want := []FirewallPort{
		{Proto: "tcp", Port: 443},
		{Proto: "udp", Port: 5353},
		{Proto: "tcp", Port: 8443},
	}
	for i, got := range cfg.Ports {
		if got != want[i] {
			t.Fatalf("cfg.Ports[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}
