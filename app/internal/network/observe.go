package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Address struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Family    string `json:"family"`
	Scope     string `json:"scope"`
	Source    string `json:"source"`
}
type Probe struct {
	State   string `json:"state"`
	Address string `json:"address,omitempty"`
	Source  string `json:"source,omitempty"`
}
type Observation struct {
	Addresses []Address         `json:"addresses"`
	IPv4      Probe             `json:"ipv4"`
	IPv6      Probe             `json:"ipv6"`
	Routes    map[string]string `json:"routes"`
	Complete  bool              `json:"complete"`
	Issues    []string          `json:"issues,omitempty"`
}

func Observe(ctx context.Context, probe bool) (Observation, error) {
	info := Observation{Addresses: []Address{}, IPv4: Probe{State: "not_checked"}, IPv6: Probe{State: "not_checked"}, Routes: map[string]string{}, Complete: true}
	interfaces, err := net.Interfaces()
	if err != nil {
		return info, fmt.Errorf("inspect network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			info.Complete = false
			info.Issues = append(info.Issues, "cannot inspect interface "+iface.Name)
			continue
		}
		for _, addr := range addresses {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			family := "ipv6"
			if ip.To4() != nil {
				family = "ipv4"
			}
			scope := "global"
			switch {
			case ip.IsLoopback():
				scope = "loopback"
			case ip.IsLinkLocalUnicast():
				scope = "link_local"
			case ip.IsPrivate():
				scope = "private"
			case !ip.IsGlobalUnicast():
				scope = "other"
			}
			info.Addresses = append(info.Addresses, Address{Interface: iface.Name, Address: addr.String(), Family: family, Scope: scope, Source: "interface"})
		}
	}
	for family, path := range map[string]string{"ipv4": "/proc/net/route", "ipv6": "/proc/net/ipv6_route"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			info.Complete = false
			info.Issues = append(info.Issues, "cannot inspect "+family+" routes")
			continue
		}
		info.Routes[family] = string(raw)
	}
	if !probe {
		return info, ctx.Err()
	}
	probeCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}
	var wg sync.WaitGroup
	for _, job := range []struct {
		network, endpoint string
		target            *Probe
	}{{"tcp4", "https://api.ipify.org", &info.IPv4}, {"tcp6", "https://api64.ipify.org", &info.IPv6}} {
		wg.Add(1)
		go func(network, endpoint string, target *Probe) {
			defer wg.Done()
			*target = probeAddress(probeCtx, network, endpoint)
		}(job.network, job.endpoint, job.target)
	}
	wg.Wait()
	if info.IPv4.State != "available" || info.IPv6.State != "available" {
		info.Complete = false
	}
	return info, ctx.Err()
}

func probeAddress(ctx context.Context, network, endpoint string) Probe {
	result := Probe{State: "unavailable", Source: endpoint}
	dialer := &net.Dialer{}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address)
	}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.State = "timeout"
		}
		return result
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.State = "timeout"
		}
		return result
	}
	ip := net.ParseIP(strings.TrimSpace(string(raw)))
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || (network == "tcp4") != (ip.To4() != nil) {
		return result
	}
	result.State = "available"
	result.Address = ip.String()
	return result
}
