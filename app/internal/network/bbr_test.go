package network

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestKernelSupportsBBRRelease(t *testing.T) {
	cases := []struct {
		release string
		ok      bool
	}{
		{"4.8.0-1-amd64", false},
		{"4.9.0", true},
		{"5.15.0-102-generic", true},
		{"unknown", false},
		{"", false},
	}
	for _, c := range cases {
		if got := kernelSupportsBBRRelease(c.release); got != c.ok {
			t.Fatalf("kernelSupportsBBRRelease(%q)=%v want %v", c.release, got, c.ok)
		}
	}
}

func TestBBRStatusInfoEnabled(t *testing.T) {
	stubCommands(t, map[string]stubResp{
		"uname -r": {out: "5.15.0-102-generic\n"},
		"sysctl -n net.ipv4.tcp_congestion_control":           {out: "bbr\n"},
		"sysctl -n net.core.default_qdisc":                    {out: "fq\n"},
		"sysctl -n net.ipv4.tcp_available_congestion_control": {out: "reno cubic bbr\n"},
	})
	status, err := BBRStatusInfo(context.Background())
	if err != nil {
		t.Fatalf("BBRStatusInfo error: %v", err)
	}
	if !status.Enabled {
		t.Fatalf("expected enabled bbr status")
	}
	if !status.KernelSupported {
		t.Fatalf("expected kernel supported")
	}
}

func TestEnableBBR(t *testing.T) {
	stubCommands(t, map[string]stubResp{
		"modprobe tcp_bbr": {},
		"sysctl -n net.ipv4.tcp_available_congestion_control": {out: "reno cubic bbr\n"},
		"sysctl -w net.core.default_qdisc=fq":                 {out: "net.core.default_qdisc = fq\n"},
		"sysctl -w net.ipv4.tcp_congestion_control=bbr":       {out: "net.ipv4.tcp_congestion_control = bbr\n"},
		"uname -r": {out: "5.15.0\n"},
		"sysctl -n net.ipv4.tcp_congestion_control": {out: "bbr\n"},
		"sysctl -n net.core.default_qdisc":          {out: "fq\n"},
	})
	status, err := EnableBBR(context.Background())
	if err != nil {
		t.Fatalf("EnableBBR error: %v", err)
	}
	if !status.Enabled {
		t.Fatalf("expected bbr enabled after enable")
	}
}

type stubResp struct {
	out string
	err error
}

func stubCommands(t *testing.T, table map[string]stubResp) {
	t.Helper()
	old := runCommandFn
	runCommandFn = func(_ context.Context, name string, args ...string) (string, error) {
		key := strings.TrimSpace(name + " " + strings.Join(args, " "))
		if resp, ok := table[key]; ok {
			return resp.out, resp.err
		}
		return "", fmt.Errorf("unexpected command: %s", key)
	}
	t.Cleanup(func() { runCommandFn = old })
}
