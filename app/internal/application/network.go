package application

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"go-proxy/internal/network"
	"go-proxy/internal/store"
)

func (a *App) NetworkStatus(ctx context.Context, probe bool) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	ports, portErr := network.DesiredFirewallPortsWithBindings(observationCtx, snapshot.Store, snapshot.Bindings)
	info, observationErr := network.Observe(observationCtx, probe)
	if portErr != nil {
		info.Complete = false
		info.Issues = append(info.Issues, "inspect desired ports: "+portErr.Error())
	}
	if observationErr != nil {
		info.Complete = false
		info.Issues = append(info.Issues, observationErr.Error())
	}
	return Result{Data: map[string]any{"network": info, "desired_ports": ports}}, ctx.Err()
}

func (a *App) NetworkBBR(ctx context.Context, enable bool) (Result, error) {
	if !enable {
		enabled, current, err := network.BBRStatus()
		return Result{Data: map[string]any{"enabled": enabled, "current": current}}, err
	}
	return a.Operation(ctx, func() (Result, error) {
		a.Progress("enabling bbr")
		before, _, err := network.BBRStatus()
		if err != nil {
			return Result{}, err
		}
		persisted, readErr := os.ReadFile(network.BBRSysctlPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			return Result{}, readErr
		}
		changed := !before || string(persisted) != "net.core.default_qdisc = fq\nnet.ipv4.tcp_congestion_control = bbr\n"
		if err := network.EnableBBR(ctx); err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "sysctl", Changed: true}
		}
		enabled, current, err := network.BBRStatus()
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
		}
		if !enabled {
			return Result{}, &Error{Code: "activation_failed", Message: "bbr is not active", Stage: "verify", Changed: true}
		}
		return Result{Changed: changed, Data: map[string]any{"enabled": enabled, "current": current}}, nil
	})
}

func (a *App) NetworkFirewallStatus(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	info, err := network.FirewallStatus(observationCtx, snapshot.Store, snapshot.Bindings)
	return Result{Data: info}, err
}

func (a *App) NetworkFirewallApply(ctx context.Context) (Result, error) {
	return a.Operation(ctx, func() (Result, error) {
		a.Progress("applying managed firewall")
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := network.ApplyFirewallConvergence(ctx, snapshot.Store); err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "firewall", Changed: true}
		}
		info, err := network.FirewallStatus(ctx, snapshot.Store, snapshot.Bindings)
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
		}
		return Result{Changed: true, Data: info}, nil
	})
}

func (a *App) NetworkFirewallClear(ctx context.Context) (Result, error) {
	return a.Operation(ctx, func() (Result, error) {
		managed, err := network.FirewallManaged(ctx)
		if err != nil {
			return Result{}, err
		}
		if !managed {
			return Result{Data: map[string]any{"managed": false}}, nil
		}
		a.Progress("removing managed firewall")
		if err := network.RemoveFirewallRules(ctx); err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "firewall", Changed: true}
		}
		return Result{Changed: true, Data: map[string]any{"managed": false}}, nil
	})
}

func (a *App) NetworkFirewallPort(ctx context.Context, port int, transport string, remove bool) (Result, error) {
	if port < 1 || port > 65535 {
		return Result{}, Invalid("port must be between 1 and 65535")
	}
	if transport != "tcp" && transport != "udp" && transport != "both" {
		return Result{}, Invalid("transport must be tcp, udp or both")
	}
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		managed, err := network.FirewallManaged(ctx)
		if err != nil {
			return Result{}, err
		}
		s := snapshot.Store
		if s.Firewall == nil {
			s.Firewall = &store.FirewallConfig{}
		}
		before := append([]store.FirewallPort(nil), s.Firewall.Ports...)
		if remove {
			kept := s.Firewall.Ports[:0]
			for _, entry := range s.Firewall.Ports {
				if entry.Port != port || (transport != "both" && entry.Proto != transport) {
					kept = append(kept, entry)
				}
			}
			s.Firewall.Ports = kept
		} else {
			for _, proto := range []string{"tcp", "udp"} {
				if transport == proto || transport == "both" {
					s.Firewall.Ports = append(s.Firewall.Ports, store.FirewallPort{Proto: proto, Port: port})
				}
			}
		}
		s.Firewall.Normalize()
		changed := !reflect.DeepEqual(before, s.Firewall.Ports)
		if changed {
			s.MarkDirty(store.FileFirewall)
			if err := a.Commit(ctx, snapshot); err != nil {
				return Result{}, err
			}
		}
		if managed {
			a.Progress("reconciling managed firewall")
			if err := network.ApplyFirewallConvergence(ctx, s); err != nil {
				return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "firewall", Changed: changed, Data: map[string]any{"pending": "firewall convergence"}}
			}
		}
		return Result{Changed: changed, Data: map[string]any{"ports": s.Firewall.Ports, "managed": managed}}, nil
	})
}

func (a *App) NetworkFail2Ban(ctx context.Context, action string) (Result, error) {
	if action == "status" {
		observationCtx, cancel := ObservationContext(ctx)
		defer cancel()
		info, err := network.Fail2BanStatus(observationCtx)
		return Result{Data: info}, err
	}
	return a.Operation(ctx, func() (Result, error) {
		a.Progress(fmt.Sprintf("%s managed fail2ban protection", action))
		before, err := network.Fail2BanStatus(ctx)
		if err != nil {
			return Result{}, err
		}
		if action == "disable" && !before.Managed {
			return Result{Data: before}, nil
		}
		changed := true
		switch action {
		case "enable":
			err = network.Fail2BanEnable(ctx)
		case "disable":
			err = network.Fail2BanDisable(ctx)
		default:
			return Result{}, Invalid("unknown fail2ban action")
		}
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "fail2ban", Changed: true}
		}
		info, err := network.Fail2BanStatus(ctx)
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
		}
		if action == "enable" && (!info.Running || !info.SSHJailEnabled) {
			return Result{}, &Error{Code: "activation_failed", Message: "fail2ban sshd jail is not active", Stage: "verify", Changed: true}
		}
		return Result{Changed: changed, Data: info}, nil
	})
}
